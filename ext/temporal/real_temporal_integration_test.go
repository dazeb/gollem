//go:build integration

package temporal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/fugue-labs/gollem/core"
)

type temporalIntegrationModel struct{}

func (m temporalIntegrationModel) ModelName() string {
	return "temporal-integration-model"
}

func (m temporalIntegrationModel) Request(_ context.Context, messages []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	switch countToolReturns(messages) {
	case 0:
		return core.ToolCallResponseWithID("collect_trace_facts", `{"topic":"trace"}`, "call_collect"), nil
	case 1:
		return core.ToolCallResponseWithID("publish_trace_report", `{"channel":"ops","report":"trace facts collected"}`, "call_publish"), nil
	default:
		return core.TextResponse("temporal integration complete"), nil
	}
}

func (m temporalIntegrationModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("streaming is not used by this integration test")
}

type collectTraceFactsParams struct {
	Topic string `json:"topic"`
}

type publishTraceReportParams struct {
	Channel string `json:"channel"`
	Report  string `json:"report"`
}

func TestTemporalAgent_RealServerWorkerHandoffTraceAndCLIReexport(t *testing.T) {
	if os.Getenv("GOLLEM_TEMPORAL_INTEGRATION") != "1" {
		t.Skip("set GOLLEM_TEMPORAL_INTEGRATION=1 to run against a real Temporal server")
	}

	address := getenvDefault("TEMPORAL_ADDRESS", "localhost:7233")
	namespace := getenvDefault("TEMPORAL_NAMESPACE", "default")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatalf("dial Temporal at %s namespace %s: %v", address, namespace, err)
	}
	defer c.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	taskQueue := "gollem-trace-it-" + suffix
	workflowID := "gollem-trace-it-" + suffix
	traceDir := t.TempDir()

	ta := NewTemporalAgent(buildTemporalIntegrationAgent(traceDir),
		WithName("trace-it-"+suffix),
		WithContinueAsNew(ContinueAsNewConfig{MaxMessages: 3}),
		WithActivityConfig(ActivityConfig{
			StartToCloseTimeout: 30 * time.Second,
			MaxRetries:          1,
			InitialInterval:     200 * time.Millisecond,
		}),
	)

	w1 := worker.New(c, taskQueue, worker.Options{})
	if err := ta.Register(w1); err != nil {
		t.Fatalf("register first worker: %v", err)
	}
	if err := w1.Start(); err != nil {
		t.Fatalf("start first worker: %v", err)
	}

	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}, ta.WorkflowName(), WorkflowInput{Prompt: "exercise temporal trace worker handoff"})
	if err != nil {
		w1.Stop()
		t.Fatalf("execute workflow: %v", err)
	}

	status, err := waitForTemporalIntegrationWaiting(ctx, c, workflowID, ta)
	if err != nil {
		w1.Stop()
		t.Fatalf("wait for approval state: %v", err)
	}
	if status.ContinueAsNewCount < 1 {
		w1.Stop()
		t.Fatalf("expected continue-as-new before approval wait, got %d", status.ContinueAsNewCount)
	}
	w1.Stop()

	w2 := worker.New(c, taskQueue, worker.Options{})
	if err := ta.Register(w2); err != nil {
		t.Fatalf("register second worker: %v", err)
	}
	if err := w2.Start(); err != nil {
		t.Fatalf("start second worker: %v", err)
	}
	defer w2.Stop()

	if len(status.PendingApprovals) != 1 {
		t.Fatalf("expected one pending approval, got %+v", status.PendingApprovals)
	}
	approval := status.PendingApprovals[0]
	if err := c.SignalWorkflow(ctx, workflowID, "", ta.ApprovalSignalName(), ApprovalSignal{
		ToolName:   approval.ToolName,
		ToolCallID: approval.ToolCallID,
		Approved:   true,
		Message:    "approved by integration test after worker handoff",
	}); err != nil {
		t.Fatalf("signal approval: %v", err)
	}

	var output WorkflowOutput
	if err := run.Get(ctx, &output); err != nil {
		if currentErr := c.GetWorkflow(ctx, workflowID, "").Get(ctx, &output); currentErr != nil {
			t.Fatalf("get workflow result: initial=%v current=%v", err, currentErr)
		}
	}
	if !output.Completed {
		t.Fatal("expected completed workflow output")
	}
	if output.ContinueAsNewCount < 1 {
		t.Fatalf("expected continue-as-new count, got %d", output.ContinueAsNewCount)
	}
	if output.TemporalWorkflowID != workflowID {
		t.Fatalf("TemporalWorkflowID = %q, want %q", output.TemporalWorkflowID, workflowID)
	}
	if output.TemporalRunID == "" {
		t.Fatal("expected TemporalRunID")
	}
	if len(output.TemporalRunChain) < 2 {
		t.Fatalf("expected at least two Temporal run IDs, got %+v", output.TemporalRunChain)
	}
	if output.TraceExport == nil || output.TraceExport.Succeeded != 1 || output.TraceExport.Failed != 0 {
		t.Fatalf("unexpected trace export status %+v", output.TraceExport)
	}

	result, err := ta.DecodeWorkflowOutput(&output)
	if err != nil {
		t.Fatalf("decode workflow output: %v", err)
	}
	if result.Output != "temporal integration complete" {
		t.Fatalf("unexpected output %q", result.Output)
	}
	if result.Trace == nil {
		t.Fatal("expected decoded trace")
	}
	var modelRequests, toolCalls int
	for _, step := range result.Trace.Steps {
		switch step.Kind {
		case core.TraceModelRequest:
			modelRequests++
		case core.TraceToolCall:
			toolCalls++
		}
	}
	if modelRequests != 3 || toolCalls != 2 {
		t.Fatalf("unexpected trace shape: modelRequests=%d toolCalls=%d steps=%+v", modelRequests, toolCalls, result.Trace.Steps)
	}
	artifact := assertOneTemporalIntegrationTraceArtifact(t, traceDir)
	if artifact.Run.ID != workflowID {
		t.Fatalf("exported artifact run id = %q, want %q", artifact.Run.ID, workflowID)
	}
	for _, kind := range []string{"approval.requested", "wait.started", "wait.resolved", "approval.resolved"} {
		if !temporalIntegrationArtifactHasEvent(artifact, kind) {
			t.Fatalf("exported artifact missing %s: %+v", kind, artifact.Events)
		}
	}

	exportPath := filepath.Join(t.TempDir(), "temporal-reexport.trace.json")
	runTemporalTraceExportCLI(t, ctx, workflowID, address, namespace, exportPath)
	reexport, err := core.ReadTraceArtifactFile(exportPath)
	if err != nil {
		t.Fatalf("read re-exported trace: %v", err)
	}
	if reexport.Run.ID != workflowID || reexport.Run.Mode != "temporal" {
		t.Fatalf("unexpected re-export identity: run=%+v", reexport.Run)
	}
	if reexport.Metadata["temporal_workflow_id"] != workflowID {
		t.Fatalf("re-export missing temporal workflow metadata: %+v", reexport.Metadata)
	}
	for _, kind := range []string{"approval.requested", "wait.started", "wait.resolved", "approval.resolved"} {
		if !temporalIntegrationArtifactHasEvent(reexport, kind) {
			t.Fatalf("re-exported artifact missing %s: %+v", kind, reexport.Events)
		}
	}
}

func buildTemporalIntegrationAgent(traceDir string) *core.Agent[string] {
	collect := core.FuncTool[collectTraceFactsParams]("collect_trace_facts", "collect trace facts", func(_ context.Context, params collectTraceFactsParams) (string, error) {
		return "facts for " + params.Topic, nil
	})
	publish := core.FuncTool[publishTraceReportParams]("publish_trace_report", "publish trace report", func(_ context.Context, params publishTraceReportParams) (string, error) {
		return "published " + params.Report + " to " + params.Channel, nil
	}, core.WithRequiresApproval())
	return core.NewAgent[string](temporalIntegrationModel{},
		core.WithTools[string](collect, publish),
		core.WithTracing[string](),
		core.WithTraceExporter[string](core.NewTraceFileExporter(traceDir)),
	)
}

func countToolReturns(messages []core.ModelMessage) int {
	count := 0
	for _, message := range messages {
		request, ok := message.(core.ModelRequest)
		if !ok {
			continue
		}
		for _, part := range request.Parts {
			if _, ok := part.(core.ToolReturnPart); ok {
				count++
			}
		}
	}
	return count
}

func waitForTemporalIntegrationWaiting(ctx context.Context, c client.Client, workflowID string, ta *TemporalAgent[string]) (WorkflowStatus, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		value, err := c.QueryWorkflow(ctx, workflowID, "", ta.StatusQueryName())
		if err == nil {
			var status WorkflowStatus
			if err := value.Get(&status); err == nil && status.Waiting {
				return status, nil
			}
		}
		select {
		case <-ctx.Done():
			return WorkflowStatus{}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return WorkflowStatus{}, fmt.Errorf("workflow %s did not enter waiting state", workflowID)
}

func assertOneTemporalIntegrationTraceArtifact(t *testing.T, dir string) *core.TraceArtifact {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one exported trace artifact, got %d", len(entries))
	}
	artifact, err := core.ReadTraceArtifactFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != core.TraceArtifactSchemaVersion {
		t.Fatalf("schema version = %q", artifact.SchemaVersion)
	}
	return artifact
}

func temporalIntegrationArtifactHasEvent(artifact *core.TraceArtifact, kind string) bool {
	for _, event := range artifact.Events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func runTemporalTraceExportCLI(t *testing.T, ctx context.Context, workflowID, address, namespace, out string) {
	t.Helper()
	root := temporalIntegrationRepoRoot(t)
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/gollem", "trace", "export",
		"--temporal", workflowID,
		"--address", address,
		"--namespace", namespace,
		"--out", out,
	)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gollem trace export failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
}

func temporalIntegrationRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

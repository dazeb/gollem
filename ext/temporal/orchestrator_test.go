package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

type fakeWorkflowClient struct {
	run          client.WorkflowRun
	err          error
	startOptions client.StartWorkflowOptions
	workflow     interface{}
	args         []interface{}
}

func (c *fakeWorkflowClient) ExecuteWorkflow(_ context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	c.startOptions = options
	c.workflow = workflow
	c.args = append([]interface{}(nil), args...)
	if c.err != nil {
		return nil, c.err
	}
	return c.run, nil
}

type fakeWorkflowRun struct {
	id     string
	runID  string
	output WorkflowOutput
	err    error
}

func (r *fakeWorkflowRun) GetID() string {
	return r.id
}

func (r *fakeWorkflowRun) GetRunID() string {
	return r.runID
}

func (r *fakeWorkflowRun) Get(_ context.Context, valuePtr interface{}) error {
	if r.err != nil {
		return r.err
	}
	if valuePtr == nil {
		return nil
	}
	out, ok := valuePtr.(*WorkflowOutput)
	if !ok {
		return errors.New("unexpected workflow output target")
	}
	*out = r.output
	return nil
}

func (r *fakeWorkflowRun) GetWithOptions(ctx context.Context, valuePtr interface{}, _ client.WorkflowRunGetOptions) error {
	return r.Get(ctx, valuePtr)
}

func TestWorkflowRunner_RunTaskStartsWorkflowAndDecodesOutput(t *testing.T) {
	ta := NewTemporalAgent(core.NewAgent[string](core.NewTestModel(core.TextResponse("unused"))), WithName("workflow-runner"))

	completedAt := time.Unix(10, 0).UTC()
	snapshot, err := core.EncodeRunSnapshot(&core.RunSnapshot{
		RunID:        "orch-run-1",
		ParentRunID:  "parent-ignored",
		Prompt:       "hello temporal",
		Usage:        core.RunUsage{Requests: 2, ToolCalls: 1},
		ToolState:    map[string]any{"counter": map[string]any{"count": 3}},
		Timestamp:    completedAt,
		RunStartTime: completedAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("EncodeRunSnapshot failed: %v", err)
	}

	run := &fakeWorkflowRun{
		id:    "orch-run-1",
		runID: "temporal-execution-run-1",
		output: WorkflowOutput{
			Completed:          true,
			OutputJSON:         json.RawMessage(`"done"`),
			Snapshot:           snapshot,
			ContinueAsNewCount: 2,
			Cost:               &core.RunCost{TotalCost: 1.5, Currency: "USD"},
		},
	}
	client := &fakeWorkflowClient{run: run}
	runner := NewWorkflowRunner(client, ta, "gollem")

	result, err := runner.RunTask(context.Background(), &orchestrator.ClaimedTask{
		Task: &orchestrator.Task{
			ID:    "task-1",
			Input: "hello temporal",
		},
		Run: &orchestrator.RunRef{
			ID:      "orch-run-1",
			TaskID:  "task-1",
			Attempt: 1,
		},
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	if got := client.workflow; got != ta.WorkflowName() {
		t.Fatalf("expected workflow name %q, got %v", ta.WorkflowName(), got)
	}
	if client.startOptions.TaskQueue != "gollem" {
		t.Fatalf("expected task queue %q, got %q", "gollem", client.startOptions.TaskQueue)
	}
	if client.startOptions.ID != "orch-run-1" {
		t.Fatalf("expected workflow ID %q, got %q", "orch-run-1", client.startOptions.ID)
	}
	if !client.startOptions.WorkflowExecutionErrorWhenAlreadyStarted {
		t.Fatal("expected WorkflowExecutionErrorWhenAlreadyStarted=true by default")
	}
	if len(client.args) != 1 {
		t.Fatalf("expected 1 workflow arg, got %d", len(client.args))
	}

	input, ok := client.args[0].(WorkflowInput)
	if !ok {
		t.Fatalf("expected WorkflowInput arg, got %T", client.args[0])
	}
	if input.Prompt != "hello temporal" {
		t.Fatalf("expected prompt %q, got %q", "hello temporal", input.Prompt)
	}
	if input.ParentRunID != "orch-run-1" {
		t.Fatalf("expected ParentRunID %q, got %q", "orch-run-1", input.ParentRunID)
	}

	if result.RunnerRunID != "orch-run-1" {
		t.Fatalf("expected RunnerRunID %q, got %q", "orch-run-1", result.RunnerRunID)
	}
	if result.Output != "done" {
		t.Fatalf("expected output %q, got %v", "done", result.Output)
	}
	if result.Usage.Requests != 2 || result.Usage.ToolCalls != 1 {
		t.Fatalf("unexpected usage %+v", result.Usage)
	}
	if result.CompletedAt != completedAt {
		t.Fatalf("expected CompletedAt %v, got %v", completedAt, result.CompletedAt)
	}
	if got := result.ToolState["counter"].(map[string]any)["count"]; got != 3 {
		t.Fatalf("unexpected tool state %+v", result.ToolState)
	}
	if result.Metadata["temporal_workflow_id"] != "orch-run-1" {
		t.Fatalf("unexpected workflow metadata %+v", result.Metadata)
	}
	if result.Metadata["temporal_run_id"] != "temporal-execution-run-1" {
		t.Fatalf("unexpected run metadata %+v", result.Metadata)
	}
	if result.Metadata["temporal_continue_as_new_count"] != 2 {
		t.Fatalf("unexpected continue-as-new metadata %+v", result.Metadata)
	}
	if result.Metadata["temporal_cost_total"] != 1.5 {
		t.Fatalf("unexpected cost metadata %+v", result.Metadata)
	}
}

func TestWorkflowRunner_CustomBuildersAndClientError(t *testing.T) {
	ta := NewTemporalAgent(core.NewAgent[string](core.NewTestModel(core.TextResponse("unused"))), WithName("workflow-runner-custom"))
	executeErr := errors.New("execute failed")
	wfClient := &fakeWorkflowClient{err: executeErr}

	var (
		builtInput bool
		builtOpts  bool
	)
	runner := NewWorkflowRunner(wfClient, ta, "gollem",
		WithWorkflowInputBuilder[string](func(claim *orchestrator.ClaimedTask) (WorkflowInput, error) {
			builtInput = true
			return WorkflowInput{
				Prompt:      claim.Task.Input + " custom",
				ParentRunID: "parent-custom",
			}, nil
		}),
		WithWorkflowStartOptionsBuilder[string](func(_ *orchestrator.ClaimedTask, options client.StartWorkflowOptions) (client.StartWorkflowOptions, error) {
			builtOpts = true
			options.ID = "custom-workflow-id"
			return options, nil
		}),
	)

	_, err := runner.RunTask(context.Background(), &orchestrator.ClaimedTask{
		Task: &orchestrator.Task{ID: "task-1", Input: "hello"},
		Run:  &orchestrator.RunRef{ID: "orch-run-1"},
	})
	if !errors.Is(err, executeErr) {
		t.Fatalf("expected execute error %v, got %v", executeErr, err)
	}
	if !builtInput || !builtOpts {
		t.Fatalf("expected custom builders to run, builtInput=%v builtOpts=%v", builtInput, builtOpts)
	}
	if len(wfClient.args) != 1 {
		t.Fatalf("expected 1 workflow arg, got %d", len(wfClient.args))
	}
	input := wfClient.args[0].(WorkflowInput)
	if input.Prompt != "hello custom" || input.ParentRunID != "parent-custom" {
		t.Fatalf("unexpected custom workflow input %+v", input)
	}
	if wfClient.startOptions.ID != "custom-workflow-id" {
		t.Fatalf("expected custom workflow ID, got %q", wfClient.startOptions.ID)
	}
}

func TestWorkflowRunner_WithWorkflowStartOptionsMergesDefaults(t *testing.T) {
	ta := NewTemporalAgent(core.NewAgent[string](core.NewTestModel(core.TextResponse("unused"))), WithName("workflow-runner-merge"))
	completedAt := time.Unix(20, 0).UTC()
	snapshot, err := core.EncodeRunSnapshot(&core.RunSnapshot{
		RunID:     "orch-run-2",
		Prompt:    "merged",
		Timestamp: completedAt,
	})
	if err != nil {
		t.Fatalf("EncodeRunSnapshot failed: %v", err)
	}

	wfClient := &fakeWorkflowClient{
		run: &fakeWorkflowRun{
			id:    "orch-run-2",
			runID: "temporal-run-2",
			output: WorkflowOutput{
				Completed:  true,
				OutputJSON: json.RawMessage(`"done"`),
				Snapshot:   snapshot,
			},
		},
	}
	runner := NewWorkflowRunner(wfClient, ta, "gollem",
		WithWorkflowStartOptions[string](client.StartWorkflowOptions{
			WorkflowTaskTimeout: 15 * time.Second,
		}),
	)

	if _, err := runner.RunTask(context.Background(), &orchestrator.ClaimedTask{
		Task: &orchestrator.Task{ID: "task-2", Input: "merged"},
		Run:  &orchestrator.RunRef{ID: "orch-run-2"},
	}); err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	if wfClient.startOptions.TaskQueue != "gollem" {
		t.Fatalf("expected merged task queue %q, got %q", "gollem", wfClient.startOptions.TaskQueue)
	}
	if !wfClient.startOptions.WorkflowExecutionErrorWhenAlreadyStarted {
		t.Fatal("expected duplicate-run protection to remain enabled")
	}
	if wfClient.startOptions.WorkflowTaskTimeout != 15*time.Second {
		t.Fatalf("expected WorkflowTaskTimeout 15s, got %v", wfClient.startOptions.WorkflowTaskTimeout)
	}
}

func TestWorkflowRunner_CompletedAtUsesLegacySnapshotJSON(t *testing.T) {
	ta := NewTemporalAgent(core.NewAgent[string](core.NewTestModel(core.TextResponse("unused"))), WithName("workflow-runner-legacy"))
	completedAt := time.Unix(30, 0).UTC()
	snapshotJSON, err := core.MarshalSnapshot(&core.RunSnapshot{
		RunID:     "orch-run-legacy",
		Prompt:    "legacy",
		Timestamp: completedAt,
	})
	if err != nil {
		t.Fatalf("MarshalSnapshot failed: %v", err)
	}

	wfClient := &fakeWorkflowClient{
		run: &fakeWorkflowRun{
			id:    "orch-run-legacy",
			runID: "temporal-run-legacy",
			output: WorkflowOutput{
				Completed:    true,
				OutputJSON:   json.RawMessage(`"done"`),
				SnapshotJSON: snapshotJSON,
			},
		},
	}
	runner := NewWorkflowRunner(wfClient, ta, "gollem")

	result, err := runner.RunTask(context.Background(), &orchestrator.ClaimedTask{
		Task: &orchestrator.Task{ID: "task-legacy", Input: "legacy"},
		Run:  &orchestrator.RunRef{ID: "orch-run-legacy"},
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if result.CompletedAt != completedAt {
		t.Fatalf("expected CompletedAt %v from legacy snapshot JSON, got %v", completedAt, result.CompletedAt)
	}
}

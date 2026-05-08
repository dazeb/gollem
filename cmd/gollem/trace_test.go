package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
	"github.com/fugue-labs/gollem/ext/tui"
)

func TestParseTraceInputOutput(t *testing.T) {
	input, out, err := parseTraceInputOutput([]string{"legacy.json", "--out", "artifact.json"})
	if err != nil {
		t.Fatalf("parseTraceInputOutput() error = %v", err)
	}
	if input != "legacy.json" {
		t.Fatalf("input = %q, want legacy.json", input)
	}
	if out != "artifact.json" {
		t.Fatalf("out = %q, want artifact.json", out)
	}
}

func TestParseTraceExportArgsTemporal(t *testing.T) {
	t.Setenv("TEMPORAL_ADDRESS", "temporal.example:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "prod")

	opts, err := parseTraceExportArgs([]string{
		"--temporal", "workflow-1",
		"--temporal-run-id", "run-abc",
		"--timeout", "3s",
		"--out", "workflow.trace.json",
	})
	if err != nil {
		t.Fatalf("parseTraceExportArgs() error = %v", err)
	}
	if !opts.temporal {
		t.Fatal("temporal = false, want true")
	}
	if opts.workflowID != "workflow-1" {
		t.Fatalf("workflowID = %q, want workflow-1", opts.workflowID)
	}
	if opts.temporalRunID != "run-abc" {
		t.Fatalf("temporalRunID = %q, want run-abc", opts.temporalRunID)
	}
	if opts.address != "temporal.example:7233" {
		t.Fatalf("address = %q, want temporal.example:7233", opts.address)
	}
	if opts.namespace != "prod" {
		t.Fatalf("namespace = %q, want prod", opts.namespace)
	}
	if opts.timeout.String() != "3s" {
		t.Fatalf("timeout = %s, want 3s", opts.timeout)
	}
	if opts.out != "workflow.trace.json" {
		t.Fatalf("out = %q, want workflow.trace.json", opts.out)
	}
}

func TestParseTraceExportArgsLocalRunIDAndTraceDir(t *testing.T) {
	opts, err := parseTraceExportArgs([]string{
		"run-123",
		"--trace-dir", "/tmp/traces",
		"--out", "run.trace.json",
	})
	if err != nil {
		t.Fatalf("parseTraceExportArgs() error = %v", err)
	}
	if opts.input != "run-123" {
		t.Fatalf("input = %q, want run-123", opts.input)
	}
	if opts.traceDir != "/tmp/traces" {
		t.Fatalf("traceDir = %q, want /tmp/traces", opts.traceDir)
	}
	if opts.out != "run.trace.json" {
		t.Fatalf("out = %q, want run.trace.json", opts.out)
	}
}

func TestExportLocalTraceArtifactFindsRunIDInTraceDir(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := traceutil.FromRunTrace(&core.RunTrace{
		RunID:     "run-lookup",
		Prompt:    "lookup",
		StartTime: start,
		EndTime:   start.Add(time.Second),
		Success:   true,
	}, nil)
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	tracePath := filepath.Join(dir, "trace_run-lookup_20260506T120000.trace.json")
	if err := traceutil.WriteFile(tracePath, artifact); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	got, err := exportLocalTraceArtifact(traceExportOptions{input: "run-lookup", traceDir: dir})
	if err != nil {
		t.Fatalf("exportLocalTraceArtifact() error = %v", err)
	}
	if got.Run.ID != "run-lookup" {
		t.Fatalf("run id = %q, want run-lookup", got.Run.ID)
	}
}

func TestExportLocalTraceArtifactFindsRunIDFromRegistry(t *testing.T) {
	registryDir := t.TempDir()
	artifactDir := t.TempDir()
	t.Setenv("GOLLEM_TRACE_DIR", registryDir)
	start := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := traceutil.FromRunTrace(&core.RunTrace{
		RunID:     "run-registry",
		Prompt:    "registry",
		StartTime: start,
		EndTime:   start.Add(time.Second),
		Success:   true,
	}, nil)
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	tracePath := filepath.Join(artifactDir, "outside.trace.json")
	if err := traceutil.WriteFile(tracePath, artifact); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	if err := writeLocalTraceRegistry(tracePath, "run-registry", "running"); err != nil {
		t.Fatalf("write local trace registry: %v", err)
	}

	got, err := exportLocalTraceArtifact(traceExportOptions{input: "run-registry", traceDir: registryDir})
	if err != nil {
		t.Fatalf("exportLocalTraceArtifact() error = %v", err)
	}
	if got.Run.ID != "run-registry" {
		t.Fatalf("run id = %q, want run-registry", got.Run.ID)
	}
}

func TestParseTraceInspectArgs(t *testing.T) {
	input, limit, err := parseTraceInspectArgs([]string{"run.trace.json", "--events", "12"})
	if err != nil {
		t.Fatalf("parseTraceInspectArgs() error = %v", err)
	}
	if input != "run.trace.json" {
		t.Fatalf("input = %q, want run.trace.json", input)
	}
	if limit != 12 {
		t.Fatalf("limit = %d, want 12", limit)
	}
}

func TestParseTraceViewArgsSupportsCompare(t *testing.T) {
	paths, err := parseTraceViewArgs([]string{"baseline.trace.json", "variant.trace.json"})
	if err != nil {
		t.Fatalf("parseTraceViewArgs() error = %v", err)
	}
	if len(paths) != 2 || paths[0] != "baseline.trace.json" || paths[1] != "variant.trace.json" {
		t.Fatalf("paths = %+v", paths)
	}
}

func TestParseTraceReplayArgsDefaultsStrict(t *testing.T) {
	input, mode, err := parseTraceReplayArgs([]string{"run.trace.json"})
	if err != nil {
		t.Fatalf("parseTraceReplayArgs() error = %v", err)
	}
	if input != "run.trace.json" {
		t.Fatalf("input = %q, want run.trace.json", input)
	}
	if mode != "strict" {
		t.Fatalf("mode = %q, want strict", mode)
	}
}

func TestParseTraceReplayArgsPRDModes(t *testing.T) {
	for _, mode := range []string{"inspect", "strict", "simulated", "fork", "live-reexec"} {
		input, got, err := parseTraceReplayArgs([]string{"run.trace.json", "--mode", mode})
		if err != nil {
			t.Fatalf("parseTraceReplayArgs(%s) error = %v", mode, err)
		}
		if input != "run.trace.json" || got != mode {
			t.Fatalf("parseTraceReplayArgs(%s) = %q/%q", mode, input, got)
		}
	}
}

func TestParseTraceReplayCommandArgsLiveReexecFromEvent(t *testing.T) {
	opts, err := parseTraceReplayCommandArgs([]string{
		"run.trace.json",
		"--mode", "live-reexec",
		"--out", "fork.trace.json",
		"--from-event", "evt_000007",
		"--append-user", "try again",
		"--planner-prompt", "planner prompt",
		"--model", "gpt-next",
		"--tool-policy", "read-only",
		"--evaluator", "unit",
		"--provider", "test",
		"--workdir", "/tmp/work",
		"--timeout", "5m",
		"--run-arg", "--no-code-mode",
	})
	if err != nil {
		t.Fatalf("parseTraceReplayCommandArgs() error = %v", err)
	}
	if opts.input != "run.trace.json" || opts.mode != "live-reexec" || opts.out != "fork.trace.json" {
		t.Fatalf("unexpected replay opts: %+v", opts)
	}
	if opts.fork.FromEventID != "evt_000007" || opts.fork.AppendUser != "try again" || opts.fork.PlannerPrompt != "planner prompt" || opts.fork.Model != "gpt-next" || opts.fork.ToolPolicy != "read-only" || opts.fork.Evaluator != "unit" {
		t.Fatalf("unexpected fork opts: %+v", opts.fork)
	}
	if opts.run.Provider != "test" || opts.run.WorkDir != "/tmp/work" || opts.run.Timeout != "5m" || len(opts.run.ExtraArgs) != 1 || opts.run.ExtraArgs[0] != "--no-code-mode" {
		t.Fatalf("unexpected run opts: %+v", opts.run)
	}
}

func TestParseTraceEvaluateArgs(t *testing.T) {
	input, out, opts, err := parseTraceEvaluateArgs([]string{
		"run.trace.json",
		"--evaluator", "contains-output",
		"--expected", "ok",
		"--out", "evaluated.trace.json",
	})
	if err != nil {
		t.Fatalf("parseTraceEvaluateArgs() error = %v", err)
	}
	if input != "run.trace.json" || out != "evaluated.trace.json" {
		t.Fatalf("input/out = %q/%q", input, out)
	}
	if opts.Evaluator != "contains-output" || opts.Expected != "ok" {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseTraceForkArgs(t *testing.T) {
	input, out, opts, runOpts, err := parseTraceForkArgs([]string{
		"run.trace.json",
		"--from-event", "evt_000007",
		"--from-checkpoint", "snap_000003",
		"--from-kind", "model.responded",
		"--append-user", "try another path",
		"--prompt", "fork prompt",
		"--system-prompt", "system prompt",
		"--planner-prompt", "planner prompt",
		"--model", "gpt-next",
		"--topology", "team",
		"--middleware", "guarded",
		"--tool-policy", "read-only",
		"--evaluator", "tests",
		"--memory-edit", "counter=5",
		"--run-id", "fork-1",
		"--continue",
		"--provider", "test",
		"--workdir", "/tmp/work",
		"--timeout", "5m",
		"--snapshot-out", "fork.materialized.snapshot.json",
		"--run-arg", "--no-code-mode",
		"--out", "fork.snapshot.json",
	})
	if err != nil {
		t.Fatalf("parseTraceForkArgs() error = %v", err)
	}
	if input != "run.trace.json" {
		t.Fatalf("input = %q, want run.trace.json", input)
	}
	if out != "fork.snapshot.json" {
		t.Fatalf("out = %q, want fork.snapshot.json", out)
	}
	if opts.FromEventID != "evt_000007" {
		t.Fatalf("from event = %q, want evt_000007", opts.FromEventID)
	}
	if opts.FromCheckpoint != "snap_000003" {
		t.Fatalf("from checkpoint = %q, want snap_000003", opts.FromCheckpoint)
	}
	if opts.FromKind != "model.responded" {
		t.Fatalf("from kind = %q, want model.responded", opts.FromKind)
	}
	if opts.AppendUser != "try another path" {
		t.Fatalf("append user = %q", opts.AppendUser)
	}
	if opts.Prompt != "fork prompt" {
		t.Fatalf("prompt = %q", opts.Prompt)
	}
	if opts.SystemPrompt != "system prompt" {
		t.Fatalf("system prompt = %q", opts.SystemPrompt)
	}
	if opts.PlannerPrompt != "planner prompt" {
		t.Fatalf("planner prompt = %q", opts.PlannerPrompt)
	}
	if opts.Model != "gpt-next" || opts.Topology != "team" || opts.Middleware != "guarded" || opts.ToolPolicy != "read-only" || opts.Evaluator != "tests" {
		t.Fatalf("unexpected fork overrides: %+v", opts)
	}
	if len(opts.MemoryEdits) != 1 || opts.MemoryEdits[0] != "counter=5" {
		t.Fatalf("memory edits = %+v", opts.MemoryEdits)
	}
	if opts.NewRunID != "fork-1" {
		t.Fatalf("run id = %q", opts.NewRunID)
	}
	if !runOpts.Continue || runOpts.Provider != "test" || runOpts.WorkDir != "/tmp/work" || runOpts.Timeout != "5m" || runOpts.SnapshotOut != "fork.materialized.snapshot.json" {
		t.Fatalf("unexpected run opts: %+v", runOpts)
	}
	if len(runOpts.ExtraArgs) != 1 || runOpts.ExtraArgs[0] != "--no-code-mode" {
		t.Fatalf("extra args = %+v", runOpts.ExtraArgs)
	}
}

func TestContinueTraceForkInvokesRunWithSnapshotAndTraceOut(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "fork.trace.json")
	var gotArgs []string
	oldExec := traceForkRunExec
	traceForkRunExec = func(args []string, _ io.Reader, _ io.Writer, _ io.Writer, _ []string) error {
		gotArgs = append([]string(nil), args...)
		snapshotPath := argAfter(args, "--resume-snapshot")
		if snapshotPath == "" {
			t.Fatalf("missing --resume-snapshot in args: %+v", args)
		}
		if _, err := loadRunSnapshotFile(snapshotPath); err != nil {
			t.Fatalf("load generated snapshot: %v", err)
		}
		traceOut := argAfter(args, "--trace-out")
		if traceOut == "" {
			t.Fatalf("missing --trace-out in args: %+v", args)
		}
		artifact, err := traceutil.FromRunTrace(&core.RunTrace{
			RunID:   "fork-run",
			Success: true,
		}, map[string]any{"resume_trace_policy": "fresh_segment"})
		if err != nil {
			t.Fatalf("build trace: %v", err)
		}
		return traceutil.WriteFile(traceOut, artifact)
	}
	defer func() { traceForkRunExec = oldExec }()

	err := continueTraceFork(out, &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "resume"}}},
		},
		RunID:  "fork-run",
		Prompt: "resume",
	}, traceutil.SnapshotRecord{ID: "snap_000001", Step: 1}, traceForkRunOptions{
		Provider:  "test",
		WorkDir:   tmp,
		Timeout:   "5s",
		ExtraArgs: []string{"--no-code-mode"},
	})
	if err != nil {
		t.Fatalf("continueTraceFork() error = %v", err)
	}
	if got := strings.Join(gotArgs, " "); !strings.Contains(got, "run --resume-snapshot") || !strings.Contains(got, "--provider test") || !strings.Contains(got, "--no-code-mode") {
		t.Fatalf("unexpected run args: %q", got)
	}
	if _, err := traceutil.ReadFile(out); err != nil {
		t.Fatalf("continued trace not written: %v", err)
	}
}

func TestRunTraceReplayLiveReexecContinuesFork(t *testing.T) {
	tmp := t.TempDir()
	tracePath := filepath.Join(tmp, "source.trace.json")
	out := filepath.Join(tmp, "live.trace.json")
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snap := &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "resume"}}},
		},
		RunID:        "live-source",
		RunStep:      1,
		RunStartTime: now,
		Prompt:       "resume",
		Timestamp:    now.Add(time.Second),
	}
	artifact, err := traceutil.FromRunTraceWithSnapshots(&core.RunTrace{RunID: "live-source", StartTime: now, EndTime: now.Add(time.Second), Success: true}, []*core.RunSnapshot{snap}, nil)
	if err != nil {
		t.Fatalf("source trace: %v", err)
	}
	if err := traceutil.WriteFile(tracePath, artifact); err != nil {
		t.Fatalf("write source trace: %v", err)
	}

	var gotArgs []string
	oldExec := traceForkRunExec
	traceForkRunExec = func(args []string, _ io.Reader, _ io.Writer, _ io.Writer, _ []string) error {
		gotArgs = append([]string(nil), args...)
		traceOut := argAfter(args, "--trace-out")
		if traceOut != out {
			t.Fatalf("trace out = %q, want %q in args %+v", traceOut, out, args)
		}
		continued, buildErr := traceutil.FromRunTrace(&core.RunTrace{
			RunID:   "live-fork",
			Success: true,
		}, map[string]any{"resume_trace_policy": "fresh_segment"})
		if buildErr != nil {
			t.Fatalf("build continued trace: %v", buildErr)
		}
		return traceutil.WriteFile(traceOut, continued)
	}
	defer func() { traceForkRunExec = oldExec }()

	if err := runTraceReplay([]string{tracePath, "--mode", "live-reexec", "--from-step", "1", "--append-user", "try live", "--provider", "test", "--out", out}); err != nil {
		t.Fatalf("runTraceReplay(live-reexec) error = %v", err)
	}
	if got := strings.Join(gotArgs, " "); !strings.Contains(got, "run --resume-snapshot") || !strings.Contains(got, "--provider test") {
		t.Fatalf("unexpected run args: %q", got)
	}
	if _, err := traceutil.ReadFile(out); err != nil {
		t.Fatalf("continued live trace not written: %v", err)
	}
}

func TestCLITraceFileExporterWritesFailedRunTrace(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "failed.trace.json")
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := traceutil.NewRuntimeRecorder(bus)
	defer recorder.Close()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	core.Publish(bus, core.RunStartedEvent{RunID: "child-run", ParentRunID: "failed-run", StartedAt: now.Add(50 * time.Millisecond)})
	core.Publish(bus, core.ErrorRaisedEvent{RunID: "child-run", ParentRunID: "failed-run", TurnNumber: 1, Error: "child model failed", RaisedAt: now.Add(100 * time.Millisecond)})

	exporter := &cliTraceFileExporter{
		path: out,
		metadata: buildCLITraceMetadata(flags{
			provider:   "test",
			modelName:  "test-model",
			teamMode:   "auto",
			workDir:    tmp,
			toolPolicy: "read-only",
			evaluator:  "unit",
		}, &core.RunSnapshot{
			ParentRunID:      "parent-run",
			RunStep:          4,
			SourceTraceRunID: "source-run",
			SourceSnapshotID: "snap_000001",
		}, "resume.snapshot.json"),
		snapshots: func() []*core.RunSnapshot {
			return []*core.RunSnapshot{{
				Messages: []core.ModelMessage{
					core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "debug failure"}}},
				},
				RunID:     "failed-run",
				RunStep:   1,
				Prompt:    "debug failure",
				Timestamp: now.Add(200 * time.Millisecond),
			}}
		},
		recorder: recorder,
	}
	err := exporter.Export(context.Background(), &core.RunTrace{
		RunID:     "failed-run",
		Prompt:    "debug failure",
		StartTime: now,
		EndTime:   now.Add(time.Second),
		Duration:  time.Second,
		Success:   false,
		Error:     "model failed",
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	artifact, err := traceutil.ReadFile(out)
	if err != nil {
		t.Fatalf("read exported failed trace: %v", err)
	}
	if artifact.Summary.Status != "failed" || artifact.Summary.Error != "model failed" {
		t.Fatalf("unexpected failed summary: %+v", artifact.Summary)
	}
	if len(artifact.Snapshots) != 1 {
		t.Fatalf("snapshots = %+v, want one snapshot", artifact.Snapshots)
	}
	if artifact.Metadata["tool_policy"] != "read-only" || artifact.Metadata["resume_trace_policy"] != "fresh_segment" {
		t.Fatalf("metadata = %+v", artifact.Metadata)
	}
	var sawError bool
	for _, event := range artifact.Events {
		if event.Kind == "error.raised" && event.AgentID == "child-run" {
			sawError = true
			break
		}
	}
	if !sawError {
		t.Fatalf("missing runtime error event: %+v", artifact.Events)
	}
}

func TestWriteCLIPartialTraceWritesRunningArtifact(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "partial.trace.json")
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snap := &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "keep going", Timestamp: now}}, Timestamp: now},
		},
		RunID:        "run-partial",
		RunStep:      1,
		RunStartTime: now,
		Prompt:       "keep going",
		Timestamp:    now.Add(time.Second),
		Usage:        core.RunUsage{Usage: core.Usage{InputTokens: 3, OutputTokens: 4}, Requests: 1},
	}

	if err := writeCLIPartialTrace(out, map[string]any{"provider": "test"}, []*core.RunSnapshot{snap}, nil, snap); err != nil {
		t.Fatalf("writeCLIPartialTrace() error = %v", err)
	}
	artifact, err := traceutil.ReadFile(out)
	if err != nil {
		t.Fatalf("read partial trace: %v", err)
	}
	if artifact.Summary.Status != "running" || artifact.Summary.Success {
		t.Fatalf("partial summary = %+v, want running unsuccessful", artifact.Summary)
	}
	if artifact.Metadata["partial"] != true || artifact.Metadata["partial_reason"] != "running" {
		t.Fatalf("partial metadata = %+v", artifact.Metadata)
	}
	if len(artifact.Snapshots) != 1 {
		t.Fatalf("partial snapshots = %+v, want one", artifact.Snapshots)
	}
	for _, event := range artifact.Events {
		if event.Kind == "run.failed" || event.Kind == "run.completed" {
			t.Fatalf("partial trace should not emit terminal run event: %+v", artifact.Events)
		}
	}
}

func TestCLITraceExporterRunsOnAgentErrorWithoutRunResult(t *testing.T) {
	out := filepath.Join(t.TempDir(), "agent-error.trace.json")
	agent := core.NewAgent[string](
		core.NewTestModel(),
		core.WithTraceExporter[string](&cliTraceFileExporter{path: out, metadata: map[string]any{"provider": "test"}}),
	)

	result, err := agent.Run(context.Background(), "fail before result")
	if err == nil {
		t.Fatal("expected agent error")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil on model error", result)
	}
	artifact, readErr := traceutil.ReadFile(out)
	if readErr != nil {
		t.Fatalf("trace artifact was not written for failed run: %v", readErr)
	}
	if artifact.Summary.Status != "failed" || artifact.Summary.Error == "" {
		t.Fatalf("unexpected failed trace summary: %+v", artifact.Summary)
	}
}

func TestTraceForkRunExecDefaultExecutesProcess(t *testing.T) {
	if os.Getenv("GOLLEM_TRACE_TEST_HELPER") == "1" {
		return
	}
	var stdout, stderr bytes.Buffer
	env := append(os.Environ(), "GOLLEM_TRACE_TEST_HELPER=1")
	if err := traceForkRunExec([]string{"-test.run=TestTraceForkRunExecDefaultExecutesProcess"}, nil, &stdout, &stderr, env); err != nil {
		t.Fatalf("traceForkRunExec() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
}

func TestRunTraceSleepyWritesEvidence(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "base.trace.json")
	candidate := filepath.Join(tmp, "candidate.trace.json")
	out := filepath.Join(tmp, "sleepy.json")
	baseArtifact, err := traceutil.FromRunTrace(&core.RunTrace{RunID: "base", Success: true}, nil)
	if err != nil {
		t.Fatalf("base trace: %v", err)
	}
	candidateArtifact, err := traceutil.FromRunTrace(&core.RunTrace{RunID: "candidate", Success: true}, map[string]any{"sleepy_candidate_id": "cand-1"})
	if err != nil {
		t.Fatalf("candidate trace: %v", err)
	}
	if err := traceutil.WriteFile(base, baseArtifact); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := traceutil.WriteFile(candidate, candidateArtifact); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	if err := runTraceSleepy([]string{base, candidate, "--out", out}); err != nil {
		t.Fatalf("runTraceSleepy() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if !bytes.Contains(data, []byte(traceutil.SleepyEvidenceSchemaVersion)) || !bytes.Contains(data, []byte("cand-1")) {
		t.Fatalf("unexpected evidence:\n%s", string(data))
	}
}

func TestRunTraceEvaluateWritesEvaluatorEvidence(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "run.trace.json")
	out := filepath.Join(tmp, "evaluated.trace.json")
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := traceutil.FromRunTrace(&core.RunTrace{
		RunID:     "eval-run",
		StartTime: now,
		EndTime:   now.Add(time.Second),
		Success:   true,
		Steps: []core.TraceStep{{
			Kind:      core.TraceModelResponse,
			Timestamp: now.Add(500 * time.Millisecond),
			Data:      map[string]any{"text": "all good"},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("build trace: %v", err)
	}
	if err := traceutil.WriteFile(input, artifact); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	if err := runTraceEvaluate([]string{input, "--evaluator", "contains-output", "--expected", "good", "--out", out}); err != nil {
		t.Fatalf("runTraceEvaluate() error = %v", err)
	}
	evaluated, err := traceutil.ReadFile(out)
	if err != nil {
		t.Fatalf("read evaluated trace: %v", err)
	}
	if evaluated.Summary.Evaluator == nil || evaluated.Summary.Evaluator.Passed == nil || !*evaluated.Summary.Evaluator.Passed {
		t.Fatalf("unexpected evaluator summary: %+v", evaluated.Summary.Evaluator)
	}
	if countTraceEvents(evaluated.Events, "evaluator.completed") != 1 {
		t.Fatalf("missing evaluator.completed event: %+v", evaluated.Events)
	}
}

func TestTraceParsersRejectMissingValues(t *testing.T) {
	for _, flag := range []string{
		"--out", "--snapshot-out", "--provider", "--workdir", "--timeout", "--run-arg", "--from-step",
		"--from-event", "--from-checkpoint", "--from-kind", "--run-id", "--prompt", "--system-prompt",
		"--planner-prompt", "--append-user", "--model", "--topology", "--middleware", "--tool-policy",
		"--evaluator", "--memory-edit",
	} {
		t.Run("fork "+flag, func(t *testing.T) {
			if _, _, _, _, err := parseTraceForkArgs([]string{"run.trace.json", flag}); err == nil {
				t.Fatalf("expected error for %s", flag)
			}
		})
	}
	for _, flag := range []string{"--out", "--temporal", "--temporal-run-id", "--address", "--namespace", "--timeout", "--trace-dir"} {
		t.Run("export "+flag, func(t *testing.T) {
			if _, err := parseTraceExportArgs([]string{flag}); err == nil {
				t.Fatalf("expected error for %s", flag)
			}
		})
	}
	for _, flag := range []string{"--out", "--key", "--pattern", "--replacement"} {
		t.Run("redact "+flag, func(t *testing.T) {
			if _, _, _, err := parseTraceRedactArgs([]string{"run.trace.json", flag}); err == nil {
				t.Fatalf("expected error for %s", flag)
			}
		})
	}
	for _, flag := range []string{"--out", "--payload-limit", "--keep-snapshots"} {
		t.Run("compact "+flag, func(t *testing.T) {
			if _, _, _, err := parseTraceCompactArgs([]string{"run.trace.json", flag}); err == nil {
				t.Fatalf("expected error for %s", flag)
			}
		})
	}
	if _, _, err := parseTraceInspectArgs([]string{"run.trace.json", "--events"}); err == nil {
		t.Fatal("expected inspect events missing value error")
	}
	if _, _, err := parseTraceInspectArgs([]string{"run.trace.json", "--events", "bad"}); err == nil {
		t.Fatal("expected inspect events invalid value error")
	}
	if _, _, err := parseTraceReplayArgs([]string{"run.trace.json", "--mode"}); err == nil {
		t.Fatal("expected replay mode missing value error")
	}
	for _, flag := range []string{"--out", "--evaluator", "--expected"} {
		t.Run("evaluate "+flag, func(t *testing.T) {
			if _, _, _, err := parseTraceEvaluateArgs([]string{"run.trace.json", flag}); err == nil {
				t.Fatalf("expected error for %s", flag)
			}
		})
	}
	if _, _, _, err := parseTraceEvaluateArgs([]string{"run.trace.json"}); err == nil {
		t.Fatal("expected evaluate missing evaluator error")
	}
	if _, _, _, err := parseTraceRedactArgs([]string{"run.trace.json", "--drop-trace=maybe"}); err == nil {
		t.Fatal("expected redact bool parse error")
	}
	if _, _, _, err := parseTraceCompactArgs([]string{"run.trace.json", "--drop-trace=maybe"}); err == nil {
		t.Fatal("expected compact bool parse error")
	}
}

func TestTraceParsersReadPromptFilesAndRejectDuplicates(t *testing.T) {
	tmp := t.TempDir()
	promptFile := filepath.Join(tmp, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("prompt from file"), 0o600); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	_, _, opts, _, err := parseTraceForkArgs([]string{
		"run.trace.json",
		"--system-prompt", promptFile,
		"--planner-prompt", promptFile,
	})
	if err != nil {
		t.Fatalf("parseTraceForkArgs(file prompts) error = %v", err)
	}
	if opts.SystemPrompt != "prompt from file" || opts.PlannerPrompt != "prompt from file" {
		t.Fatalf("prompt file contents not read: %+v", opts)
	}
	for _, tt := range []struct {
		name string
		fn   func() error
	}{
		{"export duplicate", func() error { _, err := parseTraceExportArgs([]string{"a.trace.json", "b.trace.json"}); return err }},
		{"inspect duplicate", func() error { _, _, err := parseTraceInspectArgs([]string{"a.trace.json", "b.trace.json"}); return err }},
		{"replay duplicate", func() error { _, _, err := parseTraceReplayArgs([]string{"a.trace.json", "b.trace.json"}); return err }},
		{"evaluate duplicate", func() error {
			_, _, _, err := parseTraceEvaluateArgs([]string{"a.trace.json", "b.trace.json", "--evaluator", "status-succeeded"})
			return err
		}},
		{"view too many", func() error {
			_, err := parseTraceViewArgs([]string{"a.trace.json", "b.trace.json", "c.trace.json"})
			return err
		}},
		{"redact duplicate", func() error {
			_, _, _, err := parseTraceRedactArgs([]string{"a.trace.json", "b.trace.json"})
			return err
		}},
		{"compact duplicate", func() error {
			_, _, _, err := parseTraceCompactArgs([]string{"a.trace.json", "b.trace.json"})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("expected duplicate/arity error")
			}
		})
	}
}

func TestRunTraceFileCommands(t *testing.T) {
	tmp := t.TempDir()
	start := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	base := filepath.Join(tmp, "base.trace.json")
	variant := filepath.Join(tmp, "variant.trace.json")
	exported := filepath.Join(tmp, "exported.trace.json")
	redacted := filepath.Join(tmp, "redacted.trace.json")
	compacted := filepath.Join(tmp, "compacted.trace.json")
	evaluated := filepath.Join(tmp, "evaluated.trace.json")
	forkTrace := filepath.Join(tmp, "fork-source.trace.json")
	forkSnapshot := filepath.Join(tmp, "fork.snapshot.json")

	baseArtifact, err := traceutil.FromRunTrace(&core.RunTrace{
		RunID:     "base-run",
		Prompt:    "hello secret",
		StartTime: start,
		EndTime:   start.Add(time.Second),
		Success:   true,
		Usage: core.RunUsage{
			Usage:    core.Usage{InputTokens: 5, OutputTokens: 3},
			Requests: 1,
		},
		Requests: []core.RequestTrace{{RequestID: "base/req-1", TurnNumber: 1, Response: &core.RequestTraceResponse{Usage: core.Usage{InputTokens: 5, OutputTokens: 3}}}},
		Steps: []core.TraceStep{
			{Kind: core.TraceModelRequest, Timestamp: start, Data: map[string]any{"message_count": 1}},
			{Kind: core.TraceModelResponse, Timestamp: start.Add(100 * time.Millisecond), Data: map[string]any{"text": "ok secret"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("base trace: %v", err)
	}
	variantTrace := *baseArtifact.Trace
	variantTrace.RunID = "variant-run"
	variantTrace.Usage.OutputTokens += 1
	variantArtifact, err := traceutil.FromRunTrace(&variantTrace, nil)
	if err != nil {
		t.Fatalf("variant trace: %v", err)
	}
	if err := traceutil.WriteFile(base, baseArtifact); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := traceutil.WriteFile(variant, variantArtifact); err != nil {
		t.Fatalf("write variant: %v", err)
	}

	if err := runTraceExport([]string{base, "--out", exported}); err != nil {
		t.Fatalf("runTraceExport() error = %v", err)
	}
	if _, err := traceutil.ReadFile(exported); err != nil {
		t.Fatalf("read exported trace: %v", err)
	}
	if err := runTraceValidate([]string{exported}); err != nil {
		t.Fatalf("runTraceValidate() error = %v", err)
	}
	if err := runTraceInspect([]string{exported, "--events", "2"}); err != nil {
		t.Fatalf("runTraceInspect() error = %v", err)
	}
	if err := runTraceReplay([]string{exported, "--mode", "simulated"}); err != nil {
		t.Fatalf("runTraceReplay() error = %v", err)
	}
	if err := runTraceDiff([]string{base, variant, "--format", "json"}); err != nil {
		t.Fatalf("runTraceDiff() error = %v", err)
	}
	if err := runTraceRegress([]string{base, variant, "--format", "json", "--require-status", "succeeded", "--max-token-delta", "10"}); err != nil {
		t.Fatalf("runTraceRegress() error = %v", err)
	}
	if err := runTraceEvaluate([]string{base, "--evaluator", "contains-output", "--expected", "ok", "--out", evaluated}); err != nil {
		t.Fatalf("runTraceEvaluate() error = %v", err)
	}
	if _, err := traceutil.ReadFile(evaluated); err != nil {
		t.Fatalf("read evaluated trace: %v", err)
	}
	if err := runTraceRedact([]string{base, "--pattern", "secret", "--out", redacted}); err != nil {
		t.Fatalf("runTraceRedact() error = %v", err)
	}
	redactedData, err := os.ReadFile(redacted)
	if err != nil {
		t.Fatalf("read redacted: %v", err)
	}
	if bytes.Contains(redactedData, []byte("hello secret")) || bytes.Contains(redactedData, []byte("ok secret")) {
		t.Fatalf("redacted output still contains unredacted content:\n%s", string(redactedData))
	}
	if err := runTraceCompact([]string{base, "--payload-limit", "16", "--out", compacted}); err != nil {
		t.Fatalf("runTraceCompact() error = %v", err)
	}
	if _, err := traceutil.ReadFile(compacted); err != nil {
		t.Fatalf("read compacted: %v", err)
	}

	snap := &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "resume"}}},
		},
		RunID:        "fork-source",
		RunStep:      1,
		RunStartTime: start,
		Prompt:       "resume",
		Timestamp:    start.Add(time.Second),
	}
	forkArtifact, err := traceutil.FromRunTraceWithSnapshots(&core.RunTrace{RunID: "fork-source", StartTime: start, Success: true}, []*core.RunSnapshot{snap}, nil)
	if err != nil {
		t.Fatalf("fork source trace: %v", err)
	}
	if err := traceutil.WriteFile(forkTrace, forkArtifact); err != nil {
		t.Fatalf("write fork trace: %v", err)
	}
	if err := runTraceFork([]string{forkTrace, "--from-step", "1", "--append-user", "branch", "--out", forkSnapshot}); err != nil {
		t.Fatalf("runTraceFork() error = %v", err)
	}
	if _, err := loadRunSnapshotFile(forkSnapshot); err != nil {
		t.Fatalf("load fork snapshot: %v", err)
	}
}

func TestDispatchTraceCommandRoutesSubcommands(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "base.trace.json")
	variant := filepath.Join(tmp, "variant.trace.json")
	exported := filepath.Join(tmp, "exported.trace.json")
	redacted := filepath.Join(tmp, "redacted.trace.json")
	compacted := filepath.Join(tmp, "compacted.trace.json")
	sleepyOut := filepath.Join(tmp, "sleepy.json")
	evaluated := filepath.Join(tmp, "evaluated.trace.json")
	forkTrace := filepath.Join(tmp, "fork.trace.json")
	forkSnapshot := filepath.Join(tmp, "fork.snapshot.json")
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	baseArtifact, err := traceutil.FromRunTrace(&core.RunTrace{
		RunID:     "dispatch-base",
		Prompt:    "secret prompt",
		StartTime: now,
		EndTime:   now.Add(time.Second),
		Success:   true,
		Requests: []core.RequestTrace{
			{RequestID: "req-1", TurnNumber: 1, StartedAt: now, EndedAt: now.Add(time.Millisecond), Response: &core.RequestTraceResponse{Usage: core.Usage{InputTokens: 1, OutputTokens: 1}}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("base trace: %v", err)
	}
	variantArtifact, err := traceutil.FromRunTrace(&core.RunTrace{RunID: "dispatch-variant", StartTime: now, EndTime: now.Add(time.Second), Success: true}, nil)
	if err != nil {
		t.Fatalf("variant trace: %v", err)
	}
	for path, artifact := range map[string]*traceutil.Artifact{base: baseArtifact, variant: variantArtifact} {
		if err := traceutil.WriteFile(path, artifact); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	snap := &core.RunSnapshot{RunID: "dispatch-fork", RunStep: 1, Prompt: "resume", Timestamp: now}
	forkArtifact, err := traceutil.FromRunTraceWithSnapshots(&core.RunTrace{RunID: "dispatch-fork", StartTime: now, EndTime: now.Add(time.Second), Success: true}, []*core.RunSnapshot{snap}, nil)
	if err != nil {
		t.Fatalf("fork trace: %v", err)
	}
	if err := traceutil.WriteFile(forkTrace, forkArtifact); err != nil {
		t.Fatalf("write fork trace: %v", err)
	}

	oldView, oldCompare := traceViewArtifact, traceCompareArtifacts
	traceViewArtifact = func(*traceutil.Artifact, ...tui.Option) error { return nil }
	traceCompareArtifacts = func(*traceutil.Artifact, *traceutil.Artifact, ...tui.Option) error { return nil }
	defer func() {
		traceViewArtifact = oldView
		traceCompareArtifacts = oldCompare
	}()

	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"export", "export", []string{base, "--out", exported}},
		{"inspect", "inspect", []string{base, "--events", "1"}},
		{"summarize", "summarize", []string{base, "--events", "1"}},
		{"view", "view", []string{base}},
		{"view compare", "view", []string{base, variant}},
		{"replay", "replay", []string{base, "--mode", "inspect"}},
		{"fork", "fork", []string{forkTrace, "--from-step", "1", "--out", forkSnapshot}},
		{"diff", "diff", []string{base, variant}},
		{"regress", "regress", []string{base, variant}},
		{"sleepy", "sleepy", []string{base, variant, "--out", sleepyOut}},
		{"evaluate", "evaluate", []string{base, "--evaluator", "status-succeeded", "--out", evaluated}},
		{"validate", "validate", []string{base}},
		{"redact", "redact", []string{base, "--pattern", "secret", "--out", redacted}},
		{"compact", "compact", []string{base, "--out", compacted}},
		{"help", "help", nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := dispatchTraceCommand(tt.cmd, tt.args); err != nil {
				t.Fatalf("dispatchTraceCommand(%s) error = %v", tt.cmd, err)
			}
		})
	}
	if err := dispatchTraceCommand("wat", nil); err == nil || !strings.Contains(err.Error(), "unknown trace command") {
		t.Fatalf("unknown dispatch error = %v", err)
	}
}

func TestRunTraceFormatAndThresholdErrors(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "base.trace.json")
	variant := filepath.Join(tmp, "variant.trace.json")
	for path, runID := range map[string]string{base: "base-run", variant: "variant-run"} {
		artifact, err := traceutil.FromRunTrace(&core.RunTrace{RunID: runID, Success: true}, nil)
		if err != nil {
			t.Fatalf("build trace: %v", err)
		}
		if err := traceutil.WriteFile(path, artifact); err != nil {
			t.Fatalf("write trace: %v", err)
		}
	}
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{"diff unsupported format", runTraceDiff([]string{base, variant, "--format", "yaml"}), "unsupported diff format"},
		{"regress unsupported format", runTraceRegress([]string{base, variant, "--format", "yaml"}), "unsupported regress format"},
		{"regress invalid cost", runTraceRegress([]string{"--max-cost-delta", "bad", base, variant}), "invalid --max-cost-delta"},
		{"regress missing format value", runTraceRegress([]string{"--format"}), "--format requires"},
		{"diff missing format value", runTraceDiff([]string{"--format"}), "--format requires"},
		{"sleepy unknown", runTraceSleepy([]string{base, variant, "--wat"}), "unknown sleepy"},
		{"evaluate unknown", runTraceEvaluate([]string{base, "--evaluator", "missing"}), "unknown trace evaluator"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil || !strings.Contains(tt.err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", tt.err, tt.want)
			}
		})
	}
}

func TestTraceCommandErrorBranchesAndUsageOutput(t *testing.T) {
	tmp := t.TempDir()
	tracePath := filepath.Join(tmp, "run.trace.json")
	artifact, err := traceutil.FromRunTrace(&core.RunTrace{RunID: "run-errors", Success: true}, nil)
	if err != nil {
		t.Fatalf("build trace: %v", err)
	}
	if err := traceutil.WriteFile(tracePath, artifact); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	printTraceUsage()
	_ = w.Close()
	os.Stderr = oldStderr
	usage, _ := io.ReadAll(r)
	if !bytes.Contains(usage, []byte("gollem trace")) {
		t.Fatalf("usage output missing header:\n%s", string(usage))
	}
	r, w, err = os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	printUsage()
	printRunUsage()
	printServeUsage()
	_ = w.Close()
	os.Stderr = oldStderr
	topUsage, _ := io.ReadAll(r)
	for _, want := range [][]byte{[]byte("Usage:"), []byte("gollem run"), []byte("gollem serve")} {
		if !bytes.Contains(topUsage, want) {
			t.Fatalf("top-level usage missing %q:\n%s", string(want), string(topUsage))
		}
	}

	if err := runTraceExport([]string{tracePath}); err != nil {
		t.Fatalf("runTraceExport(stdout) error = %v", err)
	}
	if err := runTraceExport([]string{tracePath, "--out", tmp}); err == nil {
		t.Fatal("expected trace export write error for directory output")
	}
	if err := runTraceInspect([]string{filepath.Join(tmp, "missing.trace.json")}); err == nil {
		t.Fatal("expected inspect read error")
	}
	if err := runTraceView([]string{tracePath, filepath.Join(tmp, "missing.trace.json")}); err == nil {
		t.Fatal("expected view variant read error")
	}
	if err := runTraceReplay([]string{filepath.Join(tmp, "missing.trace.json")}); err == nil {
		t.Fatal("expected replay read error")
	}
	if err := runTraceFork([]string{tracePath, "--from-step", "1"}); err == nil {
		t.Fatal("expected fork snapshot selection error")
	}
	if err := continueTraceFork("", &core.RunSnapshot{}, traceutil.SnapshotRecord{}, traceForkRunOptions{}); err == nil {
		t.Fatal("expected continue fork missing out error")
	}
	oldExec := traceForkRunExec
	traceForkRunExec = func(_ []string, _ io.Reader, _ io.Writer, _ io.Writer, _ []string) error {
		return os.ErrPermission
	}
	defer func() { traceForkRunExec = oldExec }()
	if err := continueTraceFork(filepath.Join(tmp, "fork.trace.json"), &core.RunSnapshot{RunID: "fork"}, traceutil.SnapshotRecord{ID: "snap", Step: 1}, traceForkRunOptions{SnapshotOut: filepath.Join(tmp, "fork.snapshot.json")}); err == nil {
		t.Fatal("expected continue fork exec error")
	}
}

func TestTraceLookupRunIDBranches(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOLLEM_TRACE_DIR", tmp)
	t.Setenv("GOLLEM_TRACE_DIRS", tmp+string(os.PathListSeparator)+tmp)
	dirs := traceLookupDirs(tmp)
	if len(dirs) == 0 || dirs[0] != tmp {
		t.Fatalf("unexpected trace lookup dirs: %+v", dirs)
	}
	if _, _, err := findLocalTraceByRunID("", dirs); err == nil {
		t.Fatal("expected empty run id error")
	}
	if err := os.WriteFile(filepath.Join(tmp, "ignore.txt"), []byte("nope"), 0o600); err != nil {
		t.Fatalf("write ignore file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "bad.trace.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write bad trace: %v", err)
	}
	start := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	older, err := traceutil.FromRunTrace(&core.RunTrace{RunID: "lookup-run", StartTime: start, Success: true}, nil)
	if err != nil {
		t.Fatalf("older trace: %v", err)
	}
	newer, err := traceutil.FromRunTrace(&core.RunTrace{RunID: "lookup-run", StartTime: start.Add(time.Hour), Success: true}, nil)
	if err != nil {
		t.Fatalf("newer trace: %v", err)
	}
	newer.Run.ID = ""
	if err := traceutil.WriteFile(filepath.Join(tmp, "older.trace.json"), older); err != nil {
		t.Fatalf("write older: %v", err)
	}
	if err := traceutil.WriteFile(filepath.Join(tmp, "newer.trace.json"), newer); err != nil {
		t.Fatalf("write newer: %v", err)
	}
	got, path, err := findLocalTraceByRunID("lookup-run", []string{"", tmp})
	if err != nil {
		t.Fatalf("findLocalTraceByRunID() error = %v", err)
	}
	if !strings.Contains(path, "newer.trace.json") || got.Trace == nil || got.Trace.RunID != "lookup-run" {
		t.Fatalf("unexpected trace lookup result path=%s artifact=%+v", path, got)
	}
	if traceMatchesRunID(nil, "lookup-run") {
		t.Fatal("nil artifact should not match run id")
	}
	if _, _, err := findLocalTraceByRunID("missing-run", []string{tmp}); err == nil {
		t.Fatal("expected missing run id error")
	}
}

func TestRunTraceExportTemporalUsesQueryHook(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "temporal.trace.json")
	oldQuery := traceQueryTemporalArtifact
	traceQueryTemporalArtifact = func(opts traceExportOptions) (*traceutil.Artifact, error) {
		if !opts.temporal || opts.workflowID != "workflow-1" {
			t.Fatalf("unexpected temporal opts: %+v", opts)
		}
		return traceutil.FromRunTrace(&core.RunTrace{RunID: "temporal-run", Success: true}, map[string]any{"source": "temporal"})
	}
	defer func() { traceQueryTemporalArtifact = oldQuery }()

	if err := runTraceExport([]string{"--temporal", "workflow-1", "--out", out}); err != nil {
		t.Fatalf("runTraceExport() error = %v", err)
	}
	artifact, err := traceutil.ReadFile(out)
	if err != nil {
		t.Fatalf("read temporal export: %v", err)
	}
	if artifact.Run.ID != "temporal-run" {
		t.Fatalf("run id = %q, want temporal-run", artifact.Run.ID)
	}
}

func TestRunTraceViewUsesInjectedViewers(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "base.trace.json")
	variant := filepath.Join(tmp, "variant.trace.json")
	for path, runID := range map[string]string{base: "base-run", variant: "variant-run"} {
		artifact, err := traceutil.FromRunTrace(&core.RunTrace{RunID: runID, Success: true}, nil)
		if err != nil {
			t.Fatalf("build %s trace: %v", runID, err)
		}
		if err := traceutil.WriteFile(path, artifact); err != nil {
			t.Fatalf("write %s trace: %v", runID, err)
		}
	}

	oldView := traceViewArtifact
	oldCompare := traceCompareArtifacts
	var singleRunID, compareRunIDs string
	traceViewArtifact = func(artifact *traceutil.Artifact, _ ...tui.Option) error {
		singleRunID = artifact.Run.ID
		return nil
	}
	traceCompareArtifacts = func(baseline, variant *traceutil.Artifact, _ ...tui.Option) error {
		compareRunIDs = baseline.Run.ID + " -> " + variant.Run.ID
		return nil
	}
	defer func() {
		traceViewArtifact = oldView
		traceCompareArtifacts = oldCompare
	}()

	if err := runTraceView([]string{base}); err != nil {
		t.Fatalf("runTraceView(single) error = %v", err)
	}
	if singleRunID != "base-run" {
		t.Fatalf("singleRunID = %q", singleRunID)
	}
	if err := runTraceView([]string{base, variant}); err != nil {
		t.Fatalf("runTraceView(compare) error = %v", err)
	}
	if compareRunIDs != "base-run -> variant-run" {
		t.Fatalf("compareRunIDs = %q", compareRunIDs)
	}
}

func TestTraceCommandArgumentErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		fn   func() error
		want string
	}{
		{"export missing input", func() error { return runTraceExport(nil) }, "requires an input"},
		{"export bad timeout", func() error { _, err := parseTraceExportArgs([]string{"--timeout", "0", "run"}); return err }, "invalid --timeout"},
		{"export temporal plus input", func() error {
			_, err := parseTraceExportArgs([]string{"run.trace.json", "--temporal", "workflow"})
			return err
		}, "either an input"},
		{"view missing path", func() error { return runTraceView(nil) }, "requires one trace path"},
		{"view unknown flag", func() error {
			_, err := parseTraceViewArgs([]string{"--wat"})
			return err
		}, "unknown view"},
		{"inspect unknown flag", func() error {
			_, _, err := parseTraceInspectArgs([]string{"--wat"})
			return err
		}, "unknown inspect"},
		{"replay unknown flag", func() error {
			_, _, err := parseTraceReplayArgs([]string{"--wat"})
			return err
		}, "unknown replay"},
		{"replay unsupported mode", func() error { return runTraceReplay([]string{"run.trace.json", "--mode", "unknown"}) }, "unsupported replay mode"},
		{"fork bad step", func() error {
			_, _, _, _, err := parseTraceForkArgs([]string{"run.trace.json", "--from-step", "-1"})
			return err
		}, "invalid --from-step"},
		{"fork unknown flag", func() error {
			_, _, _, _, err := parseTraceForkArgs([]string{"run.trace.json", "--wat"})
			return err
		}, "unknown fork"},
		{"fork duplicate input", func() error {
			_, _, _, _, err := parseTraceForkArgs([]string{"a.trace.json", "b.trace.json"})
			return err
		}, "exactly one"},
		{"diff missing args", func() error { return runTraceDiff([]string{"only-one.trace.json"}) }, "requires baseline"},
		{"regress bad token delta", func() error { return runTraceRegress([]string{"--max-token-delta", "nan", "base", "variant"}) }, "invalid --max-token-delta"},
		{"sleepy missing candidates", func() error { return runTraceSleepy([]string{"base.trace.json"}) }, "requires a baseline"},
		{"validate missing path", func() error { return runTraceValidate(nil) }, "exactly one trace path"},
		{"redact unknown flag", func() error { return runTraceRedact([]string{"run.trace.json", "--wat"}) }, "unknown redact"},
		{"compact bad limit", func() error { return runTraceCompact([]string{"run.trace.json", "--payload-limit", "-1"}) }, "invalid --payload-limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestApplyResumeSnapshotOverrides(t *testing.T) {
	f := flags{teamMode: "auto"}
	snap := &core.RunSnapshot{
		ToolState: map[string]any{
			"_gollem_fork_overrides": map[string]any{
				"model":       "gpt-next",
				"topology":    "team",
				"tool_policy": "read-only",
				"middleware":  "no-reasoning",
				"evaluator":   "tests",
			},
		},
	}

	applied, err := applyResumeSnapshotOverrides(&f, snap)
	if err != nil {
		t.Fatalf("applyResumeSnapshotOverrides() error = %v", err)
	}
	if f.modelName != "gpt-next" {
		t.Fatalf("modelName = %q, want gpt-next", f.modelName)
	}
	if f.teamMode != "on" {
		t.Fatalf("teamMode = %q, want on", f.teamMode)
	}
	if f.toolPolicy != "read-only" {
		t.Fatalf("toolPolicy = %q, want read-only", f.toolPolicy)
	}
	if !f.noReasoning {
		t.Fatal("noReasoning = false, want true")
	}
	if f.evaluator != "tests" {
		t.Fatalf("evaluator = %q, want tests", f.evaluator)
	}
	if len(applied.Applied) != 5 {
		t.Fatalf("applied overrides = %+v, want 5 entries", applied.Applied)
	}
}

func TestResumeSnapshotOverrideHelpersCoverAliasesAndDefaults(t *testing.T) {
	if applied, err := applyResumeSnapshotOverrides(nil, nil); err != nil || len(applied.Applied) != 0 {
		t.Fatalf("nil overrides = %+v err=%v", applied, err)
	}
	if got := snapshotForkOverrides(&core.RunSnapshot{}); got != nil {
		t.Fatalf("empty snapshot overrides = %+v, want nil", got)
	}
	overrides := snapshotForkOverrides(&core.RunSnapshot{ToolState: map[string]any{
		"_gollem_fork_overrides": map[string]string{
			"model":    " model-b ",
			"topology": "",
		},
	}})
	if overrides["model"] != "model-b" || len(overrides) != 1 {
		t.Fatalf("map[string]string overrides = %+v", overrides)
	}
	if got := snapshotForkOverrides(&core.RunSnapshot{ToolState: map[string]any{"_gollem_fork_overrides": 42}}); got != nil {
		t.Fatalf("unsupported overrides = %+v, want nil", got)
	}

	for input, want := range map[string]string{
		"":            "",
		"default":     "",
		"auto":        "auto",
		"team-mode":   "on",
		"multiagent":  "on",
		"single":      "off",
		"singleagent": "off",
		"local":       "off",
	} {
		got, err := forkTopologyTeamMode(input)
		if err != nil || got != want {
			t.Fatalf("forkTopologyTeamMode(%q) = %q err=%v, want %q", input, got, err, want)
		}
	}

	f := flags{}
	if got, err := applyForkToolPolicy(&f, "default"); err != nil || got != "" {
		t.Fatalf("default tool policy = %q err=%v", got, err)
	}
	if got, err := applyForkToolPolicy(&f, "readonly"); err != nil || got != "read-only" || f.toolPolicy != "read-only" {
		t.Fatalf("readonly policy = %q f=%+v err=%v", got, f, err)
	}
	if got, err := applyForkToolPolicy(&f, "no_code_mode"); err != nil || got != "no-code-mode" || !f.noCodeMode {
		t.Fatalf("no-code policy = %q f=%+v err=%v", got, f, err)
	}
	if _, err := applyForkToolPolicy(&f, "write-all"); err == nil {
		t.Fatal("expected unsupported tool policy error")
	}

	f = flags{}
	for input, want := range map[string]string{
		"":                    "",
		"none":                "",
		"disable-reasoning":   "no-reasoning",
		"disable-code-mode":   "no-code-mode",
		"no-code-mode":        "no-code-mode",
		"no-reasoning":        "no-reasoning",
		"disable-code-mode  ": "no-code-mode",
	} {
		got, err := applyForkMiddleware(&f, input)
		if err != nil || got != want {
			t.Fatalf("applyForkMiddleware(%q) = %q err=%v, want %q", input, got, err, want)
		}
	}
	if !f.noReasoning || !f.noCodeMode {
		t.Fatalf("middleware flags not applied: %+v", f)
	}
	if _, err := applyForkMiddleware(&f, "network-sandbox"); err == nil {
		t.Fatal("expected unsupported middleware error")
	}
}

func TestLoadRunSnapshotFileBranches(t *testing.T) {
	if _, err := loadRunSnapshotFile(""); err == nil {
		t.Fatal("expected empty snapshot path error")
	}
	if _, err := loadRunSnapshotFile(filepath.Join(t.TempDir(), "missing.snapshot.json")); err == nil {
		t.Fatal("expected missing snapshot error")
	}

	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.snapshot.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatalf("write bad snapshot: %v", err)
	}
	if _, err := loadRunSnapshotFile(bad); err == nil {
		t.Fatal("expected malformed snapshot error")
	}

	good := filepath.Join(tmp, "good.snapshot.json")
	data, err := core.MarshalSnapshot(&core.RunSnapshot{RunID: "snap-run", Prompt: "resume", RunStep: 2})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(good, data, 0o600); err != nil {
		t.Fatalf("write good snapshot: %v", err)
	}
	snap, err := loadRunSnapshotFile(good)
	if err != nil {
		t.Fatalf("loadRunSnapshotFile() error = %v", err)
	}
	if snap.RunID != "snap-run" || snap.RunStep != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestParseFlagsTraceStream(t *testing.T) {
	f := parseFlags([]string{"--trace-stream", "events.jsonl", "prompt"})
	if f.traceStream != "events.jsonl" {
		t.Fatalf("traceStream = %q", f.traceStream)
	}
}

func TestParseFlagsCoversRunAndTraceOptions(t *testing.T) {
	tmp := t.TempDir()
	f := parseFlags([]string{
		"--provider", "test",
		"--model", "model-a",
		"--location", "us-central1",
		"--project", "proj",
		"--workdir", tmp,
		"--trace-out", "trace.json",
		"--trace-stream", "events.jsonl",
		"--resume-snapshot", "resume.snapshot.json",
		"--timeout", "7s",
		"--thinking-budget", "123",
		"--reasoning-effort", "high",
		"--team-mode", "bogus",
		"--no-reasoning",
		"--no-code-mode",
		"prompt text",
	})
	if f.provider != "test" || f.modelName != "model-a" || !f.modelExplicit {
		t.Fatalf("unexpected provider/model flags: %+v", f)
	}
	if f.location != "us-central1" || f.project != "proj" || f.workDir != tmp {
		t.Fatalf("unexpected location/project/workdir: %+v", f)
	}
	if f.traceOut != "trace.json" || f.traceStream != "events.jsonl" || f.resumeSnapshot != "resume.snapshot.json" {
		t.Fatalf("unexpected trace flags: %+v", f)
	}
	if f.timeout != 7*time.Second || f.thinkingBudget != 123 || f.reasoningEffort != "high" {
		t.Fatalf("unexpected timeout/reasoning flags: %+v", f)
	}
	if f.teamMode != "auto" || !f.teamExplicit || !f.noReasoning || !f.noCodeMode || f.prompt != "prompt text" {
		t.Fatalf("unexpected mode/prompt flags: %+v", f)
	}

	invalidBudget := parseFlags([]string{"--thinking-budget", "bad", "prompt"})
	if invalidBudget.thinkingBudget != -1 {
		t.Fatalf("invalid thinking budget = %d, want -1", invalidBudget.thinkingBudget)
	}
}

func argAfter(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestApplyResumeSnapshotOverridesRejectsUnsupportedValues(t *testing.T) {
	f := flags{teamMode: "auto"}
	snap := &core.RunSnapshot{
		ToolState: map[string]any{
			"_gollem_fork_overrides": map[string]any{
				"topology": "orchestrator",
			},
		},
	}
	if _, err := applyResumeSnapshotOverrides(&f, snap); err == nil {
		t.Fatal("expected unsupported topology override error")
	}
}

func TestParseTraceRedactArgs(t *testing.T) {
	input, out, opts, err := parseTraceRedactArgs([]string{
		"run.trace.json",
		"--key", "authorization",
		"--pattern", "secret-value",
		"--replacement", "***",
		"--drop-trace",
		"--out", "redacted.trace.json",
	})
	if err != nil {
		t.Fatalf("parseTraceRedactArgs() error = %v", err)
	}
	if input != "run.trace.json" || out != "redacted.trace.json" {
		t.Fatalf("input/out = %q/%q", input, out)
	}
	if len(opts.Keys) != 1 || opts.Keys[0] != "authorization" {
		t.Fatalf("keys = %+v", opts.Keys)
	}
	if len(opts.Patterns) != 1 || opts.Patterns[0] != "secret-value" {
		t.Fatalf("patterns = %+v", opts.Patterns)
	}
	if opts.Replacement != "***" {
		t.Fatalf("replacement = %q", opts.Replacement)
	}
	if !opts.DropTrace {
		t.Fatal("drop trace = false, want true")
	}
}

func TestParseTraceCompactArgs(t *testing.T) {
	input, out, opts, err := parseTraceCompactArgs([]string{
		"run.trace.json",
		"--payload-limit", "128",
		"--keep-snapshots", "2",
		"--keep-trace",
		"--out", "compact.trace.json",
	})
	if err != nil {
		t.Fatalf("parseTraceCompactArgs() error = %v", err)
	}
	if input != "run.trace.json" || out != "compact.trace.json" {
		t.Fatalf("input/out = %q/%q", input, out)
	}
	if opts.EventPayloadLimit != 128 {
		t.Fatalf("payload limit = %d", opts.EventPayloadLimit)
	}
	if opts.KeepSnapshots != 2 {
		t.Fatalf("keep snapshots = %d", opts.KeepSnapshots)
	}
	if opts.DropTrace {
		t.Fatal("drop trace = true, want false")
	}
}

func countTraceEvents(events []traceutil.Event, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

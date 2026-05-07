package orchestrator_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

func TestAgentRunner_UsesTaskInputAndOrchestrationRunAsParent(t *testing.T) {
	bus := core.NewEventBus()

	var start core.RunStartedEvent
	core.Subscribe(bus, func(e core.RunStartedEvent) {
		start = e
	})

	runner := orchestrator.NewAgentRunner(core.NewAgent[string](
		core.NewTestModel(core.TextResponse("done")),
		core.WithEventBus[string](bus),
	), orchestrator.WithTaskArtifacts(func(task *orchestrator.Task, result *core.RunResult[string]) []orchestrator.ArtifactSpec {
		return []orchestrator.ArtifactSpec{{
			Kind:        "report",
			Name:        "summary.txt",
			ContentType: "text/plain",
			Body:        []byte(task.ID + ":" + result.Output),
		}}
	}))

	outcome, err := runner.RunTask(context.Background(), &orchestrator.ClaimedTask{
		Task: &orchestrator.Task{
			ID:    "task-1",
			Input: "run this task",
		},
		Run: &orchestrator.RunRef{
			ID:       "orch-run-1",
			TaskID:   "task-1",
			WorkerID: "worker-a",
			Attempt:  1,
		},
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if outcome == nil || outcome.Result == nil {
		t.Fatal("expected non-nil task outcome")
	}
	result := outcome.Result
	if result.Output != "done" {
		t.Fatalf("expected output %q, got %v", "done", result.Output)
	}
	if result.RunnerRunID == "" {
		t.Fatal("expected a runner run id")
	}
	if len(outcome.Artifacts) != 1 {
		t.Fatalf("expected 1 outcome artifact, got %d", len(outcome.Artifacts))
	}
	if got := string(outcome.Artifacts[0].Body); got != "task-1:done" {
		t.Fatalf("unexpected artifact body %q", got)
	}
	if start.Prompt != "run this task" {
		t.Fatalf("expected prompt %q, got %q", "run this task", start.Prompt)
	}
	if start.ParentRunID != "orch-run-1" {
		t.Fatalf("expected ParentRunID %q, got %q", "orch-run-1", start.ParentRunID)
	}
}

func TestAgentRunner_CanEmitReplayableTraceArtifact(t *testing.T) {
	runner := orchestrator.NewAgentRunner(core.NewAgent[string](
		core.NewTestModel(core.TextResponse("done")),
		core.WithTracing[string](),
	), orchestrator.WithTaskArtifacts(func(task *orchestrator.Task, result *core.RunResult[string]) []orchestrator.ArtifactSpec {
		spec, ok, err := traceutil.OrchestratorArtifactSpec(task, result, nil)
		if err != nil {
			t.Fatalf("OrchestratorArtifactSpec failed: %v", err)
		}
		if !ok {
			t.Fatal("expected trace artifact spec")
		}
		return []orchestrator.ArtifactSpec{spec}
	}))

	outcome, err := runner.RunTask(context.Background(), &orchestrator.ClaimedTask{
		Task: &orchestrator.Task{
			ID:    "task-trace",
			Kind:  "agent",
			Input: "trace this task",
		},
		Run: &orchestrator.RunRef{
			ID:       "orch-run-trace",
			TaskID:   "task-trace",
			WorkerID: "worker-a",
			Attempt:  1,
		},
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if len(outcome.Artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(outcome.Artifacts))
	}
	artifact, err := traceutil.Read(bytes.NewReader(outcome.Artifacts[0].Body))
	if err != nil {
		t.Fatalf("read trace artifact: %v", err)
	}
	if artifact.Run.Mode != "orchestrator" {
		t.Fatalf("mode = %q, want orchestrator", artifact.Run.Mode)
	}
	if err := traceutil.ValidateReplay(artifact); err != nil {
		t.Fatalf("trace artifact should replay strictly: %v", err)
	}
	if artifact.Metadata["orchestrator_run_id"] != "orch-run-trace" {
		t.Fatalf("missing orchestrator run metadata: %+v", artifact.Metadata)
	}
}

package orchestrator_test

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
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

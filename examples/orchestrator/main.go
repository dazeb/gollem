// Example orchestrator demonstrates the durable task orchestration layer with
// an in-memory store, in-process runner, scheduler, and task-scoped artifacts.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	memstore "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

type taskSummary struct {
	Summary string `json:"summary" jsonschema:"description=A compact summary of the completed task"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{"summary":"Reviewed the repository, verified the scheduler path, and captured a durable handoff artifact."}`),
	)
	agent := core.NewAgent[taskSummary](model,
		core.WithSystemPrompt[taskSummary](
			"You are an orchestration worker. Finish the task and return a compact summary.",
		),
	)

	store := memstore.NewStore()
	runner := orchestrator.NewAgentRunner(agent, orchestrator.WithTaskArtifacts(
		func(task *orchestrator.Task, result *core.RunResult[taskSummary]) []orchestrator.ArtifactSpec {
			return []orchestrator.ArtifactSpec{{
				Kind:        "report",
				Name:        "handoff.md",
				ContentType: "text/markdown",
				Body: []byte(fmt.Sprintf(
					"# Task Handoff\n\nTask: %s\n\nSummary: %s\n",
					task.Subject,
					result.Output.Summary,
				)),
				Metadata: map[string]any{
					"worker_output": result.Output.Summary,
				},
			}}
		},
	))
	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("demo-worker"),
		orchestrator.WithPollInterval(25*time.Millisecond),
	)

	task, err := store.CreateTask(ctx, orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Subject:     "Review scheduler path",
		Description: "Run the worker task and persist a handoff artifact.",
		Input:       "Review the scheduler path and summarize the outcome.",
		MaxAttempts: 2,
	})
	if err != nil {
		log.Fatalf("CreateTask failed: %v", err)
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		if err := scheduler.Run(runCtx); err != nil && runCtx.Err() == nil {
			log.Printf("scheduler stopped with error: %v", err)
		}
	}()

	completed, err := waitForCompletion(ctx, store, task.ID)
	if err != nil {
		log.Fatal(err)
	}

	summary := completed.Result.Output.(taskSummary)
	artifacts, err := store.ListArtifacts(ctx, orchestrator.ArtifactFilter{TaskID: completed.ID})
	if err != nil {
		log.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		log.Fatalf("expected exactly 1 persisted artifact, got %d", len(artifacts))
	}
	artifact := artifacts[0]

	fmt.Printf("Task: %s (%s)\n", completed.ID, completed.Status)
	fmt.Printf("Run: %s via %s\n", completed.Run.ID, completed.Run.WorkerID)
	fmt.Printf("Summary: %s\n", summary.Summary)
	fmt.Printf("Artifact: %s (%s, %d bytes)\n", artifact.Name, artifact.Kind, len(artifact.Body))
	fmt.Printf("Artifacts stored for task: %d\n", len(artifacts))
}

func waitForCompletion(ctx context.Context, store orchestrator.TaskStore, taskID string) (*orchestrator.Task, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := store.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		switch task.Status {
		case orchestrator.TaskCompleted:
			return task, nil
		case orchestrator.TaskFailed:
			return nil, fmt.Errorf("task %s failed: %s", task.ID, task.LastError)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

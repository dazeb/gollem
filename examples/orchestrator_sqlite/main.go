// Example orchestrator_sqlite demonstrates the persistent SQLite-backed
// orchestrator store with durable task history, artifacts, and scheduler state.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	orchestratorsqlite "github.com/fugue-labs/gollem/ext/orchestrator/sqlite"
)

type sqliteTaskSummary struct {
	Summary string `json:"summary" jsonschema:"description=A compact summary of the completed durable task"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tempDir, err := os.MkdirTemp("", "gollem-orchestrator-sqlite-*")
	if err != nil {
		log.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "orchestrator.db")
	store, err := orchestratorsqlite.NewStore(dbPath)
	if err != nil {
		log.Fatalf("NewStore failed: %v", err)
	}

	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{"summary":"Verified SQLite-backed orchestration persistence and durable event history."}`),
	)
	agent := core.NewAgent[sqliteTaskSummary](model,
		core.WithSystemPrompt[sqliteTaskSummary](
			"You are a durable orchestration worker. Finish the task and return a compact summary.",
		),
	)

	runner := orchestrator.NewAgentRunner(agent, orchestrator.WithTaskArtifacts(
		func(task *orchestrator.Task, result *core.RunResult[sqliteTaskSummary]) []orchestrator.ArtifactSpec {
			return []orchestrator.ArtifactSpec{{
				Kind:        "report",
				Name:        "handoff.md",
				ContentType: "text/markdown",
				Body: []byte(fmt.Sprintf(
					"# Durable Handoff\n\nTask: %s\n\nSummary: %s\n",
					task.Subject,
					result.Output.Summary,
				)),
			}}
		},
	))
	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("sqlite-demo-worker"),
		orchestrator.WithPollInterval(25*time.Millisecond),
	)

	task, err := store.CreateTask(ctx, orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Subject:     "Verify persistent orchestration",
		Description: "Run the task, persist an artifact, then reopen the store and inspect durable history.",
		Input:       "Verify SQLite-backed orchestration and summarize the result.",
		MaxAttempts: 2,
	})
	if err != nil {
		log.Fatalf("CreateTask failed: %v", err)
	}

	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(runCtx)
	}()

	completed, err := waitForSQLiteCompletion(ctx, store, task.ID)
	if err != nil {
		log.Fatal(err)
	}
	summary := completed.Result.Output.(sqliteTaskSummary)
	stop()
	if err := <-done; err != nil && runCtx.Err() == nil {
		log.Fatalf("scheduler stopped with error: %v", err)
	}
	if err := store.Close(); err != nil {
		log.Fatalf("Close failed: %v", err)
	}

	store, err = orchestratorsqlite.NewStore(dbPath)
	if err != nil {
		log.Fatalf("reopen store failed: %v", err)
	}
	defer store.Close()

	persisted, err := store.GetTask(ctx, completed.ID)
	if err != nil {
		log.Fatalf("GetTask after reopen failed: %v", err)
	}
	artifacts, err := store.ListArtifacts(ctx, orchestrator.ArtifactFilter{TaskID: persisted.ID})
	if err != nil {
		log.Fatalf("ListArtifacts failed: %v", err)
	}
	events, err := store.ListEvents(ctx, orchestrator.EventFilter{TaskID: persisted.ID})
	if err != nil {
		log.Fatalf("ListEvents failed: %v", err)
	}
	runTimeline, err := orchestrator.LoadRunTimeline(ctx, store, persisted.Run.ID)
	if err != nil {
		log.Fatalf("LoadRunTimeline failed: %v", err)
	}
	workerSummary, err := orchestrator.GetWorker(ctx, store, persisted.Run.WorkerID)
	if err != nil {
		log.Fatalf("GetWorker failed: %v", err)
	}
	workers, err := orchestrator.ListWorkers(ctx, store, orchestrator.WorkerFilter{})
	if err != nil {
		log.Fatalf("ListWorkers failed: %v", err)
	}
	activeRuns, err := orchestrator.ListActiveRuns(ctx, store, orchestrator.ActiveRunFilter{})
	if err != nil {
		log.Fatalf("ListActiveRuns failed: %v", err)
	}
	pendingCommands, err := orchestrator.ListPendingCommandsForWorker(ctx, store, persisted.Run.WorkerID)
	if err != nil {
		log.Fatalf("ListPendingCommandsForWorker failed: %v", err)
	}
	terminalKind := "<none>"
	if runTimeline.Terminal != nil {
		terminalKind = string(runTimeline.Terminal.Kind)
	}

	fmt.Printf("Task: %s (%s)\n", persisted.ID, persisted.Status)
	fmt.Printf("Run: %s via %s\n", persisted.Run.ID, persisted.Run.WorkerID)
	fmt.Printf("Summary: %s\n", summary.Summary)
	fmt.Printf("Artifacts stored for task: %d\n", len(artifacts))
	fmt.Printf("Run timeline events: %d (terminal: %s)\n", len(runTimeline.Events), terminalKind)
	fmt.Printf("Durable history events: %d\n", len(events))
	fmt.Printf("Worker summary: active=%d completed=%d failed=%d latest_run=%s\n",
		workerSummary.ActiveRuns,
		workerSummary.CompletedRuns,
		workerSummary.FailedRuns,
		workerSummary.LatestRunID,
	)
	fmt.Printf("Projected workers: %d\n", len(workers))
	fmt.Printf("Current active runs: %d\n", len(activeRuns))
	fmt.Printf("Pending commands for %s: %d\n", persisted.Run.WorkerID, len(pendingCommands))
	for _, event := range events {
		fmt.Printf("- #%d %s at %s\n", event.Sequence, event.Kind, event.CreatedAt.Format(time.RFC3339Nano))
	}
}

func waitForSQLiteCompletion(ctx context.Context, store orchestrator.TaskStore, taskID string) (*orchestrator.Task, error) {
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
		case orchestrator.TaskCanceled:
			return nil, fmt.Errorf("task %s canceled: %s", task.ID, task.LastError)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

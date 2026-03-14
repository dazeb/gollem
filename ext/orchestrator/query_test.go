package orchestrator_test

import (
	"context"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
	memstore "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

func TestActiveRunQueries(t *testing.T) {
	store := memstore.NewStore()
	base := time.Unix(100, 0).UTC()

	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:    "analysis",
		Subject: "task a",
		Input:   "run a",
	})
	if err != nil {
		t.Fatalf("CreateTask taskA failed: %v", err)
	}
	claimA, err := store.ClaimTask(context.Background(), taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask taskA failed: %v", err)
	}

	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:    "review",
		Subject: "task b",
		Input:   "run b",
	})
	if err != nil {
		t.Fatalf("CreateTask taskB failed: %v", err)
	}
	claimB, err := store.ClaimTask(context.Background(), taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimTask taskB failed: %v", err)
	}

	active, err := orchestrator.ListActiveRuns(context.Background(), store, orchestrator.ActiveRunFilter{})
	if err != nil {
		t.Fatalf("ListActiveRuns failed: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active runs, got %d", len(active))
	}
	if active[0].RunID != claimA.Run.ID || active[1].RunID != claimB.Run.ID {
		t.Fatalf("unexpected active run order: %+v", active)
	}

	filtered, err := orchestrator.ListActiveRuns(context.Background(), store, orchestrator.ActiveRunFilter{
		WorkerID: "worker-a",
		Kinds:    []string{"analysis"},
	})
	if err != nil {
		t.Fatalf("ListActiveRuns filtered failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].RunID != claimA.Run.ID {
		t.Fatalf("expected filtered active run %q, got %+v", claimA.Run.ID, filtered)
	}

	run, err := orchestrator.GetActiveRun(context.Background(), store, claimB.Run.ID)
	if err != nil {
		t.Fatalf("GetActiveRun failed: %v", err)
	}
	if run.WorkerID != "worker-b" || run.TaskID != taskB.ID {
		t.Fatalf("unexpected active run summary: %+v", run)
	}
}

func TestListPendingCommandsForWorker(t *testing.T) {
	store := memstore.NewStore()
	base := time.Unix(200, 0).UTC()

	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "analysis",
		Input: "run a",
	})
	if err != nil {
		t.Fatalf("CreateTask taskA failed: %v", err)
	}
	claimA, err := store.ClaimTask(context.Background(), taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask taskA failed: %v", err)
	}
	commandA, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: taskA.ID,
		RunID:  claimA.Run.ID,
		Reason: "worker a only",
	})
	if err != nil {
		t.Fatalf("CreateCommand commandA failed: %v", err)
	}

	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "review",
		Input: "run b",
	})
	if err != nil {
		t.Fatalf("CreateTask taskB failed: %v", err)
	}
	claimB, err := store.ClaimTask(context.Background(), taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimTask taskB failed: %v", err)
	}
	commandB, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: taskB.ID,
		RunID:  claimB.Run.ID,
		Reason: "worker b only",
	})
	if err != nil {
		t.Fatalf("CreateCommand commandB failed: %v", err)
	}

	taskC, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "analysis",
		Input: "retry me",
	})
	if err != nil {
		t.Fatalf("CreateTask taskC failed: %v", err)
	}
	claimC, err := store.ClaimTask(context.Background(), taskC.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-c",
		LeaseTTL: time.Minute,
		Now:      base.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimTask taskC failed: %v", err)
	}
	if _, err := store.FailTask(context.Background(), taskC.ID, claimC.Lease.Token, assertErr("boom"), base.Add(3*time.Second)); err != nil {
		t.Fatalf("FailTask taskC failed: %v", err)
	}
	commandC, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandRetryTask,
		TaskID: taskC.ID,
		Reason: "any worker can retry",
	})
	if err != nil {
		t.Fatalf("CreateCommand commandC failed: %v", err)
	}

	workerA, err := orchestrator.ListPendingCommandsForWorker(context.Background(), store, "worker-a")
	if err != nil {
		t.Fatalf("ListPendingCommandsForWorker worker-a failed: %v", err)
	}
	if len(workerA) != 2 {
		t.Fatalf("expected 2 pending commands for worker-a, got %d", len(workerA))
	}
	if workerA[0].ID != commandA.ID || workerA[1].ID != commandC.ID {
		t.Fatalf("unexpected worker-a commands: %+v", workerA)
	}

	workerB, err := orchestrator.ListPendingCommandsForWorker(context.Background(), store, "worker-b")
	if err != nil {
		t.Fatalf("ListPendingCommandsForWorker worker-b failed: %v", err)
	}
	if len(workerB) != 2 {
		t.Fatalf("expected 2 pending commands for worker-b, got %d", len(workerB))
	}
	if workerB[0].ID != commandB.ID || workerB[1].ID != commandC.ID {
		t.Fatalf("unexpected worker-b commands: %+v", workerB)
	}
}

type assertedError string

func (e assertedError) Error() string { return string(e) }

func assertErr(msg string) error { return assertedError(msg) }

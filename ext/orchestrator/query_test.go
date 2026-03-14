package orchestrator_test

import (
	"context"
	"errors"
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

func TestActiveRunQueriesUseNativeQueryStore(t *testing.T) {
	store := &runQueryStoreStub{
		Store: memstore.NewStore(),
		activeRuns: []*orchestrator.ActiveRunSummary{{
			RunID:       "run-native",
			TaskID:      "task-native",
			TaskKind:    "analysis",
			TaskSubject: "native path",
			WorkerID:    "worker-native",
			Attempt:     3,
		}},
	}

	active, err := orchestrator.ListActiveRuns(context.Background(), store, orchestrator.ActiveRunFilter{})
	if err != nil {
		t.Fatalf("ListActiveRuns failed: %v", err)
	}
	if len(active) != 1 || active[0].RunID != "run-native" {
		t.Fatalf("expected native active run, got %+v", active)
	}
	if !store.listCalled {
		t.Fatal("expected helper to use RunQueryStore.ListActiveRuns")
	}
	if store.fallbackUsed {
		t.Fatal("unexpected fallback ListTasks call")
	}

	run, err := orchestrator.GetActiveRun(context.Background(), store, "run-native")
	if err != nil {
		t.Fatalf("GetActiveRun failed: %v", err)
	}
	if run.TaskID != "task-native" {
		t.Fatalf("expected native active run task %q, got %+v", "task-native", run)
	}
	if !store.getCalled {
		t.Fatal("expected helper to use RunQueryStore.GetActiveRun")
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

func TestListPendingCommandsForWorkerUsesNativeQueryStore(t *testing.T) {
	store := &commandQueryStoreStub{
		Store: memstore.NewStore(),
		pending: []*orchestrator.Command{{
			ID:             "cmd-native",
			Kind:           orchestrator.CommandRetryTask,
			Status:         orchestrator.CommandPending,
			TargetWorkerID: "",
		}},
	}

	commands, err := orchestrator.ListPendingCommandsForWorker(context.Background(), store, "worker-native")
	if err != nil {
		t.Fatalf("ListPendingCommandsForWorker failed: %v", err)
	}
	if len(commands) != 1 || commands[0].ID != "cmd-native" {
		t.Fatalf("expected native pending command, got %+v", commands)
	}
	if !store.called {
		t.Fatal("expected helper to use CommandQueryStore.ListPendingCommandsForWorker")
	}
	if store.fallbackUsed {
		t.Fatal("unexpected fallback ListCommands call")
	}
}

type assertedError string

func (e assertedError) Error() string { return string(e) }

func assertErr(msg string) error { return assertedError(msg) }

type runQueryStoreStub struct {
	*memstore.Store
	activeRuns   []*orchestrator.ActiveRunSummary
	listCalled   bool
	getCalled    bool
	fallbackUsed bool
}

func (s *runQueryStoreStub) ListActiveRuns(_ context.Context, _ orchestrator.ActiveRunFilter) ([]*orchestrator.ActiveRunSummary, error) {
	s.listCalled = true
	out := make([]*orchestrator.ActiveRunSummary, len(s.activeRuns))
	for i, run := range s.activeRuns {
		if run == nil {
			continue
		}
		cp := *run
		out[i] = &cp
	}
	return out, nil
}

func (s *runQueryStoreStub) GetActiveRun(_ context.Context, runID string) (*orchestrator.ActiveRunSummary, error) {
	s.getCalled = true
	for _, run := range s.activeRuns {
		if run != nil && run.RunID == runID {
			cp := *run
			return &cp, nil
		}
	}
	return nil, orchestrator.ErrRunNotFound
}

func (s *runQueryStoreStub) ListTasks(context.Context, orchestrator.TaskFilter) ([]*orchestrator.Task, error) {
	s.fallbackUsed = true
	return nil, errors.New("fallback ListTasks should not be used")
}

type commandQueryStoreStub struct {
	*memstore.Store
	pending      []*orchestrator.Command
	called       bool
	fallbackUsed bool
}

func (s *commandQueryStoreStub) ListPendingCommandsForWorker(_ context.Context, _ string) ([]*orchestrator.Command, error) {
	s.called = true
	out := make([]*orchestrator.Command, len(s.pending))
	for i, command := range s.pending {
		if command == nil {
			continue
		}
		cp := *command
		cp.Metadata = nil
		out[i] = &cp
	}
	return out, nil
}

func (s *commandQueryStoreStub) ListCommands(context.Context, orchestrator.CommandFilter) ([]*orchestrator.Command, error) {
	s.fallbackUsed = true
	return nil, errors.New("fallback ListCommands should not be used")
}

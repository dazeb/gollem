package orchestrator_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
	memstore "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

func TestRecoveryManagerSweepRecoversStateAndCancelsRemoteRun(t *testing.T) {
	store := memstore.NewStore()
	base := time.Unix(100, 0).UTC()

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "expired run",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Second,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	commandTask, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "analysis",
		Input: "recover command",
	})
	if err != nil {
		t.Fatalf("CreateTask commandTask failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: commandTask.ID,
		Reason: "recover me",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	if _, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "dead-worker",
		Now:      base,
	}); err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}

	controller := &recoveryControllerStub{}
	manager := orchestrator.NewRecoveryManager(store, store,
		orchestrator.WithRecoveryController(controller),
		orchestrator.WithRecoveryCommandClaimTimeout(time.Second),
	)

	sweep, err := manager.Sweep(context.Background(), base.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Sweep failed: %v", err)
	}
	if len(sweep.LeaseRecoveries) != 1 {
		t.Fatalf("expected 1 lease recovery, got %d", len(sweep.LeaseRecoveries))
	}
	if len(sweep.CommandRecoveries) != 1 {
		t.Fatalf("expected 1 command recovery, got %d", len(sweep.CommandRecoveries))
	}
	if sweep.LocalRunCancels != 0 || sweep.RemoteRunCancels != 1 {
		t.Fatalf("unexpected sweep cancel counts: %+v", sweep)
	}

	calls := controller.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 remote cancel call, got %d", len(calls))
	}
	if calls[0].runID != claim.Run.ID || !errors.Is(calls[0].cause, orchestrator.ErrLeaseExpired) {
		t.Fatalf("unexpected remote cancel call: %+v", calls[0])
	}

	failed, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if failed.Status != orchestrator.TaskFailed || failed.LastError != "lease expired" {
		t.Fatalf("expected recovered failed task, got %+v", failed)
	}

	recoveredCommand, err := store.GetCommand(context.Background(), command.ID)
	if err != nil {
		t.Fatalf("GetCommand failed: %v", err)
	}
	if recoveredCommand.Status != orchestrator.CommandPending || recoveredCommand.ClaimedBy != "" {
		t.Fatalf("expected recovered pending command, got %+v", recoveredCommand)
	}
}

func TestRecoveryManagerSweepPrefersLocalCanceler(t *testing.T) {
	store := memstore.NewStore()
	base := time.Unix(200, 0).UTC()

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "expired local run",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Second,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	controller := &recoveryControllerStub{}
	var (
		localMu    sync.Mutex
		localCalls []string
	)
	manager := orchestrator.NewRecoveryManager(store, store,
		orchestrator.WithRecoveryController(controller),
		orchestrator.WithRecoveryLocalCanceler(func(task *orchestrator.Task, cause error) bool {
			if task == nil || task.Run == nil {
				return false
			}
			localMu.Lock()
			defer localMu.Unlock()
			if !errors.Is(cause, orchestrator.ErrLeaseExpired) {
				t.Fatalf("expected ErrLeaseExpired cause, got %v", cause)
			}
			localCalls = append(localCalls, task.Run.ID)
			return true
		}),
	)

	sweep, err := manager.Sweep(context.Background(), base.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Sweep failed: %v", err)
	}
	if sweep.LocalRunCancels != 1 || sweep.RemoteRunCancels != 0 {
		t.Fatalf("unexpected sweep cancel counts: %+v", sweep)
	}

	localMu.Lock()
	defer localMu.Unlock()
	if len(localCalls) != 1 || localCalls[0] != claim.Run.ID {
		t.Fatalf("unexpected local cancel calls: %+v", localCalls)
	}
	if len(controller.calls()) != 0 {
		t.Fatalf("expected no remote cancel calls, got %+v", controller.calls())
	}
}

func TestRecoveryManagerSweepWithoutControllerStillRecoversState(t *testing.T) {
	store := memstore.NewStore()
	base := time.Unix(300, 0).UTC()

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "expired without controller",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if _, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Second,
		Now:      base,
	}); err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	manager := orchestrator.NewRecoveryManager(store, store)
	sweep, err := manager.Sweep(context.Background(), base.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Sweep failed: %v", err)
	}
	if len(sweep.LeaseRecoveries) != 1 || sweep.RemoteRunCancels != 0 {
		t.Fatalf("unexpected sweep summary without controller: %+v", sweep)
	}

	failed, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if failed.Status != orchestrator.TaskFailed {
		t.Fatalf("expected task recovery without controller, got %+v", failed)
	}
}

type recoveryCancelCall struct {
	taskID string
	runID  string
	cause  error
}

type recoveryControllerStub struct {
	mu      sync.Mutex
	records []recoveryCancelCall
}

func (s *recoveryControllerStub) CancelRun(_ context.Context, task *orchestrator.Task, run *orchestrator.RunRef, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	call := recoveryCancelCall{cause: cause}
	if task != nil {
		call.taskID = task.ID
	}
	if run != nil {
		call.runID = run.ID
	}
	s.records = append(s.records, call)
	return nil
}

func (s *recoveryControllerStub) calls() []recoveryCancelCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recoveryCancelCall, len(s.records))
	copy(out, s.records)
	return out
}

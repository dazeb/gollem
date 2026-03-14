package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

func TestStore_CreateCommandTargetsRunningTaskOwner(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "cancel me",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "stop",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	if command.RunID != claim.Run.ID {
		t.Fatalf("expected command run id %q, got %q", claim.Run.ID, command.RunID)
	}
	if command.TargetWorkerID != "worker-a" {
		t.Fatalf("expected command target worker %q, got %q", "worker-a", command.TargetWorkerID)
	}
	if command.Status != orchestrator.CommandPending {
		t.Fatalf("expected pending command status, got %s", command.Status)
	}

	if _, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      time.Unix(2, 0).UTC(),
	}); !errors.Is(err, orchestrator.ErrNoPendingCommand) {
		t.Fatalf("expected non-target worker to see no pending command, got %v", err)
	}

	claimed, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      time.Unix(2, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}
	if claimed.ID != command.ID {
		t.Fatalf("expected claimed command %q, got %q", command.ID, claimed.ID)
	}
	if claimed.Status != orchestrator.CommandClaimed {
		t.Fatalf("expected claimed status, got %s", claimed.Status)
	}
}

func TestStore_CommandLifecyclePublishesEvents(t *testing.T) {
	bus := core.NewEventBus()
	store := NewStore(WithEventBus(bus))

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "retry me",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	if _, err := store.FailTask(context.Background(), task.ID, claim.Lease.Token, errors.New("boom"), base.Add(time.Second)); err != nil {
		t.Fatalf("FailTask failed: %v", err)
	}

	var mu sync.Mutex
	var created []orchestrator.CommandCreatedEvent
	var handled []orchestrator.CommandHandledEvent
	core.Subscribe(bus, func(event orchestrator.CommandCreatedEvent) {
		mu.Lock()
		created = append(created, event)
		mu.Unlock()
	})
	core.Subscribe(bus, func(event orchestrator.CommandHandledEvent) {
		mu.Lock()
		handled = append(handled, event)
		mu.Unlock()
	})

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandRetryTask,
		TaskID: task.ID,
		Reason: "try again",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	claimed, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      base.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}
	handledCommand, err := store.HandleCommand(context.Background(), claimed.ID, claimed.ClaimToken, "worker-b", base.Add(3*time.Second))
	if err != nil {
		t.Fatalf("HandleCommand failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(created) == 1 && len(handled) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	if created[0].CommandID != command.ID || created[0].Kind != orchestrator.CommandRetryTask {
		t.Fatalf("unexpected created command event: %+v", created[0])
	}
	if handled[0].CommandID != handledCommand.ID || handled[0].HandledBy != "worker-b" {
		t.Fatalf("unexpected handled command event: %+v", handled[0])
	}
}

func TestStore_CommandClaimAndReleasePublishEvents(t *testing.T) {
	bus := core.NewEventBus()
	store := NewStore(WithEventBus(bus))

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "cancel me later",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	var mu sync.Mutex
	var claimed []orchestrator.CommandClaimedEvent
	var released []orchestrator.CommandReleasedEvent
	core.Subscribe(bus, func(event orchestrator.CommandClaimedEvent) {
		mu.Lock()
		claimed = append(claimed, event)
		mu.Unlock()
	})
	core.Subscribe(bus, func(event orchestrator.CommandReleasedEvent) {
		mu.Lock()
		released = append(released, event)
		mu.Unlock()
	})

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "not yet",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	firstClaim, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}
	if err := store.ReleaseCommand(context.Background(), firstClaim.ID, firstClaim.ClaimToken); err != nil {
		t.Fatalf("ReleaseCommand failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(claimed) == 1 && len(released) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	if claimed[0].CommandID != command.ID || claimed[0].ClaimedBy != "worker-a" {
		t.Fatalf("unexpected claimed command event: %+v", claimed[0])
	}
	if released[0].CommandID != command.ID || released[0].ReleasedBy != "worker-a" {
		t.Fatalf("unexpected released command event: %+v", released[0])
	}
}

func TestStore_CreateCommandRejectsInvalidTaskStates(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "state checks",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if _, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandRetryTask,
		TaskID: task.ID,
	}); !errors.Is(err, orchestrator.ErrTaskNotRetryable) {
		t.Fatalf("expected ErrTaskNotRetryable for pending task, got %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	if _, err := store.CompleteTask(context.Background(), task.ID, claim.Lease.Token, &orchestrator.TaskOutcome{
		Result: &orchestrator.TaskResult{Output: "done"},
	}, base.Add(time.Second)); err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}

	if _, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
	}); !errors.Is(err, orchestrator.ErrTaskNotCancelable) {
		t.Fatalf("expected ErrTaskNotCancelable for completed task, got %v", err)
	}
}

func TestStore_ReleaseCommandReturnsItToPending(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "pending cancel",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	claimed, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}
	if err := store.ReleaseCommand(context.Background(), claimed.ID, claimed.ClaimToken); err != nil {
		t.Fatalf("ReleaseCommand failed: %v", err)
	}

	reclaimed, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      time.Unix(2, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand after release failed: %v", err)
	}
	if reclaimed.ID != command.ID {
		t.Fatalf("expected reclaimed command %q, got %q", command.ID, reclaimed.ID)
	}
	if reclaimed.ClaimedBy != "worker-b" {
		t.Fatalf("expected reclaimed command owner %q, got %q", "worker-b", reclaimed.ClaimedBy)
	}
}

func TestStore_ClaimPendingCommandRetargetsRunningCancelToCurrentWorker(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "retarget me",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	firstClaim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask first failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "stop current run",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	if command.TargetWorkerID != "worker-a" {
		t.Fatalf("expected initial command target %q, got %q", "worker-a", command.TargetWorkerID)
	}

	if _, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(30 * time.Millisecond),
	}); err != nil {
		t.Fatalf("ClaimTask second failed: %v", err)
	}

	claimed, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      base.Add(40 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}
	if claimed.ID != command.ID {
		t.Fatalf("expected claimed command %q, got %q", command.ID, claimed.ID)
	}
	if claimed.TargetWorkerID != "worker-b" {
		t.Fatalf("expected retargeted command owner %q, got %q", "worker-b", claimed.TargetWorkerID)
	}
	if claimed.RunID == firstClaim.Run.ID {
		t.Fatalf("expected command run id to refresh after reclaim, still got %q", claimed.RunID)
	}
}

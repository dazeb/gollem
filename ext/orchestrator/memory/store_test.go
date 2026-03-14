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

func TestStore_ClaimReadyTaskReclaimsExpiredLease(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	firstClaim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if firstClaim.Task.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", firstClaim.Task.Attempt)
	}
	if _, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base.Add(10 * time.Millisecond),
	}); !errors.Is(err, orchestrator.ErrNoReadyTask) {
		t.Fatalf("expected no ready task before lease expiry, got %v", err)
	}

	secondClaim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base.Add(25 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if secondClaim.Task.ID != task.ID {
		t.Fatalf("expected reclaim of %q, got %q", task.ID, secondClaim.Task.ID)
	}
	if secondClaim.Task.Attempt != 2 {
		t.Fatalf("expected attempt 2 after lease expiry, got %d", secondClaim.Task.Attempt)
	}
	if secondClaim.Lease.Token == firstClaim.Lease.Token {
		t.Fatal("expected a new lease token after reclaim")
	}
	if secondClaim.Run == nil || secondClaim.Run.Attempt != 2 {
		t.Fatalf("expected run attempt 2, got %+v", secondClaim.Run)
	}
}

func TestStore_CompleteTaskRequiresActiveLease(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if _, err := store.CompleteTask(context.Background(), task.ID, claim.Lease.Token, &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "done"}}, base.Add(25*time.Millisecond)); !errors.Is(err, orchestrator.ErrLeaseExpired) {
		t.Fatalf("expected ErrLeaseExpired, got %v", err)
	}
}

func TestStore_ClaimReadyTaskSkipsBlockedTasksUntilBlockerCompletes(t *testing.T) {
	store := NewStore()
	blocker, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:    "prompt",
		Subject: "blocker",
		Input:   "finish blocker",
	})
	if err != nil {
		t.Fatalf("CreateTask blocker failed: %v", err)
	}
	blocked, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:      "prompt",
		Subject:   "blocked",
		Input:     "wait behind blocker",
		BlockedBy: []string{blocker.ID},
	})
	if err != nil {
		t.Fatalf("CreateTask blocked failed: %v", err)
	}

	if got, err := store.GetTask(context.Background(), blocker.ID); err != nil {
		t.Fatalf("GetTask blocker failed: %v", err)
	} else if len(got.Blocks) != 1 || got.Blocks[0] != blocked.ID {
		t.Fatalf("expected reciprocal blocker relationship, got %+v", got.Blocks)
	}

	base := time.Unix(1, 0).UTC()
	firstClaim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if firstClaim.Task.ID != blocker.ID {
		t.Fatalf("expected blocker task %q to claim first, got %q", blocker.ID, firstClaim.Task.ID)
	}
	if _, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(time.Second),
	}); !errors.Is(err, orchestrator.ErrNoReadyTask) {
		t.Fatalf("expected blocked task to stay unavailable, got %v", err)
	}

	if _, err := store.CompleteTask(context.Background(), blocker.ID, firstClaim.Lease.Token, &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "done"}}, base.Add(2*time.Second)); err != nil {
		t.Fatalf("CompleteTask blocker failed: %v", err)
	}
	secondClaim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if secondClaim.Task.ID != blocked.ID {
		t.Fatalf("expected blocked task %q after blocker completion, got %q", blocked.ID, secondClaim.Task.ID)
	}
}

func TestStore_CreateTaskRejectsUnknownDependencyIDs(t *testing.T) {
	store := NewStore()

	if _, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:      "prompt",
		Input:     "bad blocked_by",
		BlockedBy: []string{"task-does-not-exist"},
	}); !errors.Is(err, orchestrator.ErrTaskDependencyNotFound) {
		t.Fatalf("expected ErrTaskDependencyNotFound for BlockedBy, got %v", err)
	}

	if _, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:   "prompt",
		Input:  "bad blocks",
		Blocks: []string{"task-does-not-exist"},
	}); !errors.Is(err, orchestrator.ErrTaskDependencyNotFound) {
		t.Fatalf("expected ErrTaskDependencyNotFound for Blocks, got %v", err)
	}

	tasks, err := store.ListTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected invalid tasks to be rejected, got %d stored tasks", len(tasks))
	}
}

func TestStore_FailTaskRetryableRequeuesUntilAttemptsExhausted(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "retry me",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	requeued, err := store.FailTask(context.Background(), task.ID, claim.Lease.Token, orchestrator.Retryable(errors.New("temporary boom")), base.Add(time.Second))
	if err != nil {
		t.Fatalf("FailTask retryable failed: %v", err)
	}
	if requeued.Status != orchestrator.TaskPending {
		t.Fatalf("expected task to requeue to pending, got %s", requeued.Status)
	}
	if requeued.Attempt != 1 {
		t.Fatalf("expected attempt count to stay at 1 after requeue, got %d", requeued.Attempt)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected lease to be released on requeue, got %v", err)
	}

	secondClaim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if secondClaim.Task.Attempt != 2 {
		t.Fatalf("expected attempt 2 on second claim, got %d", secondClaim.Task.Attempt)
	}
	failed, err := store.FailTask(context.Background(), task.ID, secondClaim.Lease.Token, orchestrator.Retryable(errors.New("still broken")), base.Add(3*time.Second))
	if err != nil {
		t.Fatalf("FailTask exhausted failed: %v", err)
	}
	if failed.Status != orchestrator.TaskFailed {
		t.Fatalf("expected final task failure after exhausting attempts, got %s", failed.Status)
	}
	if failed.LastError != "still broken" {
		t.Fatalf("expected final error %q, got %q", "still broken", failed.LastError)
	}
}

func TestStore_ClaimReadyTaskExhaustionClearsExpiredLease(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "one shot",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if _, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base.Add(25 * time.Millisecond),
	}); !errors.Is(err, orchestrator.ErrNoReadyTask) {
		t.Fatalf("expected no ready task after exhausted attempt, got %v", err)
	}

	failed, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if failed.Status != orchestrator.TaskFailed {
		t.Fatalf("expected exhausted task to fail, got %s", failed.Status)
	}
	if failed.LastError != "task exhausted max attempts" {
		t.Fatalf("expected exhausted task error, got %q", failed.LastError)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected exhausted task lease to be removed, got %v", err)
	}
	if failed.Run == nil || failed.Run.ID != claim.Run.ID {
		t.Fatalf("expected failed task to retain last run reference, got %+v", failed.Run)
	}
}

func TestStore_ReleaseLeaseRequeuesWithoutBurningAttempt(t *testing.T) {
	store := NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "release me",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if err := store.ReleaseLease(context.Background(), task.ID, claim.Lease.Token); err != nil {
		t.Fatalf("ReleaseLease failed: %v", err)
	}

	requeued, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if requeued.Status != orchestrator.TaskPending {
		t.Fatalf("expected pending task after release, got %s", requeued.Status)
	}
	if requeued.Attempt != 0 {
		t.Fatalf("expected released task attempt to roll back to 0, got %d", requeued.Attempt)
	}
	if requeued.Run != nil {
		t.Fatalf("expected released task run to be cleared, got %+v", requeued.Run)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected released task lease to be removed, got %v", err)
	}

	reclaimed, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("reclaim failed: %v", err)
	}
	if reclaimed.Task.Attempt != 1 {
		t.Fatalf("expected reclaimed task attempt 1, got %d", reclaimed.Task.Attempt)
	}
}

func TestStore_PublishesLifecycleEvents(t *testing.T) {
	bus := core.NewEventBus()
	store := NewStore(WithEventBus(bus))

	var mu sync.Mutex
	var created []orchestrator.TaskCreatedEvent
	var updated []orchestrator.TaskUpdatedEvent
	var claimed []orchestrator.TaskClaimedEvent
	var renewed []orchestrator.LeaseRenewedEvent
	var completed []orchestrator.TaskCompletedEvent
	var deleted []orchestrator.TaskDeletedEvent

	core.Subscribe(bus, func(e orchestrator.TaskCreatedEvent) {
		mu.Lock()
		created = append(created, e)
		mu.Unlock()
	})
	core.Subscribe(bus, func(e orchestrator.TaskUpdatedEvent) {
		mu.Lock()
		updated = append(updated, e)
		mu.Unlock()
	})
	core.Subscribe(bus, func(e orchestrator.TaskClaimedEvent) {
		mu.Lock()
		claimed = append(claimed, e)
		mu.Unlock()
	})
	core.Subscribe(bus, func(e orchestrator.LeaseRenewedEvent) {
		mu.Lock()
		renewed = append(renewed, e)
		mu.Unlock()
	})
	core.Subscribe(bus, func(e orchestrator.TaskCompletedEvent) {
		mu.Lock()
		completed = append(completed, e)
		mu.Unlock()
	})
	core.Subscribe(bus, func(e orchestrator.TaskDeletedEvent) {
		mu.Lock()
		deleted = append(deleted, e)
		mu.Unlock()
	})

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Subject:     "first",
		Description: "desc",
		Input:       "hello",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	updatedSubject := "renamed"
	if _, err := store.UpdateTask(context.Background(), orchestrator.UpdateTaskRequest{
		ID:      task.ID,
		Subject: &updatedSubject,
	}); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
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
	if _, err := store.RenewLease(context.Background(), task.ID, claim.Lease.Token, time.Minute, base.Add(10*time.Second)); err != nil {
		t.Fatalf("RenewLease failed: %v", err)
	}
	if _, err := store.CompleteTask(context.Background(), task.ID, claim.Lease.Token, &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "done"}}, base.Add(20*time.Second)); err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}

	second, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "bye",
	})
	if err != nil {
		t.Fatalf("CreateTask second failed: %v", err)
	}
	if err := store.DeleteTask(context.Background(), second.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(created) == 2 && len(updated) == 1 && len(claimed) == 1 && len(renewed) == 1 && len(completed) == 1 && len(deleted) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	createdIDs := map[string]bool{}
	for _, event := range created {
		createdIDs[event.TaskID] = true
	}
	if !createdIDs[task.ID] || !createdIDs[second.ID] {
		t.Fatalf("expected created events for %q and %q, got %+v", task.ID, second.ID, created)
	}
	if updated[0].Subject != updatedSubject {
		t.Fatalf("expected updated subject %q, got %q", updatedSubject, updated[0].Subject)
	}
	if claimed[0].WorkerID != "worker-a" || claimed[0].TaskID != task.ID {
		t.Fatalf("unexpected claim event: %+v", claimed[0])
	}
	if renewed[0].LeaseID != claim.Lease.ID {
		t.Fatalf("expected renewed lease %q, got %+v", claim.Lease.ID, renewed[0])
	}
	if completed[0].TaskID != task.ID {
		t.Fatalf("expected completed event for %q, got %+v", task.ID, completed[0])
	}
	if deleted[0].TaskID != second.ID {
		t.Fatalf("expected deleted event for %q, got %+v", second.ID, deleted[0])
	}
}

func TestStore_PublishesRequeueAndFailureEvents(t *testing.T) {
	bus := core.NewEventBus()
	store := NewStore(WithEventBus(bus))

	var mu sync.Mutex
	var requeued []orchestrator.TaskRequeuedEvent
	var failed []orchestrator.TaskFailedEvent
	core.Subscribe(bus, func(e orchestrator.TaskRequeuedEvent) {
		mu.Lock()
		requeued = append(requeued, e)
		mu.Unlock()
	})
	core.Subscribe(bus, func(e orchestrator.TaskFailedEvent) {
		mu.Lock()
		failed = append(failed, e)
		mu.Unlock()
	})

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "retry",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	firstClaim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask first failed: %v", err)
	}
	if _, err := store.FailTask(context.Background(), task.ID, firstClaim.Lease.Token, orchestrator.Retryable(errors.New("temporary")), base.Add(time.Second)); err != nil {
		t.Fatalf("FailTask retryable failed: %v", err)
	}

	secondClaim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimTask second failed: %v", err)
	}
	if _, err := store.FailTask(context.Background(), task.ID, secondClaim.Lease.Token, errors.New("permanent"), base.Add(3*time.Second)); err != nil {
		t.Fatalf("FailTask terminal failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(requeued) == 1 && len(failed) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	if requeued[0].TaskID != task.ID || requeued[0].LastRunID != firstClaim.Run.ID || requeued[0].LastAttempt != 1 {
		t.Fatalf("unexpected requeue event: %+v", requeued[0])
	}
	if failed[0].TaskID != task.ID || failed[0].RunID != secondClaim.Run.ID || failed[0].Error != "permanent" {
		t.Fatalf("unexpected failed event: %+v", failed[0])
	}
}

func TestStore_PublishesPeerDependencyUpdateEvents(t *testing.T) {
	bus := core.NewEventBus()
	store := NewStore(WithEventBus(bus))

	var mu sync.Mutex
	var updated []orchestrator.TaskUpdatedEvent
	core.Subscribe(bus, func(e orchestrator.TaskUpdatedEvent) {
		mu.Lock()
		updated = append(updated, e)
		mu.Unlock()
	})

	blocker, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:    "prompt",
		Subject: "blocker",
		Input:   "first",
	})
	if err != nil {
		t.Fatalf("CreateTask blocker failed: %v", err)
	}

	blocked, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:      "prompt",
		Subject:   "blocked",
		Input:     "second",
		BlockedBy: []string{blocker.ID},
	})
	if err != nil {
		t.Fatalf("CreateTask blocked failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return containsTaskUpdate(updated, blocker.ID, func(e orchestrator.TaskUpdatedEvent) bool {
			return len(e.Blocks) == 1 && e.Blocks[0] == blocked.ID
		})
	})

	if err := store.DeleteTask(context.Background(), blocker.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	waitForAsync(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return containsTaskUpdate(updated, blocked.ID, func(e orchestrator.TaskUpdatedEvent) bool {
			return len(e.BlockedBy) == 0
		})
	})
}

func waitForAsync(t *testing.T, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for async events")
}

func containsTaskUpdate(events []orchestrator.TaskUpdatedEvent, taskID string, match func(orchestrator.TaskUpdatedEvent) bool) bool {
	for _, event := range events {
		if event.TaskID != taskID {
			continue
		}
		if match == nil || match(event) {
			return true
		}
	}
	return false
}

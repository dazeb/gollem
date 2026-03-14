package memory

import (
	"context"
	"errors"
	"testing"
	"time"

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

	if _, err := store.CompleteTask(context.Background(), task.ID, claim.Lease.Token, &orchestrator.TaskResult{Output: "done"}, base.Add(25*time.Millisecond)); !errors.Is(err, orchestrator.ErrLeaseExpired) {
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

	if _, err := store.CompleteTask(context.Background(), blocker.ID, firstClaim.Lease.Token, &orchestrator.TaskResult{Output: "done"}, base.Add(2*time.Second)); err != nil {
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

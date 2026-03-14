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

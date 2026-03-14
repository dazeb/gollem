package orchestrator_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
	memstore "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

func TestScheduler_CompletesTask(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskResult, error) {
		return &orchestrator.TaskResult{Output: "done"}, nil
	})

	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("worker-1"),
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithLeaseTTL(50*time.Millisecond),
		orchestrator.WithLeaseRenewInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	completed := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCompleted)
	if completed.Result == nil || completed.Result.Output != "done" {
		t.Fatalf("expected completed task output %q, got %+v", "done", completed.Result)
	}
	if completed.Run == nil || completed.Run.WorkerID != "worker-1" {
		t.Fatalf("expected task run owned by worker-1, got %+v", completed.Run)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected task lease to be released, got %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scheduler returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler shutdown")
	}
}

func TestScheduler_FailsTask(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "fail",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskResult, error) {
		return nil, errors.New("boom")
	})

	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithLeaseTTL(50*time.Millisecond),
		orchestrator.WithLeaseRenewInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	failed := waitForTaskStatus(t, store, task.ID, orchestrator.TaskFailed)
	if failed.LastError != "boom" {
		t.Fatalf("expected task error %q, got %q", "boom", failed.LastError)
	}
	if failed.Result != nil {
		t.Fatalf("expected failed task to have nil result, got %+v", failed.Result)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scheduler returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler shutdown")
	}
}

func waitForTaskStatus(t *testing.T, store orchestrator.TaskStore, taskID string, status orchestrator.TaskStatus) *orchestrator.Task {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := store.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if task.Status == status {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for task %s to reach status %s", taskID, status)
	return nil
}

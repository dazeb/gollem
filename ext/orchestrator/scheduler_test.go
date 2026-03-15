package orchestrator_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		return &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "done"}}, nil
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

func TestScheduler_PersistsOutcomeArtifactsOnCompletion(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		return &orchestrator.TaskOutcome{
			Result: &orchestrator.TaskResult{Output: "done"},
			Artifacts: []orchestrator.ArtifactSpec{{
				Kind:        "report",
				Name:        "handoff.md",
				ContentType: "text/markdown",
				Body:        []byte("# done"),
			}},
		}, nil
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
	if completed.Run == nil {
		t.Fatalf("expected completed run ref, got %+v", completed.Run)
	}

	artifacts, err := store.ListArtifacts(context.Background(), orchestrator.ArtifactFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 persisted artifact, got %d", len(artifacts))
	}
	if artifacts[0].RunID != completed.Run.ID {
		t.Fatalf("expected artifact run ID %q, got %q", completed.Run.ID, artifacts[0].RunID)
	}
	if string(artifacts[0].Body) != "# done" {
		t.Fatalf("unexpected artifact body %q", string(artifacts[0].Body))
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

	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
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

func TestScheduler_RetryableErrorRequeuesAndCompletesOnLaterAttempt(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "retry",
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	var attempts atomic.Int32
	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			return nil, orchestrator.Retryable(errors.New("temporary"))
		}
		return &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "done"}}, nil
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

	completed := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCompleted)
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
	if completed.Attempt != 2 {
		t.Fatalf("expected stored attempt count 2, got %d", completed.Attempt)
	}
	if completed.Result == nil || completed.Result.Output != "done" {
		t.Fatalf("expected completed result after retry, got %+v", completed.Result)
	}
	if completed.LastError != "" {
		t.Fatalf("expected successful retry to clear LastError, got %q", completed.LastError)
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

func TestScheduler_RetryableErrorStopsAtMaxAttempts(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "retry",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	var attempts atomic.Int32
	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		attempts.Add(1)
		return nil, orchestrator.Retryable(errors.New("temporary"))
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
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts before exhaustion, got %d", attempts.Load())
	}
	if failed.Attempt != 2 {
		t.Fatalf("expected stored attempt count 2, got %d", failed.Attempt)
	}
	if failed.LastError != "temporary" {
		t.Fatalf("expected terminal error %q, got %q", "temporary", failed.LastError)
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

func TestScheduler_CancelRequeuesRunningTaskWithoutBurningAttempt(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "cancel me",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	started := make(chan struct{})
	runner := orchestrator.RunnerFunc(func(ctx context.Context, _ *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})

	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithLeaseTTL(50*time.Millisecond),
		orchestrator.WithLeaseRenewInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner start")
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

	requeued := waitForTaskStatus(t, store, task.ID, orchestrator.TaskPending)
	if requeued.Attempt != 0 {
		t.Fatalf("expected canceled task attempt to roll back to 0, got %d", requeued.Attempt)
	}
	if requeued.Run != nil {
		t.Fatalf("expected canceled task run to be cleared, got %+v", requeued.Run)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected canceled task lease to be removed, got %v", err)
	}

	reclaimed, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      time.Now(),
	})
	if err != nil {
		t.Fatalf("expected canceled task to be reclaimable, got %v", err)
	}
	if reclaimed.Task.Attempt != 1 {
		t.Fatalf("expected reclaimed task attempt 1, got %d", reclaimed.Task.Attempt)
	}
}

func TestScheduler_HandlesPendingCancelCommand(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "cancel before run",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "skip it",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	var runs atomic.Int32
	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		runs.Add(1)
		return &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "unexpected"}}, nil
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

	canceled := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCanceled)
	if canceled.LastError != "skip it" {
		t.Fatalf("expected cancel reason %q, got %q", "skip it", canceled.LastError)
	}
	if canceled.Run != nil {
		t.Fatalf("expected pending cancel to leave no run ref, got %+v", canceled.Run)
	}
	if runs.Load() != 0 {
		t.Fatalf("expected canceled task to never run, got %d runs", runs.Load())
	}
	handled := waitForCommandStatus(t, store, command.ID, orchestrator.CommandHandled)
	if handled.HandledBy != "worker-1" {
		t.Fatalf("expected command handled by worker-1, got %q", handled.HandledBy)
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

func TestScheduler_HandlesRunningCancelCommand(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "cancel while running",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	started := make(chan struct{})
	runner := orchestrator.RunnerFunc(func(ctx context.Context, _ *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
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

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner start")
	}

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "abort run",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	canceled := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCanceled)
	if canceled.LastError != "abort run" {
		t.Fatalf("expected cancel reason %q, got %q", "abort run", canceled.LastError)
	}
	if canceled.Run == nil || canceled.Run.WorkerID != "worker-1" {
		t.Fatalf("expected canceled task run to remain attributed to worker-1, got %+v", canceled.Run)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected running cancel to remove lease, got %v", err)
	}
	handled := waitForCommandStatus(t, store, command.ID, orchestrator.CommandHandled)
	if handled.TargetWorkerID != "worker-1" {
		t.Fatalf("expected command target worker %q, got %q", "worker-1", handled.TargetWorkerID)
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

func TestScheduler_HandlesAbortRunCommandAsTaskFailure(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "abort only the active run",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	started := make(chan struct{})
	runner := orchestrator.RunnerFunc(func(ctx context.Context, _ *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
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

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner start")
	}

	running := waitForTaskStatus(t, store, task.ID, orchestrator.TaskRunning)
	if running.Run == nil {
		t.Fatalf("expected running task run ref, got %+v", running.Run)
	}

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: task.ID,
		RunID:  running.Run.ID,
		Reason: "abort just this run",
	})
	if err != nil {
		t.Fatalf("CreateCommand abort_run failed: %v", err)
	}

	failed := waitForTaskStatus(t, store, task.ID, orchestrator.TaskFailed)
	if failed.LastError != "orchestrator run aborted: abort just this run" {
		t.Fatalf("expected abort failure reason %q, got %q", "orchestrator run aborted: abort just this run", failed.LastError)
	}
	if failed.Run == nil || failed.Run.ID != running.Run.ID {
		t.Fatalf("expected failed task to preserve aborted run %q, got %+v", running.Run.ID, failed.Run)
	}
	handled := waitForCommandStatus(t, store, command.ID, orchestrator.CommandHandled)
	if handled.HandledBy != "worker-1" {
		t.Fatalf("expected abort_run handled by worker-1, got %q", handled.HandledBy)
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

func TestScheduler_HandlesRunningCancelCommandViaRunnerControl(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "cancel remotely",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-1",
		LeaseTTL: time.Minute,
		Now:      time.Now(),
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "remote stop",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	runner := &controlledRunner{}
	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("worker-1"),
		orchestrator.WithPollInterval(5*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	canceled := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCanceled)
	if canceled.LastError != "remote stop" {
		t.Fatalf("expected cancel reason %q, got %q", "remote stop", canceled.LastError)
	}
	if canceled.Run == nil || canceled.Run.ID != claim.Run.ID {
		t.Fatalf("expected canceled task run %q, got %+v", claim.Run.ID, canceled.Run)
	}
	if _, err := store.GetLease(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected canceled task lease to be removed, got %v", err)
	}
	handled := waitForCommandStatus(t, store, command.ID, orchestrator.CommandHandled)
	if handled.HandledBy != "worker-1" {
		t.Fatalf("expected command handled by worker-1, got %q", handled.HandledBy)
	}

	calls := runner.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 remote control call, got %d", len(calls))
	}
	if calls[0].runID != claim.Run.ID {
		t.Fatalf("expected remote control run %q, got %q", claim.Run.ID, calls[0].runID)
	}
	var cancelCause *orchestrator.TaskCancelCause
	if !errors.As(calls[0].cause, &cancelCause) || cancelCause.Reason != "remote stop" {
		t.Fatalf("expected TaskCancelCause(remote stop), got %v", calls[0].cause)
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

func TestScheduler_RecoversStaleClaimedCommandAndHandlesIt(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "recover claimed command",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "recover and cancel",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	if _, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "dead-worker",
		Now:      time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}

	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		t.Fatal("runner should not execute recovered pending cancel task")
		return nil, nil
	})

	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("worker-1"),
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithRecoveryInterval(5*time.Millisecond),
		orchestrator.WithCommandClaimTimeout(5*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	canceled := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCanceled)
	if canceled.LastError != "recover and cancel" {
		t.Fatalf("expected recovered cancel reason %q, got %q", "recover and cancel", canceled.LastError)
	}
	handled := waitForCommandStatus(t, store, command.ID, orchestrator.CommandHandled)
	if handled.HandledBy != "worker-1" {
		t.Fatalf("expected recovered command handled by worker-1, got %q", handled.HandledBy)
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

func TestScheduler_RecoveryCancelsExpiredRemoteRun(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "expired remote run",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	runner := &controlledRunner{}
	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("worker-b"),
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithRecoveryInterval(5*time.Millisecond),
		// Use the real clock so the old lease is already expired on startup.
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	failed := waitForTaskStatus(t, store, task.ID, orchestrator.TaskFailed)
	if failed.LastError != "lease expired" {
		t.Fatalf("expected recovered lease failure reason %q, got %q", "lease expired", failed.LastError)
	}

	calls := runner.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 remote recovery cancel call, got %d", len(calls))
	}
	if calls[0].runID != claim.Run.ID {
		t.Fatalf("expected recovered run id %q, got %q", claim.Run.ID, calls[0].runID)
	}
	if !errors.Is(calls[0].cause, orchestrator.ErrLeaseExpired) {
		t.Fatalf("expected ErrLeaseExpired recovery cause, got %v", calls[0].cause)
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

func TestScheduler_AbortRunWithoutControllerReportsErrorAndLeavesCommandPending(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "abort remotely",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-1",
		LeaseTTL: time.Minute,
		Now:      time.Now(),
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: task.ID,
		RunID:  claim.Run.ID,
		Reason: "fatal invariant",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	errCh := make(chan error, 1)
	scheduler := orchestrator.NewScheduler(store, store, orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		t.Fatal("runner should not be called for already-running task")
		return nil, nil
	}),
		orchestrator.WithWorkerID("worker-1"),
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithSchedulerErrorHandler(func(err error) {
			select {
			case errCh <- err:
			default:
			}
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, orchestrator.ErrRunControlUnavailable) {
			t.Fatalf("expected ErrRunControlUnavailable, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler error callback")
	}

	pending := waitForCommandStatus(t, store, command.ID, orchestrator.CommandPending)
	if pending.ClaimToken != "" || pending.ClaimedBy != "" {
		t.Fatalf("expected command to be released back to pending, got %+v", pending)
	}
	running := waitForTaskStatus(t, store, task.ID, orchestrator.TaskRunning)
	if running.Run == nil || running.Run.ID != claim.Run.ID {
		t.Fatalf("expected task to remain on original running attempt %q, got %+v", claim.Run.ID, running.Run)
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

func TestScheduler_ReportsRunnerErrorAfterCancelWhenTaskAlreadyCanceled(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "cancel while reporting error",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	started := make(chan struct{})
	propagationErr := errors.New("remote cancel failed")
	runner := orchestrator.RunnerFunc(func(ctx context.Context, _ *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		close(started)
		<-ctx.Done()
		return nil, propagationErr
	})

	errCh := make(chan error, 1)
	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithWorkerID("worker-1"),
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithLeaseTTL(50*time.Millisecond),
		orchestrator.WithLeaseRenewInterval(10*time.Millisecond),
		orchestrator.WithSchedulerErrorHandler(func(err error) {
			select {
			case errCh <- err:
			default:
			}
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner start")
	}

	if _, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "abort run",
	}); err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	canceled := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCanceled)
	if canceled.LastError != "abort run" {
		t.Fatalf("expected cancel reason %q, got %q", "abort run", canceled.LastError)
	}

	select {
	case got := <-errCh:
		if !errors.Is(got, propagationErr) {
			t.Fatalf("expected scheduler error %v, got %v", propagationErr, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler error callback")
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

func TestScheduler_HandlesRetryCommandAndRunsTaskAgain(t *testing.T) {
	store := memstore.NewStore()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "retry later",
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
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandRetryTask,
		TaskID: task.ID,
		Reason: "retry now",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	var runs atomic.Int32
	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		runs.Add(1)
		return &orchestrator.TaskOutcome{Result: &orchestrator.TaskResult{Output: "done"}}, nil
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
	if completed.Attempt != 1 {
		t.Fatalf("expected retried task to complete on attempt 1 after reset, got %d", completed.Attempt)
	}
	if completed.Result == nil || completed.Result.Output != "done" {
		t.Fatalf("expected retried task result %q, got %+v", "done", completed.Result)
	}
	if runs.Load() != 1 {
		t.Fatalf("expected exactly one rerun, got %d", runs.Load())
	}
	handled := waitForCommandStatus(t, store, command.ID, orchestrator.CommandHandled)
	if handled.HandledBy != "worker-1" {
		t.Fatalf("expected retry command handled by worker-1, got %q", handled.HandledBy)
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

func TestScheduler_IgnoresContextCancellationFromContextAwareStore(t *testing.T) {
	store := &cancelAwareStore{Store: memstore.NewStore()}
	runner := orchestrator.RunnerFunc(func(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
		t.Fatal("runner should not be called")
		return nil, nil
	})

	scheduler := orchestrator.NewScheduler(store, store, runner,
		orchestrator.WithPollInterval(5*time.Millisecond),
		orchestrator.WithLeaseTTL(50*time.Millisecond),
		orchestrator.WithLeaseRenewInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scheduler returned error after context-aware cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler shutdown")
	}
}

type cancelAwareStore struct {
	*memstore.Store
}

type controlledRunner struct {
	mu        sync.Mutex
	cancelled []controlledRunCall
}

type controlledRunCall struct {
	taskID string
	runID  string
	cause  error
}

func (r *controlledRunner) RunTask(context.Context, *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
	return nil, errors.New("controlled runner should not execute tasks in this test")
}

func (r *controlledRunner) CancelRun(_ context.Context, task *orchestrator.Task, run *orchestrator.RunRef, cause error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := controlledRunCall{cause: cause}
	if task != nil {
		call.taskID = task.ID
	}
	if run != nil {
		call.runID = run.ID
	}
	r.cancelled = append(r.cancelled, call)
	return nil
}

func (r *controlledRunner) calls() []controlledRunCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]controlledRunCall, len(r.cancelled))
	copy(out, r.cancelled)
	return out
}

func (s *cancelAwareStore) ClaimReadyTask(ctx context.Context, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.ClaimReadyTask(ctx, req)
}

func (s *cancelAwareStore) ClaimPendingCommand(ctx context.Context, req orchestrator.ClaimCommandRequest) (*orchestrator.Command, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.ClaimPendingCommand(ctx, req)
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

func waitForCommandStatus(t *testing.T, store orchestrator.CommandStore, commandID string, status orchestrator.CommandStatus) *orchestrator.Command {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		command, err := store.GetCommand(context.Background(), commandID)
		if err != nil {
			t.Fatalf("GetCommand failed: %v", err)
		}
		if command.Status == status {
			return command
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for command %s to reach status %s", commandID, status)
	return nil
}

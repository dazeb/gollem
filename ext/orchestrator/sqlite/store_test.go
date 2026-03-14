package sqlite

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
)

func TestStore_PersistsClaimCompletionAndArtifactsAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "hello",
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
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	running, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask after reopen failed: %v", err)
	}
	if running.Status != orchestrator.TaskRunning {
		t.Fatalf("expected running task after reopen, got %s", running.Status)
	}
	lease, err := store.GetLease(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetLease after reopen failed: %v", err)
	}
	if lease.Token != claim.Lease.Token {
		t.Fatalf("expected persisted lease token %q, got %q", claim.Lease.Token, lease.Token)
	}

	completed, err := store.CompleteTask(context.Background(), task.ID, lease.Token, &orchestrator.TaskOutcome{
		Result: &orchestrator.TaskResult{Output: "done"},
		Artifacts: []orchestrator.ArtifactSpec{{
			Kind:        "report",
			Name:        "handoff.md",
			ContentType: "text/markdown",
			Body:        []byte("# done"),
		}},
	}, base.Add(time.Second))
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}
	if completed.Status != orchestrator.TaskCompleted {
		t.Fatalf("expected completed status, got %s", completed.Status)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close after complete failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("final Close failed: %v", err)
		}
	}()

	persisted, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask persisted failed: %v", err)
	}
	if persisted.Status != orchestrator.TaskCompleted {
		t.Fatalf("expected persisted task completed, got %s", persisted.Status)
	}
	if persisted.Result == nil || persisted.Result.Output != "done" {
		t.Fatalf("expected persisted result %q, got %+v", "done", persisted.Result)
	}

	artifacts, err := store.ListArtifacts(context.Background(), orchestrator.ArtifactFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 persisted artifact, got %d", len(artifacts))
	}
	if string(artifacts[0].Body) != "# done" {
		t.Fatalf("unexpected artifact body %q", string(artifacts[0].Body))
	}
}

func TestStore_PendingCancelCommandSurvivesReopenAndSchedulerHandlesIt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
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
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

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
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	canceled := waitForTaskStatus(t, store, task.ID, orchestrator.TaskCanceled)
	if canceled.LastError != "skip it" {
		t.Fatalf("expected cancel reason %q, got %q", "skip it", canceled.LastError)
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

func TestStore_ReclaimsExpiredLeaseAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "reclaim me",
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
		t.Fatalf("ClaimTask failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	secondClaim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(25 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("ClaimTask reclaim failed: %v", err)
	}
	if secondClaim.Task.Attempt != 2 {
		t.Fatalf("expected attempt 2 after reclaim, got %d", secondClaim.Task.Attempt)
	}
	if secondClaim.Lease.Token == firstClaim.Lease.Token {
		t.Fatal("expected a new lease token after reclaim")
	}
}

func TestStore_RetryTaskClearsTerminalErrorAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
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
	failed, err := store.FailTask(context.Background(), task.ID, claim.Lease.Token, context.DeadlineExceeded, base.Add(time.Second))
	if err != nil {
		t.Fatalf("FailTask failed: %v", err)
	}
	if failed.LastError == "" {
		t.Fatal("expected failed task to retain its terminal error before retry")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	retried, err := store.RetryTask(context.Background(), task.ID, "try again", base.Add(2*time.Second))
	if err != nil {
		t.Fatalf("RetryTask failed: %v", err)
	}
	if retried.Status != orchestrator.TaskPending {
		t.Fatalf("expected retried task pending, got %s", retried.Status)
	}
	if retried.LastError != "" {
		t.Fatalf("expected retry to clear LastError, got %q", retried.LastError)
	}
	if retried.Attempt != 0 {
		t.Fatalf("expected retry to reset attempt to 0, got %d", retried.Attempt)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close after retry failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("final Close failed: %v", err)
		}
	}()

	persisted, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask after retry reopen failed: %v", err)
	}
	if persisted.LastError != "" {
		t.Fatalf("expected persisted retry to keep LastError cleared, got %q", persisted.LastError)
	}
	if persisted.Attempt != 0 {
		t.Fatalf("expected persisted retry attempt 0, got %d", persisted.Attempt)
	}
}

func TestStore_ClaimReadyTaskCommitsExhaustedFailureWhenNoTaskClaimed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "prompt",
		Input:       "one shot",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := time.Unix(1, 0).UTC()
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Second,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	if claim.Task.Attempt != 1 {
		t.Fatalf("expected first attempt 1, got %d", claim.Task.Attempt)
	}

	if _, err := store.ClaimReadyTask(context.Background(), orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Second,
		Now:      base.Add(2 * time.Second),
	}); err != orchestrator.ErrNoReadyTask {
		t.Fatalf("expected no ready task after exhaustion, got %v", err)
	}

	persisted, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if persisted.Status != orchestrator.TaskFailed {
		t.Fatalf("expected exhausted task failed, got %s", persisted.Status)
	}
	if persisted.LastError != "task exhausted max attempts" {
		t.Fatalf("expected exhausted task error, got %q", persisted.LastError)
	}
}

func TestStore_PersistsDurableHistoryAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Subject:     "history",
		Description: "persist event history",
		Input:       "record the lifecycle",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	base := task.CreatedAt.Add(time.Second)
	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	_, err = store.CompleteTask(context.Background(), task.ID, claim.Lease.Token, &orchestrator.TaskOutcome{
		Result: &orchestrator.TaskResult{Output: "done"},
		Artifacts: []orchestrator.ArtifactSpec{{
			Kind:        "report",
			Name:        "handoff.md",
			ContentType: "text/markdown",
			Body:        []byte("# history"),
		}},
	}, base.Add(time.Second))
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("final Close failed: %v", err)
		}
	}()

	events, err := store.ListEvents(context.Background(), orchestrator.EventFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 persisted events, got %d", len(events))
	}

	wantKinds := []orchestrator.EventKind{
		orchestrator.EventTaskCreated,
		orchestrator.EventTaskClaimed,
		orchestrator.EventTaskCompleted,
		orchestrator.EventArtifactCreated,
	}
	for i, want := range wantKinds {
		if events[i].Kind != want {
			t.Fatalf("expected event[%d] kind %s, got %s", i, want, events[i].Kind)
		}
		if events[i].Sequence != int64(i+1) {
			t.Fatalf("expected event[%d] sequence %d, got %d", i, i+1, events[i].Sequence)
		}
	}

	loaded, err := store.GetEvent(context.Background(), events[1].ID)
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	var claimed orchestrator.TaskClaimedEvent
	if err := loaded.DecodePayload(&claimed); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}
	if claimed.TaskID != task.ID || claimed.WorkerID != "worker-a" {
		t.Fatalf("unexpected claimed payload: %+v", claimed)
	}

	timeline, err := orchestrator.LoadTaskTimeline(context.Background(), store, task.ID)
	if err != nil {
		t.Fatalf("LoadTaskTimeline failed: %v", err)
	}
	if len(timeline.Events) != len(events) {
		t.Fatalf("expected task timeline len %d, got %d", len(events), len(timeline.Events))
	}
	if timeline.Latest == nil || timeline.Latest.Kind != orchestrator.EventArtifactCreated {
		t.Fatalf("unexpected task timeline latest event: %+v", timeline.Latest)
	}
}

func TestStore_PersistsCommandLifecycleHistoryAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "command history",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "wait for me",
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
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	store = newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("final Close failed: %v", err)
		}
	}()

	events, err := store.ListEvents(context.Background(), orchestrator.EventFilter{CommandID: command.ID})
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 command events, got %d", len(events))
	}
	wantKinds := []orchestrator.EventKind{
		orchestrator.EventCommandCreated,
		orchestrator.EventCommandClaimed,
		orchestrator.EventCommandReleased,
	}
	for i, want := range wantKinds {
		if events[i].Kind != want {
			t.Fatalf("expected command event[%d] kind %s, got %s", i, want, events[i].Kind)
		}
		if i > 0 && events[i].Sequence <= events[i-1].Sequence {
			t.Fatalf("expected strictly increasing event sequence, got %d then %d", events[i-1].Sequence, events[i].Sequence)
		}
	}

	timeline, err := orchestrator.LoadCommandTimeline(context.Background(), store, command.ID)
	if err != nil {
		t.Fatalf("LoadCommandTimeline failed: %v", err)
	}
	if len(timeline.Events) != len(events) {
		t.Fatalf("expected command timeline len %d, got %d", len(events), len(timeline.Events))
	}
	if timeline.Latest == nil || timeline.Latest.Kind != orchestrator.EventCommandReleased {
		t.Fatalf("unexpected command timeline latest event: %+v", timeline.Latest)
	}
}

func newTestStore(t *testing.T, dbPath string) *Store {
	t.Helper()
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	return store
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

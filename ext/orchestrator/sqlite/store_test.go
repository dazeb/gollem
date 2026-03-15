package sqlite

import (
	"context"
	"errors"
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

func TestStore_TaskCommandLeaseAndArtifactPublicAPIs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	base := time.Unix(7, 0).UTC()
	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Subject:     "initial subject",
		Description: "initial description",
		Input:       "hello",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	tasks, err := store.ListTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("expected created task in list, got %#v", tasks)
	}

	subject := "updated subject"
	description := "updated description"
	updated, err := store.UpdateTask(context.Background(), orchestrator.UpdateTaskRequest{
		ID:          task.ID,
		Subject:     &subject,
		Description: &description,
		Metadata: map[string]any{
			"priority": "high",
		},
	})
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}
	if updated.Subject != subject || updated.Description != description {
		t.Fatalf("expected updated task fields, got %+v", updated)
	}

	claim, err := store.ClaimTask(context.Background(), task.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	lease, err := store.GetLease(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetLease failed: %v", err)
	}
	if lease.Token != claim.Lease.Token {
		t.Fatalf("expected lease token %q, got %q", claim.Lease.Token, lease.Token)
	}

	command, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: task.ID,
		Reason: "cancel maybe",
	})
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}

	commands, err := store.ListCommands(context.Background(), orchestrator.CommandFilter{})
	if err != nil {
		t.Fatalf("ListCommands failed: %v", err)
	}
	if len(commands) != 1 || commands[0].ID != command.ID {
		t.Fatalf("expected created command in list, got %#v", commands)
	}

	artifact, err := store.CreateArtifact(context.Background(), orchestrator.CreateArtifactRequest{
		TaskID:      task.ID,
		RunID:       claim.Run.ID,
		Kind:        "report",
		Name:        "result.txt",
		ContentType: "text/plain",
		Body:        []byte("hello"),
	})
	if err != nil {
		t.Fatalf("CreateArtifact failed: %v", err)
	}

	gotArtifact, err := store.GetArtifact(context.Background(), artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact failed: %v", err)
	}
	if string(gotArtifact.Body) != "hello" {
		t.Fatalf("expected artifact body %q, got %q", "hello", string(gotArtifact.Body))
	}

	artifacts, err := store.ListArtifacts(context.Background(), orchestrator.ArtifactFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("expected artifact in list, got %#v", artifacts)
	}

	if err := store.ReleaseLease(context.Background(), task.ID, claim.Lease.Token); err != nil {
		t.Fatalf("ReleaseLease failed: %v", err)
	}

	released, err := store.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask after release failed: %v", err)
	}
	if released.Status != orchestrator.TaskPending {
		t.Fatalf("expected pending task after lease release, got %s", released.Status)
	}

	if err := store.DeleteTask(context.Background(), task.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}
	if _, err := store.GetTask(context.Background(), task.ID); !errors.Is(err, orchestrator.ErrTaskNotFound) {
		t.Fatalf("expected deleted task lookup to fail, got %v", err)
	}
	if _, err := store.GetArtifact(context.Background(), artifact.ID); !errors.Is(err, orchestrator.ErrArtifactNotFound) {
		t.Fatalf("expected deleted task artifact lookup to fail, got %v", err)
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

func TestStore_AbortRunCommandValidatesCurrentRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "abort this run",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if _, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: task.ID,
	}); !errors.Is(err, orchestrator.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for pending task, got %v", err)
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
		Kind:   orchestrator.CommandAbortRun,
		TaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("CreateCommand abort_run failed: %v", err)
	}
	if command.RunID != claim.Run.ID {
		t.Fatalf("expected abort_run command run id %q, got %q", claim.Run.ID, command.RunID)
	}
	if command.TargetWorkerID != "worker-a" {
		t.Fatalf("expected abort_run target worker %q, got %q", "worker-a", command.TargetWorkerID)
	}
	if _, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: task.ID,
		RunID:  "run-mismatch",
	}); !errors.Is(err, orchestrator.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for mismatched run id, got %v", err)
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
	if _, err := store.RenewLease(context.Background(), task.ID, claim.Lease.Token, time.Minute, base.Add(500*time.Millisecond)); err != nil {
		t.Fatalf("RenewLease failed: %v", err)
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
	if len(events) != 5 {
		t.Fatalf("expected 5 persisted events, got %d", len(events))
	}

	wantKinds := []orchestrator.EventKind{
		orchestrator.EventTaskCreated,
		orchestrator.EventTaskClaimed,
		orchestrator.EventLeaseRenewed,
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

	incremental, err := store.ListEvents(context.Background(), orchestrator.EventFilter{
		TaskID:        task.ID,
		AfterSequence: 2,
		Limit:         2,
	})
	if err != nil {
		t.Fatalf("incremental ListEvents failed: %v", err)
	}
	if len(incremental) != 2 {
		t.Fatalf("expected 2 incremental persisted events, got %d", len(incremental))
	}
	if incremental[0].Sequence != 3 || incremental[1].Sequence != 4 {
		t.Fatalf("unexpected incremental persisted event sequences: %+v", incremental)
	}

	replayPage, replayCursor, err := orchestrator.ReplayEvents(context.Background(), store, orchestrator.EventFilter{
		TaskID: task.ID,
		Limit:  3,
	}, orchestrator.ReplayCursor{})
	if err != nil {
		t.Fatalf("ReplayEvents failed: %v", err)
	}
	if len(replayPage) != 3 {
		t.Fatalf("expected 3 replayed persisted events, got %d", len(replayPage))
	}
	if replayCursor.AfterSequence != 3 {
		t.Fatalf("expected replay cursor 3, got %d", replayCursor.AfterSequence)
	}
	if replayPage[0].Record.Kind != orchestrator.EventTaskCreated || replayPage[2].Record.Kind != orchestrator.EventLeaseRenewed {
		t.Fatalf("unexpected replay page kinds: %+v", replayPage)
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

	runTimeline, err := orchestrator.LoadRunTimeline(context.Background(), store, claim.Run.ID)
	if err != nil {
		t.Fatalf("LoadRunTimeline failed: %v", err)
	}
	if len(runTimeline.Events) != 4 {
		t.Fatalf("expected run timeline len 4, got %d", len(runTimeline.Events))
	}
	if runTimeline.TaskID != task.ID {
		t.Fatalf("expected run timeline task %q, got %q", task.ID, runTimeline.TaskID)
	}
	if runTimeline.WorkerID != "worker-a" {
		t.Fatalf("expected run timeline worker %q, got %q", "worker-a", runTimeline.WorkerID)
	}
	if runTimeline.Attempt != 1 {
		t.Fatalf("expected run timeline attempt 1, got %d", runTimeline.Attempt)
	}
	if runTimeline.Terminal == nil || runTimeline.Terminal.Kind != orchestrator.EventTaskCompleted {
		t.Fatalf("unexpected run timeline terminal event: %+v", runTimeline.Terminal)
	}
	if runTimeline.Latest == nil || runTimeline.Latest.Kind != orchestrator.EventArtifactCreated {
		t.Fatalf("unexpected run timeline latest event: %+v", runTimeline.Latest)
	}

	runSummary, err := orchestrator.GetRun(context.Background(), store, claim.Run.ID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if runSummary.Status != orchestrator.RunCompleted {
		t.Fatalf("expected run summary status %s, got %s", orchestrator.RunCompleted, runSummary.Status)
	}
	if runSummary.TerminalKind != orchestrator.EventTaskCompleted {
		t.Fatalf("expected run summary terminal kind %s, got %s", orchestrator.EventTaskCompleted, runSummary.TerminalKind)
	}

	runs, err := orchestrator.ListRuns(context.Background(), store, orchestrator.RunFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != claim.Run.ID {
		t.Fatalf("expected persisted run summary for %q, got %+v", claim.Run.ID, runs)
	}

	workerTimeline, err := orchestrator.LoadWorkerTimeline(context.Background(), store, "worker-a")
	if err != nil {
		t.Fatalf("LoadWorkerTimeline failed: %v", err)
	}
	if len(workerTimeline.Runs) != 1 || workerTimeline.Runs[0].ID != claim.Run.ID {
		t.Fatalf("unexpected persisted worker runs: %+v", workerTimeline.Runs)
	}
	if len(workerTimeline.Events) != 4 {
		t.Fatalf("expected persisted worker timeline len 4, got %d", len(workerTimeline.Events))
	}
	if workerTimeline.Latest == nil || workerTimeline.Latest.Kind != orchestrator.EventArtifactCreated {
		t.Fatalf("unexpected persisted worker latest event: %+v", workerTimeline.Latest)
	}

	workerSummary, err := orchestrator.GetWorker(context.Background(), store, "worker-a")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if workerSummary.CompletedRuns != 1 || workerSummary.LatestRunID != claim.Run.ID {
		t.Fatalf("unexpected persisted worker summary: %+v", workerSummary)
	}

	workers, err := orchestrator.ListWorkers(context.Background(), store, orchestrator.WorkerFilter{
		IDs: []string{"worker-a"},
	})
	if err != nil {
		t.Fatalf("ListWorkers failed: %v", err)
	}
	if len(workers) != 1 || workers[0].ID != "worker-a" {
		t.Fatalf("unexpected persisted worker list: %+v", workers)
	}
}

func TestStore_CurrentStateQueriesUsePersistedIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	base := time.Unix(20, 0).UTC()
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
	if _, err := store.ClaimTask(context.Background(), taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base.Add(time.Second),
	}); err != nil {
		t.Fatalf("ClaimTask taskB failed: %v", err)
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
	if _, err := store.FailTask(context.Background(), taskC.ID, claimC.Lease.Token, errors.New("boom"), base.Add(3*time.Second)); err != nil {
		t.Fatalf("FailTask taskC failed: %v", err)
	}
	commandC, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandRetryTask,
		TaskID: taskC.ID,
		Reason: "retry me",
	})
	if err != nil {
		t.Fatalf("CreateCommand retry failed: %v", err)
	}
	if _, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: taskA.ID,
		RunID:  claimA.Run.ID,
		Reason: "worker a only",
	}); err != nil {
		t.Fatalf("CreateCommand abort failed: %v", err)
	}

	active, err := orchestrator.ListActiveRuns(context.Background(), store, orchestrator.ActiveRunFilter{
		WorkerID: "worker-a",
		Kinds:    []string{"analysis"},
	})
	if err != nil {
		t.Fatalf("ListActiveRuns failed: %v", err)
	}
	if len(active) != 1 || active[0].RunID != claimA.Run.ID {
		t.Fatalf("unexpected active runs: %+v", active)
	}

	run, err := orchestrator.GetActiveRun(context.Background(), store, claimA.Run.ID)
	if err != nil {
		t.Fatalf("GetActiveRun failed: %v", err)
	}
	if run.WorkerID != "worker-a" || run.TaskID != taskA.ID {
		t.Fatalf("unexpected active run summary: %+v", run)
	}

	pending, err := orchestrator.ListPendingCommandsForWorker(context.Background(), store, "worker-a")
	if err != nil {
		t.Fatalf("ListPendingCommandsForWorker failed: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending commands for worker-a, got %d", len(pending))
	}
	got := map[string]string{
		pending[0].ID: pending[0].TargetWorkerID,
		pending[1].ID: pending[1].TargetWorkerID,
	}
	if got[commandC.ID] != "" {
		t.Fatalf("expected retry command %q to be untargeted, got %+v", commandC.ID, pending)
	}
	delete(got, commandC.ID)
	for id, target := range got {
		if target != "worker-a" {
			t.Fatalf("expected targeted pending command for worker-a, got id=%q target=%q full=%+v", id, target, pending)
		}
	}
}

func TestStore_RecoverExpiredLeasesRequeuesAndFailsPersistedTasks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	base := time.Unix(30, 0).UTC()
	requeueTask, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "recover me",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask requeueTask failed: %v", err)
	}
	requeueClaim, err := store.ClaimTask(context.Background(), requeueTask.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask requeueTask failed: %v", err)
	}

	failTask, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "one shot",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask failTask failed: %v", err)
	}
	failClaim, err := store.ClaimTask(context.Background(), failTask.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimTask failTask failed: %v", err)
	}

	recovered, err := store.RecoverExpiredLeases(context.Background(), base.Add(2*time.Second))
	if err != nil {
		t.Fatalf("RecoverExpiredLeases failed: %v", err)
	}
	if len(recovered) != 2 {
		t.Fatalf("expected 2 recovered leases, got %d", len(recovered))
	}

	requeued, err := store.GetTask(context.Background(), requeueTask.ID)
	if err != nil {
		t.Fatalf("GetTask requeueTask failed: %v", err)
	}
	if requeued.Status != orchestrator.TaskPending || requeued.LastError != "lease expired" {
		t.Fatalf("expected requeued task pending with lease-expired marker, got %+v", requeued)
	}
	if _, err := store.GetLease(context.Background(), requeueTask.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected requeued task lease removed, got %v", err)
	}
	if recovered[0].Task == nil || recovered[0].Task.Run == nil || (recovered[0].Task.Run.ID != requeueClaim.Run.ID && recovered[1].Task.Run.ID != requeueClaim.Run.ID) {
		t.Fatalf("expected recovered snapshot for run %q, got %+v", requeueClaim.Run.ID, recovered)
	}

	failed, err := store.GetTask(context.Background(), failTask.ID)
	if err != nil {
		t.Fatalf("GetTask failTask failed: %v", err)
	}
	if failed.Status != orchestrator.TaskFailed || failed.LastError != "lease expired" {
		t.Fatalf("expected failed task after exhausted recovery, got %+v", failed)
	}
	if _, err := store.GetLease(context.Background(), failTask.ID); !errors.Is(err, orchestrator.ErrLeaseNotFound) {
		t.Fatalf("expected failed task lease removed, got %v", err)
	}
	foundFailedRun := false
	for _, recovery := range recovered {
		if recovery != nil && recovery.Task != nil && recovery.Task.Run != nil && recovery.Task.Run.ID == failClaim.Run.ID {
			foundFailedRun = recovery.ResultStatus == orchestrator.TaskFailed
		}
	}
	if !foundFailedRun {
		t.Fatalf("expected failed recovery record for run %q, got %+v", failClaim.Run.ID, recovered)
	}

	recoveryEvents, err := orchestrator.ListLeaseRecoveries(context.Background(), store, orchestrator.RecoveryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListLeaseRecoveries failed: %v", err)
	}
	if len(recoveryEvents) != 2 {
		t.Fatalf("expected 2 lease recovery events, got %d", len(recoveryEvents))
	}
	if recoveryEvents[0].LeaseID != requeueClaim.Lease.ID || !recoveryEvents[0].Requeued || recoveryEvents[0].ResultStatus != orchestrator.TaskPending {
		t.Fatalf("unexpected first lease recovery event: %+v", recoveryEvents[0])
	}
	if recoveryEvents[1].LeaseID != failClaim.Lease.ID || recoveryEvents[1].Requeued || recoveryEvents[1].ResultStatus != orchestrator.TaskFailed {
		t.Fatalf("unexpected second lease recovery event: %+v", recoveryEvents[1])
	}
}

func TestStore_RecoverExpiredLeaseTargetsSinglePersistedTask(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	base := time.Unix(40, 0).UTC()
	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "recover a",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask taskA failed: %v", err)
	}
	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "analysis",
		Input:       "recover b",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask taskB failed: %v", err)
	}

	if _, err := store.ClaimTask(context.Background(), taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	}); err != nil {
		t.Fatalf("ClaimTask taskA failed: %v", err)
	}
	if _, err := store.ClaimTask(context.Background(), taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: 20 * time.Millisecond,
		Now:      base,
	}); err != nil {
		t.Fatalf("ClaimTask taskB failed: %v", err)
	}

	recovery, err := store.RecoverExpiredLease(context.Background(), taskA.ID, base.Add(2*time.Second))
	if err != nil {
		t.Fatalf("RecoverExpiredLease failed: %v", err)
	}
	if recovery == nil || recovery.Task == nil || recovery.Task.ID != taskA.ID {
		t.Fatalf("expected recovery for taskA, got %+v", recovery)
	}
	if !recovery.Requeued || recovery.ResultStatus != orchestrator.TaskPending {
		t.Fatalf("expected targeted recovery to requeue taskA, got %+v", recovery)
	}

	persistedA, err := store.GetTask(context.Background(), taskA.ID)
	if err != nil {
		t.Fatalf("GetTask taskA failed: %v", err)
	}
	if persistedA.Status != orchestrator.TaskPending {
		t.Fatalf("expected taskA pending after targeted recovery, got %s", persistedA.Status)
	}

	persistedB, err := store.GetTask(context.Background(), taskB.ID)
	if err != nil {
		t.Fatalf("GetTask taskB failed: %v", err)
	}
	if persistedB.Status != orchestrator.TaskRunning {
		t.Fatalf("expected taskB to remain running, got %s", persistedB.Status)
	}
}

func TestStore_RecoverClaimedCommandsReturnsPersistedCommandToPending(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	task, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "recover command",
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
		WorkerID: "dead-worker",
		Now:      time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand failed: %v", err)
	}

	recovered, err := store.RecoverClaimedCommands(context.Background(), time.Unix(2, 0).UTC(), time.Unix(3, 0).UTC())
	if err != nil {
		t.Fatalf("RecoverClaimedCommands failed: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered command, got %d", len(recovered))
	}
	if recovered[0].ReleasedBy != "dead-worker" {
		t.Fatalf("expected released by %q, got %q", "dead-worker", recovered[0].ReleasedBy)
	}
	if recovered[0].Command == nil || recovered[0].Command.ID != command.ID || recovered[0].Command.Status != orchestrator.CommandPending {
		t.Fatalf("expected pending recovered command %q, got %+v", command.ID, recovered[0].Command)
	}

	reclaimed, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      time.Unix(4, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand after recovery failed: %v", err)
	}
	if reclaimed.ID != claimed.ID || reclaimed.ClaimedBy != "worker-a" {
		t.Fatalf("unexpected reclaimed command after recovery: %+v", reclaimed)
	}

	recoveryEvents, err := orchestrator.ListCommandRecoveries(context.Background(), store, orchestrator.RecoveryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListCommandRecoveries failed: %v", err)
	}
	if len(recoveryEvents) != 1 {
		t.Fatalf("expected 1 command recovery event, got %d", len(recoveryEvents))
	}
	if recoveryEvents[0].CommandID != command.ID || recoveryEvents[0].ReleasedBy != "dead-worker" || recoveryEvents[0].Reason != "claim expired" {
		t.Fatalf("unexpected command recovery event: %+v", recoveryEvents[0])
	}
}

func TestStore_RecoverClaimedCommandTargetsSinglePersistedCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "recover command a",
	})
	if err != nil {
		t.Fatalf("CreateTask taskA failed: %v", err)
	}
	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "prompt",
		Input: "recover command b",
	})
	if err != nil {
		t.Fatalf("CreateTask taskB failed: %v", err)
	}

	commandA, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: taskA.ID,
	})
	if err != nil {
		t.Fatalf("CreateCommand commandA failed: %v", err)
	}
	commandB, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: taskB.ID,
	})
	if err != nil {
		t.Fatalf("CreateCommand commandB failed: %v", err)
	}
	if _, err := store.ClaimCommand(context.Background(), commandA.ID, orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("ClaimCommand commandA failed: %v", err)
	}
	if _, err := store.ClaimCommand(context.Background(), commandB.ID, orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("ClaimCommand commandB failed: %v", err)
	}

	recovery, err := store.RecoverClaimedCommand(context.Background(), commandA.ID, time.Unix(2, 0).UTC(), time.Unix(3, 0).UTC())
	if err != nil {
		t.Fatalf("RecoverClaimedCommand failed: %v", err)
	}
	if recovery == nil || recovery.Command == nil || recovery.Command.ID != commandA.ID {
		t.Fatalf("expected recovery for commandA, got %+v", recovery)
	}
	if recovery.Command.Status != orchestrator.CommandPending {
		t.Fatalf("expected commandA pending after recovery, got %+v", recovery.Command)
	}

	persistedA, err := store.GetCommand(context.Background(), commandA.ID)
	if err != nil {
		t.Fatalf("GetCommand commandA failed: %v", err)
	}
	if persistedA.Status != orchestrator.CommandPending {
		t.Fatalf("expected commandA pending in store, got %s", persistedA.Status)
	}

	persistedB, err := store.GetCommand(context.Background(), commandB.ID)
	if err != nil {
		t.Fatalf("GetCommand commandB failed: %v", err)
	}
	if persistedB.Status != orchestrator.CommandClaimed {
		t.Fatalf("expected commandB to remain claimed, got %s", persistedB.Status)
	}
}

func TestStore_RecoveryInspectionQueries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "orchestrator.db")

	store := newTestStore(t, dbPath)
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	base := time.Unix(60, 0).UTC()
	taskA, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "analysis",
		Input: "lease a",
	})
	if err != nil {
		t.Fatalf("CreateTask taskA failed: %v", err)
	}
	claimA, err := store.ClaimTask(context.Background(), taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Second,
		Now:      base,
	})
	if err != nil {
		t.Fatalf("ClaimTask taskA failed: %v", err)
	}

	taskB, err := store.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:  "analysis",
		Input: "lease b",
	})
	if err != nil {
		t.Fatalf("CreateTask taskB failed: %v", err)
	}
	claimB, err := store.ClaimTask(context.Background(), taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: 2 * time.Second,
		Now:      base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimTask taskB failed: %v", err)
	}

	expired, err := orchestrator.ListExpiredLeases(context.Background(), store, base.Add(5*time.Second))
	if err != nil {
		t.Fatalf("ListExpiredLeases failed: %v", err)
	}
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired leases, got %d", len(expired))
	}
	if expired[0].LeaseID != claimA.Lease.ID || expired[1].LeaseID != claimB.Lease.ID {
		t.Fatalf("unexpected expired lease order: %+v", expired)
	}
	if expired[0].RunID != claimA.Run.ID || expired[1].RunID != claimB.Run.ID {
		t.Fatalf("unexpected expired lease runs: %+v", expired)
	}

	commandA, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: taskA.ID,
		RunID:  claimA.Run.ID,
		Reason: "stop a",
	})
	if err != nil {
		t.Fatalf("CreateCommand commandA failed: %v", err)
	}
	claimedA, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      base.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand claimedA failed: %v", err)
	}
	if claimedA.ID != commandA.ID {
		t.Fatalf("expected claimed command %q, got %q", commandA.ID, claimedA.ID)
	}

	commandB, err := store.CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandAbortRun,
		TaskID: taskB.ID,
		RunID:  claimB.Run.ID,
		Reason: "stop b",
	})
	if err != nil {
		t.Fatalf("CreateCommand commandB failed: %v", err)
	}
	claimedB, err := store.ClaimPendingCommand(context.Background(), orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      base.Add(11 * time.Second),
	})
	if err != nil {
		t.Fatalf("ClaimPendingCommand claimedB failed: %v", err)
	}
	if claimedB.ID != commandB.ID {
		t.Fatalf("expected claimed command %q, got %q", commandB.ID, claimedB.ID)
	}

	stale, err := orchestrator.ListStaleClaimedCommands(context.Background(), store, base.Add(20*time.Second))
	if err != nil {
		t.Fatalf("ListStaleClaimedCommands failed: %v", err)
	}
	if len(stale) != 2 {
		t.Fatalf("expected 2 stale claimed commands, got %d", len(stale))
	}
	if stale[0].CommandID != claimedA.ID || stale[1].CommandID != claimedB.ID {
		t.Fatalf("unexpected stale claimed command order: %+v", stale)
	}
	if stale[0].ClaimedBy != "worker-a" || stale[1].ClaimedBy != "worker-b" {
		t.Fatalf("unexpected stale claimed command workers: %+v", stale)
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
	released, err := orchestrator.DecodeEvent(events[2])
	if err != nil {
		t.Fatalf("DecodeEvent released failed: %v", err)
	}
	releasedPayload, ok := released.Payload.(*orchestrator.CommandReleasedEvent)
	if !ok {
		t.Fatalf("expected CommandReleasedEvent payload, got %T", released.Payload)
	}
	if releasedPayload.Recovered || releasedPayload.Reason != "command released" {
		t.Fatalf("unexpected persisted command release payload: %+v", releasedPayload)
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

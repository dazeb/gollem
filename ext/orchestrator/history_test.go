package orchestrator_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
)

type fakeEventStore struct {
	events []*orchestrator.EventRecord
}

func (s fakeEventStore) GetEvent(_ context.Context, id string) (*orchestrator.EventRecord, error) {
	for _, event := range s.events {
		if event.ID == id {
			return cloneRecord(event), nil
		}
	}
	return nil, orchestrator.ErrEventNotFound
}

func (s fakeEventStore) ListEvents(_ context.Context, filter orchestrator.EventFilter) ([]*orchestrator.EventRecord, error) {
	var out []*orchestrator.EventRecord
	for _, event := range s.events {
		if filter.AfterSequence > 0 && event.Sequence <= filter.AfterSequence {
			continue
		}
		if len(filter.Kinds) > 0 {
			match := false
			for _, kind := range filter.Kinds {
				if event.Kind == kind {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if filter.TaskID != "" && event.TaskID != filter.TaskID {
			continue
		}
		if filter.RunID != "" && event.RunID != filter.RunID {
			continue
		}
		if filter.LeaseID != "" && event.LeaseID != filter.LeaseID {
			continue
		}
		if filter.CommandID != "" && event.CommandID != filter.CommandID {
			continue
		}
		if filter.ArtifactID != "" && event.ArtifactID != filter.ArtifactID {
			continue
		}
		out = append(out, cloneRecord(event))
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
	}
	return out, nil
}

func TestDecodeEventAndTimelineHelpers(t *testing.T) {
	base := time.Unix(1, 0).UTC()
	taskCreated := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  1,
		ID:        "event-1",
		Kind:      orchestrator.EventTaskCreated,
		TaskID:    "task-1",
		CreatedAt: base,
	}, orchestrator.TaskCreatedEvent{
		TaskID:    "task-1",
		Kind:      "analysis",
		CreatedAt: base,
	})
	leaseRenewed := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  2,
		ID:        "event-2",
		Kind:      orchestrator.EventLeaseRenewed,
		TaskID:    "task-1",
		RunID:     "run-1",
		LeaseID:   "lease-1",
		CreatedAt: base.Add(time.Second),
	}, orchestrator.LeaseRenewedEvent{
		TaskID:    "task-1",
		RunID:     "run-1",
		LeaseID:   "lease-1",
		WorkerID:  "worker-a",
		ExpiresAt: base.Add(2 * time.Second),
	})
	taskCompleted := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  3,
		ID:        "event-3",
		Kind:      orchestrator.EventTaskCompleted,
		TaskID:    "task-1",
		RunID:     "run-1",
		CreatedAt: base.Add(2 * time.Second),
	}, orchestrator.TaskCompletedEvent{
		TaskID:      "task-1",
		RunID:       "run-1",
		Attempt:     1,
		CompletedAt: base.Add(2 * time.Second),
	})
	commandCreated := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  4,
		ID:        "event-4",
		Kind:      orchestrator.EventCommandCreated,
		TaskID:    "task-1",
		RunID:     "run-1",
		CommandID: "command-1",
		CreatedAt: base.Add(3 * time.Second),
	}, orchestrator.CommandCreatedEvent{
		CommandID: "command-1",
		Kind:      orchestrator.CommandCancelTask,
		TaskID:    "task-1",
		RunID:     "run-1",
		CreatedAt: base.Add(3 * time.Second),
	})
	commandReleased := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  5,
		ID:        "event-5",
		Kind:      orchestrator.EventCommandReleased,
		TaskID:    "task-1",
		RunID:     "run-1",
		CommandID: "command-1",
		CreatedAt: base.Add(4 * time.Second),
	}, orchestrator.CommandReleasedEvent{
		CommandID:  "command-1",
		Kind:       orchestrator.CommandCancelTask,
		TaskID:     "task-1",
		RunID:      "run-1",
		ReleasedBy: "worker-a",
		ReleasedAt: base.Add(4 * time.Second),
	})
	secondRunClaimed := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  6,
		ID:        "event-6",
		Kind:      orchestrator.EventTaskClaimed,
		TaskID:    "task-2",
		RunID:     "run-2",
		LeaseID:   "lease-2",
		CreatedAt: base.Add(5 * time.Second),
	}, orchestrator.TaskClaimedEvent{
		TaskID:     "task-2",
		RunID:      "run-2",
		LeaseID:    "lease-2",
		WorkerID:   "worker-b",
		Attempt:    1,
		AcquiredAt: base.Add(5 * time.Second),
		ExpiresAt:  base.Add(6 * time.Second),
	})
	secondRunFailed := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  7,
		ID:        "event-7",
		Kind:      orchestrator.EventTaskFailed,
		TaskID:    "task-2",
		RunID:     "run-2",
		CreatedAt: base.Add(6 * time.Second),
	}, orchestrator.TaskFailedEvent{
		TaskID:   "task-2",
		RunID:    "run-2",
		Attempt:  1,
		Error:    "boom",
		FailedAt: base.Add(6 * time.Second),
	})

	decoded, err := orchestrator.DecodeEvent(taskCompleted)
	if err != nil {
		t.Fatalf("DecodeEvent failed: %v", err)
	}
	completedPayload, ok := decoded.Payload.(*orchestrator.TaskCompletedEvent)
	if !ok {
		t.Fatalf("expected TaskCompletedEvent payload, got %T", decoded.Payload)
	}
	if completedPayload.RunID != "run-1" {
		t.Fatalf("expected decoded run id %q, got %q", "run-1", completedPayload.RunID)
	}

	store := fakeEventStore{events: []*orchestrator.EventRecord{
		taskCreated,
		leaseRenewed,
		taskCompleted,
		commandCreated,
		commandReleased,
		secondRunClaimed,
		secondRunFailed,
	}}

	taskTimeline, err := orchestrator.LoadTaskTimeline(context.Background(), store, "task-1")
	if err != nil {
		t.Fatalf("LoadTaskTimeline failed: %v", err)
	}
	if len(taskTimeline.Events) != 5 {
		t.Fatalf("expected 5 task timeline events, got %d", len(taskTimeline.Events))
	}
	if taskTimeline.Latest == nil || taskTimeline.Latest.Kind != orchestrator.EventCommandReleased {
		t.Fatalf("unexpected task timeline latest event: %+v", taskTimeline.Latest)
	}

	commandTimeline, err := orchestrator.LoadCommandTimeline(context.Background(), store, "command-1")
	if err != nil {
		t.Fatalf("LoadCommandTimeline failed: %v", err)
	}
	if len(commandTimeline.Events) != 2 {
		t.Fatalf("expected 2 command timeline events, got %d", len(commandTimeline.Events))
	}
	if commandTimeline.Latest == nil || commandTimeline.Latest.Kind != orchestrator.EventCommandReleased {
		t.Fatalf("unexpected command timeline latest event: %+v", commandTimeline.Latest)
	}
	releasedPayload, ok := commandTimeline.Events[1].Payload.(*orchestrator.CommandReleasedEvent)
	if !ok {
		t.Fatalf("expected CommandReleasedEvent payload, got %T", commandTimeline.Events[1].Payload)
	}
	if releasedPayload.ReleasedBy != "worker-a" {
		t.Fatalf("expected released by %q, got %q", "worker-a", releasedPayload.ReleasedBy)
	}

	runTimeline, err := orchestrator.LoadRunTimeline(context.Background(), store, "run-1")
	if err != nil {
		t.Fatalf("LoadRunTimeline failed: %v", err)
	}
	if len(runTimeline.Events) != 4 {
		t.Fatalf("expected 4 run timeline events, got %d", len(runTimeline.Events))
	}
	if runTimeline.TaskID != "task-1" {
		t.Fatalf("expected run task id %q, got %q", "task-1", runTimeline.TaskID)
	}
	if runTimeline.WorkerID != "worker-a" {
		t.Fatalf("expected run worker id %q, got %q", "worker-a", runTimeline.WorkerID)
	}
	if runTimeline.Attempt != 1 {
		t.Fatalf("expected run attempt 1, got %d", runTimeline.Attempt)
	}
	if runTimeline.Terminal == nil || runTimeline.Terminal.Kind != orchestrator.EventTaskCompleted {
		t.Fatalf("unexpected run terminal event: %+v", runTimeline.Terminal)
	}
	if !runTimeline.CompletedAt.Equal(base.Add(2 * time.Second)) {
		t.Fatalf("expected run completed at %s, got %s", base.Add(2*time.Second), runTimeline.CompletedAt)
	}

	runSummary, err := orchestrator.GetRun(context.Background(), store, "run-1")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if runSummary.Status != orchestrator.RunCompleted {
		t.Fatalf("expected run summary status %s, got %s", orchestrator.RunCompleted, runSummary.Status)
	}
	if runSummary.TerminalKind != orchestrator.EventTaskCompleted {
		t.Fatalf("expected run summary terminal kind %s, got %s", orchestrator.EventTaskCompleted, runSummary.TerminalKind)
	}
	if runSummary.LatestKind != orchestrator.EventCommandReleased {
		t.Fatalf("expected run summary latest kind %s, got %s", orchestrator.EventCommandReleased, runSummary.LatestKind)
	}

	runs, err := orchestrator.ListRuns(context.Background(), store, orchestrator.RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 projected runs, got %d", len(runs))
	}
	if runs[0].ID != "run-1" || runs[1].ID != "run-2" {
		t.Fatalf("unexpected run order: %+v", runs)
	}

	failedRuns, err := orchestrator.ListRuns(context.Background(), store, orchestrator.RunFilter{
		Statuses: []orchestrator.RunStatus{orchestrator.RunFailed},
	})
	if err != nil {
		t.Fatalf("ListRuns with filter failed: %v", err)
	}
	if len(failedRuns) != 1 || failedRuns[0].ID != "run-2" {
		t.Fatalf("expected failed run filter to return run-2, got %+v", failedRuns)
	}

	incremental, err := store.ListEvents(context.Background(), orchestrator.EventFilter{
		AfterSequence: 3,
		Limit:         2,
	})
	if err != nil {
		t.Fatalf("incremental ListEvents failed: %v", err)
	}
	if len(incremental) != 2 {
		t.Fatalf("expected 2 incremental events, got %d", len(incremental))
	}
	if incremental[0].Sequence != 4 || incremental[1].Sequence != 5 {
		t.Fatalf("unexpected incremental event sequences: %+v", incremental)
	}

	cursor := orchestrator.ReplayCursor{}
	page1, cursor, err := orchestrator.ReplayEvents(context.Background(), store, orchestrator.EventFilter{
		Limit: 2,
	}, cursor)
	if err != nil {
		t.Fatalf("ReplayEvents page1 failed: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 replay events in page1, got %d", len(page1))
	}
	if cursor.AfterSequence != 2 {
		t.Fatalf("expected replay cursor after page1 to be 2, got %d", cursor.AfterSequence)
	}

	page2, cursor, err := orchestrator.ReplayEvents(context.Background(), store, orchestrator.EventFilter{
		Limit: 3,
	}, cursor)
	if err != nil {
		t.Fatalf("ReplayEvents page2 failed: %v", err)
	}
	if len(page2) != 3 {
		t.Fatalf("expected 3 replay events in page2, got %d", len(page2))
	}
	if cursor.AfterSequence != 5 {
		t.Fatalf("expected replay cursor after page2 to be 5, got %d", cursor.AfterSequence)
	}

	filteredPage, filteredCursor, err := orchestrator.ReplayEvents(context.Background(), store, orchestrator.EventFilter{
		TaskID: "task-2",
		Limit:  1,
	}, orchestrator.ReplayCursor{})
	if err != nil {
		t.Fatalf("ReplayEvents filtered page failed: %v", err)
	}
	if len(filteredPage) != 1 || filteredPage[0].Record.RunID != "run-2" {
		t.Fatalf("unexpected filtered replay page: %+v", filteredPage)
	}
	if filteredCursor.AfterSequence != 6 {
		t.Fatalf("expected filtered replay cursor 6, got %d", filteredCursor.AfterSequence)
	}

	workerTimeline, err := orchestrator.LoadWorkerTimeline(context.Background(), store, "worker-a")
	if err != nil {
		t.Fatalf("LoadWorkerTimeline failed: %v", err)
	}
	if len(workerTimeline.Runs) != 1 || workerTimeline.Runs[0].ID != "run-1" {
		t.Fatalf("unexpected worker timeline runs: %+v", workerTimeline.Runs)
	}
	if len(workerTimeline.Events) != 4 {
		t.Fatalf("expected 4 worker timeline events, got %d", len(workerTimeline.Events))
	}
	if workerTimeline.Latest == nil || workerTimeline.Latest.Kind != orchestrator.EventCommandReleased {
		t.Fatalf("unexpected worker timeline latest event: %+v", workerTimeline.Latest)
	}

	workerSummary, err := orchestrator.GetWorker(context.Background(), store, "worker-a")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if workerSummary.CompletedRuns != 1 || workerSummary.ActiveRuns != 0 {
		t.Fatalf("unexpected worker summary: %+v", workerSummary)
	}
	if workerSummary.LatestRunID != "run-1" || workerSummary.LatestTaskID != "task-1" {
		t.Fatalf("unexpected worker latest attribution: %+v", workerSummary)
	}

	workers, err := orchestrator.ListWorkers(context.Background(), store, orchestrator.WorkerFilter{})
	if err != nil {
		t.Fatalf("ListWorkers failed: %v", err)
	}
	if len(workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(workers))
	}
	if workers[0].ID != "worker-a" || workers[1].ID != "worker-b" {
		t.Fatalf("unexpected worker order: %+v", workers)
	}

	filteredWorkers, err := orchestrator.ListWorkers(context.Background(), store, orchestrator.WorkerFilter{
		IDs: []string{"worker-b"},
	})
	if err != nil {
		t.Fatalf("ListWorkers filtered failed: %v", err)
	}
	if len(filteredWorkers) != 1 || filteredWorkers[0].ID != "worker-b" {
		t.Fatalf("expected filtered workers to return worker-b, got %+v", filteredWorkers)
	}
}

func TestRecoveryHistoryHelpers(t *testing.T) {
	base := time.Unix(20, 0).UTC()
	manualLeaseRelease := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  1,
		ID:        "event-lease-manual",
		Kind:      orchestrator.EventLeaseReleased,
		TaskID:    "task-1",
		RunID:     "run-1",
		LeaseID:   "lease-1",
		CreatedAt: base,
	}, orchestrator.LeaseReleasedEvent{
		TaskID:       "task-1",
		RunID:        "run-1",
		LeaseID:      "lease-1",
		WorkerID:     "worker-a",
		ReleasedAt:   base,
		Requeued:     true,
		ResultStatus: orchestrator.TaskPending,
		Reason:       "lease released",
		Recovered:    false,
	})
	recoveredLeaseRelease := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  2,
		ID:        "event-lease-recovered",
		Kind:      orchestrator.EventLeaseReleased,
		TaskID:    "task-2",
		RunID:     "run-2",
		LeaseID:   "lease-2",
		CreatedAt: base.Add(time.Second),
	}, orchestrator.LeaseReleasedEvent{
		TaskID:       "task-2",
		RunID:        "run-2",
		LeaseID:      "lease-2",
		WorkerID:     "worker-b",
		ReleasedAt:   base.Add(time.Second),
		Requeued:     false,
		ResultStatus: orchestrator.TaskFailed,
		Reason:       "lease expired",
		Recovered:    true,
	})
	manualCommandRelease := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  3,
		ID:        "event-command-manual",
		Kind:      orchestrator.EventCommandReleased,
		TaskID:    "task-1",
		RunID:     "run-1",
		CommandID: "command-1",
		CreatedAt: base.Add(2 * time.Second),
	}, orchestrator.CommandReleasedEvent{
		CommandID:  "command-1",
		Kind:       orchestrator.CommandCancelTask,
		TaskID:     "task-1",
		RunID:      "run-1",
		ReleasedBy: "worker-a",
		ReleasedAt: base.Add(2 * time.Second),
		Reason:     "command released",
		Recovered:  false,
	})
	recoveredCommandRelease := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  4,
		ID:        "event-command-recovered",
		Kind:      orchestrator.EventCommandReleased,
		TaskID:    "task-2",
		RunID:     "run-2",
		CommandID: "command-2",
		CreatedAt: base.Add(3 * time.Second),
	}, orchestrator.CommandReleasedEvent{
		CommandID:  "command-2",
		Kind:       orchestrator.CommandAbortRun,
		TaskID:     "task-2",
		RunID:      "run-2",
		ReleasedBy: "worker-b",
		ReleasedAt: base.Add(3 * time.Second),
		Reason:     "claim expired",
		Recovered:  true,
	})

	store := fakeEventStore{events: []*orchestrator.EventRecord{
		manualLeaseRelease,
		recoveredLeaseRelease,
		manualCommandRelease,
		recoveredCommandRelease,
	}}

	leaseRecoveries, err := orchestrator.ListLeaseRecoveries(context.Background(), store, orchestrator.RecoveryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListLeaseRecoveries failed: %v", err)
	}
	if len(leaseRecoveries) != 1 {
		t.Fatalf("expected 1 recovered lease event, got %d", len(leaseRecoveries))
	}
	if leaseRecoveries[0].LeaseID != "lease-2" || leaseRecoveries[0].ResultStatus != orchestrator.TaskFailed {
		t.Fatalf("unexpected recovered lease summary: %+v", leaseRecoveries[0])
	}

	commandRecoveries, err := orchestrator.ListCommandRecoveries(context.Background(), store, orchestrator.RecoveryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListCommandRecoveries failed: %v", err)
	}
	if len(commandRecoveries) != 1 {
		t.Fatalf("expected 1 recovered command event, got %d", len(commandRecoveries))
	}
	if commandRecoveries[0].CommandID != "command-2" || commandRecoveries[0].Reason != "claim expired" {
		t.Fatalf("unexpected recovered command summary: %+v", commandRecoveries[0])
	}

	filtered, err := orchestrator.ListCommandRecoveries(context.Background(), store, orchestrator.RecoveryHistoryFilter{
		AfterSequence: 3,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("ListCommandRecoveries filtered failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Sequence != 4 {
		t.Fatalf("unexpected filtered recovered commands: %+v", filtered)
	}
}

func mustRecord(t *testing.T, record *orchestrator.EventRecord, payload any) *orchestrator.EventRecord {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	record.Payload = encoded
	return record
}

func cloneRecord(src *orchestrator.EventRecord) *orchestrator.EventRecord {
	if src == nil {
		return nil
	}
	clone := *src
	clone.Payload = append([]byte(nil), src.Payload...)
	return &clone
}

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
		if filter.TaskID != "" && event.TaskID != filter.TaskID {
			continue
		}
		if filter.CommandID != "" && event.CommandID != filter.CommandID {
			continue
		}
		out = append(out, cloneRecord(event))
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
	taskCompleted := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  2,
		ID:        "event-2",
		Kind:      orchestrator.EventTaskCompleted,
		TaskID:    "task-1",
		RunID:     "run-1",
		CreatedAt: base.Add(time.Second),
	}, orchestrator.TaskCompletedEvent{
		TaskID:      "task-1",
		RunID:       "run-1",
		Attempt:     1,
		CompletedAt: base.Add(time.Second),
	})
	commandCreated := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  3,
		ID:        "event-3",
		Kind:      orchestrator.EventCommandCreated,
		TaskID:    "task-1",
		CommandID: "command-1",
		CreatedAt: base.Add(2 * time.Second),
	}, orchestrator.CommandCreatedEvent{
		CommandID: "command-1",
		Kind:      orchestrator.CommandCancelTask,
		TaskID:    "task-1",
		CreatedAt: base.Add(2 * time.Second),
	})
	commandReleased := mustRecord(t, &orchestrator.EventRecord{
		Sequence:  4,
		ID:        "event-4",
		Kind:      orchestrator.EventCommandReleased,
		TaskID:    "task-1",
		CommandID: "command-1",
		CreatedAt: base.Add(3 * time.Second),
	}, orchestrator.CommandReleasedEvent{
		CommandID:  "command-1",
		Kind:       orchestrator.CommandCancelTask,
		TaskID:     "task-1",
		ReleasedBy: "worker-a",
		ReleasedAt: base.Add(3 * time.Second),
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
		taskCompleted,
		commandCreated,
		commandReleased,
	}}

	taskTimeline, err := orchestrator.LoadTaskTimeline(context.Background(), store, "task-1")
	if err != nil {
		t.Fatalf("LoadTaskTimeline failed: %v", err)
	}
	if len(taskTimeline.Events) != 4 {
		t.Fatalf("expected 4 task timeline events, got %d", len(taskTimeline.Events))
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

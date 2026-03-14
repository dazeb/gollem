package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// EventKind identifies a durable orchestration history event.
type EventKind string

const (
	EventTaskCreated     EventKind = "task_created"
	EventTaskUpdated     EventKind = "task_updated"
	EventTaskDeleted     EventKind = "task_deleted"
	EventTaskClaimed     EventKind = "task_claimed"
	EventLeaseRenewed    EventKind = "lease_renewed"
	EventLeaseReleased   EventKind = "lease_released"
	EventTaskRequeued    EventKind = "task_requeued"
	EventTaskCompleted   EventKind = "task_completed"
	EventTaskFailed      EventKind = "task_failed"
	EventTaskCanceled    EventKind = "task_canceled"
	EventArtifactCreated EventKind = "artifact_created"
	EventCommandCreated  EventKind = "command_created"
	EventCommandHandled  EventKind = "command_handled"
)

// EventRecord is a durable history entry for an orchestration transition.
// Payload stores the JSON encoding of one of the concrete event structs in events.go.
type EventRecord struct {
	ID         string
	Kind       EventKind
	TaskID     string
	RunID      string
	LeaseID    string
	CommandID  string
	ArtifactID string
	CreatedAt  time.Time
	Payload    json.RawMessage
}

// DecodePayload unmarshals the record payload into v.
func (e *EventRecord) DecodePayload(v any) error {
	if e == nil {
		return errors.New("orchestrator: nil event record")
	}
	if len(e.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(e.Payload, v); err != nil {
		return fmt.Errorf("unmarshal orchestrator event payload: %w", err)
	}
	return nil
}

// EventFilter narrows durable history queries.
type EventFilter struct {
	Kinds      []EventKind
	TaskID     string
	RunID      string
	LeaseID    string
	CommandID  string
	ArtifactID string
}

// EventStore exposes durable orchestration history.
type EventStore interface {
	GetEvent(ctx context.Context, id string) (*EventRecord, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]*EventRecord, error)
}

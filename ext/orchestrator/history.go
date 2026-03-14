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
	EventCommandClaimed  EventKind = "command_claimed"
	EventCommandReleased EventKind = "command_released"
	EventCommandHandled  EventKind = "command_handled"
)

// EventRecord is a durable history entry for an orchestration transition.
// Payload stores the JSON encoding of one of the concrete event structs in events.go.
type EventRecord struct {
	Sequence   int64
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

// DecodedEvent pairs an event record with its typed payload.
type DecodedEvent struct {
	Record  *EventRecord
	Payload any
}

// TaskTimeline is a decoded event sequence for a single task.
type TaskTimeline struct {
	TaskID   string
	Events   []DecodedEvent
	Latest   *EventRecord
	LatestAt time.Time
}

// CommandTimeline is a decoded event sequence for a single command.
type CommandTimeline struct {
	CommandID string
	Events    []DecodedEvent
	Latest    *EventRecord
	LatestAt  time.Time
}

// DecodeEvent returns the concrete payload struct for a durable event record.
func DecodeEvent(record *EventRecord) (DecodedEvent, error) {
	if record == nil {
		return DecodedEvent{}, errors.New("orchestrator: nil event record")
	}

	var payload any
	switch record.Kind {
	case EventTaskCreated:
		payload = &TaskCreatedEvent{}
	case EventTaskUpdated:
		payload = &TaskUpdatedEvent{}
	case EventTaskDeleted:
		payload = &TaskDeletedEvent{}
	case EventTaskClaimed:
		payload = &TaskClaimedEvent{}
	case EventLeaseRenewed:
		payload = &LeaseRenewedEvent{}
	case EventLeaseReleased:
		payload = &LeaseReleasedEvent{}
	case EventTaskRequeued:
		payload = &TaskRequeuedEvent{}
	case EventTaskCompleted:
		payload = &TaskCompletedEvent{}
	case EventTaskFailed:
		payload = &TaskFailedEvent{}
	case EventTaskCanceled:
		payload = &TaskCanceledEvent{}
	case EventArtifactCreated:
		payload = &ArtifactCreatedEvent{}
	case EventCommandCreated:
		payload = &CommandCreatedEvent{}
	case EventCommandClaimed:
		payload = &CommandClaimedEvent{}
	case EventCommandReleased:
		payload = &CommandReleasedEvent{}
	case EventCommandHandled:
		payload = &CommandHandledEvent{}
	default:
		return DecodedEvent{}, fmt.Errorf("orchestrator: unsupported event kind %q", record.Kind)
	}

	if err := record.DecodePayload(payload); err != nil {
		return DecodedEvent{}, err
	}
	return DecodedEvent{Record: record, Payload: payload}, nil
}

// LoadTaskTimeline decodes the durable history for a single task.
func LoadTaskTimeline(ctx context.Context, store EventStore, taskID string) (*TaskTimeline, error) {
	if taskID == "" {
		return nil, errors.New("orchestrator: task id must not be empty")
	}
	events, err := store.ListEvents(ctx, EventFilter{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	timeline := &TaskTimeline{TaskID: taskID}
	for _, record := range events {
		decoded, err := DecodeEvent(record)
		if err != nil {
			return nil, err
		}
		timeline.Events = append(timeline.Events, decoded)
		timeline.Latest = decoded.Record
		timeline.LatestAt = decoded.Record.CreatedAt
	}
	return timeline, nil
}

// LoadCommandTimeline decodes the durable history for a single command.
func LoadCommandTimeline(ctx context.Context, store EventStore, commandID string) (*CommandTimeline, error) {
	if commandID == "" {
		return nil, errors.New("orchestrator: command id must not be empty")
	}
	events, err := store.ListEvents(ctx, EventFilter{CommandID: commandID})
	if err != nil {
		return nil, err
	}
	timeline := &CommandTimeline{CommandID: commandID}
	for _, record := range events {
		decoded, err := DecodeEvent(record)
		if err != nil {
			return nil, err
		}
		timeline.Events = append(timeline.Events, decoded)
		timeline.Latest = decoded.Record
		timeline.LatestAt = decoded.Record.CreatedAt
	}
	return timeline, nil
}

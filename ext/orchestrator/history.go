package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	Kinds         []EventKind
	TaskID        string
	RunID         string
	LeaseID       string
	CommandID     string
	ArtifactID    string
	AfterSequence int64
	Limit         int
}

// EventStore exposes durable orchestration history.
// ListEvents returns records in append order, oldest first.
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

// ReplayCursor tracks the last durable event consumed by a replay client.
// AfterSequence is exclusive: the next replay page starts after this event.
type ReplayCursor struct {
	AfterSequence int64
}

// WorkerTimeline is a decoded event sequence for all runs owned by a worker.
type WorkerTimeline struct {
	WorkerID string
	Runs     []*RunSummary
	Events   []DecodedEvent
	Latest   *EventRecord
	LatestAt time.Time
}

// RunTimeline is a decoded event sequence for a single run attempt.
type RunTimeline struct {
	RunID       string
	TaskID      string
	WorkerID    string
	Attempt     int
	Events      []DecodedEvent
	Latest      *EventRecord
	LatestAt    time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	Terminal    *EventRecord
}

// RunStatus is the projected status of a single run attempt.
type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCanceled  RunStatus = "canceled"
	RunRequeued  RunStatus = "requeued"
	RunReleased  RunStatus = "released"
)

// RunSummary is a projected summary of a single run attempt.
type RunSummary struct {
	ID           string
	TaskID       string
	WorkerID     string
	Attempt      int
	Status       RunStatus
	StartedAt    time.Time
	CompletedAt  time.Time
	LatestAt     time.Time
	LatestKind   EventKind
	TerminalKind EventKind
}

// RunFilter narrows projected run queries.
type RunFilter struct {
	TaskID   string
	WorkerID string
	Statuses []RunStatus
}

// WorkerSummary is the projected durable summary for a single worker.
type WorkerSummary struct {
	ID             string
	ActiveRuns     int
	CompletedRuns  int
	FailedRuns     int
	CanceledRuns   int
	RequeuedRuns   int
	ReleasedRuns   int
	LatestAt       time.Time
	LatestRunID    string
	LatestTaskID   string
	LatestStatus   RunStatus
	LatestKind     EventKind
	LatestTerminal EventKind
}

// WorkerFilter narrows projected worker queries.
type WorkerFilter struct {
	IDs        []string
	ActiveOnly bool
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

// ReplayEventRecords loads the next append-ordered page of durable event records
// after the supplied cursor and returns the advanced cursor.
func ReplayEventRecords(ctx context.Context, store EventStore, filter EventFilter, cursor ReplayCursor) ([]*EventRecord, ReplayCursor, error) {
	filter.AfterSequence = maxInt64(filter.AfterSequence, cursor.AfterSequence)
	records, err := store.ListEvents(ctx, filter)
	if err != nil {
		return nil, cursor, err
	}
	return records, advanceReplayCursor(cursor, records), nil
}

// ReplayEvents loads and decodes the next append-ordered page of durable events
// after the supplied cursor and returns the advanced cursor.
func ReplayEvents(ctx context.Context, store EventStore, filter EventFilter, cursor ReplayCursor) ([]DecodedEvent, ReplayCursor, error) {
	records, next, err := ReplayEventRecords(ctx, store, filter, cursor)
	if err != nil {
		return nil, cursor, err
	}
	decoded := make([]DecodedEvent, 0, len(records))
	for _, record := range records {
		event, err := DecodeEvent(record)
		if err != nil {
			return nil, cursor, err
		}
		decoded = append(decoded, event)
	}
	return decoded, next, nil
}

// LoadWorkerTimeline decodes the durable history for all runs owned by a worker.
func LoadWorkerTimeline(ctx context.Context, store EventStore, workerID string) (*WorkerTimeline, error) {
	if workerID == "" {
		return nil, errors.New("orchestrator: worker id must not be empty")
	}
	runs, err := ListRuns(ctx, store, RunFilter{WorkerID: workerID})
	if err != nil {
		return nil, err
	}
	timeline := &WorkerTimeline{
		WorkerID: workerID,
		Runs:     runs,
	}
	for _, run := range runs {
		events, err := store.ListEvents(ctx, EventFilter{RunID: run.ID})
		if err != nil {
			return nil, err
		}
		for _, record := range events {
			decoded, err := DecodeEvent(record)
			if err != nil {
				return nil, err
			}
			timeline.Events = append(timeline.Events, decoded)
		}
	}
	sort.Slice(timeline.Events, func(i, j int) bool {
		left := timeline.Events[i].Record
		right := timeline.Events[j].Record
		if left == nil || right == nil {
			return i < j
		}
		if left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})
	if n := len(timeline.Events); n > 0 {
		timeline.Latest = timeline.Events[n-1].Record
		if timeline.Latest != nil {
			timeline.LatestAt = timeline.Latest.CreatedAt
		}
	}
	return timeline, nil
}

// LoadRunTimeline decodes the durable history for a single run attempt.
func LoadRunTimeline(ctx context.Context, store EventStore, runID string) (*RunTimeline, error) {
	if runID == "" {
		return nil, errors.New("orchestrator: run id must not be empty")
	}
	events, err := store.ListEvents(ctx, EventFilter{RunID: runID})
	if err != nil {
		return nil, err
	}
	timeline, err := buildRunTimeline(runID, events)
	if err != nil {
		return nil, err
	}
	return timeline, nil
}

// GetRun projects the latest durable summary for a single run attempt.
func GetRun(ctx context.Context, store EventStore, runID string) (*RunSummary, error) {
	timeline, err := LoadRunTimeline(ctx, store, runID)
	if err != nil {
		return nil, err
	}
	if len(timeline.Events) == 0 {
		return nil, ErrRunNotFound
	}
	return summarizeRunTimeline(timeline), nil
}

// ListRuns projects durable run summaries from the event store.
func ListRuns(ctx context.Context, store EventStore, filter RunFilter) ([]*RunSummary, error) {
	events, err := store.ListEvents(ctx, EventFilter{})
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]*EventRecord)
	var order []string
	for _, record := range events {
		if record == nil || record.RunID == "" {
			continue
		}
		if _, ok := grouped[record.RunID]; !ok {
			order = append(order, record.RunID)
		}
		grouped[record.RunID] = append(grouped[record.RunID], record)
	}

	var runs []*RunSummary
	for _, runID := range order {
		timeline, err := buildRunTimeline(runID, grouped[runID])
		if err != nil {
			return nil, err
		}
		summary := summarizeRunTimeline(timeline)
		if matchesRunFilter(summary, filter) {
			runs = append(runs, summary)
		}
	}
	return runs, nil
}

// GetWorker projects the latest durable summary for a single worker.
func GetWorker(ctx context.Context, store EventStore, workerID string) (*WorkerSummary, error) {
	if workerID == "" {
		return nil, errors.New("orchestrator: worker id must not be empty")
	}
	runs, err := ListRuns(ctx, store, RunFilter{WorkerID: workerID})
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, ErrRunNotFound
	}
	return summarizeWorkerRuns(workerID, runs), nil
}

// ListWorkers projects durable worker summaries from the event store.
func ListWorkers(ctx context.Context, store EventStore, filter WorkerFilter) ([]*WorkerSummary, error) {
	runs, err := ListRuns(ctx, store, RunFilter{})
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]*RunSummary)
	var order []string
	for _, run := range runs {
		if run == nil || run.WorkerID == "" {
			continue
		}
		if _, ok := grouped[run.WorkerID]; !ok {
			order = append(order, run.WorkerID)
		}
		grouped[run.WorkerID] = append(grouped[run.WorkerID], run)
	}

	workers := make([]*WorkerSummary, 0, len(order))
	for _, workerID := range order {
		summary := summarizeWorkerRuns(workerID, grouped[workerID])
		if matchesWorkerFilter(summary, filter) {
			workers = append(workers, summary)
		}
	}
	return workers, nil
}

func buildRunTimeline(runID string, events []*EventRecord) (*RunTimeline, error) {
	timeline := &RunTimeline{RunID: runID}
	for _, record := range events {
		decoded, err := DecodeEvent(record)
		if err != nil {
			return nil, err
		}
		timeline.Events = append(timeline.Events, decoded)
		timeline.Latest = decoded.Record
		timeline.LatestAt = decoded.Record.CreatedAt
		if timeline.TaskID == "" && decoded.Record.TaskID != "" {
			timeline.TaskID = decoded.Record.TaskID
		}
		applyRunProjection(timeline, decoded)
	}
	return timeline, nil
}

func summarizeRunTimeline(timeline *RunTimeline) *RunSummary {
	if timeline == nil {
		return nil
	}

	summary := &RunSummary{
		ID:          timeline.RunID,
		TaskID:      timeline.TaskID,
		WorkerID:    timeline.WorkerID,
		Attempt:     timeline.Attempt,
		StartedAt:   timeline.StartedAt,
		CompletedAt: timeline.CompletedAt,
		LatestAt:    timeline.LatestAt,
		Status:      RunRunning,
	}
	if timeline.Latest != nil {
		summary.LatestKind = timeline.Latest.Kind
	}
	if timeline.Terminal != nil {
		summary.TerminalKind = timeline.Terminal.Kind
		summary.Status = runStatusFromEvent(timeline.Terminal.Kind)
	}
	return summary
}

func summarizeWorkerRuns(workerID string, runs []*RunSummary) *WorkerSummary {
	summary := &WorkerSummary{ID: workerID}
	for _, run := range runs {
		if run == nil {
			continue
		}
		switch run.Status {
		case RunRunning:
			summary.ActiveRuns++
		case RunCompleted:
			summary.CompletedRuns++
		case RunFailed:
			summary.FailedRuns++
		case RunCanceled:
			summary.CanceledRuns++
		case RunRequeued:
			summary.RequeuedRuns++
		case RunReleased:
			summary.ReleasedRuns++
		}
		if summary.LatestAt.IsZero() || run.LatestAt.After(summary.LatestAt) {
			summary.LatestAt = run.LatestAt
			summary.LatestRunID = run.ID
			summary.LatestTaskID = run.TaskID
			summary.LatestStatus = run.Status
			summary.LatestKind = run.LatestKind
			summary.LatestTerminal = run.TerminalKind
		}
	}
	return summary
}

func matchesRunFilter(summary *RunSummary, filter RunFilter) bool {
	if summary == nil {
		return false
	}
	if filter.TaskID != "" && summary.TaskID != filter.TaskID {
		return false
	}
	if filter.WorkerID != "" && summary.WorkerID != filter.WorkerID {
		return false
	}
	if len(filter.Statuses) == 0 {
		return true
	}
	for _, status := range filter.Statuses {
		if summary.Status == status {
			return true
		}
	}
	return false
}

func matchesWorkerFilter(summary *WorkerSummary, filter WorkerFilter) bool {
	if summary == nil {
		return false
	}
	if len(filter.IDs) > 0 {
		matched := false
		for _, id := range filter.IDs {
			if summary.ID == id {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if filter.ActiveOnly && summary.ActiveRuns == 0 {
		return false
	}
	return true
}

func runStatusFromEvent(kind EventKind) RunStatus {
	switch kind {
	case EventTaskCompleted:
		return RunCompleted
	case EventTaskFailed:
		return RunFailed
	case EventTaskCanceled:
		return RunCanceled
	case EventTaskRequeued:
		return RunRequeued
	case EventLeaseReleased:
		return RunReleased
	default:
		return RunRunning
	}
}

func applyRunProjection(timeline *RunTimeline, decoded DecodedEvent) {
	if timeline == nil || decoded.Record == nil {
		return
	}

	switch payload := decoded.Payload.(type) {
	case *TaskClaimedEvent:
		if timeline.WorkerID == "" {
			timeline.WorkerID = payload.WorkerID
		}
		if payload.Attempt > 0 {
			timeline.Attempt = payload.Attempt
		}
		if timeline.StartedAt.IsZero() || payload.AcquiredAt.Before(timeline.StartedAt) {
			timeline.StartedAt = payload.AcquiredAt
		}
	case *LeaseRenewedEvent:
		if timeline.WorkerID == "" {
			timeline.WorkerID = payload.WorkerID
		}
		if timeline.StartedAt.IsZero() {
			timeline.StartedAt = decoded.Record.CreatedAt
		}
	case *LeaseReleasedEvent:
		if timeline.WorkerID == "" {
			timeline.WorkerID = payload.WorkerID
		}
		timeline.CompletedAt = payload.ReleasedAt
		timeline.Terminal = decoded.Record
	case *TaskRequeuedEvent:
		if payload.LastAttempt > 0 {
			timeline.Attempt = payload.LastAttempt
		}
		timeline.CompletedAt = payload.RequeuedAt
		timeline.Terminal = decoded.Record
	case *TaskCompletedEvent:
		if payload.Attempt > 0 {
			timeline.Attempt = payload.Attempt
		}
		timeline.CompletedAt = payload.CompletedAt
		timeline.Terminal = decoded.Record
	case *TaskFailedEvent:
		if payload.Attempt > 0 {
			timeline.Attempt = payload.Attempt
		}
		timeline.CompletedAt = payload.FailedAt
		timeline.Terminal = decoded.Record
	case *TaskCanceledEvent:
		if payload.Attempt > 0 {
			timeline.Attempt = payload.Attempt
		}
		timeline.CompletedAt = payload.CanceledAt
		timeline.Terminal = decoded.Record
	}
}

func advanceReplayCursor(cursor ReplayCursor, records []*EventRecord) ReplayCursor {
	for _, record := range records {
		if record == nil {
			continue
		}
		cursor.AfterSequence = maxInt64(cursor.AfterSequence, record.Sequence)
	}
	return cursor
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

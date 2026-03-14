package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
)

func (s *Store) GetEvent(ctx context.Context, id string) (*orchestrator.EventRecord, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	event, err := s.loadEvent(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return cloneEventRecord(event), nil
}

func (s *Store) ListEvents(ctx context.Context, filter orchestrator.EventFilter) ([]*orchestrator.EventRecord, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	events, err := s.listEvents(ctx, s.db)
	if err != nil {
		return nil, err
	}
	var out []*orchestrator.EventRecord
	for _, event := range events {
		if matchesEventFilter(event, filter) {
			out = append(out, cloneEventRecord(event))
		}
	}
	return out, nil
}

func (s *Store) saveEventTx(ctx context.Context, tx *sql.Tx, event *orchestrator.EventRecord) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event %q: %w", event.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO events (id, created_at, kind, task_id, run_id, lease_id, command_id, artifact_id, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			created_at = excluded.created_at,
			kind = excluded.kind,
			task_id = excluded.task_id,
			run_id = excluded.run_id,
			lease_id = excluded.lease_id,
			command_id = excluded.command_id,
			artifact_id = excluded.artifact_id,
			payload = excluded.payload
	`, event.ID, formatTime(event.CreatedAt), string(event.Kind), event.TaskID, event.RunID, event.LeaseID, event.CommandID, event.ArtifactID, payload)
	if err != nil {
		return fmt.Errorf("save event %q: %w", event.ID, err)
	}
	return nil
}

func (s *Store) saveEventsTx(ctx context.Context, tx *sql.Tx, events ...*orchestrator.EventRecord) error {
	for _, event := range events {
		if event == nil {
			continue
		}
		if err := s.saveEventTx(ctx, tx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadEvent(ctx context.Context, db queryer, id string) (*orchestrator.EventRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT payload FROM events WHERE id = ?`, id)
	event, err := scanEventRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrEventNotFound
		}
		return nil, err
	}
	return event, nil
}

func (s *Store) listEvents(ctx context.Context, db queryer) ([]*orchestrator.EventRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM events ORDER BY rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []*orchestrator.EventRecord
	for rows.Next() {
		event, err := scanEventRecord(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func scanEventRecord(row interface{ Scan(dest ...any) error }) (*orchestrator.EventRecord, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	var event orchestrator.EventRecord
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event payload: %w", err)
	}
	return &event, nil
}

func newEventRecord(kind orchestrator.EventKind, createdAt time.Time, payload any, taskID, runID, leaseID, commandID, artifactID string) (*orchestrator.EventRecord, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal orchestrator history payload: %w", err)
	}
	return &orchestrator.EventRecord{
		ID:         newID("event"),
		Kind:       kind,
		TaskID:     taskID,
		RunID:      runID,
		LeaseID:    leaseID,
		CommandID:  commandID,
		ArtifactID: artifactID,
		CreatedAt:  normalizeNow(createdAt),
		Payload:    encoded,
	}, nil
}

func taskCreatedRecord(task *orchestrator.Task) (*orchestrator.EventRecord, error) {
	if task == nil {
		return nil, nil
	}
	payload := orchestrator.TaskCreatedEvent{
		TaskID:      task.ID,
		Kind:        task.Kind,
		Subject:     task.Subject,
		Description: task.Description,
		CreatedAt:   task.CreatedAt,
	}
	return newEventRecord(orchestrator.EventTaskCreated, payload.CreatedAt, payload, task.ID, "", "", "", "")
}

func taskUpdatedRecord(task *orchestrator.Task) (*orchestrator.EventRecord, error) {
	if task == nil {
		return nil, nil
	}
	payload := orchestrator.TaskUpdatedEvent{
		TaskID:    task.ID,
		Subject:   task.Subject,
		Blocks:    cloneStrings(task.Blocks),
		BlockedBy: cloneStrings(task.BlockedBy),
		UpdatedAt: task.UpdatedAt,
	}
	return newEventRecord(orchestrator.EventTaskUpdated, payload.UpdatedAt, payload, task.ID, "", "", "", "")
}

func taskDeletedRecord(taskID string, deletedAt time.Time) (*orchestrator.EventRecord, error) {
	payload := orchestrator.TaskDeletedEvent{
		TaskID:    taskID,
		DeletedAt: deletedAt,
	}
	return newEventRecord(orchestrator.EventTaskDeleted, payload.DeletedAt, payload, taskID, "", "", "", "")
}

func taskClaimedRecord(task *orchestrator.Task, lease *orchestrator.Lease) (*orchestrator.EventRecord, error) {
	if task == nil || lease == nil {
		return nil, nil
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	payload := orchestrator.TaskClaimedEvent{
		TaskID:     task.ID,
		RunID:      runID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		Attempt:    task.Attempt,
		AcquiredAt: lease.AcquiredAt,
		ExpiresAt:  lease.ExpiresAt,
	}
	return newEventRecord(orchestrator.EventTaskClaimed, payload.AcquiredAt, payload, task.ID, runID, lease.ID, "", "")
}

func leaseRenewedRecord(lease *orchestrator.Lease) (*orchestrator.EventRecord, error) {
	if lease == nil {
		return nil, nil
	}
	payload := orchestrator.LeaseRenewedEvent{
		TaskID:    lease.TaskID,
		LeaseID:   lease.ID,
		WorkerID:  lease.WorkerID,
		ExpiresAt: lease.ExpiresAt,
	}
	return newEventRecord(orchestrator.EventLeaseRenewed, payload.ExpiresAt, payload, lease.TaskID, "", lease.ID, "", "")
}

func leaseReleasedRecord(lease *orchestrator.Lease, requeued bool, releasedAt time.Time) (*orchestrator.EventRecord, error) {
	if lease == nil {
		return nil, nil
	}
	payload := orchestrator.LeaseReleasedEvent{
		TaskID:     lease.TaskID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		ReleasedAt: releasedAt,
		Requeued:   requeued,
	}
	return newEventRecord(orchestrator.EventLeaseReleased, payload.ReleasedAt, payload, lease.TaskID, "", lease.ID, "", "")
}

func taskRequeuedRecord(task *orchestrator.Task, lastRunID string, lastAttempt int, reason string) (*orchestrator.EventRecord, error) {
	if task == nil {
		return nil, nil
	}
	payload := orchestrator.TaskRequeuedEvent{
		TaskID:      task.ID,
		LastRunID:   lastRunID,
		LastAttempt: lastAttempt,
		Reason:      reason,
		RequeuedAt:  task.UpdatedAt,
	}
	return newEventRecord(orchestrator.EventTaskRequeued, payload.RequeuedAt, payload, task.ID, lastRunID, "", "", "")
}

func taskCompletedRecord(task *orchestrator.Task) (*orchestrator.EventRecord, error) {
	if task == nil {
		return nil, nil
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	payload := orchestrator.TaskCompletedEvent{
		TaskID:      task.ID,
		RunID:       runID,
		Attempt:     task.Attempt,
		CompletedAt: task.CompletedAt,
	}
	return newEventRecord(orchestrator.EventTaskCompleted, payload.CompletedAt, payload, task.ID, runID, "", "", "")
}

func taskFailedRecord(task *orchestrator.Task) (*orchestrator.EventRecord, error) {
	if task == nil {
		return nil, nil
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	payload := orchestrator.TaskFailedEvent{
		TaskID:   task.ID,
		RunID:    runID,
		Attempt:  task.Attempt,
		Error:    task.LastError,
		FailedAt: task.CompletedAt,
	}
	return newEventRecord(orchestrator.EventTaskFailed, payload.FailedAt, payload, task.ID, runID, "", "", "")
}

func taskCanceledRecord(task *orchestrator.Task) (*orchestrator.EventRecord, error) {
	if task == nil {
		return nil, nil
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	payload := orchestrator.TaskCanceledEvent{
		TaskID:     task.ID,
		RunID:      runID,
		Attempt:    task.Attempt,
		Reason:     task.LastError,
		CanceledAt: task.CompletedAt,
	}
	return newEventRecord(orchestrator.EventTaskCanceled, payload.CanceledAt, payload, task.ID, runID, "", "", "")
}

func artifactCreatedRecord(artifact *orchestrator.Artifact) (*orchestrator.EventRecord, error) {
	if artifact == nil {
		return nil, nil
	}
	payload := orchestrator.ArtifactCreatedEvent{
		ArtifactID:  artifact.ID,
		TaskID:      artifact.TaskID,
		RunID:       artifact.RunID,
		Kind:        artifact.Kind,
		Name:        artifact.Name,
		ContentType: artifact.ContentType,
		SizeBytes:   len(artifact.Body),
		CreatedAt:   artifact.CreatedAt,
	}
	return newEventRecord(orchestrator.EventArtifactCreated, payload.CreatedAt, payload, artifact.TaskID, artifact.RunID, "", "", artifact.ID)
}

func commandCreatedRecord(command *orchestrator.Command) (*orchestrator.EventRecord, error) {
	if command == nil {
		return nil, nil
	}
	payload := orchestrator.CommandCreatedEvent{
		CommandID:      command.ID,
		Kind:           command.Kind,
		TaskID:         command.TaskID,
		RunID:          command.RunID,
		TargetWorkerID: command.TargetWorkerID,
		CreatedAt:      command.CreatedAt,
	}
	return newEventRecord(orchestrator.EventCommandCreated, payload.CreatedAt, payload, command.TaskID, command.RunID, "", command.ID, "")
}

func commandHandledRecord(command *orchestrator.Command) (*orchestrator.EventRecord, error) {
	if command == nil {
		return nil, nil
	}
	payload := orchestrator.CommandHandledEvent{
		CommandID: command.ID,
		Kind:      command.Kind,
		TaskID:    command.TaskID,
		RunID:     command.RunID,
		HandledBy: command.HandledBy,
		HandledAt: command.HandledAt,
	}
	return newEventRecord(orchestrator.EventCommandHandled, payload.HandledAt, payload, command.TaskID, command.RunID, "", command.ID, "")
}

func matchesEventFilter(event *orchestrator.EventRecord, filter orchestrator.EventFilter) bool {
	if event == nil {
		return false
	}
	if filter.TaskID != "" && event.TaskID != filter.TaskID {
		return false
	}
	if filter.RunID != "" && event.RunID != filter.RunID {
		return false
	}
	if filter.LeaseID != "" && event.LeaseID != filter.LeaseID {
		return false
	}
	if filter.CommandID != "" && event.CommandID != filter.CommandID {
		return false
	}
	if filter.ArtifactID != "" && event.ArtifactID != filter.ArtifactID {
		return false
	}
	if len(filter.Kinds) == 0 {
		return true
	}
	for _, kind := range filter.Kinds {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func cloneEventRecord(src *orchestrator.EventRecord) *orchestrator.EventRecord {
	if src == nil {
		return nil
	}
	return &orchestrator.EventRecord{
		ID:         src.ID,
		Kind:       src.Kind,
		TaskID:     src.TaskID,
		RunID:      src.RunID,
		LeaseID:    src.LeaseID,
		CommandID:  src.CommandID,
		ArtifactID: src.ArtifactID,
		CreatedAt:  src.CreatedAt,
		Payload:    cloneBytes(src.Payload),
	}
}

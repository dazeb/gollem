package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

	events, err := s.listEvents(ctx, s.db, filter)
	if err != nil {
		return nil, err
	}
	return cloneEventRecords(events), nil
}

func (s *Store) ensureEventSchema(ctx context.Context) error {
	columns, err := s.eventTableColumns(ctx)
	if err != nil {
		return err
	}
	if len(columns) == 0 {
		return s.createEventSchema(ctx)
	}
	for _, column := range columns {
		if column == "sequence" {
			return s.createEventIndexes(ctx)
		}
	}
	return s.migrateLegacyEventsTable(ctx)
}

func (s *Store) eventTableColumns(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(events)`)
	if err != nil {
		return nil, fmt.Errorf("inspect events schema: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return nil, fmt.Errorf("scan events schema: %w", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events schema: %w", err)
	}
	return columns, nil
}

func (s *Store) createEventSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			task_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			lease_id TEXT NOT NULL,
			command_id TEXT NOT NULL,
			artifact_id TEXT NOT NULL,
			payload BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_command_id ON events(command_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_artifact_id ON events(artifact_id, sequence)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize event history schema: %w", err)
		}
	}
	return nil
}

func (s *Store) createEventIndexes(ctx context.Context) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_command_id ON events(command_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_events_artifact_id ON events(artifact_id, sequence)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create event history indexes: %w", err)
		}
	}
	return nil
}

func (s *Store) migrateLegacyEventsTable(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin events migration: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, indexName := range []string{
		"idx_events_created_at",
		"idx_events_task_id",
		"idx_events_run_id",
		"idx_events_command_id",
		"idx_events_artifact_id",
	} {
		if _, err = tx.ExecContext(ctx, `DROP INDEX IF EXISTS `+indexName); err != nil {
			return fmt.Errorf("drop legacy event index %s: %w", indexName, err)
		}
	}
	if _, err = tx.ExecContext(ctx, `ALTER TABLE events RENAME TO events_legacy`); err != nil {
		return fmt.Errorf("rename legacy events table: %w", err)
	}
	if err = createEventSchemaTx(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, created_at, kind, task_id, run_id, lease_id, command_id, artifact_id, payload
		FROM events_legacy
		ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("read legacy events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id         string
			createdAt  string
			kind       string
			taskID     string
			runID      string
			leaseID    string
			commandID  string
			artifactID string
			payload    []byte
		)
		if err = rows.Scan(&id, &createdAt, &kind, &taskID, &runID, &leaseID, &commandID, &artifactID, &payload); err != nil {
			return fmt.Errorf("scan legacy event: %w", err)
		}
		record := &orchestrator.EventRecord{
			ID:         id,
			Kind:       orchestrator.EventKind(kind),
			TaskID:     taskID,
			RunID:      runID,
			LeaseID:    leaseID,
			CommandID:  commandID,
			ArtifactID: artifactID,
			CreatedAt:  parseLegacyEventTime(createdAt),
		}
		var legacy orchestrator.EventRecord
		if err := json.Unmarshal(payload, &legacy); err == nil && legacy.Kind != "" {
			record.Kind = legacy.Kind
			record.TaskID = coalesceString(record.TaskID, legacy.TaskID)
			record.RunID = coalesceString(record.RunID, legacy.RunID)
			record.LeaseID = coalesceString(record.LeaseID, legacy.LeaseID)
			record.CommandID = coalesceString(record.CommandID, legacy.CommandID)
			record.ArtifactID = coalesceString(record.ArtifactID, legacy.ArtifactID)
			if !legacy.CreatedAt.IsZero() {
				record.CreatedAt = legacy.CreatedAt
			}
			record.Payload = cloneBytes(legacy.Payload)
		} else {
			record.Payload = cloneBytes(payload)
		}
		if err := s.saveEventTx(ctx, tx, record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy events: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE events_legacy`); err != nil {
		return fmt.Errorf("drop legacy events table: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit events migration: %w", err)
	}
	return nil
}

func createEventSchemaTx(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			task_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			lease_id TEXT NOT NULL,
			command_id TEXT NOT NULL,
			artifact_id TEXT NOT NULL,
			payload BLOB NOT NULL
		)`,
		`CREATE INDEX idx_events_created_at ON events(created_at, sequence)`,
		`CREATE INDEX idx_events_task_id ON events(task_id, sequence)`,
		`CREATE INDEX idx_events_run_id ON events(run_id, sequence)`,
		`CREATE INDEX idx_events_command_id ON events(command_id, sequence)`,
		`CREATE INDEX idx_events_artifact_id ON events(artifact_id, sequence)`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create migrated event schema: %w", err)
		}
	}
	return nil
}

func (s *Store) saveEventTx(ctx context.Context, tx *sql.Tx, event *orchestrator.EventRecord) error {
	if event == nil {
		return nil
	}
	event.CreatedAt = normalizeNow(event.CreatedAt)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO events (id, created_at, kind, task_id, run_id, lease_id, command_id, artifact_id, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, formatTime(event.CreatedAt), string(event.Kind), event.TaskID, event.RunID, event.LeaseID, event.CommandID, event.ArtifactID, []byte("{}"))
	if err != nil {
		return fmt.Errorf("insert event %q: %w", event.ID, err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("fetch event sequence for %q: %w", event.ID, err)
	}
	event.Sequence = sequence
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event %q: %w", event.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE events SET payload = ? WHERE sequence = ?`, payload, sequence); err != nil {
		return fmt.Errorf("update event payload %q: %w", event.ID, err)
	}
	return nil
}

func (s *Store) saveEventsTx(ctx context.Context, tx *sql.Tx, events ...*orchestrator.EventRecord) error {
	for _, event := range events {
		if err := s.saveEventTx(ctx, tx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadEvent(ctx context.Context, db queryer, id string) (*orchestrator.EventRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT sequence, payload FROM events WHERE id = ?`, id)
	event, err := scanEventRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrEventNotFound
		}
		return nil, err
	}
	return event, nil
}

func (s *Store) listEvents(ctx context.Context, db queryer, filter orchestrator.EventFilter) ([]*orchestrator.EventRecord, error) {
	query := `SELECT sequence, payload FROM events`
	var (
		where []string
		args  []any
	)
	if filter.AfterSequence > 0 {
		where = append(where, `sequence > ?`)
		args = append(args, filter.AfterSequence)
	}
	if filter.TaskID != "" {
		where = append(where, `task_id = ?`)
		args = append(args, filter.TaskID)
	}
	if filter.RunID != "" {
		where = append(where, `run_id = ?`)
		args = append(args, filter.RunID)
	}
	if filter.LeaseID != "" {
		where = append(where, `lease_id = ?`)
		args = append(args, filter.LeaseID)
	}
	if filter.CommandID != "" {
		where = append(where, `command_id = ?`)
		args = append(args, filter.CommandID)
	}
	if filter.ArtifactID != "" {
		where = append(where, `artifact_id = ?`)
		args = append(args, filter.ArtifactID)
	}
	if len(filter.Kinds) > 0 {
		where = append(where, fmt.Sprintf(`kind IN (%s)`, placeholders(len(filter.Kinds))))
		for _, kind := range filter.Kinds {
			args = append(args, string(kind))
		}
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY sequence ASC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := db.QueryContext(ctx, query, args...)
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

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range n {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func scanEventRecord(row interface{ Scan(dest ...any) error }) (*orchestrator.EventRecord, error) {
	var (
		sequence int64
		payload  []byte
	)
	if err := row.Scan(&sequence, &payload); err != nil {
		return nil, err
	}
	var event orchestrator.EventRecord
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event payload: %w", err)
	}
	if event.Sequence == 0 {
		event.Sequence = sequence
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

func leaseRenewedRecord(lease *orchestrator.Lease, runID string) (*orchestrator.EventRecord, error) {
	if lease == nil {
		return nil, nil
	}
	payload := orchestrator.LeaseRenewedEvent{
		TaskID:    lease.TaskID,
		RunID:     runID,
		LeaseID:   lease.ID,
		WorkerID:  lease.WorkerID,
		ExpiresAt: lease.ExpiresAt,
	}
	return newEventRecord(orchestrator.EventLeaseRenewed, payload.ExpiresAt, payload, lease.TaskID, runID, lease.ID, "", "")
}

func leaseReleasedRecord(lease *orchestrator.Lease, runID string, requeued bool, releasedAt time.Time) (*orchestrator.EventRecord, error) {
	if lease == nil {
		return nil, nil
	}
	payload := orchestrator.LeaseReleasedEvent{
		TaskID:     lease.TaskID,
		RunID:      runID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		ReleasedAt: releasedAt,
		Requeued:   requeued,
	}
	return newEventRecord(orchestrator.EventLeaseReleased, payload.ReleasedAt, payload, lease.TaskID, runID, lease.ID, "", "")
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

func commandClaimedRecord(command *orchestrator.Command) (*orchestrator.EventRecord, error) {
	if command == nil {
		return nil, nil
	}
	payload := orchestrator.CommandClaimedEvent{
		CommandID: command.ID,
		Kind:      command.Kind,
		TaskID:    command.TaskID,
		RunID:     command.RunID,
		ClaimedBy: command.ClaimedBy,
		ClaimedAt: command.ClaimedAt,
	}
	return newEventRecord(orchestrator.EventCommandClaimed, payload.ClaimedAt, payload, command.TaskID, command.RunID, "", command.ID, "")
}

func commandReleasedRecord(command *orchestrator.Command, releasedBy string, releasedAt time.Time) (*orchestrator.EventRecord, error) {
	if command == nil {
		return nil, nil
	}
	payload := orchestrator.CommandReleasedEvent{
		CommandID:  command.ID,
		Kind:       command.Kind,
		TaskID:     command.TaskID,
		RunID:      command.RunID,
		ReleasedBy: releasedBy,
		ReleasedAt: releasedAt,
	}
	return newEventRecord(orchestrator.EventCommandReleased, payload.ReleasedAt, payload, command.TaskID, command.RunID, "", command.ID, "")
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

func cloneEventRecords(src []*orchestrator.EventRecord) []*orchestrator.EventRecord {
	if len(src) == 0 {
		return nil
	}
	out := make([]*orchestrator.EventRecord, 0, len(src))
	for _, record := range src {
		out = append(out, cloneEventRecord(record))
	}
	return out
}

func cloneEventRecord(src *orchestrator.EventRecord) *orchestrator.EventRecord {
	if src == nil {
		return nil
	}
	return &orchestrator.EventRecord{
		Sequence:   src.Sequence,
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

func parseLegacyEventTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(timeFormat, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func coalesceString(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

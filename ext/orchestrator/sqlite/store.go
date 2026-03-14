package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

const timeFormat = time.RFC3339Nano

// Store is a persistent orchestrator store backed by SQLite.
type Store struct {
	db       *sql.DB
	mu       sync.Mutex
	eventBus *core.EventBus
}

var (
	_ orchestrator.TaskStore     = (*Store)(nil)
	_ orchestrator.LeaseStore    = (*Store)(nil)
	_ orchestrator.CommandStore  = (*Store)(nil)
	_ orchestrator.ArtifactStore = (*Store)(nil)
)

// Option configures a SQLite store.
type Option func(*Store)

// WithEventBus publishes concrete orchestrator lifecycle events to the supplied bus.
func WithEventBus(bus *core.EventBus) Option {
	return func(s *Store) {
		s.eventBus = bus
	}
}

// NewStore opens or creates a persistent orchestrator store backed by SQLite.
func NewStore(dbPath string, opts ...Option) (*Store, error) {
	if dbPath == "" {
		return nil, errors.New("gollem/orchestrator/sqlite: db path must not be empty")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	for _, opt := range opts {
		opt(store)
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CreateTask implements orchestrator.TaskStore.
func (s *Store) CreateTask(ctx context.Context, req orchestrator.CreateTaskRequest) (*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		task        *orchestrator.Task
		peerUpdates []*orchestrator.Task
	)
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := s.validateTaskDependenciesTx(ctx, tx, req.Blocks, req.BlockedBy); err != nil {
			return err
		}

		now := time.Now().UTC()
		task = &orchestrator.Task{
			ID:          newID("task"),
			Kind:        req.Kind,
			Subject:     req.Subject,
			Description: req.Description,
			Input:       req.Input,
			Status:      orchestrator.TaskPending,
			Blocks:      cloneStrings(req.Blocks),
			BlockedBy:   cloneStrings(req.BlockedBy),
			MaxAttempts: req.MaxAttempts,
			Metadata:    cloneAnyMap(req.Metadata),
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		var err error
		peerUpdates, err = s.linkTaskDependenciesTx(ctx, tx, task, now)
		if err != nil {
			return err
		}
		return s.saveTaskTx(ctx, tx, task)
	}); err != nil {
		return nil, err
	}

	s.publishTaskCreated(task)
	s.publishTaskUpdates(peerUpdates...)
	return cloneTask(task), nil
}

// GetTask implements orchestrator.TaskStore.
func (s *Store) GetTask(ctx context.Context, id string) (*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	task, err := s.loadTask(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

// ListTasks implements orchestrator.TaskStore.
func (s *Store) ListTasks(ctx context.Context, filter orchestrator.TaskFilter) ([]*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.listTasks(ctx, s.db)
	if err != nil {
		return nil, err
	}
	var out []*orchestrator.Task
	for _, task := range tasks {
		if matchesTaskFilter(task, filter) {
			out = append(out, cloneTask(task))
		}
	}
	return out, nil
}

// ClaimReadyTask implements orchestrator.TaskStore.
func (s *Store) ClaimReadyTask(ctx context.Context, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	if req.LeaseTTL <= 0 {
		return nil, errors.New("gollem/orchestrator/sqlite: lease ttl must be positive")
	}
	ctx = normalizeContext(ctx)
	now := normalizeNow(req.Now)

	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		claim       *orchestrator.ClaimedTask
		failedTasks []*orchestrator.Task
	)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		ids, err := s.listTaskIDsTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			var exhausted *orchestrator.Task
			claim, exhausted, err = s.claimTaskTx(ctx, tx, id, req, now)
			if exhausted != nil {
				failedTasks = append(failedTasks, exhausted)
			}
			if err == nil {
				return nil
			}
			if errors.Is(err, orchestrator.ErrNoReadyTask) || errors.Is(err, orchestrator.ErrTaskBlocked) {
				continue
			}
			return err
		}
		return orchestrator.ErrNoReadyTask
	})
	for _, task := range failedTasks {
		s.publishTaskFailed(task)
	}
	if err != nil {
		return nil, err
	}
	if claim != nil {
		s.publishTaskClaimed(claim.Task, claim.Lease)
	}
	return cloneClaimedTask(claim), nil
}

// ClaimTask implements orchestrator.TaskStore.
func (s *Store) ClaimTask(ctx context.Context, taskID string, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	if req.LeaseTTL <= 0 {
		return nil, errors.New("gollem/orchestrator/sqlite: lease ttl must be positive")
	}
	ctx = normalizeContext(ctx)
	now := normalizeNow(req.Now)

	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		claim     *orchestrator.ClaimedTask
		exhausted *orchestrator.Task
	)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		claim, exhausted, err = s.claimTaskTx(ctx, tx, taskID, req, now)
		return err
	})
	if exhausted != nil {
		s.publishTaskFailed(exhausted)
	}
	if err != nil {
		return nil, err
	}
	s.publishTaskClaimed(claim.Task, claim.Lease)
	return cloneClaimedTask(claim), nil
}

// UpdateTask implements orchestrator.TaskStore.
func (s *Store) UpdateTask(ctx context.Context, req orchestrator.UpdateTaskRequest) (*orchestrator.Task, error) {
	if req.ID == "" {
		return nil, errors.New("gollem/orchestrator/sqlite: task id must not be empty")
	}
	ctx = normalizeContext(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		task        *orchestrator.Task
		peerUpdates []*orchestrator.Task
	)
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loaded, err := s.loadTaskTx(ctx, tx, req.ID)
		if err != nil {
			return err
		}
		task = loaded
		if err := s.validateTaskDependenciesTx(ctx, tx, req.AddBlocks, req.AddBlockedBy); err != nil {
			return err
		}

		if req.Subject != nil {
			task.Subject = *req.Subject
		}
		if req.Description != nil {
			task.Description = *req.Description
		}
		if len(req.AddBlocks) > 0 {
			task.Blocks = appendUniqueStrings(task.Blocks, req.AddBlocks...)
		}
		if len(req.AddBlockedBy) > 0 {
			task.BlockedBy = appendUniqueStrings(task.BlockedBy, req.AddBlockedBy...)
		}
		if len(req.Metadata) > 0 {
			if task.Metadata == nil {
				task.Metadata = make(map[string]any)
			}
			for key, value := range req.Metadata {
				if value == nil {
					delete(task.Metadata, key)
					continue
				}
				task.Metadata[key] = value
			}
		}

		now := time.Now().UTC()
		task.UpdatedAt = now
		peerUpdates, err = s.linkTaskDependenciesTx(ctx, tx, task, now)
		if err != nil {
			return err
		}
		return s.saveTaskTx(ctx, tx, task)
	}); err != nil {
		return nil, err
	}

	s.publishTaskUpdated(task)
	s.publishTaskUpdates(peerUpdates...)
	return cloneTask(task), nil
}

// DeleteTask implements orchestrator.TaskStore.
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	var peerUpdates []*orchestrator.Task
	now := time.Now().UTC()
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.loadTaskTx(ctx, tx, id); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete task: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, id); err != nil {
			return fmt.Errorf("delete task lease: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM artifacts WHERE task_id = ?`, id); err != nil {
			return fmt.Errorf("delete task artifacts: %w", err)
		}

		tasks, err := s.listTasksTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			blocksBefore := len(task.Blocks)
			blockedByBefore := len(task.BlockedBy)
			task.Blocks = removeString(task.Blocks, id)
			task.BlockedBy = removeString(task.BlockedBy, id)
			if len(task.Blocks) == blocksBefore && len(task.BlockedBy) == blockedByBefore {
				continue
			}
			task.UpdatedAt = now
			if err := s.saveTaskTx(ctx, tx, task); err != nil {
				return err
			}
			peerUpdates = append(peerUpdates, task)
		}
		return nil
	}); err != nil {
		return err
	}

	s.publishTaskDeleted(id, now)
	s.publishTaskUpdates(peerUpdates...)
	return nil
}

// CompleteTask implements orchestrator.TaskStore.
func (s *Store) CompleteTask(ctx context.Context, taskID, leaseToken string, outcome *orchestrator.TaskOutcome, now time.Time) (*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	now = normalizeNow(now)
	var (
		task      *orchestrator.Task
		artifacts []*orchestrator.Artifact
	)
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loadedTask, lease, err := s.validateLeaseTx(ctx, tx, taskID, leaseToken, now)
		if err != nil {
			return err
		}
		task = loadedTask

		result := (*orchestrator.TaskResult)(nil)
		if outcome != nil {
			result = outcome.Result
		}
		if result != nil && result.CompletedAt.IsZero() {
			result = cloneTaskResult(result)
			result.CompletedAt = now
		}

		task.Status = orchestrator.TaskCompleted
		task.Result = cloneTaskResult(result)
		task.LastError = ""
		task.CompletedAt = now
		task.UpdatedAt = now

		if err := s.saveTaskTx(ctx, tx, task); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, lease.TaskID); err != nil {
			return fmt.Errorf("delete completed lease: %w", err)
		}
		if outcome != nil {
			var err error
			artifacts, err = s.createOutcomeArtifactsTx(ctx, tx, task, outcome.Artifacts, now)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	s.publishTaskCompleted(task)
	for _, artifact := range artifacts {
		s.publishArtifactCreated(artifact)
	}
	return cloneTask(task), nil
}

// FailTask implements orchestrator.TaskStore.
func (s *Store) FailTask(ctx context.Context, taskID, leaseToken string, runErr error, now time.Time) (*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	now = normalizeNow(now)
	var (
		task        *orchestrator.Task
		requeued    bool
		lastRunID   string
		lastAttempt int
	)
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loadedTask, lease, err := s.validateLeaseTx(ctx, tx, taskID, leaseToken, now)
		if err != nil {
			return err
		}
		task = loadedTask

		if isRetryable(runErr) && !exhaustedAttempts(task) {
			lastRunID, lastAttempt = requeueTask(task, now, false)
			task.LastError = errorString(runErr, "retryable failure")
			if err := s.saveTaskTx(ctx, tx, task); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, lease.TaskID); err != nil {
				return fmt.Errorf("delete requeued lease: %w", err)
			}
			requeued = true
			return nil
		}

		task.Status = orchestrator.TaskFailed
		task.Result = nil
		task.LastError = errorString(runErr, "task failed")
		task.CompletedAt = now
		task.UpdatedAt = now
		if err := s.saveTaskTx(ctx, tx, task); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, lease.TaskID); err != nil {
			return fmt.Errorf("delete failed lease: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if requeued {
		s.publishTaskRequeued(task, lastRunID, lastAttempt, "retryable failure")
		return cloneTask(task), nil
	}
	if task.Status == orchestrator.TaskPending {
		return cloneTask(task), nil
	}
	s.publishTaskFailed(task)
	return cloneTask(task), nil
}

// CancelTask implements orchestrator.TaskStore.
func (s *Store) CancelTask(ctx context.Context, taskID, leaseToken, reason string, now time.Time) (*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	now = normalizeNow(now)
	var task *orchestrator.Task
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loadedTask, err := s.loadTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		task = loadedTask

		switch task.Status {
		case orchestrator.TaskPending:
			if leaseToken != "" {
				return orchestrator.ErrLeaseMismatch
			}
		case orchestrator.TaskRunning:
			lease, err := s.loadLeaseTx(ctx, tx, taskID)
			if err != nil {
				return err
			}
			if !lease.ExpiresAt.After(now) {
				return orchestrator.ErrLeaseExpired
			}
			if leaseToken == "" || lease.Token != leaseToken {
				return orchestrator.ErrLeaseMismatch
			}
		default:
			return orchestrator.ErrTaskNotCancelable
		}

		task.Status = orchestrator.TaskCanceled
		task.Result = nil
		task.LastError = cancelReason(reason)
		task.CompletedAt = now
		task.UpdatedAt = now
		if err := s.saveTaskTx(ctx, tx, task); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, taskID); err != nil {
			return fmt.Errorf("delete canceled lease: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	s.publishTaskCanceled(task)
	return cloneTask(task), nil
}

// RetryTask implements orchestrator.TaskStore.
func (s *Store) RetryTask(ctx context.Context, taskID, reason string, now time.Time) (*orchestrator.Task, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	now = normalizeNow(now)
	var (
		task        *orchestrator.Task
		lastRunID   string
		lastAttempt int
	)
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loadedTask, err := s.loadTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		task = loadedTask
		if task.Status != orchestrator.TaskFailed && task.Status != orchestrator.TaskCanceled {
			return orchestrator.ErrTaskNotRetryable
		}

		lastRunID, lastAttempt = requeueTask(task, now, false)
		task.LastError = ""
		task.Attempt = 0
		if err := s.saveTaskTx(ctx, tx, task); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, taskID); err != nil {
			return fmt.Errorf("delete retried lease: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	s.publishTaskRequeued(task, lastRunID, lastAttempt, retryReason(reason))
	return cloneTask(task), nil
}

// GetLease implements orchestrator.LeaseStore.
func (s *Store) GetLease(ctx context.Context, taskID string) (*orchestrator.Lease, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	lease, err := s.loadLease(ctx, s.db, taskID)
	if err != nil {
		return nil, err
	}
	return cloneLease(lease), nil
}

// RenewLease implements orchestrator.LeaseStore.
func (s *Store) RenewLease(ctx context.Context, taskID, leaseToken string, ttl time.Duration, now time.Time) (*orchestrator.Lease, error) {
	if ttl <= 0 {
		return nil, errors.New("gollem/orchestrator/sqlite: lease ttl must be positive")
	}
	ctx = normalizeContext(ctx)
	now = normalizeNow(now)

	s.mu.Lock()
	defer s.mu.Unlock()

	var lease *orchestrator.Lease
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loadedLease, err := s.loadLeaseTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if loadedLease.Token != leaseToken {
			return orchestrator.ErrLeaseMismatch
		}
		if !loadedLease.ExpiresAt.After(now) {
			return orchestrator.ErrLeaseExpired
		}
		loadedLease.ExpiresAt = now.Add(ttl)
		if err := s.saveLeaseTx(ctx, tx, loadedLease); err != nil {
			return err
		}
		lease = loadedLease
		return nil
	}); err != nil {
		return nil, err
	}

	s.publishLeaseRenewed(lease)
	return cloneLease(lease), nil
}

// ReleaseLease implements orchestrator.LeaseStore.
func (s *Store) ReleaseLease(ctx context.Context, taskID, leaseToken string) error {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		released    *orchestrator.Lease
		requeued    bool
		task        *orchestrator.Task
		lastRunID   string
		lastAttempt int
	)
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		lease, err := s.loadLeaseTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if lease.Token != leaseToken {
			return orchestrator.ErrLeaseMismatch
		}
		released = lease

		task, err = s.tryLoadTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if task != nil && task.Status == orchestrator.TaskRunning {
			requeued = true
			lastRunID, lastAttempt = requeueTask(task, time.Now().UTC(), true)
			if err := s.saveTaskTx(ctx, tx, task); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, taskID); err != nil {
			return fmt.Errorf("delete released lease: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	s.publishLeaseReleased(released, requeued)
	if requeued && task != nil {
		s.publishTaskRequeued(task, lastRunID, lastAttempt, "lease released")
	}
	return nil
}

// CreateArtifact implements orchestrator.ArtifactStore.
func (s *Store) CreateArtifact(ctx context.Context, req orchestrator.CreateArtifactRequest) (*orchestrator.Artifact, error) {
	if req.TaskID == "" {
		return nil, orchestrator.ErrArtifactTaskRequired
	}
	ctx = normalizeContext(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	var artifact *orchestrator.Artifact
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.loadTaskTx(ctx, tx, req.TaskID); err != nil {
			return err
		}
		artifact = &orchestrator.Artifact{
			ID:          newID("artifact"),
			TaskID:      req.TaskID,
			RunID:       req.RunID,
			Kind:        req.Kind,
			Name:        req.Name,
			ContentType: req.ContentType,
			Body:        cloneBytes(req.Body),
			Metadata:    cloneAnyMap(req.Metadata),
			CreatedAt:   time.Now().UTC(),
		}
		return s.saveArtifactTx(ctx, tx, artifact)
	}); err != nil {
		return nil, err
	}

	s.publishArtifactCreated(artifact)
	return cloneArtifact(artifact), nil
}

// GetArtifact implements orchestrator.ArtifactStore.
func (s *Store) GetArtifact(ctx context.Context, id string) (*orchestrator.Artifact, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	artifact, err := s.loadArtifact(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return cloneArtifact(artifact), nil
}

// ListArtifacts implements orchestrator.ArtifactStore.
func (s *Store) ListArtifacts(ctx context.Context, filter orchestrator.ArtifactFilter) ([]*orchestrator.Artifact, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM artifacts ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []*orchestrator.Artifact
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		if matchesArtifactFilter(artifact, filter) {
			artifacts = append(artifacts, cloneArtifact(artifact))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts: %w", err)
	}
	return artifacts, nil
}

// CreateCommand implements orchestrator.CommandStore.
func (s *Store) CreateCommand(ctx context.Context, req orchestrator.CreateCommandRequest) (*orchestrator.Command, error) {
	if req.TaskID == "" {
		return nil, orchestrator.ErrTaskNotFound
	}
	ctx = normalizeContext(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	var command *orchestrator.Command
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		task, err := s.loadTaskTx(ctx, tx, req.TaskID)
		if err != nil {
			return err
		}
		runID, targetWorkerID, err := validateCommandTarget(task, req)
		if err != nil {
			return err
		}

		command = &orchestrator.Command{
			ID:             newID("command"),
			Kind:           req.Kind,
			TaskID:         req.TaskID,
			RunID:          runID,
			TargetWorkerID: targetWorkerID,
			Reason:         req.Reason,
			Metadata:       cloneAnyMap(req.Metadata),
			Status:         orchestrator.CommandPending,
			CreatedAt:      time.Now().UTC(),
		}
		return s.saveCommandTx(ctx, tx, command)
	}); err != nil {
		return nil, err
	}

	s.publishCommandCreated(command)
	return cloneCommand(command), nil
}

// GetCommand implements orchestrator.CommandStore.
func (s *Store) GetCommand(ctx context.Context, id string) (*orchestrator.Command, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	command, err := s.loadCommand(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return cloneCommand(command), nil
}

// ListCommands implements orchestrator.CommandStore.
func (s *Store) ListCommands(ctx context.Context, filter orchestrator.CommandFilter) ([]*orchestrator.Command, error) {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM commands ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}
	defer rows.Close()

	var commands []*orchestrator.Command
	for rows.Next() {
		command, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		if matchesCommandFilter(command, filter) {
			commands = append(commands, cloneCommand(command))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commands: %w", err)
	}
	return commands, nil
}

// ClaimPendingCommand implements orchestrator.CommandStore.
func (s *Store) ClaimPendingCommand(ctx context.Context, req orchestrator.ClaimCommandRequest) (*orchestrator.Command, error) {
	if req.WorkerID == "" {
		return nil, errors.New("gollem/orchestrator/sqlite: command claim worker id must not be empty")
	}
	ctx = normalizeContext(ctx)
	now := normalizeNow(req.Now)

	s.mu.Lock()
	defer s.mu.Unlock()

	var command *orchestrator.Command
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT payload FROM commands ORDER BY created_at ASC, id ASC`)
		if err != nil {
			return fmt.Errorf("list pending commands: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			loaded, err := scanCommand(rows)
			if err != nil {
				return err
			}
			if loaded.Status != orchestrator.CommandPending {
				continue
			}

			if err := s.refreshCommandTargetTx(ctx, tx, loaded); err != nil {
				return err
			}
			if loaded.TargetWorkerID != "" && loaded.TargetWorkerID != req.WorkerID {
				continue
			}
			loaded.Status = orchestrator.CommandClaimed
			loaded.ClaimedBy = req.WorkerID
			loaded.ClaimToken = fmt.Sprintf("%s-claim-%d", loaded.ID, now.UnixNano())
			loaded.ClaimedAt = now
			if err := s.saveCommandTx(ctx, tx, loaded); err != nil {
				return err
			}
			command = loaded
			return nil
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pending commands: %w", err)
		}
		return orchestrator.ErrNoPendingCommand
	}); err != nil {
		return nil, err
	}

	return cloneCommand(command), nil
}

// HandleCommand implements orchestrator.CommandStore.
func (s *Store) HandleCommand(ctx context.Context, id, claimToken, handledBy string, now time.Time) (*orchestrator.Command, error) {
	now = normalizeNow(now)
	ctx = normalizeContext(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	var command *orchestrator.Command
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		loaded, err := s.loadCommandTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if loaded.Status != orchestrator.CommandClaimed || loaded.ClaimToken != claimToken {
			return orchestrator.ErrCommandClaimMismatch
		}
		loaded.Status = orchestrator.CommandHandled
		loaded.HandledBy = handledBy
		loaded.HandledAt = now
		loaded.ClaimToken = ""
		command = loaded
		return s.saveCommandTx(ctx, tx, loaded)
	}); err != nil {
		return nil, err
	}

	s.publishCommandHandled(command)
	return cloneCommand(command), nil
}

// ReleaseCommand implements orchestrator.CommandStore.
func (s *Store) ReleaseCommand(ctx context.Context, id, claimToken string) error {
	ctx = normalizeContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		command, err := s.loadCommandTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if command.Status != orchestrator.CommandClaimed || command.ClaimToken != claimToken {
			return orchestrator.ErrCommandClaimMismatch
		}
		command.Status = orchestrator.CommandPending
		command.ClaimedBy = ""
		command.ClaimToken = ""
		command.ClaimedAt = time.Time{}
		return s.saveCommandTx(ctx, tx, command)
	})
}

func (s *Store) init() error {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("set sqlite journal mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("set sqlite busy timeout: %w", err)
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			payload BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at, id)`,
		`CREATE TABLE IF NOT EXISTS leases (
			task_id TEXT PRIMARY KEY,
			expires_at TEXT NOT NULL,
			payload BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_leases_expires_at ON leases(expires_at)`,
		`CREATE TABLE IF NOT EXISTS commands (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			task_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			target_worker_id TEXT NOT NULL,
			payload BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_commands_created_at ON commands(created_at, id)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			task_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			name TEXT NOT NULL,
			payload BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_created_at ON artifacts(created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_task_id ON artifacts(task_id)`,
	}
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}
	return nil
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return err
}

func (s *Store) claimTaskTx(ctx context.Context, tx *sql.Tx, taskID string, req orchestrator.ClaimTaskRequest, now time.Time) (*orchestrator.ClaimedTask, *orchestrator.Task, error) {
	task, err := s.loadTaskTx(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if !matchesKinds(task, req.Kinds) {
		return nil, nil, orchestrator.ErrNoReadyTask
	}
	blocked, err := s.isBlockedTx(ctx, tx, task)
	if err != nil {
		return nil, nil, err
	}
	if blocked {
		return nil, nil, orchestrator.ErrTaskBlocked
	}

	lease, err := s.tryLoadLeaseTx(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	leaseExpired := lease != nil && !lease.ExpiresAt.After(now)
	hasActiveLease := lease != nil && !leaseExpired
	if hasActiveLease {
		return nil, nil, orchestrator.ErrNoReadyTask
	}

	if task.MaxAttempts > 0 && task.Attempt >= task.MaxAttempts {
		if task.Status != orchestrator.TaskCompleted && task.Status != orchestrator.TaskFailed {
			task.Status = orchestrator.TaskFailed
			task.LastError = "task exhausted max attempts"
			task.CompletedAt = now
			task.UpdatedAt = now
			if err := s.saveTaskTx(ctx, tx, task); err != nil {
				return nil, nil, err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE task_id = ?`, task.ID); err != nil {
				return nil, nil, fmt.Errorf("delete exhausted lease: %w", err)
			}
		}
		return nil, cloneTask(task), orchestrator.ErrNoReadyTask
	}

	switch task.Status {
	case orchestrator.TaskPending:
	case orchestrator.TaskRunning:
		if !leaseExpired && lease != nil {
			return nil, nil, orchestrator.ErrNoReadyTask
		}
	default:
		return nil, nil, orchestrator.ErrNoReadyTask
	}

	task.Attempt++
	task.Status = orchestrator.TaskRunning
	task.Run = &orchestrator.RunRef{
		ID:        newID("run"),
		TaskID:    task.ID,
		WorkerID:  req.WorkerID,
		Attempt:   task.Attempt,
		StartedAt: now,
	}
	task.Result = nil
	task.LastError = ""
	task.StartedAt = now
	task.CompletedAt = time.Time{}
	task.UpdatedAt = now
	if err := s.saveTaskTx(ctx, tx, task); err != nil {
		return nil, nil, err
	}

	taskLease := &orchestrator.Lease{
		ID:         newID("lease"),
		TaskID:     task.ID,
		WorkerID:   req.WorkerID,
		Token:      newID("lease-token"),
		AcquiredAt: now,
		ExpiresAt:  now.Add(req.LeaseTTL),
	}
	if err := s.saveLeaseTx(ctx, tx, taskLease); err != nil {
		return nil, nil, err
	}

	return &orchestrator.ClaimedTask{
		Task:  cloneTask(task),
		Lease: cloneLease(taskLease),
		Run:   cloneRunRef(task.Run),
	}, nil, nil
}

func (s *Store) validateTaskDependenciesTx(ctx context.Context, tx *sql.Tx, blocks, blockedBy []string) error {
	for _, taskID := range blocks {
		if _, err := s.loadTaskTx(ctx, tx, taskID); err != nil {
			if errors.Is(err, orchestrator.ErrTaskNotFound) {
				return orchestrator.ErrTaskDependencyNotFound
			}
			return err
		}
	}
	for _, taskID := range blockedBy {
		if _, err := s.loadTaskTx(ctx, tx, taskID); err != nil {
			if errors.Is(err, orchestrator.ErrTaskNotFound) {
				return orchestrator.ErrTaskDependencyNotFound
			}
			return err
		}
	}
	return nil
}

func (s *Store) linkTaskDependenciesTx(ctx context.Context, tx *sql.Tx, task *orchestrator.Task, now time.Time) ([]*orchestrator.Task, error) {
	updated := map[string]*orchestrator.Task{}
	for _, blockedID := range task.Blocks {
		blocked, err := s.loadTaskTx(ctx, tx, blockedID)
		if err != nil {
			return nil, err
		}
		before := len(blocked.BlockedBy)
		blocked.BlockedBy = appendUniqueStrings(blocked.BlockedBy, task.ID)
		if len(blocked.BlockedBy) == before {
			continue
		}
		blocked.UpdatedAt = now
		if err := s.saveTaskTx(ctx, tx, blocked); err != nil {
			return nil, err
		}
		updated[blocked.ID] = blocked
	}
	for _, blockerID := range task.BlockedBy {
		blocker, err := s.loadTaskTx(ctx, tx, blockerID)
		if err != nil {
			return nil, err
		}
		before := len(blocker.Blocks)
		blocker.Blocks = appendUniqueStrings(blocker.Blocks, task.ID)
		if len(blocker.Blocks) == before {
			continue
		}
		blocker.UpdatedAt = now
		if err := s.saveTaskTx(ctx, tx, blocker); err != nil {
			return nil, err
		}
		updated[blocker.ID] = blocker
	}
	if len(updated) == 0 {
		return nil, nil
	}
	peers := make([]*orchestrator.Task, 0, len(updated))
	for _, task := range updated {
		peers = append(peers, task)
	}
	return peers, nil
}

func (s *Store) isBlockedTx(ctx context.Context, tx *sql.Tx, task *orchestrator.Task) (bool, error) {
	for _, blockerID := range task.BlockedBy {
		blocker, err := s.tryLoadTaskTx(ctx, tx, blockerID)
		if err != nil {
			return false, err
		}
		if blocker == nil {
			continue
		}
		if blocker.Status != orchestrator.TaskCompleted {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) validateLeaseTx(ctx context.Context, tx *sql.Tx, taskID, leaseToken string, now time.Time) (*orchestrator.Task, *orchestrator.Lease, error) {
	task, err := s.loadTaskTx(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	lease, err := s.loadLeaseTx(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if lease.Token != leaseToken {
		return nil, nil, orchestrator.ErrLeaseMismatch
	}
	if !lease.ExpiresAt.After(normalizeNow(now)) {
		return nil, nil, orchestrator.ErrLeaseExpired
	}
	return task, lease, nil
}

func (s *Store) createOutcomeArtifactsTx(ctx context.Context, tx *sql.Tx, task *orchestrator.Task, artifacts []orchestrator.ArtifactSpec, createdAt time.Time) ([]*orchestrator.Artifact, error) {
	if task == nil || len(artifacts) == 0 {
		return nil, nil
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	var created []*orchestrator.Artifact
	for _, spec := range artifacts {
		artifact := &orchestrator.Artifact{
			ID:          newID("artifact"),
			TaskID:      task.ID,
			RunID:       runID,
			Kind:        spec.Kind,
			Name:        spec.Name,
			ContentType: spec.ContentType,
			Body:        cloneBytes(spec.Body),
			Metadata:    cloneAnyMap(spec.Metadata),
			CreatedAt:   createdAt,
		}
		if err := s.saveArtifactTx(ctx, tx, artifact); err != nil {
			return nil, err
		}
		created = append(created, artifact)
	}
	return created, nil
}

func (s *Store) refreshCommandTargetTx(ctx context.Context, tx *sql.Tx, command *orchestrator.Command) error {
	if command == nil {
		return nil
	}
	task, err := s.tryLoadTaskTx(ctx, tx, command.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		command.TargetWorkerID = ""
		command.RunID = ""
		return s.saveCommandTx(ctx, tx, command)
	}

	changed := false
	switch command.Kind {
	case orchestrator.CommandCancelTask:
		switch task.Status {
		case orchestrator.TaskRunning:
			runID := ""
			workerID := ""
			if task.Run != nil {
				runID = task.Run.ID
				workerID = task.Run.WorkerID
			}
			if command.RunID != runID || command.TargetWorkerID != workerID {
				command.RunID = runID
				command.TargetWorkerID = workerID
				changed = true
			}
		case orchestrator.TaskPending:
			if command.RunID != "" || command.TargetWorkerID != "" {
				command.RunID = ""
				command.TargetWorkerID = ""
				changed = true
			}
		}
	case orchestrator.CommandRetryTask:
		if command.TargetWorkerID != "" {
			command.TargetWorkerID = ""
			changed = true
		}
		if task.Run == nil && command.RunID != "" {
			command.RunID = ""
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveCommandTx(ctx, tx, command)
}

func validateCommandTarget(task *orchestrator.Task, req orchestrator.CreateCommandRequest) (runID, targetWorkerID string, err error) {
	if task == nil {
		return "", "", orchestrator.ErrTaskNotFound
	}
	runID = req.RunID
	if runID == "" && task.Run != nil {
		runID = task.Run.ID
	}

	switch req.Kind {
	case orchestrator.CommandCancelTask:
		switch task.Status {
		case orchestrator.TaskPending:
			return runID, "", nil
		case orchestrator.TaskRunning:
			if task.Run == nil || task.Run.WorkerID == "" {
				return "", "", orchestrator.ErrInvalidCommand
			}
			return runID, task.Run.WorkerID, nil
		default:
			return "", "", orchestrator.ErrTaskNotCancelable
		}
	case orchestrator.CommandRetryTask:
		switch task.Status {
		case orchestrator.TaskFailed, orchestrator.TaskCanceled:
			return runID, "", nil
		default:
			return "", "", orchestrator.ErrTaskNotRetryable
		}
	default:
		return "", "", orchestrator.ErrInvalidCommand
	}
}

func (s *Store) loadTask(ctx context.Context, db queryer, id string) (*orchestrator.Task, error) {
	row := db.QueryRowContext(ctx, `SELECT payload FROM tasks WHERE id = ?`, id)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrTaskNotFound
		}
		return nil, err
	}
	return task, nil
}

func (s *Store) tryLoadTaskTx(ctx context.Context, tx *sql.Tx, id string) (*orchestrator.Task, error) {
	row := tx.QueryRowContext(ctx, `SELECT payload FROM tasks WHERE id = ?`, id)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

func (s *Store) loadTaskTx(ctx context.Context, tx *sql.Tx, id string) (*orchestrator.Task, error) {
	task, err := s.tryLoadTaskTx(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, orchestrator.ErrTaskNotFound
	}
	return task, nil
}

func (s *Store) listTasks(ctx context.Context, db queryer) ([]*orchestrator.Task, error) {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM tasks ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *Store) listTasksTx(ctx context.Context, tx *sql.Tx) ([]*orchestrator.Task, error) {
	rows, err := tx.QueryContext(ctx, `SELECT payload FROM tasks ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *Store) listTaskIDsTx(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM tasks ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list task ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan task id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task ids: %w", err)
	}
	return ids, nil
}

func (s *Store) saveTaskTx(ctx context.Context, tx *sql.Tx, task *orchestrator.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task %q: %w", task.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (id, created_at, kind, status, payload)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			created_at = excluded.created_at,
			kind = excluded.kind,
			status = excluded.status,
			payload = excluded.payload
	`, task.ID, formatTime(task.CreatedAt), task.Kind, string(task.Status), payload)
	if err != nil {
		return fmt.Errorf("save task %q: %w", task.ID, err)
	}
	return nil
}

func (s *Store) loadLease(ctx context.Context, db queryer, taskID string) (*orchestrator.Lease, error) {
	row := db.QueryRowContext(ctx, `SELECT payload FROM leases WHERE task_id = ?`, taskID)
	lease, err := scanLease(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrLeaseNotFound
		}
		return nil, err
	}
	return lease, nil
}

func (s *Store) tryLoadLeaseTx(ctx context.Context, tx *sql.Tx, taskID string) (*orchestrator.Lease, error) {
	row := tx.QueryRowContext(ctx, `SELECT payload FROM leases WHERE task_id = ?`, taskID)
	lease, err := scanLease(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return lease, nil
}

func (s *Store) loadLeaseTx(ctx context.Context, tx *sql.Tx, taskID string) (*orchestrator.Lease, error) {
	lease, err := s.tryLoadLeaseTx(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, orchestrator.ErrLeaseNotFound
	}
	return lease, nil
}

func (s *Store) saveLeaseTx(ctx context.Context, tx *sql.Tx, lease *orchestrator.Lease) error {
	payload, err := json.Marshal(lease)
	if err != nil {
		return fmt.Errorf("marshal lease %q: %w", lease.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO leases (task_id, expires_at, payload)
		VALUES (?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			expires_at = excluded.expires_at,
			payload = excluded.payload
	`, lease.TaskID, formatTime(lease.ExpiresAt), payload)
	if err != nil {
		return fmt.Errorf("save lease %q: %w", lease.ID, err)
	}
	return nil
}

func (s *Store) saveCommandTx(ctx context.Context, tx *sql.Tx, command *orchestrator.Command) error {
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("marshal command %q: %w", command.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO commands (id, created_at, kind, status, task_id, run_id, target_worker_id, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			created_at = excluded.created_at,
			kind = excluded.kind,
			status = excluded.status,
			task_id = excluded.task_id,
			run_id = excluded.run_id,
			target_worker_id = excluded.target_worker_id,
			payload = excluded.payload
	`, command.ID, formatTime(command.CreatedAt), string(command.Kind), string(command.Status), command.TaskID, command.RunID, command.TargetWorkerID, payload)
	if err != nil {
		return fmt.Errorf("save command %q: %w", command.ID, err)
	}
	return nil
}

func (s *Store) loadCommand(ctx context.Context, db queryer, id string) (*orchestrator.Command, error) {
	row := db.QueryRowContext(ctx, `SELECT payload FROM commands WHERE id = ?`, id)
	command, err := scanCommand(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrCommandNotFound
		}
		return nil, err
	}
	return command, nil
}

func (s *Store) loadCommandTx(ctx context.Context, tx *sql.Tx, id string) (*orchestrator.Command, error) {
	row := tx.QueryRowContext(ctx, `SELECT payload FROM commands WHERE id = ?`, id)
	command, err := scanCommand(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrCommandNotFound
		}
		return nil, err
	}
	return command, nil
}

func (s *Store) saveArtifactTx(ctx context.Context, tx *sql.Tx, artifact *orchestrator.Artifact) error {
	payload, err := json.Marshal(artifact)
	if err != nil {
		return fmt.Errorf("marshal artifact %q: %w", artifact.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO artifacts (id, created_at, task_id, run_id, kind, name, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			created_at = excluded.created_at,
			task_id = excluded.task_id,
			run_id = excluded.run_id,
			kind = excluded.kind,
			name = excluded.name,
			payload = excluded.payload
	`, artifact.ID, formatTime(artifact.CreatedAt), artifact.TaskID, artifact.RunID, artifact.Kind, artifact.Name, payload)
	if err != nil {
		return fmt.Errorf("save artifact %q: %w", artifact.ID, err)
	}
	return nil
}

func (s *Store) loadArtifact(ctx context.Context, db queryer, id string) (*orchestrator.Artifact, error) {
	row := db.QueryRowContext(ctx, `SELECT payload FROM artifacts WHERE id = ?`, id)
	artifact, err := scanArtifact(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, orchestrator.ErrArtifactNotFound
		}
		return nil, err
	}
	return artifact, nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func scanTask(row interface{ Scan(dest ...any) error }) (*orchestrator.Task, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	var task orchestrator.Task
	if err := json.Unmarshal(payload, &task); err != nil {
		return nil, fmt.Errorf("unmarshal task payload: %w", err)
	}
	return &task, nil
}

func scanTasks(rows *sql.Rows) ([]*orchestrator.Task, error) {
	var tasks []*orchestrator.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func scanLease(row interface{ Scan(dest ...any) error }) (*orchestrator.Lease, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	var lease orchestrator.Lease
	if err := json.Unmarshal(payload, &lease); err != nil {
		return nil, fmt.Errorf("unmarshal lease payload: %w", err)
	}
	return &lease, nil
}

func scanCommand(row interface{ Scan(dest ...any) error }) (*orchestrator.Command, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	var command orchestrator.Command
	if err := json.Unmarshal(payload, &command); err != nil {
		return nil, fmt.Errorf("unmarshal command payload: %w", err)
	}
	return &command, nil
}

func scanArtifact(row interface{ Scan(dest ...any) error }) (*orchestrator.Artifact, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	var artifact orchestrator.Artifact
	if err := json.Unmarshal(payload, &artifact); err != nil {
		return nil, fmt.Errorf("unmarshal artifact payload: %w", err)
	}
	return &artifact, nil
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func formatTime(t time.Time) string {
	return normalizeNow(t).Format(timeFormat)
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.NewString())
}

func matchesTaskFilter(task *orchestrator.Task, filter orchestrator.TaskFilter) bool {
	return matchesKinds(task, filter.Kinds) && matchesStatuses(task, filter.Statuses)
}

func matchesKinds(task *orchestrator.Task, kinds []string) bool {
	if task == nil {
		return false
	}
	if len(kinds) == 0 {
		return true
	}
	for _, kind := range kinds {
		if task.Kind == kind {
			return true
		}
	}
	return false
}

func matchesStatuses(task *orchestrator.Task, statuses []orchestrator.TaskStatus) bool {
	if task == nil {
		return false
	}
	if len(statuses) == 0 {
		return true
	}
	for _, status := range statuses {
		if task.Status == status {
			return true
		}
	}
	return false
}

func matchesCommandFilter(command *orchestrator.Command, filter orchestrator.CommandFilter) bool {
	if command == nil {
		return false
	}
	if filter.TaskID != "" && command.TaskID != filter.TaskID {
		return false
	}
	if filter.RunID != "" && command.RunID != filter.RunID {
		return false
	}
	if filter.TargetWorkerID != "" && command.TargetWorkerID != filter.TargetWorkerID {
		return false
	}
	if len(filter.Kinds) > 0 {
		match := false
		for _, kind := range filter.Kinds {
			if command.Kind == kind {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if len(filter.Statuses) > 0 {
		match := false
		for _, status := range filter.Statuses {
			if command.Status == status {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func matchesArtifactFilter(artifact *orchestrator.Artifact, filter orchestrator.ArtifactFilter) bool {
	if artifact == nil {
		return false
	}
	if filter.TaskID != "" && artifact.TaskID != filter.TaskID {
		return false
	}
	if filter.RunID != "" && artifact.RunID != filter.RunID {
		return false
	}
	if filter.Kind != "" && artifact.Kind != filter.Kind {
		return false
	}
	if filter.Name != "" && artifact.Name != filter.Name {
		return false
	}
	return true
}

func exhaustedAttempts(task *orchestrator.Task) bool {
	return task != nil && task.MaxAttempts > 0 && task.Attempt >= task.MaxAttempts
}

func requeueTask(task *orchestrator.Task, now time.Time, rollbackAttempt bool) (string, int) {
	lastRunID := ""
	lastAttempt := task.Attempt
	if task.Run != nil {
		lastRunID = task.Run.ID
	}
	task.Status = orchestrator.TaskPending
	task.Result = nil
	task.Run = nil
	task.StartedAt = time.Time{}
	task.CompletedAt = time.Time{}
	task.UpdatedAt = normalizeNow(now)
	if rollbackAttempt && task.Attempt > 0 {
		task.Attempt--
	}
	return lastRunID, lastAttempt
}

func errorString(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return err.Error()
}

func isRetryable(err error) bool {
	var retryable *orchestrator.RetryableError
	return errors.As(err, &retryable)
}

func cancelReason(reason string) string {
	if reason != "" {
		return reason
	}
	return "task canceled"
}

func retryReason(reason string) string {
	if reason != "" {
		return "command retry: " + reason
	}
	return "command retry"
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func appendUniqueStrings(src []string, values ...string) []string {
	if len(values) == 0 {
		return cloneStrings(src)
	}
	out := cloneStrings(src)
	for _, value := range values {
		exists := false
		for _, current := range out {
			if current == value {
				exists = true
				break
			}
		}
		if !exists {
			out = append(out, value)
		}
	}
	return out
}

func removeString(src []string, target string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, 0, len(src))
	for _, value := range src {
		if value == target {
			continue
		}
		out = append(out, value)
	}
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func cloneRunRef(src *orchestrator.RunRef) *orchestrator.RunRef {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func cloneTaskResult(src *orchestrator.TaskResult) *orchestrator.TaskResult {
	if src == nil {
		return nil
	}
	return &orchestrator.TaskResult{
		RunnerRunID: src.RunnerRunID,
		Output:      src.Output,
		Usage:       src.Usage,
		ToolState:   cloneAnyMap(src.ToolState),
		Metadata:    cloneAnyMap(src.Metadata),
		CompletedAt: src.CompletedAt,
	}
}

func cloneTask(src *orchestrator.Task) *orchestrator.Task {
	if src == nil {
		return nil
	}
	return &orchestrator.Task{
		ID:          src.ID,
		Kind:        src.Kind,
		Subject:     src.Subject,
		Description: src.Description,
		Input:       src.Input,
		Status:      src.Status,
		Attempt:     src.Attempt,
		MaxAttempts: src.MaxAttempts,
		Blocks:      cloneStrings(src.Blocks),
		BlockedBy:   cloneStrings(src.BlockedBy),
		Metadata:    cloneAnyMap(src.Metadata),
		Run:         cloneRunRef(src.Run),
		Result:      cloneTaskResult(src.Result),
		LastError:   src.LastError,
		CreatedAt:   src.CreatedAt,
		UpdatedAt:   src.UpdatedAt,
		StartedAt:   src.StartedAt,
		CompletedAt: src.CompletedAt,
	}
}

func cloneLease(src *orchestrator.Lease) *orchestrator.Lease {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func cloneClaimedTask(src *orchestrator.ClaimedTask) *orchestrator.ClaimedTask {
	if src == nil {
		return nil
	}
	return &orchestrator.ClaimedTask{
		Task:  cloneTask(src.Task),
		Lease: cloneLease(src.Lease),
		Run:   cloneRunRef(src.Run),
	}
}

func cloneCommand(src *orchestrator.Command) *orchestrator.Command {
	if src == nil {
		return nil
	}
	return &orchestrator.Command{
		ID:             src.ID,
		Kind:           src.Kind,
		TaskID:         src.TaskID,
		RunID:          src.RunID,
		TargetWorkerID: src.TargetWorkerID,
		Reason:         src.Reason,
		Metadata:       cloneAnyMap(src.Metadata),
		Status:         src.Status,
		ClaimToken:     src.ClaimToken,
		ClaimedBy:      src.ClaimedBy,
		HandledBy:      src.HandledBy,
		CreatedAt:      src.CreatedAt,
		ClaimedAt:      src.ClaimedAt,
		HandledAt:      src.HandledAt,
	}
}

func cloneArtifact(src *orchestrator.Artifact) *orchestrator.Artifact {
	if src == nil {
		return nil
	}
	return &orchestrator.Artifact{
		ID:          src.ID,
		TaskID:      src.TaskID,
		RunID:       src.RunID,
		Kind:        src.Kind,
		Name:        src.Name,
		ContentType: src.ContentType,
		Body:        cloneBytes(src.Body),
		Metadata:    cloneAnyMap(src.Metadata),
		CreatedAt:   src.CreatedAt,
	}
}

func (s *Store) publishTaskCreated(task *orchestrator.Task) {
	if s.eventBus == nil || task == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskCreatedEvent{
		TaskID:      task.ID,
		Kind:        task.Kind,
		Subject:     task.Subject,
		Description: task.Description,
		CreatedAt:   task.CreatedAt,
	})
}

func (s *Store) publishTaskUpdated(task *orchestrator.Task) {
	if s.eventBus == nil || task == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskUpdatedEvent{
		TaskID:    task.ID,
		Subject:   task.Subject,
		Blocks:    cloneStrings(task.Blocks),
		BlockedBy: cloneStrings(task.BlockedBy),
		UpdatedAt: task.UpdatedAt,
	})
}

func (s *Store) publishTaskUpdates(tasks ...*orchestrator.Task) {
	for _, task := range tasks {
		s.publishTaskUpdated(task)
	}
}

func (s *Store) publishTaskDeleted(taskID string, deletedAt time.Time) {
	if s.eventBus == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskDeletedEvent{
		TaskID:    taskID,
		DeletedAt: deletedAt,
	})
}

func (s *Store) publishTaskClaimed(task *orchestrator.Task, lease *orchestrator.Lease) {
	if s.eventBus == nil || task == nil || lease == nil {
		return
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskClaimedEvent{
		TaskID:     task.ID,
		RunID:      runID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		Attempt:    task.Attempt,
		AcquiredAt: lease.AcquiredAt,
		ExpiresAt:  lease.ExpiresAt,
	})
}

func (s *Store) publishLeaseRenewed(lease *orchestrator.Lease) {
	if s.eventBus == nil || lease == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.LeaseRenewedEvent{
		TaskID:    lease.TaskID,
		LeaseID:   lease.ID,
		WorkerID:  lease.WorkerID,
		ExpiresAt: lease.ExpiresAt,
	})
}

func (s *Store) publishLeaseReleased(lease *orchestrator.Lease, requeued bool) {
	if s.eventBus == nil || lease == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.LeaseReleasedEvent{
		TaskID:     lease.TaskID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		ReleasedAt: time.Now().UTC(),
		Requeued:   requeued,
	})
}

func (s *Store) publishTaskRequeued(task *orchestrator.Task, lastRunID string, lastAttempt int, reason string) {
	if s.eventBus == nil || task == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskRequeuedEvent{
		TaskID:      task.ID,
		LastRunID:   lastRunID,
		LastAttempt: lastAttempt,
		Reason:      reason,
		RequeuedAt:  task.UpdatedAt,
	})
}

func (s *Store) publishTaskCompleted(task *orchestrator.Task) {
	if s.eventBus == nil || task == nil {
		return
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskCompletedEvent{
		TaskID:      task.ID,
		RunID:       runID,
		Attempt:     task.Attempt,
		CompletedAt: task.CompletedAt,
	})
}

func (s *Store) publishTaskFailed(task *orchestrator.Task) {
	if s.eventBus == nil || task == nil {
		return
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskFailedEvent{
		TaskID:   task.ID,
		RunID:    runID,
		Attempt:  task.Attempt,
		Error:    task.LastError,
		FailedAt: task.CompletedAt,
	})
}

func (s *Store) publishTaskCanceled(task *orchestrator.Task) {
	if s.eventBus == nil || task == nil {
		return
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskCanceledEvent{
		TaskID:     task.ID,
		RunID:      runID,
		Attempt:    task.Attempt,
		Reason:     task.LastError,
		CanceledAt: task.CompletedAt,
	})
}

func (s *Store) publishArtifactCreated(artifact *orchestrator.Artifact) {
	if s.eventBus == nil || artifact == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.ArtifactCreatedEvent{
		ArtifactID:  artifact.ID,
		TaskID:      artifact.TaskID,
		RunID:       artifact.RunID,
		Kind:        artifact.Kind,
		Name:        artifact.Name,
		ContentType: artifact.ContentType,
		SizeBytes:   len(artifact.Body),
		CreatedAt:   artifact.CreatedAt,
	})
}

func (s *Store) publishCommandCreated(command *orchestrator.Command) {
	if s.eventBus == nil || command == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.CommandCreatedEvent{
		CommandID:      command.ID,
		Kind:           command.Kind,
		TaskID:         command.TaskID,
		RunID:          command.RunID,
		TargetWorkerID: command.TargetWorkerID,
		CreatedAt:      command.CreatedAt,
	})
}

func (s *Store) publishCommandHandled(command *orchestrator.Command) {
	if s.eventBus == nil || command == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.CommandHandledEvent{
		CommandID: command.ID,
		Kind:      command.Kind,
		TaskID:    command.TaskID,
		RunID:     command.RunID,
		HandledBy: command.HandledBy,
		HandledAt: command.HandledAt,
	})
}

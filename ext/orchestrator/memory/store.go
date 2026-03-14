package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

// Store is an in-memory TaskStore and LeaseStore implementation.
type Store struct {
	mu            sync.Mutex
	tasks         map[string]*orchestrator.Task
	taskOrder     []string
	leases        map[string]*orchestrator.Lease
	commands      map[string]*orchestrator.Command
	commandOrder  []string
	artifacts     map[string]*orchestrator.Artifact
	artifactOrder []string
	nextTask      int
	nextLease     int
	nextRun       int
	nextCommand   int
	nextArtifact  int
	eventBus      *core.EventBus
}

var (
	_ orchestrator.TaskStore     = (*Store)(nil)
	_ orchestrator.LeaseStore    = (*Store)(nil)
	_ orchestrator.CommandStore  = (*Store)(nil)
	_ orchestrator.ArtifactStore = (*Store)(nil)
)

// Option configures a Store.
type Option func(*Store)

// WithEventBus publishes concrete orchestrator lifecycle events to the supplied bus.
func WithEventBus(bus *core.EventBus) Option {
	return func(s *Store) {
		s.eventBus = bus
	}
}

// NewStore creates an empty in-memory orchestration store.
func NewStore(opts ...Option) *Store {
	store := &Store{
		tasks:     make(map[string]*orchestrator.Task),
		leases:    make(map[string]*orchestrator.Lease),
		commands:  make(map[string]*orchestrator.Command),
		artifacts: make(map[string]*orchestrator.Artifact),
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// CreateTask implements orchestrator.TaskStore.
func (s *Store) CreateTask(_ context.Context, req orchestrator.CreateTaskRequest) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateTaskDependencies(req.Blocks, req.BlockedBy); err != nil {
		return nil, err
	}

	s.nextTask++
	now := time.Now()
	task := &orchestrator.Task{
		ID:          fmt.Sprintf("task-%d", s.nextTask),
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
	peerUpdates := s.linkTaskDependencies(task, now)
	s.tasks[task.ID] = task
	s.taskOrder = append(s.taskOrder, task.ID)
	s.publishTaskCreated(task)
	s.publishTaskUpdates(peerUpdates...)
	return cloneTask(task), nil
}

// GetTask implements orchestrator.TaskStore.
func (s *Store) GetTask(_ context.Context, id string) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, orchestrator.ErrTaskNotFound
	}
	return cloneTask(task), nil
}

// ListTasks implements orchestrator.TaskStore.
func (s *Store) ListTasks(_ context.Context, filter orchestrator.TaskFilter) ([]*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var tasks []*orchestrator.Task
	for _, id := range s.taskOrder {
		task, ok := s.tasks[id]
		if !ok || !matchesFilter(task, filter) {
			continue
		}
		tasks = append(tasks, cloneTask(task))
	}
	return tasks, nil
}

// ClaimReadyTask implements orchestrator.TaskStore.
func (s *Store) ClaimReadyTask(_ context.Context, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	if req.LeaseTTL <= 0 {
		return nil, errors.New("orchestrator/memory: lease ttl must be positive")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.taskOrder {
		claim, err := s.claimTaskLocked(id, req, now)
		if err == nil {
			return claim, nil
		}
		if errors.Is(err, orchestrator.ErrNoReadyTask) || errors.Is(err, orchestrator.ErrTaskBlocked) {
			continue
		}
		return nil, err
	}

	return nil, orchestrator.ErrNoReadyTask
}

// ClaimTask implements orchestrator.TaskStore.
func (s *Store) ClaimTask(_ context.Context, taskID string, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	if req.LeaseTTL <= 0 {
		return nil, errors.New("orchestrator/memory: lease ttl must be positive")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.claimTaskLocked(taskID, req, now)
}

// UpdateTask implements orchestrator.TaskStore.
func (s *Store) UpdateTask(_ context.Context, req orchestrator.UpdateTaskRequest) (*orchestrator.Task, error) {
	if req.ID == "" {
		return nil, errors.New("orchestrator/memory: task id must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[req.ID]
	if !ok {
		return nil, orchestrator.ErrTaskNotFound
	}

	if err := s.validateTaskDependencies(req.AddBlocks, req.AddBlockedBy); err != nil {
		return nil, err
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

	now := time.Now()
	peerUpdates := s.linkTaskDependencies(task, now)
	task.UpdatedAt = now
	s.publishTaskUpdated(task)
	s.publishTaskUpdates(peerUpdates...)
	return cloneTask(task), nil
}

// DeleteTask implements orchestrator.TaskStore.
func (s *Store) DeleteTask(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return orchestrator.ErrTaskNotFound
	}
	now := time.Now()
	delete(s.tasks, id)
	delete(s.leases, id)
	s.taskOrder = removeString(s.taskOrder, id)
	s.deleteTaskArtifactsLocked(id)
	var peerUpdates []*orchestrator.Task
	for _, task := range s.tasks {
		blocksBefore := len(task.Blocks)
		blockedByBefore := len(task.BlockedBy)
		task.Blocks = removeString(task.Blocks, id)
		task.BlockedBy = removeString(task.BlockedBy, id)
		if len(task.Blocks) != blocksBefore || len(task.BlockedBy) != blockedByBefore {
			task.UpdatedAt = now
			peerUpdates = append(peerUpdates, task)
		}
	}
	s.publishTaskDeleted(id)
	s.publishTaskUpdates(peerUpdates...)
	return nil
}

func (s *Store) deleteTaskArtifactsLocked(taskID string) {
	if len(s.artifactOrder) == 0 {
		return
	}
	filtered := s.artifactOrder[:0]
	for _, artifactID := range s.artifactOrder {
		artifact, ok := s.artifacts[artifactID]
		if ok && artifact.TaskID == taskID {
			delete(s.artifacts, artifactID)
			continue
		}
		filtered = append(filtered, artifactID)
	}
	s.artifactOrder = filtered
}

// CompleteTask implements orchestrator.TaskStore.
func (s *Store) CompleteTask(_ context.Context, taskID, leaseToken string, outcome *orchestrator.TaskOutcome, now time.Time) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, lease, err := s.validateLeaseLocked(taskID, leaseToken, now)
	if err != nil {
		return nil, err
	}

	result := (*orchestrator.TaskResult)(nil)
	if outcome != nil {
		result = outcome.Result
	}
	if result != nil && result.CompletedAt.IsZero() {
		result = cloneTaskResult(result)
		result.CompletedAt = normalizeNow(now)
	}
	task.Status = orchestrator.TaskCompleted
	task.Result = cloneTaskResult(result)
	task.LastError = ""
	task.CompletedAt = normalizeNow(now)
	task.UpdatedAt = task.CompletedAt
	delete(s.leases, lease.TaskID)
	if outcome != nil {
		s.createOutcomeArtifactsLocked(task, outcome.Artifacts, task.CompletedAt)
	}
	s.publishTaskCompleted(task)
	return cloneTask(task), nil
}

// FailTask implements orchestrator.TaskStore.
func (s *Store) FailTask(_ context.Context, taskID, leaseToken string, runErr error, now time.Time) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, lease, err := s.validateLeaseLocked(taskID, leaseToken, now)
	if err != nil {
		return nil, err
	}

	retryable := false
	if runErr != nil {
		retryable = isRetryable(runErr)
	}
	if retryable && !s.exhaustedAttempts(task) {
		lastRunID, lastAttempt := s.requeueTaskLocked(task, lease.TaskID, now, false)
		task.LastError = runErr.Error()
		s.publishTaskRequeued(task, lastRunID, lastAttempt, "retryable failure")
		return cloneTask(task), nil
	}

	task.Status = orchestrator.TaskFailed
	task.Result = nil
	if runErr != nil {
		task.LastError = runErr.Error()
	} else {
		task.LastError = "task failed"
	}
	task.CompletedAt = normalizeNow(now)
	task.UpdatedAt = task.CompletedAt
	delete(s.leases, lease.TaskID)
	s.publishTaskFailed(task)
	return cloneTask(task), nil
}

// CancelTask implements orchestrator.TaskStore.
func (s *Store) CancelTask(_ context.Context, taskID, leaseToken, reason string, now time.Time) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, orchestrator.ErrTaskNotFound
	}
	if now.IsZero() {
		now = time.Now()
	}
	switch task.Status {
	case orchestrator.TaskPending:
		if leaseToken != "" {
			return nil, orchestrator.ErrLeaseMismatch
		}
	case orchestrator.TaskRunning:
		lease, ok := s.leases[taskID]
		if !ok {
			return nil, orchestrator.ErrLeaseNotFound
		}
		if !lease.ExpiresAt.After(now) {
			return nil, orchestrator.ErrLeaseExpired
		}
		if leaseToken == "" || lease.Token != leaseToken {
			return nil, orchestrator.ErrLeaseMismatch
		}
	default:
		return nil, orchestrator.ErrTaskNotCancelable
	}

	task.Status = orchestrator.TaskCanceled
	task.Result = nil
	task.LastError = cancelReason(reason)
	task.CompletedAt = normalizeNow(now)
	task.UpdatedAt = task.CompletedAt
	delete(s.leases, taskID)
	s.publishTaskCanceled(task)
	return cloneTask(task), nil
}

// RetryTask implements orchestrator.TaskStore.
func (s *Store) RetryTask(_ context.Context, taskID, reason string, now time.Time) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, orchestrator.ErrTaskNotFound
	}
	if task.Status != orchestrator.TaskFailed && task.Status != orchestrator.TaskCanceled {
		return nil, orchestrator.ErrTaskNotRetryable
	}
	if now.IsZero() {
		now = time.Now()
	}

	lastRunID := ""
	lastAttempt := task.Attempt
	if task.Run != nil {
		lastRunID = task.Run.ID
	}
	task.Status = orchestrator.TaskPending
	task.Result = nil
	task.LastError = ""
	task.Run = nil
	task.StartedAt = time.Time{}
	task.CompletedAt = time.Time{}
	task.UpdatedAt = now
	task.Attempt = 0
	delete(s.leases, taskID)
	s.publishTaskRequeued(task, lastRunID, lastAttempt, retryReason(reason))
	return cloneTask(task), nil
}

// GetLease implements orchestrator.LeaseStore.
func (s *Store) GetLease(_ context.Context, taskID string) (*orchestrator.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lease, ok := s.leases[taskID]
	if !ok {
		return nil, orchestrator.ErrLeaseNotFound
	}
	return cloneLease(lease), nil
}

// RenewLease implements orchestrator.LeaseStore.
func (s *Store) RenewLease(_ context.Context, taskID, leaseToken string, ttl time.Duration, now time.Time) (*orchestrator.Lease, error) {
	if ttl <= 0 {
		return nil, errors.New("orchestrator/memory: lease ttl must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	lease, ok := s.leases[taskID]
	if !ok {
		return nil, orchestrator.ErrLeaseNotFound
	}
	if lease.Token != leaseToken {
		return nil, orchestrator.ErrLeaseMismatch
	}
	now = normalizeNow(now)
	if !lease.ExpiresAt.After(now) {
		return nil, orchestrator.ErrLeaseExpired
	}

	lease.ExpiresAt = now.Add(ttl)
	runID := ""
	if task, ok := s.tasks[taskID]; ok && task.Run != nil {
		runID = task.Run.ID
	}
	s.publishLeaseRenewed(lease, runID)
	return cloneLease(lease), nil
}

// ReleaseLease implements orchestrator.LeaseStore.
func (s *Store) ReleaseLease(_ context.Context, taskID, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lease, ok := s.leases[taskID]
	if !ok {
		return orchestrator.ErrLeaseNotFound
	}
	if lease.Token != leaseToken {
		return orchestrator.ErrLeaseMismatch
	}
	if task, ok := s.tasks[taskID]; ok && task.Status == orchestrator.TaskRunning {
		released := cloneLease(lease)
		lastRunID, lastAttempt := s.requeueTaskLocked(task, taskID, time.Now(), true)
		s.publishLeaseReleased(released, lastRunID, true)
		s.publishTaskRequeued(task, lastRunID, lastAttempt, "lease released")
		return nil
	}
	delete(s.leases, taskID)
	runID := ""
	if task, ok := s.tasks[taskID]; ok && task.Run != nil {
		runID = task.Run.ID
	}
	s.publishLeaseReleased(cloneLease(lease), runID, false)
	return nil
}

func (s *Store) validateLeaseLocked(taskID, leaseToken string, now time.Time) (*orchestrator.Task, *orchestrator.Lease, error) {
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, nil, orchestrator.ErrTaskNotFound
	}
	lease, ok := s.leases[taskID]
	if !ok {
		return nil, nil, orchestrator.ErrLeaseNotFound
	}
	if lease.Token != leaseToken {
		return nil, nil, orchestrator.ErrLeaseMismatch
	}
	now = normalizeNow(now)
	if !lease.ExpiresAt.After(now) {
		return nil, nil, orchestrator.ErrLeaseExpired
	}
	return task, lease, nil
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}

func matchesFilter(task *orchestrator.Task, filter orchestrator.TaskFilter) bool {
	return matchesKinds(task, filter.Kinds) && matchesStatuses(task, filter.Statuses)
}

func (s *Store) isBlocked(task *orchestrator.Task) bool {
	for _, blockerID := range task.BlockedBy {
		blocker, ok := s.tasks[blockerID]
		if !ok {
			continue
		}
		if blocker.Status != orchestrator.TaskCompleted {
			return true
		}
	}
	return false
}

func (s *Store) exhaustedAttempts(task *orchestrator.Task) bool {
	return task.MaxAttempts > 0 && task.Attempt >= task.MaxAttempts
}

func (s *Store) linkTaskDependencies(task *orchestrator.Task, now time.Time) []*orchestrator.Task {
	updated := map[string]*orchestrator.Task{}
	for _, blockedID := range task.Blocks {
		if blocked, ok := s.tasks[blockedID]; ok {
			before := len(blocked.BlockedBy)
			blocked.BlockedBy = appendUniqueStrings(blocked.BlockedBy, task.ID)
			if len(blocked.BlockedBy) != before {
				blocked.UpdatedAt = now
				updated[blocked.ID] = blocked
			}
		}
	}
	for _, blockerID := range task.BlockedBy {
		if blocker, ok := s.tasks[blockerID]; ok {
			before := len(blocker.Blocks)
			blocker.Blocks = appendUniqueStrings(blocker.Blocks, task.ID)
			if len(blocker.Blocks) != before {
				blocker.UpdatedAt = now
				updated[blocker.ID] = blocker
			}
		}
	}
	if len(updated) == 0 {
		return nil
	}
	peers := make([]*orchestrator.Task, 0, len(updated))
	for _, task := range updated {
		peers = append(peers, task)
	}
	return peers
}

func (s *Store) claimTaskLocked(taskID string, req orchestrator.ClaimTaskRequest, now time.Time) (*orchestrator.ClaimedTask, error) {
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, orchestrator.ErrTaskNotFound
	}
	if !matchesKinds(task, req.Kinds) {
		return nil, orchestrator.ErrNoReadyTask
	}
	if s.isBlocked(task) {
		return nil, orchestrator.ErrTaskBlocked
	}

	lease := s.leases[taskID]
	leaseExpired := lease != nil && !lease.ExpiresAt.After(now)
	hasActiveLease := lease != nil && !leaseExpired
	if hasActiveLease {
		return nil, orchestrator.ErrNoReadyTask
	}

	if task.MaxAttempts > 0 && task.Attempt >= task.MaxAttempts {
		if task.Status != orchestrator.TaskCompleted && task.Status != orchestrator.TaskFailed {
			task.Status = orchestrator.TaskFailed
			task.LastError = "task exhausted max attempts"
			task.CompletedAt = now
			task.UpdatedAt = now
			delete(s.leases, task.ID)
			s.publishTaskFailed(task)
		}
		return nil, orchestrator.ErrNoReadyTask
	}

	switch task.Status {
	case orchestrator.TaskPending:
	case orchestrator.TaskRunning:
		if !leaseExpired && lease != nil {
			return nil, orchestrator.ErrNoReadyTask
		}
	default:
		return nil, orchestrator.ErrNoReadyTask
	}

	s.nextRun++
	task.Attempt++
	task.Status = orchestrator.TaskRunning
	task.Run = &orchestrator.RunRef{
		ID:        fmt.Sprintf("run-%d", s.nextRun),
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

	s.nextLease++
	leaseID := fmt.Sprintf("lease-%d", s.nextLease)
	taskLease := &orchestrator.Lease{
		ID:         leaseID,
		TaskID:     task.ID,
		WorkerID:   req.WorkerID,
		Token:      leaseID,
		AcquiredAt: now,
		ExpiresAt:  now.Add(req.LeaseTTL),
	}
	s.leases[task.ID] = taskLease
	s.publishTaskClaimed(task, taskLease)

	return &orchestrator.ClaimedTask{
		Task:  cloneTask(task),
		Lease: cloneLease(taskLease),
		Run:   cloneRunRef(task.Run),
	}, nil
}

func (s *Store) requeueTaskLocked(task *orchestrator.Task, taskID string, now time.Time, rollbackAttempt bool) (string, int) {
	lastRunID := ""
	lastAttempt := task.Attempt
	if task.Run != nil {
		lastRunID = task.Run.ID
	}
	delete(s.leases, taskID)
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

func (s *Store) createOutcomeArtifactsLocked(task *orchestrator.Task, artifacts []orchestrator.ArtifactSpec, createdAt time.Time) {
	if task == nil || len(artifacts) == 0 {
		return
	}
	runID := ""
	if task.Run != nil {
		runID = task.Run.ID
	}
	for _, spec := range artifacts {
		s.nextArtifact++
		artifact := &orchestrator.Artifact{
			ID:          fmt.Sprintf("artifact-%d", s.nextArtifact),
			TaskID:      task.ID,
			RunID:       runID,
			Kind:        spec.Kind,
			Name:        spec.Name,
			ContentType: spec.ContentType,
			Body:        cloneBytes(spec.Body),
			Metadata:    cloneAnyMap(spec.Metadata),
			CreatedAt:   createdAt,
		}
		s.artifacts[artifact.ID] = artifact
		s.artifactOrder = append(s.artifactOrder, artifact.ID)
		s.publishArtifactCreated(artifact)
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

func (s *Store) publishTaskDeleted(taskID string) {
	if s.eventBus == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskDeletedEvent{
		TaskID:    taskID,
		DeletedAt: time.Now(),
	})
}

func (s *Store) publishTaskClaimed(task *orchestrator.Task, lease *orchestrator.Lease) {
	if s.eventBus == nil || task == nil || lease == nil || task.Run == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.TaskClaimedEvent{
		TaskID:     task.ID,
		RunID:      task.Run.ID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		Attempt:    task.Attempt,
		AcquiredAt: lease.AcquiredAt,
		ExpiresAt:  lease.ExpiresAt,
	})
}

func (s *Store) publishLeaseRenewed(lease *orchestrator.Lease, runID string) {
	if s.eventBus == nil || lease == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.LeaseRenewedEvent{
		TaskID:    lease.TaskID,
		RunID:     runID,
		LeaseID:   lease.ID,
		WorkerID:  lease.WorkerID,
		ExpiresAt: lease.ExpiresAt,
	})
}

func (s *Store) publishLeaseReleased(lease *orchestrator.Lease, runID string, requeued bool) {
	if s.eventBus == nil || lease == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.LeaseReleasedEvent{
		TaskID:     lease.TaskID,
		RunID:      runID,
		LeaseID:    lease.ID,
		WorkerID:   lease.WorkerID,
		ReleasedAt: time.Now(),
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

func (s *Store) validateTaskDependencies(blocks, blockedBy []string) error {
	for _, taskID := range blocks {
		if _, ok := s.tasks[taskID]; !ok {
			return fmt.Errorf("%w: blocks %q", orchestrator.ErrTaskDependencyNotFound, taskID)
		}
	}
	for _, taskID := range blockedBy {
		if _, ok := s.tasks[taskID]; !ok {
			return fmt.Errorf("%w: blocked_by %q", orchestrator.ErrTaskDependencyNotFound, taskID)
		}
	}
	return nil
}

func matchesKinds(task *orchestrator.Task, kinds []string) bool {
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

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(src))
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}

func cloneRunRef(ref *orchestrator.RunRef) *orchestrator.RunRef {
	if ref == nil {
		return nil
	}
	cp := *ref
	return &cp
}

func cloneLease(lease *orchestrator.Lease) *orchestrator.Lease {
	if lease == nil {
		return nil
	}
	cp := *lease
	return &cp
}

func cloneTaskResult(result *orchestrator.TaskResult) *orchestrator.TaskResult {
	if result == nil {
		return nil
	}
	cp := *result
	cp.ToolState = cloneAnyMap(result.ToolState)
	cp.Metadata = cloneAnyMap(result.Metadata)
	return &cp
}

func cloneTask(task *orchestrator.Task) *orchestrator.Task {
	if task == nil {
		return nil
	}
	cp := *task
	cp.Blocks = cloneStrings(task.Blocks)
	cp.BlockedBy = cloneStrings(task.BlockedBy)
	cp.Metadata = cloneAnyMap(task.Metadata)
	cp.Run = cloneRunRef(task.Run)
	cp.Result = cloneTaskResult(task.Result)
	return &cp
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]string, len(src))
	copy(cloned, src)
	return cloned
}

func appendUniqueStrings(dst []string, values ...string) []string {
	if len(values) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	return dst
}

func removeString(slice []string, target string) []string {
	if len(slice) == 0 {
		return slice
	}
	result := slice[:0]
	for _, item := range slice {
		if item != target {
			result = append(result, item)
		}
	}
	return result
}

func isRetryable(err error) bool {
	var retryable *orchestrator.RetryableError
	return errors.As(err, &retryable)
}

func cancelReason(reason string) string {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return trimmed
	}
	return "task canceled"
}

func retryReason(reason string) string {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return "command retry: " + trimmed
	}
	return "command retry"
}

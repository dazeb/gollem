package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
)

// Store is an in-memory TaskStore and LeaseStore implementation.
type Store struct {
	mu        sync.Mutex
	tasks     map[string]*orchestrator.Task
	taskOrder []string
	leases    map[string]*orchestrator.Lease
	nextTask  int
	nextLease int
	nextRun   int
}

var (
	_ orchestrator.TaskStore  = (*Store)(nil)
	_ orchestrator.LeaseStore = (*Store)(nil)
)

// NewStore creates an empty in-memory orchestration store.
func NewStore() *Store {
	return &Store{
		tasks:  make(map[string]*orchestrator.Task),
		leases: make(map[string]*orchestrator.Lease),
	}
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
	s.linkTaskDependencies(task)
	s.tasks[task.ID] = task
	s.taskOrder = append(s.taskOrder, task.ID)
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
		task, ok := s.tasks[id]
		if !ok || !matchesKinds(task, req.Kinds) {
			continue
		}
		if s.isBlocked(task) {
			continue
		}

		lease := s.leases[id]
		leaseExpired := lease != nil && !lease.ExpiresAt.After(now)
		hasActiveLease := lease != nil && !leaseExpired
		if hasActiveLease {
			continue
		}

		if task.MaxAttempts > 0 && task.Attempt >= task.MaxAttempts {
			if task.Status != orchestrator.TaskCompleted && task.Status != orchestrator.TaskFailed {
				task.Status = orchestrator.TaskFailed
				task.LastError = "task exhausted max attempts"
				task.CompletedAt = now
				task.UpdatedAt = now
				delete(s.leases, task.ID)
			}
			continue
		}

		switch task.Status {
		case orchestrator.TaskPending:
		case orchestrator.TaskRunning:
			if !leaseExpired && lease != nil {
				continue
			}
		default:
			continue
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

		return &orchestrator.ClaimedTask{
			Task:  cloneTask(task),
			Lease: cloneLease(taskLease),
			Run:   cloneRunRef(task.Run),
		}, nil
	}

	return nil, orchestrator.ErrNoReadyTask
}

// CompleteTask implements orchestrator.TaskStore.
func (s *Store) CompleteTask(_ context.Context, taskID, leaseToken string, result *orchestrator.TaskResult, now time.Time) (*orchestrator.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, lease, err := s.validateLeaseLocked(taskID, leaseToken, now)
	if err != nil {
		return nil, err
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
		s.requeueTaskLocked(task, lease.TaskID, now, false)
		task.LastError = runErr.Error()
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
		s.requeueTaskLocked(task, taskID, time.Now(), true)
		return nil
	}
	delete(s.leases, taskID)
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

func (s *Store) linkTaskDependencies(task *orchestrator.Task) {
	for _, blockedID := range task.Blocks {
		if blocked, ok := s.tasks[blockedID]; ok {
			blocked.BlockedBy = appendUniqueStrings(blocked.BlockedBy, task.ID)
		}
	}
	for _, blockerID := range task.BlockedBy {
		if blocker, ok := s.tasks[blockerID]; ok {
			blocker.Blocks = appendUniqueStrings(blocker.Blocks, task.ID)
		}
	}
}

func (s *Store) requeueTaskLocked(task *orchestrator.Task, taskID string, now time.Time, rollbackAttempt bool) {
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

func isRetryable(err error) bool {
	var retryable *orchestrator.RetryableError
	return errors.As(err, &retryable)
}

package team

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	memstore "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

const taskBoardClaimTTL = 365 * 24 * time.Hour

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskBlocked    TaskStatus = "blocked"
)

// Task represents a unit of work on the task board.
type Task struct {
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	Status      TaskStatus     `json:"status"`
	Owner       string         `json:"owner,omitempty"`
	Blocks      []string       `json:"blocks,omitempty"`
	BlockedBy   []string       `json:"blocked_by,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TaskBoard is a legacy compatibility adapter over orchestrator-backed team tasks.
//
// Deprecated: prefer ext/orchestrator for new task orchestration code.
type TaskBoard struct {
	mu               sync.RWMutex
	tasks            orchestrator.TaskStore
	leases           orchestrator.LeaseStore
	ownerOverrides   map[string]string
	blockedOverrides map[string]bool
}

// NewTaskBoard creates an empty task board.
func NewTaskBoard() *TaskBoard {
	return newTaskBoard(nil)
}

func newTaskBoard(bus *core.EventBus) *TaskBoard {
	store := memstore.NewStore(memstore.WithEventBus(bus))
	return &TaskBoard{
		tasks:            store,
		leases:           store,
		ownerOverrides:   make(map[string]string),
		blockedOverrides: make(map[string]bool),
	}
}

// Create adds a new task and returns its ID.
func (tb *TaskBoard) Create(subject, description string) string {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	task, err := tb.tasks.CreateTask(context.Background(), orchestrator.CreateTaskRequest{
		Kind:        "team",
		Subject:     subject,
		Description: description,
	})
	if err != nil {
		panic(fmt.Sprintf("team task create failed: %v", err))
	}
	return task.ID
}

// Get returns a copy of the task with the given ID.
func (tb *TaskBoard) Get(id string) (*Task, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.getLocked(id)
}

func (tb *TaskBoard) getLocked(id string) (*Task, error) {
	task, err := tb.tasks.GetTask(context.Background(), id)
	if err != nil {
		if errors.Is(err, orchestrator.ErrTaskNotFound) {
			return nil, fmt.Errorf("task %q not found", id)
		}
		return nil, err
	}
	return tb.convertTaskLocked(task), nil
}

// List returns copies of all tasks.
func (tb *TaskBoard) List() []*Task {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	rawTasks, err := tb.tasks.ListTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		return nil
	}
	result := make([]*Task, 0, len(rawTasks))
	for _, task := range rawTasks {
		result = append(result, tb.convertTaskLocked(task))
	}
	return result
}

// TaskUpdateOption is a functional option for updating a task.
type TaskUpdateOption func(*Task)

// WithStatus sets the task status.
func WithStatus(s TaskStatus) TaskUpdateOption {
	return func(t *Task) { t.Status = s }
}

// WithOwner sets the task owner.
func WithOwner(owner string) TaskUpdateOption {
	return func(t *Task) { t.Owner = owner }
}

// WithSubject sets the task subject.
func WithSubject(subject string) TaskUpdateOption {
	return func(t *Task) { t.Subject = subject }
}

// WithDescription sets the task description.
func WithDescription(desc string) TaskUpdateOption {
	return func(t *Task) { t.Description = desc }
}

// WithAddBlocks adds task IDs that this task blocks.
func WithAddBlocks(ids ...string) TaskUpdateOption {
	return func(t *Task) { t.Blocks = appendUnique(t.Blocks, ids...) }
}

// WithAddBlockedBy adds task IDs that block this task.
func WithAddBlockedBy(ids ...string) TaskUpdateOption {
	return func(t *Task) { t.BlockedBy = appendUnique(t.BlockedBy, ids...) }
}

// WithMetadata merges metadata into the task. Nil values delete keys.
func WithMetadata(meta map[string]any) TaskUpdateOption {
	return func(t *Task) {
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		for k, v := range meta {
			if v == nil {
				delete(t.Metadata, k)
			} else {
				t.Metadata[k] = v
			}
		}
	}
}

// Update applies options to an existing task and maintains reciprocal
// Blocks/BlockedBy relationships.
func (tb *TaskBoard) Update(id string, opts ...TaskUpdateOption) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	current, err := tb.getLocked(id)
	if err != nil {
		return err
	}

	desired := tb.copyTask(current)
	for _, opt := range opts {
		opt(desired)
	}

	updateReq := orchestrator.UpdateTaskRequest{ID: id}
	if desired.Subject != current.Subject {
		updateReq.Subject = &desired.Subject
	}
	if desired.Description != current.Description {
		updateReq.Description = &desired.Description
	}
	if added := addedStrings(current.Blocks, desired.Blocks); len(added) > 0 {
		updateReq.AddBlocks = added
	}
	if added := addedStrings(current.BlockedBy, desired.BlockedBy); len(added) > 0 {
		updateReq.AddBlockedBy = added
	}
	if delta := metadataDelta(current.Metadata, desired.Metadata); len(delta) > 0 {
		updateReq.Metadata = delta
	}
	if updateReq.Subject != nil || updateReq.Description != nil || len(updateReq.AddBlocks) > 0 || len(updateReq.AddBlockedBy) > 0 || len(updateReq.Metadata) > 0 {
		if _, err := tb.tasks.UpdateTask(context.Background(), updateReq); err != nil {
			return tb.translateTaskError(id, err)
		}
	}

	activeLease, _ := tb.activeLeaseLocked(id)
	statusChanged := desired.Status != current.Status

	if desired.Owner != current.Owner {
		if desired.Owner == "" {
			delete(tb.ownerOverrides, id)
		} else if activeLease == nil {
			tb.ownerOverrides[id] = desired.Owner
		} else if activeLease.WorkerID != desired.Owner {
			return fmt.Errorf("task %q already owned by %q", id, activeLease.WorkerID)
		}
	}

	if !statusChanged {
		return nil
	}

	switch desired.Status {
	case TaskBlocked:
		if activeLease != nil {
			if err := tb.releaseLeaseLocked(id, activeLease.Token); err != nil {
				return err
			}
			activeLease = nil
		}
		tb.blockedOverrides[id] = true
	case TaskPending:
		delete(tb.blockedOverrides, id)
		if activeLease != nil {
			if err := tb.releaseLeaseLocked(id, activeLease.Token); err != nil {
				return err
			}
			activeLease = nil
		}
	case TaskInProgress:
		delete(tb.blockedOverrides, id)
		owner := desired.Owner
		if owner == "" {
			if activeLease != nil {
				owner = activeLease.WorkerID
			} else {
				owner = tb.ownerOverrides[id]
			}
		}
		if owner == "" {
			return fmt.Errorf("task %q requires an owner", id)
		}
		if activeLease == nil {
			if _, err := tb.claimLocked(id, owner); err != nil {
				return err
			}
		} else if activeLease.WorkerID != owner {
			return fmt.Errorf("task %q already owned by %q", id, activeLease.WorkerID)
		}
		delete(tb.ownerOverrides, id)
	case TaskCompleted:
		delete(tb.blockedOverrides, id)
		owner := desired.Owner
		if owner == "" {
			if activeLease != nil {
				owner = activeLease.WorkerID
			} else if tb.ownerOverrides[id] != "" {
				owner = tb.ownerOverrides[id]
			} else {
				owner = "team"
			}
		}
		if activeLease == nil {
			claim, err := tb.claimLocked(id, owner)
			if err != nil {
				return err
			}
			activeLease = claim.Lease
		}
		if _, err := tb.tasks.CompleteTask(context.Background(), id, activeLease.Token, nil, time.Now()); err != nil {
			return tb.translateTaskError(id, err)
		}
		delete(tb.ownerOverrides, id)
	}

	return nil
}

// Claim atomically assigns an unowned, unblocked task to the given owner.
func (tb *TaskBoard) Claim(id, owner string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if owner == "" {
		return errors.New("owner must not be empty")
	}
	if tb.blockedOverrides[id] {
		return fmt.Errorf("task %q is blocked", id)
	}
	if override := tb.ownerOverrides[id]; override != "" {
		return fmt.Errorf("task %q already owned by %q", id, override)
	}
	if _, leaseErr := tb.activeLeaseLocked(id); leaseErr == nil {
		task, _ := tb.tasks.GetTask(context.Background(), id)
		if task != nil && task.Run != nil && task.Run.WorkerID != "" {
			return fmt.Errorf("task %q already owned by %q", id, task.Run.WorkerID)
		}
		return fmt.Errorf("task %q already owned", id)
	}
	_, err := tb.claimLocked(id, owner)
	return err
}

// Available returns all tasks that are pending, unowned, and unblocked.
func (tb *TaskBoard) Available() []*Task {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	rawTasks, err := tb.tasks.ListTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		return nil
	}

	var result []*Task
	for _, raw := range rawTasks {
		task := tb.convertTaskLocked(raw)
		if task.Status != TaskPending || task.Owner != "" {
			continue
		}
		if tb.isBlockedLocked(raw.ID, raw.BlockedBy) {
			continue
		}
		result = append(result, task)
	}
	return result
}

// Delete removes a task from the board.
func (tb *TaskBoard) Delete(id string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if err := tb.tasks.DeleteTask(context.Background(), id); err != nil {
		return tb.translateTaskError(id, err)
	}
	delete(tb.ownerOverrides, id)
	delete(tb.blockedOverrides, id)
	return nil
}

func (tb *TaskBoard) convertTaskLocked(task *orchestrator.Task) *Task {
	owner := tb.ownerOverrides[task.ID]
	if lease, err := tb.activeLeaseLocked(task.ID); err == nil && lease != nil {
		owner = lease.WorkerID
	} else if owner == "" && task.Run != nil && (task.Status == orchestrator.TaskCompleted || task.Status == orchestrator.TaskFailed) {
		owner = task.Run.WorkerID
	}

	status := TaskPending
	switch task.Status {
	case orchestrator.TaskCompleted:
		status = TaskCompleted
	case orchestrator.TaskRunning:
		status = TaskInProgress
	case orchestrator.TaskFailed:
		status = TaskBlocked
	default:
		if tb.blockedOverrides[task.ID] {
			status = TaskBlocked
		}
	}

	return &Task{
		ID:          task.ID,
		Subject:     task.Subject,
		Description: task.Description,
		Status:      status,
		Owner:       owner,
		Blocks:      cloneStrings(task.Blocks),
		BlockedBy:   cloneStrings(task.BlockedBy),
		Metadata:    cloneAnyMap(task.Metadata),
	}
}

func (tb *TaskBoard) claimLocked(id, owner string) (*orchestrator.ClaimedTask, error) {
	claim, err := tb.tasks.ClaimTask(context.Background(), id, orchestrator.ClaimTaskRequest{
		WorkerID: owner,
		LeaseTTL: taskBoardClaimTTL,
		Now:      time.Now(),
	})
	if err != nil {
		return nil, tb.translateClaimError(id, owner, err)
	}
	delete(tb.ownerOverrides, id)
	delete(tb.blockedOverrides, id)
	return claim, nil
}

func (tb *TaskBoard) releaseLeaseLocked(id, leaseToken string) error {
	if err := tb.leases.ReleaseLease(context.Background(), id, leaseToken); err != nil {
		return tb.translateTaskError(id, err)
	}
	return nil
}

func (tb *TaskBoard) activeLeaseLocked(id string) (*orchestrator.Lease, error) {
	lease, err := tb.leases.GetLease(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if !lease.ExpiresAt.After(time.Now()) {
		return nil, orchestrator.ErrLeaseExpired
	}
	return lease, nil
}

func (tb *TaskBoard) isBlockedLocked(id string, blockedBy []string) bool {
	if tb.blockedOverrides[id] {
		return true
	}
	for _, blockerID := range blockedBy {
		blocker, err := tb.tasks.GetTask(context.Background(), blockerID)
		if err != nil {
			continue
		}
		if blocker.Status != orchestrator.TaskCompleted {
			return true
		}
	}
	return false
}

func (tb *TaskBoard) copyTask(t *Task) *Task {
	cp := *t
	cp.Blocks = cloneStrings(t.Blocks)
	cp.BlockedBy = cloneStrings(t.BlockedBy)
	cp.Metadata = cloneAnyMap(t.Metadata)
	return &cp
}

func (tb *TaskBoard) translateClaimError(id, owner string, err error) error {
	switch {
	case errors.Is(err, orchestrator.ErrTaskBlocked):
		return fmt.Errorf("task %q is blocked", id)
	case errors.Is(err, orchestrator.ErrTaskNotFound):
		return fmt.Errorf("task %q not found", id)
	case errors.Is(err, orchestrator.ErrNoReadyTask):
		task, getErr := tb.tasks.GetTask(context.Background(), id)
		if getErr == nil && task != nil {
			if lease, leaseErr := tb.activeLeaseLocked(id); leaseErr == nil && lease != nil {
				return fmt.Errorf("task %q already owned by %q", id, lease.WorkerID)
			}
			if override := tb.ownerOverrides[id]; override != "" {
				return fmt.Errorf("task %q already owned by %q", id, override)
			}
			if tb.isBlockedLocked(id, task.BlockedBy) {
				return fmt.Errorf("task %q is blocked", id)
			}
		}
		return fmt.Errorf("task %q not claimable for %q", id, owner)
	default:
		return err
	}
}

func (tb *TaskBoard) translateTaskError(id string, err error) error {
	switch {
	case errors.Is(err, orchestrator.ErrTaskNotFound):
		return fmt.Errorf("task %q not found", id)
	case errors.Is(err, orchestrator.ErrTaskDependencyNotFound):
		return err
	default:
		return err
	}
}

func appendUnique(slice []string, items ...string) []string {
	seen := make(map[string]bool, len(slice))
	for _, s := range slice {
		seen[s] = true
	}
	for _, item := range items {
		if !seen[item] {
			slice = append(slice, item)
			seen[item] = true
		}
	}
	return slice
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]string, len(src))
	copy(cloned, src)
	return cloned
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func addedStrings(before, after []string) []string {
	if len(after) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(before))
	for _, item := range before {
		seen[item] = struct{}{}
	}
	var added []string
	for _, item := range after {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		added = append(added, item)
	}
	return added
}

func metadataDelta(before, after map[string]any) map[string]any {
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	delta := make(map[string]any)
	seen := make(map[string]struct{}, len(before)+len(after))
	for key, value := range after {
		seen[key] = struct{}{}
		if before == nil {
			delta[key] = value
			continue
		}
		if current, ok := before[key]; !ok || !reflect.DeepEqual(current, value) {
			delta[key] = value
		}
	}
	for key := range before {
		if _, ok := seen[key]; ok {
			continue
		}
		delta[key] = nil
	}
	if len(delta) == 0 {
		return nil
	}
	return delta
}

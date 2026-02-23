package team

import (
	"fmt"
	"strconv"
	"sync"
)

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
	ID          string            `json:"id"`
	Subject     string            `json:"subject"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	Owner       string            `json:"owner,omitempty"`
	Blocks      []string          `json:"blocks,omitempty"`
	BlockedBy   []string          `json:"blocked_by,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
}

// TaskBoard is a mutex-protected shared task list for team coordination.
type TaskBoard struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	nextID int
}

// NewTaskBoard creates an empty task board.
func NewTaskBoard() *TaskBoard {
	return &TaskBoard{
		tasks: make(map[string]*Task),
	}
}

// Create adds a new task and returns its ID.
func (tb *TaskBoard) Create(subject, description string) string {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.nextID++
	id := strconv.Itoa(tb.nextID)
	tb.tasks[id] = &Task{
		ID:          id,
		Subject:     subject,
		Description: description,
		Status:      TaskPending,
	}
	return id
}

// Get returns a copy of the task with the given ID.
func (tb *TaskBoard) Get(id string) (*Task, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	t, ok := tb.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return tb.copyTask(t), nil
}

// copyTask returns a deep copy of a task. Must be called with at least a read lock held.
func (tb *TaskBoard) copyTask(t *Task) *Task {
	cp := *t
	cp.Blocks = append([]string(nil), t.Blocks...)
	cp.BlockedBy = append([]string(nil), t.BlockedBy...)
	if t.Metadata != nil {
		cp.Metadata = make(map[string]any, len(t.Metadata))
		for k, v := range t.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

// List returns copies of all tasks.
func (tb *TaskBoard) List() []*Task {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make([]*Task, 0, len(tb.tasks))
	for _, t := range tb.tasks {
		result = append(result, tb.copyTask(t))
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

	t, ok := tb.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}

	// Snapshot current Blocks/BlockedBy to detect additions.
	oldBlocks := make(map[string]bool, len(t.Blocks))
	for _, b := range t.Blocks {
		oldBlocks[b] = true
	}
	oldBlockedBy := make(map[string]bool, len(t.BlockedBy))
	for _, b := range t.BlockedBy {
		oldBlockedBy[b] = true
	}

	for _, o := range opts {
		o(t)
	}

	// Maintain reciprocal relationships for newly added Blocks entries.
	// If task A says it blocks task B, then B.BlockedBy should contain A.
	for _, b := range t.Blocks {
		if !oldBlocks[b] {
			if blocked, exists := tb.tasks[b]; exists {
				blocked.BlockedBy = appendUnique(blocked.BlockedBy, id)
			}
		}
	}

	// Maintain reciprocal relationships for newly added BlockedBy entries.
	// If task B says it's blocked by task A, then A.Blocks should contain B.
	for _, b := range t.BlockedBy {
		if !oldBlockedBy[b] {
			if blocker, exists := tb.tasks[b]; exists {
				blocker.Blocks = appendUnique(blocker.Blocks, id)
			}
		}
	}

	return nil
}

// Claim atomically assigns an unowned, unblocked task to the given owner.
func (tb *TaskBoard) Claim(id, owner string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	t, ok := tb.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if t.Owner != "" {
		return fmt.Errorf("task %q already owned by %q", id, t.Owner)
	}
	if tb.isBlocked(t) {
		return fmt.Errorf("task %q is blocked", id)
	}
	t.Owner = owner
	t.Status = TaskInProgress
	return nil
}

// Available returns all tasks that are pending, unowned, and unblocked.
func (tb *TaskBoard) Available() []*Task {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var result []*Task
	for _, t := range tb.tasks {
		if t.Status == TaskPending && t.Owner == "" && !tb.isBlocked(t) {
			result = append(result, tb.copyTask(t))
		}
	}
	return result
}

// Delete removes a task from the board.
func (tb *TaskBoard) Delete(id string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if _, ok := tb.tasks[id]; !ok {
		return fmt.Errorf("task %q not found", id)
	}
	delete(tb.tasks, id)
	// Clean up references in other tasks.
	for _, t := range tb.tasks {
		t.Blocks = removeString(t.Blocks, id)
		t.BlockedBy = removeString(t.BlockedBy, id)
	}
	return nil
}

// isBlocked returns true if any of the task's blockers are still incomplete.
// Must be called with at least a read lock held.
func (tb *TaskBoard) isBlocked(t *Task) bool {
	for _, bid := range t.BlockedBy {
		if blocker, ok := tb.tasks[bid]; ok && blocker.Status != TaskCompleted {
			return true
		}
	}
	return false
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

func removeString(slice []string, s string) []string {
	result := slice[:0]
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

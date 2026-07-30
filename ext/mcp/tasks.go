package mcp

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"
)

// ResultTypeTask is the resultType value for a CreateTaskResult, indicating the
// request was converted into a long-running task the client polls via tasks/*.
const ResultTypeTask = "task"

// Task status values. Working and input_required are non-terminal; completed,
// failed, and cancelled are terminal.
const (
	TaskStatusWorking       = "working"
	TaskStatusInputRequired = "input_required"
	TaskStatusCompleted     = "completed"
	TaskStatusFailed        = "failed"
	TaskStatusCancelled     = "cancelled"
)

// Task is the unit of long-running work surfaced by the tasks extension. On
// completion, Result holds what the original request would have returned; on
// failure, Error holds a JSON-RPC error; on input_required, InputRequests holds
// the inputs the client must satisfy via tasks/update.
type Task struct {
	TaskID         string          `json:"taskId"`
	Status         string          `json:"status"`
	TTLMs          *int64          `json:"ttlMs,omitempty"`
	PollIntervalMs *int64          `json:"pollIntervalMs,omitempty"`
	StatusMessage  string          `json:"statusMessage,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          *jsonRPCError   `json:"error,omitempty"`
	InputRequests  InputRequests   `json:"inputRequests,omitempty"`
}

// CreateTaskResult is returned with resultType "task" when a request is
// converted into a long-running task.
type CreateTaskResult struct {
	ResultType string `json:"resultType"`
	Task       Task   `json:"task"`
}

// taskEntry is the server-side store entry for a task. It guards the task with
// a mutex and accumulates input responses submitted via tasks/update.
type taskEntry struct {
	mu              sync.Mutex
	task            Task
	pendingInputs   InputResponses
	cancelRequested bool
}

func (e *taskEntry) snapshot() Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	clone := e.task
	if e.cancelRequested && !isTerminalStatus(clone.Status) {
		clone.Status = TaskStatusCancelled
	}
	return clone
}

func (e *taskEntry) update(mutate func(*Task)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	mutate(&e.task)
}

func (e *taskEntry) applyInputResponses(responses InputResponses) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pendingInputs == nil {
		e.pendingInputs = InputResponses{}
	}
	for k, v := range responses {
		e.pendingInputs[k] = v
	}
}

// PendingInputs returns a copy of the input responses submitted so far. The
// application advances the task by consuming these in its work loop.
func (e *taskEntry) PendingInputs() InputResponses {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(InputResponses, len(e.pendingInputs))
	for k, v := range e.pendingInputs {
		out[k] = v
	}
	return out
}

func (e *taskEntry) requestCancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancelRequested = true
}

func isTerminalStatus(status string) bool {
	switch status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// taskStore is the in-memory backing store for the tasks extension. It is
// keyed by taskId. A TTL sweeper is intentionally omitted; the store is
// per-Server and cleared when the server closes.
type taskStore struct {
	mu    sync.Mutex
	tasks map[string]*taskEntry
}

func newTaskStore() *taskStore {
	return &taskStore{tasks: make(map[string]*taskEntry)}
}

func (ts *taskStore) put(e *taskEntry) {
	ts.mu.Lock()
	ts.tasks[e.task.TaskID] = e
	ts.mu.Unlock()
}

func (ts *taskStore) get(id string) (*taskEntry, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	e, ok := ts.tasks[id]
	return e, ok
}

// newTaskID mints a crypto-random task id. The id is opaque to the client and
// unguessable so a client cannot poll arbitrary task ids.
func newTaskID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fall back to a counter if the system RNG is unavailable so task
		// creation never fails.
		return fallbackTaskID()
	}
	return "task_" + hex.EncodeToString(buf[:])
}

var fallbackTaskCounter atomic.Uint64

func fallbackTaskID() string {
	n := fallbackTaskCounter.Add(1)
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[:8], n)
	return "task_" + hex.EncodeToString(buf[:])
}

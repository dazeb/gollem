package core

import (
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrSteerQueueClosed       = errors.New("core: steer queue is closed")
	ErrSteerQueueFull         = errors.New("core: steer queue is full")
	ErrSteerMessageEmpty      = errors.New("core: steer message is empty")
	ErrSteeringNeedsStreaming = errors.New("core: steering requires RunStream")
)

// SteerQueueMaxPending bounds instructions waiting for a safe request boundary.
const SteerQueueMaxPending = 64

// SteerMessage is one user instruction queued for the next safe model-request
// boundary of an active streaming run.
type SteerMessage struct {
	ID       string
	Text     string
	QueuedAt time.Time
}

// SteerQueueHooks let a runtime prepare a durable request boundary, acknowledge
// when an instruction enters that request, or reject it when the run ends first.
type SteerQueueHooks struct {
	OnPrepared func([]SteerMessage) error
	OnConsumed func(SteerMessage) error
	OnRejected func(SteerMessage, error)
}

// SteerQueue accepts concurrent steering instructions for one streaming run.
// The agent owns draining and terminal closure.
type SteerQueue struct {
	mu      sync.Mutex
	pending []SteerMessage
	closed  bool
	hooks   SteerQueueHooks
}

func NewSteerQueue(hooks SteerQueueHooks) *SteerQueue {
	return &SteerQueue{hooks: hooks}
}

func (q *SteerQueue) Enqueue(message SteerMessage) error {
	if q == nil {
		return ErrSteerQueueClosed
	}
	message.Text = strings.TrimSpace(message.Text)
	if message.Text == "" {
		return ErrSteerMessageEmpty
	}
	if message.QueuedAt.IsZero() {
		message.QueuedAt = time.Now().UTC()
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrSteerQueueClosed
	}
	if len(q.pending) >= SteerQueueMaxPending {
		return ErrSteerQueueFull
	}
	q.pending = append(q.pending, message)
	return nil
}

// RejectAll closes the queue and rejects every instruction that has not
// entered a model request. It is intended for runtime setup failures before an
// agent stream takes ownership.
func (q *SteerQueue) RejectAll(reason error) {
	q.close(reason)
}

func (q *SteerQueue) take(terminal bool) []SteerMessage {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	if len(q.pending) == 0 {
		if terminal {
			q.closed = true
		}
		return nil
	}
	messages := append([]SteerMessage(nil), q.pending...)
	q.pending = nil
	return messages
}

func (q *SteerQueue) consumed(messages []SteerMessage) error {
	if q == nil || len(messages) == 0 || q.hooks.OnConsumed == nil {
		return nil
	}
	for i, message := range messages {
		if err := q.hooks.OnConsumed(message); err != nil {
			q.reject(messages[i:], err)
			return err
		}
	}
	return nil
}

func (q *SteerQueue) prepared(messages []SteerMessage) error {
	if q == nil || len(messages) == 0 || q.hooks.OnPrepared == nil {
		return nil
	}
	if err := q.hooks.OnPrepared(append([]SteerMessage(nil), messages...)); err != nil {
		q.reject(messages, err)
		return err
	}
	return nil
}

func (q *SteerQueue) reject(messages []SteerMessage, reason error) {
	if q == nil || len(messages) == 0 || q.hooks.OnRejected == nil {
		return
	}
	for _, message := range messages {
		q.hooks.OnRejected(message, reason)
	}
}

func (q *SteerQueue) close(reason error) {
	if q == nil {
		return
	}
	if reason == nil {
		reason = ErrSteerQueueClosed
	}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	pending := append([]SteerMessage(nil), q.pending...)
	q.pending = nil
	q.mu.Unlock()
	q.reject(pending, reason)
}

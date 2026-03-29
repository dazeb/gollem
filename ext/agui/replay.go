package agui

import "sync"

// EventBuffer is an in-memory replay buffer that stores events for reconnect
// replay. When a client reconnects with a last_seq, the buffer can replay
// all events after that sequence number.
//
// For production use with durable sessions (Temporal), this should be backed
// by a persistent store. This in-memory implementation is suitable for
// in-process core runs.
type EventBuffer struct {
	mu     sync.RWMutex
	events []Event
	cap    int
}

// NewEventBuffer creates a replay buffer with the given capacity.
// When capacity is exceeded, the oldest events are dropped.
func NewEventBuffer(capacity int) *EventBuffer {
	if capacity <= 0 {
		capacity = 10000
	}
	return &EventBuffer{
		events: make([]Event, 0, min(capacity, 256)),
		cap:    capacity,
	}
}

// Append adds an event to the buffer. If the buffer is at capacity,
// the oldest event is dropped.
func (b *EventBuffer) Append(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.events) >= b.cap {
		// Drop oldest 10% to avoid frequent shifts
		drop := b.cap / 10
		if drop < 1 {
			drop = 1
		}
		copy(b.events, b.events[drop:])
		newLen := len(b.events) - drop
		// Zero out stale references so GC can collect evicted event payloads.
		var zero Event
		for i := newLen; i < len(b.events); i++ {
			b.events[i] = zero
		}
		b.events = b.events[:newLen]
	}

	b.events = append(b.events, ev)
}

// Since returns all events with sequence > lastSeq, in order.
// The boolean return indicates whether the replay is complete:
//   - true: all events after lastSeq are returned (no gap).
//   - false: some events were evicted from the buffer. The caller
//     should send a session.snapshot instead of replaying.
//
// Replay is defined over normalized session sequences, which also map directly
// to SSE `id` values on reconnectable transports.
//
// When lastSeq is 0 (fresh client), all buffered events are returned with
// complete=true. This means the client accepts whatever history is available;
// if events were evicted before the client connected, those are simply not
// available and no gap is reported. Use LastSeq() == 0 && Len() > 0 to detect
// whether the buffer has been active before a fresh client connects.
//
// The transport must perform an atomic replay-to-live handoff around this call:
// attach a live subscriber first, capture a replay high-water mark from the
// same session/log lock, replay (lastSeq, highWatermark], then drain any queued
// live events with sequence > highWatermark. If Since reports complete=false,
// skip partial replay and send a snapshot whose snapshot_sequence comes from
// the same transaction used to capture snapshot contents.
//
// Returns (nil, true) if no new events exist.
func (b *EventBuffer) Since(lastSeq uint64) (events []Event, complete bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.events) == 0 {
		return nil, true
	}

	// If lastSeq is before the oldest buffered event, we have a gap.
	oldestSeq := b.events[0].Sequence
	if lastSeq > 0 && lastSeq < oldestSeq-1 {
		// Gap detected: events between lastSeq and oldestSeq were evicted.
		return nil, false
	}

	// Binary search for the first event after lastSeq.
	lo, hi := 0, len(b.events)
	for lo < hi {
		mid := (lo + hi) / 2
		if b.events[mid].Sequence <= lastSeq {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	if lo >= len(b.events) {
		return nil, true
	}

	result := make([]Event, len(b.events)-lo)
	copy(result, b.events[lo:])
	return result, true
}

// All returns all buffered events in order.
func (b *EventBuffer) All() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]Event, len(b.events))
	copy(result, b.events)
	return result
}

// Len returns the number of buffered events.
func (b *EventBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}

// LastSeq returns the sequence number of the most recent event, or 0 if empty.
func (b *EventBuffer) LastSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.events) == 0 {
		return 0
	}
	return b.events[len(b.events)-1].Sequence
}

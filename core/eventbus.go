package core

import (
	"reflect"
	"sync"
)

// EventBus provides typed publish-subscribe for agent coordination.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[reflect.Type][]subscriberEntry
	nextID      int

	queueMu sync.Mutex
	queueCV *sync.Cond
	queue   []queuedEvent
	closed  bool
	done    chan struct{}
}

type queuedEvent struct {
	subs []subscriberEntry
	arg  reflect.Value
}

type subscriberEntry struct {
	id int
	fn reflect.Value
}

// NewEventBus creates a new event bus. Callers should invoke Close when the
// bus is no longer needed to stop the background dispatch goroutine.
func NewEventBus() *EventBus {
	bus := &EventBus{
		subscribers: make(map[reflect.Type][]subscriberEntry),
		done:        make(chan struct{}),
	}
	bus.queueCV = sync.NewCond(&bus.queueMu)
	go bus.dispatchLoop()
	return bus
}

// Close signals the dispatch loop to drain its queue and exit. Subsequent
// PublishAsync calls become no-ops. Safe to call more than once. Returns
// after the dispatch goroutine has exited.
func (bus *EventBus) Close() {
	bus.queueMu.Lock()
	if bus.closed {
		bus.queueMu.Unlock()
		<-bus.done
		return
	}
	bus.closed = true
	bus.queueCV.Broadcast()
	bus.queueMu.Unlock()
	<-bus.done
}

// Subscribe registers a handler for events of a specific type.
// Returns an unsubscribe function.
func Subscribe[E any](bus *EventBus, handler func(E)) func() {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	var zero E
	eventType := reflect.TypeOf(zero)

	id := bus.nextID
	bus.nextID++

	bus.subscribers[eventType] = append(bus.subscribers[eventType], subscriberEntry{
		id: id,
		fn: reflect.ValueOf(handler),
	})

	return func() {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		subs := bus.subscribers[eventType]
		for i, s := range subs {
			if s.id == id {
				bus.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
}

// Publish sends an event to all matching subscribers synchronously.
func Publish[E any](bus *EventBus, event E) {
	subs, arg := snapshotSubscribers(bus, event)
	if len(subs) == 0 {
		return
	}
	dispatchSubscribers(subs, arg)
}

// PublishAsync sends an event to subscribers asynchronously via the bus queue.
// Delivery preserves async event order per bus but does not wait for handlers to run.
func PublishAsync[E any](bus *EventBus, event E) {
	subs, arg := snapshotSubscribers(bus, event)
	if len(subs) == 0 {
		return
	}
	bus.enqueue(queuedEvent{
		subs: subs,
		arg:  arg,
	})
}

func snapshotSubscribers[E any](bus *EventBus, event E) ([]subscriberEntry, reflect.Value) {
	var zero E
	eventType := reflect.TypeOf(zero)

	bus.mu.RLock()
	subs := make([]subscriberEntry, len(bus.subscribers[eventType]))
	copy(subs, bus.subscribers[eventType])
	bus.mu.RUnlock()

	return subs, reflect.ValueOf(event)
}

func (bus *EventBus) enqueue(event queuedEvent) {
	bus.queueMu.Lock()
	if bus.closed {
		bus.queueMu.Unlock()
		return
	}
	bus.queue = append(bus.queue, event)
	bus.queueCV.Signal()
	bus.queueMu.Unlock()
}

func (bus *EventBus) dispatchLoop() {
	defer close(bus.done)
	for {
		event, ok := bus.dequeue()
		if !ok {
			return
		}
		panicValue := dispatchEvent(event)
		if panicValue != nil {
			go func(v any) {
				panic(v)
			}(panicValue)
		}
	}
}

// dequeue returns the next queued event. The second return is false when
// the bus has been closed and the queue is empty — the dispatch loop uses
// this to exit.
func (bus *EventBus) dequeue() (queuedEvent, bool) {
	bus.queueMu.Lock()
	defer bus.queueMu.Unlock()

	for len(bus.queue) == 0 {
		if bus.closed {
			return queuedEvent{}, false
		}
		bus.queueCV.Wait()
	}

	event := bus.queue[0]
	copy(bus.queue, bus.queue[1:])
	bus.queue[len(bus.queue)-1] = queuedEvent{}
	bus.queue = bus.queue[:len(bus.queue)-1]
	return event, true
}

func dispatchEvent(event queuedEvent) (panicValue any) {
	defer func() {
		panicValue = recover()
	}()

	dispatchSubscribers(event.subs, event.arg)
	return nil
}

func dispatchSubscribers(subs []subscriberEntry, arg reflect.Value) {
	args := []reflect.Value{arg}
	for _, sub := range subs {
		sub.fn.Call(args)
	}
}

// WithEventBus attaches an event bus to the agent, accessible via RunContext.
func WithEventBus[T any](bus *EventBus) AgentOption[T] {
	return func(a *Agent[T]) {
		a.eventBus = bus
	}
}

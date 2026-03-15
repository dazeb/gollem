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
}

type queuedEvent struct {
	subs []subscriberEntry
	arg  reflect.Value
}

type subscriberEntry struct {
	id int
	fn reflect.Value
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	bus := &EventBus{
		subscribers: make(map[reflect.Type][]subscriberEntry),
	}
	bus.queueCV = sync.NewCond(&bus.queueMu)
	go bus.dispatchLoop()
	return bus
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
	bus.queue = append(bus.queue, event)
	bus.queueCV.Signal()
	bus.queueMu.Unlock()
}

func (bus *EventBus) dispatchLoop() {
	for {
		event := bus.dequeue()
		panicValue := dispatchEvent(event)
		if panicValue != nil {
			go func(v any) {
				panic(v)
			}(panicValue)
		}
	}
}

func (bus *EventBus) dequeue() queuedEvent {
	bus.queueMu.Lock()
	defer bus.queueMu.Unlock()

	for len(bus.queue) == 0 {
		bus.queueCV.Wait()
	}

	event := bus.queue[0]
	copy(bus.queue, bus.queue[1:])
	bus.queue[len(bus.queue)-1] = queuedEvent{}
	bus.queue = bus.queue[:len(bus.queue)-1]
	return event
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

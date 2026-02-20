package gollem

import (
	"reflect"
	"sync"
)

// EventBus provides typed publish-subscribe for agent coordination.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[reflect.Type][]subscriberEntry
	nextID      int
}

type subscriberEntry struct {
	id int
	fn reflect.Value
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[reflect.Type][]subscriberEntry),
	}
}

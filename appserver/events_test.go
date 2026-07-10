package appserver

import (
	"encoding/json"
	"testing"
)

func TestEventQueueDrainFiltersOptedOutNotifications(t *testing.T) {
	queue := NewEventQueue()
	queue.Publish("fs/changed", map[string]any{"path": "a.txt"})
	queue.Publish("process/outputDelta", map[string]any{"id": "proc-1"})

	events := queue.Drain(func(method string) bool {
		return method != "process/outputDelta"
	})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one filtered notification", events)
	}
	if events[0].Method != "fs/changed" {
		t.Fatalf("event method = %q, want fs/changed", events[0].Method)
	}
	var params map[string]any
	if err := json.Unmarshal(events[0].Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params["path"] != "a.txt" {
		t.Fatalf("params = %#v", params)
	}

	if remaining := queue.Drain(nil); len(remaining) != 0 {
		t.Fatalf("queue was not drained: %#v", remaining)
	}
}

func TestEventQueueSignalsPublish(t *testing.T) {
	queue := NewEventQueue()
	queue.Publish("fs/changed", nil)
	select {
	case <-queue.Signal():
	default:
		t.Fatal("publish did not signal")
	}
}

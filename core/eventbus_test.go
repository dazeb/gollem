package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testEvent struct {
	Value string
}

type otherEvent struct {
	Count int
}

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()

	var received testEvent
	Subscribe(bus, func(e testEvent) {
		received = e
	})

	Publish(bus, testEvent{Value: "hello"})

	if received.Value != "hello" {
		t.Errorf("expected 'hello', got %q", received.Value)
	}
}

func TestEventBus_TypeSafety(t *testing.T) {
	bus := NewEventBus()

	var testReceived bool
	var otherReceived bool

	Subscribe(bus, func(_ testEvent) {
		testReceived = true
	})
	Subscribe(bus, func(_ otherEvent) {
		otherReceived = true
	})

	Publish(bus, testEvent{Value: "test"})

	if !testReceived {
		t.Error("testEvent handler should have been called")
	}
	if otherReceived {
		t.Error("otherEvent handler should NOT have been called")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()

	var count atomic.Int32
	Subscribe(bus, func(_ testEvent) { count.Add(1) })
	Subscribe(bus, func(_ testEvent) { count.Add(1) })
	Subscribe(bus, func(_ testEvent) { count.Add(1) })

	Publish(bus, testEvent{Value: "multi"})

	if count.Load() != 3 {
		t.Errorf("expected 3 handlers called, got %d", count.Load())
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()

	var count atomic.Int32
	unsub := Subscribe(bus, func(_ testEvent) { count.Add(1) })

	Publish(bus, testEvent{Value: "before"})
	if count.Load() != 1 {
		t.Fatalf("expected 1, got %d", count.Load())
	}

	unsub()
	Publish(bus, testEvent{Value: "after"})
	if count.Load() != 1 {
		t.Errorf("expected 1 (unsubscribed), got %d", count.Load())
	}
}

func TestEventBus_Async(t *testing.T) {
	bus := NewEventBus()

	var wg sync.WaitGroup
	wg.Add(1)

	var received atomic.Bool
	Subscribe(bus, func(_ testEvent) {
		received.Store(true)
		wg.Done()
	})

	PublishAsync(bus, testEvent{Value: "async"})

	// Wait for the async handler.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !received.Load() {
			t.Error("expected async handler to fire")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async handler")
	}
}

func TestEventBus_ConcurrentSafe(t *testing.T) {
	bus := NewEventBus()

	var wg sync.WaitGroup

	// Concurrent subscribes.
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := Subscribe(bus, func(_ testEvent) {})
			time.Sleep(time.Millisecond)
			unsub()
		}()
	}

	// Concurrent publishes.
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Publish(bus, testEvent{Value: "concurrent"})
		}()
	}

	wg.Wait()
}

func TestEventBus_NoSubscribers(t *testing.T) {
	bus := NewEventBus()
	// Publishing without subscribers should not panic.
	Publish(bus, testEvent{Value: "no one listening"})
	PublishAsync(bus, testEvent{Value: "async no one"})
}

func TestEventBus_AgentIntegration(t *testing.T) {
	bus := NewEventBus()

	var startEvent RunStartedEvent
	var toolEvent ToolCalledEvent
	var completeEvent RunCompletedEvent

	Subscribe(bus, func(e RunStartedEvent) { startEvent = e })
	Subscribe(bus, func(e ToolCalledEvent) { toolEvent = e })
	Subscribe(bus, func(e RunCompletedEvent) { completeEvent = e })

	type Params struct {
		N int `json:"n"`
	}
	var busFromTool *EventBus
	tool := FuncTool[Params]("echo", "echo", func(ctx context.Context, rc *RunContext, p Params) (string, error) {
		busFromTool = rc.EventBus
		return "echoed", nil
	})

	model := NewTestModel(
		ToolCallResponse("echo", `{"n":1}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithEventBus[string](bus),
	)

	_, err := agent.Run(context.Background(), "test event bus")
	if err != nil {
		t.Fatal(err)
	}

	if startEvent.Prompt != "test event bus" {
		t.Errorf("expected RunStartedEvent with prompt 'test event bus', got %q", startEvent.Prompt)
	}
	if startEvent.RunID == "" {
		t.Error("expected RunStartedEvent to include RunID")
	}
	if startEvent.ParentRunID != "" {
		t.Errorf("expected top-level run to have empty ParentRunID, got %q", startEvent.ParentRunID)
	}
	if startEvent.StartedAt.IsZero() {
		t.Error("expected RunStartedEvent to include StartedAt")
	}
	if toolEvent.ToolName != "echo" {
		t.Errorf("expected ToolCalledEvent with tool 'echo', got %q", toolEvent.ToolName)
	}
	if toolEvent.ToolCallID == "" {
		t.Error("expected ToolCalledEvent to include ToolCallID")
	}
	if toolEvent.RunID != startEvent.RunID {
		t.Errorf("expected tool event RunID %q, got %q", startEvent.RunID, toolEvent.RunID)
	}
	if toolEvent.CalledAt.IsZero() {
		t.Error("expected ToolCalledEvent to include CalledAt")
	}
	if !completeEvent.Success {
		t.Error("expected RunCompletedEvent with Success=true")
	}
	if completeEvent.RunID != startEvent.RunID {
		t.Errorf("expected completion RunID %q, got %q", startEvent.RunID, completeEvent.RunID)
	}
	if !completeEvent.StartedAt.Equal(startEvent.StartedAt) {
		t.Errorf("expected completion StartedAt %v, got %v", startEvent.StartedAt, completeEvent.StartedAt)
	}
	if completeEvent.CompletedAt.IsZero() {
		t.Error("expected RunCompletedEvent to include CompletedAt")
	}
	if busFromTool != bus {
		t.Error("expected EventBus to be accessible via RunContext in tool")
	}
}

func TestEventBus_ChildRunCarriesParentRunID(t *testing.T) {
	bus := NewEventBus()

	var (
		mu          sync.Mutex
		startEvents []RunStartedEvent
	)
	Subscribe(bus, func(e RunStartedEvent) {
		mu.Lock()
		defer mu.Unlock()
		startEvents = append(startEvents, e)
	})

	child := NewAgent[string](
		NewTestModel(TextResponse("child complete")),
		WithEventBus[string](bus),
	)
	delegate := FuncTool[struct{}]("delegate", "delegate to child", func(ctx context.Context, _ *RunContext, _ struct{}) (string, error) {
		result, err := child.Run(ctx, "child task")
		if err != nil {
			return "", err
		}
		return result.Output, nil
	})
	parent := NewAgent[string](
		NewTestModel(
			ToolCallResponse("delegate", `{}`),
			TextResponse("parent complete"),
		),
		WithTools[string](delegate),
		WithEventBus[string](bus),
	)

	if _, err := parent.Run(context.Background(), "parent task"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(startEvents) != 2 {
		t.Fatalf("expected 2 RunStartedEvents, got %d", len(startEvents))
	}
	parentStart := startEvents[0]
	childStart := startEvents[1]
	if parentStart.ParentRunID != "" {
		t.Fatalf("expected top-level run ParentRunID to be empty, got %q", parentStart.ParentRunID)
	}
	if childStart.ParentRunID != parentStart.RunID {
		t.Fatalf("expected child ParentRunID %q, got %q", parentStart.RunID, childStart.ParentRunID)
	}
}

func TestEventBus_IterPublishesLifecycleEvents(t *testing.T) {
	bus := NewEventBus()

	var (
		startCount    atomic.Int32
		completeCount atomic.Int32
	)
	Subscribe(bus, func(RunStartedEvent) {
		startCount.Add(1)
	})
	Subscribe(bus, func(RunCompletedEvent) {
		completeCount.Add(1)
	})

	agent := NewAgent[string](
		NewTestModel(TextResponse("iter complete")),
		WithEventBus[string](bus),
	)
	iter := agent.Iter(context.Background(), "iter task")
	for !iter.Done() {
		if _, err := iter.Next(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := iter.Result(); err != nil {
		t.Fatal(err)
	}

	if startCount.Load() != 1 {
		t.Fatalf("expected 1 RunStartedEvent, got %d", startCount.Load())
	}
	if completeCount.Load() != 1 {
		t.Fatalf("expected 1 RunCompletedEvent, got %d", completeCount.Load())
	}
}

func TestEventBus_RunStreamDoesNotPublishStartOnBootstrapFailure(t *testing.T) {
	bus := NewEventBus()

	var startCount atomic.Int32
	Subscribe(bus, func(RunStartedEvent) {
		startCount.Add(1)
	})

	agent := NewAgent[string](
		NewTestModel(TextResponse("unused")),
		WithEventBus[string](bus),
		WithDynamicSystemPrompt[string](func(context.Context, *RunContext) (string, error) {
			return "", errors.New("dynamic prompt failed")
		}),
	)

	if _, err := agent.RunStream(context.Background(), "broken"); err == nil {
		t.Fatal("expected RunStream to fail during bootstrap")
	}
	if startCount.Load() != 0 {
		t.Fatalf("expected no RunStartedEvent on bootstrap failure, got %d", startCount.Load())
	}
}

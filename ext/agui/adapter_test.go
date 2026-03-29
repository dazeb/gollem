package agui

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// collectEvents subscribes to an adapter and collects all emitted JSON events.
func collectEvents(a *Adapter) *eventCollector {
	c := &eventCollector{}
	a.OnEvent(func(data json.RawMessage) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.events = append(c.events, data)
	})
	return c
}

type eventCollector struct {
	mu     sync.Mutex
	events []json.RawMessage
}

func (c *eventCollector) all() []json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]json.RawMessage, len(c.events))
	copy(cp, c.events)
	return cp
}

func parseType(data json.RawMessage) string {
	var m map[string]any
	json.Unmarshal(data, &m)
	if t, ok := m["type"].(string); ok {
		return t
	}
	return ""
}

func parseField(data json.RawMessage, field string) any {
	var m map[string]any
	json.Unmarshal(data, &m)
	return m[field]
}

// ── Adapter lifecycle tests ─────────────────────────────────────────

func TestAdapter_RunStartedFinished(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{
		RunID: "run_1", ParentRunID: "", Prompt: "hello", StartedAt: time.Now(),
	})
	core.Publish(bus, core.RunCompletedEvent{
		RunID: "run_1", Success: true, CompletedAt: time.Now(),
	})

	adapter.Close()

	events := c.all()
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	if got := parseType(events[0]); got != AGUIRunStarted {
		t.Errorf("event[0] type = %q, want %q", got, AGUIRunStarted)
	}
	if got := parseField(events[0], "threadId"); got != "thread_1" {
		t.Errorf("event[0] threadId = %v, want %q", got, "thread_1")
	}
	if got := parseField(events[0], "runId"); got != "run_1" {
		t.Errorf("event[0] runId = %v, want %q", got, "run_1")
	}

	last := events[len(events)-1]
	if got := parseType(last); got != AGUIRunFinished {
		t.Errorf("last event type = %q, want %q", got, AGUIRunFinished)
	}
}

func TestAdapter_RunError(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{
		RunID: "run_1", Success: false, Error: "boom", CompletedAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	last := events[len(events)-1]
	if got := parseType(last); got != AGUIRunError {
		t.Errorf("last event type = %q, want %q", got, AGUIRunError)
	}
	if got := parseField(last, "message"); got != "boom" {
		t.Errorf("RunError message = %v, want %q", got, "boom")
	}
	if got := parseField(last, "runId"); got != "run_1" {
		t.Errorf("RunError runId = %v, want %q", got, "run_1")
	}
}

func TestAdapter_RunDeferredDoesNotEmitError(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "run_1", StartedAt: time.Now()})
	core.Publish(bus, core.RunWaitingEvent{RunID: "run_1", Reason: "deferred", WaitingAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{
		RunID: "run_1", Success: false, Deferred: true,
		Error:       "agent run deferred: 1 tool call(s) require external resolution",
		CompletedAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	last := events[len(events)-1]
	if got := parseType(last); got != AGUIRunFinished {
		t.Errorf("last event type = %q, want %q", got, AGUIRunFinished)
	}
}

func TestAdapter_RunResumedAndDeferredResolvedAsCustom(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunResumedEvent{RunID: "run_1", ResumedAt: time.Now()})
	core.Publish(bus, core.DeferredResolvedEvent{
		RunID: "run_1", ToolCallID: "tc1", ToolName: "search",
		Content: "done", IsError: false, ResolvedAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if got := parseType(events[0]); got != AGUICustom {
		t.Fatalf("event[0] type = %q, want %q", got, AGUICustom)
	}
	if got := parseField(events[0], "name"); got != "gollem.run.resumed" {
		t.Errorf("event[0] name = %v, want %q", got, "gollem.run.resumed")
	}
	if got := parseField(events[1], "name"); got != "gollem.deferred.resolved" {
		t.Errorf("event[1] name = %v, want %q", got, "gollem.deferred.resolved")
	}
}

// ── Step (turn) event tests ─────────────────────────────────────────

func TestAdapter_TurnMapsToStep(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, CompletedAt: time.Now()})
	adapter.Close()

	events := c.all()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if got := parseType(events[0]); got != AGUIStepStarted {
		t.Errorf("event[0] type = %q, want %q", got, AGUIStepStarted)
	}
	if got := parseField(events[0], "stepName"); got != "turn_1" {
		t.Errorf("stepName = %v, want %q", got, "turn_1")
	}
	if got := parseType(events[1]); got != AGUIStepFinished {
		t.Errorf("event[1] type = %q, want %q", got, AGUIStepFinished)
	}
}

// ── Tool call event tests ───────────────────────────────────────────

func TestAdapter_ToolCallLifecycle(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "get_weather",
		ArgsJSON: `{"city":"NYC"}`, CalledAt: time.Now(),
	})
	core.Publish(bus, core.ToolCompletedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "get_weather",
		Result: "sunny", CompletedAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	types := make([]string, len(events))
	for i, ev := range events {
		types[i] = parseType(ev)
	}

	// Expect: TOOL_CALL_START, TOOL_CALL_ARGS, TOOL_CALL_END, TOOL_CALL_RESULT
	expected := []string{AGUIToolCallStart, AGUIToolCallArgs, AGUIToolCallEnd, AGUIToolCallResult}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events %v, got %d: %v", len(expected), expected, len(types), types)
	}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d] type = %q, want %q", i, types[i], want)
		}
	}

	// Check TOOL_CALL_START fields
	if got := parseField(events[0], "toolCallId"); got != "tc1" {
		t.Errorf("toolCallId = %v, want %q", got, "tc1")
	}
	if got := parseField(events[0], "toolCallName"); got != "get_weather" {
		t.Errorf("toolCallName = %v, want %q", got, "get_weather")
	}

	// Check TOOL_CALL_RESULT fields
	if got := parseField(events[3], "content"); got != "sunny" {
		t.Errorf("result content = %v, want %q", got, "sunny")
	}
	if got := parseField(events[3], "role"); got != "tool" {
		t.Errorf("result role = %v, want %q", got, "tool")
	}
	if got := parseField(events[3], "messageId"); got == nil || got == "" {
		t.Error("result messageId should be non-empty")
	}
}

func TestAdapter_ToolFailed(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "bad_tool",
		ArgsJSON: `{}`, CalledAt: time.Now(),
	})
	core.Publish(bus, core.ToolFailedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "bad_tool",
		Error: "timeout", FailedAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	last := events[len(events)-1]
	if got := parseType(last); got != AGUIToolCallResult {
		t.Errorf("last event type = %q, want %q", got, AGUIToolCallResult)
	}
	if got := parseField(last, "content").(string); got != "error: timeout" {
		t.Errorf("error content = %q, want %q", got, "error: timeout")
	}
}

// ── Text streaming tests ────────────────────────────────────────────

func TestAdapter_TextMessageLifecycle(t *testing.T) {
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)

	adapter.EmitTextDelta("msg_1", "hello")
	adapter.EmitTextDelta("msg_1", " world")

	// Close triggers TextMessageEnd
	bus := core.NewEventBus()
	adapter.SubscribeTo(bus)
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, CompletedAt: time.Now()})
	adapter.Close()

	events := c.all()
	types := make([]string, len(events))
	for i, ev := range events {
		types[i] = parseType(ev)
	}

	// TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END, STEP_FINISHED
	if types[0] != AGUITextMessageStart {
		t.Errorf("event[0] = %q, want TEXT_MESSAGE_START", types[0])
	}
	if types[1] != AGUITextMessageContent {
		t.Errorf("event[1] = %q, want TEXT_MESSAGE_CONTENT", types[1])
	}
	if types[2] != AGUITextMessageContent {
		t.Errorf("event[2] = %q, want TEXT_MESSAGE_CONTENT", types[2])
	}

	// Verify role on start
	if got := parseField(events[0], "role"); got != "assistant" {
		t.Errorf("TextMessageStart role = %v, want %q", got, "assistant")
	}
	// Verify delta
	if got := parseField(events[1], "delta"); got != "hello" {
		t.Errorf("delta = %v, want %q", got, "hello")
	}
}

func TestAdapter_TextMessageAutoClose_OnToolCall(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	adapter.EmitTextDelta("msg_1", "thinking...")
	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "search",
		ArgsJSON: `{}`, CalledAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	types := make([]string, len(events))
	for i, ev := range events {
		types[i] = parseType(ev)
	}

	// Should see: TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END, TOOL_CALL_START, ...
	endIdx := -1
	for i, t := range types {
		if t == AGUITextMessageEnd {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		t.Fatal("TEXT_MESSAGE_END not found")
	}

	toolStartIdx := -1
	for i, t := range types {
		if t == AGUIToolCallStart {
			toolStartIdx = i
			break
		}
	}
	if toolStartIdx == -1 {
		t.Fatal("TOOL_CALL_START not found")
	}
	if endIdx >= toolStartIdx {
		t.Errorf("TEXT_MESSAGE_END (idx=%d) should come before TOOL_CALL_START (idx=%d)", endIdx, toolStartIdx)
	}
}

func TestAdapter_EmptyDeltaNotEmitted(t *testing.T) {
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)

	adapter.EmitTextDelta("msg_1", "")
	adapter.Close()

	events := c.all()
	for _, ev := range events {
		if parseType(ev) == AGUITextMessageContent {
			t.Error("should not emit TEXT_MESSAGE_CONTENT for empty delta")
		}
	}
}

// ── Reasoning tests ─────────────────────────────────────────────────

func TestAdapter_ReasoningLifecycle(t *testing.T) {
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)

	adapter.EmitReasoningDelta("reason_1", "let me think")

	bus := core.NewEventBus()
	adapter.SubscribeTo(bus)
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, CompletedAt: time.Now()})
	adapter.Close()

	events := c.all()
	types := make([]string, len(events))
	for i, ev := range events {
		types[i] = parseType(ev)
	}

	if types[0] != AGUIReasoningStart {
		t.Errorf("event[0] = %q, want REASONING_START", types[0])
	}
	if types[1] != AGUIReasoningMessageStart {
		t.Errorf("event[1] = %q, want REASONING_MESSAGE_START", types[1])
	}
	// Verify role is "reasoning" per AG-UI spec
	if got := parseField(events[1], "role"); got != "reasoning" {
		t.Errorf("ReasoningMessageStart role = %v, want %q", got, "reasoning")
	}
	if types[2] != AGUIReasoningMessageContent {
		t.Errorf("event[2] = %q, want REASONING_MESSAGE_CONTENT", types[2])
	}
}

// ── Custom events (gollem-specific) ─────────────────────────────────

func TestAdapter_ApprovalEventsAsCustom(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.ApprovalRequestedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "delete_file",
		ArgsJSON: `{"path":"/tmp"}`, RequestedAt: time.Now(),
	})
	core.Publish(bus, core.ApprovalResolvedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "delete_file",
		Approved: true, ResolvedAt: time.Now(),
	})
	adapter.Close()

	events := c.all()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if got := parseType(events[0]); got != AGUICustom {
		t.Errorf("event[0] type = %q, want CUSTOM", got)
	}
	if got := parseField(events[0], "name"); got != "gollem.approval.requested" {
		t.Errorf("custom name = %v, want %q", got, "gollem.approval.requested")
	}
}

// ── Concurrency tests ───────────────────────────────────────────────

func TestAdapter_ConcurrentToolEvents_OrderPreserved(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	// Simulate concurrent tool completions from parallel goroutines.
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			core.Publish(bus, core.ToolCompletedEvent{
				RunID:       "r1",
				ToolCallID:  "tc_" + string(rune('A'+n)),
				ToolName:    "tool",
				Result:      "ok",
				CompletedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()
	adapter.Close()

	events := c.all()
	if len(events) != 20 {
		t.Fatalf("expected 20 TOOL_CALL_RESULT events, got %d", len(events))
	}

	// Every event should be TOOL_CALL_RESULT (no interleaving with other types).
	for i, ev := range events {
		if got := parseType(ev); got != AGUIToolCallResult {
			t.Errorf("event[%d] type = %q, want TOOL_CALL_RESULT", i, got)
		}
	}
}

func TestAdapter_ConcurrentTextAndTools_NoInterleavedBatch(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	// Fire text delta and tool call concurrently.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		adapter.EmitTextDelta("msg_1", "hello")
	}()
	go func() {
		defer wg.Done()
		core.Publish(bus, core.ToolCalledEvent{
			RunID: "r1", ToolCallID: "tc1", ToolName: "search",
			ArgsJSON: `{}`, CalledAt: time.Now(),
		})
	}()
	wg.Wait()
	adapter.Close()

	events := c.all()
	// Verify no TEXT_MESSAGE_CONTENT appears after TEXT_MESSAGE_END for the same messageId.
	endSeen := map[string]bool{}
	for _, ev := range events {
		typ := parseType(ev)
		msgID, _ := parseField(ev, "messageId").(string)
		if typ == AGUITextMessageEnd {
			endSeen[msgID] = true
		}
		if typ == AGUITextMessageContent && endSeen[msgID] {
			t.Errorf("TEXT_MESSAGE_CONTENT after TEXT_MESSAGE_END for message %q", msgID)
		}
	}
}

// ── Close safety tests ──────────────────────────────────────────────

func TestAdapter_DoubleClose_NoPanic(t *testing.T) {
	adapter := NewAdapter("t1")
	adapter.Close()
	adapter.Close() // should not panic
}

func TestAdapter_CloseWhilePublishing_NoPanic(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	adapter.SubscribeTo(bus)

	// Fire events and close concurrently.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			core.Publish(bus, core.ToolCompletedEvent{
				RunID: "r1", ToolCallID: "tc", ToolName: "t",
				Result: "ok", CompletedAt: time.Now(),
			})
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		adapter.Close()
	}()
	wg.Wait()
	// If we get here without panic, test passes.
}

func TestAdapter_ListenerPanic_DoesNotCrash(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()

	adapter.OnEvent(func(data json.RawMessage) {
		panic("boom")
	})
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})
	adapter.Close()
	// If we get here without panic, the panic recovery works.
}

// ── SubscribeTo guard ───────────────────────────────────────────────

func TestAdapter_SubscribeTo_PanicsOnDouble(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	adapter.SubscribeTo(bus)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on double SubscribeTo")
		}
	}()
	adapter.SubscribeTo(bus) // should panic
}

// ── Timestamp format ────────────────────────────────────────────────

func TestAdapter_TimestampIsUnixMillis(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("t1")
	defer adapter.Close()
	c := collectEvents(adapter)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	adapter.Close()

	events := c.all()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	ts := parseField(events[0], "timestamp")
	tsFloat, ok := ts.(float64) // JSON numbers are float64
	if !ok {
		t.Fatalf("timestamp is %T, want float64 (JSON number)", ts)
	}
	// Should be a reasonable Unix millis value (after 2020).
	if tsFloat < 1577836800000 {
		t.Errorf("timestamp %v is too small for Unix millis", tsFloat)
	}
}

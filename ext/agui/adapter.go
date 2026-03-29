package agui

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// AG-UI protocol event type constants per the official specification.
// See https://docs.ag-ui.com/concepts/events
// See https://github.com/ag-ui-protocol/ag-ui/tree/main/sdks/community/go
const (
	AGUIRunStarted  = "RUN_STARTED"
	AGUIRunFinished = "RUN_FINISHED"
	AGUIRunError    = "RUN_ERROR"

	AGUIStepStarted  = "STEP_STARTED"
	AGUIStepFinished = "STEP_FINISHED"

	AGUITextMessageStart   = "TEXT_MESSAGE_START"
	AGUITextMessageContent = "TEXT_MESSAGE_CONTENT"
	AGUITextMessageEnd     = "TEXT_MESSAGE_END"

	AGUIToolCallStart  = "TOOL_CALL_START"
	AGUIToolCallArgs   = "TOOL_CALL_ARGS"
	AGUIToolCallEnd    = "TOOL_CALL_END"
	AGUIToolCallResult = "TOOL_CALL_RESULT"

	AGUIReasoningStart          = "REASONING_START"
	AGUIReasoningMessageStart   = "REASONING_MESSAGE_START"
	AGUIReasoningMessageContent = "REASONING_MESSAGE_CONTENT"
	AGUIReasoningMessageEnd     = "REASONING_MESSAGE_END"
	AGUIReasoningEnd            = "REASONING_END"

	AGUICustom = "CUSTOM"
)

// Adapter translates gollem runtime events into AG-UI protocol events.
// It subscribes to gollem's EventBus and emits spec-conformant AG-UI events
// as json.RawMessage, ready for SSE transport.
//
// Each event type is marshaled using a dedicated struct matching the official
// AG-UI Go SDK types. This avoids JSON field collisions and ensures required
// fields are always present.
//
// Events are delivered to listeners in strict order via a serializing channel.
// A single goroutine drains the channel, guaranteeing that concurrent tool
// goroutines cannot reorder events, and that listener panics are isolated.
//
// All public methods and event handlers are safe for concurrent use.
type Adapter struct {
	mu        sync.Mutex // protects all mutable state
	threadID  string     // immutable after construction
	listeners []func(json.RawMessage)
	unsubs    []func()

	activeMessageID   string
	activeReasoningID string
	msgCounter        int64 // only accessed under mu

	outCh     chan []json.RawMessage // serializing delivery channel
	done      chan struct{}          // closed when delivery goroutine exits
	closeOnce sync.Once              // ensures Close is idempotent
	sendMu    sync.RWMutex           // synchronizes sends with Close
	closed    bool
}

// NewAdapter creates an AG-UI adapter for a gollem session.
// threadID maps to AG-UI's threadId concept (immutable after construction).
func NewAdapter(threadID string) *Adapter {
	a := &Adapter{
		threadID: threadID,
		outCh:    make(chan []json.RawMessage, 256),
		done:     make(chan struct{}),
	}
	go a.deliverLoop()
	return a
}

// deliverLoop is the single goroutine that delivers events to listeners
// in strict order. Panics in listeners are recovered per-event.
func (a *Adapter) deliverLoop() {
	defer close(a.done)
	for batch := range a.outCh {
		a.mu.Lock()
		listeners := make([]func(json.RawMessage), len(a.listeners))
		copy(listeners, a.listeners)
		a.mu.Unlock()

		for _, data := range batch {
			for _, fn := range listeners {
				if fn != nil {
					func() {
						defer func() { _ = recover() }()
						fn(data)
					}()
				}
			}
		}
	}
}

// OnEvent registers a listener that receives raw AG-UI JSON events.
// Each event is a complete JSON object ready for SSE `data:` framing.
func (a *Adapter) OnEvent(fn func(json.RawMessage)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.listeners = append(a.listeners, fn)
	idx := len(a.listeners) - 1
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if idx < len(a.listeners) {
			a.listeners[idx] = nil
		}
	}
}

// pendingEvents accumulates marshaled events during a handler's execution.
// Events are sent to the delivery channel after all state mutations are
// complete, ensuring multi-event sequences are emitted atomically and
// cross-handler ordering is preserved via the serializing channel.
type pendingEvents struct {
	data    []json.RawMessage
	adapter *Adapter
}

// enqueue marshals an event and adds it to the pending batch.
func (p *pendingEvents) enqueue(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	p.data = append(p.data, data)
}

// send submits the batch to the delivery channel. Non-blocking if the
// channel has capacity; blocks if the delivery goroutine is backed up.
// Must be called AFTER releasing the adapter mutex.
// Safe to call after Close() — batches are dropped once the adapter closes.
func (p *pendingEvents) send() {
	if len(p.data) == 0 {
		return
	}
	p.adapter.sendMu.RLock()
	defer p.adapter.sendMu.RUnlock()
	if p.adapter.closed {
		return
	}
	p.adapter.outCh <- p.data
}

// beginEmit returns a pendingEvents batch targeting the delivery channel.
// MUST be called with a.mu held.
func (a *Adapter) beginEmit() *pendingEvents {
	return &pendingEvents{adapter: a}
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// nextMessageID generates a unique message ID. MUST be called with a.mu held.
func (a *Adapter) nextMessageID() string {
	a.msgCounter++
	return fmt.Sprintf("msg_%s_%d", a.threadID, a.msgCounter)
}

// SubscribeTo connects the adapter to a gollem EventBus.
// Must only be called once; panics if called after subscriptions exist.
func (a *Adapter) SubscribeTo(bus *core.EventBus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.unsubs) > 0 {
		panic("agui: SubscribeTo called on an adapter that is already subscribed")
	}
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onRunStarted))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onRunCompleted))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onTurnStarted))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onTurnCompleted))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onToolCalled))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onToolCompleted))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onToolFailed))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onApprovalRequested))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onApprovalResolved))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onDeferredRequested))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onRunWaiting))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onDeferredResolved))
	a.unsubs = append(a.unsubs, core.Subscribe(bus, a.onRunResumed))
}

// Close unsubscribes from all event bus subscriptions and shuts down
// the delivery goroutine. Blocks until all queued events are delivered.
// Safe to call multiple times (idempotent).
func (a *Adapter) Close() {
	a.mu.Lock()
	unsubs := a.unsubs
	a.unsubs = nil
	a.mu.Unlock()

	for _, unsub := range unsubs {
		unsub()
	}
	a.closeOnce.Do(func() {
		a.sendMu.Lock()
		a.closed = true
		close(a.outCh)
		a.sendMu.Unlock()
		<-a.done
	})
}

// ── Per-event-type structs (match official AG-UI Go SDK field names) ──

type aguiRunStarted struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	ThreadID  string `json:"threadId"`
	RunID     string `json:"runId"`
}

type aguiRunFinished struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	ThreadID  string `json:"threadId"`
	RunID     string `json:"runId"`
}

type aguiRunError struct {
	Type      string  `json:"type"`
	Timestamp int64   `json:"timestamp"`
	Message   string  `json:"message"`
	Code      *string `json:"code,omitempty"`
	RunID     string  `json:"runId,omitempty"`
}

type aguiStepStarted struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	StepName  string `json:"stepName"`
}

type aguiStepFinished struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	StepName  string `json:"stepName"`
}

type aguiTextMessageStart struct {
	Type      string  `json:"type"`
	Timestamp int64   `json:"timestamp"`
	MessageID string  `json:"messageId"`
	Role      *string `json:"role,omitempty"`
}

type aguiTextMessageContent struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

type aguiTextMessageEnd struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
}

type aguiToolCallStart struct {
	Type            string  `json:"type"`
	Timestamp       int64   `json:"timestamp"`
	ToolCallID      string  `json:"toolCallId"`
	ToolCallName    string  `json:"toolCallName"`
	ParentMessageID *string `json:"parentMessageId,omitempty"`
}

type aguiToolCallArgs struct {
	Type       string `json:"type"`
	Timestamp  int64  `json:"timestamp"`
	ToolCallID string `json:"toolCallId"`
	Delta      string `json:"delta"`
}

type aguiToolCallEnd struct {
	Type       string `json:"type"`
	Timestamp  int64  `json:"timestamp"`
	ToolCallID string `json:"toolCallId"`
}

type aguiToolCallResult struct {
	Type       string  `json:"type"`
	Timestamp  int64   `json:"timestamp"`
	MessageID  string  `json:"messageId"`
	ToolCallID string  `json:"toolCallId"`
	Content    string  `json:"content"`
	Role       *string `json:"role,omitempty"`
}

type aguiReasoningStart struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
}

type aguiReasoningMessageStart struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
	Role      string `json:"role"`
}

type aguiReasoningMessageContent struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

type aguiReasoningMessageEnd struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
}

type aguiReasoningEnd struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"messageId"`
}

type aguiCustom struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Name      string          `json:"name"`
	Value     json.RawMessage `json:"value,omitempty"`
}

// ── Lifecycle ───────────────────────────────────────────────────────

func (a *Adapter) onRunStarted(ev core.RunStartedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiRunStarted{
		Type: AGUIRunStarted, Timestamp: nowMillis(),
		ThreadID: a.threadID, RunID: ev.RunID,
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onRunCompleted(ev core.RunCompletedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	a.enqueueCloseActiveMessage(batch)
	if ev.Success || ev.Deferred {
		batch.enqueue(aguiRunFinished{
			Type: AGUIRunFinished, Timestamp: nowMillis(),
			ThreadID: a.threadID, RunID: ev.RunID,
		})
	} else {
		batch.enqueue(aguiRunError{
			Type: AGUIRunError, Timestamp: nowMillis(),
			Message: ev.Error, RunID: ev.RunID,
		})
	}
	a.mu.Unlock()
	batch.send()
}

// ── Steps (gollem turns → AG-UI steps) ──────────────────────────────

func (a *Adapter) onTurnStarted(ev core.TurnStartedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiStepStarted{
		Type: AGUIStepStarted, Timestamp: nowMillis(),
		StepName: fmt.Sprintf("turn_%d", ev.TurnNumber),
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onTurnCompleted(ev core.TurnCompletedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	a.enqueueCloseActiveMessage(batch)
	batch.enqueue(aguiStepFinished{
		Type: AGUIStepFinished, Timestamp: nowMillis(),
		StepName: fmt.Sprintf("turn_%d", ev.TurnNumber),
	})
	a.mu.Unlock()
	batch.send()
}

// ── Text streaming ──────────────────────────────────────────────────

// EmitTextDelta is called by the streaming adapter (Phase 4) when text
// tokens arrive. It manages the TextMessageStart/Content/End lifecycle.
// Safe for concurrent use.
func (a *Adapter) EmitTextDelta(messageID, delta string) {
	a.mu.Lock()
	batch := a.beginEmit()

	if a.activeMessageID != messageID {
		a.activeMessageID = messageID
		role := "assistant"
		batch.enqueue(aguiTextMessageStart{
			Type: AGUITextMessageStart, Timestamp: nowMillis(),
			MessageID: messageID, Role: &role,
		})
	}
	if delta != "" {
		batch.enqueue(aguiTextMessageContent{
			Type: AGUITextMessageContent, Timestamp: nowMillis(),
			MessageID: messageID, Delta: delta,
		})
	}

	a.mu.Unlock()
	batch.send()
}

// EmitReasoningDelta is called when thinking/reasoning tokens arrive.
// Safe for concurrent use.
func (a *Adapter) EmitReasoningDelta(messageID, delta string) {
	a.mu.Lock()
	batch := a.beginEmit()

	if a.activeReasoningID != messageID {
		a.activeReasoningID = messageID
		batch.enqueue(aguiReasoningStart{
			Type: AGUIReasoningStart, Timestamp: nowMillis(), MessageID: messageID,
		})
		batch.enqueue(aguiReasoningMessageStart{
			Type: AGUIReasoningMessageStart, Timestamp: nowMillis(),
			MessageID: messageID, Role: "reasoning",
		})
	}
	if delta != "" {
		batch.enqueue(aguiReasoningMessageContent{
			Type: AGUIReasoningMessageContent, Timestamp: nowMillis(),
			MessageID: messageID, Delta: delta,
		})
	}

	a.mu.Unlock()
	batch.send()
}

// enqueueCloseActiveMessage enqueues End events for any active text/reasoning
// message and clears the active IDs. MUST be called with a.mu held.
func (a *Adapter) enqueueCloseActiveMessage(batch *pendingEvents) {
	ts := nowMillis()
	if a.activeMessageID != "" {
		batch.enqueue(aguiTextMessageEnd{
			Type: AGUITextMessageEnd, Timestamp: ts, MessageID: a.activeMessageID,
		})
		a.activeMessageID = ""
	}
	if a.activeReasoningID != "" {
		batch.enqueue(aguiReasoningMessageEnd{
			Type: AGUIReasoningMessageEnd, Timestamp: ts, MessageID: a.activeReasoningID,
		})
		batch.enqueue(aguiReasoningEnd{
			Type: AGUIReasoningEnd, Timestamp: ts, MessageID: a.activeReasoningID,
		})
		a.activeReasoningID = ""
	}
}

// ── Tool calls ──────────────────────────────────────────────────────

func (a *Adapter) onToolCalled(ev core.ToolCalledEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	a.enqueueCloseActiveMessage(batch)

	ts := nowMillis()
	batch.enqueue(aguiToolCallStart{
		Type: AGUIToolCallStart, Timestamp: ts,
		ToolCallID: ev.ToolCallID, ToolCallName: ev.ToolName,
	})
	if ev.ArgsJSON != "" {
		batch.enqueue(aguiToolCallArgs{
			Type: AGUIToolCallArgs, Timestamp: ts,
			ToolCallID: ev.ToolCallID, Delta: ev.ArgsJSON,
		})
	}
	batch.enqueue(aguiToolCallEnd{
		Type: AGUIToolCallEnd, Timestamp: ts, ToolCallID: ev.ToolCallID,
	})

	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onToolCompleted(ev core.ToolCompletedEvent) {
	role := "tool"
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiToolCallResult{
		Type: AGUIToolCallResult, Timestamp: nowMillis(),
		MessageID: a.nextMessageID(), ToolCallID: ev.ToolCallID,
		Content: ev.Result, Role: &role,
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onToolFailed(ev core.ToolFailedEvent) {
	role := "tool"
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiToolCallResult{
		Type: AGUIToolCallResult, Timestamp: nowMillis(),
		MessageID: a.nextMessageID(), ToolCallID: ev.ToolCallID,
		Content: "error: " + ev.Error, Role: &role,
	})
	a.mu.Unlock()
	batch.send()
}

// ── Gollem-specific → AG-UI CUSTOM ─────────────────────────────────

func (a *Adapter) onApprovalRequested(ev core.ApprovalRequestedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiCustom{
		Type: AGUICustom, Timestamp: nowMillis(),
		Name: "gollem.approval.requested",
		Value: mustMarshal(map[string]any{
			"toolCallId": ev.ToolCallID, "toolName": ev.ToolName, "argsJson": ev.ArgsJSON,
		}),
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onApprovalResolved(ev core.ApprovalResolvedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiCustom{
		Type: AGUICustom, Timestamp: nowMillis(),
		Name: "gollem.approval.resolved",
		Value: mustMarshal(map[string]any{
			"toolCallId": ev.ToolCallID, "toolName": ev.ToolName, "approved": ev.Approved,
		}),
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onDeferredRequested(ev core.DeferredRequestedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiCustom{
		Type: AGUICustom, Timestamp: nowMillis(),
		Name: "gollem.deferred.requested",
		Value: mustMarshal(map[string]any{
			"toolCallId": ev.ToolCallID, "toolName": ev.ToolName, "argsJson": ev.ArgsJSON,
		}),
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onRunWaiting(ev core.RunWaitingEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiCustom{
		Type: AGUICustom, Timestamp: nowMillis(),
		Name:  "gollem.run.waiting",
		Value: mustMarshal(map[string]any{"reason": ev.Reason}),
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onDeferredResolved(ev core.DeferredResolvedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiCustom{
		Type: AGUICustom, Timestamp: nowMillis(),
		Name: "gollem.deferred.resolved",
		Value: mustMarshal(map[string]any{
			"toolCallId": ev.ToolCallID,
			"toolName":   ev.ToolName,
			"content":    ev.Content,
			"isError":    ev.IsError,
		}),
	})
	a.mu.Unlock()
	batch.send()
}

func (a *Adapter) onRunResumed(ev core.RunResumedEvent) {
	a.mu.Lock()
	batch := a.beginEmit()
	batch.enqueue(aguiCustom{
		Type: AGUICustom, Timestamp: nowMillis(),
		Name: "gollem.run.resumed",
		Value: mustMarshal(map[string]any{
			"runId": ev.RunID,
		}),
	})
	a.mu.Unlock()
	batch.send()
}

// ── Helpers ─────────────────────────────────────────────────────────

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

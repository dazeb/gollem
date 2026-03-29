package agui

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// ── AG-UI protocol invariant tests ──────────────────────────────────
//
// These tests verify structural invariants of the AG-UI event stream
// that any compliant client depends on:
//
// 1. Every RUN_STARTED has a matching RUN_FINISHED or RUN_ERROR
// 2. Every STEP_STARTED has a matching STEP_FINISHED (same stepName)
// 3. Every TEXT_MESSAGE_START has a matching TEXT_MESSAGE_END (same messageId)
// 4. TEXT_MESSAGE_CONTENT only appears between START and END for its messageId
// 5. Every TOOL_CALL_START has a matching TOOL_CALL_END (same toolCallId)
// 6. TOOL_CALL_ARGS only appears between START and END for its toolCallId
// 7. Every REASONING_START has a matching REASONING_END
// 8. Event ordering: no CONTENT after END for the same ID

type protocolChecker struct {
	t      *testing.T
	events []map[string]any

	activeRuns      map[string]bool
	activeSteps     map[string]bool
	activeMessages  map[string]bool
	activeToolCalls map[string]bool
	activeReasoning map[string]bool
	endedMessages   map[string]bool
}

func newProtocolChecker(t *testing.T) *protocolChecker {
	return &protocolChecker{
		t:               t,
		activeRuns:      make(map[string]bool),
		activeSteps:     make(map[string]bool),
		activeMessages:  make(map[string]bool),
		activeToolCalls: make(map[string]bool),
		activeReasoning: make(map[string]bool),
		endedMessages:   make(map[string]bool),
	}
}

func (pc *protocolChecker) collect(data json.RawMessage) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		pc.t.Errorf("failed to unmarshal event: %v", err)
		return
	}
	pc.events = append(pc.events, m)
}

func (pc *protocolChecker) validate() {
	pc.t.Helper()

	for i, ev := range pc.events {
		typ, _ := ev["type"].(string)
		switch typ {
		case AGUIRunStarted:
			runID, _ := ev["runId"].(string)
			if pc.activeRuns[runID] {
				pc.t.Errorf("event[%d]: duplicate RUN_STARTED for run %q", i, runID)
			}
			pc.activeRuns[runID] = true

		case AGUIRunFinished:
			runID, _ := ev["runId"].(string)
			if !pc.activeRuns[runID] {
				pc.t.Errorf("event[%d]: RUN_FINISHED without RUN_STARTED for run %q", i, runID)
			}
			delete(pc.activeRuns, runID)

		case AGUIRunError:
			// RunError may or may not have runId
			if runID, ok := ev["runId"].(string); ok && runID != "" {
				delete(pc.activeRuns, runID)
			}

		case AGUIStepStarted:
			name, _ := ev["stepName"].(string)
			if pc.activeSteps[name] {
				pc.t.Errorf("event[%d]: duplicate STEP_STARTED for step %q", i, name)
			}
			pc.activeSteps[name] = true

		case AGUIStepFinished:
			name, _ := ev["stepName"].(string)
			if !pc.activeSteps[name] {
				pc.t.Errorf("event[%d]: STEP_FINISHED without STEP_STARTED for step %q", i, name)
			}
			delete(pc.activeSteps, name)

		case AGUITextMessageStart:
			msgID, _ := ev["messageId"].(string)
			if pc.activeMessages[msgID] {
				pc.t.Errorf("event[%d]: duplicate TEXT_MESSAGE_START for message %q", i, msgID)
			}
			pc.activeMessages[msgID] = true

		case AGUITextMessageContent:
			msgID, _ := ev["messageId"].(string)
			if !pc.activeMessages[msgID] {
				pc.t.Errorf("event[%d]: TEXT_MESSAGE_CONTENT without active TEXT_MESSAGE_START for message %q", i, msgID)
			}
			if pc.endedMessages[msgID] {
				pc.t.Errorf("event[%d]: TEXT_MESSAGE_CONTENT after TEXT_MESSAGE_END for message %q", i, msgID)
			}
			delta, _ := ev["delta"].(string)
			if delta == "" {
				pc.t.Errorf("event[%d]: TEXT_MESSAGE_CONTENT has empty delta", i)
			}

		case AGUITextMessageEnd:
			msgID, _ := ev["messageId"].(string)
			if !pc.activeMessages[msgID] {
				pc.t.Errorf("event[%d]: TEXT_MESSAGE_END without TEXT_MESSAGE_START for message %q", i, msgID)
			}
			delete(pc.activeMessages, msgID)
			pc.endedMessages[msgID] = true

		case AGUIToolCallStart:
			tcID, _ := ev["toolCallId"].(string)
			if pc.activeToolCalls[tcID] {
				pc.t.Errorf("event[%d]: duplicate TOOL_CALL_START for %q", i, tcID)
			}
			pc.activeToolCalls[tcID] = true
			// Required fields
			if _, ok := ev["toolCallName"]; !ok {
				pc.t.Errorf("event[%d]: TOOL_CALL_START missing toolCallName", i)
			}

		case AGUIToolCallArgs:
			tcID, _ := ev["toolCallId"].(string)
			if !pc.activeToolCalls[tcID] {
				pc.t.Errorf("event[%d]: TOOL_CALL_ARGS without active TOOL_CALL_START for %q", i, tcID)
			}
			delta, _ := ev["delta"].(string)
			if delta == "" {
				pc.t.Errorf("event[%d]: TOOL_CALL_ARGS has empty delta", i)
			}

		case AGUIToolCallEnd:
			tcID, _ := ev["toolCallId"].(string)
			if !pc.activeToolCalls[tcID] {
				pc.t.Errorf("event[%d]: TOOL_CALL_END without TOOL_CALL_START for %q", i, tcID)
			}
			delete(pc.activeToolCalls, tcID)

		case AGUIToolCallResult:
			// Verify required fields
			if _, ok := ev["messageId"]; !ok {
				pc.t.Errorf("event[%d]: TOOL_CALL_RESULT missing messageId", i)
			}
			if _, ok := ev["toolCallId"]; !ok {
				pc.t.Errorf("event[%d]: TOOL_CALL_RESULT missing toolCallId", i)
			}
			if _, ok := ev["content"]; !ok {
				pc.t.Errorf("event[%d]: TOOL_CALL_RESULT missing content", i)
			}

		case AGUIReasoningStart:
			msgID, _ := ev["messageId"].(string)
			pc.activeReasoning[msgID] = true

		case AGUIReasoningEnd:
			msgID, _ := ev["messageId"].(string)
			if !pc.activeReasoning[msgID] {
				pc.t.Errorf("event[%d]: REASONING_END without REASONING_START for %q", i, msgID)
			}
			delete(pc.activeReasoning, msgID)

		case AGUICustom:
			if _, ok := ev["name"]; !ok {
				pc.t.Errorf("event[%d]: CUSTOM missing name", i)
			}
		}

		// Every event must have a timestamp (Unix millis number)
		if _, ok := ev["timestamp"]; !ok {
			pc.t.Errorf("event[%d] (%s): missing timestamp", i, typ)
		}
	}

	// Check for unclosed entities
	for run := range pc.activeRuns {
		pc.t.Errorf("unclosed RUN_STARTED for run %q", run)
	}
	for step := range pc.activeSteps {
		pc.t.Errorf("unclosed STEP_STARTED for step %q", step)
	}
	for msg := range pc.activeMessages {
		pc.t.Errorf("unclosed TEXT_MESSAGE_START for message %q", msg)
	}
	for tc := range pc.activeToolCalls {
		pc.t.Errorf("unclosed TOOL_CALL_START for tool call %q", tc)
	}
	for r := range pc.activeReasoning {
		pc.t.Errorf("unclosed REASONING_START for %q", r)
	}
}

// ── Full lifecycle invariant tests ──────────────────────────────────

func TestInvariant_FullRunLifecycle(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	// Simulate: run start → turn 1 (text) → turn 2 (tool call + result) → run finish
	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})

	// Turn 1: text response
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_1", "Hello ")
	adapter.EmitTextDelta("msg_1", "world!")
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, HasText: true, CompletedAt: time.Now()})

	// Turn 2: tool call
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 2, StartedAt: time.Now()})
	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "search",
		ArgsJSON: `{"q":"test"}`, CalledAt: time.Now(),
	})
	core.Publish(bus, core.ToolCompletedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "search",
		Result: "found it", CompletedAt: time.Now(),
	})
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 2, HasToolCalls: true, CompletedAt: time.Now()})

	// Turn 3: final text
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 3, StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_2", "Done!")
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 3, HasText: true, CompletedAt: time.Now()})

	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})

	adapter.Close()
	pc.validate()
}

func TestInvariant_RunWithError(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_1", "partial output")
	core.Publish(bus, core.TurnCompletedEvent{
		RunID: "r1", TurnNumber: 1, Error: "model failed", CompletedAt: time.Now(),
	})
	core.Publish(bus, core.RunCompletedEvent{
		RunID: "r1", Success: false, Error: "model failed", CompletedAt: time.Now(),
	})

	adapter.Close()
	pc.validate()
}

func TestInvariant_MultipleToolsConcurrent(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})

	// Simulate concurrent tool calls
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tcID := string(rune('A'+n)) + "_tc"
			core.Publish(bus, core.ToolCalledEvent{
				RunID: "r1", ToolCallID: tcID, ToolName: "tool_" + tcID,
				ArgsJSON: `{"n":` + string(rune('0'+n)) + `}`, CalledAt: time.Now(),
			})
			core.Publish(bus, core.ToolCompletedEvent{
				RunID: "r1", ToolCallID: tcID, ToolName: "tool_" + tcID,
				Result: "ok", CompletedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()

	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, HasToolCalls: true, CompletedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})

	adapter.Close()
	pc.validate()
}

func TestInvariant_TextThenToolThenText(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})

	// Turn 1: text + tool in same turn
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_1", "I'll search for that...")

	// Tool call should auto-close the text message
	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "search",
		ArgsJSON: `{"q":"test"}`, CalledAt: time.Now(),
	})
	core.Publish(bus, core.ToolCompletedEvent{
		RunID: "r1", ToolCallID: "tc1", Result: "results", CompletedAt: time.Now(),
	})
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, CompletedAt: time.Now()})

	// Turn 2: text response with results
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 2, StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_2", "Here are the results!")
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 2, HasText: true, CompletedAt: time.Now()})

	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})

	adapter.Close()
	pc.validate()
}

func TestInvariant_ReasoningThenText(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})

	// Reasoning followed by text
	adapter.EmitReasoningDelta("reason_1", "thinking about this...")
	adapter.EmitTextDelta("msg_1", "Here's my answer")

	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, HasText: true, CompletedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})

	adapter.Close()
	pc.validate()
}

func TestInvariant_ApprovalDuringRun(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})

	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "delete",
		ArgsJSON: `{"path":"/important"}`, CalledAt: time.Now(),
	})
	core.Publish(bus, core.ApprovalRequestedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "delete", RequestedAt: time.Now(),
	})
	core.Publish(bus, core.RunWaitingEvent{RunID: "r1", Reason: "approval", WaitingAt: time.Now()})
	core.Publish(bus, core.RunResumedEvent{RunID: "r1", ResumedAt: time.Now()})
	core.Publish(bus, core.ApprovalResolvedEvent{
		RunID: "r1", ToolCallID: "tc1", Approved: true, ResolvedAt: time.Now(),
	})
	core.Publish(bus, core.ToolCompletedEvent{
		RunID: "r1", ToolCallID: "tc1", Result: "deleted", CompletedAt: time.Now(),
	})
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, CompletedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})

	adapter.Close()
	pc.validate()
}

func TestInvariant_DeferredWaitThenResume(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.TurnStartedEvent{RunID: "r1", TurnNumber: 1, StartedAt: time.Now()})
	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "fetch",
		ArgsJSON: `{"id":1}`, CalledAt: time.Now(),
	})
	core.Publish(bus, core.DeferredRequestedEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "fetch",
		ArgsJSON: `{"id":1}`, RequestedAt: time.Now(),
	})
	core.Publish(bus, core.RunWaitingEvent{RunID: "r1", Reason: "deferred", WaitingAt: time.Now()})
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r1", TurnNumber: 1, HasToolCalls: true, CompletedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{
		RunID: "r1", Success: false, Deferred: true,
		Error:       "agent run deferred: 1 tool call(s) require external resolution",
		CompletedAt: time.Now(),
	})

	core.Publish(bus, core.RunStartedEvent{RunID: "r2", StartedAt: time.Now()})
	core.Publish(bus, core.RunResumedEvent{RunID: "r2", ResumedAt: time.Now()})
	core.Publish(bus, core.DeferredResolvedEvent{
		RunID: "r2", ToolCallID: "tc1", ToolName: "fetch",
		Content: "done", ResolvedAt: time.Now(),
	})
	core.Publish(bus, core.TurnStartedEvent{RunID: "r2", TurnNumber: 1, StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_2", "Resolved.")
	core.Publish(bus, core.TurnCompletedEvent{RunID: "r2", TurnNumber: 1, HasText: true, CompletedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{RunID: "r2", Success: true, CompletedAt: time.Now()})

	adapter.Close()
	pc.validate()

	var (
		sawRunError         bool
		sawRunResumed       bool
		sawDeferredResolved bool
	)
	for _, ev := range pc.events {
		if ev["type"] == AGUIRunError {
			sawRunError = true
		}
		if ev["type"] != AGUICustom {
			continue
		}
		name, _ := ev["name"].(string)
		if name == "gollem.run.resumed" {
			sawRunResumed = true
		}
		if name == "gollem.deferred.resolved" {
			sawDeferredResolved = true
		}
	}
	if sawRunError {
		t.Fatal("deferred wait should not be emitted as RUN_ERROR")
	}
	if !sawRunResumed {
		t.Fatal("expected gollem.run.resumed custom event")
	}
	if !sawDeferredResolved {
		t.Fatal("expected gollem.deferred.resolved custom event")
	}
}

func TestInvariant_NoEventsAfterClose(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	core.Publish(bus, core.RunCompletedEvent{RunID: "r1", Success: true, CompletedAt: time.Now()})
	adapter.Close()

	countBefore := len(pc.events)

	// Events after close should be silently dropped (send on closed channel recovered)
	core.Publish(bus, core.RunStartedEvent{RunID: "r2", StartedAt: time.Now()})
	time.Sleep(10 * time.Millisecond)

	if len(pc.events) != countBefore {
		t.Errorf("events received after Close: %d new events", len(pc.events)-countBefore)
	}

	pc.validate()
}

func TestInvariant_MessageIDUniqueness(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	// Fire 20 tool completions concurrently — each should get a unique messageId
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			core.Publish(bus, core.ToolCompletedEvent{
				RunID:       "r1",
				ToolCallID:  string(rune('A' + n)),
				ToolName:    "tool",
				Result:      "ok",
				CompletedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()
	adapter.Close()

	// Collect all messageIds from TOOL_CALL_RESULT events
	seen := map[string]bool{}
	for _, ev := range pc.events {
		if ev["type"] == AGUIToolCallResult {
			msgID, _ := ev["messageId"].(string)
			if msgID == "" {
				t.Error("TOOL_CALL_RESULT has empty messageId")
			}
			if seen[msgID] {
				t.Errorf("duplicate messageId: %q", msgID)
			}
			seen[msgID] = true
		}
	}
}

func TestInvariant_EmptyArgsNotEmitted(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.ToolCalledEvent{
		RunID: "r1", ToolCallID: "tc1", ToolName: "no_args",
		ArgsJSON: "", CalledAt: time.Now(),
	})
	adapter.Close()

	for _, ev := range pc.events {
		if ev["type"] == AGUIToolCallArgs {
			t.Error("should not emit TOOL_CALL_ARGS for empty argsJSON")
		}
	}
	pc.validate()
}

func TestInvariant_RunErrorClosesActiveText(t *testing.T) {
	bus := core.NewEventBus()
	adapter := NewAdapter("thread_1")
	pc := newProtocolChecker(t)
	adapter.OnEvent(pc.collect)
	adapter.SubscribeTo(bus)

	core.Publish(bus, core.RunStartedEvent{RunID: "r1", StartedAt: time.Now()})
	adapter.EmitTextDelta("msg_1", "partial...")
	// Error without explicitly closing the message — adapter should auto-close
	core.Publish(bus, core.RunCompletedEvent{
		RunID: "r1", Success: false, Error: "crash", CompletedAt: time.Now(),
	})
	adapter.Close()

	pc.validate() // should have no unclosed TEXT_MESSAGE_START
}

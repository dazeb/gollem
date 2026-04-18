package anthropic

import (
	"io"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// TestParseSSEStreamThinking tests streaming with extended thinking blocks.
// Anthropic sends thinking content and signatures as separate delta events.
func TestParseSSEStreamThinking(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":50,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" about this carefully."}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_part1"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_part2"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Here is the answer."}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":30}}

event: message_stop
data: {"type":"message_stop"}

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, ClaudeSonnet46)

	var events []core.ModelResponseStreamEvent
	for {
		event, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, event)
	}

	// Expected events:
	// 0: PartStart (thinking)
	// 1: PartDelta (thinking "Let me think")
	// 2: PartDelta (thinking " about this carefully.")
	// 3: (signature_delta - no event emitted)
	// 4: (signature_delta - no event emitted)
	// 5: PartEnd (thinking)
	// 6: PartStart (text)
	// 7: PartDelta (text "Here is the answer.")
	// 8: PartEnd (text)
	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d", len(events))
	}

	// Verify thinking block start.
	start0, ok := events[0].(core.PartStartEvent)
	if !ok {
		t.Fatalf("event[0]: expected PartStartEvent, got %T", events[0])
	}
	if _, ok := start0.Part.(core.ThinkingPart); !ok {
		t.Fatalf("event[0].Part: expected ThinkingPart, got %T", start0.Part)
	}

	// Verify thinking deltas.
	d1, ok := events[1].(core.PartDeltaEvent)
	if !ok {
		t.Fatalf("event[1]: expected PartDeltaEvent, got %T", events[1])
	}
	td1, ok := d1.Delta.(core.ThinkingPartDelta)
	if !ok {
		t.Fatalf("event[1].Delta: expected ThinkingPartDelta, got %T", d1.Delta)
	}
	if td1.ContentDelta != "Let me think" {
		t.Errorf("thinking delta 1 = %q, want 'Let me think'", td1.ContentDelta)
	}

	d2, ok := events[2].(core.PartDeltaEvent)
	if !ok {
		t.Fatalf("event[2]: expected PartDeltaEvent, got %T", events[2])
	}
	td2, ok := d2.Delta.(core.ThinkingPartDelta)
	if !ok {
		t.Fatalf("event[2].Delta: expected ThinkingPartDelta, got %T", d2.Delta)
	}
	if td2.ContentDelta != " about this carefully." {
		t.Errorf("thinking delta 2 = %q", td2.ContentDelta)
	}

	// Verify final response has thinking block with accumulated content and signature.
	resp := stream.Response()
	if len(resp.Parts) != 2 {
		t.Fatalf("expected 2 parts in response, got %d", len(resp.Parts))
	}

	tp, ok := resp.Parts[0].(core.ThinkingPart)
	if !ok {
		t.Fatalf("part[0]: expected ThinkingPart, got %T", resp.Parts[0])
	}
	if tp.Content != "Let me think about this carefully." {
		t.Errorf("thinking content = %q, want 'Let me think about this carefully.'", tp.Content)
	}
	if tp.Signature != "sig_part1sig_part2" {
		t.Errorf("thinking signature = %q, want 'sig_part1sig_part2'", tp.Signature)
	}

	txtPart, ok := resp.Parts[1].(core.TextPart)
	if !ok {
		t.Fatalf("part[1]: expected TextPart, got %T", resp.Parts[1])
	}
	if txtPart.Content != "Here is the answer." {
		t.Errorf("text content = %q", txtPart.Content)
	}
}

// TestParseSSEStreamThinkingThenToolCall tests the sequence:
// thinking -> tool_use, which is common with extended thinking + tools.
func TestParseSSEStreamThinkingThenToolCall(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":50,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"I need to run a command."}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"think_sig_123"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_abc","name":"bash"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}

event: message_stop
data: {"type":"message_stop"}

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, ClaudeSonnet46)

	for {
		_, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	resp := stream.Response()
	if len(resp.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(resp.Parts))
	}

	// First part: thinking.
	tp, ok := resp.Parts[0].(core.ThinkingPart)
	if !ok {
		t.Fatalf("part[0]: expected ThinkingPart, got %T", resp.Parts[0])
	}
	if tp.Content != "I need to run a command." {
		t.Errorf("thinking = %q", tp.Content)
	}
	if tp.Signature != "think_sig_123" {
		t.Errorf("signature = %q, want 'think_sig_123'", tp.Signature)
	}

	// Second part: tool call.
	tc, ok := resp.Parts[1].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[1]: expected ToolCallPart, got %T", resp.Parts[1])
	}
	if tc.ToolName != "bash" {
		t.Errorf("tool name = %q", tc.ToolName)
	}
	if tc.ArgsJSON != `{"command":"ls"}` {
		t.Errorf("args = %q", tc.ArgsJSON)
	}
	if tc.ToolCallID != "call_abc" {
		t.Errorf("call id = %q", tc.ToolCallID)
	}

	if resp.FinishReason != core.FinishReasonToolCall {
		t.Errorf("finish reason = %q, want tool_call", resp.FinishReason)
	}
}

// TestParseSSEStreamError tests that mid-stream errors are properly propagated.
func TestParseSSEStreamErrorMidContent(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Partial"}}

event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, ClaudeSonnet46)

	// First events should be text start and delta.
	event1, err := stream.Next()
	if err != nil {
		t.Fatalf("expected first event, got error: %v", err)
	}
	if _, ok := event1.(core.PartStartEvent); !ok {
		t.Fatalf("expected PartStartEvent, got %T", event1)
	}

	event2, err := stream.Next()
	if err != nil {
		t.Fatalf("expected delta event, got error: %v", err)
	}
	if _, ok := event2.(core.PartDeltaEvent); !ok {
		t.Fatalf("expected PartDeltaEvent, got %T", event2)
	}

	// Next call should return the error.
	_, err = stream.Next()
	if err == nil {
		t.Fatal("expected error after error event")
	}
	if !strings.Contains(err.Error(), "Overloaded") {
		t.Errorf("error = %v, want to contain 'Overloaded'", err)
	}
	if !strings.Contains(err.Error(), "overloaded_error") {
		t.Errorf("error = %v, want to contain 'overloaded_error'", err)
	}
}

// TestParseSSEStreamMultipleToolCalls tests streaming with multiple
// sequential tool calls in the same response.
func TestParseSSEStreamMultipleToolCalls(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":50,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"bash"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_2","name":"view"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"main.go\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}

event: message_stop
data: {"type":"message_stop"}

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, ClaudeSonnet46)

	for {
		_, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	resp := stream.Response()
	if len(resp.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(resp.Parts))
	}

	tc1, ok := resp.Parts[0].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[0]: expected ToolCallPart, got %T", resp.Parts[0])
	}
	if tc1.ToolName != "bash" || tc1.ToolCallID != "call_1" {
		t.Errorf("tc1 = %q / %q", tc1.ToolName, tc1.ToolCallID)
	}
	if tc1.ArgsJSON != `{"command":"ls"}` {
		t.Errorf("tc1 args = %q", tc1.ArgsJSON)
	}

	tc2, ok := resp.Parts[1].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[1]: expected ToolCallPart, got %T", resp.Parts[1])
	}
	if tc2.ToolName != "view" || tc2.ToolCallID != "call_2" {
		t.Errorf("tc2 = %q / %q", tc2.ToolName, tc2.ToolCallID)
	}
	if tc2.ArgsJSON != `{"file":"main.go"}` {
		t.Errorf("tc2 args = %q", tc2.ArgsJSON)
	}
}

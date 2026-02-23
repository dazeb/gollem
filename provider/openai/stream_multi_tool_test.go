package openai

import (
	"io"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// TestParseSSEStreamMultipleToolCalls tests streaming with multiple tool calls
// that arrive across multiple chunks, which is common with GPT-4o.
func TestParseSSEStreamMultipleToolCalls(t *testing.T) {
	// Simulate GPT-4o returning 3 tool calls with interleaved argument deltas.
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"view","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\"ls\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"file\":\"main.go\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":2,"id":"call_c","type":"function","function":{"name":"search","arguments":"{\"q\":\"test\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":20,"total_tokens":70}}

data: [DONE]

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gpt-4o")

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

	// Should have: 3 PartStart + argument deltas.
	// PartStart(call_a), Delta(call_a args), PartStart(call_b), Delta(call_a args),
	// Delta(call_b args), PartStart(call_c).
	if len(events) < 6 {
		t.Fatalf("expected at least 6 events, got %d", len(events))
	}

	// Verify final response has 3 tool calls with correct accumulated args.
	resp := stream.Response()
	if len(resp.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(resp.Parts))
	}

	tc0, ok := resp.Parts[0].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[0]: expected ToolCallPart, got %T", resp.Parts[0])
	}
	if tc0.ToolName != "bash" {
		t.Errorf("tc0 name = %q", tc0.ToolName)
	}
	if tc0.ToolCallID != "call_a" {
		t.Errorf("tc0 id = %q", tc0.ToolCallID)
	}
	if tc0.ArgsJSON != `{"command":"ls"}` {
		t.Errorf("tc0 args = %q, want %q", tc0.ArgsJSON, `{"command":"ls"}`)
	}

	tc1, ok := resp.Parts[1].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[1]: expected ToolCallPart, got %T", resp.Parts[1])
	}
	if tc1.ToolName != "view" {
		t.Errorf("tc1 name = %q", tc1.ToolName)
	}
	if tc1.ToolCallID != "call_b" {
		t.Errorf("tc1 id = %q", tc1.ToolCallID)
	}
	if tc1.ArgsJSON != `{"file":"main.go"}` {
		t.Errorf("tc1 args = %q, want %q", tc1.ArgsJSON, `{"file":"main.go"}`)
	}

	tc2, ok := resp.Parts[2].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[2]: expected ToolCallPart, got %T", resp.Parts[2])
	}
	if tc2.ToolName != "search" {
		t.Errorf("tc2 name = %q", tc2.ToolName)
	}
	if tc2.ToolCallID != "call_c" {
		t.Errorf("tc2 id = %q", tc2.ToolCallID)
	}
	if tc2.ArgsJSON != `{"q":"test"}` {
		t.Errorf("tc2 args = %q, want %q", tc2.ArgsJSON, `{"q":"test"}`)
	}

	if resp.FinishReason != core.FinishReasonToolCall {
		t.Errorf("finish reason = %q, want tool_call", resp.FinishReason)
	}
}

// TestParseSSEStreamTextThenMultipleToolCalls tests text followed by multiple
// tool calls, which happens when the model explains before acting.
func TestParseSSEStreamTextThenMultipleToolCalls(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Let me check."},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"view","arguments":"{\"file\":\"go.mod\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gpt-4o")

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
	if len(resp.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(resp.Parts))
	}

	// First: text.
	tp, ok := resp.Parts[0].(core.TextPart)
	if !ok {
		t.Fatalf("part[0]: expected TextPart, got %T", resp.Parts[0])
	}
	if tp.Content != "Let me check." {
		t.Errorf("text = %q", tp.Content)
	}

	// Second and third: tool calls.
	tc1, ok := resp.Parts[1].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[1]: expected ToolCallPart, got %T", resp.Parts[1])
	}
	if tc1.ToolName != "bash" || tc1.ToolCallID != "call_1" {
		t.Errorf("tc1 = %q / %q", tc1.ToolName, tc1.ToolCallID)
	}

	tc2, ok := resp.Parts[2].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[2]: expected ToolCallPart, got %T", resp.Parts[2])
	}
	if tc2.ToolName != "view" || tc2.ToolCallID != "call_2" {
		t.Errorf("tc2 = %q / %q", tc2.ToolName, tc2.ToolCallID)
	}
}

// TestParseSSEStreamEmptyToolArgs tests that tool calls with no arguments
// get "{}" as the default ArgsJSON.
func TestParseSSEStreamEmptyToolArgs(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_empty","type":"function","function":{"name":"get_time","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gpt-4o")

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
	if len(resp.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(resp.Parts))
	}
	tc, ok := resp.Parts[0].(core.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", resp.Parts[0])
	}
	if tc.ArgsJSON != "{}" {
		t.Errorf("args = %q, want '{}'", tc.ArgsJSON)
	}
}

// TestParseSSEStreamCachedTokensAndReasoning tests that cached tokens
// and reasoning tokens from the usage chunk are properly parsed.
func TestParseSSEStreamCachedTokensAndReasoning(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Done."},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":50},"completion_tokens_details":{"reasoning_tokens":5}}}

data: [DONE]

`

	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "o3-mini")

	for {
		_, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	usage := stream.Usage()
	if usage.InputTokens != 100 {
		t.Errorf("input tokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 10 {
		t.Errorf("output tokens = %d, want 10", usage.OutputTokens)
	}
	if usage.CacheReadTokens != 50 {
		t.Errorf("cache read tokens = %d, want 50", usage.CacheReadTokens)
	}
	if usage.Details == nil || usage.Details["reasoning_tokens"] != 5 {
		t.Errorf("reasoning tokens = %v, want 5", usage.Details)
	}
}

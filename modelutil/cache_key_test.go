package modelutil

import (
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestStableCacheKey_IgnoresUnstableCoreRequestMetadata(t *testing.T) {
	t1 := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 2, 10, 5, 0, 0, time.UTC)

	messages1 := []core.ModelMessage{
		core.ModelRequest{
			Timestamp: t1,
			Parts: []core.ModelRequestPart{
				core.SystemPromptPart{Content: "You are terse.", Timestamp: t1},
				core.UserPromptPart{Content: "Look up the status.", Timestamp: t1},
			},
		},
		core.ModelResponse{
			Timestamp:    t1,
			ModelName:    "gpt-4o-mini",
			FinishReason: core.FinishReasonToolCall,
			Usage:        core.Usage{InputTokens: 10, OutputTokens: 5},
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{
					ToolName:   "lookup",
					ArgsJSON:   `{"b":2,"a":1}`,
					ToolCallID: "call-alpha",
				},
			},
		},
		core.ModelRequest{
			Timestamp: t1,
			Parts: []core.ModelRequestPart{
				core.ToolReturnPart{
					ToolName:   "lookup",
					ToolCallID: "call-alpha",
					Content:    map[string]any{"ok": true},
					Timestamp:  t1,
				},
			},
		},
	}

	messages2 := []core.ModelMessage{
		core.ModelRequest{
			Timestamp: t2,
			Parts: []core.ModelRequestPart{
				core.SystemPromptPart{Content: "You are terse.", Timestamp: t2},
				core.UserPromptPart{Content: "Look up the status.", Timestamp: t2},
			},
		},
		core.ModelResponse{
			Timestamp:    t2,
			ModelName:    "claude-sonnet-4-6",
			FinishReason: core.FinishReasonStop,
			Usage:        core.Usage{InputTokens: 999, OutputTokens: 111},
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{
					ToolName:   "lookup",
					ArgsJSON:   `{"a":1,"b":2}`,
					ToolCallID: "call-beta",
				},
			},
		},
		core.ModelRequest{
			Timestamp: t2,
			Parts: []core.ModelRequestPart{
				core.ToolReturnPart{
					ToolName:   "lookup",
					ToolCallID: "call-beta",
					Content:    map[string]any{"ok": true},
					Timestamp:  t2,
				},
			},
		},
	}

	key1 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: messages1})
	key2 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: messages2})
	if key1 != key2 {
		t.Fatalf("stable keys differ for equivalent requests:\n%s\n%s", key1, key2)
	}
}

func TestStableCacheKey_DifferentSemanticContentDiffers(t *testing.T) {
	msg1 := []core.ModelMessage{core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "first"}}}}
	msg2 := []core.ModelMessage{core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "second"}}}}

	key1 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: msg1})
	key2 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: msg2})
	if key1 == key2 {
		t.Fatal("stable keys matched for different user content")
	}
}

func TestStableCacheKey_NormalizesSafeOrderingNoise(t *testing.T) {
	params1 := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "beta", ParametersSchema: core.Schema{"type": "object"}},
			{Name: "alpha", ParametersSchema: core.Schema{
				"type":     "object",
				"required": []string{"zeta", "query"},
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"zeta":  map[string]any{"type": "integer"},
				},
			}},
		},
		AllowTextOutput: true,
	}
	params2 := &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{Name: "alpha", ParametersSchema: core.Schema{
				"properties": map[string]any{
					"zeta":  map[string]any{"type": "integer"},
					"query": map[string]any{"type": "string"},
				},
				"required": []string{"query", "zeta"},
				"type":     "object",
			}},
			{Name: "beta", ParametersSchema: core.Schema{"type": "object"}},
		},
		AllowTextOutput: true,
	}

	messages := []core.ModelMessage{core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "use a tool"}}}}
	key1 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: messages, Params: params1})
	key2 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: messages, Params: params2})
	if key1 != key2 {
		t.Fatalf("stable keys differ for safe ordering noise:\n%s\n%s", key1, key2)
	}
}

func TestStableCacheKey_PreservesSchemaPropertyNamesThatLookUnstable(t *testing.T) {
	messages := []core.ModelMessage{core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "extract"}}}}
	withTimestampProperty := &core.ModelRequestParameters{FunctionTools: []core.ToolDefinition{{
		Name: "extract",
		ParametersSchema: core.Schema{
			"type": "object",
			"properties": map[string]any{
				"timestamp": map[string]any{"type": "string"},
			},
		},
	}}}
	withoutTimestampProperty := &core.ModelRequestParameters{FunctionTools: []core.ToolDefinition{{
		Name: "extract",
		ParametersSchema: core.Schema{
			"type":       "object",
			"properties": map[string]any{},
		},
	}}}

	key1 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: messages, Params: withTimestampProperty})
	key2 := mustStableKey(t, StableCacheKeyInput{Model: "test-model", Messages: messages, Params: withoutTimestampProperty})
	if key1 == key2 {
		t.Fatal("stable keys matched after dropping a semantic schema property")
	}
}

func TestStableCacheKeyFromJSON_StripsProviderTransportOnlyFields(t *testing.T) {
	payload1 := map[string]any{
		"model":                  "gpt-4o-mini",
		"stream":                 false,
		"stream_options":         map[string]any{"include_usage": true},
		"prompt_cache_key":       "trace-a",
		"prompt_cache_retention": "24h",
		"messages": []any{
			map[string]any{"role": "user", "content": "Use lookup."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call-a",
						"type": "function",
						"function": map[string]any{
							"name":      "lookup",
							"arguments": `{"b":2,"a":1}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call-a", "content": "ok"},
		},
	}
	payload2 := map[string]any{
		"model":                  "gpt-4o-mini",
		"stream":                 true,
		"stream_options":         map[string]any{"include_usage": false},
		"prompt_cache_key":       "trace-b",
		"prompt_cache_retention": "1h",
		"messages": []any{
			map[string]any{"role": "user", "content": "Use lookup."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call-b",
						"type": "function",
						"function": map[string]any{
							"name":      "lookup",
							"arguments": `{"a":1,"b":2}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call-b", "content": "ok"},
		},
	}

	key1, err := StableCacheKeyFromJSON("openai", "gpt-4o-mini", payload1)
	if err != nil {
		t.Fatal(err)
	}
	key2, err := StableCacheKeyFromJSON("openai", "gpt-4o-mini", payload2)
	if err != nil {
		t.Fatal(err)
	}
	if key1 != key2 {
		t.Fatalf("stable keys differ for provider transport-only changes:\n%s\n%s", key1, key2)
	}
}

func mustStableKey(t *testing.T, input StableCacheKeyInput) string {
	t.Helper()
	key, err := StableCacheKey(input)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

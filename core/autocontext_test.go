package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAutoContext_NoCompression(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "short"}}},
	}

	config := &AutoContextConfig{
		MaxTokens: 10000, // way above the message size
		KeepLastN: 4,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(messages) {
		t.Errorf("expected %d messages (no compression), got %d", len(messages), len(result))
	}
}

func TestAutoContext_CompressesOld(t *testing.T) {
	// Create enough messages to exceed the token threshold.
	var messages []ModelMessage
	for range 20 {
		messages = append(messages, ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "This is a fairly long user message that has many words in it to inflate the token count above our threshold"},
			},
		})
		messages = append(messages, ModelResponse{
			Parts: []ModelResponsePart{TextPart{Content: "This is a long assistant response with plenty of content to drive up the estimated token count"}},
		})
	}

	summaryModel := NewTestModel(TextResponse("Summary of the conversation so far."))

	config := &AutoContextConfig{
		MaxTokens:    100, // very low threshold to force compression
		KeepLastN:    4,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should be compressed: 1 first msg + 1 summary + 4 recent = 6
	if len(result) != 6 {
		t.Errorf("expected 6 messages after compression, got %d", len(result))
	}
}

func TestAutoContext_KeepsRecent(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "old message 1 with lots of words to inflate token count"}}},
		ModelResponse{Parts: []ModelResponsePart{TextPart{Content: "old response 1 with many tokens"}}},
		ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "recent 1"}}},
		ModelResponse{Parts: []ModelResponsePart{TextPart{Content: "recent 2"}}},
	}

	summaryModel := NewTestModel(TextResponse("Summary"))
	config := &AutoContextConfig{
		MaxTokens:    5, // force compression
		KeepLastN:    2,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should be: 1 first msg + 1 summary + 2 recent = 4
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}

	// Last two should be the recent messages.
	if req, ok := result[2].(ModelRequest); ok {
		if up, ok := req.Parts[0].(UserPromptPart); ok {
			if up.Content != "recent 1" {
				t.Errorf("expected 'recent 1', got %q", up.Content)
			}
		}
	}
}

func TestAutoContext_CustomModel(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "old message with lots of words"}}},
		ModelResponse{Parts: []ModelResponsePart{TextPart{Content: "old response with lots of content"}}},
		ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "recent"}}},
		ModelResponse{Parts: []ModelResponsePart{TextPart{Content: "recent response"}}},
	}

	customModel := NewTestModel(TextResponse("Custom summary"))
	config := &AutoContextConfig{
		MaxTokens:    5,
		KeepLastN:    2,
		SummaryModel: customModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the summary uses the custom model (first msg + summary + 2 recent).
	// Summary is at index 1 (after the preserved first message) and is a ModelResponse.
	if resp, ok := result[1].(ModelResponse); ok {
		text := resp.TextContent()
		if text != "[Conversation Summary] Custom summary" {
			t.Errorf("expected custom summary, got %q", text)
		}
	} else {
		t.Errorf("expected summary at index 1 to be ModelResponse, got %T", result[1])
	}
}

func TestAutoContext_MessageAlternation(t *testing.T) {
	// Verify that compressed output maintains proper user/assistant alternation.
	// This is critical for Anthropic's API which requires strict alternation.
	// The bug: when the summary was emitted as a ModelRequest with SystemPromptPart,
	// Anthropic extracted it to the top-level system field, producing no API message.
	// This caused firstMsg (user) to be adjacent to recentMessages[0] (user).

	// Build a typical agent conversation: alternating request/response pairs.
	var messages []ModelMessage
	for i := range 10 {
		messages = append(messages, ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: fmt.Sprintf("user message %d with enough words to inflate token count", i)},
			},
		})
		messages = append(messages, ModelResponse{
			Parts: []ModelResponsePart{TextPart{Content: fmt.Sprintf("assistant response %d with enough words", i)}},
		})
	}

	summaryModel := NewTestModel(TextResponse("Summary of conversation"))
	config := &AutoContextConfig{
		MaxTokens:    10, // force compression
		KeepLastN:    4,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify alternation: ModelRequest, ModelResponse, ModelRequest, ModelResponse, ...
	for i, msg := range result {
		switch msg.(type) {
		case ModelRequest:
			if i > 0 {
				if _, prevIsReq := result[i-1].(ModelRequest); prevIsReq {
					t.Errorf("adjacent ModelRequest messages at indices %d and %d", i-1, i)
				}
			}
		case ModelResponse:
			if i > 0 {
				if _, prevIsResp := result[i-1].(ModelResponse); prevIsResp {
					t.Errorf("adjacent ModelResponse messages at indices %d and %d", i-1, i)
				}
			}
		}
	}

	// Verify the summary is at index 1 and is a ModelResponse.
	if _, ok := result[1].(ModelResponse); !ok {
		t.Errorf("expected summary at index 1 to be ModelResponse, got %T", result[1])
	}

	// Verify first message is preserved.
	if req, ok := result[0].(ModelRequest); ok {
		if up, ok := req.Parts[0].(UserPromptPart); ok {
			if !strings.Contains(up.Content, "user message 0") {
				t.Errorf("first message not preserved: %q", up.Content)
			}
		}
	}
}

func TestAutoContext_MessageAlternationOddKeepN(t *testing.T) {
	// Test with odd keepN where recentMessages would start with ModelResponse.
	// The boundary adjustment should include one extra message to maintain alternation.
	var messages []ModelMessage
	for i := range 10 {
		messages = append(messages, ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: fmt.Sprintf("user message %d with enough words to inflate token count", i)},
			},
		})
		messages = append(messages, ModelResponse{
			Parts: []ModelResponsePart{TextPart{Content: fmt.Sprintf("assistant response %d with enough words", i)}},
		})
	}
	// 20 messages total. With keepN=3, startRecent = 17 (odd index = ModelResponse).
	// The adjustment should move startRecent to 16 (ModelRequest).

	summaryModel := NewTestModel(TextResponse("Summary"))
	config := &AutoContextConfig{
		MaxTokens:    10,
		KeepLastN:    3,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify alternation.
	for i, msg := range result {
		switch msg.(type) {
		case ModelRequest:
			if i > 0 {
				if _, prevIsReq := result[i-1].(ModelRequest); prevIsReq {
					t.Errorf("adjacent ModelRequest at indices %d and %d", i-1, i)
				}
			}
		case ModelResponse:
			if i > 0 {
				if _, prevIsResp := result[i-1].(ModelResponse); prevIsResp {
					t.Errorf("adjacent ModelResponse at indices %d and %d", i-1, i)
				}
			}
		}
	}

	// First should be ModelRequest, second should be ModelResponse (summary).
	if _, ok := result[0].(ModelRequest); !ok {
		t.Errorf("expected index 0 to be ModelRequest, got %T", result[0])
	}
	if _, ok := result[1].(ModelResponse); !ok {
		t.Errorf("expected index 1 to be ModelResponse (summary), got %T", result[1])
	}
}

func TestAutoContext_EmptySummaryFallback(t *testing.T) {
	// When the summary model returns an empty response, autoCompressMessages
	// should fall back to returning original messages instead of creating a
	// near-empty summary that discards conversation history.
	var messages []ModelMessage
	for i := range 10 {
		messages = append(messages, ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: fmt.Sprintf("user message %d with enough words to inflate token count", i)},
			},
		})
		messages = append(messages, ModelResponse{
			Parts: []ModelResponsePart{TextPart{Content: fmt.Sprintf("assistant response %d with enough words", i)}},
		})
	}

	// Model returns empty text — simulates a failed/empty summarization.
	summaryModel := NewTestModel(TextResponse(""))
	config := &AutoContextConfig{
		MaxTokens:    10,
		KeepLastN:    4,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should return original messages unchanged since summary was empty.
	if len(result) != len(messages) {
		t.Errorf("expected %d messages (fallback to original), got %d", len(messages), len(result))
	}
}

func TestAutoContext_AgentIntegration(t *testing.T) {
	// Create a model that returns text responses.
	model := NewTestModel(TextResponse("result"))

	agent := NewAgent[string](model,
		WithAutoContext[string](AutoContextConfig{
			MaxTokens: 100000, // high threshold, no compression
			KeepLastN: 4,
		}),
	)

	result, err := agent.Run(context.Background(), "test auto context")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "result" {
		t.Errorf("expected 'result', got %q", result.Output)
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		wantLen  bool // true = check len <= maxBytes+3 (for "...")
	}{
		{"ascii_no_truncate", "hello world", 100, false},
		{"ascii_truncate", "hello world", 5, true},
		{"cjk_between_chars", "世界你好测试", 9, true}, // 世界你 = 9 bytes, clean boundary
		{"cjk_mid_char", "世界你好测试", 7, true},      // 7 is mid-char of 你 (starts at byte 6)
		{"emoji_mid_char", "Hello 🌍🌎🌏", 8, true}, // 8 is mid-emoji
		{"empty", "", 10, false},
		{"zero_max", "hello", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateStr(tt.input, tt.maxBytes)
			if !utf8.ValidString(result) {
				t.Errorf("result is not valid UTF-8: %q", result)
			}
			if tt.wantLen {
				// Should be truncated (contains "...")
				if !strings.HasSuffix(result, "...") {
					t.Errorf("expected truncated result to end with '...', got %q", result)
				}
			}
		})
	}
}

func TestAutoContext_CJKContent_UTF8Safety(t *testing.T) {
	// Verify that CJK/multi-byte content doesn't produce invalid UTF-8
	// during compression. This was a real bug: content[:500] splits multi-byte chars.
	cjk := strings.Repeat("错误信息：测试失败了，需要修复代码。", 30) // ~900 bytes of CJK
	var messages []ModelMessage
	for i := range 10 {
		messages = append(messages, ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: fmt.Sprintf("用户消息 %d: %s", i, cjk)},
				ToolReturnPart{ToolName: "bash", Content: cjk},
			},
		})
		messages = append(messages, ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: fmt.Sprintf("助手回复 %d: %s", i, cjk)},
				ToolCallPart{ToolName: "edit", ArgsJSON: fmt.Sprintf(`{"content":"%s"}`, cjk)},
			},
		})
	}

	summaryModel := NewTestModel(TextResponse("摘要：对话中讨论了代码修复。"))
	config := &AutoContextConfig{
		MaxTokens:    10,
		KeepLastN:    4,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all messages in the result contain valid UTF-8.
	for i, msg := range result {
		switch m := msg.(type) {
		case ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case UserPromptPart:
					if !utf8.ValidString(p.Content) {
						t.Errorf("message %d: UserPromptPart has invalid UTF-8", i)
					}
				}
			}
		case ModelResponse:
			if text := m.TextContent(); !utf8.ValidString(text) {
				t.Errorf("message %d: response text has invalid UTF-8", i)
			}
		}
	}
}

// TestStripOrphanedToolResults verifies that tool results whose matching tool
// calls were dropped are converted to user prompts rather than remaining as
// orphaned tool_result blocks that APIs would reject.
func TestStripOrphanedToolResults(t *testing.T) {
	messages := []ModelMessage{
		// First message with system prompt.
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Do the task"},
			},
		},
		// Summary (no tool calls).
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "[Summary] previous work"},
			},
		},
		// Orphaned tool result — the matching tool call was dropped.
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "view",
					ToolCallID: "orphan_1",
					Content:    "file contents here",
				},
			},
		},
		// Valid tool call.
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{
					ToolName:   "edit",
					ToolCallID: "valid_1",
					ArgsJSON:   `{"path":"foo.go"}`,
				},
			},
		},
		// Matching tool result (should be kept).
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "edit",
					ToolCallID: "valid_1",
					Content:    "edit applied",
				},
			},
		},
	}

	result := stripOrphanedToolResults(messages)

	// The orphaned tool result should be converted, not left as ToolReturnPart.
	for i, msg := range result {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if tr, ok := part.(ToolReturnPart); ok {
					if tr.ToolCallID == "orphan_1" {
						t.Errorf("message %d: orphaned ToolReturnPart with ID %q was not stripped", i, tr.ToolCallID)
					}
				}
			}
		}
	}

	// The valid tool result should still exist.
	foundValid := false
	for _, msg := range result {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if tr, ok := part.(ToolReturnPart); ok && tr.ToolCallID == "valid_1" {
					foundValid = true
				}
			}
		}
	}
	if !foundValid {
		t.Error("valid ToolReturnPart (valid_1) was incorrectly removed")
	}

	// The orphaned content should be preserved as a UserPromptPart.
	foundConverted := false
	for _, msg := range result {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if up, ok := part.(UserPromptPart); ok && strings.Contains(up.Content, "file contents here") {
					foundConverted = true
				}
			}
		}
	}
	if !foundConverted {
		t.Error("orphaned tool result content was not preserved as UserPromptPart")
	}
}

// TestAutoContext_ToolCallPairIntegrity verifies that auto-context compression
// does not produce orphaned tool results when tool call/result pairs span the
// compression boundary.
func TestAutoContext_ToolCallPairIntegrity(t *testing.T) {
	messages := []ModelMessage{
		// First message.
		ModelRequest{Parts: []ModelRequestPart{
			UserPromptPart{Content: "Implement the feature with lots of words to inflate tokens"},
		}},
	}

	// Add 6 tool call/result pairs to get enough messages.
	for i := range 6 {
		messages = append(messages, ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: fmt.Sprintf("Working on step %d with many extra words to inflate count", i)},
				ToolCallPart{
					ToolName:   "edit",
					ArgsJSON:   fmt.Sprintf(`{"path":"file%d.go"}`, i),
					ToolCallID: fmt.Sprintf("call_%d", i),
				},
			},
		})
		messages = append(messages, ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "edit",
					ToolCallID: fmt.Sprintf("call_%d", i),
					Content:    fmt.Sprintf("edit applied to file%d.go successfully with details", i),
				},
			},
		})
	}

	// Add final response.
	messages = append(messages, ModelResponse{
		Parts: []ModelResponsePart{TextPart{Content: "Done with implementation work"}},
	})

	summaryModel := NewTestModel(TextResponse("Summary of work done"))
	config := &AutoContextConfig{
		MaxTokens:    10, // very low to force compression
		KeepLastN:    4,
		SummaryModel: summaryModel,
	}

	result, err := autoCompressMessages(context.Background(), messages, config, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Collect all tool call IDs.
	callIDs := make(map[string]bool)
	for _, msg := range result {
		if resp, ok := msg.(ModelResponse); ok {
			for _, part := range resp.Parts {
				if tc, ok := part.(ToolCallPart); ok {
					callIDs[tc.ToolCallID] = true
				}
			}
		}
	}

	// Verify no orphaned tool results.
	for i, msg := range result {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if tr, ok := part.(ToolReturnPart); ok {
					if !callIDs[tr.ToolCallID] {
						t.Errorf("message %d has orphaned ToolReturnPart with ID %q", i, tr.ToolCallID)
					}
				}
			}
		}
	}

	// Verify proper alternation.
	for i := 1; i < len(result); i++ {
		_, prevReq := result[i-1].(ModelRequest)
		_, prevResp := result[i-1].(ModelResponse)
		_, curReq := result[i].(ModelRequest)
		_, curResp := result[i].(ModelResponse)
		if prevReq && curReq {
			t.Errorf("adjacent ModelRequests at %d and %d", i-1, i)
		}
		if prevResp && curResp {
			t.Errorf("adjacent ModelResponses at %d and %d", i-1, i)
		}
		_ = prevReq && prevResp
		_ = curReq && curResp
	}
}

// TestAutoContext_PersistentCompression verifies that auto-context compression
// persists into state.messages so the summary model is only called when
// compression triggers, not redundantly on every subsequent turn.
// Before this fix, state.messages grew unbounded and the summary model was
// called on every turn after the budget was first exceeded.
func TestAutoContext_PersistentCompression(t *testing.T) {
	// Track how many times the summary model is called.
	summaryModel := NewTestModel(TextResponse("Summary of prior work"))

	// Create a main model that generates several tool-call turns, then completes.
	// Turns 1-6: tool call → tool result (builds up history)
	// Turn 7: text response (completes)
	mainModel := NewTestModel(
		ToolCallResponseWithID("test_tool", `{"input":"step1"}`, "call_1"),
		ToolCallResponseWithID("test_tool", `{"input":"step2"}`, "call_2"),
		ToolCallResponseWithID("test_tool", `{"input":"step3"}`, "call_3"),
		ToolCallResponseWithID("test_tool", `{"input":"step4"}`, "call_4"),
		ToolCallResponseWithID("test_tool", `{"input":"step5"}`, "call_5"),
		ToolCallResponseWithID("test_tool", `{"input":"step6"}`, "call_6"),
		TextResponse("Done"),
	)

	type testInput struct {
		Input string `json:"input"`
	}

	// Simple tool that returns a string.
	testTool := FuncTool[testInput]("test_tool", "A test tool", func(ctx context.Context, params testInput) (string, error) {
		return "result: " + params.Input + strings.Repeat(" padding words to inflate token count", 50), nil
	})

	agent := NewAgent[string](mainModel,
		WithAutoContext[string](AutoContextConfig{
			MaxTokens:    50, // Very low to force compression after a few turns.
			KeepLastN:    4,
			SummaryModel: summaryModel,
		}),
		WithTools[string](testTool),
	)

	result, err := agent.Run(context.Background(), "Run all steps")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "Done" {
		t.Errorf("expected 'Done', got %q", result.Output)
	}

	numSummaryCalls := len(summaryModel.Calls())
	// With persistent compression, the summary model is called each time
	// state.messages exceeds the budget — but after compression, the
	// message count drops significantly. The key invariant is that the
	// number of summary calls is bounded relative to the number of turns,
	// not equal to the number of turns (which would happen without
	// persistence).
	numMainCalls := len(mainModel.Calls())
	if numSummaryCalls == 0 {
		t.Error("summary model was never called (compression should have triggered)")
	}
	if numSummaryCalls >= numMainCalls {
		t.Errorf("summary model called %d times (>= main model calls %d) — compression not persisting effectively", numSummaryCalls, numMainCalls)
	}
	t.Logf("main model calls: %d, summary model calls: %d", numMainCalls, numSummaryCalls)

	// Verify the result messages are compressed (fewer than the full
	// uncompressed history which would be 1 + 6*2 + 1 = 14 messages).
	if len(result.Messages) >= 14 {
		t.Errorf("expected compressed messages (< 14), got %d — compression not persisting", len(result.Messages))
	}
	t.Logf("final message count: %d", len(result.Messages))

	// Verify the result messages maintain proper alternation.
	for i := 1; i < len(result.Messages); i++ {
		_, prevReq := result.Messages[i-1].(ModelRequest)
		_, currReq := result.Messages[i].(ModelRequest)
		_, prevResp := result.Messages[i-1].(ModelResponse)
		_, currResp := result.Messages[i].(ModelResponse)
		if prevReq && currReq {
			t.Errorf("adjacent ModelRequests at %d and %d", i-1, i)
		}
		if prevResp && currResp {
			t.Errorf("adjacent ModelResponses at %d and %d", i-1, i)
		}
	}
}

// TestStripOrphanedToolResults_NoConsecutiveMessages verifies that stripping
// orphaned tool results does not create consecutive same-role messages.
// When a ModelRequest contains ONLY orphaned tool results (e.g., the tool
// result content is empty or non-string), the message must be kept with a
// placeholder rather than dropped — dropping it creates consecutive
// ModelResponse (assistant) messages that violate API alternation requirements.
func TestStripOrphanedToolResults_NoConsecutiveMessages(t *testing.T) {
	messages := []ModelMessage{
		// First message.
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Do the task"},
			},
		},
		// Summary (assistant role).
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "[Summary] previous work"},
			},
		},
		// Orphaned tool result with non-string content — converting gives empty
		// string, so previously this ModelRequest would be dropped entirely.
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "bash",
					ToolCallID: "orphan_1",
					Content:    42, // non-string content
				},
			},
		},
		// Next response (assistant role) — if the above ModelRequest is dropped,
		// this becomes consecutive with the summary ModelResponse.
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{
					ToolName:   "edit",
					ToolCallID: "valid_1",
					ArgsJSON:   `{"path":"foo.go"}`,
				},
			},
		},
		// Valid tool result.
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "edit",
					ToolCallID: "valid_1",
					Content:    "edit applied",
				},
			},
		},
	}

	result := stripOrphanedToolResults(messages)

	// Verify no consecutive same-role messages.
	for i := 1; i < len(result); i++ {
		_, prevIsReq := result[i-1].(ModelRequest)
		_, currIsReq := result[i].(ModelRequest)
		_, prevIsResp := result[i-1].(ModelResponse)
		_, currIsResp := result[i].(ModelResponse)
		if prevIsReq && currIsReq {
			t.Errorf("consecutive ModelRequest messages at indices %d and %d — would cause Anthropic 400", i-1, i)
		}
		if prevIsResp && currIsResp {
			t.Errorf("consecutive ModelResponse messages at indices %d and %d — would cause Anthropic 400", i-1, i)
		}
	}

	// The result should still contain 5 messages (none dropped).
	if len(result) != 5 {
		t.Errorf("expected 5 messages (placeholder for orphaned), got %d", len(result))
	}
}

// TestStripOrphanedToolResults_NonStringContent verifies that orphaned
// ToolReturnParts with non-string content (maps, slices, numbers) are
// preserved as UserPromptParts. Previously, only string content was
// converted — structured data was silently dropped.
func TestStripOrphanedToolResults_NonStringContent(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Do the task"},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "[Summary] previous work"},
			},
		},
		// Orphaned tool result with structured (map) content.
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "api_call",
					ToolCallID: "orphan_map",
					Content:    map[string]any{"status": "ok", "count": float64(42)},
				},
			},
		},
		// Orphaned tool result with numeric content.
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "Continuing work"},
			},
		},
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "counter",
					ToolCallID: "orphan_num",
					Content:    float64(99),
				},
			},
		},
	}

	result := stripOrphanedToolResults(messages)

	// The structured content should be preserved as JSON in UserPromptParts.
	foundMap := false
	foundNum := false
	for _, msg := range result {
		if req, ok := msg.(ModelRequest); ok {
			for _, part := range req.Parts {
				if up, ok := part.(UserPromptPart); ok {
					if strings.Contains(up.Content, "api_call") && strings.Contains(up.Content, "count") {
						foundMap = true
					}
					if strings.Contains(up.Content, "counter") && strings.Contains(up.Content, "99") {
						foundNum = true
					}
				}
			}
		}
	}
	if !foundMap {
		t.Error("orphaned tool result with map content was not preserved as UserPromptPart")
	}
	if !foundNum {
		t.Error("orphaned tool result with numeric content was not preserved as UserPromptPart")
	}
}

// TestAutoContext_IterPersistence verifies that auto-context compression in the
// Iter API persists to state.messages, preventing unbounded growth. This catches
// the bug fixed in 387a2b9 where compression was transient — applied to a local
// copy rather than persisted back to ar.state.messages.
func TestAutoContext_IterPersistence(t *testing.T) {
	// Create a model that returns:
	// 1-5: tool calls (to build up many messages via tool call/result pairs)
	// 6+: final text response (repeats last response)
	var responses []*ModelResponse
	for i := range 5 {
		responses = append(responses, ToolCallResponseWithID(
			"echo",
			fmt.Sprintf(`{"text":"message %d with enough content to inflate token estimates above our threshold budget"}`, i),
			fmt.Sprintf("call_%d", i),
		))
	}
	responses = append(responses, TextResponse("done"))
	mainModel := NewTestModel(responses...)

	summaryModel := NewTestModel(TextResponse("Summary: several echo tool calls were made."))

	// A simple echo tool that returns its input.
	echoTool := FuncTool[struct {
		Text string `json:"text"`
	}]("echo", "Echo text", func(_ context.Context, _ *RunContext, p struct {
		Text string `json:"text"`
	}) (string, error) {
		return "Echo: " + p.Text, nil
	})

	agent := NewAgent[string](mainModel,
		WithTools[string](echoTool),
		WithAutoContext[string](AutoContextConfig{
			MaxTokens:    50, // Very low to force compression early.
			KeepLastN:    4,
			SummaryModel: summaryModel,
		}),
	)

	ctx := context.Background()
	iter := agent.Iter(ctx, "Run echo tool 5 times")

	var maxMsgCount int
	var steps int
	for !iter.Done() {
		_, err := iter.Next()
		if err != nil {
			break
		}
		steps++
		msgCount := len(iter.Messages())
		if msgCount > maxMsgCount {
			maxMsgCount = msgCount
		}
	}

	if steps == 0 {
		t.Fatal("expected at least one iteration step")
	}

	finalMsgCount := len(iter.Messages())

	// With 5 tool calls, without compression we'd have ~12 messages:
	// initial request + (5 * (response + request)) + final response = 12
	// With persistent compression, the count should be bounded.
	// KeepLastN=4 + first message + summary = ~6.
	//
	// If compression is NOT persisted (the old bug), message count would
	// stay at ~12 because each call would re-compress a local copy but
	// never update state.messages. The summary model would also be called
	// on every subsequent turn (wasting tokens) instead of once.
	t.Logf("steps=%d maxMsgCount=%d finalMsgCount=%d", steps, maxMsgCount, finalMsgCount)

	// The key assertion: final message count should be less than max.
	// With persistent compression, messages are compressed mid-run and
	// stay compressed. Without it, they'd only ever grow.
	if finalMsgCount >= 10 {
		t.Errorf("expected compression to reduce message count below 10 (got %d) — auto-context compression may not be persisted in Iter API", finalMsgCount)
	}

	// Verify the summary model was called (compression happened).
	summaryCalls := summaryModel.Calls()
	if len(summaryCalls) == 0 {
		t.Error("expected summary model to be called for auto-context compression")
	}

	// Verify result is obtainable.
	result, err := iter.Result()
	if err != nil {
		t.Fatalf("iter.Result() failed: %v", err)
	}
	if result.Output != "done" {
		t.Errorf("expected output 'done', got %q", result.Output)
	}
}

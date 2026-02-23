package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
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

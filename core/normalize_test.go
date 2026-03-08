package core

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeHistory_OrphanedToolReturn(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Hello"},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "Hi"},
			},
		},
		ModelRequest{
			Parts: []ModelRequestPart{
				// Orphaned: no matching tool call in any response.
				ToolReturnPart{ToolName: "tool_x", ToolCallID: "orphan_1", Content: "result"},
			},
		},
	}

	proc := NormalizeHistory()
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}
	// The orphaned ToolReturnPart's request should be dropped (empty after filter).
	if len(result) != 2 {
		t.Errorf("expected 2 messages after removing orphaned tool return, got %d", len(result))
	}
}

func TestNormalizeHistory_ValidToolReturn(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Hello"},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{ToolName: "search", ToolCallID: "call_1", ArgsJSON: "{}"},
			},
		},
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{ToolName: "search", ToolCallID: "call_1", Content: "results"},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "Done"},
			},
		},
	}

	proc := NormalizeHistory()
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}
	// All 4 messages should be preserved.
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}
}

func TestNormalizeHistory_ImageStripping(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Hello"},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{ToolName: "screenshot", ToolCallID: "call_1", ArgsJSON: "{}"},
			},
		},
		// Old turn with images — should be stripped.
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "screenshot",
					ToolCallID: "call_1",
					Content:    "screenshot taken",
					Images:     []ImagePart{{URL: "data:image/png;base64,abc"}},
				},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{ToolName: "screenshot", ToolCallID: "call_2", ArgsJSON: "{}"},
			},
		},
		// Current turn with images — should be preserved.
		ModelRequest{
			Parts: []ModelRequestPart{
				ToolReturnPart{
					ToolName:   "screenshot",
					ToolCallID: "call_2",
					Content:    "screenshot taken again",
					Images:     []ImagePart{{URL: "data:image/png;base64,def"}},
				},
			},
		},
	}

	proc := NormalizeHistory()
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(result))
	}

	// Old turn (index 2) images should be cleared.
	oldReq := result[2].(ModelRequest)
	oldTR := oldReq.Parts[0].(ToolReturnPart)
	if len(oldTR.Images) != 0 {
		t.Errorf("expected old turn images cleared, got %d images", len(oldTR.Images))
	}

	// Current turn (index 4, last request) images should be preserved.
	curReq := result[4].(ModelRequest)
	curTR := curReq.Parts[0].(ToolReturnPart)
	if len(curTR.Images) != 1 {
		t.Errorf("expected current turn images preserved, got %d images", len(curTR.Images))
	}
}

func TestNormalizeHistory_EmptyHistory(t *testing.T) {
	proc := NormalizeHistory()
	result, err := proc(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestNormalizeHistory_NoOrphans(t *testing.T) {
	now := time.Now()
	messages := []ModelMessage{
		ModelRequest{
			Parts:     []ModelRequestPart{UserPromptPart{Content: "Hello"}},
			Timestamp: now,
		},
		ModelResponse{
			Parts:     []ModelResponsePart{TextPart{Content: "Hi"}},
			Timestamp: now,
		},
	}

	proc := NormalizeHistory()
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 messages unchanged, got %d", len(result))
	}
}

package core

import (
	"context"
	"strings"
	"testing"
)

func TestBuildInstructionPin_Basic(t *testing.T) {
	messages := []ModelMessage{
		ModelRequest{
			Parts: []ModelRequestPart{
				SystemPromptPart{Content: "You are a coding agent."},
				SystemPromptPart{Content: "Never skip validation."},
				UserPromptPart{Content: "Implement the parser and keep CSV headers unchanged."},
			},
		},
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "I will inspect parser.go first."},
			},
		},
		ModelRequest{
			Parts: []ModelRequestPart{
				UserPromptPart{Content: "Also preserve output format exactly."},
			},
		},
	}

	pin := BuildInstructionPin(messages)
	if pin == "" {
		t.Fatal("expected non-empty instruction pin")
	}
	for _, want := range []string{
		"[Instruction Pin]",
		"System directives:",
		"S1: You are a coding agent.",
		"S2: Never skip validation.",
		"Original task: Implement the parser and keep CSV headers unchanged.",
		"Latest user instruction: Also preserve output format exactly.",
	} {
		if !strings.Contains(pin, want) {
			t.Errorf("pin missing %q\npin:\n%s", want, pin)
		}
	}
}

func TestAutoContext_SummaryIncludesInstructionPin(t *testing.T) {
	var messages []ModelMessage
	messages = append(messages, ModelRequest{
		Parts: []ModelRequestPart{
			SystemPromptPart{Content: "You are precise and must keep exact output formats."},
			UserPromptPart{Content: "Solve the task and preserve schema."},
		},
	})
	for range 8 {
		messages = append(messages,
			ModelResponse{
				Parts: []ModelResponsePart{
					TextPart{Content: "assistant response with enough words to inflate token estimation significantly"},
				},
			},
			ModelRequest{
				Parts: []ModelRequestPart{
					UserPromptPart{Content: "user follow-up with enough words to keep this conversation long"},
				},
			},
		)
	}
	messages = append(messages, ModelRequest{
		Parts: []ModelRequestPart{
			UserPromptPart{Content: "Final instruction: keep column order unchanged."},
		},
	})

	config := &AutoContextConfig{
		MaxTokens:    25, // force compression
		KeepLastN:    4,
		SummaryModel: NewTestModel(TextResponse("summary body")),
	}

	compressed, err := autoCompressMessages(context.Background(), messages, config, nil, estimateTokens(messages))
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) < 2 {
		t.Fatalf("expected compressed history with summary, got %d messages", len(compressed))
	}

	summary, ok := compressed[1].(ModelResponse)
	if !ok {
		t.Fatalf("expected message[1] to be ModelResponse summary, got %T", compressed[1])
	}

	text := summary.TextContent()
	if !strings.Contains(text, "[Instruction Pin]") {
		t.Fatalf("expected instruction pin in summary, got:\n%s", text)
	}
	if !strings.Contains(text, "Original task: Solve the task and preserve schema.") {
		t.Fatalf("expected original task in instruction pin, got:\n%s", text)
	}
	if !strings.Contains(text, "Latest user instruction: Final instruction: keep column order unchanged.") {
		t.Fatalf("expected latest user instruction in pin, got:\n%s", text)
	}
}

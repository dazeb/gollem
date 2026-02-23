package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AutoContextConfig configures automatic context window management.
type AutoContextConfig struct {
	MaxTokens    int   // maximum token estimate before summarization
	KeepLastN    int   // number of recent messages to always keep (default: 4)
	SummaryModel Model // model to use for summarization (optional, uses agent model if nil)
}

// WithAutoContext enables automatic context window management.
// When estimated tokens exceed MaxTokens, older messages are summarized.
func WithAutoContext[T any](config AutoContextConfig) AgentOption[T] {
	if config.KeepLastN <= 0 {
		config.KeepLastN = 4
	}
	return func(a *Agent[T]) {
		a.autoContext = &config
	}
}

// estimateTokens estimates the token count of messages using a simple word-based heuristic.
// Uses ~1.3 tokens per word as a rough approximation.
func estimateTokens(messages []ModelMessage) int {
	total := 0
	for _, msg := range messages {
		switch m := msg.(type) {
		case ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case SystemPromptPart:
					total += estimateStringTokens(p.Content)
				case UserPromptPart:
					total += estimateStringTokens(p.Content)
				case ToolReturnPart:
					if s, ok := p.Content.(string); ok {
						total += estimateStringTokens(s)
					}
				case RetryPromptPart:
					total += estimateStringTokens(p.Content)
				}
			}
		case ModelResponse:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case TextPart:
					total += estimateStringTokens(p.Content)
				case ToolCallPart:
					total += estimateStringTokens(p.ArgsJSON)
				case ThinkingPart:
					total += estimateStringTokens(p.Content)
				}
			}
		}
	}
	return total
}

// estimateStringTokens estimates token count for a string.
func estimateStringTokens(s string) int {
	words := len(strings.Fields(s))
	// ~1.3 tokens per word
	return int(float64(words) * 1.3)
}

// autoCompressMessages summarizes old messages to fit within the token budget.
func autoCompressMessages(ctx context.Context, messages []ModelMessage, config *AutoContextConfig, fallbackModel Model) ([]ModelMessage, error) {
	estimated := estimateTokens(messages)
	if estimated <= config.MaxTokens {
		return messages, nil
	}

	// Keep the last N messages AND the first message (which contains the
	// task description, system prompt, and environment context). Losing
	// the original task requirements is a common failure mode.
	keepN := config.KeepLastN
	if keepN >= len(messages) {
		return messages, nil // can't compress further
	}

	// Always preserve the first message (task + system prompt).
	// Summarize messages[1:len-keepN], keep messages[0] and messages[len-keepN:].
	firstMsg := messages[0]
	oldMessages := messages[1 : len(messages)-keepN]
	recentMessages := messages[len(messages)-keepN:]

	if len(oldMessages) == 0 {
		return messages, nil // nothing to compress
	}

	// Build a summary of old messages.
	summaryModel := config.SummaryModel
	if summaryModel == nil {
		summaryModel = fallbackModel
	}

	// Build summary prompt that includes tool calls and results,
	// not just user/assistant text. This preserves critical context
	// about what files were modified, what tests were run, and what
	// errors occurred.
	var sb strings.Builder
	sb.WriteString("Summarize this conversation concisely, preserving:\n")
	sb.WriteString("- What files were created, edited, or read (include EXACT file paths)\n")
	sb.WriteString("- What commands were run and their results (especially test results with pass/fail COUNTS and specific error messages)\n")
	sb.WriteString("- Key decisions made and current approach being used\n")
	sb.WriteString("- Any constraints or requirements discovered (include exact values: sizes, thresholds, formats)\n")
	sb.WriteString("- What approaches were tried and whether they succeeded or failed (include WHY they failed)\n")
	sb.WriteString("- BEST RESULT SO FAR: what was the best test pass count or closest-to-correct output, and what exact code/config produced it\n")
	sb.WriteString("- Current state: what's done, what's remaining, and what the NEXT STEP should be\n")
	sb.WriteString("- CRITICAL: preserve any discovered format requirements (JSON schema, CSV headers, encoding, trailing newlines)\n\n")
	for _, msg := range oldMessages {
		switch m := msg.(type) {
		case ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case UserPromptPart:
					content := p.Content
					if len(content) > 500 {
						content = content[:500] + "..."
					}
					fmt.Fprintf(&sb, "User: %s\n", content)
				case ToolReturnPart:
					content := fmt.Sprintf("%v", p.Content)
					if len(content) > 800 {
						content = content[:800] + "..."
					}
					fmt.Fprintf(&sb, "[Tool result: %s] %s\n", p.ToolName, content)
				}
			}
		case ModelResponse:
			if text := m.TextContent(); text != "" {
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				fmt.Fprintf(&sb, "Assistant: %s\n", text)
			}
			for _, part := range m.Parts {
				if tc, ok := part.(ToolCallPart); ok {
					args := tc.ArgsJSON
					if len(args) > 500 {
						args = args[:500] + "..."
					}
					fmt.Fprintf(&sb, "[Tool call: %s] %s\n", tc.ToolName, args)
				}
			}
		}
	}

	summaryReq := []ModelMessage{
		ModelRequest{
			Parts:     []ModelRequestPart{UserPromptPart{Content: sb.String(), Timestamp: time.Now()}},
			Timestamp: time.Now(),
		},
	}

	summaryResp, err := summaryModel.Request(ctx, summaryReq, nil, &ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return messages, err
	}

	// Build new message list: first message + summary + recent messages.
	summaryMsg := ModelRequest{
		Parts: []ModelRequestPart{
			SystemPromptPart{
				Content:   "[Conversation Summary] " + summaryResp.TextContent(),
				Timestamp: time.Now(),
			},
		},
		Timestamp: time.Now(),
	}

	result := make([]ModelMessage, 0, 2+len(recentMessages))
	result = append(result, firstMsg)
	result = append(result, summaryMsg)
	result = append(result, recentMessages...)
	return result, nil
}

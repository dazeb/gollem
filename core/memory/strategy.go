package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// SlidingWindowMemory keeps only the last N message pairs (request + response).
// Always preserves the first message (system prompt) and the last message.
func SlidingWindowMemory(windowSize int) core.HistoryProcessor {
	return func(ctx context.Context, messages []core.ModelMessage) ([]core.ModelMessage, error) {
		if len(messages) <= windowSize*2+1 {
			// Under the window, return as-is.
			return messages, nil
		}

		// Keep first message (system prompt/initial request), then last windowSize*2 messages.
		start := len(messages) - windowSize*2
		if start < 1 {
			start = 1
		}

		// The first message is a ModelRequest (user role). The kept tail must
		// start with a ModelResponse (assistant role) to maintain proper
		// user/assistant alternation required by Anthropic's API.
		if start > 1 {
			if _, isReq := messages[start].(core.ModelRequest); isReq {
				start--
			}
		}

		result := make([]core.ModelMessage, 0, 1+len(messages)-start)
		result = append(result, messages[0])
		result = append(result, messages[start:]...)
		return result, nil
	}
}

// TokenBudgetMemory keeps messages within an approximate token budget,
// dropping the oldest messages (after the system prompt) first.
// Token estimation: ~4 characters per token.
func TokenBudgetMemory(maxTokens int) core.HistoryProcessor {
	return func(ctx context.Context, messages []core.ModelMessage) ([]core.ModelMessage, error) {
		total := estimateMessageTokens(messages)
		if total <= maxTokens {
			return messages, nil
		}

		// Always keep first and last messages.
		if len(messages) <= 2 {
			return messages, nil
		}

		// Drop messages in pairs from position 1 until under budget.
		// Dropping pairs (request + response together) maintains proper
		// user/assistant alternation required by Anthropic's API.
		result := make([]core.ModelMessage, len(messages))
		copy(result, messages)

		for len(result) > 3 && estimateMessageTokens(result) > maxTokens {
			// Drop two messages (a pair) to maintain alternation.
			result = append(result[:1], result[3:]...)
		}

		return result, nil
	}
}

// estimateMessageTokens estimates total tokens in a message list.
// Uses a simple heuristic of ~4 characters per token.
func estimateMessageTokens(messages []core.ModelMessage) int {
	total := 0
	for _, msg := range messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.SystemPromptPart:
					total += len(p.Content) / 4
				case core.UserPromptPart:
					total += len(p.Content) / 4
				case core.ToolReturnPart:
					if s, ok := p.Content.(string); ok {
						total += len(s) / 4
					} else {
						total += 50 // estimate for structured content
					}
				case core.RetryPromptPart:
					total += len(p.Content) / 4
				}
			}
		case core.ModelResponse:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.TextPart:
					total += len(p.Content) / 4
				case core.ToolCallPart:
					total += len(p.ArgsJSON) / 4
				case core.ThinkingPart:
					total += len(p.Content) / 4
				}
			}
		}
	}
	if total == 0 {
		total = 1
	}
	return total
}

// SummaryMemory summarizes older messages when the conversation exceeds
// maxMessages, using the provided model to generate summaries. The summary
// replaces the dropped messages as a system prompt.
func SummaryMemory(summarizer core.Model, maxMessages int) core.HistoryProcessor {
	return func(ctx context.Context, messages []core.ModelMessage) ([]core.ModelMessage, error) {
		if len(messages) <= maxMessages {
			return messages, nil
		}

		// Split: keep first message, summarize middle, keep last maxMessages/2 messages.
		keepLast := maxMessages / 2
		if keepLast < 1 {
			keepLast = 1
		}
		if keepLast >= len(messages) {
			return messages, nil
		}

		toSummarize := messages[1 : len(messages)-keepLast]
		if len(toSummarize) == 0 {
			return messages, nil
		}

		// Build a summarization prompt.
		var sb strings.Builder
		for _, msg := range toSummarize {
			switch m := msg.(type) {
			case core.ModelRequest:
				for _, part := range m.Parts {
					if up, ok := part.(core.UserPromptPart); ok {
						sb.WriteString("User: ")
						sb.WriteString(up.Content)
						sb.WriteString("\n")
					}
				}
			case core.ModelResponse:
				text := m.TextContent()
				if text != "" {
					sb.WriteString("Assistant: ")
					sb.WriteString(text)
					sb.WriteString("\n")
				}
			}
		}
		summaryText := sb.String()

		// Ask the summarizer model to summarize.
		summaryReq := core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{
					Content:   "Summarize this conversation concisely:\n" + summaryText,
					Timestamp: time.Now(),
				},
			},
			Timestamp: time.Now(),
		}

		resp, err := summarizer.Request(ctx, []core.ModelMessage{summaryReq}, nil, &core.ModelRequestParameters{
			AllowTextOutput: true,
		})
		if err != nil {
			return nil, fmt.Errorf("summary memory: %w", err)
		}

		summary := resp.TextContent()
		if summary == "" {
			summary = "(conversation summary unavailable)"
		}

		// Reconstruct: first message + summary (assistant role) + recent messages.
		// The summary is emitted as a ModelResponse (assistant role) so that
		// providers that extract SystemPromptPart to a top-level field (e.g.,
		// Anthropic) still produce an API message, maintaining proper
		// user/assistant alternation.
		recentStart := len(messages) - keepLast

		// Ensure recent messages start with a ModelRequest (user role) since
		// the summary is a ModelResponse (assistant role).
		if recentStart > 1 {
			if _, isResp := messages[recentStart].(core.ModelResponse); isResp {
				recentStart--
			}
		}

		result := make([]core.ModelMessage, 0, 2+len(messages)-recentStart)
		result = append(result, messages[0])
		result = append(result, core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.TextPart{Content: "[Conversation Summary] " + summary},
			},
			Timestamp: time.Now(),
		})
		result = append(result, messages[recentStart:]...)

		return result, nil
	}
}

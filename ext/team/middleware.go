package team

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// TeamAwarenessMiddleware drains the teammate's mailbox before each model
// request and injects pending messages as a UserPromptPart. This ensures
// the agent sees messages from other teammates between turns.
func TeamAwarenessMiddleware(tm *Teammate) core.AgentMiddleware {
	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		// Drain any pending messages.
		pending := tm.mailbox.DrainAll()
		if len(pending) == 0 {
			return next(ctx, messages, settings, params)
		}

		// Format messages and inject as UserPromptPart.
		var parts []string
		hasShutdown := false
		for _, msg := range pending {
			if msg.Type == MessageShutdownRequest {
				hasShutdown = true
				parts = append(parts, fmt.Sprintf("[SHUTDOWN REQUEST from %s]: %s — Wrap up your current work and finish.", msg.From, msg.Content))
			} else {
				parts = append(parts, fmt.Sprintf("[Message from %s]: %s", msg.From, msg.Content))
			}
		}

		injection := strings.Join(parts, "\n\n")
		if hasShutdown {
			injection += "\n\nIMPORTANT: A shutdown has been requested. Complete your current task as quickly as possible and provide your final response."
		}

		injectedMsg := core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: injection},
			},
		}

		// Copy to avoid corrupting the caller's slice backing array.
		newMessages := make([]core.ModelMessage, len(messages)+1)
		copy(newMessages, messages)
		newMessages[len(messages)] = injectedMsg

		return next(ctx, newMessages, settings, params)
	}
}

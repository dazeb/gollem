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

		// Merge into the last ModelRequest to avoid consecutive user-role
		// messages, which cause a 400 error from Anthropic's API.
		newMessages := make([]core.ModelMessage, len(messages))
		copy(newMessages, messages)
		merged := false
		for i := len(newMessages) - 1; i >= 0; i-- {
			if req, ok := newMessages[i].(core.ModelRequest); ok {
				newParts := make([]core.ModelRequestPart, len(req.Parts)+1)
				copy(newParts, req.Parts)
				newParts[len(req.Parts)] = core.UserPromptPart{Content: injection}
				req.Parts = newParts
				newMessages[i] = req
				merged = true
				break
			}
		}
		if !merged {
			// Fallback: no ModelRequest found, append a new one.
			newMessages = append(newMessages, core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: injection},
				},
			})
		}

		return next(ctx, newMessages, settings, params)
	}
}

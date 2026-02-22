package codetool

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// ReasoningSandwichConfig configures the reasoning sandwich middleware.
type ReasoningSandwichConfig struct {
	// PlanningBudget is the thinking token budget for early turns (planning/discovery).
	// Higher budget allows deeper analysis of the task and codebase.
	PlanningBudget int

	// ImplementationBudget is the thinking token budget for middle turns (building).
	// Lower budget keeps implementation turns fast.
	ImplementationBudget int

	// VerificationBudget is the thinking token budget for late turns (verification/fix).
	// Higher budget allows careful analysis of test results and errors.
	VerificationBudget int

	// PlanningTurns is the number of early turns that get the planning budget.
	PlanningTurns int

	// VerificationThreshold is the turn number (from the end of max turns) at which
	// we switch to the verification budget. If we don't know max turns, we use
	// a heuristic: switch when verification commands are detected in recent output.
	VerificationThreshold int
}

// DefaultReasoningSandwichConfig returns a balanced reasoning sandwich config.
// Planning: 32k tokens for deep analysis (first 5 turns)
// Implementation: 10k tokens for fast execution (middle turns)
// Verification: 32k tokens for careful error analysis (last turns or when testing)
func DefaultReasoningSandwichConfig() ReasoningSandwichConfig {
	return ReasoningSandwichConfig{
		PlanningBudget:        32000,
		ImplementationBudget:  10000,
		VerificationBudget:    32000,
		PlanningTurns:         5,
		VerificationThreshold: 0, // Use heuristic: detect verification commands
	}
}

// ReasoningSandwichMiddleware implements the "reasoning sandwich" pattern:
// high thinking budget for planning → lower for implementation → high for verification.
//
// This technique was shown to improve Terminal-Bench scores by +10pts in
// LangChain's harness engineering work. The key insight: LLMs benefit from
// more reasoning compute during planning and verification phases, but waste
// time/tokens during straightforward implementation.
func ReasoningSandwichMiddleware(cfg ReasoningSandwichConfig) core.AgentMiddleware {
	var mu sync.Mutex
	turn := 0
	inVerification := false

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		mu.Lock()
		turn++
		currentTurn := turn

		// Detect if we're in verification phase by checking recent tool calls.
		if !inVerification {
			inVerification = detectVerificationPhase(messages)
		}
		isVerifying := inVerification
		mu.Unlock()

		// Only modify settings if thinking budget is configured.
		if settings != nil && settings.ThinkingBudget != nil {
			s := *settings
			var newBudget int
			var phase string

			switch {
			case currentTurn <= cfg.PlanningTurns:
				newBudget = cfg.PlanningBudget
				phase = "planning"
			case isVerifying:
				newBudget = cfg.VerificationBudget
				phase = "verification"
			default:
				newBudget = cfg.ImplementationBudget
				phase = "implementation"
			}

			s.ThinkingBudget = &newBudget
			// Ensure max_tokens > thinking budget (Anthropic requirement).
			if s.MaxTokens != nil && *s.MaxTokens <= newBudget {
				maxT := newBudget + 16000
				s.MaxTokens = &maxT
			}

			fmt.Fprintf(os.Stderr, "[gollem] reasoning: turn %d phase=%s budget=%d\n",
				currentTurn, phase, newBudget)

			return next(ctx, messages, &s, params)
		}

		return next(ctx, messages, settings, params)
	}
}

// detectVerificationPhase checks if recent messages contain verification commands.
func detectVerificationPhase(messages []core.ModelMessage) bool {
	// Check the last few messages for test/build commands.
	start := len(messages) - 4
	if start < 0 {
		start = 0
	}
	for _, msg := range messages[start:] {
		if resp, ok := msg.(core.ModelResponse); ok {
			for _, part := range resp.Parts {
				if tc, ok := part.(core.ToolCallPart); ok && tc.ToolName == "bash" {
					if isVerificationCommand(tc.ArgsJSON) {
						return true
					}
				}
			}
		}
	}
	return false
}

// TimeBudgetMiddleware injects time-remaining warnings into the conversation
// when the agent is approaching its timeout. This helps the agent prioritize
// completing its work rather than spending time on perfection.
func TimeBudgetMiddleware(timeout time.Duration) core.AgentMiddleware {
	startTime := time.Now()
	warned50 := false
	warned75 := false
	warned90 := false

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		elapsed := time.Since(startTime)
		pct := float64(elapsed) / float64(timeout)
		remaining := timeout - elapsed

		var warning string
		switch {
		case pct >= 0.90 && !warned90:
			warned90 = true
			warning = fmt.Sprintf("⚠️ TIME CRITICAL: Only %s remaining (%.0f%% elapsed). "+
				"Wrap up NOW: run final tests, clean up artifacts, and complete the task. "+
				"Do not start new approaches.", remaining.Round(time.Second), pct*100)
		case pct >= 0.75 && !warned75:
			warned75 = true
			warning = fmt.Sprintf("⏰ Time warning: %s remaining (%.0f%% elapsed). "+
				"Start wrapping up. Focus on verifying your current approach works "+
				"rather than trying alternatives.", remaining.Round(time.Second), pct*100)
		case pct >= 0.50 && !warned50:
			warned50 = true
			warning = fmt.Sprintf("⏰ Halfway point: %s remaining (%.0f%% elapsed). "+
				"If your current approach isn't working, consider switching strategies now.",
				remaining.Round(time.Second), pct*100)
		}

		if warning != "" {
			fmt.Fprintf(os.Stderr, "[gollem] time: %s\n", warning)
			timeMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: warning},
				},
			}
			messages = append(messages, timeMsg)
		}

		return next(ctx, messages, settings, params)
	}
}

package codetool

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// ReasoningLevel defines a provider-agnostic reasoning intensity.
// It maps to ThinkingBudget (Anthropic) and ReasoningEffort (OpenAI).
type ReasoningLevel struct {
	// ThinkingBudget is the token budget for Anthropic extended thinking.
	ThinkingBudget int
	// ReasoningEffort is the effort level for OpenAI o-series ("low", "medium", "high").
	ReasoningEffort string
}

// ReasoningSandwichConfig configures the reasoning sandwich middleware.
// This is provider-agnostic: it sets both ThinkingBudget (Anthropic) and
// ReasoningEffort (OpenAI) for each phase. Each provider uses whichever
// field applies to it.
type ReasoningSandwichConfig struct {
	// Planning is the reasoning level for early turns (planning/discovery).
	// Higher reasoning allows deeper analysis of the task and codebase.
	Planning ReasoningLevel

	// Implementation is the reasoning level for middle turns (building).
	// Lower reasoning keeps implementation turns fast.
	Implementation ReasoningLevel

	// Verification is the reasoning level for late turns (verification/fix).
	// Higher reasoning allows careful analysis of test results and errors.
	Verification ReasoningLevel

	// PlanningTurns is the number of early turns that get the planning level.
	PlanningTurns int

	// VerificationThreshold is the turn number (from the end of max turns) at which
	// we switch to the verification level. If we don't know max turns, we use
	// a heuristic: switch when verification commands are detected in recent output.
	VerificationThreshold int
}

// DefaultReasoningSandwichConfig returns the recommended reasoning sandwich config.
// Based on LangChain's harness engineering research showing xhigh-high-xhigh
// outperforms high-low-high by +3 points on Terminal-Bench 2.0.
//
// Planning: extra-high reasoning for deep task analysis (first 5 turns).
// Implementation: high reasoning for quality code generation (middle turns).
// Verification: extra-high reasoning for careful error analysis (when testing).
func DefaultReasoningSandwichConfig() ReasoningSandwichConfig {
	return ReasoningSandwichConfig{
		Planning:       ReasoningLevel{ThinkingBudget: 48000, ReasoningEffort: "high"},
		Implementation: ReasoningLevel{ThinkingBudget: 16000, ReasoningEffort: "high"},
		Verification:   ReasoningLevel{ThinkingBudget: 48000, ReasoningEffort: "high"},
		PlanningTurns:         5,
		VerificationThreshold: 0, // Use heuristic: detect verification commands
	}
}

// ReasoningSandwichMiddleware implements the "reasoning sandwich" pattern:
// high reasoning for planning → lower for implementation → high for verification.
//
// This is provider-agnostic: it sets both ThinkingBudget (Anthropic/Vertex)
// and ReasoningEffort (OpenAI) for each phase. Each provider uses whichever
// field applies to it, ignoring the other.
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

		// Only modify settings if reasoning is configured (either provider).
		if settings == nil || (settings.ThinkingBudget == nil && settings.ReasoningEffort == nil) {
			return next(ctx, messages, settings, params)
		}

		s := *settings
		var level ReasoningLevel
		var phase string

		switch {
		case currentTurn <= cfg.PlanningTurns:
			level = cfg.Planning
			phase = "planning"
		case isVerifying:
			level = cfg.Verification
			phase = "verification"
		default:
			level = cfg.Implementation
			phase = "implementation"
		}

		// Set provider-appropriate reasoning level.
		// Each provider ignores the field that doesn't apply to it.
		if s.ThinkingBudget != nil {
			s.ThinkingBudget = &level.ThinkingBudget
			// Ensure max_tokens > thinking budget (Anthropic requirement).
			if s.MaxTokens != nil && *s.MaxTokens <= level.ThinkingBudget {
				maxT := level.ThinkingBudget + 16000
				s.MaxTokens = &maxT
			}
		}
		if s.ReasoningEffort != nil {
			s.ReasoningEffort = &level.ReasoningEffort
		}

		fmt.Fprintf(os.Stderr, "[gollem] reasoning: turn %d phase=%s budget=%d effort=%s\n",
			currentTurn, phase, level.ThinkingBudget, level.ReasoningEffort)

		return next(ctx, messages, &s, params)
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
	warned25 := false
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
			warning = fmt.Sprintf("TIME CRITICAL: Only %s remaining (%.0f%% elapsed). "+
				"STOP all new work. Final actions only: "+
				"(1) ensure output files exist, (2) run final test, (3) rm build artifacts. "+
				"Do NOT start new approaches or fix more issues.", remaining.Round(time.Second), pct*100)
		case pct >= 0.75 && !warned75:
			warned75 = true
			warning = fmt.Sprintf("TIME WARNING: %s remaining (%.0f%% elapsed). "+
				"Start wrapping up. Focus on verifying your current approach works. "+
				"Run tests, fix only critical failures, clean up artifacts.", remaining.Round(time.Second), pct*100)
		case pct >= 0.50 && !warned50:
			warned50 = true
			warning = fmt.Sprintf("HALFWAY: %s remaining (%.0f%% elapsed). "+
				"If your current approach isn't working, switch strategies NOW. "+
				"If you haven't created output files yet, do that IMMEDIATELY.",
				remaining.Round(time.Second), pct*100)
		case pct >= 0.25 && !warned25:
			warned25 = true
			warning = fmt.Sprintf("QUARTER TIME: %s remaining (%.0f%% elapsed). "+
				"You should have output files created by now. If not, stop analyzing and start writing.",
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

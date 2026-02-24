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

// subagentReasoningConfig returns a reasoning sandwich config tuned for subagents.
// Subagents run shorter tasks (50 turns max) so they get fewer planning turns
// and slightly lower budgets to keep them fast. The verification phase still
// gets high reasoning since careful error analysis is critical.
func subagentReasoningConfig() ReasoningSandwichConfig {
	return ReasoningSandwichConfig{
		Planning:       ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		Implementation: ReasoningLevel{ThinkingBudget: 12000, ReasoningEffort: "medium"},
		Verification:   ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		PlanningTurns:         3,
		VerificationThreshold: 0, // Use heuristic
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
	verificationCooldown := 0 // turns remaining in verification mode after detection

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

		// Detect verification phase from recent messages every turn.
		// Unlike the old one-way latch (which stayed in verification forever
		// once triggered), this re-evaluates on each turn with a cooldown.
		// The 3-turn cooldown ensures high reasoning persists through the
		// critical test→analyze→fix cycle before dropping back to
		// implementation-level reasoning.
		if detectVerificationPhase(messages) {
			verificationCooldown = 3
		} else if verificationCooldown > 0 {
			verificationCooldown--
		}
		isVerifying := verificationCooldown > 0
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
				if tc, ok := part.(core.ToolCallPart); ok {
					if tc.ToolName == "bash" && isVerificationCommand(tc.ArgsJSON) {
						return true
					}
					if tc.ToolName == "execute_code" && isVerificationCode(tc.ArgsJSON) {
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
	warned95 := false

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
		case pct >= 0.95 && !warned95:
			warned95 = true
			warning = fmt.Sprintf("EMERGENCY: Only %s remaining (%.0f%% elapsed). "+
				"This is your LAST CHANCE. You will be killed soon. "+
				"DO ONLY: (1) if output files don't exist, write them NOW with whatever you have, "+
				"(2) remove only known intermediates: find . -name '__pycache__' -type d -exec rm -rf {} + 2>/dev/null; find . -name '*.pyc' -delete 2>/dev/null; rm -f *.o a.out 2>/dev/null "+
				"(3) STOP. Do nothing else. Any further exploration or debugging is wasted.",
				remaining.Round(time.Second), pct*100)
		case pct >= 0.90 && !warned90:
			warned90 = true
			warning = fmt.Sprintf("TIME CRITICAL: Only %s remaining (%.0f%% elapsed). "+
				"STOP all new work. Final actions only: "+
				"(1) ensure output files exist, (2) run final test, (3) remove only __pycache__/*.pyc/*.o intermediates (keep solution files). "+
				"Do NOT start new approaches or fix more issues.", remaining.Round(time.Second), pct*100)
		case pct >= 0.75 && !warned75:
			warned75 = true
			warning = fmt.Sprintf("TIME WARNING: %s remaining (%.0f%% elapsed). "+
				"Start wrapping up. Focus on verifying your current approach works. "+
				"Run tests, fix only critical failures, clean up artifacts.", remaining.Round(time.Second), pct*100)
		case pct >= 0.50 && !warned50:
			warned50 = true
			warning = fmt.Sprintf("HALFWAY CHECK: %s remaining (%.0f%% elapsed). "+
				"Do these NOW: "+
				"(1) Verify your output files exist — if not, create them IMMEDIATELY. "+
				"(2) Run the test suite and note which tests pass/fail. "+
				"(3) Read test FAILURE output carefully — it tells you EXACTLY what's wrong. "+
				"(4) If your approach has fundamental problems (< 30%% passing), switch to a simpler approach. "+
				"(5) Focus on fixing the HIGHEST-VALUE failures first.",
				remaining.Round(time.Second), pct*100)
		case pct >= 0.25 && !warned25:
			warned25 = true
			warning = fmt.Sprintf("QUARTER TIME: %s remaining (%.0f%% elapsed). "+
				"Checkpoint: (1) Output files should exist by now — if not, stop analyzing and start writing. "+
				"(2) You should have run tests at least once. "+
				"(3) If stuck on infrastructure (packages, compilation, networking), give it 2 more turns max then work around it.",
				remaining.Round(time.Second), pct*100)
		}

		if warning != "" {
			fmt.Fprintf(os.Stderr, "[gollem] time: %s\n", warning)
			// Inject the time warning into the last ModelRequest rather than
			// appending a new one. Messages always end with a ModelRequest
			// (initial request or tool results), so appending another would
			// create consecutive user-role messages that Anthropic rejects
			// with a 400 error.
			messages = injectUserPromptIntoLastRequest(messages, warning)
		}

		return next(ctx, messages, settings, params)
	}
}

// injectUserPromptIntoLastRequest adds a UserPromptPart to the last
// ModelRequest in the message list. This avoids creating a separate
// ModelRequest which would produce consecutive user-role messages.
// The returned slice is a shallow copy so the original is not mutated.
func injectUserPromptIntoLastRequest(messages []core.ModelMessage, content string) []core.ModelMessage {
	result := make([]core.ModelMessage, len(messages))
	copy(result, messages)
	for i := len(result) - 1; i >= 0; i-- {
		if req, ok := result[i].(core.ModelRequest); ok {
			newParts := make([]core.ModelRequestPart, len(req.Parts)+1)
			copy(newParts, req.Parts)
			newParts[len(req.Parts)] = core.UserPromptPart{Content: content}
			req.Parts = newParts
			result[i] = req
			return result
		}
	}
	// Fallback: no ModelRequest found, append a new one.
	return append(result, core.ModelRequest{
		Parts: []core.ModelRequestPart{core.UserPromptPart{Content: content}},
	})
}

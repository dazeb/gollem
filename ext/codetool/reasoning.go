package codetool

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// timeBudgetNow is overridden in tests for deterministic time-based behavior.
var timeBudgetNow = time.Now

// ReasoningLevel defines a provider-agnostic reasoning intensity.
// It maps to ThinkingBudget (Anthropic) and ReasoningEffort (OpenAI).
type ReasoningLevel struct {
	// ThinkingBudget is the token budget for Anthropic extended thinking.
	ThinkingBudget int
	// ReasoningEffort is the effort level for OpenAI models ("low", "medium", "high", "xhigh").
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
//
// Planning: high reasoning for deep task analysis (first 5 turns).
// Implementation: high reasoning for quality code generation (middle turns).
// Verification: high reasoning for careful error analysis (when testing).
func DefaultReasoningSandwichConfig() ReasoningSandwichConfig {
	return ReasoningSandwichConfig{
		// 32000 is the highest value that works across all providers:
		// Gemini caps at 32768, Anthropic supports much higher.
		Planning:              ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		Implementation:        ReasoningLevel{ThinkingBudget: 16000, ReasoningEffort: "high"},
		Verification:          ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		PlanningTurns:         5,
		VerificationThreshold: 0, // Use heuristic: detect verification commands
	}
}

// ReasoningSandwichConfigForMaxEffort returns a sandwich profile where planning
// and verification use maxEffort, while implementation uses one step lower.
// Examples:
// - maxEffort="xhigh" => planning/verification=xhigh, implementation=high
// - maxEffort="high"  => planning/verification=high, implementation=medium
func ReasoningSandwichConfigForMaxEffort(maxEffort string) ReasoningSandwichConfig {
	return withMaxReasoningEffort(DefaultReasoningSandwichConfig(), maxEffort)
}

// subagentReasoningConfig returns a reasoning sandwich config tuned for subagents.
// Subagents run shorter tasks (50 turns max) so they get fewer planning turns
// and slightly lower budgets to keep them fast. The verification phase still
// gets high reasoning since careful error analysis is critical.
func subagentReasoningConfig() ReasoningSandwichConfig {
	return ReasoningSandwichConfig{
		Planning:              ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		Implementation:        ReasoningLevel{ThinkingBudget: 12000, ReasoningEffort: "medium"},
		Verification:          ReasoningLevel{ThinkingBudget: 32000, ReasoningEffort: "high"},
		PlanningTurns:         3,
		VerificationThreshold: 0, // Use heuristic
	}
}

func subagentReasoningConfigForMaxEffort(maxEffort string) ReasoningSandwichConfig {
	return withMaxReasoningEffort(subagentReasoningConfig(), maxEffort)
}

func withMaxReasoningEffort(base ReasoningSandwichConfig, maxEffort string) ReasoningSandwichConfig {
	maxEffort = normalizeReasoningEffort(maxEffort)
	if maxEffort == "" {
		return base
	}
	base.Planning.ReasoningEffort = maxEffort
	base.Verification.ReasoningEffort = maxEffort
	base.Implementation.ReasoningEffort = lowerReasoningEffort(maxEffort)
	return base
}

func normalizeReasoningEffort(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	default:
		return ""
	}
}

func lowerReasoningEffort(level string) string {
	switch normalizeReasoningEffort(level) {
	case "xhigh":
		return "high"
	case "high":
		return "medium"
	case "medium":
		return "low"
	case "low":
		return "low"
	default:
		return "high"
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

type greedyPressureProfile struct {
	stage              string
	thinkingBudgetCap  int
	maxTokensCap       int
	maxReasoningEffort string
}

func greedyPressureForPct(pct float64) (greedyPressureProfile, bool) {
	switch {
	case pct >= 0.95:
		return greedyPressureProfile{
			stage:              "emergency",
			thinkingBudgetCap:  2000,
			maxTokensCap:       6000,
			maxReasoningEffort: "low",
		}, true
	case pct >= 0.90:
		return greedyPressureProfile{
			stage:              "critical",
			thinkingBudgetCap:  4000,
			maxTokensCap:       10000,
			maxReasoningEffort: "low",
		}, true
	case pct >= 0.75:
		return greedyPressureProfile{
			stage:              "hurry",
			thinkingBudgetCap:  8000,
			maxTokensCap:       16000,
			maxReasoningEffort: "medium",
		}, true
	case pct >= 0.50:
		return greedyPressureProfile{
			stage:              "conserve",
			thinkingBudgetCap:  16000,
			maxTokensCap:       28000,
			maxReasoningEffort: "high",
		}, true
	default:
		return greedyPressureProfile{}, false
	}
}

func effortRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "xhigh":
		return 4
	default:
		// Unknown levels are treated as highest so caps still apply.
		return 10
	}
}

func capReasoningEffort(ptr *string, capLevel string) bool {
	if ptr == nil || capLevel == "" {
		return false
	}
	current := strings.TrimSpace(*ptr)
	if current == "" {
		return false
	}
	if effortRank(current) <= effortRank(capLevel) {
		return false
	}
	*ptr = capLevel
	return true
}

func capPositiveInt(ptr *int, maxAllowed int) bool {
	if ptr == nil || maxAllowed <= 0 {
		return false
	}
	if *ptr <= 0 || *ptr <= maxAllowed {
		return false
	}
	*ptr = maxAllowed
	return true
}

func applyGreedyPressure(settings *core.ModelSettings, pct float64) (greedyPressureProfile, bool) {
	profile, ok := greedyPressureForPct(pct)
	if !ok || settings == nil {
		return profile, false
	}

	changed := false
	changed = capPositiveInt(settings.MaxTokens, profile.maxTokensCap) || changed
	changed = capPositiveInt(settings.ThinkingBudget, profile.thinkingBudgetCap) || changed
	changed = capReasoningEffort(settings.ReasoningEffort, profile.maxReasoningEffort) || changed

	// Keep Anthropic-compliant relation max_tokens > thinking_budget.
	if settings.MaxTokens != nil && settings.ThinkingBudget != nil && *settings.MaxTokens <= *settings.ThinkingBudget {
		mt := *settings.ThinkingBudget + 1024
		*settings.MaxTokens = mt
		changed = true
	}
	return profile, changed
}

func cloneModelSettingsForGreedyPressure(settings *core.ModelSettings) core.ModelSettings {
	if settings == nil {
		return core.ModelSettings{}
	}
	s := *settings
	if settings.MaxTokens != nil {
		v := *settings.MaxTokens
		s.MaxTokens = &v
	}
	if settings.ThinkingBudget != nil {
		v := *settings.ThinkingBudget
		s.ThinkingBudget = &v
	}
	if settings.ReasoningEffort != nil {
		v := *settings.ReasoningEffort
		s.ReasoningEffort = &v
	}
	return s
}

// TimeBudgetMiddleware injects time-remaining warnings into the conversation
// when the agent is approaching its timeout. This helps the agent prioritize
// completing its work rather than spending time on perfection.
func TimeBudgetMiddleware(timeout time.Duration) core.AgentMiddleware {
	return timeBudgetMiddleware(timeout, false)
}

// TimeBudgetMiddlewareNoGreedy injects time warnings but disables greedy
// reasoning/token caps as time elapses.
func TimeBudgetMiddlewareNoGreedy(timeout time.Duration) core.AgentMiddleware {
	return timeBudgetMiddleware(timeout, true)
}

func timeBudgetMiddleware(timeout time.Duration, disableGreedyPressure bool) core.AgentMiddleware {
	startTime := timeBudgetNow()
	warned25 := false
	warned50 := false
	warned75 := false
	warned90 := false
	warned95 := false
	lastGreedyStage := ""

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		elapsed := timeBudgetNow().Sub(startTime)
		if elapsed < 0 {
			elapsed = 0
		}
		if timeout <= 0 {
			return next(ctx, messages, settings, params)
		}
		pct := float64(elapsed) / float64(timeout)
		if pct < 0 {
			pct = 0
		}
		remaining := timeout - elapsed
		if remaining < 0 {
			remaining = 0
		}

		var warning string
		switch {
		case pct >= 0.95 && !warned95:
			warned95 = true
			warning = fmt.Sprintf("TIME EMERGENCY: %s left (%.0f%% elapsed).",
				remaining.Round(time.Second), pct*100)
		case pct >= 0.90 && !warned90:
			warned90 = true
			warning = fmt.Sprintf("TIME CRITICAL: %s left (%.0f%% elapsed).",
				remaining.Round(time.Second), pct*100)
		case pct >= 0.75 && !warned75:
			warned75 = true
			warning = fmt.Sprintf("TIME WARNING: %s left (%.0f%% elapsed).",
				remaining.Round(time.Second), pct*100)
		case pct >= 0.50 && !warned50:
			warned50 = true
			label := "HALFWAY"
			if pct >= 0.60 {
				label = "HALFWAY (late)"
			}
			warning = fmt.Sprintf("TIME %s: %s left (%.0f%% elapsed).",
				label, remaining.Round(time.Second), pct*100)
		case pct >= 0.25 && !warned25:
			warned25 = true
			label := "QUARTER"
			if pct >= 0.35 {
				label = "QUARTER (late)"
			}
			warning = fmt.Sprintf("TIME %s: %s left (%.0f%% elapsed).",
				label, remaining.Round(time.Second), pct*100)
		}

		if warning != "" {
			fmt.Fprintf(os.Stderr, "[gollem] time: %s\n", warning)
			content := warning
			if guidance := timeBudgetGuidance(pct); guidance != "" {
				content = warning + "\n" + guidance
			}
			// Inject the time warning into the last ModelRequest rather than
			// appending a new one. Messages always end with a ModelRequest
			// (initial request or tool results), so appending another would
			// create consecutive user-role messages that Anthropic rejects
			// with a 400 error.
			messages = injectUserPromptIntoLastRequest(messages, content)
		}

		adjustedSettings := settings
		if settings != nil && !disableGreedyPressure {
			s := cloneModelSettingsForGreedyPressure(settings)
			if profile, _ := applyGreedyPressure(&s, pct); profile.stage != "" {
				if profile.stage != lastGreedyStage {
					lastGreedyStage = profile.stage
					fmt.Fprintf(os.Stderr,
						"[gollem] time: greedy scaling -> %s (effort<=%s, thinking<=%d, max_tokens<=%d)\n",
						profile.stage, profile.maxReasoningEffort, profile.thinkingBudgetCap, profile.maxTokensCap)
				}
				adjustedSettings = &s
			}
		}

		return next(ctx, messages, adjustedSettings, params)
	}
}

func timeBudgetGuidance(pct float64) string {
	switch {
	case pct >= 0.90:
		return "Execution mode: no new exploration. Finalize the highest-confidence candidate, run the verifier/tests now, and submit the best passing output."
	case pct >= 0.75:
		return "Pivot now: stop speculative paths. Use a reference-first loop: produce required artifacts, run verifier/tests, patch exact failing deltas only."
	case pct >= 0.50:
		return "Half-time checkpoint: re-anchor to the latest task instruction and verifier criteria. Prove a minimal correct path before any optimization/refactor."
	case pct >= 0.25:
		return "Quarter checkpoint: create runnable outputs early and validate quickly. Prefer simple baseline implementations before complex strategies."
	default:
		return ""
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

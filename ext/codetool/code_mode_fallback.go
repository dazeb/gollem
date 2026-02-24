package codetool

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

const (
	defaultCodeModeFailureThreshold = 3
	defaultCodeModeCooldownTurns    = 2
	defaultCodeModeMaxRecentResults = 12
)

type codeModeFallbackConfig struct {
	failureThreshold int
	cooldownTurns    int
	maxRecentResults int
}

type executeCodeResult struct {
	messageIndex int
	failed       bool
}

// disableExecuteCodeOnImportFailuresPrepare temporarily removes execute_code
// after repeated sandbox capability failures in monty (missing modules,
// unsupported syntax/features). It re-enables execute_code after a short
// cooldown so the agent can try code mode again.
func disableExecuteCodeOnImportFailuresPrepare() core.AgentToolsPrepareFunc {
	cfg := loadCodeModeFallbackConfig()

	return func(_ context.Context, rc *core.RunContext, tools []core.ToolDefinition) []core.ToolDefinition {
		if rc == nil {
			return tools
		}

		disable, reason := shouldTemporarilyDisableExecuteCode(rc.Messages, cfg)
		if !disable {
			return tools
		}

		filtered := make([]core.ToolDefinition, 0, len(tools))
		removed := false
		for _, t := range tools {
			if t.Name == "execute_code" {
				removed = true
				continue
			}
			filtered = append(filtered, t)
		}
		if removed {
			fmt.Fprintf(os.Stderr, "[gollem] code-mode: temporarily disabling execute_code (%s)\n", reason)
			return filtered
		}
		return tools
	}
}

func loadCodeModeFallbackConfig() codeModeFallbackConfig {
	cfg := codeModeFallbackConfig{
		failureThreshold: defaultCodeModeFailureThreshold,
		cooldownTurns:    defaultCodeModeCooldownTurns,
		maxRecentResults: defaultCodeModeMaxRecentResults,
	}

	cfg.failureThreshold = envInt("GOLLEM_CODE_MODE_FAILURE_THRESHOLD", cfg.failureThreshold, 1)
	cfg.cooldownTurns = envInt("GOLLEM_CODE_MODE_COOLDOWN_TURNS", cfg.cooldownTurns, 1)
	cfg.maxRecentResults = envInt("GOLLEM_CODE_MODE_MAX_RECENT_RESULTS", cfg.maxRecentResults, cfg.failureThreshold)
	if cfg.maxRecentResults < cfg.failureThreshold {
		cfg.maxRecentResults = cfg.failureThreshold
	}
	return cfg
}

func envInt(name string, fallback, min int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	return v
}

func shouldTemporarilyDisableExecuteCode(messages []core.ModelMessage, cfg codeModeFallbackConfig) (bool, string) {
	results := collectRecentExecuteCodeResults(messages, cfg.maxRecentResults)
	if len(results) == 0 {
		return false, ""
	}

	streak := consecutiveFailureStreak(messages, results, cfg.cooldownTurns)
	if streak < cfg.failureThreshold {
		return false, ""
	}

	turnsSinceLastExecuteCode := countModelResponses(messages, results[0].messageIndex+1, len(messages))
	if turnsSinceLastExecuteCode >= cfg.cooldownTurns {
		return false, ""
	}

	remaining := cfg.cooldownTurns - turnsSinceLastExecuteCode
	reason := fmt.Sprintf("%d consecutive capability failures; cooldown %d turn(s) remaining", streak, remaining)
	return true, reason
}

func collectRecentExecuteCodeResults(messages []core.ModelMessage, limit int) []executeCodeResult {
	if limit <= 0 {
		return nil
	}

	results := make([]executeCodeResult, 0, limit)
	checked := 0
	for i := len(messages) - 1; i >= 0 && checked < limit; i-- {
		req, ok := messages[i].(core.ModelRequest)
		if !ok {
			continue
		}
		for j := len(req.Parts) - 1; j >= 0 && checked < limit; j-- {
			tr, ok := req.Parts[j].(core.ToolReturnPart)
			if !ok || tr.ToolName != "execute_code" {
				continue
			}
			checked++
			results = append(results, executeCodeResult{
				messageIndex: i,
				failed:       isCodeModeFailure(fmt.Sprint(tr.Content)),
			})
		}
	}

	return results
}

func consecutiveFailureStreak(messages []core.ModelMessage, results []executeCodeResult, streakResetTurns int) int {
	streak := 0
	for i, r := range results {
		if !r.failed {
			break
		}

		// If execute_code was not used for a while, treat the next use as a
		// fresh attempt and don't carry old failures into a new streak.
		if i > 0 {
			older := results[i].messageIndex
			newer := results[i-1].messageIndex
			if countModelResponses(messages, older+1, newer) >= streakResetTurns {
				break
			}
		}

		streak++
	}
	return streak
}

func countModelResponses(messages []core.ModelMessage, startInclusive, endExclusive int) int {
	if startInclusive < 0 {
		startInclusive = 0
	}
	if endExclusive > len(messages) {
		endExclusive = len(messages)
	}
	if startInclusive >= endExclusive {
		return 0
	}

	count := 0
	for i := startInclusive; i < endExclusive; i++ {
		if _, ok := messages[i].(core.ModelResponse); ok {
			count++
		}
	}
	return count
}

func isCodeModeFailure(content string) bool {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "modulenotfounderror") ||
		strings.Contains(lower, "importerror") ||
		strings.Contains(lower, "no module named") {
		return true
	}

	// Monty parser limitations (e.g., `with`, `del`, some syntax constructs).
	if strings.Contains(lower, "monty syntax parser") ||
		(strings.Contains(lower, "notimplementederror") && strings.Contains(lower, "monty")) {
		return true
	}

	// Monty sandbox may not expose expected builtins; repeated failures mean
	// execute_code is the wrong tool for this run.
	if strings.Contains(lower, "nameerror: name 'open' is not defined") {
		return true
	}

	return false
}

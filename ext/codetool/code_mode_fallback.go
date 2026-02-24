package codetool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// disableExecuteCodeOnImportFailuresPrepare removes execute_code from the
// available tools when recent execute_code results show repeated import
// failures in the monty sandbox. This forces a fast fallback to direct tools.
func disableExecuteCodeOnImportFailuresPrepare() core.AgentToolsPrepareFunc {
	return func(_ context.Context, rc *core.RunContext, tools []core.ToolDefinition) []core.ToolDefinition {
		if rc == nil || !hasRepeatedCodeImportFailures(rc.Messages) {
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
			fmt.Fprintf(os.Stderr, "[gollem] code-mode: disabling execute_code after repeated import failures\n")
			return filtered
		}
		return tools
	}
}

func hasRepeatedCodeImportFailures(messages []core.ModelMessage) bool {
	const maxRecentExecuteCodeResults = 6

	checked := 0
	failures := 0

	for i := len(messages) - 1; i >= 0 && checked < maxRecentExecuteCodeResults; i-- {
		req, ok := messages[i].(core.ModelRequest)
		if !ok {
			continue
		}
		for j := len(req.Parts) - 1; j >= 0 && checked < maxRecentExecuteCodeResults; j-- {
			tr, ok := req.Parts[j].(core.ToolReturnPart)
			if !ok || tr.ToolName != "execute_code" {
				continue
			}
			checked++
			if isCodeImportFailure(fmt.Sprint(tr.Content)) {
				failures++
			}
		}
	}

	// Two import failures in recent execute_code results usually means the
	// sandbox environment is unsuitable for this approach.
	return failures >= 2
}

func isCodeImportFailure(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "modulenotfounderror") ||
		strings.Contains(lower, "importerror") ||
		strings.Contains(lower, "no module named")
}

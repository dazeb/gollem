package codetool

import (
	"context"
	"strconv"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

func applyToolPolicy(policy string, tools []core.Tool) []core.Tool {
	switch normalizeToolPolicy(policy) {
	case "read-only":
		out := make([]core.Tool, len(tools))
		for i, tool := range tools {
			out[i] = tool
			if !readOnlyBlockedTool(tool.Definition.Name) {
				continue
			}
			name := tool.Definition.Name
			out[i].Handler = func(context.Context, *core.RunContext, string) (any, error) {
				return nil, core.NewModelRetryError(
					"tool " + strconv.Quote(name) + " is blocked by read-only tool policy; use read-only inspection tools instead",
				)
			}
		}
		return out
	default:
		return tools
	}
}

func toolPolicyPrepare(policy string) core.AgentToolsPrepareFunc {
	switch normalizeToolPolicy(policy) {
	case "":
		return nil
	case "read-only":
		return func(_ context.Context, _ *core.RunContext, tools []core.ToolDefinition) []core.ToolDefinition {
			filtered := make([]core.ToolDefinition, 0, len(tools))
			for _, tool := range tools {
				if readOnlyBlockedTool(tool.Name) {
					continue
				}
				filtered = append(filtered, tool)
			}
			return filtered
		}
	default:
		return nil
	}
}

func composeToolsPrepare(fns ...core.AgentToolsPrepareFunc) core.AgentToolsPrepareFunc {
	return func(ctx context.Context, rc *core.RunContext, tools []core.ToolDefinition) []core.ToolDefinition {
		for _, fn := range fns {
			if fn == nil {
				continue
			}
			tools = fn(ctx, rc, tools)
		}
		return tools
	}
}

func readOnlyBlockedTool(name string) bool {
	switch name {
	case "bash",
		"bash_status",
		"bash_kill",
		"execute_code",
		"write",
		"edit",
		"multi_edit",
		"lsp",
		"delegate",
		"spawn_teammate",
		"shutdown_teammate",
		"task_create",
		"task_fail_current",
		"send_teammate":
		return true
	default:
		return false
	}
}

func normalizeToolPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "default":
		return ""
	case "read-only", "readonly", "read_only", "no-mutation", "no_mutation":
		return "read-only"
	default:
		return strings.ToLower(strings.TrimSpace(policy))
	}
}

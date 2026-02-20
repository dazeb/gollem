package gollem

import (
	"context"
	"encoding/json"
	"fmt"
)

// agentToolParams is the input schema for an agent-as-tool.
type agentToolParams struct {
	Prompt string `json:"prompt" jsonschema:"description=The prompt to send to the inner agent"`
}

// AgentTool wraps an agent as a tool that can be called by another agent.
// The inner agent receives the tool call arguments as its prompt and returns
// its output as the tool result. Usage is aggregated to the outer agent.
func AgentTool[T any](name, description string, agent *Agent[T]) Tool {
	return FuncTool[agentToolParams](
		name,
		description,
		func(ctx context.Context, rc *RunContext, params agentToolParams) (any, error) {
			result, err := agent.Run(ctx, params.Prompt)
			if err != nil {
				return nil, fmt.Errorf("inner agent %q failed: %w", name, err)
			}

			// Aggregate inner agent usage to the outer agent.
			// We serialize the output to return as tool result.
			output, marshalErr := json.Marshal(result.Output)
			if marshalErr != nil {
				// If we can't serialize as JSON, return the raw output.
				return result.Output, nil //nolint:nilerr // graceful fallback
			}
			return json.RawMessage(output), nil
		},
	)
}

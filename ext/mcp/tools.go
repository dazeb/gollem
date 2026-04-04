package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fugue-labs/gollem/core"
)

// ToolCaller can execute MCP tools.
type ToolCaller interface {
	CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error)
}

type toolClient interface {
	ToolCaller
	ListTools(ctx context.Context) ([]Tool, error)
}

func toolsForSource(ctx context.Context, source toolClient) ([]core.Tool, error) {
	tools, err := source.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]core.Tool, 0, len(tools))
	for _, mt := range tools {
		result = append(result, convertTool(source, mt))
	}
	return result, nil
}

// Tools converts MCP tools into core.Tool instances that call back to the MCP server.
func (c *Client) Tools(ctx context.Context) ([]core.Tool, error) {
	return toolsForSource(ctx, c)
}

// convertTool converts a single MCP tool definition to a core.Tool.
func convertTool(client ToolCaller, mt Tool) core.Tool {
	return buildCoreTool(mt.Name, mt.Description, mt.InputSchema, client, mt.Name)
}

func buildCoreTool(name, description string, inputSchema json.RawMessage, caller ToolCaller, originalName string) core.Tool {
	schema := decodeToolSchema(inputSchema)

	def := core.ToolDefinition{
		Name:             name,
		Description:      description,
		ParametersSchema: schema,
		Kind:             core.ToolKindFunction,
	}

	handler := func(ctx context.Context, _ *core.RunContext, argsJSON string) (any, error) {
		if caller == nil {
			return nil, errors.New("mcp: no tool caller configured")
		}

		var args map[string]any
		if argsJSON != "" && argsJSON != "{}" {
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return nil, err
			}
		}
		if args == nil {
			args = make(map[string]any)
		}

		result, err := caller.CallTool(ctx, originalName, args)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("mcp: tool %q returned no result", originalName)
		}
		if result.IsError {
			return nil, &core.ModelRetryError{Message: result.TextContent()}
		}

		return result.TextContent(), nil
	}

	return core.Tool{
		Definition: def,
		Handler:    handler,
	}
}

func decodeToolSchema(raw json.RawMessage) core.Schema {
	var schema core.Schema
	if raw != nil {
		if err := json.Unmarshal(raw, &schema); err != nil {
			schema = nil
		}
	}
	if schema == nil {
		schema = core.Schema{"type": "object"}
	}
	return schema
}

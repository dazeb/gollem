package mcp

import (
	"context"
	"encoding/json"

	"github.com/trevorprater/gollem"
)

// MCPTool represents a tool definition from an MCP server.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPToolResult represents the result of an MCP tool call.
type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents a content block in an MCP response.
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextContent returns the concatenated text content from the result.
func (r *MCPToolResult) TextContent() string {
	var result string
	for _, c := range r.Content {
		if c.Type == "text" {
			if result != "" {
				result += "\n"
			}
			result += c.Text
		}
	}
	return result
}

// Tools converts MCP tools into gollem.Tool instances that call back to the MCP server.
func (c *Client) Tools(ctx context.Context) ([]gollem.Tool, error) {
	mcpTools, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	var tools []gollem.Tool
	for _, mt := range mcpTools {
		tool := convertTool(c, mt)
		tools = append(tools, tool)
	}
	return tools, nil
}

// convertTool converts a single MCP tool definition to a gollem.Tool.
func convertTool(client *Client, mt MCPTool) gollem.Tool {
	// Parse the input schema.
	var schema gollem.Schema
	if mt.InputSchema != nil {
		json.Unmarshal(mt.InputSchema, &schema)
	}
	if schema == nil {
		schema = gollem.Schema{"type": "object"}
	}

	def := gollem.ToolDefinition{
		Name:             mt.Name,
		Description:      mt.Description,
		ParametersSchema: schema,
		Kind:             gollem.ToolKindFunction,
	}

	handler := func(ctx context.Context, rc *gollem.RunContext, argsJSON string) (any, error) {
		var args map[string]any
		if argsJSON != "" && argsJSON != "{}" {
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return nil, err
			}
		}
		if args == nil {
			args = make(map[string]any)
		}

		result, err := client.CallTool(ctx, mt.Name, args)
		if err != nil {
			return nil, err
		}

		if result.IsError {
			return nil, &gollem.ModelRetryError{Message: result.TextContent()}
		}

		return result.TextContent(), nil
	}

	return gollem.Tool{
		Definition: def,
		Handler:    handler,
	}
}

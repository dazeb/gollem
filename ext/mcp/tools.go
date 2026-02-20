package mcp

import (
	"context"
	"encoding/json"

	"github.com/fugue-labs/gollem"
)

// Tool represents a tool definition from an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolResult represents the result of an MCP tool call.
type ToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents a content block in an MCP response.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextContent returns the concatenated text content from the result.
func (r *ToolResult) TextContent() string {
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
	tools, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	var result []gollem.Tool
	for _, mt := range tools {
		tool := convertTool(c, mt)
		result = append(result, tool)
	}
	return result, nil
}

// convertTool converts a single MCP tool definition to a gollem.Tool.
func convertTool(client *Client, mt Tool) gollem.Tool {
	// Parse the input schema.
	var schema gollem.Schema
	if mt.InputSchema != nil {
		if err := json.Unmarshal(mt.InputSchema, &schema); err != nil {
			schema = nil
		}
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

	handler := func(ctx context.Context, _ *gollem.RunContext, argsJSON string) (any, error) {
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

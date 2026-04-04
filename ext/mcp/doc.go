// Package mcp provides Model Context Protocol (MCP) clients, servers, model
// bridges, and a multi-server manager for discovering and invoking tools,
// prompts, and resources. It supports stdio, deprecated SSE, and streamable
// HTTP transports over JSON-RPC 2.0, including server-initiated client
// requests such as roots/list, sampling/createMessage, and elicitation/create.
//
// # Usage
//
//	client, err := mcp.NewStdioClient(ctx, "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
//	defer client.Close()
//
//	tools, err := client.Tools(ctx)
//	agent := core.NewAgent[string](model, core.WithTools[string](tools...))
//
//	server := mcp.NewServer()
//	server.AddTool(mcp.Tool{
//		Name:        "ask_client",
//		Description: "Sample via the connected MCP client",
//		InputSchema: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}}}`),
//	}, func(ctx context.Context, rc *mcp.RequestContext, args map[string]any) (*mcp.ToolResult, error) {
//		model := mcp.NewMCPModel(rc)
//		resp, err := model.Request(ctx, []core.ModelMessage{
//			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: args["prompt"].(string)}}},
//		}, nil, nil)
//		if err != nil {
//			return nil, err
//		}
//		return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: resp.TextContent()}}}, nil
//	})
//
// ModelSamplingHandler automatically advertises sampling.tools because the
// bridge can translate MCP tool-bearing sampling requests into gollem tool
// definitions. Raw custom SamplingHandler functions only advertise base
// sampling support unless ClientConfig.Capabilities opts into more.
package mcp

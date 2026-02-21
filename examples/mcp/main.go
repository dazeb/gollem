// Example mcp demonstrates MCP tool discovery and tool execution through a
// gollem agent. It runs offline by default using an in-process mock MCP
// server and TestModel, and can use live Anthropic with GOLLEM_USE_LIVE_MODELS=1.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/fugue-labs/gollem/core"
	mcpclient "github.com/fugue-labs/gollem/ext/mcp"
	"github.com/fugue-labs/gollem/provider/anthropic"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--mock-mcp-server" {
		runMockMCPServer()
		return
	}

	ctx := context.Background()

	command, args, note := resolveServerCommand()
	client, err := mcpclient.NewStdioClient(ctx, command, args...)
	if err != nil {
		log.Fatalf("Failed to connect to MCP server: %v", err)
	}
	defer client.Close()

	tools, err := client.Tools(ctx)
	if err != nil {
		log.Fatalf("Failed to discover tools: %v", err)
	}

	fmt.Printf("Connected to MCP server command: %s %v\n", command, args)
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Definition.Name, t.Definition.Description)
	}
	if note != "" {
		fmt.Printf("Note: %s\n", note)
	}
	fmt.Println()

	model, live, modelNote := selectModel(tools)
	agent := core.NewAgent[string](model,
		core.WithSystemPrompt[string]("You are a practical assistant. Use tools when helpful, then summarize findings clearly."),
		core.WithTools[string](tools...),
	)

	result, err := agent.Run(ctx, "List the available files and read one interesting file.")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Agent Response ===")
	fmt.Println(result.Output)
	fmt.Printf("\nRun stats: %d requests, %d tool calls\n", result.Usage.Requests, result.Usage.ToolCalls)
	if modelNote != "" {
		fmt.Printf("Note: %s\n", modelNote)
	}
	if !live {
		fmt.Println("(offline demo mode: set GOLLEM_USE_LIVE_MODELS=1 and ANTHROPIC_API_KEY to use a live model)")
	}
}

func resolveServerCommand() (command string, args []string, note string) {
	if len(os.Args) >= 2 {
		return os.Args[1], os.Args[2:], ""
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("failed to resolve executable path: %v", err)
	}
	return exe, []string{"--mock-mcp-server"}, "no MCP command provided, using built-in mock server"
}

func selectModel(tools []core.Tool) (core.Model, bool, string) {
	if os.Getenv("GOLLEM_USE_LIVE_MODELS") == "1" && os.Getenv("ANTHROPIC_API_KEY") != "" {
		return anthropic.New(), true, ""
	}

	hasListFiles := false
	hasReadFile := false
	for _, t := range tools {
		switch t.Definition.Name {
		case "list_files":
			hasListFiles = true
		case "read_file":
			hasReadFile = true
		}
	}

	if hasListFiles && hasReadFile {
		return core.NewTestModel(
			core.ToolCallResponse("list_files", `{"path":"/workspace"}`),
			core.ToolCallResponse("read_file", `{"path":"README.md"}`),
			core.TextResponse("I found README.md, main.go, and docs/guide.md. README.md outlines setup and example usage."),
		), false, ""
	}

	return core.NewTestModel(core.TextResponse("Tools were discovered successfully, but this demo model is not configured for those tool names.")),
		false,
		"provide a live model to let the agent choose arbitrary MCP tools"
}

func runMockMCPServer() {
	type request struct {
		ID     any            `json:"id,omitempty"`
		Method string         `json:"method"`
		Params map[string]any `json:"params,omitempty"`
	}

	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	writeResponse := func(id any, result any, rpcErr map[string]any) {
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
		}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		_, _ = writer.Write(append(data, '\n'))
		_ = writer.Flush()
	}

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		// Notifications do not include an ID and do not require a response.
		if req.ID == nil {
			continue
		}

		switch req.Method {
		case "initialize":
			writeResponse(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "mock-filesystem", "version": "1.0.0"},
			}, nil)
		case "tools/list":
			writeResponse(req.ID, map[string]any{
				"tools": []map[string]any{
					{
						"name":        "list_files",
						"description": "List files in a path",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path": map[string]any{"type": "string"},
							},
							"required": []string{"path"},
						},
					},
					{
						"name":        "read_file",
						"description": "Read a file by path",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path": map[string]any{"type": "string"},
							},
							"required": []string{"path"},
						},
					},
				},
			}, nil)
		case "tools/call":
			name := ""
			if req.Params != nil {
				if v, ok := req.Params["name"].(string); ok {
					name = v
				}
			}

			switch name {
			case "list_files":
				writeResponse(req.ID, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "README.md\nmain.go\ndocs/guide.md"},
					},
				}, nil)
			case "read_file":
				writeResponse(req.ID, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "README.md: Gollem quickstart and feature overview."},
					},
				}, nil)
			default:
				writeResponse(req.ID, nil, map[string]any{
					"code":    -32601,
					"message": "unknown tool",
				})
			}
		default:
			writeResponse(req.ID, nil, map[string]any{
				"code":    -32601,
				"message": "method not found",
			})
		}
	}
}

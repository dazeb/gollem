// Command gollem-mcp-server exposes gollem agent capabilities as MCP tools
// over stdio using JSON-RPC 2.0. It implements the Model Context Protocol
// (protocol version 2024-11-05) so that MCP clients such as Claude Desktop
// or Claude Code can invoke gollem agents and execute Python code via WASM.
//
// Usage:
//
//	gollem-mcp-server            # start MCP server on stdin/stdout
//	gollem-mcp-server --help     # show usage
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	montygo "github.com/fugue-labs/monty-go"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
)

// Protocol version we advertise during the MCP handshake.
const protocolVersion = "2024-11-05"

// JSON-RPC 2.0 types ---------------------------------------------------

type jsonRPCRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
)

// MCP types -------------------------------------------------------------

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type capabilities struct {
	Tools *toolsCap `json:"tools,omitempty"`
}

type toolsCap struct {
	ListChanged bool `json:"listChanged"`
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type mcpTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type toolCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// Server ----------------------------------------------------------------

// Server implements an MCP server over stdio.
type Server struct {
	reader *bufio.Reader
	writer io.Writer
	logger *log.Logger

	// Lazily initialized monty-go runner.
	runner *montygo.Runner
}

// NewServer creates a new MCP server reading from r and writing to w.
// Logs are written to logW (typically os.Stderr).
func NewServer(r io.Reader, w io.Writer, logW io.Writer) *Server {
	return &Server{
		reader: bufio.NewReader(r),
		writer: w,
		logger: log.New(logW, "gollem-mcp: ", log.LstdFlags),
	}
}

// Run reads JSON-RPC messages from the reader and dispatches them until
// the reader returns EOF or the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, codeParseError, "parse error: "+err.Error())
			continue
		}

		s.dispatch(ctx, &req)
	}
}

func (s *Server) dispatch(ctx context.Context, req *jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// Notification; no response needed.
		s.logger.Println("client initialized")
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	default:
		s.sendError(req.ID, codeMethodNotFound, "method not found: "+req.Method)
	}
}

// handleInitialize responds to the MCP initialize handshake.
func (s *Server) handleInitialize(req *jsonRPCRequest) {
	result := initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities: capabilities{
			Tools: &toolsCap{ListChanged: false},
		},
		ServerInfo: serverInfo{
			Name:    "gollem-mcp-server",
			Version: "1.0.0",
		},
	}
	s.sendResult(req.ID, result)
}

// handleToolsList returns the list of available MCP tools.
func (s *Server) handleToolsList(req *jsonRPCRequest) {
	tools := []mcpTool{
		{
			Name:        "run_agent",
			Description: "Run a gollem agent with a given prompt. Returns the agent's text response.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "The prompt to send to the agent",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "Model name to use (e.g. 'gpt-4o', 'claude-sonnet-4-5-20250929'). Uses provider default if not set.",
					},
					"provider": map[string]any{
						"type":        "string",
						"description": "LLM provider to use",
						"enum":        []string{"openai", "anthropic", "ollama"},
					},
				},
				"required": []string{"prompt"},
			},
		},
		{
			Name:        "execute_python",
			Description: "Execute Python code in a WASM sandbox using monty-go. The last expression is the return value.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{
						"type":        "string",
						"description": "Python code to execute",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Maximum execution time in seconds (default: 30)",
					},
				},
				"required": []string{"code"},
			},
		},
		{
			Name:        "list_providers",
			Description: "List available LLM providers and their default models.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}

	s.sendResult(req.ID, toolsListResult{Tools: tools})
}

// handleToolsCall dispatches a tool call to the appropriate handler.
func (s *Server) handleToolsCall(ctx context.Context, req *jsonRPCRequest) {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, codeInvalidRequest, "invalid params: "+err.Error())
		return
	}

	switch params.Name {
	case "run_agent":
		s.callRunAgent(ctx, req, params.Arguments)
	case "execute_python":
		s.callExecutePython(ctx, req, params.Arguments)
	case "list_providers":
		s.callListProviders(req)
	default:
		s.sendError(req.ID, codeMethodNotFound, "unknown tool: "+params.Name)
	}
}

// Tool implementations --------------------------------------------------

func (s *Server) callRunAgent(ctx context.Context, req *jsonRPCRequest, args map[string]any) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		s.sendToolError(req.ID, "prompt is required")
		return
	}

	providerName, _ := args["provider"].(string)
	if providerName == "" {
		providerName = "openai"
	}

	modelName, _ := args["model"].(string)

	model, err := createModel(providerName, modelName)
	if err != nil {
		s.sendToolError(req.ID, err.Error())
		return
	}

	agent := core.NewAgent(model,
		core.WithSystemPrompt[string]("You are a helpful assistant."),
		core.WithMaxTokens[string](4096),
	)

	result, err := agent.Run(ctx, prompt)
	if err != nil {
		s.sendToolError(req.ID, fmt.Sprintf("agent run failed: %v", err))
		return
	}

	s.sendToolResult(req.ID, result.Output)
}

func (s *Server) callExecutePython(ctx context.Context, req *jsonRPCRequest, args map[string]any) {
	code, _ := args["code"].(string)
	if code == "" {
		s.sendToolError(req.ID, "code is required")
		return
	}

	timeoutSec := 30
	if ts, ok := args["timeout_seconds"].(float64); ok && ts > 0 {
		timeoutSec = int(ts)
	}

	// Lazily initialize monty-go runner.
	if s.runner == nil {
		r, err := montygo.New()
		if err != nil {
			s.sendToolError(req.ID, fmt.Sprintf("failed to initialize Python runtime: %v", err))
			return
		}
		s.runner = r
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var prints strings.Builder
	opts := []montygo.ExecuteOption{
		montygo.WithLimits(montygo.Limits{
			MaxDuration: time.Duration(timeoutSec) * time.Second,
		}),
		montygo.WithPrintFunc(func(s string) {
			prints.WriteString(s)
		}),
	}

	result, err := s.runner.Execute(execCtx, code, nil, opts...)
	if err != nil {
		s.sendToolError(req.ID, fmt.Sprintf("execution failed: %v", err))
		return
	}

	// Build response combining result and any printed output.
	var output string
	if prints.Len() > 0 {
		resultJSON, _ := json.Marshal(result)
		output = fmt.Sprintf("Result: %s\nOutput:\n%s", string(resultJSON), prints.String())
	} else {
		resultJSON, _ := json.Marshal(result)
		output = string(resultJSON)
	}

	s.sendToolResult(req.ID, output)
}

func (s *Server) callListProviders(req *jsonRPCRequest) {
	providers := []map[string]string{
		{
			"name":          "openai",
			"default_model": "gpt-4o",
			"description":   "OpenAI Chat Completions API (GPT-4o, O3, etc.)",
		},
		{
			"name":          "anthropic",
			"default_model": "claude-sonnet-4-5-20250929",
			"description":   "Anthropic Messages API (Claude Sonnet, Opus, Haiku)",
		},
		{
			"name":          "ollama",
			"default_model": "llama3",
			"description":   "Local Ollama instance (OpenAI-compatible, no API key needed)",
		},
	}

	data, _ := json.MarshalIndent(providers, "", "  ")
	s.sendToolResult(req.ID, string(data))
}

// Response helpers ------------------------------------------------------

func (s *Server) sendResult(id *json.RawMessage, result any) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      normalizeID(id),
		Result:  result,
	}
	s.writeResponse(resp)
}

func (s *Server) sendError(id *json.RawMessage, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      normalizeID(id),
		Error:   &rpcError{Code: code, Message: message},
	}
	s.writeResponse(resp)
}

func (s *Server) sendToolResult(id *json.RawMessage, text string) {
	result := toolCallResult{
		Content: []contentBlock{{Type: "text", Text: text}},
	}
	s.sendResult(id, result)
}

func (s *Server) sendToolError(id *json.RawMessage, text string) {
	result := toolCallResult{
		Content: []contentBlock{{Type: "text", Text: text}},
		IsError: true,
	}
	s.sendResult(id, result)
}

func (s *Server) writeResponse(resp jsonRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Printf("failed to marshal response: %v", err)
		return
	}
	fmt.Fprintf(s.writer, "%s\n", data)
}

// normalizeID converts the raw JSON id to a concrete type for serialization.
func normalizeID(raw *json.RawMessage) any {
	if raw == nil {
		return nil
	}
	// Try integer first, then string, then fall back to raw.
	var intID int64
	if err := json.Unmarshal(*raw, &intID); err == nil {
		return intID
	}
	var strID string
	if err := json.Unmarshal(*raw, &strID); err == nil {
		return strID
	}
	return raw
}

// Provider creation -----------------------------------------------------

func createModel(provider, modelName string) (core.Model, error) {
	switch provider {
	case "openai":
		var opts []openai.Option
		if modelName != "" {
			opts = append(opts, openai.WithModel(modelName))
		}
		return openai.New(opts...), nil
	case "anthropic":
		var opts []anthropic.Option
		if modelName != "" {
			opts = append(opts, anthropic.WithModel(modelName))
		}
		return anthropic.New(opts...), nil
	case "ollama":
		var opts []openai.Option
		if modelName != "" {
			opts = append(opts, openai.WithModel(modelName))
		}
		return openai.NewOllama(opts...), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (available: openai, anthropic, ollama)", provider)
	}
}

// main ------------------------------------------------------------------

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Fprintf(os.Stderr, `gollem-mcp-server - MCP server for gollem agent capabilities

Reads JSON-RPC 2.0 messages from stdin and writes responses to stdout.
Logs are written to stderr.

Tools:
  run_agent         Run a gollem agent with a given prompt
  execute_python    Execute Python code in a WASM sandbox
  list_providers    List available LLM providers

Usage:
  gollem-mcp-server              Start the MCP server
  gollem-mcp-server --help       Show this help
`)
		os.Exit(0)
	}

	server := NewServer(os.Stdin, os.Stdout, os.Stderr)
	server.logger.Println("starting MCP server")

	if err := server.Run(context.Background()); err != nil {
		server.logger.Fatalf("server error: %v", err)
	}
}

// Command gollem-mcp-server exposes gollem agent capabilities as MCP tools
// over stdio using JSON-RPC 2.0. It implements the Model Context Protocol
// (protocol version 2025-11-25) so MCP clients such as Claude Code can invoke
// gollem agents, borrow the connected client's model through MCP sampling,
// and execute Python code via WASM.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	montygo "github.com/fugue-labs/monty-go"

	"github.com/fugue-labs/gollem/core"
	mcp "github.com/fugue-labs/gollem/ext/mcp"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
)

const protocolVersion = mcp.ProtocolVersion

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

type initializeResult = mcp.InitializeResult
type toolsListResult struct {
	Tools []mcp.Tool `json:"tools"`
}
type toolCallResult = mcp.ToolResult

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
)

// Server wraps the reusable MCP server with command-specific tool handlers.
type Server struct {
	reader io.Reader
	writer io.Writer
	logger *log.Logger

	inner  *mcp.Server
	runner *montygo.Runner

	runnerMu sync.Mutex
}

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error { return nil }

// NewServer creates a new MCP server reading from r and writing to w.
func NewServer(r io.Reader, w io.Writer, logW io.Writer) *Server {
	s := &Server{
		reader: r,
		writer: w,
		logger: log.New(logW, "gollem-mcp: ", log.LstdFlags),
		inner: mcp.NewServer(
			mcp.WithServerInfo(mcp.ServerInfo{
				Name:    "gollem-mcp-server",
				Version: "1.0.0",
			}),
		),
	}
	s.registerTools()
	return s
}

// Run serves stdio JSON-RPC until EOF or context cancellation.
func (s *Server) Run(ctx context.Context) error {
	transport := mcp.NewStdioServerTransport(s.inner, s.reader, nopWriteCloser{Writer: s.writer})
	err := transport.Run(ctx)
	s.inner.WaitIdle()
	return err
}

func (s *Server) registerTools() {
	s.inner.AddTool(mcp.Tool{
		Name:        "run_agent",
		Description: "Run a gollem agent with a given prompt. Returns the agent's text response.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The prompt to send to the agent",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model name to use. With provider=mcp this becomes a model preference hint for the connected client.",
				},
				"provider": map[string]any{
					"type":        "string",
					"description": "LLM provider to use (default: openai). Use mcp to borrow the connected client's model.",
					"enum":        []string{"mcp", "openai", "anthropic", "ollama"},
				},
			},
			"required": []string{"prompt"},
		}),
	}, s.handleRunAgent)

	s.inner.AddTool(mcp.Tool{
		Name:        "execute_python",
		Description: "Execute Python code in a WASM sandbox using monty-go. The last expression is the return value.",
		InputSchema: mustJSON(map[string]any{
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
		}),
	}, s.handleExecutePython)

	s.inner.AddTool(mcp.Tool{
		Name:        "list_providers",
		Description: "List available LLM providers and their default models.",
		InputSchema: mustJSON(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	}, s.handleListProviders)
}

func (s *Server) handleRunAgent(ctx context.Context, rc *mcp.RequestContext, args map[string]any) (*mcp.ToolResult, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return toolError("prompt is required"), nil
	}

	providerName := resolveProviderName(args)
	modelName, _ := args["model"].(string)

	model, err := createModel(providerName, modelName, rc)
	if err != nil {
		return toolError(err.Error()), nil
	}

	agent := core.NewAgent(model,
		core.WithSystemPrompt[string]("You are a helpful assistant."),
		core.WithMaxTokens[string](4096),
	)

	result, err := agent.Run(ctx, prompt)
	if err != nil {
		return toolError(fmt.Sprintf("agent run failed: %v", err)), nil
	}

	return textResult(result.Output), nil
}

func (s *Server) handleExecutePython(ctx context.Context, _ *mcp.RequestContext, args map[string]any) (*mcp.ToolResult, error) {
	code, _ := args["code"].(string)
	if code == "" {
		return toolError("code is required"), nil
	}

	timeoutSec := 30
	if ts, ok := args["timeout_seconds"].(float64); ok && ts > 0 {
		timeoutSec = int(ts)
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

	result, initErr, execErr := s.executePython(execCtx, code, opts...)
	if initErr != nil {
		return toolError(fmt.Sprintf("failed to initialize Python runtime: %v", initErr)), nil
	}
	if execErr != nil {
		return toolError(fmt.Sprintf("execution failed: %v", execErr)), nil
	}

	resultJSON, _ := json.Marshal(result)
	output := string(resultJSON)
	if prints.Len() > 0 {
		output = fmt.Sprintf("Result: %s\nOutput:\n%s", string(resultJSON), prints.String())
	}
	return textResult(output), nil
}

func (s *Server) executePython(ctx context.Context, code string, opts ...montygo.ExecuteOption) (result any, initErr error, execErr error) {
	s.runnerMu.Lock()
	defer s.runnerMu.Unlock()

	if s.runner == nil {
		r, err := montygo.New()
		if err != nil {
			return nil, err, nil
		}
		s.runner = r
	}

	result, err := s.runner.Execute(ctx, code, nil, opts...)
	if err != nil {
		return nil, nil, err
	}
	return result, nil, nil
}

func (s *Server) handleListProviders(context.Context, *mcp.RequestContext, map[string]any) (*mcp.ToolResult, error) {
	providers := []map[string]string{
		{
			"name":          "mcp",
			"default_model": "client-default",
			"description":   "Borrow the connected MCP client's configured model via sampling/createMessage",
		},
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
	return textResult(string(data)), nil
}

func resolveProviderName(args map[string]any) string {
	providerName, _ := args["provider"].(string)
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return "openai"
	}
	return providerName
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func textResult(text string) *mcp.ToolResult {
	return &mcp.ToolResult{
		Content: []mcp.Content{{Type: "text", Text: text}},
	}
}

func toolError(text string) *mcp.ToolResult {
	return &mcp.ToolResult{
		Content: []mcp.Content{{Type: "text", Text: text}},
		IsError: true,
	}
}

// normalizeID converts the raw JSON id to a concrete type for test serialization.
func normalizeID(raw *json.RawMessage) any {
	if raw == nil {
		return nil
	}
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

func createModel(provider, modelName string, rc *mcp.RequestContext) (core.Model, error) {
	switch provider {
	case "mcp":
		if rc == nil || rc.ClientCapabilities().Sampling == nil {
			return nil, fmt.Errorf("provider %q requires an MCP client with sampling support", provider)
		}
		var opts []mcp.MCPModelOption
		if modelName != "" {
			opts = append(opts,
				mcp.WithMCPModelName(modelName),
				mcp.WithMCPModelPreferences(mcp.ModelPreferences{
					Hints: []mcp.ModelHint{{Name: modelName}},
				}),
			)
		}
		return mcp.NewMCPModel(rc, opts...), nil
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
		return nil, fmt.Errorf("unknown provider %q (available: mcp, openai, anthropic, ollama)", provider)
	}
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Fprintf(os.Stderr, `gollem-mcp-server - MCP server for gollem agent capabilities

Reads JSON-RPC 2.0 messages from stdin and writes responses to stdout.
Logs are written to stderr.

Tools:
  run_agent         Run a gollem agent with a given prompt
  execute_python    Execute Python code in a WASM sandbox
  list_providers    List available LLM providers

Providers:
  mcp              Borrow the connected client's model via MCP sampling
  openai           Use OpenAI directly
  anthropic        Use Anthropic directly
  ollama           Use a local Ollama instance

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

// Command gollem provides a CLI for running coding agents and debugging via TUI.
//
// Usage:
//
//	gollem run --provider anthropic --model claude-opus-4-6 "Fix the failing tests"
//	gollem debug --provider openai --model gpt-4o "prompt"
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	lf "github.com/fugue-labs/langfuse-go"
	montygo "github.com/fugue-labs/monty-go"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/middleware"
	"github.com/fugue-labs/gollem/ext/tui"
	"github.com/fugue-labs/gollem/modelutil"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
	"github.com/fugue-labs/gollem/provider/vertexai"
	"github.com/fugue-labs/gollem/provider/vertexai_anthropic"
)

// Default thinking budget for Anthropic models with extended thinking.
// Balanced for quality vs speed: allows deep reasoning without burning
// too many seconds per turn on time-limited benchmark tasks.
const defaultThinkingBudget = 16000

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "run":
		runAgent()
	case "debug":
		runDebug()
	case "--help", "-h", "help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

type flags struct {
	provider        string
	modelName       string
	location        string // GCP region for vertexai providers
	project         string // GCP project ID for vertexai providers
	workDir         string
	prompt          string
	timeout         time.Duration
	thinkingBudget  int
	reasoningEffort string // OpenAI: "low", "medium", "high"
	noCodeMode      bool
	noReasoning     bool // Disable all reasoning (thinking + effort)
}

func parseFlags(args []string) flags {
	f := flags{
		timeout:        30 * time.Minute,
		thinkingBudget: -1, // -1 means "use default"
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 < len(args) {
				f.provider = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				f.modelName = args[i+1]
				i++
			}
		case "--location":
			if i+1 < len(args) {
				f.location = args[i+1]
				i++
			}
		case "--project":
			if i+1 < len(args) {
				f.project = args[i+1]
				i++
			}
		case "--workdir":
			if i+1 < len(args) {
				f.workDir = args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					f.timeout = d
				}
				i++
			}
		case "--thinking-budget":
			if i+1 < len(args) {
				if _, err := fmt.Sscanf(args[i+1], "%d", &f.thinkingBudget); err != nil {
					f.thinkingBudget = -1
				}
				i++
			}
		case "--reasoning-effort":
			if i+1 < len(args) {
				f.reasoningEffort = args[i+1]
				i++
			}
		case "--no-thinking", "--no-reasoning":
			f.noReasoning = true
		case "--no-code-mode":
			f.noCodeMode = true
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		default:
			if !strings.HasPrefix(args[i], "-") && f.prompt == "" {
				f.prompt = args[i]
			}
		}
	}

	if f.workDir == "" {
		f.workDir, _ = os.Getwd()
	}

	return f
}

func runAgent() {
	f := parseFlags(os.Args[2:])

	if f.prompt == "" {
		fmt.Fprintln(os.Stderr, "error: prompt is required")
		printRunUsage()
		os.Exit(1)
	}

	// Detect the real task deadline for accurate time budget warnings.
	// GOLLEM_TIMEOUT_SEC (from Harbor) or task.toml represents the outer
	// deadline. The --timeout flag (f.timeout) may be shorter to leave a
	// cleanup buffer. We track both:
	//   - f.timeout:    execution timeout for context.WithTimeout (shorter)
	//   - budgetTimeout: real deadline for TimeBudgetMiddleware warnings
	budgetTimeout := f.timeout
	if taskTimeout := detectTaskTimeout(f.workDir); taskTimeout > 0 {
		budgetTimeout = taskTimeout
		// Also shorten exec timeout if task.toml is shorter than --timeout.
		if taskTimeout < f.timeout {
			fmt.Fprintf(os.Stderr, "gollem: detected task timeout: %v (overriding %v)\n", taskTimeout, f.timeout)
			f.timeout = taskTimeout
		}
	}
	if envTimeout := os.Getenv("GOLLEM_TIMEOUT_SEC"); envTimeout != "" {
		var secs float64
		if _, err := fmt.Sscanf(envTimeout, "%f", &secs); err == nil && secs > 0 {
			envDuration := time.Duration(secs) * time.Second
			// Use env timeout for budget tracking (accurate deadline).
			budgetTimeout = envDuration
			fmt.Fprintf(os.Stderr, "gollem: budget timeout: %v, exec timeout: %v\n", envDuration, f.timeout)
		}
	}

	if f.provider == "" {
		f.provider = detectProvider()
		if f.provider == "" {
			fmt.Fprintln(os.Stderr, "error: --provider is required (or set ANTHROPIC_API_KEY / OPENAI_API_KEY)")
			os.Exit(1)
		}
	}

	baseModel, err := createModel(f.provider, f.modelName, f.location, f.project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating model: %v\n", err)
		os.Exit(1)
	}

	// Apply middleware layers to the model.
	var mws []middleware.Middleware

	// Langfuse observability (if configured via environment).
	var langfuseProcessor *lf.BatchProcessor
	if os.Getenv("LANGFUSE_SECRET_KEY") != "" {
		lfOpts := []lf.Option{
			lf.WithKeys(os.Getenv("LANGFUSE_PUBLIC_KEY"), os.Getenv("LANGFUSE_SECRET_KEY")),
		}
		if baseURL := os.Getenv("LANGFUSE_BASE_URL"); baseURL != "" {
			lfOpts = append(lfOpts, lf.WithBaseURL(baseURL))
		}
		client := lf.New(lfOpts...)
		langfuseProcessor = lf.NewBatchProcessor(client)
		mws = append(mws, newLangfuseMiddleware(langfuseProcessor))
		fmt.Fprintf(os.Stderr, "gollem: langfuse tracing enabled\n")
	}

	var model = baseModel
	if len(mws) > 0 {
		model = middleware.Wrap(model, mws...)
	}

	// Wrap with retry for API resilience (exponential backoff on 429/5xx).
	model = modelutil.NewRetryModel(model, modelutil.DefaultRetryConfig())

	// Build tool options, including code mode if enabled.
	var toolOpts []codetool.Option
	var runner *montygo.Runner
	if !f.noCodeMode {
		runner, err = montygo.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: code mode unavailable: %v\n", err)
		} else {
			toolOpts = append(toolOpts, codetool.WithCodeMode(runner))
			fmt.Fprintf(os.Stderr, "gollem: code mode enabled (monty-go WASM Python)\n")
		}
	}
	if runner != nil {
		defer runner.Close()
	}

	// Pass the model so the coding agent can spawn subagents for delegation.
	toolOpts = append(toolOpts, codetool.WithModel(model))
	// Pass the budget timeout (real deadline) for time budget warnings.
	// This is separate from f.timeout (exec timeout) which may be shorter.
	toolOpts = append(toolOpts, codetool.WithTimeout(budgetTimeout))

	// Provider-aware auto-context limits. This flows through to BOTH the main
	// agent AND subagents via WithAutoContextConfig. Previously, only the main
	// agent got the override and subagents were stuck at 80K — wasting 70K of
	// Claude's context window on every subagent invocation.
	switch f.provider {
	case "anthropic", "vertexai-anthropic":
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 150000,
			KeepLastN: 12,
		}))
		fmt.Fprintf(os.Stderr, "gollem: auto-context limit: 150K tokens (Claude) — main + subagents\n")
	case "vertexai":
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 900000,
			KeepLastN: 20,
		}))
		fmt.Fprintf(os.Stderr, "gollem: auto-context limit: 900K tokens (Gemini 1M) — main + subagents\n")
	case "openai":
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 350000,
			KeepLastN: 20,
		}))
		fmt.Fprintf(os.Stderr, "gollem: auto-context limit: 350K tokens (OpenAI 400K) — main + subagents\n")
	}

	// Build the coding agent with the full recommended setup.
	agentOpts := codetool.AgentOptions(f.workDir, toolOpts...)
	agentOpts = append(agentOpts, core.WithRunCondition[string](core.MaxRunDuration(f.timeout)))

	// Enable reasoning by default for providers that support it.
	// This is provider-agnostic: Anthropic uses ThinkingBudget,
	// OpenAI uses ReasoningEffort. The reasoning sandwich middleware
	// will vary these per-turn for optimal performance.
	if !f.noReasoning {
		switch f.provider {
		case "anthropic", "vertexai-anthropic":
			if f.thinkingBudget < 0 {
				f.thinkingBudget = defaultThinkingBudget
			}
			if f.thinkingBudget > 0 {
				agentOpts = append(agentOpts, core.WithThinkingBudget[string](f.thinkingBudget))
				maxTokens := f.thinkingBudget + 16000
				agentOpts = append(agentOpts, core.WithMaxTokens[string](maxTokens))
				fmt.Fprintf(os.Stderr, "gollem: thinking enabled (budget: %d, max_tokens: %d)\n",
					f.thinkingBudget, maxTokens)
			}
		case "openai":
			// Auto-enable reasoning effort for models that support it:
			// - O-series models (o3, o4-mini, etc.): default to "high"
			// - Codex models (gpt-5.x-codex): default to "xhigh"
			// Other OpenAI-compatible models (grok, together, etc.) may not
			// support the parameter, so don't enable by default.
			isOSeries := strings.HasPrefix(f.modelName, "o") && len(f.modelName) >= 2
			isCodex := strings.Contains(f.modelName, "codex")
			if isCodex {
				if f.reasoningEffort == "" {
					f.reasoningEffort = "xhigh"
				}
				agentOpts = append(agentOpts, core.WithReasoningEffort[string](f.reasoningEffort))
				// Codex with xhigh reasoning needs a large output budget since
				// reasoning tokens count toward max_output_tokens.
				agentOpts = append(agentOpts, core.WithMaxTokens[string](50000))
				agentOpts = append(agentOpts, core.WithTemperature[string](0))
				fmt.Fprintf(os.Stderr, "gollem: codex mode (reasoning: %s, max_output: 50000, temp: 0)\n", f.reasoningEffort)
			} else if isOSeries {
				if f.reasoningEffort == "" {
					f.reasoningEffort = "high"
				}
				agentOpts = append(agentOpts, core.WithReasoningEffort[string](f.reasoningEffort))
				fmt.Fprintf(os.Stderr, "gollem: reasoning effort enabled (%s)\n", f.reasoningEffort)
			} else if f.reasoningEffort != "" {
				agentOpts = append(agentOpts, core.WithReasoningEffort[string](f.reasoningEffort))
				fmt.Fprintf(os.Stderr, "gollem: reasoning effort enabled (%s)\n", f.reasoningEffort)
			}
		case "vertexai":
			// Gemini 2.5+ and 3.x support thinkingConfig.
			if f.thinkingBudget < 0 {
				f.thinkingBudget = defaultThinkingBudget
			}
			if f.thinkingBudget > 0 {
				agentOpts = append(agentOpts, core.WithThinkingBudget[string](f.thinkingBudget))
				fmt.Fprintf(os.Stderr, "gollem: gemini thinking enabled (budget: %d)\n",
					f.thinkingBudget)
			}
		default:
			if f.thinkingBudget > 0 {
				agentOpts = append(agentOpts, core.WithThinkingBudget[string](f.thinkingBudget))
				maxTokens := f.thinkingBudget + 16000
				agentOpts = append(agentOpts, core.WithMaxTokens[string](maxTokens))
				fmt.Fprintf(os.Stderr, "gollem: thinking enabled (budget: %d, max_tokens: %d)\n",
					f.thinkingBudget, maxTokens)
			}
		}
	}

	agent := core.NewAgent[string](model, agentOpts...)

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	fmt.Fprintf(os.Stderr, "gollem: running with %s in %s (timeout: %v)\n",
		baseModel.ModelName(), f.workDir, f.timeout)

	result, err := agent.Run(ctx, f.prompt)

	// Flush Langfuse traces before exiting.
	if langfuseProcessor != nil {
		_ = langfuseProcessor.Close()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Print the result.
	fmt.Println(result.Output)
	fmt.Fprintf(os.Stderr, "\ngollem: done (tokens: %d in, %d out, tools: %d)\n",
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.ToolCalls)
}

// newLangfuseMiddleware creates a provider-level middleware that sends traces
// to Langfuse. It wraps each model request with a generation event containing
// token usage, timing, input messages, and output.
func newLangfuseMiddleware(processor *lf.BatchProcessor) middleware.Middleware {
	traceID := lf.NewID()
	processor.Enqueue(lf.TraceEvent{
		ID:        traceID,
		Name:      "gollem-run",
		Timestamp: time.Now().UTC(),
	})

	turn := 0
	return middleware.Func(func(next middleware.RequestFunc) middleware.RequestFunc {
		return func(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
			turn++
			genID := lf.NewID()
			startTime := time.Now().UTC()

			// Capture input: summarize messages for Langfuse.
			input := langfuseSummarizeMessages(messages)

			resp, err := next(ctx, messages, settings, params)

			endTime := time.Now().UTC()
			gen := lf.GenerationEvent{
				ID:        genID,
				TraceID:   traceID,
				Name:      fmt.Sprintf("turn-%d", turn),
				StartTime: startTime,
				EndTime:   &endTime,
				Input:     input,
			}

			if err == nil && resp != nil {
				gen.Model = resp.ModelName
				gen.Output = langfuseSummarizeResponse(resp)
				gen.Usage = &lf.Usage{
					Input:  resp.Usage.InputTokens,
					Output: resp.Usage.OutputTokens,
					Total:  resp.Usage.TotalTokens(),
					Unit:   "TOKENS",
				}
			} else if err != nil {
				gen.Level = "ERROR"
				gen.StatusMessage = err.Error()
			}

			processor.Enqueue(gen)
			return resp, err
		}
	})
}

// langfuseSummarizeMessages extracts a simplified view of messages for tracing.
func langfuseSummarizeMessages(messages []core.ModelMessage) []map[string]any {
	var result []map[string]any
	for _, msg := range messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.SystemPromptPart:
					content := p.Content
					if len(content) > 500 {
						content = content[:500] + "..."
					}
					result = append(result, map[string]any{"role": "system", "content": content})
				case core.UserPromptPart:
					content := p.Content
					if len(content) > 2000 {
						content = content[:2000] + "..."
					}
					result = append(result, map[string]any{"role": "user", "content": content})
				case core.ToolReturnPart:
					content := fmt.Sprintf("%v", p.Content)
					if len(content) > 1000 {
						content = content[:1000] + "..."
					}
					result = append(result, map[string]any{"role": "tool", "tool": p.ToolName, "content": content})
				}
			}
		case core.ModelResponse:
			result = append(result, map[string]any{"role": "assistant", "content": m.TextContent()})
		}
	}
	return result
}

// langfuseSummarizeResponse extracts text + tool calls from a model response.
func langfuseSummarizeResponse(resp *core.ModelResponse) map[string]any {
	out := map[string]any{}
	if text := resp.TextContent(); text != "" {
		if len(text) > 2000 {
			text = text[:2000] + "..."
		}
		out["text"] = text
	}
	var tools []map[string]string
	for _, part := range resp.Parts {
		if tc, ok := part.(core.ToolCallPart); ok {
			args := tc.ArgsJSON
			if len(args) > 500 {
				args = args[:500] + "..."
			}
			tools = append(tools, map[string]string{"tool": tc.ToolName, "args": args})
		}
	}
	if len(tools) > 0 {
		out["tool_calls"] = tools
	}
	return out
}

func runDebug() {
	f := parseFlags(os.Args[2:])
	provider, modelName, prompt := f.provider, f.modelName, f.prompt

	if prompt == "" {
		fmt.Fprintln(os.Stderr, "error: prompt is required")
		printUsage()
		os.Exit(1)
	}

	if provider == "" {
		provider = "test"
	}

	model, err := createModel(provider, modelName, f.location, f.project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating model: %v\n", err)
		os.Exit(1)
	}

	agent := core.NewAgent[string](model,
		core.WithSystemPrompt[string]("You are a helpful assistant."),
	)

	result, err := tui.DebugUI(agent, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if result != nil {
		fmt.Printf("\nResult: %s\n", result.Output)
		fmt.Printf("Tokens: %d input, %d output\n",
			result.Usage.InputTokens, result.Usage.OutputTokens)
	}
}

// detectTaskTimeout reads the agent timeout from task.toml (Terminal-Bench format).
// This ensures the agent's time budget warnings match the actual deadline.
func detectTaskTimeout(workDir string) time.Duration {
	candidates := []string{
		workDir + "/task.toml",
		"/app/task_file/task.toml",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		inAgentSection := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "[agent]" {
				inAgentSection = true
				continue
			}
			if strings.HasPrefix(trimmed, "[") {
				inAgentSection = false
				continue
			}
			if inAgentSection && strings.Contains(trimmed, "timeout_sec") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					var secs float64
					if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &secs); err == nil && secs > 0 {
						return time.Duration(secs) * time.Second
					}
				}
			}
		}
	}
	return 0
}

func detectProvider() string {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "anthropic"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "openai"
	}
	if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" {
		return "vertexai"
	}
	return ""
}

func createModel(provider, modelName, location, project string) (core.Model, error) {
	// Use an HTTP client with a per-request timeout to prevent individual API
	// calls from hanging forever. Without this, a single hanging connection
	// (Docker networking, API overload) blocks the entire agent timeout,
	// producing zero output. 10 minutes is generous enough for extended
	// thinking responses while still recovering from network hangs.
	httpClient := &http.Client{Timeout: 10 * time.Minute}

	switch provider {
	case "test":
		return core.NewTestModel(
			core.TextResponse("Hello! I'm a test model. This is a demonstration of the TUI debugger."),
		), nil
	case "anthropic":
		opts := []anthropic.Option{anthropic.WithHTTPClient(httpClient)}
		if modelName != "" {
			opts = append(opts, anthropic.WithModel(modelName))
		}
		return anthropic.New(opts...), nil
	case "openai":
		opts := []openai.Option{openai.WithHTTPClient(httpClient)}
		if modelName != "" {
			opts = append(opts, openai.WithModel(modelName))
		}
		return openai.New(opts...), nil
	case "vertexai":
		opts := []vertexai.Option{vertexai.WithHTTPClient(httpClient)}
		if modelName != "" {
			opts = append(opts, vertexai.WithModel(modelName))
		}
		if location != "" {
			opts = append(opts, vertexai.WithLocation(location))
		}
		if project != "" {
			opts = append(opts, vertexai.WithProject(project))
		}
		return vertexai.New(opts...), nil
	case "vertexai-anthropic":
		opts := []vertexai_anthropic.Option{vertexai_anthropic.WithHTTPClient(httpClient)}
		if modelName != "" {
			opts = append(opts, vertexai_anthropic.WithModel(modelName))
		}
		if location != "" {
			opts = append(opts, vertexai_anthropic.WithLocation(location))
		}
		if project != "" {
			opts = append(opts, vertexai_anthropic.WithProject(project))
		}
		return vertexai_anthropic.New(opts...), nil
	default:
		return nil, fmt.Errorf("provider %q not supported (available: test, anthropic, openai, vertexai, vertexai-anthropic)", provider)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `gollem - Coding agent framework CLI

Usage:
  gollem <command> [options] "prompt"

Commands:
  run     Run a coding agent with tools (for benchmarks and automation)
  debug   Run the interactive TUI debugger

Run 'gollem <command> --help' for command-specific help.
`)
}

func printRunUsage() {
	fmt.Fprintf(os.Stderr, `gollem run - Run a coding agent

Usage:
  gollem run [options] "prompt"

Options:
  --provider <name>        Model provider (auto-detected from API keys if not set)
                           Available: anthropic, openai, vertexai, vertexai-anthropic
  --model <name>           Model name (uses provider default if not set)
  --location <region>      GCP region for vertexai providers (default: us-central1)
  --project <id>           GCP project ID for vertexai providers (default: GOOGLE_CLOUD_PROJECT)
  --workdir <path>         Working directory (default: current directory)
  --timeout <duration>     Maximum run time (default: 30m)
  --thinking-budget <n>    Thinking token budget for Anthropic (default: 16000)
  --reasoning-effort <l>   Reasoning effort for OpenAI: low, medium, high (default: high)
  --no-reasoning           Disable all reasoning (thinking + effort)
  --no-code-mode           Disable code mode (monty-go WASM Python batching)
  -h, --help               Show this help

Examples:
  gollem run --provider anthropic "Fix the failing tests in this project"
  gollem run --provider openai --model gpt-5.3 "Implement the TODO items"
  gollem run --workdir /path/to/project "Add error handling to main.go"
  gollem run --provider vertexai --model gemini-3-flash-preview --location global "What is 2+2"
  gollem run --provider anthropic --thinking-budget 64000 "Refactor the auth system"

Environment variables:
  ANTHROPIC_API_KEY        API key for the anthropic provider
  OPENAI_API_KEY           API key for the openai provider
  GOOGLE_CLOUD_PROJECT     GCP project for vertexai and vertexai-anthropic providers
  LANGFUSE_SECRET_KEY      Langfuse secret key (enables tracing)
  LANGFUSE_PUBLIC_KEY      Langfuse public key
  LANGFUSE_BASE_URL        Langfuse API URL (default: https://cloud.langfuse.com)
`)
}

// Command gollem provides a CLI for running coding agents and debugging via TUI.
//
// Usage:
//
//	gollem run --provider anthropic --model claude-opus-4-6 "Fix the failing tests"
//	gollem debug --provider openai --model gpt-4o "prompt"
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	teamMode        string // "auto", "on", "off"
	noCodeMode      bool
	noReasoning     bool // Disable all reasoning (thinking + effort)
}

func parseFlags(args []string) flags {
	teamMode := normalizeTeamMode(os.Getenv("GOLLEM_TEAM_MODE"))
	if teamMode == "" {
		teamMode = "auto"
	}
	f := flags{
		timeout:        30 * time.Minute,
		thinkingBudget: -1, // -1 means "use default"
		teamMode:       teamMode,
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
		case "--team-mode":
			if i+1 < len(args) {
				f.teamMode = normalizeTeamMode(args[i+1])
				if f.teamMode == "" {
					f.teamMode = "auto"
				}
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

	requestTimeout := deriveRequestTimeout(f.timeout)
	baseModel, err := createModel(f.provider, f.modelName, f.location, f.project, requestTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating model: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "gollem: model request timeout: %v\n", requestTimeout)

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
	retryCfg := modelutil.DefaultRetryConfig()
	switch {
	case f.timeout <= 5*time.Minute:
		retryCfg.MaxRetries = 1
		retryCfg.InitialBackoff = 500 * time.Millisecond
		retryCfg.MaxBackoff = 2 * time.Second
	case f.timeout <= 12*time.Minute:
		retryCfg.MaxRetries = 2
		retryCfg.InitialBackoff = 500 * time.Millisecond
		retryCfg.MaxBackoff = 5 * time.Second
	default:
		retryCfg.MaxRetries = 3
		retryCfg.InitialBackoff = 1 * time.Second
		retryCfg.MaxBackoff = 8 * time.Second
	}
	retryCfg.MinRemaining = 20 * time.Second
	model = modelutil.NewRetryModel(model, retryCfg)
	fmt.Fprintf(os.Stderr, "gollem: retry config: max_retries=%d backoff=%v..%v min_remaining=%v\n",
		retryCfg.MaxRetries, retryCfg.InitialBackoff, retryCfg.MaxBackoff, retryCfg.MinRemaining)

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

	// Feature-based team routing: enable team tools on complex, long-horizon tasks.
	// No task-name hardcoding; use prompt/instruction/workspace signals.
	teamEnabled, teamReason := decideTeamMode(f.teamMode, f.workDir, f.prompt, budgetTimeout)
	if teamEnabled {
		toolOpts = append(toolOpts, codetool.WithTeamMode())
		fmt.Fprintf(os.Stderr, "gollem: team mode enabled (%s)\n", teamReason)
	} else {
		fmt.Fprintf(os.Stderr, "gollem: team mode disabled (%s)\n", teamReason)
	}

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
				// Set an explicit output budget so long reasoning turns don't
				// end with token-limit errors before producing a final answer.
				maxTokens := f.thinkingBudget + 24000
				if maxTokens < 40000 {
					maxTokens = 40000
				}
				agentOpts = append(agentOpts, core.WithMaxTokens[string](maxTokens))
				fmt.Fprintf(os.Stderr, "gollem: gemini thinking enabled (budget: %d, max_output: %d)\n",
					f.thinkingBudget, maxTokens)
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

	model, err := createModel(provider, modelName, f.location, f.project, deriveRequestTimeout(f.timeout))
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

type teamModeAutoDecision struct {
	Enabled bool
	Score   int
	Reasons []string
}

func normalizeTeamMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return "auto"
	case "on", "true", "1", "yes":
		return "on"
	case "off", "false", "0", "no":
		return "off"
	default:
		return ""
	}
}

func decideTeamMode(mode, workDir, prompt string, budgetTimeout time.Duration) (bool, string) {
	normalized := normalizeTeamMode(mode)
	if normalized == "" {
		normalized = "auto"
	}
	switch normalized {
	case "on":
		return true, "forced by --team-mode=on (or GOLLEM_TEAM_MODE=on)"
	case "off":
		return false, "forced by --team-mode=off (or GOLLEM_TEAM_MODE=off)"
	default:
		d := evaluateTeamModeAuto(workDir, prompt, budgetTimeout)
		if len(d.Reasons) == 0 {
			return d.Enabled, fmt.Sprintf("auto score=%d", d.Score)
		}
		return d.Enabled, fmt.Sprintf("auto score=%d (%s)", d.Score, strings.Join(d.Reasons, ", "))
	}
}

func evaluateTeamModeAuto(workDir, prompt string, budgetTimeout time.Duration) teamModeAutoDecision {
	instruction := loadTaskInstruction(workDir)
	workspaceFiles := boundedWorkspaceFileCount(workDir, 160)
	return evaluateTeamModeAutoFromSignals(prompt, instruction, budgetTimeout, workspaceFiles)
}

func evaluateTeamModeAutoFromSignals(
	prompt string,
	instruction string,
	budgetTimeout time.Duration,
	workspaceFileCount int,
) teamModeAutoDecision {
	score, reasons := computeTeamModeScore(prompt, instruction, budgetTimeout, workspaceFileCount)

	// Conservative gate:
	// - Enable for very strong complexity signals (score >= 5), OR
	// - Enable for medium/high complexity when the task has enough budget.
	enabled := score >= 5 || (budgetTimeout >= 20*time.Minute && score >= 4)

	if budgetTimeout > 0 && budgetTimeout < 10*time.Minute && score < 6 {
		enabled = false
		reasons = append(reasons, "short timeout")
	}

	return teamModeAutoDecision{
		Enabled: enabled,
		Score:   score,
		Reasons: reasons,
	}
}

func computeTeamModeScore(
	prompt string,
	instruction string,
	budgetTimeout time.Duration,
	workspaceFileCount int,
) (int, []string) {
	score := 0
	reasons := []string{}

	if budgetTimeout >= 20*time.Minute {
		score++
		reasons = append(reasons, "timeout>=20m")
	}
	if budgetTimeout >= 45*time.Minute {
		score++
		reasons = append(reasons, "timeout>=45m")
	}

	chars := len(instruction)
	lines := lineCount(instruction)
	if chars >= 2000 {
		score++
		reasons = append(reasons, "long-instruction")
	}
	if chars >= 3500 {
		score++
		reasons = append(reasons, "very-long-instruction")
	}
	if lines >= 35 {
		score++
		reasons = append(reasons, "many-constraints")
	}

	text := strings.ToLower(prompt + "\n" + instruction)

	if containsAny(text, []string{
		"benchmark", "latency", "throughput", "threshold", "cost model",
		"optimiz", "performance", "speedup", "faster",
	}) {
		score++
		reasons = append(reasons, "perf-sensitive")
	}

	if containsAny(text, []string{
		"pipeline parallel", "tensor parallel", "microbatch", "distributed",
		"all-forward-all-backward", "process group", "scheduler",
	}) {
		score++
		reasons = append(reasons, "distributed-logic")
	}

	if containsAny(text, []string{
		"repository", "repo", "branch", "merge", "bundle", "history",
		"sanitize", "vulnerability", "all files",
	}) {
		score++
		reasons = append(reasons, "repo-wide-edits")
	}

	if containsAny(text, []string{
		"image", "video", "pdf", "ocr", "transcribe", "mask", "jpg", "png",
	}) {
		score++
		reasons = append(reasons, "multimodal")
	}

	if containsAny(text, []string{
		"deliverables", "output_data", "summary.csv", "plan_b1", "plan_b2",
		"output.toml", "exactly these columns",
	}) {
		score++
		reasons = append(reasons, "multi-artifact")
	}

	if workspaceFileCount >= 40 {
		score++
		reasons = append(reasons, "workspace>=40-files")
	}
	if workspaceFileCount >= 120 {
		score++
		reasons = append(reasons, "workspace>=120-files")
	}

	return score, reasons
}

func lineCount(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}

func loadTaskInstruction(workDir string) string {
	candidates := []string{
		filepath.Join(workDir, "instruction.md"),
		filepath.Join(workDir, "task_file", "instruction.md"),
		"/app/instruction.md",
		"/app/task_file/instruction.md",
		filepath.Join(workDir, "README.md"),
		"/app/README.md",
	}

	for _, path := range candidates {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		// Cap for predictable startup overhead; enough for signal extraction.
		if len(data) > 20000 {
			data = data[:20000]
		}
		return string(data)
	}
	return ""
}

func boundedWorkspaceFileCount(workDir string, max int) int {
	if workDir == "" || max <= 0 {
		return 0
	}

	count := 0
	stop := errors.New("stop-count")
	skipDirs := map[string]bool{
		".git":         true,
		".venv":        true,
		"venv":         true,
		"node_modules": true,
		"__pycache__":  true,
		"dist":         true,
		"build":        true,
		".cache":       true,
	}

	err := filepath.WalkDir(workDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && path != workDir {
				return filepath.SkipDir
			}
			return nil
		}
		count++
		if count >= max {
			return stop
		}
		return nil
	})

	if err != nil && !errors.Is(err, stop) {
		return count
	}
	return count
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

func deriveRequestTimeout(runTimeout time.Duration) time.Duration {
	// Keep per-request timeouts much shorter than the full run timeout so
	// a single hung provider call doesn't consume the whole benchmark budget.
	timeout := runTimeout / 5
	if timeout < 45*time.Second {
		timeout = 45 * time.Second
	}
	if timeout > 4*time.Minute {
		timeout = 4 * time.Minute
	}
	return timeout
}

func createModel(provider, modelName, location, project string, requestTimeout time.Duration) (core.Model, error) {
	// Use an HTTP client with a bounded per-request timeout so transient
	// provider hangs fail fast and can be retried within the run budget.
	httpClient := &http.Client{Timeout: requestTimeout}

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
  --team-mode <mode>       Team tool routing: auto, on, off (default: auto)
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
  OPENAI_PROMPT_CACHE_KEY  Optional stable key for OpenAI prompt caching
  OPENAI_PROMPT_CACHE_RETENTION Optional OpenAI cache retention policy (e.g. in_memory, 24h)
  GOLLEM_TEAM_MODE         Team mode override: auto, on, off (default: auto)
  GOLLEM_CODE_MODE_FAILURE_THRESHOLD Consecutive execute_code capability failures before cooldown (default: 3)
  GOLLEM_CODE_MODE_COOLDOWN_TURNS Turns to keep execute_code disabled before retrying (default: 2)
  GOLLEM_CODE_MODE_MAX_RECENT_RESULTS Number of recent execute_code results to inspect (default: 12)
  GOOGLE_CLOUD_PROJECT     GCP project for vertexai and vertexai-anthropic providers
  LANGFUSE_SECRET_KEY      Langfuse secret key (enables tracing)
  LANGFUSE_PUBLIC_KEY      Langfuse public key
  LANGFUSE_BASE_URL        Langfuse API URL (default: https://cloud.langfuse.com)
`)
}

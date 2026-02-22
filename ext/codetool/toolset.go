package codetool

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/deep"
	"github.com/fugue-labs/gollem/ext/monty"
	"github.com/fugue-labs/gollem/ext/team"
	"github.com/fugue-labs/gollem/modelutil"
)

// Toolset returns all coding agent tools as a core.Toolset.
// Use this to add the full suite of coding tools to an agent:
//
//	ts := codetool.Toolset(codetool.WithWorkDir("/my/project"))
//	agent := core.NewAgent(model, "...", core.WithToolset(ts))
func Toolset(opts ...Option) *core.Toolset {
	return core.NewToolset("codetool",
		Bash(opts...),
		View(opts...),
		Write(opts...),
		Edit(opts...),
		MultiEdit(opts...),
		Grep(opts...),
		Glob(opts...),
		Ls(opts...),
	)
}

// AllTools returns all coding agent tools as a slice.
func AllTools(opts ...Option) []core.Tool {
	return []core.Tool{
		Bash(opts...),
		View(opts...),
		Write(opts...),
		Edit(opts...),
		MultiEdit(opts...),
		Grep(opts...),
		Glob(opts...),
		Ls(opts...),
	}
}

// AgentOptions returns the recommended set of agent options for a coding agent.
// This includes the system prompt, toolset, middleware (loop detection, context
// injection, verification checkpoint), guardrails, tracing, hooks, auto-context
// management, and tool timeouts.
//
// Usage:
//
//	opts := codetool.AgentOptions("/path/to/project")
//	agent := core.NewAgent[string](model, opts...)
func AgentOptions(workDir string, toolOpts ...Option) []core.AgentOption[string] {
	if workDir != "" {
		toolOpts = append([]Option{WithWorkDir(workDir)}, toolOpts...)
	}
	cfg := applyOpts(toolOpts)
	verifyMW, verifyValidator := VerificationCheckpoint()

	// Build system prompt and tool options.
	// When code mode is enabled, the agent gets both individual tools AND
	// an execute_code tool that batches N tool calls per API round-trip.
	systemPrompt := SystemPrompt
	toolOptions := []core.AgentOption[string]{
		core.WithToolsets[string](Toolset(toolOpts...)),
	}
	if cfg.Runner != nil {
		cm := monty.New(cfg.Runner, AllTools(toolOpts...))
		toolOptions = append(toolOptions, core.WithTools[string](cm.Tool()))
		systemPrompt += "\n\n" + cm.SystemPrompt()
	}

	// Planning tool: persistent task list for tracking progress on multi-step work.
	toolOptions = append(toolOptions, core.WithTools[string](deep.PlanningTool()))

	// Default personality generation: when a model is available but no
	// explicit generator was provided, auto-create a cached generator.
	// Append to toolOpts so it flows through to SubAgentTool as well.
	if cfg.Model != nil && cfg.PersonalityGenerator == nil {
		cfg.PersonalityGenerator = modelutil.CachedPersonalityGenerator(
			modelutil.GeneratePersonality(cfg.Model),
		)
		toolOpts = append(toolOpts, WithPersonalityGenerator(cfg.PersonalityGenerator))
	}

	// SubAgent delegation: spawn focused subagents for subtask execution.
	// Disabled in team mode (team tools replace delegation).
	if cfg.Model != nil && !cfg.TeamMode {
		toolOptions = append(toolOptions, core.WithTools[string](SubAgentTool(cfg.Model, toolOpts...)))
	}

	// Team mode: leader agent with tools to spawn teammates and coordinate work.
	var teamLeaderMW core.AgentMiddleware
	if cfg.TeamMode && cfg.Model != nil {
		t := team.NewTeam(team.TeamConfig{
			Name:                 "coding-team",
			Leader:               "leader",
			Model:                cfg.Model,
			Toolset:              Toolset(toolOpts...),
			PersonalityGenerator: cfg.PersonalityGenerator,
		})
		// Register the leader so workers can send messages to it.
		teamLeaderMW = t.RegisterLeader("leader")
		toolOptions = append(toolOptions,
			core.WithTools[string](team.LeaderTools(t)...),
		)
		systemPrompt += "\n\n" + team.LeaderSystemPrompt("coding-team")
	}

	opts := []core.AgentOption[string]{
		// System prompt with coding agent instructions (+ code mode if enabled).
		core.WithSystemPrompt[string](systemPrompt),
	}
	opts = append(opts, toolOptions...)
	opts = append(opts,
		// Retry malformed output up to 3 times.
		core.WithMaxRetries[string](3),

		// Middleware chain (outermost first):
		// 1. Loop detection — break doom loops of repeated edits.
		core.WithAgentMiddleware[string](LoopDetectionMiddleware(4)),
		// 2. Context injection — discover environment on first turn.
		core.WithAgentMiddleware[string](ContextInjectionMiddleware(workDir)),
		// 3. Reasoning sandwich — vary thinking budget by phase.
		core.WithAgentMiddleware[string](ReasoningSandwichMiddleware(DefaultReasoningSandwichConfig())),
		// 4. Verification tracking — track whether agent runs tests.
		core.WithAgentMiddleware[string](verifyMW),
		// 5. Stderr logging — real-time tool call observability.
		core.WithAgentMiddleware[string](stderrLoggingMiddleware()),

		// Output validator: reject completion without verification.
		core.WithOutputValidator[string](verifyValidator),

		// Override default request limit of 50 — coding tasks need more turns.
		core.WithUsageLimits[string](core.UsageLimits{RequestLimit: core.IntPtr(500)}),

		// Guardrails: prevent infinite loops.
		core.WithTurnGuardrail[string]("max-turns", core.MaxTurns(500)),

		// Tool timeout: individual tools get 2 minutes max.
		core.WithDefaultToolTimeout[string](2 * time.Minute),

		// Auto context: compress old messages when context grows too large.
		core.WithAutoContext[string](core.AutoContextConfig{
			MaxTokens: 180000,
			KeepLastN: 10,
		}),

		// Tracing: capture full execution trace for post-run analysis.
		core.WithTracing[string](),

		// Hooks: real-time observability to stderr.
		core.WithHooks[string](core.Hook{
			OnRunStart: func(_ context.Context, _ *core.RunContext, prompt string) {
				if len(prompt) > 100 {
					prompt = prompt[:100] + "..."
				}
				fmt.Fprintf(os.Stderr, "[gollem] run started: %s\n", prompt)
			},
			OnRunEnd: func(_ context.Context, _ *core.RunContext, _ []core.ModelMessage, err error) {
				if err != nil {
					fmt.Fprintf(os.Stderr, "[gollem] run ended with error: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "[gollem] run completed successfully\n")
				}
			},
			OnToolStart: func(_ context.Context, _ *core.RunContext, name string, argsJSON string) {
				summary := argsJSON
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
				fmt.Fprintf(os.Stderr, "[gollem] tool:start %s %s\n", name, summary)
			},
			OnToolEnd: func(_ context.Context, _ *core.RunContext, name string, result string, err error) {
				if err != nil {
					fmt.Fprintf(os.Stderr, "[gollem] tool:end   %s ERROR: %v\n", name, err)
				} else {
					summary := result
					if len(summary) > 150 {
						summary = summary[:150] + "..."
					}
					fmt.Fprintf(os.Stderr, "[gollem] tool:end   %s %s\n", name, strings.ReplaceAll(summary, "\n", "\\n"))
				}
			},
		}),
	)

	// Team leader middleware: drain incoming messages from workers between turns.
	if teamLeaderMW != nil {
		opts = append(opts, core.WithAgentMiddleware[string](teamLeaderMW))
	}

	// Time budget awareness: warn the agent when approaching timeout.
	if cfg.Timeout > 0 {
		opts = append(opts, core.WithAgentMiddleware[string](TimeBudgetMiddleware(cfg.Timeout)))
	}

	// Export traces to /tmp/gollem-traces if the dir is writable.
	traceDir := "/tmp/gollem-traces"
	if err := os.MkdirAll(traceDir, 0o755); err == nil {
		opts = append(opts, core.WithTraceExporter[string](core.NewJSONFileExporter(traceDir)))
	}

	return opts
}

// stderrLoggingMiddleware logs each model turn to stderr with timing.
func stderrLoggingMiddleware() core.AgentMiddleware {
	turn := 0
	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		turn++
		fmt.Fprintf(os.Stderr, "[gollem] turn %d: sending %d messages to model\n", turn, len(messages))
		start := time.Now()
		resp, err := next(ctx, messages, settings, params)
		elapsed := time.Since(start)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[gollem] turn %d: model error after %v: %v\n", turn, elapsed, err)
		} else {
			// Count tool calls in response.
			toolCalls := 0
			for _, part := range resp.Parts {
				if _, ok := part.(core.ToolCallPart); ok {
					toolCalls++
				}
			}
			textLen := len(resp.TextContent())
			fmt.Fprintf(os.Stderr, "[gollem] turn %d: response in %v (tools: %d, text: %d chars)\n",
				turn, elapsed.Round(time.Millisecond), toolCalls, textLen)
		}
		return resp, err
	}
}

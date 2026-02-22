package codetool

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/modelutil"
)

// subagentParams is the input schema for the delegate tool.
type subagentParams struct {
	Task string `json:"task" jsonschema:"description=A clear, self-contained description of the subtask to delegate. Include all necessary context — the subagent has no memory of previous conversation."`
}

// SubAgentTool creates a tool that delegates subtasks to a fresh agent.
// The subagent gets the same coding tools but runs with a focused prompt
// and limited turns, preventing runaway execution. Results are returned
// to the parent agent.
//
// This mirrors the "task" tool in Deep Agents / Claude Code — the key
// differentiator on benchmarks like Terminal-Bench where complex tasks
// benefit from decomposition into focused subtasks.
func SubAgentTool(model core.Model, opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[subagentParams](
		"delegate",
		"Delegate a subtask to a focused subagent. The subagent gets the same "+
			"coding tools (bash, view, edit, write, grep, glob, ls) and runs "+
			"independently with its own context. Use this for: (1) parallel-safe "+
			"subtasks like researching an API or reading large files, (2) focused "+
			"debugging of a specific component, (3) writing a self-contained module "+
			"or test. The subagent has NO memory of your conversation — include all "+
			"necessary context in the task description.",
		func(ctx context.Context, params subagentParams) (any, error) {
			if params.Task == "" {
				return nil, &core.ModelRetryError{Message: "task description must not be empty"}
			}

			// Generate a task-specific system prompt if a personality generator
			// is configured, falling back to the static prompt on error.
			systemPrompt := subAgentSystemPrompt
			if cfg.PersonalityGenerator != nil {
				if generated, err := cfg.PersonalityGenerator(ctx, modelutil.PersonalityRequest{
					Task:       params.Task,
					Role:       "focused coding subagent",
					BasePrompt: subAgentSystemPrompt,
				}); err == nil {
					systemPrompt = generated
					fmt.Fprintf(os.Stderr, "[gollem] subagent:personality generated (%d chars)\n", len(generated))
				} else {
					fmt.Fprintf(os.Stderr, "[gollem] subagent:personality fallback: %v\n", err)
				}
			}

			// Build a lightweight subagent with coding tools but no delegation
			// (prevents infinite recursion).
			subOpts := []core.AgentOption[string]{
				core.WithSystemPrompt[string](systemPrompt),
				core.WithToolsets[string](Toolset(opts...)),
				core.WithMaxRetries[string](2),
				core.WithUsageLimits[string](core.UsageLimits{RequestLimit: core.IntPtr(50)}),
				core.WithTurnGuardrail[string]("max-turns", core.MaxTurns(50)),
				core.WithDefaultToolTimeout[string](2 * time.Minute),
				// Auto-compress context on long subtasks to prevent context overflow.
				core.WithAutoContext[string](core.AutoContextConfig{
					MaxTokens: 80000,
					KeepLastN: 8,
				}),
			}

			agent := core.NewAgent[string](model, subOpts...)

			fmt.Fprintf(os.Stderr, "[gollem] subagent:start task=%q\n", truncateForLog(params.Task, 100))

			result, err := agent.Run(ctx, params.Task)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[gollem] subagent:error %v\n", err)
				return nil, fmt.Errorf("subagent failed: %w", err)
			}

			fmt.Fprintf(os.Stderr, "[gollem] subagent:done (tokens: %d in, %d out, tools: %d)\n",
				result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.ToolCalls)

			return result.Output, nil
		},
	)
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

const subAgentSystemPrompt = `You are a focused coding assistant executing a specific subtask.
You have access to bash, view, edit, write, grep, glob, and ls tools.

## Be Concise
Minimize text output. Every character costs tokens. Don't explain what you're about to do — just do it.

## Rules
1. Complete the assigned task precisely — don't do extra work
2. Read relevant files before modifying them
3. Make precise edits — match exact strings including whitespace
4. Verify your changes work (run tests/builds when appropriate)
5. If something fails, read the FULL error message, fix the root cause, and verify
6. Clean up any temporary/build artifacts you create
7. Report what you did and the outcome clearly in your final response
8. If the task is impossible or blocked, explain why immediately — don't waste turns

## Error Recovery
When something fails:
1. Read the FULL error output — don't skim
2. Identify the file and line number
3. View that file section
4. Understand WHY it failed before attempting a fix
5. Make the minimal fix needed
6. Re-run the exact same command that failed

If the same fix fails twice, try a fundamentally different approach.

## Performance
Write efficient code. Use O(n log n) over O(n²), hash maps for lookups, vectorized operations over loops. Prefer built-in/native operations.

Your response will be returned to the parent agent, so be concise but complete.`

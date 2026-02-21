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
	"os"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/tui"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
	"github.com/fugue-labs/gollem/provider/vertexai"
	"github.com/fugue-labs/gollem/provider/vertexai_anthropic"
)

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

func parseFlags(args []string) (provider, modelName, workDir, prompt string, timeout time.Duration, thinkingBudget int) {
	provider = ""
	timeout = 30 * time.Minute
	thinkingBudget = 0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 < len(args) {
				provider = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				modelName = args[i+1]
				i++
			}
		case "--workdir":
			if i+1 < len(args) {
				workDir = args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					timeout = d
				}
				i++
			}
		case "--thinking-budget":
			if i+1 < len(args) {
				if _, err := fmt.Sscanf(args[i+1], "%d", &thinkingBudget); err != nil {
					thinkingBudget = 0
				}
				i++
			}
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		default:
			if !strings.HasPrefix(args[i], "-") && prompt == "" {
				prompt = args[i]
			}
		}
	}

	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	return provider, modelName, workDir, prompt, timeout, thinkingBudget
}

func runAgent() {
	provider, modelName, workDir, prompt, timeout, thinkingBudget := parseFlags(os.Args[2:])

	if prompt == "" {
		fmt.Fprintln(os.Stderr, "error: prompt is required")
		printRunUsage()
		os.Exit(1)
	}

	if provider == "" {
		provider = detectProvider()
		if provider == "" {
			fmt.Fprintln(os.Stderr, "error: --provider is required (or set ANTHROPIC_API_KEY / OPENAI_API_KEY)")
			os.Exit(1)
		}
	}

	model, err := createModel(provider, modelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating model: %v\n", err)
		os.Exit(1)
	}

	// Build the coding agent with the full recommended setup: tools, system
	// prompt, loop detection, context injection, and verification checkpoint.
	agentOpts := codetool.AgentOptions(workDir)
	agentOpts = append(agentOpts, core.WithRunCondition[string](core.MaxRunDuration(timeout)))

	if thinkingBudget > 0 {
		agentOpts = append(agentOpts, core.WithThinkingBudget[string](thinkingBudget))
	}

	agent := core.NewAgent[string](model, agentOpts...)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Fprintf(os.Stderr, "gollem: running with %s in %s\n", model.ModelName(), workDir)

	result, err := agent.Run(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Print the result.
	fmt.Println(result.Output)
	fmt.Fprintf(os.Stderr, "\ngollem: done (tokens: %d in, %d out, tools: %d)\n",
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.ToolCalls)
}

func runDebug() {
	provider, modelName, _, prompt, _, _ := parseFlags(os.Args[2:])

	if prompt == "" {
		fmt.Fprintln(os.Stderr, "error: prompt is required")
		printUsage()
		os.Exit(1)
	}

	if provider == "" {
		provider = "test"
	}

	model, err := createModel(provider, modelName)
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

func createModel(provider, modelName string) (core.Model, error) {
	switch provider {
	case "test":
		return core.NewTestModel(
			core.TextResponse("Hello! I'm a test model. This is a demonstration of the TUI debugger."),
		), nil
	case "anthropic":
		var opts []anthropic.Option
		if modelName != "" {
			opts = append(opts, anthropic.WithModel(modelName))
		}
		return anthropic.New(opts...), nil
	case "openai":
		var opts []openai.Option
		if modelName != "" {
			opts = append(opts, openai.WithModel(modelName))
		}
		return openai.New(opts...), nil
	case "vertexai":
		var opts []vertexai.Option
		if modelName != "" {
			opts = append(opts, vertexai.WithModel(modelName))
		}
		return vertexai.New(opts...), nil
	case "vertexai-anthropic":
		var opts []vertexai_anthropic.Option
		if modelName != "" {
			opts = append(opts, vertexai_anthropic.WithModel(modelName))
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
  --workdir <path>         Working directory (default: current directory)
  --timeout <duration>     Maximum run time (default: 30m)
  --thinking-budget <n>    Thinking/reasoning token budget (Anthropic extended thinking)
  -h, --help               Show this help

Examples:
  gollem run --provider anthropic "Fix the failing tests in this project"
  gollem run --provider openai --model gpt-4o "Implement the TODO items"
  gollem run --workdir /path/to/project "Add error handling to main.go"
  gollem run --provider anthropic --thinking-budget 10000 "Refactor the auth system"

Environment variables:
  ANTHROPIC_API_KEY       API key for the anthropic provider
  OPENAI_API_KEY          API key for the openai provider
  GOOGLE_CLOUD_PROJECT    GCP project for vertexai and vertexai-anthropic providers
`)
}

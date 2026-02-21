package codetool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// VerificationCheckpoint creates a middleware and output validator pair that
// forces the agent to run verification commands (tests, builds, linting) before
// declaring completion. This is the single highest-impact harness technique for
// coding benchmarks — LangChain reported +13.7 points from harness engineering
// alone, with self-verification being the biggest contributor.
//
// The middleware tracks bash tool calls across the conversation. The output
// validator rejects the agent's output if no verification was detected, forcing
// a retry with instructions to verify.
//
// Usage:
//
//	mw, validator := codetool.VerificationCheckpoint()
//	agent := core.NewAgent[string](model,
//	    core.WithAgentMiddleware[string](mw),
//	    core.WithOutputValidator[string](validator),
//	)
func VerificationCheckpoint() (core.AgentMiddleware, core.OutputValidatorFunc[string]) {
	var mu sync.Mutex
	verified := false

	mw := func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		mu.Lock()
		if !verified {
			for _, msg := range messages {
				if resp, ok := msg.(core.ModelResponse); ok {
					for _, part := range resp.Parts {
						if tc, ok := part.(core.ToolCallPart); ok && tc.ToolName == "bash" {
							if isVerificationCommand(tc.ArgsJSON) {
								verified = true
								break
							}
						}
					}
					if verified {
						break
					}
				}
			}
		}
		mu.Unlock()

		return next(ctx, messages, settings, params)
	}

	validator := func(_ context.Context, _ *core.RunContext, output string) (string, error) {
		mu.Lock()
		v := verified
		mu.Unlock()

		if !v {
			return output, &core.ModelRetryError{
				Message: "STOP. You MUST verify your changes before completing the task. " +
					"Run the relevant test suite (e.g., `go test ./...`, `pytest`, `npm test`) " +
					"and/or build command (e.g., `go build ./...`, `make`, `cargo build`) to " +
					"confirm your changes are correct. Do NOT declare completion without " +
					"evidence that your solution works.",
			}
		}
		return output, nil
	}

	return mw, validator
}

// isVerificationCommand checks whether a bash tool call's ArgsJSON contains a
// command that looks like a test, build, or lint verification step.
func isVerificationCommand(argsJSON string) bool {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}

	cmd := strings.ToLower(args.Command)

	// Test commands.
	testPatterns := []string{
		"go test",
		"pytest", "python -m pytest",
		"npm test", "npm run test", "yarn test", "pnpm test",
		"npx jest", "npx mocha", "npx vitest",
		"cargo test",
		"make test", "make check",
		"mvn test", "gradle test",
		"dotnet test",
		"ruby -e", "rake test", "rspec", "bundle exec",
		"phpunit",
	}

	// Build/compile commands.
	buildPatterns := []string{
		"go build", "go vet",
		"npm run build", "yarn build", "pnpm build",
		"cargo build", "cargo check", "cargo clippy",
		"make", "cmake",
		"gcc ", "g++ ", "clang ",
		"javac ", "mvn compile", "gradle build",
		"dotnet build",
		"tsc",
		"python -m py_compile", "python -c",
	}

	// Lint/check commands.
	lintPatterns := []string{
		"eslint", "pylint", "flake8", "mypy",
		"golangci-lint", "staticcheck", "go vet",
		"rubocop", "shellcheck",
	}

	for _, p := range testPatterns {
		if strings.Contains(cmd, p) {
			return true
		}
	}
	for _, p := range buildPatterns {
		if strings.Contains(cmd, p) {
			return true
		}
	}
	for _, p := range lintPatterns {
		if strings.Contains(cmd, p) {
			return true
		}
	}

	return false
}

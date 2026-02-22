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
	completionAttempts := 0

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
		attempts := completionAttempts
		completionAttempts++
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

		// Pre-completion checklist: on first completion attempt after verification,
		// force the agent to re-check requirements. This catches cases where the
		// agent ran tests but missed requirements.
		if attempts == 0 {
			return output, &core.ModelRetryError{
				Message: "Before finalizing: run through this checklist.\n" +
					"1. Re-read the ORIGINAL task requirements — did you address every single point?\n" +
					"2. List all output/working directories — are there leftover files that shouldn't be there? (rm build artifacts, temp files, __pycache__, .o files)\n" +
					"3. If there are test scripts in /tests/ or test directories, run them one more time to confirm they pass.\n" +
					"4. If global constraints exist (e.g., 'max N across all outputs'), verify them with a script.\n" +
					"Only declare completion after confirming all the above.",
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
		"pytest", "python -m pytest", "python -m unittest", "python3 -m unittest",
		"npm test", "npm run test", "yarn test", "pnpm test",
		"npx jest", "npx mocha", "npx vitest",
		"cargo test",
		"make test", "make check",
		"mvn test", "gradle test",
		"dotnet test",
		"ruby -e", "rake test", "rspec", "bundle exec", "ruby -itest",
		"phpunit",
		"ctest",
		"julia -e", "julia --project",
		// Terminal-Bench patterns: tasks often have test scripts.
		"test_outputs", "test_output", "run_tests",
		"python3 /app/test", "python /app/test",
		"python3 /tests/", "python /tests/",
		"python3 test_", "python test_",
		"bash /tests/", "sh /tests/",
		"bash run_", "sh run_",
		"./test", "./run_test", "./check", "./verify", "./run_verify",
		"./run_", // common script naming pattern
		"node test", "node /tests/", "node /app/test",
		"python3 verify", "python verify",
		"python3 check", "python check",
		"python3 /app/scripts/", "python /app/scripts/",
		"bash /app/scripts/", "sh /app/scripts/",
	}

	// Build/compile commands.
	buildPatterns := []string{
		"go build", "go vet",
		"npm run build", "yarn build", "pnpm build",
		"cargo build", "cargo check", "cargo clippy",
		"make", "cmake",
		"gcc ", "g++ ", "clang ", "cc ",
		"javac ", "mvn compile", "gradle build",
		"dotnet build",
		"tsc",
		"python -m py_compile", "python -c",
		"python3 -c",
		"rustc ",
		"gfortran ", "gdc ", "ldc2 ",
	}

	// Lint/check commands.
	lintPatterns := []string{
		"eslint", "pylint", "flake8", "mypy",
		"golangci-lint", "staticcheck", "go vet",
		"rubocop", "shellcheck",
		"black --check", "ruff check",
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

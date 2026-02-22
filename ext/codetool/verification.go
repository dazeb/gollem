package codetool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
// When a timeout is provided, the pre-completion checklist is skipped if more
// than 80% of the time has elapsed — wasting a turn on the checklist when the
// agent is about to be killed is counterproductive.
//
// When workDir is provided, the pre-completion checklist also programmatically
// checks that expected output files actually exist.
//
// Usage:
//
//	mw, validator := codetool.VerificationCheckpoint("/app", timeout)
//	agent := core.NewAgent[string](model,
//	    core.WithAgentMiddleware[string](mw),
//	    core.WithOutputValidator[string](validator),
//	)
func VerificationCheckpoint(workDir string, timeout ...time.Duration) (core.AgentMiddleware, core.OutputValidatorFunc[string]) {
	var mu sync.Mutex
	verified := false
	completionAttempts := 0
	startTime := time.Now()

	// Determine effective timeout for skip-checklist logic.
	effectiveTimeout := time.Duration(0)
	if len(timeout) > 0 && timeout[0] > 0 {
		effectiveTimeout = timeout[0]
	}
	if effectiveTimeout == 0 {
		if envTimeout := os.Getenv("GOLLEM_TIMEOUT_SEC"); envTimeout != "" {
			var secs float64
			if _, err := fmt.Sscanf(envTimeout, "%f", &secs); err == nil && secs > 0 {
				effectiveTimeout = time.Duration(secs) * time.Second
			}
		}
	}

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
						if tc, ok := part.(core.ToolCallPart); ok {
							if tc.ToolName == "bash" && isVerificationCommand(tc.ArgsJSON) {
								verified = true
								break
							}
							if tc.ToolName == "execute_code" && isVerificationCode(tc.ArgsJSON) {
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
		//
		// Skip the checklist if time is running out — wasting a turn when the
		// agent is about to be killed is worse than missing a requirement.
		if attempts == 0 {
			skipChecklist := false
			if effectiveTimeout > 0 {
				elapsed := time.Since(startTime)
				pct := float64(elapsed) / float64(effectiveTimeout)
				if pct > 0.80 {
					skipChecklist = true
					fmt.Fprintf(os.Stderr, "[gollem] verification: skipping checklist (%.0f%% time elapsed)\n", pct*100)
				}
			}
			if !skipChecklist {
				// Programmatic check: are expected output files actually present?
				// This catches agents that ran tests but forgot to create outputs.
				var missingOutputHint string
				if workDir != "" {
					missingOutputHint = checkExpectedOutputsExist(workDir)
				}
				checklistMsg := "Before finalizing: run through this checklist.\n" +
					"1. Re-read the ORIGINAL task requirements — did you address every single point?\n" +
					"2. Clean up known build intermediates only (tests often check directory contents with os.listdir/ls):\n" +
					"   Run: find . -name '__pycache__' -type d -exec rm -rf {} + 2>/dev/null; find . -name '*.pyc' -delete 2>/dev/null; rm -f *.o a.out 2>/dev/null\n" +
					"   DO NOT delete files that are part of your solution (executables, source files, output data).\n" +
					"3. If there are test scripts in /tests/ or test directories, run them one more time to confirm they pass.\n" +
					"4. If global constraints exist (e.g., 'max N across all outputs'), verify them with a script.\n" +
					"5. Check output file formatting (common gotchas that cause test failures):\n" +
					"   - Trailing newline: some tests expect it, some don't. Check with: xxd <file> | tail -1\n" +
					"   - Encoding: ensure UTF-8 (no BOM). Check with: file <output_file>\n" +
					"   - If tests compare output: diff your output against expected output character-by-character\n"
				if missingOutputHint != "" {
					checklistMsg += missingOutputHint
				}
				checklistMsg += "Only declare completion after confirming all the above."
				return output, &core.ModelRetryError{Message: checklistMsg}
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
	return isVerificationString(strings.ToLower(args.Command))
}

// isVerificationCode checks whether an execute_code tool call's ArgsJSON
// contains code that looks like verification (running tests, checking output,
// comparing results). This handles the case where the agent uses execute_code
// instead of bash for verification.
func isVerificationCode(argsJSON string) bool {
	var args struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	code := strings.ToLower(args.Code)

	// Check if the code calls bash() with a verification command.
	// execute_code wraps tool calls as Python functions, e.g.:
	//   bash(command="python /app/test_outputs.py")
	//   bash(command="pytest")
	if strings.Contains(code, "bash(") {
		return isVerificationString(code)
	}

	// Check for verification-like code patterns.
	codeVerifyPatterns := []string{
		"assert ", "assert(",       // Python assertions
		"assertEqual", "assertTrue", // unittest assertions
		"test_output", "test_result", "run_test",
		"verify(", "validate(",
		"expected", "== expected",
		"diff(", "compare(",
		"open(", // reading output files to check them
	}
	for _, p := range codeVerifyPatterns {
		if strings.Contains(code, p) {
			return true
		}
	}
	return false
}

// isVerificationString checks whether a command/code string contains patterns
// that look like verification (tests, builds, lints, constraint checks).
func isVerificationString(cmd string) bool {
	// Test commands.
	testPatterns := []string{
		"go test",
		"pytest", "python -m pytest", "python -m unittest", "python3 -m unittest",
		"npm test", "npm run test", "yarn test", "pnpm test",
		"npx jest", "npx mocha", "npx vitest",
		"cargo test",
		"make test", "make check",
		"mvn test", "gradle test", "gradlew test", "./gradlew",
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
		// Pattern: inline test execution.
		"python3 -c \"import", "python -c \"import",
		"python3 -c 'import", "python -c 'import",
		// Pattern: pmars (corewars simulator).
		"pmars ",
		// Lean 4 build (theorem proving).
		"lake build", "lake env",
		// OCaml build systems.
		"dune build", "dune test", "dune exec",
		// Haskell build systems.
		"stack build", "stack test", "cabal build", "cabal test",
		// Coq proof checker.
		"coqc ", "coq_makefile",
		// Elixir/Erlang.
		"mix test", "mix compile",
		// Zig build.
		"zig build", "zig test",
		// Perl.
		"prove ", "perl -e",
		// R language.
		"rscript ", "rscript -e",
		// Swift.
		"swift test", "swift build",
		// Nim.
		"nim c ", "nim compile", "nim test",
		// Inline output validation patterns.
		"python3 -c \"open(", "python -c \"open(",
		"python3 -c 'open(", "python -c 'open(",
		// Solution execution with output redirect (common test pattern).
		"./solution >", "./program >", "./main >",
	}

	// Build/compile commands.
	buildPatterns := []string{
		"go build", "go vet",
		"npm run build", "yarn build", "pnpm build",
		"cargo build", "cargo check", "cargo clippy",
		"make", "cmake",
		"gcc ", "g++ ", "clang ", "cc ",
		"javac ", "mvn compile", "gradle build", "gradlew build",
		"dotnet build",
		"tsc",
		"python -m py_compile", "python -c",
		"python3 -c",
		"rustc ",
		"gfortran ", "gdc ", "ldc2 ",
		// Assembly.
		"nasm ", "yasm ",
		// Swift.
		"swiftc ",
		// Nim.
		"nim c ",
		// OCaml direct compilation.
		"ocamlopt ", "ocamlfind ",
		// Haskell direct compilation.
		"ghc ",
	}

	// Lint/check commands.
	lintPatterns := []string{
		"eslint", "pylint", "flake8", "mypy",
		"golangci-lint", "staticcheck", "go vet",
		"rubocop", "shellcheck",
		"black --check", "ruff check",
	}

	// Constraint verification commands (size checks, output validation).
	constraintPatterns := []string{
		"wc -c", "wc -l", "stat ",
		"du -", "file ",
		"diff ", "cmp ",
		"md5sum", "sha256sum", "sha1sum",
		"grep -c", "grep --count", // counting matches is verification
		"sqlite3 ",                 // querying database to verify contents
		"xxd ",                     // hex dump for byte-level comparison
		"valgrind ",                // memory leak checking
		"curl localhost", "curl 127.0.0.1", "curl http://localhost", // service verification
		"wget localhost", "wget 127.0.0.1",
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
	for _, p := range constraintPatterns {
		if strings.Contains(cmd, p) {
			return true
		}
	}

	return false
}

// checkExpectedOutputsExist checks whether expected output files/directories
// actually exist. Returns a warning string if missing outputs are detected,
// or empty string if everything looks fine. Called during the pre-completion
// checklist to programmatically verify deliverables exist.
func checkExpectedOutputsExist(workDir string) string {
	// Check for common output directories that should be populated.
	outputDirs := []struct {
		path string
		name string
	}{
		{filepath.Join(workDir, "output_data"), "output_data/"},
		{"/app/task_file/output_data", "/app/task_file/output_data/"},
		{filepath.Join(workDir, "output"), "output/"},
	}
	for _, od := range outputDirs {
		info, err := os.Stat(od.path)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(od.path)
		if err != nil {
			continue
		}
		// Count non-hidden files.
		fileCount := 0
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				fileCount++
			}
		}
		if fileCount == 0 {
			return fmt.Sprintf("6. WARNING: %s directory exists but is EMPTY! You likely need to create output files in it.\n", od.name)
		}
	}

	// Check for expected solution files (common deliverable names).
	solutionFiles := []string{
		"solution.py", "solution.js", "solution.ts", "solution.go",
		"solution.rs", "solution.c", "solution.cpp", "solution.java",
		"solution.rb", "solution.sh",
	}
	for _, sf := range solutionFiles {
		for _, dir := range []string{workDir, "/app"} {
			path := filepath.Join(dir, sf)
			if info, err := os.Stat(path); err == nil && info.Size() == 0 {
				return fmt.Sprintf("6. WARNING: %s exists but is EMPTY (0 bytes)! Write your solution to it.\n", sf)
			}
		}
	}

	return ""
}

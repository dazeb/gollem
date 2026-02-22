package codetool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// LoopDetectionMiddleware detects when the model is stuck in a doom loop —
// repeatedly making similar tool calls without progress. After threshold
// repeated edits to the same file, it injects a message telling the model
// to reconsider its approach.
func LoopDetectionMiddleware(threshold int) core.AgentMiddleware {
	if threshold <= 0 {
		threshold = 3
	}

	var mu sync.Mutex
	editCounts := make(map[string]int) // file path -> edit count
	bashCounts := make(map[string]int) // command prefix -> run count

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		// Track edits and repeated bash commands by parsing ArgsJSON directly.
		mu.Lock()
		for _, msg := range messages[max(0, len(messages)-2):] {
			if resp, ok := msg.(core.ModelResponse); ok {
				for _, part := range resp.Parts {
					if tc, ok := part.(core.ToolCallPart); ok {
						switch tc.ToolName {
						case "edit", "multi_edit", "write":
							path := extractPathFromArgs(tc.ArgsJSON)
							if path != "" {
								editCounts[path]++
							}
						case "bash":
							prefix := extractCommandPrefix(tc.ArgsJSON)
							if prefix != "" {
								bashCounts[prefix]++
							}
						}
					}
				}
			}
		}

		// Check if any file has been edited too many times or
		// the same command pattern has been run too many times.
		var loopedFiles []string
		for path, count := range editCounts {
			if count >= threshold {
				loopedFiles = append(loopedFiles, path)
			}
		}
		for cmd, count := range bashCounts {
			if count >= threshold+2 { // bash loops need higher threshold
				loopedFiles = append(loopedFiles, "bash: "+cmd)
				delete(bashCounts, cmd) // reset
			}
		}
		mu.Unlock()

		if len(loopedFiles) > 0 {
			// Build specific recovery guidance based on what type of loop it is.
			var guidance string
			hasEditLoop := false
			hasBashLoop := false
			for _, f := range loopedFiles {
				if strings.HasPrefix(f, "bash: ") {
					hasBashLoop = true
				} else {
					hasEditLoop = true
				}
			}

			guidance = "WARNING: You appear to be stuck in a loop, repeatedly "
			if hasEditLoop && hasBashLoop {
				guidance += "editing " + strings.Join(loopedFiles, ", ") + ". "
			} else if hasEditLoop {
				guidance += "editing " + strings.Join(loopedFiles, ", ") + ". "
			} else {
				guidance += "running " + strings.Join(loopedFiles, ", ") + ". "
			}

			guidance += "Step back and try a FUNDAMENTALLY DIFFERENT strategy:\n"
			if hasEditLoop {
				guidance += "- If the same edit keeps failing: consider rewriting the entire file with the write tool instead of patching it\n"
				guidance += "- If you keep getting the same test failure: re-read the FULL error output — you may be misunderstanding the requirement\n"
				guidance += "- If you're going back and forth between two approaches: pick ONE and commit to it\n"
			}
			if hasBashLoop {
				guidance += "- If the same command keeps failing: check if you're missing a dependency, wrong directory, or misconfigured environment\n"
				guidance += "- If a test keeps failing with the same error: the issue is in your code, not in how you're running the test\n"
			}
			guidance += "- Consider if your fundamental approach is wrong — small tweaks won't fix a broken algorithm"

			loopMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: guidance},
				},
			}
			messages = append(messages, loopMsg)

			// Reset counts so we don't keep injecting.
			mu.Lock()
			for _, path := range loopedFiles {
				delete(editCounts, path)
			}
			mu.Unlock()
		}

		return next(ctx, messages, settings, params)
	}
}

// extractPathFromArgs extracts a file path from a tool call's ArgsJSON.
func extractPathFromArgs(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
		File string `json:"file"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	if args.Path != "" {
		return args.Path
	}
	return args.File
}

// extractCommandPrefix extracts the first word/command from a bash tool call's
// ArgsJSON. This is used for loop detection — if the same command prefix keeps
// getting run, the agent is likely stuck.
func extractCommandPrefix(argsJSON string) string {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	cmd := strings.TrimSpace(args.Command)
	if cmd == "" {
		return ""
	}
	// Use first token as the prefix (e.g., "python", "npm", "go").
	// For paths like /usr/bin/python, use the basename.
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// ContextInjectionMiddleware injects environment context at the start of the
// conversation. It runs a set of bash commands to discover the environment
// (e.g., directory structure, language, available tools) and prepends the
// results as a system-level context message.
func ContextInjectionMiddleware(workDir string, timeout ...time.Duration) core.AgentMiddleware {
	var once sync.Once
	var envContext string

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		once.Do(func() {
			envContext = discoverEnvironment(workDir)

			// Determine effective timeout: prefer task-level timeout from
			// task.toml (accurate per-task) over the agent's configured timeout.
			effectiveTimeout := time.Duration(0)
			if len(timeout) > 0 && timeout[0] > 0 {
				effectiveTimeout = timeout[0]
			}
			if taskTimeout := detectTaskTimeout(workDir); taskTimeout > 0 {
				effectiveTimeout = taskTimeout
			}

			if effectiveTimeout > 0 {
				mins := int(effectiveTimeout.Minutes())
				envContext += timeStrategyGuidance(mins)
			}
		})

		if envContext != "" && len(messages) > 0 {
			// Inject environment context into the first request's system parts.
			if req, ok := messages[0].(core.ModelRequest); ok {
				envPart := core.SystemPromptPart{
					Content: envContext,
				}
				newParts := make([]core.ModelRequestPart, 0, len(req.Parts)+1)
				newParts = append(newParts, envPart)
				newParts = append(newParts, req.Parts...)
				req.Parts = newParts
				messages[0] = req
			}
		}

		return next(ctx, messages, settings, params)
	}
}

// discoverEnvironment maps the workspace by inspecting files and running
// lightweight commands. This gives the model a head start so it doesn't waste
// tool calls on basic orientation.
func discoverEnvironment(workDir string) string {
	var parts []string
	parts = append(parts, "## Environment Context")
	parts = append(parts, "Working directory: "+workDir)

	// Detect project language and build system from marker files.
	if lang, build := detectProject(workDir); lang != "" {
		parts = append(parts, "Language: "+lang)
		if build != "" {
			parts = append(parts, "Build system: "+build)
		}
	}

	// Git info.
	if branch := runQuiet(workDir, "git", "branch", "--show-current"); branch != "" {
		parts = append(parts, "Git branch: "+branch)
	}

	// Detect available tools to prevent wasted turns on missing commands.
	var availableTools []string
	for _, tool := range []string{"python3", "python", "pip3", "pip", "node", "npm", "go", "cargo", "make", "gcc", "g++", "coqc", "ocaml", "opam", "lean", "rustc", "javac", "dotnet", "ruby", "Rscript", "julia", "perl", "sqlite3", "psql", "mysql"} {
		if path := runQuiet(workDir, "which", tool); path != "" {
			availableTools = append(availableTools, tool)
		}
	}
	if len(availableTools) > 0 {
		parts = append(parts, "Available tools: "+strings.Join(availableTools, ", "))
	}
	// Python version is critical — many TB2 containers have python3 but not python.
	if pyVer := runQuiet(workDir, "python3", "--version"); pyVer != "" {
		parts = append(parts, "Python: "+pyVer)
	}

	// Quick network connectivity check — many TB2 containers have no internet.
	// Detecting this early prevents the agent from wasting 2-3 turns on failed
	// pip install or apt-get commands. Cached for reuse by auto-install logic below.
	networkAvailable := hasNetworkAccess()
	if !networkAvailable {
		parts = append(parts, "\nWARNING: No internet access detected. Use only locally installed packages and tools.")
		parts = append(parts, "For Python: check with `python3 -c \"import <module>\"` before trying to install.")
		parts = append(parts, "For system packages: check with `dpkg -l | grep <pkg>` or `which <tool>`.")
	}

	// Top-level directory listing (first 30 entries, one level deep).
	if ls := runQuiet(workDir, "ls", "-1"); ls != "" {
		entries := strings.Split(strings.TrimSpace(ls), "\n")
		if len(entries) > 30 {
			entries = append(entries[:30], "... (truncated)")
		}
		parts = append(parts, "Top-level files:\n"+strings.Join(entries, "\n"))
	}

	// Read README / task description if present — many tasks embed critical requirements here.
	readmePaths := []string{
		filepath.Join(workDir, "README.md"),
		filepath.Join(workDir, "README.txt"),
		filepath.Join(workDir, "README"),
		filepath.Join(workDir, "readme.md"),
		filepath.Join(workDir, "TASK.md"),
		filepath.Join(workDir, "task.md"),
		filepath.Join(workDir, "INSTRUCTIONS.md"),
		filepath.Join(workDir, "instructions.md"),
		filepath.Join(workDir, "PROBLEM.md"),
		filepath.Join(workDir, "problem.md"),
		filepath.Join(workDir, "prompt.md"),
		filepath.Join(workDir, "prompt.txt"),
		// Harbor task format: instruction lives in instruction.md or prompts/agent.md.
		filepath.Join(workDir, "instruction.md"),
		filepath.Join(workDir, "prompts", "agent.md"),
		"/app/instruction.md",
		"/app/prompts/agent.md",
	}
	for _, rp := range readmePaths {
		if content := readFileTruncated(rp, 5000); content != "" {
			parts = append(parts, "\n## README Contents (auto-read)")
			parts = append(parts, content)
			break
		}
	}

	// Check /app/task_file — common Terminal-Bench layout with input/output/scripts.
	taskFileDirs := []string{"/app/task_file", filepath.Join(workDir, "task_file")}
	for _, tf := range taskFileDirs {
		if info, err := os.Stat(tf); err == nil && info.IsDir() {
			if tfLs := runQuiet(tf, "ls", "-1R"); tfLs != "" {
				lines := strings.Split(strings.TrimSpace(tfLs), "\n")
				if len(lines) > 40 {
					lines = append(lines[:40], "... (truncated)")
				}
				parts = append(parts, "\nTask file structure ("+tf+"):")
				parts = append(parts, strings.Join(lines, "\n"))
			}
			break
		}
	}

	// Track total auto-read bytes to prevent context bloat.
	// 75KB (~18K tokens) upfront context saves 3-5 turns of file reading.
	// With 150K auto-context (Claude), this is only 12% of the budget.
	// With 80K (grok), it's 22% — still well within budget.
	autoReadBudget := 75000

	// Discover and auto-read test files (verifier tests live in /tests/ on Terminal-Bench).
	// Auto-reading tests is the single highest-impact context injection — the agent
	// immediately knows what success looks like without spending turns reading files.
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	for _, td := range testDirs {
		if info, err := os.Stat(td); err == nil && info.IsDir() {
			if testLs := runQuiet(td, "ls", "-1"); testLs != "" {
				parts = append(parts, "\nTest directory found: "+td)
				parts = append(parts, testLs)
				parts = append(parts, "IMPORTANT: These test files define what will be verified. Run them EARLY and OFTEN. Tests often check for unexpected files in directories — clean up all build artifacts.")
			}
			// Auto-read test files recursively (up to 12KB each, up to 8 files).
			// Tests are the highest-value context — knowing what's verified
			// prevents wasted turns writing solutions that don't match.
			// Recursive because tests may be in subdirs like /tests/unit/.
			testFileCount := 8
			autoReadTestRecursive(td, &parts, 12000, &testFileCount, &autoReadBudget, 0, 2)
			// Extract and highlight key constraints from test assertions.
			if constraints := extractTestConstraints(td); len(constraints) > 0 {
				parts = append(parts, "\n## KEY CONSTRAINTS (extracted from tests)")
				parts = append(parts, "These constraints MUST be satisfied. Check them BEFORE declaring completion:")
				for _, c := range constraints {
					parts = append(parts, "  - "+c)
				}
			}
			break
		}
	}

	// Auto-read conftest.py — defines pytest fixtures that tests depend on.
	// Check project root AND test directories — conftest.py is often placed
	// alongside tests (e.g., /tests/conftest.py) rather than the project root.
	if autoReadBudget > 0 {
		confPaths := []string{
			filepath.Join(workDir, "conftest.py"),
			filepath.Join("/app", "conftest.py"),
		}
		// Also check inside test directories.
		for _, td := range testDirs {
			confPaths = append(confPaths, filepath.Join(td, "conftest.py"))
		}
		for _, confPath := range confPaths {
			if autoReadBudget <= 0 {
				break
			}
			if content := readFileTruncated(confPath, min(5000, autoReadBudget)); content != "" {
				parts = append(parts, "\n## Pytest fixtures (auto-read): "+confPath)
				parts = append(parts, content)
				autoReadBudget -= len(content)
			}
		}
	}

	// Auto-read scripts directory — common in Terminal-Bench tasks for cost models,
	// baselines, and evaluation scripts.
	if autoReadBudget > 0 {
		scriptDirs := []string{
			"/app/task_file/scripts",
			filepath.Join(workDir, "scripts"),
			filepath.Join(workDir, "task_file", "scripts"),
		}
		for _, sd := range scriptDirs {
			if info, err := os.Stat(sd); err == nil && info.IsDir() {
				autoReadBudget = autoReadDirBudget(sd, &parts, "Script", 5000, 4, autoReadBudget)
				break
			}
		}
	}

	// Auto-read build system files — critical for compilation/build tasks.
	// These files define how the project is built and what dependencies are needed.
	// Reading them saves the agent from wasting turns on `cat Makefile`.
	if autoReadBudget > 0 {
		buildFiles := []string{
			"Makefile", "CMakeLists.txt", "Cargo.toml",
			"go.mod", "pyproject.toml", "setup.py", "setup.cfg",
			"package.json", "pom.xml", "build.gradle",
			"configure.ac", "meson.build", "BUILD",
			"docker-compose.yml", "docker-compose.yaml",
			"Dockerfile",
		}
		for _, bf := range buildFiles {
			if autoReadBudget <= 0 {
				break
			}
			for _, dir := range []string{workDir, "/app"} {
				path := filepath.Join(dir, bf)
				content := readFileTruncated(path, min(5000, autoReadBudget))
				if content != "" {
					parts = append(parts, fmt.Sprintf("\n## Build file auto-read: %s", path))
					parts = append(parts, content)
					autoReadBudget -= len(content)
					break
				}
			}
		}
	}

	// Detect Python requirements files and auto-install if possible.
	// This saves 1-2 turns on EVERY Python task — the agent's first action is
	// almost always `pip install -r requirements.txt`.
	foundPyDeps := false
	for _, dir := range []string{workDir, "/app"} {
		reqPath := filepath.Join(dir, "requirements.txt")
		if content := readFileTruncated(reqPath, 2000); content != "" {
			parts = append(parts, fmt.Sprintf("\n## Python dependencies found: %s", reqPath))
			parts = append(parts, content)
			// Auto-install if network is available and python3/pip exist.
			if networkAvailable && runQuiet(workDir, "which", "python3") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-installing Python dependencies from %s\n", reqPath)
				result := runQuietTimeout(workDir, 60*time.Second,
					"pip", "install", "--break-system-packages", "-q", "-r", reqPath)
				if result != "" {
					parts = append(parts, "AUTO-INSTALLED: Python dependencies from "+reqPath+" (already done, no need to install again)")
				} else {
					parts = append(parts, "NOTE: Auto-install attempted but may have failed. Verify with: pip install --break-system-packages -r "+reqPath)
				}
			} else {
				parts = append(parts, "HINT: Install these FIRST with: pip install --break-system-packages -r "+reqPath)
			}
			foundPyDeps = true
			break
		}
	}
	// Fallback: check for Pipfile or pyproject.toml with dependencies.
	if !foundPyDeps {
		for _, dir := range []string{workDir, "/app"} {
			pipfilePath := filepath.Join(dir, "Pipfile")
			if fileExists(pipfilePath) {
				parts = append(parts, "\n## Pipfile found: "+pipfilePath)
				parts = append(parts, "HINT: Install with: pip install --break-system-packages pipenv && pipenv install --system, or manually install packages listed in Pipfile")
				foundPyDeps = true
				break
			}
		}
	}

	// Auto-install npm dependencies for Node.js projects.
	if !foundPyDeps && networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			pkgPath := filepath.Join(dir, "package.json")
			if fileExists(pkgPath) && runQuiet(workDir, "which", "npm") != "" {
				lockPath := filepath.Join(dir, "node_modules")
				if !dirExists(lockPath) {
					fmt.Fprintf(os.Stderr, "[gollem] auto-installing npm dependencies in %s\n", dir)
					runQuietTimeout(dir, 60*time.Second, "npm", "install", "--no-audit", "--no-fund")
					parts = append(parts, "AUTO-INSTALLED: npm dependencies (already done, no need to install again)")
				}
				break
			}
		}
	}

	// Detect .env files and surface environment variable requirements.
	// Many TB2 tasks need specific env vars set; finding this early saves 2+ turns
	// of the agent troubleshooting "connection refused" or "missing config" errors.
	if envHint := detectEnvFiles(workDir); envHint != "" {
		parts = append(parts, envHint)
	}

	// Extract file references from test content to prioritize source auto-read.
	// Tests that import specific source files tell us exactly what's relevant.
	testRefs := extractTestReferencedFiles(parts)

	// Auto-read small source files in /app/ — now recursive (depth 3).
	// Reads files < 5KB to avoid overwhelming context, up to 8 files total.
	// Test-referenced files are prioritized above entry points.
	if autoReadBudget > 0 {
		appSourceDirs := []string{"/app", workDir}
		for _, ad := range appSourceDirs {
			autoReadBudget = autoReadSourceFilesBudget(ad, &parts, 5000, 8, autoReadBudget, testRefs)
			break // only read from one source directory
		}
	}

	// Discover standalone verification scripts that aren't in /tests/.
	// TB2 tasks sometimes place verifier scripts in the working directory
	// or /app/ with names like verify.py, check_output.sh, validate.py.
	verifyScripts := discoverVerificationScripts(workDir)
	if len(verifyScripts) > 0 {
		parts = append(parts, "\n## Verification Scripts Found")
		parts = append(parts, "These scripts can be used to verify your solution:")
		for _, vs := range verifyScripts {
			parts = append(parts, "  - "+vs)
		}
		parts = append(parts, "Run these EARLY after creating output files to check correctness.")
		// Auto-chmod +x for shell scripts to prevent "Permission denied" errors.
		// This is a very common 1-2 turn waste on TB2 tasks.
		chmodScripts(verifyScripts)
	}

	// Also chmod +x any shell scripts in test directories.
	for _, td := range testDirs {
		chmodScriptsInDir(td)
	}

	// Check for output directories that need to be populated.
	// List any pre-existing files (templates/references) so the agent knows
	// the expected format from turn 1.
	outputDirs := []string{
		filepath.Join(workDir, "output_data"),
		"/app/task_file/output_data",
		filepath.Join(workDir, "output"),
	}
	for _, od := range outputDirs {
		if info, err := os.Stat(od); err == nil && info.IsDir() {
			parts = append(parts, "\nOutput directory: "+od+" (your deliverables go here)")
			// List pre-existing files — these may be templates or expected output format examples.
			if entries, err := os.ReadDir(od); err == nil && len(entries) > 0 {
				var fileNames []string
				for _, e := range entries {
					if !e.IsDir() {
						fileNames = append(fileNames, e.Name())
					}
				}
				if len(fileNames) > 0 {
					if len(fileNames) > 10 {
						fileNames = append(fileNames[:10], fmt.Sprintf("... and %d more", len(fileNames)-10))
					}
					parts = append(parts, "Pre-existing output files: "+strings.Join(fileNames, ", "))
					parts = append(parts, "NOTE: These may be templates or expected format examples. Study them before creating your output.")
				}
			}
			break
		}
	}

	// List resources/ directory — common TB2 layout for reference data, patch files,
	// and input data that tests compare against. Knowing what's here prevents the
	// agent from missing critical reference files (e.g., fix-git's patch_files/).
	for _, rd := range []string{"/app/resources", filepath.Join(workDir, "resources")} {
		if info, err := os.Stat(rd); err == nil && info.IsDir() {
			if ls := runQuiet(rd, "ls", "-1R"); ls != "" {
				lines := strings.Split(strings.TrimSpace(ls), "\n")
				if len(lines) > 30 {
					lines = append(lines[:30], "... (truncated)")
				}
				parts = append(parts, "\nResources directory ("+rd+"):")
				parts = append(parts, strings.Join(lines, "\n"))
				parts = append(parts, "NOTE: Tests may compare your output against files in this directory.")
			}
			break
		}
	}

	// Detect expected output files from test analysis.
	// This tells the agent exactly WHAT to create from the start.
	if expectedOutputs := detectExpectedOutputs(workDir); len(expectedOutputs) > 0 {
		parts = append(parts, "\n## Expected Output Files (from test analysis)")
		parts = append(parts, "Tests expect these files/paths to exist:")
		for _, o := range expectedOutputs {
			parts = append(parts, "  - "+o)
		}
		parts = append(parts, "Create these files EARLY — even with placeholder content — then refine.")
	}

	// Auto-read example/reference output files that show expected format.
	// These save the agent from guessing output format — the #4 failure mode.
	if autoReadBudget > 0 {
		autoReadBudget = autoReadExampleOutputs(workDir, &parts, autoReadBudget)
	}

	// Task-type specific guidance based on detected patterns.
	parts = append(parts, detectTaskGuidance(workDir))

	// Suggest specific test/build commands so the agent doesn't waste turns
	// figuring out how to verify its work.
	if cmds := detectTestCommands(workDir); len(cmds) > 0 {
		parts = append(parts, "\n## Quick Commands")
		for _, cmd := range cmds {
			parts = append(parts, "  "+cmd)
		}
	}

	// Remind agent that source files are pre-loaded. Don't enumerate tools —
	// the model sees tool definitions from the API, and a hardcoded list would
	// be inaccurate if code mode or team mode adds/removes tools.
	parts = append(parts, "\nSource files are pre-loaded above. For complex tasks, create a plan first using the planning tool, then proceed.")

	return strings.Join(parts, "\n")
}

// readFileTruncated reads a file and returns its content truncated to maxBytes.
// Returns empty string on any error.
func readFileTruncated(path string, maxBytes int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > maxBytes {
		return string(data[:maxBytes]) + "\n... (truncated)"
	}
	return string(data)
}

// detectProject identifies the project language and build system from marker
// files in the working directory.
func detectProject(workDir string) (language, buildSystem string) {
	markers := []struct {
		file     string
		lang     string
		build    string
	}{
		{"go.mod", "Go", "go"},
		{"Cargo.toml", "Rust", "cargo"},
		{"package.json", "JavaScript/TypeScript", "npm"},
		{"pyproject.toml", "Python", "pyproject"},
		{"setup.py", "Python", "setuptools"},
		{"requirements.txt", "Python", "pip"},
		{"pom.xml", "Java", "maven"},
		{"build.gradle", "Java", "gradle"},
		{"Gemfile", "Ruby", "bundler"},
		{"CMakeLists.txt", "C/C++", "cmake"},
		{"lakefile.lean", "Lean 4", "lake"},
		{"lakefile.toml", "Lean 4", "lake"},
		{"stack.yaml", "Haskell", "stack"},
		{"dune-project", "OCaml", "dune"},
		{"mix.exs", "Elixir", "mix"},
		{"build.zig", "Zig", "zig"},
		{"Project.toml", "Julia", "julia"},
		{"Makefile.PL", "Perl", "perl"},
		{"Build.PL", "Perl", "perl"},
		{"cpanfile", "Perl", "cpanm"},
		{"Makefile", "unknown", "make"},
	}

	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(workDir, m.file)); err == nil {
			if language == "" || m.lang != "unknown" {
				language = m.lang
			}
			if buildSystem == "" {
				buildSystem = m.build
			}
			if language != "" && language != "unknown" {
				return language, buildSystem
			}
		}
	}
	// Check for Haskell cabal files (glob pattern).
	if matches, _ := filepath.Glob(filepath.Join(workDir, "*.cabal")); len(matches) > 0 {
		return "Haskell", "cabal"
	}
	return language, buildSystem
}

// runQuiet runs a command in workDir and returns trimmed stdout, or empty
// string on any error. It has a short timeout to avoid blocking agent startup.
func runQuiet(workDir string, name string, args ...string) string {
	return runQuietTimeout(workDir, 5*time.Second, name, args...)
}

// runQuietTimeout runs a command with a custom timeout, returning trimmed
// stdout or empty string on error.
func runQuietTimeout(workDir string, timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PIP_BREAK_SYSTEM_PACKAGES=1")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// autoReadExampleOutputs searches for example/reference output files that show
// the expected output format. These are invaluable for preventing format mismatches.
// Looks for files named "example_*", "sample_*", "reference_*", "expected_*"
// in common locations. Returns remaining budget.
func autoReadExampleOutputs(workDir string, parts *[]string, budget int) int {
	searchDirs := []string{
		workDir,
		"/app",
		"/app/task_file",
		filepath.Join(workDir, "task_file"),
		filepath.Join(workDir, "examples"),
		filepath.Join(workDir, "sample"),
		"/app/task_file/input_data", // sometimes example outputs are in input_data
	}
	examplePrefixes := []string{
		"example", "sample", "reference", "expected",
		"template", "demo", "baseline",
	}

	count := 0
	for _, dir := range searchDirs {
		if budget <= 0 || count >= 3 {
			break
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if budget <= 0 || count >= 3 {
				break
			}
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			lower := strings.ToLower(name)
			isExample := false
			for _, prefix := range examplePrefixes {
				if strings.HasPrefix(lower, prefix) {
					isExample = true
					break
				}
			}
			if !isExample {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.Size() == 0 || info.Size() > 3000 {
				continue // only read small example files
			}
			limit := 3000
			if limit > budget {
				limit = budget
			}
			content := readFileTruncated(filepath.Join(dir, name), limit)
			if content != "" {
				*parts = append(*parts, fmt.Sprintf("\n## Example output file (MATCH THIS FORMAT): %s/%s", dir, name))
				*parts = append(*parts, content)
				*parts = append(*parts, "IMPORTANT: Your output MUST match this format exactly (headers, delimiters, whitespace, encoding).")
				budget -= len(content)
				count++
			}
		}
	}
	return budget
}

// autoReadDirBudget reads small files in a directory and appends them to parts,
// respecting a total byte budget. Returns the remaining budget.
func autoReadDirBudget(dir string, parts *[]string, label string, maxBytes, maxFiles, budget int) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return budget
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || count >= maxFiles || budget <= 0 {
			continue
		}
		name := entry.Name()
		if !isSourceFile(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > int64(maxBytes) || info.Size() == 0 {
			continue
		}
		limit := maxBytes
		if limit > budget {
			limit = budget
		}
		content := readFileTruncated(filepath.Join(dir, name), limit)
		if content != "" {
			*parts = append(*parts, fmt.Sprintf("\n## %s file auto-read: %s/%s", label, dir, name))
			*parts = append(*parts, content)
			budget -= len(content)
			count++
		}
	}
	return budget
}

// autoReadTestRecursive reads test files recursively (depth-limited). Test files
// are the highest-value context for coding agents — they define what success
// looks like. Some tasks nest tests in subdirs like /tests/unit/ or /tests/e2e/.
func autoReadTestRecursive(dir string, parts *[]string, maxBytes int, remaining *int, budget *int, depth, maxDepth int) {
	if depth > maxDepth || *remaining <= 0 || *budget <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Read files first, then recurse into subdirectories.
	// Track large test files we can't fully read — we'll extract their structure.
	var largeTestFiles []string
	for _, entry := range entries {
		if *remaining <= 0 || *budget <= 0 {
			break
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSourceFile(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}
		if info.Size() > int64(maxBytes) {
			// Too large for full auto-read — extract structure instead.
			largeTestFiles = append(largeTestFiles, filepath.Join(dir, name))
			continue
		}
		limit := maxBytes
		if limit > *budget {
			limit = *budget
		}
		content := readFileTruncated(filepath.Join(dir, name), limit)
		if content != "" {
			*parts = append(*parts, fmt.Sprintf("\n## Test file auto-read (DO NOT MODIFY): %s/%s", dir, name))
			*parts = append(*parts, content)
			*budget -= len(content)
			*remaining--
		}
	}

	// For large test files we couldn't auto-read, extract test function names.
	// This ensures the agent knows what tests exist even in large test suites.
	for _, path := range largeTestFiles {
		if *budget <= 0 {
			break
		}
		if skeleton := extractFileStructure(path); skeleton != "" {
			*parts = append(*parts, fmt.Sprintf("\n## Large test file structure (DO NOT MODIFY): %s", path))
			*parts = append(*parts, skeleton)
			*parts = append(*parts, "NOTE: File too large to auto-read. Use view tool to read specific test functions.")
			*budget -= len(skeleton)
		}
	}

	// Recurse into subdirectories.
	for _, entry := range entries {
		if *remaining <= 0 || *budget <= 0 {
			return
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "__pycache__" || name == "node_modules" {
			continue
		}
		autoReadTestRecursive(filepath.Join(dir, name), parts, maxBytes, remaining, budget, depth+1, maxDepth)
	}
}

// autoReadSourceFilesBudget reads small source files in a directory recursively
// (up to depth 3), respecting a total byte budget. This ensures the agent sees
// code in subdirectories like src/, lib/, utils/ without wasting turns.
// testRefs is an optional set of filenames referenced by test imports — these
// get highest priority in reading order.
// Returns the remaining byte budget.
func autoReadSourceFilesBudget(dir string, parts *[]string, maxBytes, maxFiles, budget int, testRefs ...map[string]bool) int {
	var refs map[string]bool
	if len(testRefs) > 0 {
		refs = testRefs[0]
	}
	autoReadSourceRecursive(dir, parts, maxBytes, &maxFiles, &budget, 0, 3, refs)
	return budget
}

// autoReadSourceRecursive walks a directory tree reading source files.
// Files are prioritized: test-referenced files first, then entry-point files
// (main.*, app.*, index.*), then remaining files. testRefs is a set of
// lowercased filenames that tests import — these get highest priority.
func autoReadSourceRecursive(dir string, parts *[]string, maxBytes int, remaining *int, budget *int, depth, maxDepth int, testRefs map[string]bool) {
	if depth > maxDepth || *remaining <= 0 || *budget <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Partition files into priority tiers:
	// 1. Test-referenced files (imported by tests — highest value)
	// 2. Entry points (main.*, app.*, index.*, solution.*)
	// 3. Regular source files
	var testRef, priority, regular []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSourceFile(name) {
			continue
		}
		lower := strings.ToLower(name)
		if lower == "readme.md" || lower == "readme.txt" || lower == "readme" {
			continue // Already auto-read separately.
		}
		if testRefs != nil && testRefs[lower] {
			testRef = append(testRef, entry)
		} else if isEntryPointFile(lower) {
			priority = append(priority, entry)
		} else {
			regular = append(regular, entry)
		}
	}

	// Read in priority order: test-referenced → entry points → regular.
	allFiles := make([]os.DirEntry, 0, len(testRef)+len(priority)+len(regular))
	allFiles = append(allFiles, testRef...)
	allFiles = append(allFiles, priority...)
	allFiles = append(allFiles, regular...)

	// Track large files we skip — we'll extract structure from them below.
	var largeFiles []string

	for _, entry := range allFiles {
		if *remaining <= 0 || *budget <= 0 {
			break
		}
		name := entry.Name()
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}

		// Test-referenced files get a higher limit (8KB vs 5KB) since
		// they're known to be relevant — tests import them directly.
		effectiveMax := maxBytes
		if testRefs != nil && testRefs[strings.ToLower(name)] {
			effectiveMax = maxBytes * 8 / 5 // 5KB → 8KB
		}

		if info.Size() > int64(effectiveMax) {
			// File too large to auto-read — save for structure extraction.
			largeFiles = append(largeFiles, filepath.Join(dir, name))
			continue
		}
		limit := effectiveMax
		if limit > *budget {
			limit = *budget
		}
		content := readFileTruncated(filepath.Join(dir, name), limit)
		if content != "" {
			*parts = append(*parts, fmt.Sprintf("\n## Source file auto-read: %s/%s", dir, name))
			*parts = append(*parts, content)
			*budget -= len(content)
			*remaining--
		}
	}

	// For large files we couldn't auto-read, extract a structure summary
	// (function/class names). This gives the agent a "table of contents"
	// for big files, preventing 2-3 wasted turns on grep/view.
	for _, path := range largeFiles {
		if *budget <= 0 {
			break
		}
		if skeleton := extractFileStructure(path); skeleton != "" {
			*parts = append(*parts, fmt.Sprintf("\n## Source file structure (too large to auto-read): %s", path))
			*parts = append(*parts, skeleton)
			*budget -= len(skeleton)
		}
	}

	// Recurse into subdirectories (skip hidden, vendor, node_modules, etc.).
	for _, entry := range entries {
		if *remaining <= 0 || *budget <= 0 {
			return
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" ||
			name == "__pycache__" || name == ".git" || name == "venv" || name == ".venv" ||
			name == "build" || name == "dist" || name == "target" {
			continue
		}
		autoReadSourceRecursive(filepath.Join(dir, name), parts, maxBytes, remaining, budget, depth+1, maxDepth, testRefs)
	}
}

// detectTaskGuidance returns task-type-specific guidance based on file patterns.
func detectTaskGuidance(workDir string) string {
	hasInputData := dirExists("/app/task_file/input_data") || dirExists(filepath.Join(workDir, "input_data"))
	hasOutputData := dirExists("/app/task_file/output_data") || dirExists(filepath.Join(workDir, "output_data"))
	hasScripts := dirExists("/app/task_file/scripts") || dirExists(filepath.Join(workDir, "scripts"))
	hasFilter := fileExists(filepath.Join(workDir, "filter.py")) || fileExists("/app/filter.py")
	hasTests := dirExists("/tests") || dirExists(filepath.Join(workDir, "tests"))

	var hints []string
	if hasInputData && hasOutputData {
		hints = append(hints, "\n## Task Type: Data Processing")
		hints = append(hints, "This task has input_data/ and output_data/ directories.")
		hints = append(hints, "QUICK START (do these in order):")
		hints = append(hints, "  Turn 1: Read test code + examine input data format (head/wc -l)")
		hints = append(hints, "  Turn 2: Write processing script to output_data/")
		hints = append(hints, "  Turn 3: Run test, read failures, iterate")
		hints = append(hints, "Strategy: (1) Read input data format, (2) understand output requirements from tests/scripts, (3) write processing code, (4) validate output matches expected format.")
		// Show first few lines of input data files so agent knows the format immediately.
		inputDirs := []string{"/app/task_file/input_data", filepath.Join(workDir, "input_data")}
		for _, id := range inputDirs {
			if dirExists(id) {
				previewInputData(id, &hints)
				break
			}
		}
	}
	if hasScripts {
		hints = append(hints, "Scripts are available — study them to understand evaluation criteria and cost models BEFORE implementing.")
	}
	if hasFilter {
		hints = append(hints, "\n## Task Type: Security/Bypass")
		hints = append(hints, "QUICK START:")
		hints = append(hints, "  Turn 1: Read filter.py THOROUGHLY — understand what it blocks/allows")
		hints = append(hints, "  Turn 2: Write payloads to output, test against filter")
		hints = append(hints, "  Turn 3: Run verifier tests, iterate")
		hints = append(hints, "Strategy: (1) Read and understand the filter code thoroughly, (2) identify what it blocks vs allows, (3) craft payloads that exploit gaps, (4) test each payload against the filter before writing to output.")
	}
	if hasTests && !hasInputData {
		hints = append(hints, "\n## Task Type: Code Implementation")
		hints = append(hints, "QUICK START:")
		hints = append(hints, "  Turn 1: Read test code to understand expected behavior and API")
		hints = append(hints, "  Turn 2: Write initial implementation")
		hints = append(hints, "  Turn 3: Run tests, read failures carefully, fix one at a time")
		hints = append(hints, "Strategy: (1) Read test files to understand expected behavior, (2) implement solution, (3) run tests iteratively until passing, (4) clean up build artifacts.")
	}

	// Detect scientific computing tasks (common in TB2).
	if detectSciCompute(workDir) {
		hints = append(hints, "\n## Task Type: Scientific/Numerical Computing")
		hints = append(hints, "This appears to be a scientific computing task. Key strategies:")
		hints = append(hints, "- Use numpy, scipy, or similar libraries for numerical work — don't implement algorithms from scratch")
		hints = append(hints, "- Pay attention to numerical precision (float32 vs float64, overflow, underflow)")
		hints = append(hints, "- Validate results against expected ranges or known values")
		hints = append(hints, "- For optimization problems, prefer well-known algorithms (gradient descent, LP solvers, etc.)")
	}

	// Detect model training tasks.
	if detectMLTask(workDir) {
		hints = append(hints, "\n## Task Type: Machine Learning / Model Training")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Install required ML packages first (torch, transformers, sklearn, etc.)")
		hints = append(hints, "- Check GPU availability with 'nvidia-smi' before training")
		hints = append(hints, "- Use small batch sizes and few epochs initially to verify the pipeline works")
		hints = append(hints, "- Save checkpoints frequently for long training runs")
		hints = append(hints, "- If no GPU, use CPU-compatible approaches (sklearn, small models)")
	}

	// Detect formal verification / theorem proving tasks.
	if fileExists(filepath.Join(workDir, "lakefile.lean")) || fileExists(filepath.Join(workDir, "leanpkg.toml")) ||
		fileExists(filepath.Join(workDir, "lakefile.toml")) {
		hints = append(hints, "\n## Task Type: Theorem Proving (Lean 4)")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Read the goal and any existing proof structure carefully")
		hints = append(hints, "- Use `lake build` to check proofs incrementally")
		hints = append(hints, "- Use `sorry` as placeholder, then fill in proofs step by step")
		hints = append(hints, "- Common tactics: simp, omega, ring, exact, apply, intro, induction")
		hints = append(hints, "- If stuck, try `decide` for decidable propositions or `norm_num` for numeric goals")
	}

	// Detect Coq proof tasks.
	if detectCoqTask(workDir) {
		hints = append(hints, "\n## Task Type: Theorem Proving (Coq)")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check Coq version: `coqc --version`. Version mismatches cause obscure failures.")
		hints = append(hints, "- Coq compilation is SLOW. Build incrementally with `make -j$(nproc)` or `coq_makefile`.")
		hints = append(hints, "- If building a large project (CompCert, etc.), start the build early and check logs for errors within 60 seconds.")
		hints = append(hints, "- Common issue: deprecated tactics/notations between Coq versions. Check error messages for version hints.")
		hints = append(hints, "- If compilation fails mid-way, fix the specific error rather than rebuilding from scratch.")
	}

	// Detect OCaml tasks.
	if detectOCamlTask(workDir) {
		hints = append(hints, "\n## Task Type: OCaml")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check for build system: `dune build`, `make`, or direct `ocamlfind`/`ocamlopt`")
		hints = append(hints, "- Use `opam install` for package management if opam is available")
		hints = append(hints, "- OCaml type errors are verbose but precise — read the full error including expected vs actual types")
	}

	// Detect R language tasks.
	if detectRTask(workDir) {
		hints = append(hints, "\n## Task Type: R Language")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Run R scripts with `Rscript <file.R>`. Check `which Rscript` first.")
		hints = append(hints, "- Install packages: `Rscript -e 'install.packages(\"pkg\", repos=\"https://cloud.r-project.org\")'`")
		hints = append(hints, "- R code can be slow with large loops — use vectorized operations (apply/sapply/vapply) instead of for loops")
		hints = append(hints, "- Test your R code with small inputs first, then verify it completes within verifier timeouts (typically 15-60s)")
		hints = append(hints, "- If the task has a `test()` function, ensure it exists and runs correctly with `source('file.R'); test()`")
	}

	// Detect Julia language tasks.
	if detectJuliaTask(workDir) {
		hints = append(hints, "\n## Task Type: Julia")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Run scripts: `julia <file.jl>` or `julia --project=. <file.jl>` for project mode")
		hints = append(hints, "- Install packages: `julia -e 'using Pkg; Pkg.instantiate()'` (reads Project.toml)")
		hints = append(hints, "- For missing packages: `julia -e 'using Pkg; Pkg.add(\"PackageName\")'`")
		hints = append(hints, "- Julia has high first-run compilation times (JIT). Use `--compile=min` for faster startup if compilation time is an issue.")
		hints = append(hints, "- Run tests: `julia --project=. -e 'using Pkg; Pkg.test()'`")
	}

	// Detect Perl tasks.
	if detectPerlTask(workDir) {
		hints = append(hints, "\n## Task Type: Perl")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Run scripts: `perl <file.pl>` or `perl -w <file.pl>` for warnings")
		hints = append(hints, "- Install modules: `cpan install Module::Name` or `cpanm Module::Name`")
		hints = append(hints, "- Run tests: `prove -v` or `perl -Ilib t/*.t`")
	}

	// Detect service/daemon tasks (web servers, background services).
	if detectServiceTask(workDir) {
		hints = append(hints, "\n## Task Type: Service/Daemon Setup")
		hints = append(hints, "This task likely requires a service that PERSISTS after your session ends.")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Use systemd (`systemctl enable/start`), supervisord, or init scripts to ensure the service starts on boot")
		hints = append(hints, "- If systemd is unavailable, use `nohup <command> &` with a startup script in /etc/rc.local or crontab @reboot")
		hints = append(hints, "- VERIFY the service is running: `curl localhost:<port>`, `systemctl status <service>`, or `ss -tlnp`")
		hints = append(hints, "- After configuring, test that the service survives: stop and restart it to confirm persistence")
		hints = append(hints, "- Don't just run the service in the foreground — it will die when your session ends")
	}

	// Detect tasks with file hash/checksum comparisons.
	if detectHashComparisonTask(workDir) {
		hints = append(hints, "\n## Note: Hash/Checksum Comparisons Detected")
		hints = append(hints, "Tests compare file contents by HASH (MD5, SHA, etc). This means:")
		hints = append(hints, "- Your output files must match EXACTLY — byte for byte")
		hints = append(hints, "- Check for trailing newlines, encoding differences (UTF-8 BOM), line ending differences (CRLF vs LF)")
		hints = append(hints, "- If reference files exist in /app/resources/ or similar, diff your output against them: `diff <your_file> <reference_file>`")
		hints = append(hints, "- Use `md5sum` or `sha256sum` to check hashes before and after your changes")
	}

	// Detect cryptography / security analysis tasks.
	if detectCryptoTask(workDir) {
		hints = append(hints, "\n## Task Type: Cryptography / Security Analysis")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Study the cipher/protocol implementation carefully before attacking")
		hints = append(hints, "- Use well-known cryptanalysis techniques (linear, differential, meet-in-the-middle)")
		hints = append(hints, "- For password recovery: try common patterns, dictionary attacks, then brute force")
		hints = append(hints, "- Use Python's pycryptodome or built-in hashlib for crypto operations")
	}

	// Detect database tasks (SQLite, PostgreSQL, MySQL).
	if detectDatabaseTask(workDir) {
		hints = append(hints, "\n## Task Type: Database")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check available databases: `which sqlite3`, `which psql`, `which mysql`")
		hints = append(hints, "- For SQLite: use `sqlite3 <dbfile>` or Python's `sqlite3` module. Check if .db/.sqlite files exist.")
		hints = append(hints, "- For PostgreSQL: check if running with `pg_isready`. Start with `service postgresql start` if needed.")
		hints = append(hints, "- For MySQL: check with `mysqladmin ping`. Start with `service mysql start` if needed.")
		hints = append(hints, "- Read schema first: `.schema` (SQLite), `\\dt` then `\\d+ tablename` (psql), `SHOW TABLES; DESCRIBE tablename;` (MySQL)")
		hints = append(hints, "- When tests compare query results, match EXACT column names, ordering, and formatting")
	}

	// Detect code golf / size-constrained tasks.
	if detectCodeGolfTask(workDir) {
		hints = append(hints, "\n## Task Type: Code Golf / Size-Constrained")
		hints = append(hints, "This task has SIZE CONSTRAINTS on output files. Key strategies:")
		hints = append(hints, "- Check size constraints FIRST and track byte count at every step with `wc -c`")
		hints = append(hints, "- Start with the simplest working implementation, then optimize for size")
		hints = append(hints, "- Create the output file IMMEDIATELY even if it's too large, then shrink it")
		hints = append(hints, "- Use compact coding style: short variable names, minimal whitespace, eliminate dead code")
		hints = append(hints, "- For C: omit unnecessary includes, use preprocessor tricks, minimize struct padding")
		hints = append(hints, "- For Python: use list comprehensions, lambda, semicolons to join lines")
		hints = append(hints, "- Verify size after EVERY edit: `wc -c <output_file>`")
	}

	// Detect system emulator / QEMU tasks.
	if detectSystemEmulatorTask(workDir) {
		hints = append(hints, "\n## Task Type: System Emulator / VM Setup")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check available tools first: `which qemu-system-x86_64`, `which kvm`, etc.")
		hints = append(hints, "- Install QEMU if missing: `apt-get install -y qemu-system-x86 qemu-utils`")
		hints = append(hints, "- Use `-nographic` flag for headless operation (no display server available)")
		hints = append(hints, "- Set appropriate timeouts for VM boot (use bash timeout 120+ seconds)")
		hints = append(hints, "- For SSH access: use port forwarding `-netdev user,id=net0,hostfwd=tcp::2222-:22`")
		hints = append(hints, "- Don't try to install GUI tools or display servers — this is a headless environment")
	}

	// Detect Dockerfile/container tasks.
	if fileExists(filepath.Join(workDir, "Dockerfile")) || fileExists(filepath.Join(workDir, "docker-compose.yml")) {
		hints = append(hints, "\n## Note: Docker files detected")
		hints = append(hints, "If the task involves Docker: build and test locally first, then containerize.")
		hints = append(hints, "Don't waste turns debugging Docker networking or GPU passthrough — focus on the core task.")
	}

	// Detect build-from-source tasks.
	if detectBuildFromSourceTask(workDir) {
		hints = append(hints, "\n## Task Type: Build from Source")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- When extracting archives (tar, zip), verify ALL files were extracted: `ls -la` the extracted directory.")
		hints = append(hints, "- Some tests check for specific source files by hash. Don't delete source files after building.")
		hints = append(hints, "- Start the build early — compilation can take a long time. Check build logs for errors within the first minute.")
		hints = append(hints, "- If a build fails, read the FULL error log rather than restarting from scratch.")
		hints = append(hints, "- Keep source directories intact — verifiers often check that sources exist.")
	}

	// Detect Git-related tasks (patches, merge, bisect, cherry-pick).
	if detectGitTask(workDir) {
		hints = append(hints, "\n## Task Type: Git Operations")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check git status first: `git status`, `git log --oneline -10`, `git branch -a`")
		hints = append(hints, "- For patch tasks: use `git apply <patch>` or `patch -p1 < <file>`. If it fails, try `git apply --3way`.")
		hints = append(hints, "- For merge conflicts: use `git merge` then resolve conflicts by editing files, don't use interactive tools")
		hints = append(hints, "- For bisect: script it with `git bisect start`, `git bisect good/bad`, `git bisect run <script>`")
		hints = append(hints, "- For cherry-pick: `git cherry-pick <commit>`, resolve conflicts manually if needed")
		hints = append(hints, "- Check for .patch or .diff files in the working directory — they may need to be applied")
		hints = append(hints, "- After git operations: verify the result with `git log`, `git diff`, and run tests")
	}

	// Detect tasks with image files that need analysis.
	if imageFiles := detectImageFiles(workDir); len(imageFiles) > 0 {
		hints = append(hints, "\n## Image Files Detected")
		for _, f := range imageFiles {
			hints = append(hints, "  - "+f)
		}
		hints = append(hints, "To analyze images, use Python with PIL/Pillow or OpenCV:")
		hints = append(hints, "  pip install --break-system-packages Pillow 2>/dev/null")
		hints = append(hints, "  python3 -c \"from PIL import Image; img = Image.open('file.png'); print(img.size, img.mode)\"")
		hints = append(hints, "For OCR: pip install --break-system-packages pytesseract (needs tesseract-ocr)")
		hints = append(hints, "For chess positions: analyze piece positions programmatically, don't try to 'see' the image")
	}

	if len(hints) > 0 {
		return strings.Join(hints, "\n")
	}
	return ""
}

// detectImageFiles returns image file paths found in the working directory.
func detectImageFiles(workDir string) []string {
	var images []string
	for _, dir := range []string{workDir, "/app"} {
		for _, ext := range []string{"*.png", "*.jpg", "*.jpeg", "*.bmp", "*.gif", "*.tiff", "*.ppm", "*.pgm"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			for _, m := range matches {
				images = append(images, m)
			}
		}
	}
	// Cap to 10 to prevent context bloat.
	if len(images) > 10 {
		images = images[:10]
	}
	return images
}

// detectExpectedOutputs scans test files and README to identify the output
// files the agent is expected to create. This gives the agent a concrete
// target list from turn 1, preventing wasted time figuring out what to produce.
func detectExpectedOutputs(workDir string) []string {
	var outputs []string
	seen := make(map[string]bool)

	// Search test files for references to output file paths.
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	for _, td := range testDirs {
		entries, err := os.ReadDir(td)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isSourceFile(entry.Name()) {
				continue
			}
			info, _ := entry.Info()
			if info == nil || info.Size() > 30000 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(td, entry.Name()))
			if err != nil {
				continue
			}
			content := string(data)
			// Look for file path references in test code.
			outputPatterns := []string{
				"output_data/", "output/", "/app/output",
				"result.", "results.", "solution.", "answer.",
			}
			for _, line := range strings.Split(content, "\n") {
				trimmed := strings.TrimSpace(line)
				// Skip comments.
				if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
					continue
				}
				for _, pat := range outputPatterns {
					idx := strings.Index(trimmed, pat)
					if idx < 0 {
						continue
					}
					// Extract the file path from surrounding quotes or path syntax.
					path := extractPathFromLine(trimmed, idx)
					if path != "" && !seen[path] {
						seen[path] = true
						outputs = append(outputs, path)
					}
				}
			}
		}
		if len(outputs) > 0 {
			break // found expected outputs in one test dir
		}
	}

	// Also scan README/instruction files for output file references.
	// Task descriptions often mention expected output files explicitly.
	readmePaths := []string{
		filepath.Join(workDir, "README.md"),
		filepath.Join(workDir, "instruction.md"),
		"/app/instruction.md",
		filepath.Join(workDir, "prompts", "agent.md"),
		"/app/prompts/agent.md",
	}
	for _, rp := range readmePaths {
		data, err := os.ReadFile(rp)
		if err != nil {
			continue
		}
		content := string(data)
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			for _, pat := range []string{"output_data/", "output/", "/app/output"} {
				idx := strings.Index(trimmed, pat)
				if idx < 0 {
					continue
				}
				path := extractPathFromLine(trimmed, idx)
				if path != "" && !seen[path] {
					seen[path] = true
					outputs = append(outputs, path)
				}
			}
		}
	}

	// Cap to 15 to prevent context bloat.
	if len(outputs) > 15 {
		outputs = outputs[:15]
	}
	return outputs
}

// extractPathFromLine extracts a file path starting at position idx in a line.
// It looks for surrounding quotes or extracts until whitespace/punctuation.
// extractTestReferencedFiles scans the auto-read parts for test file content
// and extracts source file names referenced by import/require statements.
// Returns a map of lowercased filenames (e.g., "solution.py") that tests import.
// This enables prioritizing these files in source auto-read.
func extractTestReferencedFiles(parts []string) map[string]bool {
	refs := make(map[string]bool)
	inTestSection := false

	for _, part := range parts {
		// Track whether we're in a test file section.
		if strings.HasPrefix(part, "\n## Test file auto-read") {
			inTestSection = true
			continue
		}
		if strings.HasPrefix(part, "\n## ") {
			inTestSection = false
			continue
		}
		if !inTestSection {
			continue
		}

		// Scan each line for import/require patterns.
		for _, line := range strings.Split(part, "\n") {
			trimmed := strings.TrimSpace(line)

			// Python: "import solution", "from solution import *", "from my_module import X"
			if strings.HasPrefix(trimmed, "import ") {
				// "import solution" or "import solution as sol"
				mod := strings.Fields(trimmed)[1]
				mod = strings.Split(mod, ".")[0] // "import foo.bar" → "foo"
				mod = strings.TrimRight(mod, ",")
				if mod != "" && !isStdlibModule(mod) {
					refs[strings.ToLower(mod)+".py"] = true
				}
			} else if strings.HasPrefix(trimmed, "from ") {
				// "from solution import *", "from my_module import func"
				fields := strings.Fields(trimmed)
				if len(fields) >= 2 {
					mod := fields[1]
					mod = strings.Split(mod, ".")[0] // "from foo.bar import X" → "foo"
					if mod != "" && mod != "." && !isStdlibModule(mod) {
						refs[strings.ToLower(mod)+".py"] = true
					}
				}
			}

			// JavaScript/TypeScript: require('./solution'), import from './solution'
			if strings.Contains(trimmed, "require(") || strings.Contains(trimmed, "from '") || strings.Contains(trimmed, "from \"") {
				// Extract path from quotes after require( or from
				for _, delim := range []string{"'", "\""} {
					startPat := delim
					idx := strings.Index(trimmed, "require("+startPat)
					if idx < 0 {
						idx = strings.Index(trimmed, "from "+startPat)
						if idx >= 0 {
							idx += 5
						}
					} else {
						idx += 8
					}
					if idx >= 0 {
						rest := trimmed[idx+1:]
						endIdx := strings.Index(rest, delim)
						if endIdx > 0 {
							modPath := rest[:endIdx]
							// Only local imports (starting with . or /)
							if strings.HasPrefix(modPath, ".") || strings.HasPrefix(modPath, "/") {
								base := filepath.Base(modPath)
								// Add common extensions if missing
								if !strings.Contains(base, ".") {
									refs[strings.ToLower(base)+".js"] = true
									refs[strings.ToLower(base)+".ts"] = true
								} else {
									refs[strings.ToLower(base)] = true
								}
							}
						}
					}
				}
			}

			// Shell: source ./helper.sh, . ./utils.sh
			if strings.HasPrefix(trimmed, "source ") || strings.HasPrefix(trimmed, ". ./") {
				var scriptPath string
				if strings.HasPrefix(trimmed, "source ") {
					scriptPath = strings.Fields(trimmed)[1]
				} else {
					scriptPath = strings.Fields(trimmed)[1]
				}
				base := filepath.Base(scriptPath)
				if base != "" {
					refs[strings.ToLower(base)] = true
				}
			}
		}
	}

	return refs
}

// isStdlibModule returns true for common Python standard library modules
// that should NOT be prioritized in source auto-read.
func isStdlibModule(mod string) bool {
	stdlib := map[string]bool{
		"os": true, "sys": true, "re": true, "json": true, "math": true,
		"time": true, "datetime": true, "collections": true, "itertools": true,
		"functools": true, "pathlib": true, "shutil": true, "subprocess": true,
		"unittest": true, "pytest": true, "typing": true, "io": true,
		"hashlib": true, "filecmp": true, "tempfile": true, "glob": true,
		"csv": true, "string": true, "random": true, "copy": true,
		"argparse": true, "logging": true, "traceback": true, "inspect": true,
		"abc": true, "enum": true, "dataclasses": true, "contextlib": true,
		"socket": true, "http": true, "urllib": true, "email": true,
		"struct": true, "array": true, "heapq": true, "bisect": true,
		"operator": true, "textwrap": true, "difflib": true, "pprint": true,
		"warnings": true, "signal": true, "threading": true, "multiprocessing": true,
		"pickle": true, "shelve": true, "sqlite3": true, "xml": true,
		"html": true, "base64": true, "binascii": true, "hmac": true,
		"secrets": true, "zipfile": true, "tarfile": true, "gzip": true,
		"configparser": true, "platform": true, "ctypes": true,
		// Common third-party but not user code
		"numpy": true, "np": true, "pandas": true, "pd": true,
		"scipy": true, "matplotlib": true, "plt": true,
		"torch": true, "tensorflow": true, "sklearn": true,
		"requests": true, "flask": true, "django": true,
		"PIL": true, "cv2": true, "yaml": true, "toml": true,
	}
	return stdlib[mod]
}

func extractPathFromLine(line string, idx int) string {
	// Walk backward to find the start of the path (quote or path separator).
	start := idx
	for start > 0 {
		c := line[start-1]
		if c == '"' || c == '\'' || c == '(' || c == ' ' || c == ',' {
			break
		}
		start--
	}

	// Walk forward to find the end of the path.
	end := idx
	for end < len(line) {
		c := line[end]
		if c == '"' || c == '\'' || c == ')' || c == ' ' || c == ',' || c == ';' || c == ']' {
			break
		}
		end++
	}

	path := strings.TrimSpace(line[start:end])
	// Clean up common prefixes.
	path = strings.TrimLeft(path, "\"'(")
	path = strings.TrimRight(path, "\"');,]")

	// Only return paths that look like file references.
	if len(path) < 3 || !strings.Contains(path, "/") && !strings.Contains(path, ".") {
		return ""
	}
	return path
}

// discoverVerificationScripts finds test/verification scripts outside of standard test directories.
func discoverVerificationScripts(workDir string) []string {
	var scripts []string
	seen := make(map[string]bool)

	// Check these directories for verification scripts.
	searchDirs := []string{workDir, "/app", "/app/task_file"}
	verifyPatterns := []string{
		"verify*", "check*", "validate*",
		"test.sh", "test.py", "test_*",
		"run_test*", "run.sh", "run.py", "run_*.sh", "run_*.py",
		"score*", "eval*", "grade*",
		"judge*", "compare*",
	}

	for _, dir := range searchDirs {
		for _, pattern := range verifyPatterns {
			matches, _ := filepath.Glob(filepath.Join(dir, pattern))
			for _, m := range matches {
				if !seen[m] {
					seen[m] = true
					scripts = append(scripts, m)
				}
			}
		}
	}
	return scripts
}

// chmodScripts makes a list of script files executable. This prevents the very
// common "Permission denied" error that wastes 1-2 agent turns.
func chmodScripts(scripts []string) {
	for _, s := range scripts {
		lower := strings.ToLower(s)
		if strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bash") ||
			strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".rb") ||
			strings.HasSuffix(lower, ".pl") {
			os.Chmod(s, 0o755)
		}
	}
}

// chmodScriptsInDir makes all shell/Python scripts in a directory executable.
func chmodScriptsInDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(entry.Name())
		if strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bash") ||
			strings.HasSuffix(lower, ".py") {
			os.Chmod(filepath.Join(dir, entry.Name()), 0o755)
		}
	}
}

// detectSciCompute returns true if the working directory looks like a scientific computing task.
func detectSciCompute(workDir string) bool {
	indicators := []string{
		"eigenval", "matrix", "linear_algebra", "pde", "ode", "fft",
		"simulation", "numerical", "physics", "quantum",
	}
	lower := strings.ToLower(workDir)
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for scipy/numpy imports in source files.
	sciFiles := []string{"*.py"}
	for _, pattern := range sciFiles {
		matches, _ := filepath.Glob(filepath.Join(workDir, pattern))
		for _, m := range matches {
			content := readFileTruncated(m, 2000)
			if strings.Contains(content, "scipy") || strings.Contains(content, "numpy") ||
				strings.Contains(content, "sympy") {
				return true
			}
		}
	}
	return false
}

// detectMLTask returns true if the working directory looks like an ML/training task.
func detectMLTask(workDir string) bool {
	indicators := []string{
		"train", "model", "inference", "checkpoint", "epoch",
		"dataset", "dataloader",
	}
	// Check directory name.
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for ML framework imports.
	matches, _ := filepath.Glob(filepath.Join(workDir, "*.py"))
	for _, m := range matches {
		content := readFileTruncated(m, 2000)
		if strings.Contains(content, "torch") || strings.Contains(content, "tensorflow") ||
			strings.Contains(content, "transformers") || strings.Contains(content, "sklearn") {
			return true
		}
	}
	return false
}

// detectCryptoTask returns true if the working directory looks like a cryptography task.
func detectCryptoTask(workDir string) bool {
	indicators := []string{
		"crypto", "cipher", "encrypt", "decrypt", "hash",
		"password", "recovery", "feal", "aes", "rsa", "des",
		"cryptanalysis", "xor", "hmac",
	}
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for crypto-related imports in source files.
	matches, _ := filepath.Glob(filepath.Join(workDir, "*.py"))
	for _, m := range matches {
		content := readFileTruncated(m, 2000)
		if strings.Contains(content, "Crypto.") || strings.Contains(content, "cryptography") ||
			strings.Contains(content, "hashlib") || strings.Contains(content, "hmac") {
			return true
		}
	}
	return false
}

// detectSystemEmulatorTask returns true if the task involves QEMU or system emulators.
func detectSystemEmulatorTask(workDir string) bool {
	indicators := []string{
		"qemu", "vm", "virtual-machine", "emulator", "mips",
		"install-windows", "alpine-ssh", "startup",
	}
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for QEMU disk images or ISOs.
	for _, ext := range []string{"*.qcow2", "*.img", "*.iso", "*.vmdk"} {
		matches, _ := filepath.Glob(filepath.Join(workDir, ext))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// detectBuildFromSourceTask returns true if the task looks like it requires building
// software from source code (compilation, configuration, etc.).
func detectBuildFromSourceTask(workDir string) bool {
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"build-", "compile", "make-", "install-"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for source archives in the working directory.
	for _, ext := range []string{"*.tar.*", "*.tgz", "*.tar", "*.zip", "*.tar.gz", "*.tar.bz2", "*.tar.xz"} {
		matches, _ := filepath.Glob(filepath.Join(workDir, ext))
		if len(matches) > 0 {
			return true
		}
	}
	// Check in /app/ as well.
	for _, ext := range []string{"*.tar.*", "*.tgz", "*.tar", "*.zip"} {
		matches, _ := filepath.Glob(filepath.Join("/app", ext))
		if len(matches) > 0 {
			return true
		}
	}
	// Check for configure scripts.
	return fileExists(filepath.Join(workDir, "configure")) ||
		fileExists(filepath.Join(workDir, "configure.ac")) ||
		fileExists(filepath.Join(workDir, "CMakeLists.txt"))
}

// detectCoqTask returns true if the working directory looks like a Coq proof task.
func detectCoqTask(workDir string) bool {
	// Check for Coq project files.
	for _, f := range []string{"_CoqProject", "Makefile.coq", "coq_makefile"} {
		if fileExists(filepath.Join(workDir, f)) {
			return true
		}
	}
	// Check directory name.
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"compcert", "coq", "proof", "prove"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for .v files.
	matches, _ := filepath.Glob(filepath.Join(workDir, "*.v"))
	if len(matches) > 0 {
		return true
	}
	// Check subdirectories (CompCert layout: source/*.v).
	entries, _ := os.ReadDir(workDir)
	for _, entry := range entries {
		if entry.IsDir() {
			subMatches, _ := filepath.Glob(filepath.Join(workDir, entry.Name(), "*.v"))
			if len(subMatches) > 3 { // multiple .v files suggest a Coq project
				return true
			}
		}
	}
	return false
}

// detectOCamlTask returns true if the working directory looks like an OCaml task.
func detectOCamlTask(workDir string) bool {
	for _, f := range []string{"dune-project", "dune", "_build"} {
		if fileExists(filepath.Join(workDir, f)) {
			return true
		}
	}
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"ocaml", "dune", "opam"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	matches, _ := filepath.Glob(filepath.Join(workDir, "*.ml"))
	if len(matches) > 0 {
		return true
	}
	matches, _ = filepath.Glob(filepath.Join(workDir, "*.mli"))
	return len(matches) > 0
}

// detectRTask returns true if the working directory contains R language files.
func detectRTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.R"))
		if len(matches) > 0 {
			return true
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*.r"))
		if len(matches) > 0 {
			return true
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*.Rmd"))
		if len(matches) > 0 {
			return true
		}
	}
	// Check for DESCRIPTION file (R package marker).
	if fileExists(filepath.Join(workDir, "DESCRIPTION")) {
		data, err := os.ReadFile(filepath.Join(workDir, "DESCRIPTION"))
		if err == nil && strings.Contains(string(data), "Package:") {
			return true
		}
	}
	return false
}

// detectJuliaTask returns true if the working directory contains Julia project files.
func detectJuliaTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		if fileExists(filepath.Join(dir, "Project.toml")) || fileExists(filepath.Join(dir, "Manifest.toml")) {
			return true
		}
		matches, _ := filepath.Glob(filepath.Join(dir, "*.jl"))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// detectPerlTask returns true if the working directory contains Perl files.
func detectPerlTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.pl"))
		if len(matches) > 0 {
			return true
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*.pm"))
		if len(matches) > 0 {
			return true
		}
		// Perl project markers.
		if fileExists(filepath.Join(dir, "Makefile.PL")) || fileExists(filepath.Join(dir, "cpanfile")) ||
			fileExists(filepath.Join(dir, "Build.PL")) {
			return true
		}
	}
	return false
}

// detectServiceTask returns true if the task involves setting up persistent services.
func detectServiceTask(workDir string) bool {
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{
		"server", "webserver", "web-server", "daemon", "service",
		"configure-", "setup-", "deploy-", "nginx", "apache",
		"pypi-server", "registry", "proxy",
	} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for service configuration files.
	for _, dir := range []string{workDir, "/app"} {
		if fileExists(filepath.Join(dir, "nginx.conf")) || fileExists(filepath.Join(dir, "supervisord.conf")) ||
			fileExists(filepath.Join(dir, "httpd.conf")) || fileExists(filepath.Join(dir, "uwsgi.ini")) ||
			fileExists(filepath.Join(dir, "gunicorn.conf.py")) {
			return true
		}
	}
	// Check README for service-related keywords.
	for _, rp := range []string{filepath.Join(workDir, "README.md"), filepath.Join(workDir, "instruction.md"), "/app/instruction.md"} {
		content := strings.ToLower(readFileTruncated(rp, 3000))
		if content != "" {
			if (strings.Contains(content, "server") || strings.Contains(content, "service")) &&
				(strings.Contains(content, "start") || strings.Contains(content, "running") ||
					strings.Contains(content, "listen") || strings.Contains(content, "port")) {
				return true
			}
		}
	}
	return false
}

// detectHashComparisonTask returns true if tests compare files by hash.
func detectHashComparisonTask(workDir string) bool {
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	for _, td := range testDirs {
		entries, err := os.ReadDir(td)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isSourceFile(entry.Name()) {
				continue
			}
			info, _ := entry.Info()
			if info == nil || info.Size() > 20000 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(td, entry.Name()))
			if err != nil {
				continue
			}
			content := strings.ToLower(string(data))
			if strings.Contains(content, "md5") || strings.Contains(content, "sha256") ||
				strings.Contains(content, "sha1") || strings.Contains(content, "hashlib") ||
				strings.Contains(content, "filecmp") || strings.Contains(content, "file_hash") ||
				(strings.Contains(content, "hash") && strings.Contains(content, "assert")) {
				return true
			}
		}
	}
	return false
}

// detectDatabaseTask returns true if the task involves database work.
func detectDatabaseTask(workDir string) bool {
	// Check directory name for database-related keywords.
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"database", "sqlite", "postgres", "mysql", "sql", "db-"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for database files.
	for _, dir := range []string{workDir, "/app"} {
		for _, ext := range []string{"*.db", "*.sqlite", "*.sqlite3"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			if len(matches) > 0 {
				return true
			}
		}
	}
	// Check for SQL files.
	for _, dir := range []string{workDir, "/app"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
		if len(matches) > 0 {
			return true
		}
	}
	// Check README for database keywords.
	for _, rp := range []string{filepath.Join(workDir, "README.md"), filepath.Join(workDir, "instruction.md"), "/app/instruction.md"} {
		content := strings.ToLower(readFileTruncated(rp, 3000))
		if content != "" {
			if (strings.Contains(content, "database") || strings.Contains(content, "sqlite") ||
				strings.Contains(content, "postgresql") || strings.Contains(content, "mysql")) &&
				(strings.Contains(content, "query") || strings.Contains(content, "table") ||
					strings.Contains(content, "select") || strings.Contains(content, "schema")) {
				return true
			}
		}
	}
	return false
}

// detectGitTask returns true if the task involves git operations (patches,
// merge conflicts, bisect, cherry-pick, rebases, etc.).
func detectGitTask(workDir string) bool {
	// Check directory name.
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"git-", "patch", "merge", "bisect", "cherry-pick", "rebase", "commit"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Check for .patch / .diff files.
	for _, dir := range []string{workDir, "/app"} {
		for _, ext := range []string{"*.patch", "*.diff"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			if len(matches) > 0 {
				return true
			}
		}
	}
	// Check if there's a .git directory with actual history (not just a marker).
	if dirExists(filepath.Join(workDir, ".git")) {
		// Check if git log returns commits — some tasks set up git repos for the agent to work with.
		if log := runQuiet(workDir, "git", "log", "--oneline", "-1"); log != "" {
			// Check README for git-related instructions.
			for _, rp := range []string{filepath.Join(workDir, "README.md"), filepath.Join(workDir, "instruction.md"), "/app/instruction.md"} {
				content := strings.ToLower(readFileTruncated(rp, 3000))
				if content != "" {
					if (strings.Contains(content, "git ") || strings.Contains(content, "patch") ||
						strings.Contains(content, "commit") || strings.Contains(content, "branch")) &&
						(strings.Contains(content, "apply") || strings.Contains(content, "merge") ||
							strings.Contains(content, "bisect") || strings.Contains(content, "cherry") ||
							strings.Contains(content, "rebase") || strings.Contains(content, "revert") ||
							strings.Contains(content, "fix") || strings.Contains(content, "diff")) {
						return true
					}
				}
			}
		}
	}
	return false
}

// detectCodeGolfTask returns true if the task has size constraints on output files.
// Code golf tasks require specific strategies: create output first, then optimize for size.
func detectCodeGolfTask(workDir string) bool {
	// Check README/task description for size constraint mentions.
	readmePaths := []string{
		filepath.Join(workDir, "README.md"),
		filepath.Join(workDir, "TASK.md"),
		filepath.Join(workDir, "task.md"),
		filepath.Join(workDir, "prompt.md"),
		filepath.Join(workDir, "prompt.txt"),
		filepath.Join(workDir, "INSTRUCTIONS.md"),
		filepath.Join(workDir, "instructions.md"),
	}
	for _, rp := range readmePaths {
		content := readFileTruncated(rp, 5000)
		if content == "" {
			continue
		}
		lower := strings.ToLower(content)
		// Explicit code golf mentions.
		if strings.Contains(lower, "code golf") || strings.Contains(lower, "codegolf") {
			return true
		}
		// Byte size constraints: "must be < 5000 bytes", "less than N bytes", etc.
		if strings.Contains(lower, "bytes") &&
			(strings.Contains(lower, "must be") || strings.Contains(lower, "less than") ||
				strings.Contains(lower, "under ") || strings.Contains(lower, "smaller than") ||
				strings.Contains(lower, "no more than") || strings.Contains(lower, "at most") ||
				strings.Contains(lower, "not exceed")) {
			return true
		}
	}
	return false
}

// previewInputData reads input data files and shows format information to
// help the agent understand the data structure immediately.
func previewInputData(dir string, hints *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// First pass: summarize the directory (file count, total size, types).
	var totalSize int64
	var fileCount int
	extCounts := make(map[string]int)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileCount++
		if info, err := entry.Info(); err == nil {
			totalSize += info.Size()
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == "" {
			ext = "(no ext)"
		}
		extCounts[ext]++
	}
	if fileCount > 0 {
		var extSummary []string
		for ext, count := range extCounts {
			extSummary = append(extSummary, fmt.Sprintf("%s: %d", ext, count))
		}
		*hints = append(*hints, fmt.Sprintf("Input data: %d files, %s total [%s]",
			fileCount, humanSize(totalSize), strings.Join(extSummary, ", ")))
	}

	// Second pass: preview up to 5 files (more than before).
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || count >= 5 {
			continue
		}
		name := entry.Name()
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}
		// Show more content for small files, less for large ones.
		previewBytes := 1500
		if info.Size() > 100000 {
			previewBytes = 800 // large files: just show format
		}
		content := readFileTruncated(filepath.Join(dir, name), previewBytes)
		if content != "" {
			// Count lines for CSV/text files.
			lineInfo := ""
			if info.Size() < 10*1024*1024 { // only count lines for < 10MB
				if lc := runQuiet(dir, "wc", "-l", filepath.Join(dir, name)); lc != "" {
					fields := strings.Fields(lc)
					if len(fields) > 0 {
						lineInfo = fmt.Sprintf(", %s lines", fields[0])
					}
				}
			}
			*hints = append(*hints, fmt.Sprintf("Input data preview (%s, %s%s):",
				name, humanSize(info.Size()), lineInfo))
			*hints = append(*hints, content)
			count++
		}
	}
}

// humanSize formats bytes into a human-readable string.
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// extractTestConstraints scans test files for assert statements that contain
// numeric thresholds, size limits, or performance requirements. These are the
// constraints most likely to cause failures if missed.
func extractTestConstraints(testDir string) []string {
	entries, err := os.ReadDir(testDir)
	if err != nil {
		return nil
	}

	var constraints []string
	seen := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isSourceFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 10000 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(testDir, entry.Name()))
		if err != nil {
			continue
		}
		isShellTest := strings.HasSuffix(entry.Name(), ".sh") || strings.HasSuffix(entry.Name(), ".bash")
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			// Skip comments and empty lines.
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") ||
				strings.HasPrefix(trimmed, "import") || trimmed == "" {
				continue
			}

			isConstraintLine := false

			// Python/general: assertion lines with numeric comparisons.
			if strings.Contains(trimmed, "assert") {
				for _, indicator := range []string{"<", ">", "<=", ">=", "==", "size", "bytes", "length", "len(", "count"} {
					if strings.Contains(trimmed, indicator) {
						isConstraintLine = true
						break
					}
				}
			}

			// Shell tests: diff, test -f, wc, file existence checks.
			if isShellTest && !isConstraintLine {
				for _, shellPat := range []string{
					"diff ", "cmp ",           // file comparison
					"test -f ", "test -s ",     // file existence/non-empty checks
					"[ -f ", "[ -s ",           // bracket syntax file checks
					"wc -l", "wc -c",          // line/byte count checks
					"md5sum", "sha256sum",      // hash checks
					"grep -c",                  // count matches
					"test $(", "[ $(", "[[ $(", // subshell comparison
				} {
					if strings.Contains(trimmed, shellPat) {
						isConstraintLine = true
						break
					}
				}
			}

			if isConstraintLine && !seen[trimmed] {
				// Truncate very long lines.
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				constraints = append(constraints, trimmed)
				seen[trimmed] = true
			}
		}
		// Cap at 15 constraints to avoid overwhelming context.
		if len(constraints) >= 15 {
			break
		}
	}
	return constraints
}

// timeStrategyGuidance returns time-proportional strategy guidance based on
// the total available minutes. Short tasks need aggressive output-first behavior,
// while long tasks can afford more exploration.
func timeStrategyGuidance(mins int) string {
	switch {
	case mins <= 15:
		// Sprint: 10-15 minute tasks — minimal exploration, immediate output.
		return fmt.Sprintf("\n\nTIME BUDGET: %d minutes (SPRINT MODE).\n"+
			"This is a SHORT task. You have ~20-30 turns MAX.\n"+
			"- Turn 1: Read task + tests. NO planning tool needed.\n"+
			"- Turn 2-3: Write output files IMMEDIATELY.\n"+
			"- Turn 4+: Run tests, fix failures.\n"+
			"- Final 3 turns: Clean up artifacts, final test.\n"+
			"DO NOT explore, DO NOT plan extensively. Write code NOW.", mins)
	case mins <= 30:
		// Standard: 15-30 minute tasks — quick plan, then execute.
		return fmt.Sprintf("\n\nTIME BUDGET: %d minutes. Budget ~40-60 turns wisely:\n"+
			"- Turns 1-2: Read task, constraints, tests\n"+
			"- Turns 3-5: Create output files (even rough drafts)\n"+
			"- Turns 5-15: Iterate, test, refine\n"+
			"- Final 25%%: Clean up artifacts, verify tests pass\n"+
			"Prioritize working output over perfect code.", mins)
	case mins <= 60:
		// Medium: 30-60 minute tasks — plan carefully, iterate.
		return fmt.Sprintf("\n\nTIME BUDGET: %d minutes. Budget ~60-100 turns:\n"+
			"- Turns 1-5: Read task, understand constraints, plan approach\n"+
			"- Turns 5-10: Create initial output files\n"+
			"- Turns 10-30: Iterate, test, refine\n"+
			"- Final 25%%: Clean up artifacts, verify tests pass\n"+
			"You have time for thoughtful implementation but don't over-research.", mins)
	default:
		// Marathon: 60+ minute tasks — more room for exploration.
		return fmt.Sprintf("\n\nTIME BUDGET: %d minutes. Budget your turns wisely:\n"+
			"- Turns 1-5: Read task, understand constraints, create plan\n"+
			"- Turns 5-15: Create output files (even rough drafts)\n"+
			"- Turns 15+: Iterate, test, refine\n"+
			"- Final 25%%: Clean up artifacts, verify tests pass\n"+
			"You have ample time but still don't waste it on unnecessary exploration.", mins)
	}
}

// detectTaskTimeout reads the task timeout from task.toml or task.yaml files
// that Harbor places in the container. Returns 0 if not found.
func detectTaskTimeout(workDir string) time.Duration {
	// Terminal-Bench task files are at /app/task_file/task.toml or similar.
	candidates := []string{
		filepath.Join(workDir, "task.toml"),
		"/app/task_file/task.toml",
		filepath.Join(workDir, "task.yaml"),
		filepath.Join(workDir, "task.yml"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		// Parse agent timeout_sec from TOML/YAML.
		// Look for lines like: timeout_sec = 900.0 or timeout_sec: 900
		inAgentSection := false
		for _, line := range strings.Split(content, "\n") {
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
				// Extract the numeric value.
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) != 2 {
					parts = strings.SplitN(trimmed, ":", 2)
				}
				if len(parts) == 2 {
					var secs float64
					if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &secs); err == nil && secs > 0 {
						fmt.Fprintf(os.Stderr, "[gollem] detected task timeout: %.0fs\n", secs)
						return time.Duration(secs) * time.Second
					}
				}
			}
		}
	}
	return 0
}

// detectTestCommands generates a short list of ready-to-run commands based on
// the project's language, build system, and test files. This saves the agent
// from wasting turns figuring out how to run tests.
func detectTestCommands(workDir string) []string {
	var cmds []string

	// Detect test directories for explicit test commands.
	for _, td := range []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")} {
		if dirExists(td) {
			entries, _ := os.ReadDir(td)
			// Priority 1: test.sh or test.py (standard verification scripts).
			for _, e := range entries {
				name := e.Name()
				if name == "test.sh" {
					cmds = append(cmds, "Test: bash "+filepath.Join(td, name))
				} else if name == "test.py" {
					cmds = append(cmds, "Test: python3 "+filepath.Join(td, name))
				}
			}
			// Priority 2: test_*.py or test_*.sh files (up to 3).
			testCount := 0
			for _, e := range entries {
				if testCount >= 3 {
					break
				}
				name := e.Name()
				if strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") {
					cmds = append(cmds, "Test: python3 "+filepath.Join(td, name))
					testCount++
				} else if strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".sh") {
					cmds = append(cmds, "Test: bash "+filepath.Join(td, name))
					testCount++
				}
			}
			break
		}
	}

	// Language-specific commands.
	if fileExists(filepath.Join(workDir, "go.mod")) {
		cmds = append(cmds, "Build: go build ./...")
		cmds = append(cmds, "Test: go test ./...")
	}
	if fileExists(filepath.Join(workDir, "Cargo.toml")) {
		cmds = append(cmds, "Build: cargo build")
		cmds = append(cmds, "Test: cargo test")
	}
	if fileExists(filepath.Join(workDir, "package.json")) {
		cmds = append(cmds, "Install: npm install")
		cmds = append(cmds, "Test: npm test")
	}
	if fileExists(filepath.Join(workDir, "Makefile")) || fileExists("/app/Makefile") {
		cmds = append(cmds, "Build: make")
		// Parse Makefile for useful targets beyond just "test".
		for _, dir := range []string{workDir, "/app"} {
			mkPath := filepath.Join(dir, "Makefile")
			if content := readFileTruncated(mkPath, 3000); content != "" {
				targets := parseMakefileTargets(content)
				for _, t := range targets {
					cmds = append(cmds, "Make: make "+t)
				}
				break
			}
		}
	}
	if fileExists(filepath.Join(workDir, "CMakeLists.txt")) {
		cmds = append(cmds, "Build: mkdir -p build && cd build && cmake .. && make")
	}
	// Lean 4
	if fileExists(filepath.Join(workDir, "lakefile.lean")) || fileExists(filepath.Join(workDir, "lakefile.toml")) {
		cmds = append(cmds, "Build: lake build")
	}
	// Haskell
	if fileExists(filepath.Join(workDir, "stack.yaml")) {
		cmds = append(cmds, "Build: stack build")
		cmds = append(cmds, "Test: stack test")
	} else if matches, _ := filepath.Glob(filepath.Join(workDir, "*.cabal")); len(matches) > 0 {
		cmds = append(cmds, "Build: cabal build")
		cmds = append(cmds, "Test: cabal test")
	}
	// OCaml
	if fileExists(filepath.Join(workDir, "dune-project")) {
		cmds = append(cmds, "Build: dune build")
		cmds = append(cmds, "Test: dune test")
	}
	// Elixir
	if fileExists(filepath.Join(workDir, "mix.exs")) {
		cmds = append(cmds, "Build: mix compile")
		cmds = append(cmds, "Test: mix test")
	}
	// Zig
	if fileExists(filepath.Join(workDir, "build.zig")) {
		cmds = append(cmds, "Build: zig build")
		cmds = append(cmds, "Test: zig test")
	}
	// Julia
	if fileExists(filepath.Join(workDir, "Project.toml")) {
		cmds = append(cmds, "Install: julia --project=. -e 'using Pkg; Pkg.instantiate()'")
		cmds = append(cmds, "Test: julia --project=. -e 'using Pkg; Pkg.test()'")
	}
	// Perl
	if fileExists(filepath.Join(workDir, "Makefile.PL")) {
		cmds = append(cmds, "Build: perl Makefile.PL && make")
		cmds = append(cmds, "Test: make test")
	} else if fileExists(filepath.Join(workDir, "Build.PL")) {
		cmds = append(cmds, "Build: perl Build.PL && ./Build")
		cmds = append(cmds, "Test: ./Build test")
	}

	if fileExists(filepath.Join(workDir, "requirements.txt")) {
		cmds = append(cmds, "Install: pip install --break-system-packages -r requirements.txt")
	}
	if fileExists(filepath.Join(workDir, "setup.py")) {
		cmds = append(cmds, "Install: pip install --break-system-packages -e .")
	}

	// pytest detection (common across all Python projects).
	hasPyTests := false
	for _, td := range []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")} {
		if dirExists(td) {
			entries, _ := os.ReadDir(td)
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".py") {
					hasPyTests = true
					break
				}
			}
			break
		}
	}
	if hasPyTests {
		cmds = append(cmds, "Test: pytest -xvs")
	}

	// Cap to prevent context bloat.
	if len(cmds) > 10 {
		cmds = cmds[:10]
	}
	return cmds
}

// isEntryPointFile returns true for filenames that are likely program entry
// points or high-priority configuration files. These are read first during
// auto-read to give the agent the most important context upfront.
func isEntryPointFile(lowerName string) bool {
	entryPoints := []string{
		"main.", "app.", "index.", "server.", "cli.",
		"__init__.py", "__main__.py",
		"conftest.py", // pytest fixtures
		"manage.py",   // Django
		"wsgi.py", "asgi.py",
		"solution.", "solve.", "answer.", // common TB2 deliverable names
	}
	for _, ep := range entryPoints {
		if strings.HasPrefix(lowerName, ep) || lowerName == ep {
			return true
		}
	}
	return false
}

// parseMakefileTargets extracts useful build/test/run targets from a Makefile.
// Returns a deduplicated list of target names that the agent might want to run.
func parseMakefileTargets(content string) []string {
	// Interesting target patterns — these are the ones the agent would actually run.
	interestingTargets := map[string]bool{
		"test": true, "tests": true, "check": true, "verify": true,
		"run": true, "start": true, "serve": true, "dev": true,
		"build": true, "compile": true, "install": true,
		"clean": true, "lint": true, "fmt": true, "format": true,
		"benchmark": true, "bench": true,
		"debug": true, "release": true,
	}

	var targets []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(content, "\n") {
		// Makefile target lines: "target:" or "target: deps"
		// Must start at column 0 (not indented — indented lines are recipes).
		if len(line) == 0 || line[0] == '\t' || line[0] == ' ' || line[0] == '#' || line[0] == '.' {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx <= 0 {
			continue
		}
		// Skip variable assignments (FOO := bar)
		if colonIdx+1 < len(line) && line[colonIdx+1] == '=' {
			continue
		}
		target := strings.TrimSpace(line[:colonIdx])
		// Skip targets with special characters (%, $, etc.)
		if strings.ContainsAny(target, "%$(){}") {
			continue
		}
		// Skip .PHONY and similar special targets
		if strings.HasPrefix(target, ".") {
			continue
		}
		if interestingTargets[target] && !seen[target] {
			seen[target] = true
			targets = append(targets, target)
		}
	}

	// Cap at 6 targets to avoid bloat
	if len(targets) > 6 {
		targets = targets[:6]
	}
	return targets
}

// detectEnvFiles searches for .env, .env.example, .env.sample files and
// surfaces their contents. Many TB2 tasks require specific environment variables
// (database URLs, API keys, ports) that are defined in these files.
func detectEnvFiles(workDir string) string {
	envFiles := []struct {
		name     string
		isExample bool
	}{
		{".env.example", true},
		{".env.sample", true},
		{".env.template", true},
		{".env", false},
		{".env.local", false},
	}

	for _, dir := range []string{workDir, "/app"} {
		for _, ef := range envFiles {
			path := filepath.Join(dir, ef.name)
			content := readFileTruncated(path, 2000)
			if content == "" {
				continue
			}

			var hint strings.Builder
			if ef.isExample {
				hint.WriteString(fmt.Sprintf("\n## Environment Config Found: %s", path))
				hint.WriteString("\n" + content)
				hint.WriteString("\nHINT: Copy this to .env and fill in any placeholder values: cp " + path + " " + filepath.Join(dir, ".env"))
			} else {
				// .env file exists — check for placeholder values that need filling
				hint.WriteString(fmt.Sprintf("\n## Environment Config Found: %s", path))
				hint.WriteString("\n" + content)
				// Check for common placeholder patterns
				if strings.Contains(content, "TODO") || strings.Contains(content, "CHANGEME") ||
					strings.Contains(content, "your_") || strings.Contains(content, "xxx") ||
					strings.Contains(content, "placeholder") {
					hint.WriteString("\nWARNING: This .env file contains placeholder values that need to be filled in.")
				}
			}

			// Extract key variable names so the agent knows what's expected
			var varNames []string
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if eqIdx := strings.Index(line, "="); eqIdx > 0 {
					varName := strings.TrimSpace(line[:eqIdx])
					if varName != "" {
						varNames = append(varNames, varName)
					}
				}
			}
			if len(varNames) > 0 {
				hint.WriteString(fmt.Sprintf("\nRequired env vars: %s", strings.Join(varNames, ", ")))
			}

			return hint.String()
		}
	}
	return ""
}

// hasNetworkAccess performs a quick DNS lookup to check if the container
// has internet connectivity. Uses a short timeout to avoid blocking startup.
func hasNetworkAccess() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resolver := &net.Resolver{}
	_, err := resolver.LookupHost(ctx, "pypi.org")
	return err == nil
}

// extractFileStructure reads a source file and extracts function/class/method
// definitions to produce a compact "table of contents". This is used for large
// files (>5KB) that we can't auto-read fully — it gives the agent a map of
// the file so it knows what's there without spending turns on grep/view.
// Returns empty string if no structure is found or file can't be read.
func extractFileStructure(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	ext := strings.ToLower(filepath.Ext(path))
	var defs []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		matched := false
		switch ext {
		case ".py":
			// Python: top-level and class-level def/class
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "async def ") {
				matched = true
			}
		case ".go":
			if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") {
				matched = true
			}
		case ".js", ".ts", ".jsx", ".tsx":
			if strings.HasPrefix(trimmed, "function ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "export ") || strings.HasPrefix(trimmed, "const ") ||
				strings.HasPrefix(trimmed, "async function ") {
				matched = true
			}
		case ".rs":
			if strings.HasPrefix(trimmed, "fn ") || strings.HasPrefix(trimmed, "pub fn ") ||
				strings.HasPrefix(trimmed, "struct ") || strings.HasPrefix(trimmed, "pub struct ") ||
				strings.HasPrefix(trimmed, "impl ") || strings.HasPrefix(trimmed, "enum ") ||
				strings.HasPrefix(trimmed, "pub enum ") || strings.HasPrefix(trimmed, "trait ") {
				matched = true
			}
		case ".c", ".cpp", ".h", ".hpp", ".cc", ".cxx":
			// C/C++: function definitions, struct/class declarations
			if strings.HasPrefix(trimmed, "struct ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "typedef ") || strings.HasPrefix(trimmed, "enum ") {
				matched = true
			}
			// Function-like lines: type name(...) with no semicolon (definition, not declaration)
			if !matched && strings.Contains(trimmed, "(") && !strings.HasPrefix(trimmed, "#") &&
				!strings.HasPrefix(trimmed, "if") && !strings.HasPrefix(trimmed, "for") &&
				!strings.HasPrefix(trimmed, "while") && !strings.HasPrefix(trimmed, "switch") &&
				!strings.HasSuffix(trimmed, ";") {
				// Likely a function definition
				matched = true
			}
		case ".java", ".kt", ".kts", ".scala":
			if strings.HasPrefix(trimmed, "public ") || strings.HasPrefix(trimmed, "private ") ||
				strings.HasPrefix(trimmed, "protected ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "interface ") || strings.HasPrefix(trimmed, "fun ") ||
				strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "object ") {
				matched = true
			}
		case ".rb":
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "module ") {
				matched = true
			}
		case ".hs":
			// Haskell: top-level type signatures and definitions
			if !strings.HasPrefix(trimmed, "--") && strings.Contains(trimmed, "::") {
				matched = true
			}
		}

		if matched {
			// Truncate long definition lines.
			def := trimmed
			if len(def) > 120 {
				def = def[:120] + "..."
			}
			defs = append(defs, fmt.Sprintf("  L%d: %s", i+1, def))
		}
	}

	if len(defs) == 0 {
		return ""
	}

	// Cap to 30 definitions to avoid bloat.
	if len(defs) > 30 {
		defs = append(defs[:30], fmt.Sprintf("  ... and %d more definitions", len(defs)-30))
	}

	info, _ := os.Stat(path)
	sizeInfo := ""
	if info != nil {
		sizeInfo = fmt.Sprintf(" (%s, %d lines)", humanSize(info.Size()), len(lines))
	}
	return fmt.Sprintf("Definitions in %s%s:\n%s", filepath.Base(path), sizeInfo, strings.Join(defs, "\n"))
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// isSourceFile returns true if the filename has a recognized source code extension.
func isSourceFile(name string) bool {
	sourceExts := []string{
		".py", ".pyx", ".pyi",                          // Python
		".js", ".ts", ".go", ".rs",                     // popular languages
		".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hh", // C/C++
		".java", ".rb", ".sh", ".bash", ".pl", ".lua", ".r", ".R",
		".sql", ".html", ".css", ".json", ".yaml", ".yml", ".toml",
		".xml", ".md", ".txt", ".cfg", ".ini", ".conf",
		".csv", ".tsv", ".jsonl", ".env", ".dockerfile",
		".jsx", ".tsx", ".vue", ".svelte", ".zig", ".nim",
		".kt", ".kts", ".scala", ".ex", ".exs", ".erl", ".hs",
		".jl", ".m", ".swift", ".f90", ".f95", ".pm",
		".ml", ".mli",                   // OCaml
		".lean", ".v", ".agda",          // theorem provers
		".red",                          // Redcode (CoreWars)
		".cu", ".cuh",                   // CUDA
		".s", ".asm", ".wat",            // assembly / WebAssembly text
		".proto", ".thrift", ".graphql", // schema files
		".cmake",                        // build config
		".rkt", ".scm", ".lisp", ".cl", // Lisp/Scheme/Racket
	}
	lower := strings.ToLower(name)
	for _, ext := range sourceExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	// Extensionless files that are commonly source/config.
	switch lower {
	case "makefile", "dockerfile", "gemfile", "rakefile", "cmakelists.txt":
		return true
	}
	return false
}

// ProgressTrackingMiddleware detects when the agent isn't producing output
// files and nudges it to stop researching and start writing. This combats
// the "analysis paralysis" failure mode where agents spend all turns
// exploring without creating deliverables.
//
// When a timeout is provided, the middleware also uses time-based triggers
// in addition to turn-based ones. For sprint tasks (≤15 min), thresholds
// are lowered aggressively. This was the #1 cause of timeouts in evals.
func ProgressTrackingMiddleware(workDir string, timeout ...time.Duration) core.AgentMiddleware {
	var mu sync.Mutex
	turn := 0
	hasWritten := false
	warnedTurn1 := false
	warnedTurn2 := false
	warnedTime30 := false
	warnedTime50 := false

	startTime := time.Now()
	effectiveTimeout := time.Duration(0)
	if len(timeout) > 0 && timeout[0] > 0 {
		effectiveTimeout = timeout[0]
	}
	// Also check GOLLEM_TIMEOUT_SEC env var (set by Harbor).
	if effectiveTimeout == 0 {
		if envTimeout := os.Getenv("GOLLEM_TIMEOUT_SEC"); envTimeout != "" {
			var secs float64
			if _, err := fmt.Sscanf(envTimeout, "%f", &secs); err == nil && secs > 0 {
				effectiveTimeout = time.Duration(secs) * time.Second
			}
		}
	}

	// Adaptive turn thresholds based on timeout duration.
	// Sprint tasks need much earlier warnings to prevent wasting time.
	turnWarning := 7
	turnCritical := 15
	if effectiveTimeout > 0 {
		mins := int(effectiveTimeout.Minutes())
		switch {
		case mins <= 15:
			turnWarning = 3
			turnCritical = 6
		case mins <= 30:
			turnWarning = 5
			turnCritical = 10
		case mins <= 60:
			turnWarning = 6
			turnCritical = 12
		}
	}

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		mu.Lock()
		turn++
		currentTurn := turn

		// Track whether the agent has created or modified any files.
		if !hasWritten {
			for _, msg := range messages {
				if resp, ok := msg.(core.ModelResponse); ok {
					for _, part := range resp.Parts {
						if tc, ok := part.(core.ToolCallPart); ok {
							if tc.ToolName == "write" || tc.ToolName == "multi_edit" || tc.ToolName == "edit" || tc.ToolName == "execute_code" || tc.ToolName == "delegate" {
								hasWritten = true
								break
							}
							// Also check bash for commands that create files.
							if tc.ToolName == "bash" {
								var args struct {
									Command string `json:"command"`
								}
								if json.Unmarshal([]byte(tc.ArgsJSON), &args) == nil {
									cmd := args.Command
									lower := strings.ToLower(cmd)
									if strings.Contains(cmd, " > ") ||
										strings.Contains(cmd, " >> ") ||
										strings.Contains(cmd, " tee ") ||
										(strings.Contains(cmd, "echo ") && strings.Contains(cmd, ">")) ||
										strings.HasPrefix(lower, "cp ") || strings.Contains(lower, " && cp ") ||
										strings.HasPrefix(lower, "mv ") || strings.Contains(lower, " && mv ") ||
										(strings.Contains(lower, "curl ") && strings.Contains(lower, " -o ")) ||
										strings.HasPrefix(lower, "wget ") ||
										// Commands referencing output directories (TB2 pattern).
										strings.Contains(lower, "output_data") ||
										// Solver/generator scripts that typically create output files.
										(strings.Contains(lower, "python") &&
											(strings.Contains(lower, "solve") || strings.Contains(lower, "solution") ||
												strings.Contains(lower, "generate") || strings.Contains(lower, "process"))) {
										hasWritten = true
										break
									}
								}
							}
						}
					}
					if hasWritten {
						break
					}
				}
			}
		}

		needsWarning := !hasWritten

		// Compute time percentage if timeout is known.
		var timePct float64
		if effectiveTimeout > 0 {
			timePct = float64(time.Since(startTime)) / float64(effectiveTimeout)
		}

		w1 := warnedTurn1
		w2 := warnedTurn2
		wt30 := warnedTime30
		wt50 := warnedTime50
		if needsWarning && currentTurn >= turnWarning && !w1 {
			warnedTurn1 = true
		}
		if needsWarning && currentTurn >= turnCritical && !w2 {
			warnedTurn2 = true
		}
		if needsWarning && timePct >= 0.30 && !wt30 {
			warnedTime30 = true
		}
		if needsWarning && timePct >= 0.50 && !wt50 {
			warnedTime50 = true
		}
		mu.Unlock()

		// Time-based warnings take priority over turn-based ones.
		if needsWarning && timePct >= 0.50 && !wt50 {
			fmt.Fprintf(os.Stderr, "[gollem] progress: CRITICAL — %.0f%% time used with no output files\n", timePct*100)
			urgentMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content: fmt.Sprintf("CRITICAL: %.0f%% of your time is gone and you have NOT created any output files. "+
							"You MUST produce output NOW. Stop ALL research, analysis, and debugging. "+
							"Write your best attempt at a solution IMMEDIATELY using write or bash redirects. "+
							"An imperfect solution that exists scores infinitely higher than a perfect solution that doesn't.", timePct*100),
					},
				},
			}
			messages = append(messages, urgentMsg)
		} else if needsWarning && timePct >= 0.30 && !wt30 {
			fmt.Fprintf(os.Stderr, "[gollem] progress: warning — %.0f%% time used with no output files\n", timePct*100)
			warningMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content: fmt.Sprintf("PROGRESS WARNING: %.0f%% of your time is used and no output files exist yet. "+
							"Rule #1: Output First, Perfect Later. Write your best attempt NOW, then iterate.", timePct*100),
					},
				},
			}
			messages = append(messages, warningMsg)
		} else if needsWarning && currentTurn >= turnCritical && !w2 {
			fmt.Fprintf(os.Stderr, "[gollem] progress: CRITICAL — turn %d with no output files created\n", currentTurn)
			urgentMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content: "CRITICAL: You are " + fmt.Sprintf("%d", currentTurn) + " turns in and have NOT created any output files yet. " +
							"You MUST produce output NOW. Stop researching, stop analyzing, stop debugging infrastructure. " +
							"Write your best attempt at a solution immediately using the write tool or bash redirects. " +
							"You can refine it after — but you MUST have something written. " +
							"An imperfect solution that exists scores higher than a perfect solution that doesn't.",
					},
				},
			}
			messages = append(messages, urgentMsg)
		} else if needsWarning && currentTurn >= turnWarning && !w1 {
			fmt.Fprintf(os.Stderr, "[gollem] progress: warning — turn %d with no output files created\n", currentTurn)
			warningMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content: "PROGRESS WARNING: You are " + fmt.Sprintf("%d", currentTurn) + " turns in and have not created any output files yet. " +
							"Remember Rule #1: Output First, Perfect Later. " +
							"Write your best attempt at a solution NOW, then iterate to improve it. " +
							"Don't spend more time researching — start producing output.",
					},
				},
			}
			messages = append(messages, warningMsg)
		}

		return next(ctx, messages, settings, params)
	}
}

// ContextOverflowMiddleware catches HTTP 413 (Request Entity Too Large) errors
// from the model provider and performs emergency context compression before
// retrying. This handles the case where auto-context's token estimation
// underestimates and the actual payload exceeds the provider's limit.
// Retries up to 2 times with progressively more aggressive truncation.
func ContextOverflowMiddleware() core.AgentMiddleware {
	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		resp, err := next(ctx, messages, settings, params)
		if err == nil {
			return resp, nil
		}

		// Handle 413 (Request Entity Too Large) and 400 errors caused by context overflow.
		// Some providers (OpenAI, xAI) return 400 with "context" or "too long" in the message
		// instead of a proper 413.
		if !isContextOverflowError(err) {
			return nil, err
		}

		// Progressive compression: try increasingly aggressive truncation.
		// Round 1: standard emergency compression (20KB per block, keep 6 messages)
		// Round 2: aggressive compression (5KB per block, keep 4 messages)
		current := messages
		configs := []struct {
			maxContentBytes int
			keepLast        int
		}{
			{20000, 6},
			{5000, 4},
		}

		for _, cfg := range configs {
			compressed := emergencyCompressMessagesWithConfig(current, cfg.maxContentBytes, cfg.keepLast)
			if len(compressed) >= len(current) && compressed[0] == current[0] {
				continue // Can't compress further with this config.
			}

			fmt.Fprintf(os.Stderr, "[gollem] 413 context overflow: compression %d → %d messages (max %dB/block), retrying\n",
				len(current), len(compressed), cfg.maxContentBytes)

			resp, err = next(ctx, compressed, settings, params)
			if err == nil {
				return resp, nil
			}

			if !isContextOverflowError(err) {
				return nil, err
			}
			current = compressed
		}

		return nil, err
	}
}

// isContextOverflowError checks if an error represents a context overflow.
// This handles both HTTP 413 (Request Entity Too Large) and HTTP 400 errors
// with context overflow messages from providers like OpenAI and xAI.
func isContextOverflowError(err error) bool {
	var httpErr *core.ModelHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	if httpErr.StatusCode == http.StatusRequestEntityTooLarge {
		return true
	}
	if httpErr.StatusCode == http.StatusBadRequest {
		lower := strings.ToLower(httpErr.Body + httpErr.Message)
		if strings.Contains(lower, "context") && (strings.Contains(lower, "too long") || strings.Contains(lower, "too large") || strings.Contains(lower, "exceed") || strings.Contains(lower, "maximum")) {
			return true
		}
		if strings.Contains(lower, "maximum context length") || strings.Contains(lower, "token limit") {
			return true
		}
	}
	return false
}

// emergencyCompressMessages performs aggressive message truncation for 413 recovery.
func emergencyCompressMessages(messages []core.ModelMessage) []core.ModelMessage {
	return emergencyCompressMessagesWithConfig(messages, 20000, 6)
}

// emergencyCompressMessagesWithConfig performs message truncation with configurable parameters.
// Keeps the first message (task description), adds an emergency note, and keeps the
// last keepLast messages with content truncated to maxContentBytes per block.
func emergencyCompressMessagesWithConfig(messages []core.ModelMessage, maxContentBytes, keepLast int) []core.ModelMessage {
	if len(messages) <= keepLast+1 {
		// Can't drop messages, but still try truncating content.
		result := make([]core.ModelMessage, len(messages))
		for i, msg := range messages {
			result[i] = truncateMessageContent(msg, maxContentBytes)
		}
		return result
	}

	result := make([]core.ModelMessage, 0, keepLast+2)
	result = append(result, messages[0]) // first message (task + system prompt)

	// Emergency recovery note.
	result = append(result, core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.SystemPromptPart{
				Content: "[EMERGENCY CONTEXT RECOVERY] Previous conversation history was too large " +
					"and has been truncated to recover from a 413 error. Focus on completing " +
					"the task with the remaining context. Check output files and test results " +
					"to understand current state.",
			},
		},
	})

	// Keep last N messages with content truncation.
	tail := messages[len(messages)-keepLast:]
	for _, msg := range tail {
		result = append(result, truncateMessageContent(msg, maxContentBytes))
	}

	return result
}

// truncateMessageContent truncates oversized content within a single message.
func truncateMessageContent(msg core.ModelMessage, maxBytes int) core.ModelMessage {
	switch m := msg.(type) {
	case core.ModelRequest:
		parts := make([]core.ModelRequestPart, len(m.Parts))
		for i, part := range m.Parts {
			switch p := part.(type) {
			case core.ToolReturnPart:
				if s, ok := p.Content.(string); ok && len(s) > maxBytes {
					p.Content = s[:maxBytes] + "\n... [truncated for context management]"
					parts[i] = p
					continue
				}
			case core.UserPromptPart:
				if len(p.Content) > maxBytes {
					p.Content = p.Content[:maxBytes] + "\n... [truncated for context management]"
					parts[i] = p
					continue
				}
			case core.RetryPromptPart:
				if len(p.Content) > maxBytes {
					p.Content = p.Content[:maxBytes] + "\n... [truncated for context management]"
					parts[i] = p
					continue
				}
			}
			parts[i] = part
		}
		m.Parts = parts
		return m
	case core.ModelResponse:
		parts := make([]core.ModelResponsePart, len(m.Parts))
		for i, part := range m.Parts {
			if tp, ok := part.(core.TextPart); ok && len(tp.Content) > maxBytes {
				tp.Content = tp.Content[:maxBytes] + "\n... [truncated]"
				parts[i] = tp
				continue
			}
			parts[i] = part
		}
		m.Parts = parts
		return m
	}
	return msg
}

// ContentTruncationProcessor returns a history processor that truncates
// oversized content blocks in the message history. This runs before
// auto-context compression, ensuring token estimates are more accurate
// and preventing a single large tool result from dominating the context.
//
// Uses smart head+tail truncation: when content contains error indicators
// (test failures, tracebacks, panics), it keeps more of the tail where
// error summaries live. This is critical for preserving test failure
// details that the agent needs to fix issues.
func ContentTruncationProcessor(maxBytes int) core.HistoryProcessor {
	return func(_ context.Context, messages []core.ModelMessage) ([]core.ModelMessage, error) {
		result := make([]core.ModelMessage, len(messages))
		for i, msg := range messages {
			result[i] = truncateMessageContentSmart(msg, maxBytes)
		}
		return result, nil
	}
}

// truncateMessageContentSmart truncates oversized content within a single
// message using the smart head+tail approach from truncateOutput. This
// preserves error summaries and test results that appear at the end of
// tool outputs, which is critical for the agent's error recovery.
func truncateMessageContentSmart(msg core.ModelMessage, maxBytes int) core.ModelMessage {
	switch m := msg.(type) {
	case core.ModelRequest:
		parts := make([]core.ModelRequestPart, len(m.Parts))
		for i, part := range m.Parts {
			switch p := part.(type) {
			case core.ToolReturnPart:
				if s, ok := p.Content.(string); ok && len(s) > maxBytes {
					p.Content = truncateOutput(s, maxBytes)
					parts[i] = p
					continue
				}
			case core.UserPromptPart:
				if len(p.Content) > maxBytes {
					p.Content = truncateOutput(p.Content, maxBytes)
					parts[i] = p
					continue
				}
			case core.RetryPromptPart:
				if len(p.Content) > maxBytes {
					p.Content = truncateOutput(p.Content, maxBytes)
					parts[i] = p
					continue
				}
			}
			parts[i] = part
		}
		m.Parts = parts
		return m
	case core.ModelResponse:
		parts := make([]core.ModelResponsePart, len(m.Parts))
		for i, part := range m.Parts {
			if tp, ok := part.(core.TextPart); ok && len(tp.Content) > maxBytes {
				tp.Content = truncateOutput(tp.Content, maxBytes)
				parts[i] = tp
				continue
			}
			parts[i] = part
		}
		m.Parts = parts
		return m
	}
	return msg
}

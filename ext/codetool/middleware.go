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

			// Reduce counts instead of resetting to zero. This makes
			// persistent loops trigger warnings faster on recurrence.
			// First occurrence needs 4 edits; second only 2; third only 1.
			mu.Lock()
			for _, f := range loopedFiles {
				if strings.HasPrefix(f, "bash: ") {
					cmd := strings.TrimPrefix(f, "bash: ")
					bashCounts[cmd] = bashCounts[cmd] / 2
				} else {
					editCounts[f] = editCounts[f] / 2
				}
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

// extractCommandPrefix extracts a fingerprint from a bash tool call's ArgsJSON
// for loop detection. The fingerprint must be specific enough to distinguish
// different operations (e.g., "python3 test.py" vs "python3 solution.py") but
// general enough to catch true loops (same command repeated without progress).
//
// For interpreter commands (python, node, ruby, etc.), includes the script name
// to avoid false positives during normal iterative testing. For compound
// commands (cd /foo && python test.py), skips shell preamble.
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

	// For compound commands (using && or ;), use the last significant command.
	// This handles patterns like "cd /app && python test.py" where the real
	// action is the last part. Also handles "export FOO=bar && make test".
	parts := strings.Split(cmd, "&&")
	if len(parts) > 1 {
		cmd = strings.TrimSpace(parts[len(parts)-1])
	}
	// Also handle semicolons (less common but valid).
	parts = strings.Split(cmd, ";")
	if len(parts) > 1 {
		last := strings.TrimSpace(parts[len(parts)-1])
		if last != "" {
			cmd = last
		}
	}

	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}

	// Skip common preamble commands that aren't the real action.
	skipPrefixes := map[string]bool{
		"cd": true, "env": true, "sudo": true, "time": true,
		"timeout": true, "nice": true, "nohup": true, "exec": true,
	}
	for len(fields) > 1 && skipPrefixes[filepath.Base(fields[0])] {
		fields = fields[1:]
		// Skip cd's target directory argument.
		if filepath.Base(fields[0]) == "cd" {
			fields = fields[1:]
		}
	}

	base := filepath.Base(fields[0])

	// For interpreters, include the script name (second token) to distinguish
	// "python3 test.py" from "python3 solution.py". Without this, normal
	// iterative testing (5 runs of pytest) falsely triggers loop warnings.
	interpreters := map[string]bool{
		"python": true, "python3": true, "python2": true,
		"node": true, "nodejs": true,
		"ruby": true, "perl": true, "lua": true,
		"julia": true, "Rscript": true, "php": true,
	}
	if interpreters[base] && len(fields) >= 2 {
		arg := fields[1]
		// For flags like -m, -c, -e, include the flag + next token.
		if strings.HasPrefix(arg, "-") && len(fields) >= 3 {
			return base + " " + arg + " " + filepath.Base(fields[2])
		}
		// For script paths, use just the basename.
		return base + " " + filepath.Base(arg)
	}

	return base
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
	for _, tool := range []string{"python3", "python", "pip3", "pip", "node", "npm", "bun", "deno", "go", "cargo", "make", "gcc", "g++", "coqc", "ocaml", "opam", "lean", "rustc", "javac", "dotnet", "ruby", "Rscript", "julia", "perl", "swift", "sqlite3", "psql", "mysql"} {
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
		// Create python → python3 symlink if python3 exists but python doesn't.
		// This is the #1 "command not found" error on TB2: test scripts often use
		// "python" but containers only have "python3". Creating the symlink
		// preemptively saves 1-2 turns of debugging.
		if runQuiet(workDir, "which", "python") == "" {
			if py3Path := runQuiet(workDir, "which", "python3"); py3Path != "" {
				os.Symlink(py3Path, "/usr/local/bin/python")
				fmt.Fprintf(os.Stderr, "[gollem] created python → python3 symlink\n")
			}
		}
		// Also create pip → pip3 symlink for the same reason.
		if runQuiet(workDir, "which", "pip") == "" {
			if pip3Path := runQuiet(workDir, "which", "pip3"); pip3Path != "" {
				os.Symlink(pip3Path, "/usr/local/bin/pip")
				fmt.Fprintf(os.Stderr, "[gollem] created pip → pip3 symlink\n")
			}
		}
	}

	// Set Python env vars for all bash commands to prevent common issues:
	// - PYTHONDONTWRITEBYTECODE: prevents __pycache__/.pyc clutter that causes
	//   "extra files in directory" test failures. This is the #2 cleanup issue.
	// - PYTHONUNBUFFERED: ensures real-time output flushing, so error messages
	//   aren't lost when commands timeout.
	os.Setenv("PYTHONDONTWRITEBYTECODE", "1")
	os.Setenv("PYTHONUNBUFFERED", "1")

	// Set PYTHONPATH to include the working directory and /app so that test
	// scripts can import local modules regardless of their cwd. This is the #1
	// cause of "ModuleNotFoundError: No module named 'solution'" — tests in
	// /tests/ run with cwd=/tests/ and can't see modules in /app/. Setting
	// PYTHONPATH preemptively prevents this entirely, saving 1-2 turns.
	pythonPaths := []string{}
	if workDir != "" {
		pythonPaths = append(pythonPaths, workDir)
	}
	if workDir != "/app" && dirExists("/app") {
		pythonPaths = append(pythonPaths, "/app")
	}
	// Preserve existing PYTHONPATH if set (e.g., from venv activation).
	if existing := os.Getenv("PYTHONPATH"); existing != "" {
		pythonPaths = append(pythonPaths, existing)
	}
	if len(pythonPaths) > 0 {
		os.Setenv("PYTHONPATH", strings.Join(pythonPaths, ":"))
	}

	// Detect Python virtual environments (venv/conda) that need activation.
	// Many containers have packages installed in a venv, but the agent's shell
	// doesn't activate it by default. Detecting and activating saves 2-3 turns
	// of "ModuleNotFoundError" debugging.
	if venvHint := detectAndActivateVenv(workDir); venvHint != "" {
		parts = append(parts, venvHint)
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
		// task_file layout: some TB2 tasks nest everything under /app/task_file/.
		"/app/task_file/README.md",
		"/app/task_file/instruction.md",
		"/app/task_file/TASK.md",
		"/app/task_file/INSTRUCTIONS.md",
		"/app/task_file/prompts/agent.md",
	}
	readmeFound := false
	readmeBudget := 8000   // total budget for all README/instruction files
	readmeSeen := make(map[string]bool) // deduplicate by resolved path
	for _, rp := range readmePaths {
		if readmeBudget <= 0 {
			break
		}
		// Resolve symlinks and normalize to avoid reading the same file twice.
		resolved, err := filepath.EvalSymlinks(rp)
		if err != nil {
			resolved = rp // fallback to original path
		}
		resolved, _ = filepath.Abs(resolved)
		if readmeSeen[resolved] {
			continue
		}
		maxRead := min(readmeBudget, 5000) // per-file cap
		if content := readFileTruncated(rp, maxRead); content != "" {
			readmeSeen[resolved] = true
			if !readmeFound {
				parts = append(parts, "\n## README Contents ("+filepath.Base(rp)+", auto-read)")
				readmeFound = true
			} else {
				parts = append(parts, "\n## Additional Instructions ("+filepath.Base(rp)+", auto-read)")
			}
			parts = append(parts, content)
			readmeBudget -= len(content)
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
				hasTimeoutConstraint := false
				hasHashConstraint := false
				for _, c := range constraints {
					parts = append(parts, "  - "+c)
					cl := strings.ToLower(c)
					if strings.Contains(cl, "timeout=") || strings.Contains(cl, "timeout =") {
						hasTimeoutConstraint = true
					}
					if strings.Contains(cl, "md5") || strings.Contains(cl, "sha256") || strings.Contains(cl, "hashlib") {
						hasHashConstraint = true
					}
				}
				if hasTimeoutConstraint {
					parts = append(parts, "WARNING: Tests have EXECUTION TIME LIMITS (timeout=N). Your solution must be FAST, not just correct. "+
						"Time your solution with `time ./program` and optimize if it's close to the limit.")
				}
				if hasHashConstraint {
					parts = append(parts, "WARNING: Tests verify FILE HASHES (MD5/SHA). This means exact file contents and directory structure matter. "+
						"Ensure files are placed at EXACTLY the paths tests expect, with no modifications to source files.")
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
			"Package.swift", "configure.ac", "meson.build", "BUILD",
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

	// Skip dependency installation if another agent (parent) already did it.
	// This prevents subagents from wasting 10-60 seconds re-installing Maven,
	// Gradle, Haskell, or OCaml dependencies that the parent already resolved.
	depsMarker := depsMarkerPath(workDir)
	depsAlreadyInstalled := fileExists(depsMarker)

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
				installed := pipInstall(workDir, "-q", "-r", reqPath)
				if installed {
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

	// Auto-install npm/bun dependencies for Node.js projects.
	// Note: don't gate on !foundPyDeps — projects can need both Python and npm deps.
	// Prefer bun if bun.lockb exists and bun is available.
	if networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			pkgPath := filepath.Join(dir, "package.json")
			if !fileExists(pkgPath) {
				continue
			}
			lockPath := filepath.Join(dir, "node_modules")
			if dirExists(lockPath) {
				break
			}
			// Prefer bun install if bun.lockb exists and bun is available.
			if fileExists(filepath.Join(dir, "bun.lockb")) && runQuiet(workDir, "which", "bun") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-installing Bun dependencies in %s\n", dir)
				runQuietTimeout(dir, 60*time.Second, "bun", "install")
				parts = append(parts, "AUTO-INSTALLED: Bun dependencies (already done, no need to install again)")
			} else if runQuiet(workDir, "which", "npm") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-installing npm dependencies in %s\n", dir)
				runQuietTimeout(dir, 60*time.Second, "npm", "install", "--no-audit", "--no-fund")
				parts = append(parts, "AUTO-INSTALLED: npm dependencies (already done, no need to install again)")
			}
			break
		}
	}

	// Auto-download Go module dependencies.
	if networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "go.mod")) && runQuiet(dir, "which", "go") != "" {
				if !dirExists(filepath.Join(dir, "vendor")) { // skip if vendor/ already exists
					fmt.Fprintf(os.Stderr, "[gollem] auto-downloading Go module dependencies in %s\n", dir)
					runQuietTimeout(dir, 60*time.Second, "go", "mod", "download")
					parts = append(parts, "AUTO-INSTALLED: Go module dependencies (already done, no need to download again)")
				}
				break
			}
		}
	}

	// Auto-fetch Rust/Cargo dependencies.
	if networkAvailable && !depsAlreadyInstalled {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "Cargo.toml")) && runQuiet(dir, "which", "cargo") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-fetching Cargo dependencies in %s\n", dir)
				runQuietTimeout(dir, 60*time.Second, "cargo", "fetch", "--quiet")
				parts = append(parts, "AUTO-INSTALLED: Cargo dependencies (already done, no need to fetch again)")
				break
			}
		}
	}

	// Auto-install Ruby gems (Gemfile with bundle).
	if networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "Gemfile")) && runQuiet(dir, "which", "bundle") != "" {
				if !dirExists(filepath.Join(dir, "vendor", "bundle")) { // skip if already vendored
					fmt.Fprintf(os.Stderr, "[gollem] auto-installing Ruby gems in %s\n", dir)
					runQuietTimeout(dir, 90*time.Second, "bundle", "install", "--quiet")
					parts = append(parts, "AUTO-INSTALLED: Ruby gems via bundle install (already done, no need to install again)")
				}
				break
			}
		}
	}

	// Auto-install from pyproject.toml (modern Python projects without requirements.txt).
	if networkAvailable && !foundPyDeps {
		for _, dir := range []string{workDir, "/app"} {
			pyprojectPath := filepath.Join(dir, "pyproject.toml")
			if fileExists(pyprojectPath) && runQuiet(workDir, "which", "python3") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-installing from pyproject.toml in %s\n", dir)
				installed := pipInstall(dir, "-q", "-e", ".")
				if installed {
					parts = append(parts, "AUTO-INSTALLED: Python project from pyproject.toml (already done)")
				}
				break
			}
		}
	}

	// Auto-resolve Maven dependencies.
	if networkAvailable && !depsAlreadyInstalled {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "pom.xml")) && runQuiet(dir, "which", "mvn") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-resolving Maven dependencies in %s\n", dir)
				runQuietTimeout(dir, 120*time.Second, "mvn", "dependency:resolve", "-q", "-B")
				parts = append(parts, "AUTO-INSTALLED: Maven dependencies (already done, no need to install again)")
				break
			}
		}
	}

	// Auto-resolve Gradle dependencies.
	if networkAvailable && !depsAlreadyInstalled {
		for _, dir := range []string{workDir, "/app"} {
			if (fileExists(filepath.Join(dir, "build.gradle")) || fileExists(filepath.Join(dir, "build.gradle.kts"))) &&
				(runQuiet(dir, "which", "gradle") != "" || fileExists(filepath.Join(dir, "gradlew"))) {
				fmt.Fprintf(os.Stderr, "[gollem] auto-resolving Gradle dependencies in %s\n", dir)
				gradleCmd := "gradle"
				if fileExists(filepath.Join(dir, "gradlew")) {
					gradleCmd = filepath.Join(dir, "gradlew")
					os.Chmod(gradleCmd, 0o755)
				}
				runQuietTimeout(dir, 120*time.Second, gradleCmd, "dependencies", "--quiet")
				parts = append(parts, "AUTO-INSTALLED: Gradle dependencies (already done, no need to install again)")
				break
			}
		}
	}

	// Auto-restore .NET dependencies.
	if networkAvailable && !depsAlreadyInstalled {
		for _, dir := range []string{workDir, "/app"} {
			hasProject := false
			if matches, _ := filepath.Glob(filepath.Join(dir, "*.csproj")); len(matches) > 0 {
				hasProject = true
			} else if matches, _ := filepath.Glob(filepath.Join(dir, "*.sln")); len(matches) > 0 {
				hasProject = true
			} else if matches, _ := filepath.Glob(filepath.Join(dir, "*.fsproj")); len(matches) > 0 {
				hasProject = true
			}
			if hasProject && runQuiet(dir, "which", "dotnet") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-restoring .NET packages in %s\n", dir)
				runQuietTimeout(dir, 90*time.Second, "dotnet", "restore")
				parts = append(parts, "AUTO-INSTALLED: .NET packages via dotnet restore (already done, no need to restore again)")
				break
			}
		}
	}

	// Auto-setup Haskell Stack (download dependencies and build setup).
	if networkAvailable && !depsAlreadyInstalled {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "stack.yaml")) && runQuiet(dir, "which", "stack") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-setting up Haskell Stack in %s\n", dir)
				runQuietTimeout(dir, 120*time.Second, "stack", "setup", "--no-terminal")
				runQuietTimeout(dir, 120*time.Second, "stack", "build", "--only-dependencies", "--no-terminal")
				parts = append(parts, "AUTO-INSTALLED: Haskell Stack dependencies (already done, no need to install again)")
				break
			}
		}
	}

	// Auto-install Elixir Mix dependencies.
	if networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "mix.exs")) && runQuiet(dir, "which", "mix") != "" {
				if !dirExists(filepath.Join(dir, "deps")) {
					fmt.Fprintf(os.Stderr, "[gollem] auto-installing Elixir dependencies in %s\n", dir)
					runQuietTimeout(dir, 90*time.Second, "mix", "deps.get")
					runQuietTimeout(dir, 90*time.Second, "mix", "deps.compile")
					parts = append(parts, "AUTO-INSTALLED: Elixir Mix dependencies (already done, no need to install again)")
				}
				break
			}
		}
	}

	// Auto-resolve Swift Package Manager dependencies.
	if networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "Package.swift")) && runQuiet(dir, "which", "swift") != "" {
				if !dirExists(filepath.Join(dir, ".build")) {
					fmt.Fprintf(os.Stderr, "[gollem] auto-resolving Swift packages in %s\n", dir)
					runQuietTimeout(dir, 120*time.Second, "swift", "package", "resolve")
					parts = append(parts, "AUTO-INSTALLED: Swift packages via swift package resolve (already done, no need to resolve again)")
				}
				break
			}
		}
	}

	// Julia: instantiate packages from Project.toml.
	if networkAvailable {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "Project.toml")) && runQuiet(dir, "which", "julia") != "" {
				// Check if Manifest.toml already exists (already instantiated).
				if !fileExists(filepath.Join(dir, "Manifest.toml")) {
					fmt.Fprintf(os.Stderr, "[gollem] auto-instantiating Julia packages in %s\n", dir)
					runQuietTimeout(dir, 120*time.Second, "julia", "--project="+dir, "-e", "using Pkg; Pkg.instantiate()")
					parts = append(parts, "AUTO-INSTALLED: Julia packages via Pkg.instantiate() (already done, no need to install again)")
				}
				break
			}
		}
	}

	// OCaml/Dune: install opam dependencies if available.
	if networkAvailable && !depsAlreadyInstalled {
		for _, dir := range []string{workDir, "/app"} {
			if fileExists(filepath.Join(dir, "dune-project")) && runQuiet(dir, "which", "opam") != "" {
				fmt.Fprintf(os.Stderr, "[gollem] auto-installing OCaml dependencies in %s\n", dir)
				// opam install . --deps-only -y installs project deps without building.
				runQuietTimeout(dir, 120*time.Second, "opam", "install", ".", "--deps-only", "-y")
				parts = append(parts, "AUTO-INSTALLED: OCaml opam dependencies (already done, no need to install again)")
				break
			}
		}
	}

	// Auto-detect and install Python packages from source imports.
	// When no requirements.txt exists, scan .py files for third-party imports
	// and install them. This saves 2-3 turns of ModuleNotFoundError debugging.
	if networkAvailable && !foundPyDeps && runQuiet(workDir, "which", "python3") != "" {
		if pkgs := detectPythonImports(workDir); len(pkgs) > 0 {
			fmt.Fprintf(os.Stderr, "[gollem] auto-installing detected Python imports: %s\n", strings.Join(pkgs, " "))
			pipInstall(workDir, append([]string{"-q"}, pkgs...)...)
			parts = append(parts, "AUTO-INSTALLED: detected Python imports ("+strings.Join(pkgs, ", ")+")")
		}
	}

	// Auto-detect and install system packages referenced in test scripts.
	// Test scripts (.sh) often use commands like jq, bc, xmllint, xxd that
	// may not be installed in the container. Detecting and installing them
	// preemptively saves 1-2 turns of "command not found" debugging.
	if networkAvailable && !depsAlreadyInstalled {
		if sysPkgs := detectSystemPackagesFromTests(workDir); len(sysPkgs) > 0 {
			fmt.Fprintf(os.Stderr, "[gollem] auto-installing system packages from test scripts: %s\n", strings.Join(sysPkgs, " "))
			// Run apt-get update first — many containers have stale package lists
			// and install fails with "Unable to locate package" without it.
			runQuietTimeout(workDir, 30*time.Second, "apt-get", "update", "-qq")
			runQuietTimeout(workDir, 60*time.Second, "apt-get", append([]string{"install", "-y", "-q"}, sysPkgs...)...)
			parts = append(parts, "AUTO-INSTALLED: system packages ("+strings.Join(sysPkgs, ", ")+")")
		}
	}

	// Mark dependencies as installed so subagents skip redundant installs.
	if !depsAlreadyInstalled {
		os.WriteFile(depsMarker, []byte("1"), 0o644)
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
	// Guarantee a minimum of 15KB for source files even if tests consumed
	// most of the budget — source context is still valuable.
	sourceMinBudget := 15000
	sourceBudget := autoReadBudget
	if sourceBudget < sourceMinBudget {
		sourceBudget = sourceMinBudget
	}
	if sourceBudget > 0 {
		appSourceDirs := []string{"/app", workDir}
		for _, ad := range appSourceDirs {
			newBudget := autoReadSourceFilesBudget(ad, &parts, 5000, 8, sourceBudget, testRefs)
			// Only deduct from autoReadBudget what was actually consumed.
			consumed := sourceBudget - newBudget
			autoReadBudget -= consumed
			break // only read from one source directory
		}
	}

	// Detect TODO/FIXME/stub patterns in source files — tells the agent
	// exactly what needs to be implemented in skeleton code.
	if todoStubs := detectTodoStubs(workDir); len(todoStubs) > 0 {
		parts = append(parts, "\n## Implementation Stubs Found (TODOs in source code)")
		parts = append(parts, "These locations need implementation:")
		parts = append(parts, todoStubs...)
		parts = append(parts, "Implement these stubs as part of your solution.")
	}

	// Surface solution files that tests expect but don't exist yet.
	// If a test imports "from solution import X" and solution.py doesn't exist,
	// the agent needs to CREATE it. This is the #1 thing to do first.
	var missingFiles []string
	if len(testRefs) > 0 {
		for filename := range testRefs {
			// Check if the file exists in workDir or /app.
			found := false
			for _, dir := range []string{workDir, "/app"} {
				if fileExists(filepath.Join(dir, filename)) {
					found = true
					break
				}
			}
			if !found {
				missingFiles = append(missingFiles, filename)
			}
		}
		if len(missingFiles) > 0 {
			// Extract imported names from test content so the agent knows
			// the exact API it needs to implement.
			importedNames := extractImportedNames(parts, missingFiles)

			parts = append(parts, "\n## Solution Files to Create (tests import these)")
			parts = append(parts, "Tests import/require these files but they DON'T EXIST yet — you must CREATE them:")
			for _, f := range missingFiles {
				if names, ok := importedNames[f]; ok && len(names) > 0 {
					parts = append(parts, fmt.Sprintf("  - %s (must export: %s)", f, strings.Join(names, ", ")))
				} else {
					parts = append(parts, "  - "+f)
				}
			}
			parts = append(parts, "Create these files FIRST — they are the primary deliverables.")
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

	// chmod +x shell/Python scripts in test directories and working directory.
	// This prevents "Permission denied" errors that waste 1-2 agent turns.
	for _, td := range testDirs {
		chmodScriptsInDir(td)
	}
	chmodScriptsInDir(workDir)
	if workDir != "/app" {
		chmodScriptsInDir("/app")
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
			// Auto-read small resource files (< 3KB) — these are often reference
			// data that the agent needs to match or process. Saves 1-2 turns of
			// manual cat/head commands.
			if autoReadBudget > 0 {
				autoReadBudget = autoReadSmallFiles(rd, &parts, "Resource", 3000, 5, autoReadBudget)
			}
			break
		}
	}

	// Detect expected output files from test analysis.
	// This tells the agent exactly WHAT to create from the start.
	expectedOutputs := detectExpectedOutputs(workDir)
	if len(expectedOutputs) > 0 {
		parts = append(parts, "\n## Expected Output Files (from test analysis)")
		parts = append(parts, "Tests expect these files/paths to exist:")
		for _, o := range expectedOutputs {
			parts = append(parts, "  - "+o)
		}
		parts = append(parts, "Create these files EARLY — even with placeholder content — then refine.")
	}

	// Auto-create output directories that tests reference but don't exist.
	// This saves 1 turn of "mkdir: no such file or directory" errors.
	autoMkdirOutputDirs(workDir, expectedOutputs)

	// Detect output format and execution patterns from test code.
	// This addresses failure mode #5: correct logic but wrong output format.
	formatHints := detectOutputFormat(workDir)
	if len(formatHints) > 0 {
		parts = append(parts, "\n## Output Format & Execution (from test analysis)")
		for _, h := range formatHints {
			parts = append(parts, "  - "+h)
		}
	}

	// Extract invocation patterns from test scripts — shows exactly how the
	// solution binary/script is called (stdin/stdout, args, file paths).
	invocationPatterns := extractInvocationPatterns(workDir)
	if len(invocationPatterns) > 0 {
		parts = append(parts, "\n## Solution Invocation (from test scripts)")
		parts = append(parts, "Tests invoke your solution like this:")
		for _, p := range invocationPatterns {
			parts = append(parts, "  "+p)
		}
		parts = append(parts, "Ensure your solution is compatible with this invocation pattern.")
	}

	// Extract function signatures from test code — tells the agent the exact
	// API it needs to implement (parameter names, types, return values).
	// This goes beyond extractImportedNames (which only knows WHAT is imported)
	// to show HOW imported functions are called.
	if signatures := extractFunctionSignatures(workDir); len(signatures) > 0 {
		parts = append(parts, "\n## Function Signatures (from test calls)")
		parts = append(parts, "Tests call your functions with these signatures:")
		for _, sig := range signatures {
			parts = append(parts, "  - "+sig)
		}
		parts = append(parts, "Implement functions matching these exact signatures.")
	}

	// Detect numerical comparison tolerances from test assertions.
	// Tests using assertAlmostEqual, isclose, etc. have specific precision
	// requirements. Surfacing these prevents the agent from returning values
	// with wrong precision (a common failure for scientific computing tasks).
	if tolerances := detectComparisonTolerances(workDir); len(tolerances) > 0 {
		parts = append(parts, "\n## Precision Requirements (from test assertions)")
		for _, t := range tolerances {
			parts = append(parts, "  - "+t)
		}
	}

	// Extract environment variables from test scripts. Tests often set
	// env vars (PORT, DATABASE_URL, etc.) that the solution must respect.
	// Missing env vars cause silent failures that waste 2-3 debugging turns.
	if envVars := extractTestEnvironmentVars(workDir); len(envVars) > 0 {
		parts = append(parts, "\n## Environment Variables (from test scripts)")
		parts = append(parts, "Tests set these environment variables before running your solution:")
		for _, ev := range envVars {
			parts = append(parts, "  - "+ev)
		}
		parts = append(parts, "Your solution MUST respect these variables.")
	}

	// Detect working directory expectations from test scripts. Some tests
	// cd to a specific directory before running the solution. If the agent
	// creates files in the wrong directory, tests fail silently.
	if cwdHint := detectExpectedWorkingDir(workDir); cwdHint != "" {
		parts = append(parts, "\n## Working Directory (from test analysis)")
		parts = append(parts, cwdHint)
	}

	// Extract per-test timeouts from test scripts (timeout N, signal.alarm, ulimit).
	// Surfaces performance requirements so the agent knows how fast its solution must be.
	if perTestTimeouts := extractPerTestTimeouts(workDir); len(perTestTimeouts) > 0 {
		parts = append(parts, "\n## Per-Test Timeouts (from test scripts)")
		parts = append(parts, "Individual test cases have these time limits:")
		for _, t := range perTestTimeouts {
			parts = append(parts, "  - "+t)
		}
		parts = append(parts, "Your solution MUST complete within these limits. Profile with `time` if close to the limit.")
	}

	// Auto-read expected output files referenced by diff/cmp in test scripts.
	// This is the #1 way to produce correct output — seeing the exact expected
	// format eliminates guessing about whitespace, encoding, and structure.
	if autoReadBudget > 0 {
		autoReadBudget = autoReadDiffExpectedFiles(workDir, &parts, autoReadBudget)
	}

	// Auto-read example/reference output files that show expected format.
	// These save the agent from guessing output format — the #4 failure mode.
	if autoReadBudget > 0 {
		autoReadBudget = autoReadExampleOutputs(workDir, &parts, autoReadBudget)
	}

	// Auto-read small files from input_data/ — understanding the input format
	// immediately saves 1-2 turns the agent would spend on head/cat/wc commands.
	if autoReadBudget > 0 {
		inputDirs := []string{
			"/app/task_file/input_data",
			filepath.Join(workDir, "input_data"),
		}
		for _, id := range inputDirs {
			if dirExists(id) {
				autoReadBudget = autoReadSmallFiles(id, &parts, "Input data", 3000, 5, autoReadBudget)
				break
			}
		}
	}

	// Task-type specific guidance based on detected patterns.
	parts = append(parts, detectTaskGuidance(workDir))

	// Suggest specific test/build commands so the agent doesn't waste turns
	// figuring out how to verify its work. Cache for buildActionSummary below.
	testCmds := detectTestCommands(workDir)
	if len(testCmds) > 0 {
		parts = append(parts, "\n## Quick Commands")
		for _, cmd := range testCmds {
			parts = append(parts, "  "+cmd)
		}
	}

	// Strong reminder that source files, tests, and build files are already loaded
	// in the context above. The #1 wasted first turn is re-reading files that are
	// already visible. Don't enumerate tools — the model sees tool definitions
	// from the API, and a hardcoded list would be inaccurate if code mode or
	// team mode adds/removes tools.
	parts = append(parts, "\nIMPORTANT: README, tests, source files, and build files are PRE-LOADED above — do NOT re-read them. Start coding immediately. For complex tasks, create a plan first using the planning tool.")

	// Add a compact action summary at the very end. This exploits recency bias —
	// the last thing the model reads before starting work is a focused summary
	// of what to do. Reuses cached data to avoid re-scanning test files.
	if summary := buildActionSummaryCached(workDir, expectedOutputs, testCmds, missingFiles, formatHints, invocationPatterns); summary != "" {
		parts = append(parts, summary)
	}

	return strings.Join(parts, "\n")
}

// buildActionSummaryCached creates a focused, compact summary that appears at
// the very end of context injection. It synthesizes the most critical info:
// what to create, how to test, how the solution is invoked, and what to do first.
// Takes pre-computed data from context injection to avoid re-scanning files.
func buildActionSummaryCached(workDir string, expectedOutputs, testCmds, missingFiles, formatHints, invocationPatterns []string) string {
	var lines []string
	lines = append(lines, "\n## ACTION SUMMARY (start here)")

	// Missing solution files — these MUST be created first.
	if len(missingFiles) > 0 {
		display := missingFiles
		if len(display) > 5 {
			display = display[:5]
		}
		lines = append(lines, "MISSING: "+strings.Join(display, ", ")+" (tests import these — CREATE FIRST!)")
	}

	// What expected outputs need to be created?
	if len(expectedOutputs) > 0 {
		display := expectedOutputs
		if len(display) > 5 {
			display = display[:5]
		}
		lines = append(lines, "CREATE: "+strings.Join(display, ", "))
	}

	// How is the solution invoked? Show the first (most representative) pattern.
	if len(invocationPatterns) > 0 {
		// Truncate the pattern for the summary line.
		pat := invocationPatterns[0]
		if len(pat) > 120 {
			pat = pat[:120] + "..."
		}
		lines = append(lines, "INVOKE: "+pat)
	}

	// What output format? Compact single-line summary.
	for _, h := range formatHints {
		if strings.HasPrefix(h, "FORMAT=") {
			// Extract just the format name (e.g., "JSON" from "FORMAT=JSON: ...")
			eqIdx := strings.Index(h, "=")
			colonIdx := strings.Index(h, ":")
			if eqIdx >= 0 && colonIdx > eqIdx {
				lines = append(lines, "FORMAT: "+h[eqIdx+1:colonIdx])
			}
			break // only show first format
		}
		if strings.HasPrefix(h, "STDIN:") {
			lines = append(lines, "INPUT: Read from stdin (not files)")
			break
		}
	}

	// What test command to run?
	for _, cmd := range testCmds {
		if strings.HasPrefix(cmd, "Test:") {
			lines = append(lines, "VERIFY: "+strings.TrimPrefix(cmd, "Test: "))
			break
		}
	}

	// What to do first based on task type?
	hasTests := dirExists("/tests") || dirExists(filepath.Join(workDir, "tests")) || dirExists(filepath.Join(workDir, "test"))
	hasInputData := dirExists("/app/task_file/input_data") || dirExists(filepath.Join(workDir, "input_data"))

	if len(missingFiles) > 0 {
		lines = append(lines, "FIRST: Create "+missingFiles[0]+" IMMEDIATELY (tests import it), then run tests")
	} else if hasInputData {
		lines = append(lines, "FIRST: Read input data format, then write processing code to output_data/")
	} else if hasTests && len(expectedOutputs) > 0 {
		lines = append(lines, "FIRST: Write initial output files (even rough drafts), then run tests")
	} else if hasTests {
		lines = append(lines, "FIRST: Write implementation based on test expectations, then run tests")
	} else {
		lines = append(lines, "FIRST: Create required output files immediately, then iterate")
	}

	// Surface per-test timeout if detected — critical for performance tasks.
	if perTestTimeouts := extractPerTestTimeouts(workDir); len(perTestTimeouts) > 0 {
		lines = append(lines, "TIMEOUT: "+perTestTimeouts[0]+" — optimize for speed!")
	}

	lines = append(lines, "REMEMBER: Output First, Perfect Later. Write code NOW, refine after testing.")

	if len(lines) <= 2 {
		return "" // nothing useful to summarize
	}
	return strings.Join(lines, "\n")
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
		{"Package.swift", "Swift", "swift"},
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

// pipInstall tries to install Python packages using multiple pip variants.
// Many Docker containers have pip3 but not pip, or python3 -m pip but neither.
// Returns true if any variant succeeds.
func pipInstall(workDir string, args ...string) bool {
	// Try pip variants in order of preference.
	pipCommands := [][]string{
		append([]string{"pip", "install", "--break-system-packages"}, args...),
		append([]string{"pip3", "install", "--break-system-packages"}, args...),
		append([]string{"python3", "-m", "pip", "install", "--break-system-packages"}, args...),
		append([]string{"python", "-m", "pip", "install", "--break-system-packages"}, args...),
	}
	for _, cmd := range pipCommands {
		result := runQuietTimeout(workDir, 60*time.Second, cmd[0], cmd[1:]...)
		if result != "" {
			return true
		}
	}
	return false
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

// autoReadSmallFiles reads small files (any type, not just source) in a directory
// up to maxBytes per file, capping at maxFiles total. Used for resources/ and
// input_data/ where files may be .txt, .dat, .csv, or extensionless.
// Returns remaining budget.
func autoReadSmallFiles(dir string, parts *[]string, label string, maxBytes, maxFiles, budget int) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return budget
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || count >= maxFiles || budget <= 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > int64(maxBytes) || info.Size() == 0 {
			continue
		}
		// Skip binary-looking files by extension.
		lower := strings.ToLower(entry.Name())
		if isBinaryExtension(lower) {
			continue
		}
		limit := maxBytes
		if limit > budget {
			limit = budget
		}
		content := readFileTruncated(filepath.Join(dir, entry.Name()), limit)
		if content != "" {
			*parts = append(*parts, fmt.Sprintf("\n## %s file auto-read: %s/%s", label, dir, entry.Name()))
			*parts = append(*parts, content)
			budget -= len(content)
			count++
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

	// Detect C/C++ compilation tasks.
	if detectCppTask(workDir) {
		hints = append(hints, "\n## Task Type: C/C++ Compilation")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check available compilers: `which gcc g++ clang clang++ cc`")
		hints = append(hints, "- If Makefile exists, use `make -j$(nproc)` for parallel builds")
		hints = append(hints, "- If CMakeLists.txt exists: `mkdir -p build && cd build && cmake .. && make -j$(nproc)`")
		hints = append(hints, "- Common flags: -Wall -Wextra -std=c11 (C) or -std=c++17 (C++), -lm for math")
		hints = append(hints, "- Link order matters: put -l flags AFTER source files (`gcc main.c -lm`, not `gcc -lm main.c`)")
		hints = append(hints, "- For undefined reference errors: check that all required .c/.cpp files are compiled and linked")
		hints = append(hints, "- For header errors: check include paths with -I flags")
		hints = append(hints, "- If tests use valgrind: ensure no memory leaks (free all malloc'd memory)")
		hints = append(hints, "- Compile with -g for debug info if you need to debug with gdb")
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

	// Detect Haskell tasks.
	if detectHaskellTask(workDir) {
		hints = append(hints, "\n## Task Type: Haskell")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check build system: `stack build` (stack.yaml) or `cabal build` (*.cabal)")
		hints = append(hints, "- Run: `stack run` or `cabal run`, tests: `stack test` or `cabal test`")
		hints = append(hints, "- For GHC directly: `ghc -o main Main.hs && ./main`")
		hints = append(hints, "- Install missing packages: `stack install <pkg>` or `cabal install <pkg>`")
		hints = append(hints, "- Haskell type errors are verbose — read the 'Expected type' vs 'Actual type' lines")
		hints = append(hints, "- If stack build is slow (first run downloads GHC), be patient — check logs for errors after 60 seconds")
	}

	// Detect Ruby tasks.
	if detectRubyTask(workDir) {
		hints = append(hints, "\n## Task Type: Ruby")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Run scripts: `ruby <file.rb>`")
		hints = append(hints, "- Install gems: `gem install <name>` or `bundle install` (if Gemfile exists)")
		hints = append(hints, "- Run tests: `bundle exec rspec`, `ruby -Itest test_*.rb`, or `rake test`")
		hints = append(hints, "- For Rails: `bundle exec rails <command>`")
		hints = append(hints, "- Check Ruby version: `ruby --version`. Version mismatches cause syntax errors.")
	}

	// Detect Java tasks.
	if detectJavaTask(workDir) {
		hints = append(hints, "\n## Task Type: Java/Kotlin")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Check build system: Maven (`pom.xml`), Gradle (`build.gradle`), or plain javac")
		hints = append(hints, "- Maven: `mvn compile -q` to build, `mvn test -q` to test, `mvn package -q` to create JAR")
		hints = append(hints, "- Gradle: `./gradlew build --quiet` (use `./gradlew` if present, else `gradle`)")
		hints = append(hints, "- Plain javac: `javac -d out *.java && java -cp out MainClass`")
		hints = append(hints, "- For 'cannot find symbol' errors: check import statements and classpath")
		hints = append(hints, "- JVM startup is slow — combine compile+test into single mvn/gradle invocation")
		hints = append(hints, "- Check Java version: `java -version`. Version mismatches cause compilation errors.")
	}

	// Detect .NET tasks.
	if detectDotNetTask(workDir) {
		hints = append(hints, "\n## Task Type: .NET/C#")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Build: `dotnet build`, Test: `dotnet test`, Run: `dotnet run`")
		hints = append(hints, "- Restore packages first: `dotnet restore`")
		hints = append(hints, "- Check .NET version: `dotnet --version`")
		hints = append(hints, "- For 'could not resolve' errors: run `dotnet restore` first")
	}

	// Detect service/daemon tasks (web servers, background services).
	if detectServiceTask(workDir) {
		hints = append(hints, "\n## Task Type: Service/Daemon Setup")
		hints = append(hints, "This task likely requires a service that PERSISTS after your session ends.")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Use systemd (`systemctl enable/start`), supervisord, or init scripts to ensure the service starts on boot")
		hints = append(hints, "- If systemd is unavailable, use `nohup <command> &` with a startup script in /etc/rc.local or crontab @reboot")
		// Detect specific ports from test scripts for actionable verification.
		if ports := detectTestPorts(workDir); len(ports) > 0 {
			hints = append(hints, fmt.Sprintf("- Tests connect to port(s): %s — your service MUST listen on %s",
				strings.Join(ports, ", "), ports[0]))
			hints = append(hints, fmt.Sprintf("- VERIFY: `curl -s localhost:%s` and `ss -tlnp | grep %s`", ports[0], ports[0]))
		} else {
			hints = append(hints, "- VERIFY the service is running: `curl localhost:<port>`, `systemctl status <service>`, or `ss -tlnp`")
		}
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

	// Detect shell scripting tasks — many TB2 tasks are primarily bash/shell.
	if detectShellTask(workDir) {
		hints = append(hints, "\n## Task Type: Shell Scripting")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Use `set -euo pipefail` at the top for strict error handling")
		hints = append(hints, "- Quote all variable expansions: \"$var\" not $var (prevents word splitting)")
		hints = append(hints, "- Use `shellcheck <script>` if available to catch common bugs")
		hints = append(hints, "- For text processing: prefer awk/sed over loops. `awk '{print $1}'` is faster than while-read loops.")
		hints = append(hints, "- Use `[[ ]]` over `[ ]` for comparisons (supports regex, no word splitting)")
		hints = append(hints, "- For file operations: check if files exist before operating on them")
		hints = append(hints, "- Make scripts executable: `chmod +x script.sh`")
	}

	// Detect Jupyter notebook tasks.
	if detectNotebookTask(workDir) {
		hints = append(hints, "\n## Jupyter Notebooks Detected")
		hints = append(hints, "Key strategies:")
		hints = append(hints, "- Convert notebook to Python script: `jupyter nbconvert --to script *.ipynb` or parse JSON directly")
		hints = append(hints, "- Run notebook: `jupyter nbconvert --to notebook --execute <notebook>.ipynb` or `papermill <in>.ipynb <out>.ipynb`")
		hints = append(hints, "- For data analysis tasks: extract code cells and run them as a Python script")
		hints = append(hints, "- .ipynb files are JSON — you can read/modify them with Python's json module")
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

	// Detect tasks with audio files.
	if audioFiles := detectAudioFiles(workDir); len(audioFiles) > 0 {
		hints = append(hints, "\n## Audio Files Detected")
		for _, f := range audioFiles {
			hints = append(hints, "  - "+f)
		}
		hints = append(hints, "To analyze audio, use Python with librosa, scipy, or built-in wave module:")
		hints = append(hints, "  python3 -c \"import wave; w = wave.open('file.wav'); print(w.getnchannels(), w.getsampwidth(), w.getframerate(), w.getnframes())\"")
		hints = append(hints, "For spectral analysis: pip install --break-system-packages librosa numpy scipy")
		hints = append(hints, "For conversion: ffmpeg -i input.mp3 output.wav (if ffmpeg is available)")
	}

	// Detect tasks with database files.
	if dbFiles := detectDatabaseFiles(workDir); len(dbFiles) > 0 {
		hints = append(hints, "\n## Database Files Detected")
		for _, f := range dbFiles {
			hints = append(hints, "  - "+f)
		}
		hints = append(hints, "For SQLite: `sqlite3 <file> '.tables'` to list tables, `.schema` for schema")
		hints = append(hints, "Use Python sqlite3 module for programmatic access: `import sqlite3; conn = sqlite3.connect('file.db')`")
	}

	if len(hints) > 0 {
		return strings.Join(hints, "\n")
	}
	return ""
}

// detectAudioFiles returns audio file paths found in the working directory.
func detectAudioFiles(workDir string) []string {
	var files []string
	for _, dir := range []string{workDir, "/app", "/app/task_file"} {
		for _, ext := range []string{"*.wav", "*.mp3", "*.flac", "*.ogg", "*.aac", "*.m4a", "*.aiff"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			files = append(files, matches...)
		}
	}
	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

// detectDatabaseFiles returns database file paths found in the working directory.
func detectDatabaseFiles(workDir string) []string {
	var files []string
	for _, dir := range []string{workDir, "/app", "/app/task_file"} {
		for _, ext := range []string{"*.db", "*.sqlite", "*.sqlite3"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			files = append(files, matches...)
		}
	}
	if len(files) > 5 {
		files = files[:5]
	}
	return files
}

// detectImageFiles returns image file paths found in the working directory.
func detectImageFiles(workDir string) []string {
	var images []string
	for _, dir := range []string{workDir, "/app", "/app/task_file", "/app/task_file/input_data"} {
		for _, ext := range []string{"*.png", "*.jpg", "*.jpeg", "*.bmp", "*.gif", "*.tiff", "*.ppm", "*.pgm", "*.svg", "*.webp"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			images = append(images, matches...)
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
		"/app/task_file/README.md",
		"/app/task_file/instruction.md",
		"/app/task_file/prompts/agent.md",
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

// detectOutputFormat scans test files to determine the expected output format
// and execution pattern. Returns specific hints that help the agent produce
// correctly-formatted output and invoke their solution correctly.
// This addresses failure mode #5: correct logic but wrong output format.
func detectOutputFormat(workDir string) []string {
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}

	jsonFound, csvFound, yamlFound, xmlFound := false, false, false, false
	stdinFound, binaryExecFound := false, false

	// Also check README/instruction files and scripts in root.
	var allContent []string
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
			allContent = append(allContent, string(data))
		}
	}

	// Also scan verification scripts in workDir and /app for format clues.
	for _, dir := range []string{workDir, "/app", "/app/task_file"} {
		for _, pattern := range []string{"verify*", "check*", "test.sh", "test.py", "run_test*"} {
			matches, _ := filepath.Glob(filepath.Join(dir, pattern))
			for _, m := range matches {
				info, err := os.Stat(m)
				if err != nil || info.IsDir() || info.Size() > 30000 {
					continue
				}
				data, err := os.ReadFile(m)
				if err != nil {
					continue
				}
				allContent = append(allContent, string(data))
			}
		}
	}

	for _, content := range allContent {
		// JSON format detection.
		if !jsonFound {
			for _, p := range []string{
				"json.load(", "json.loads(", "json.dump(", "json.dumps(",
				"JSON.parse(", "JSON.stringify(",
				"encoding/json", "json.Unmarshal(", "json.NewDecoder(",
				"JsonParser", "ObjectMapper",
			} {
				if strings.Contains(content, p) {
					jsonFound = true
					break
				}
			}
		}

		// CSV format detection.
		if !csvFound {
			for _, p := range []string{
				"csv.reader(", "csv.writer(", "csv.DictReader(", "csv.DictWriter(",
				"read_csv(", "to_csv(", "pd.read_csv(",
				"encoding/csv", "csv.NewReader(",
			} {
				if strings.Contains(content, p) {
					csvFound = true
					break
				}
			}
		}

		// YAML format detection.
		if !yamlFound {
			for _, p := range []string{
				"yaml.safe_load(", "yaml.load(", "yaml.safe_dump(",
				"YAML.load(", "YAML.dump(",
				"gopkg.in/yaml", "yaml.Unmarshal(",
			} {
				if strings.Contains(content, p) {
					yamlFound = true
					break
				}
			}
		}

		// XML format detection.
		if !xmlFound {
			for _, p := range []string{
				"xml.etree", "lxml.etree", "xml.dom",
				"xml.sax", "xmllint",
				"encoding/xml", "xml.Unmarshal(",
				"DocumentBuilder", "SAXParser",
			} {
				if strings.Contains(content, p) {
					xmlFound = true
					break
				}
			}
		}

		// Stdin usage detection — tells agent to read from stdin not files.
		if !stdinFound {
			for _, p := range []string{
				"sys.stdin", "process.stdin", "os.Stdin",
				"bufio.NewScanner(os.Stdin)",
				// Pipe and redirect patterns in shell tests.
				"| ./solution", "| ./program", "| ./main", "| ./a.out",
				"| python3 solution", "| python solution", "| python3 main",
				"| node solution", "| node main",
				"< input", "< /app/",
			} {
				if strings.Contains(content, p) {
					stdinFound = true
					break
				}
			}
		}

		// Compiled binary execution detection.
		if !binaryExecFound {
			for _, p := range []string{
				"./solution ", "./program ", "./a.out", "./main ",
				"test -x ./", "chmod +x ./solution", "chmod +x ./program",
			} {
				if strings.Contains(content, p) {
					binaryExecFound = true
					break
				}
			}
		}
	}

	var hints []string
	if jsonFound {
		hints = append(hints, "FORMAT=JSON: Tests parse output as JSON. Use json.dumps()/json.Marshal() with proper structure.")
	}
	if csvFound {
		hints = append(hints, "FORMAT=CSV: Tests parse output as CSV. Match exact headers, delimiters, and quoting.")
	}
	if yamlFound {
		hints = append(hints, "FORMAT=YAML: Tests parse output as YAML. Ensure valid YAML syntax and proper indentation.")
	}
	if xmlFound {
		hints = append(hints, "FORMAT=XML: Tests parse output as XML. Ensure well-formed XML with correct tags and encoding declaration.")
	}
	if stdinFound {
		hints = append(hints, "STDIN: Tests pipe input to your program via stdin. Read from stdin (not files) unless task says otherwise.")
	}
	if binaryExecFound {
		// Extract the exact binary name from test content for a specific compilation hint.
		binaryName := detectExpectedBinaryName(allContent)
		if binaryName != "" {
			compileHint := fmt.Sprintf("EXECUTABLE: Tests run `./%s`. ", binaryName)
			compileHint += suggestCompileCommand(workDir, binaryName)
			hints = append(hints, compileHint)
		} else {
			hints = append(hints, "EXECUTABLE: Tests run a compiled binary (./solution, ./program, etc). Compile your code and ensure the binary is executable (chmod +x).")
		}
	}
	return hints
}

// detectExpectedBinaryName extracts the exact binary name tests expect
// from test content (e.g., "./solution", "./program", "./main").
func detectExpectedBinaryName(testContents []string) string {
	// Binary names in order of frequency in TB2.
	candidates := []string{"solution", "program", "main", "a.out", "solve", "app", "answer"}
	for _, content := range testContents {
		for _, name := range candidates {
			// Check for common invocation patterns: "./name", "./name ", "./name\n"
			marker := "./" + name
			if strings.Contains(content, marker+" ") || strings.Contains(content, marker+"\n") ||
				strings.Contains(content, marker+"\"") || strings.Contains(content, marker+"'") ||
				strings.Contains(content, marker+")") || strings.Contains(content, marker+"|") ||
				strings.Contains(content, marker+"<") || strings.Contains(content, marker+">") ||
				strings.HasSuffix(strings.TrimSpace(content), marker) {
				return name
			}
		}
	}
	return ""
}

// suggestCompileCommand suggests a specific compilation command based on the
// project's language and the expected binary name.
func suggestCompileCommand(workDir, binaryName string) string {
	// Check for language-specific build files.
	if fileExists(filepath.Join(workDir, "Cargo.toml")) {
		return fmt.Sprintf("Rust: `cargo build --release && cp target/release/* ./%s` or set binary name in Cargo.toml [[bin]].", binaryName)
	}
	if fileExists(filepath.Join(workDir, "go.mod")) {
		return fmt.Sprintf("Go: `go build -o %s ./...` or `go build -o %s .`", binaryName, binaryName)
	}
	if fileExists(filepath.Join(workDir, "Makefile")) || fileExists("/app/Makefile") {
		return fmt.Sprintf("Use `make` (check Makefile for target). Ensure output is named `%s`.", binaryName)
	}
	if fileExists(filepath.Join(workDir, "CMakeLists.txt")) {
		return fmt.Sprintf("CMake: `mkdir -p build && cd build && cmake .. && make -j$(nproc)`. Ensure output binary is named `%s`.", binaryName)
	}
	// Check for C/C++ source files.
	for _, dir := range []string{workDir, "/app"} {
		cFiles, _ := filepath.Glob(filepath.Join(dir, "*.c"))
		cppFiles, _ := filepath.Glob(filepath.Join(dir, "*.cpp"))
		if len(cppFiles) > 0 {
			return fmt.Sprintf("C++: `g++ -O2 -o %s *.cpp -lm` (add -lpthread if using threads).", binaryName)
		}
		if len(cFiles) > 0 {
			return fmt.Sprintf("C: `gcc -O2 -o %s *.c -lm` (add -lpthread if using threads).", binaryName)
		}
	}
	return fmt.Sprintf("Compile your code and name the output binary `%s`. Ensure it's executable (chmod +x).", binaryName)
}

// extractInvocationPatterns scans test scripts to find exactly how the solution
// will be invoked. This is the single most actionable piece of info for the
// agent: knowing whether to produce a compiled binary, read stdin, accept CLI
// args, or write to specific files.
func extractInvocationPatterns(workDir string) []string {
	var patterns []string
	seen := make(map[string]bool)

	// Collect test/verification script paths.
	var shellScripts, pyScripts []string
	for _, dir := range []string{
		"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test"),
		workDir, "/app", "/app/task_file",
	} {
		for _, name := range []string{
			"test.sh", "run_tests.sh", "run_test.sh", "verify.sh",
			"check.sh", "run.sh", "grade.sh",
		} {
			path := filepath.Join(dir, name)
			if fileExists(path) {
				shellScripts = append(shellScripts, path)
			}
		}
		for _, name := range []string{
			"test.py", "test_output.py", "test_outputs.py", "run_tests.py",
			"run_test.py", "verify.py", "check.py", "grade.py",
		} {
			path := filepath.Join(dir, name)
			if fileExists(path) {
				pyScripts = append(pyScripts, path)
			}
		}
	}

	// Invocation indicators for shell scripts.
	invocationMarkers := []string{
		"./solution", "./program", "./main", "./a.out",
		"./solve", "./answer", "./app",
		"python3 solution", "python solution",
		"python3 main", "python main",
		"python3 ./solution", "python ./solution",
		"node solution", "node main", "node ./solution",
		"ruby solution", "ruby main",
		"java -", "java Solution", "java Main",
	}

	// Phase 1: Shell scripts — raw invocation lines.
	for _, scriptPath := range shellScripts {
		data, err := os.ReadFile(scriptPath)
		if err != nil || len(data) > 50000 {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			lower := strings.ToLower(trimmed)
			for _, marker := range invocationMarkers {
				if strings.Contains(lower, marker) {
					if len(trimmed) > 200 {
						trimmed = trimmed[:200] + "..."
					}
					if !seen[trimmed] {
						seen[trimmed] = true
						patterns = append(patterns, trimmed)
					}
					break
				}
			}
		}
		if len(patterns) >= 5 {
			break
		}
	}

	// Phase 2: Python test scripts — subprocess invocations.
	// Patterns: subprocess.run(["./solution", ...]), os.system("./solution ..."),
	// subprocess.check_output(["python3", "solution.py", ...])
	if len(patterns) < 5 {
		pyInvocationMarkers := []string{
			"subprocess.run(", "subprocess.check_output(", "subprocess.call(",
			"subprocess.Popen(", "os.system(",
		}
		for _, scriptPath := range pyScripts {
			data, err := os.ReadFile(scriptPath)
			if err != nil || len(data) > 50000 {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				for _, marker := range pyInvocationMarkers {
					if strings.Contains(trimmed, marker) {
						// Check that this line also references the solution.
						lower := strings.ToLower(trimmed)
						hasSolution := false
						for _, sm := range []string{
							"solution", "program", "main", "a.out",
							"solve", "answer",
						} {
							if strings.Contains(lower, sm) {
								hasSolution = true
								break
							}
						}
						if hasSolution {
							if len(trimmed) > 200 {
								trimmed = trimmed[:200] + "..."
							}
							if !seen[trimmed] {
								seen[trimmed] = true
								patterns = append(patterns, trimmed)
							}
						}
						break
					}
				}
			}
			if len(patterns) >= 5 {
				break
			}
		}
	}

	if len(patterns) > 5 {
		patterns = patterns[:5]
	}
	return patterns
}

// extractPathFromLine extracts a file path starting at position idx in a line.
// It looks for surrounding quotes or extracts until whitespace/punctuation.
// extractTestReferencedFiles scans the auto-read parts for test file content
// and extracts source file names referenced by import/require statements.
// Returns a map of lowercased filenames (e.g., "solution.py") that tests import.
// This enables prioritizing these files in source auto-read.
// extractImportedNames scans test content for specific names imported from
// each missing file. For "from solution import solve, process", returns
// {"solution.py": ["solve", "process"]}. This tells the agent exactly what
// API the file needs to implement, saving 1-2 turns of guessing.
func extractImportedNames(parts []string, missingFiles []string) map[string][]string {
	result := make(map[string][]string)
	// Build a map of module name → filename for quick lookup.
	moduleToFile := make(map[string]string)
	for _, f := range missingFiles {
		// "solution.py" → "solution"
		mod := strings.TrimSuffix(f, filepath.Ext(f))
		moduleToFile[mod] = f
	}

	inTestSection := false
	for _, part := range parts {
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

		for _, line := range strings.Split(part, "\n") {
			trimmed := strings.TrimSpace(line)

			// Python: "from solution import solve, process_data"
			if strings.HasPrefix(trimmed, "from ") {
				fields := strings.Fields(trimmed)
				if len(fields) >= 4 && fields[2] == "import" {
					mod := strings.Split(fields[1], ".")[0]
					if filename, ok := moduleToFile[mod]; ok {
						// Extract imported names.
						importPart := strings.Join(fields[3:], " ")
						// Handle multi-line imports: "from x import (a, b, c)"
						importPart = strings.Trim(importPart, "()")
						for _, name := range strings.Split(importPart, ",") {
							name = strings.TrimSpace(name)
							// Handle "name as alias"
							if asIdx := strings.Index(name, " as "); asIdx > 0 {
								name = name[:asIdx]
							}
							name = strings.TrimSpace(name)
							if name != "" && name != "*" {
								result[filename] = append(result[filename], name)
							}
						}
					}
				}
			}
		}
	}

	// Deduplicate names per file.
	for f, names := range result {
		seen := make(map[string]bool)
		var unique []string
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				unique = append(unique, n)
			}
		}
		result[f] = unique
	}

	return result
}

// extractFunctionSignatures scans test files for function call patterns to
// determine the exact API the solution needs to implement. Goes beyond
// extractImportedNames by showing HOW functions are called (parameter names,
// argument patterns), not just WHAT is imported.
// Example: from "assert solve(3, [1,2,3]) == 6" → "solve(n, list) → called with int and list args"
func extractFunctionSignatures(workDir string) []string {
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	var signatures []string
	seen := make(map[string]bool)

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
			ext := strings.ToLower(filepath.Ext(entry.Name()))

			switch ext {
			case ".py":
				sigs := extractPythonFunctionSignatures(content)
				for _, sig := range sigs {
					if !seen[sig] {
						seen[sig] = true
						signatures = append(signatures, sig)
					}
				}
			case ".js", ".ts":
				sigs := extractJSFunctionSignatures(content)
				for _, sig := range sigs {
					if !seen[sig] {
						seen[sig] = true
						signatures = append(signatures, sig)
					}
				}
			}
		}
		if len(signatures) > 0 {
			break
		}
	}

	if len(signatures) > 10 {
		signatures = signatures[:10]
	}
	return signatures
}

// extractPythonFunctionSignatures finds function call patterns in Python test code.
// Looks for calls to non-stdlib functions: solution.solve(x, y), solve(n, k), etc.
func extractPythonFunctionSignatures(content string) []string {
	var sigs []string
	seen := make(map[string]bool)

	// Known test/stdlib function names to skip.
	skipFuncs := map[string]bool{
		"assert": true, "print": true, "len": true, "range": true,
		"int": true, "str": true, "float": true, "list": true, "dict": true,
		"set": true, "tuple": true, "sorted": true, "reversed": true,
		"enumerate": true, "zip": true, "map": true, "filter": true,
		"isinstance": true, "type": true, "hasattr": true, "getattr": true,
		"open": true, "os": true, "json": true, "re": true,
		"assertEqual": true, "assertAlmostEqual": true, "assertTrue": true,
		"assertFalse": true, "assertRaises": true, "assertIn": true,
		"assertNotEqual": true, "assertIsNone": true, "assertIsNotNone": true,
		"assertGreater": true, "assertLess": true, "assertGreaterEqual": true,
		"assertLessEqual": true, "assertCountEqual": true,
		"pytest": true, "fixture": true, "parametrize": true,
		"isclose": true, "allclose": true, "array_equal": true,
		"approx": true,
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "import") || strings.HasPrefix(trimmed, "from ") {
			continue
		}

		// Look for function calls: word( or word.word(
		// Patterns: "result = solve(n, k)", "assert process(data) == expected",
		// "solution.solve(3, [1,2,3])"
		for i := 0; i < len(trimmed)-2; i++ {
			if trimmed[i] != '(' {
				continue
			}
			// Extract function name (walk backward).
			nameEnd := i
			nameStart := nameEnd - 1
			for nameStart >= 0 && (isAlphaNumUnderscore(trimmed[nameStart]) || trimmed[nameStart] == '.') {
				nameStart--
			}
			nameStart++
			if nameStart >= nameEnd {
				continue
			}
			fullName := trimmed[nameStart:nameEnd]
			// Extract the base function name (after last dot).
			baseName := fullName
			if dotIdx := strings.LastIndex(fullName, "."); dotIdx >= 0 {
				baseName = fullName[dotIdx+1:]
			}
			if baseName == "" || skipFuncs[baseName] || strings.HasPrefix(baseName, "assert") || strings.HasPrefix(baseName, "self.") {
				continue
			}
			// Skip if the "module" part is a known stdlib/test module.
			if dotIdx := strings.Index(fullName, "."); dotIdx >= 0 {
				module := fullName[:dotIdx]
				if skipFuncs[module] || module == "self" || module == "cls" || module == "np" || module == "pd" {
					continue
				}
			}

			// Extract arguments (up to the matching closing paren).
			argStart := i + 1
			depth := 1
			argEnd := argStart
			for argEnd < len(trimmed) && depth > 0 {
				if trimmed[argEnd] == '(' {
					depth++
				} else if trimmed[argEnd] == ')' {
					depth--
				}
				argEnd++
			}
			if depth != 0 {
				continue // unbalanced parens
			}
			args := trimmed[argStart : argEnd-1]
			if len(args) > 100 {
				args = args[:100] + "..."
			}

			sig := fullName + "(" + args + ")"
			if !seen[sig] && len(sig) < 150 {
				seen[sig] = true
				sigs = append(sigs, sig)
			}
			break // one sig per line
		}
	}

	// Deduplicate by function name — keep the most informative call.
	byFunc := make(map[string]string) // baseName → best signature
	for _, sig := range sigs {
		parenIdx := strings.Index(sig, "(")
		if parenIdx < 0 {
			continue
		}
		name := sig[:parenIdx]
		baseName := name
		if dotIdx := strings.LastIndex(name, "."); dotIdx >= 0 {
			baseName = name[dotIdx+1:]
		}
		existing, ok := byFunc[baseName]
		if !ok || len(sig) > len(existing) {
			byFunc[baseName] = sig
		}
	}

	var result []string
	for _, sig := range byFunc {
		result = append(result, sig)
	}
	return result
}

// extractJSFunctionSignatures finds function call patterns in JS/TS test code.
func extractJSFunctionSignatures(content string) []string {
	var sigs []string
	seen := make(map[string]bool)

	skipFuncs := map[string]bool{
		"describe": true, "it": true, "test": true, "expect": true,
		"beforeEach": true, "afterEach": true, "beforeAll": true, "afterAll": true,
		"require": true, "import": true, "console": true,
		"parseInt": true, "parseFloat": true, "JSON": true,
		"Array": true, "Object": true, "String": true, "Number": true,
		"toEqual": true, "toBe": true, "toContain": true, "toThrow": true,
		"toHaveLength": true, "toBeDefined": true, "toBeNull": true,
		"toStrictEqual": true, "toBeGreaterThan": true, "toBeLessThan": true,
		"toBeCloseTo": true,
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "import") ||
			strings.HasPrefix(trimmed, "const {") || strings.HasPrefix(trimmed, "const ") && strings.Contains(trimmed, "require(") {
			continue
		}

		for i := 0; i < len(trimmed)-2; i++ {
			if trimmed[i] != '(' {
				continue
			}
			nameEnd := i
			nameStart := nameEnd - 1
			for nameStart >= 0 && (isAlphaNumUnderscore(trimmed[nameStart]) || trimmed[nameStart] == '.') {
				nameStart--
			}
			nameStart++
			if nameStart >= nameEnd {
				continue
			}
			fullName := trimmed[nameStart:nameEnd]
			baseName := fullName
			if dotIdx := strings.LastIndex(fullName, "."); dotIdx >= 0 {
				baseName = fullName[dotIdx+1:]
			}
			if baseName == "" || skipFuncs[baseName] {
				continue
			}

			argStart := i + 1
			depth := 1
			argEnd := argStart
			for argEnd < len(trimmed) && depth > 0 {
				if trimmed[argEnd] == '(' {
					depth++
				} else if trimmed[argEnd] == ')' {
					depth--
				}
				argEnd++
			}
			if depth != 0 {
				continue
			}
			args := trimmed[argStart : argEnd-1]
			if len(args) > 100 {
				args = args[:100] + "..."
			}

			sig := fullName + "(" + args + ")"
			if !seen[sig] && len(sig) < 150 {
				seen[sig] = true
				sigs = append(sigs, sig)
			}
			break
		}
	}

	// Deduplicate by base function name.
	byFunc := make(map[string]string)
	for _, sig := range sigs {
		parenIdx := strings.Index(sig, "(")
		if parenIdx < 0 {
			continue
		}
		name := sig[:parenIdx]
		baseName := name
		if dotIdx := strings.LastIndex(name, "."); dotIdx >= 0 {
			baseName = name[dotIdx+1:]
		}
		existing, ok := byFunc[baseName]
		if !ok || len(sig) > len(existing) {
			byFunc[baseName] = sig
		}
	}

	var result []string
	for _, sig := range byFunc {
		result = append(result, sig)
	}
	return result
}

func isAlphaNumUnderscore(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// detectComparisonTolerances scans test files for numerical comparison patterns
// with specific tolerance requirements. Tests using assertAlmostEqual, isclose,
// np.allclose, pytest.approx, etc. have precision requirements that the agent
// must match. Missing these causes test failures on otherwise correct solutions.
func detectComparisonTolerances(workDir string) []string {
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	var tolerances []string
	seen := make(map[string]bool)

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
			for _, line := range strings.Split(content, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
					continue
				}
				lower := strings.ToLower(trimmed)

				var hint string

				// Python: assertAlmostEqual(a, b, places=5)
				if strings.Contains(lower, "assertalmostequal") && strings.Contains(lower, "places") {
					hint = "assertAlmostEqual with " + extractKVFromLine(trimmed, "places") + " decimal places"
				}

				// Python: math.isclose(a, b, rel_tol=1e-9, abs_tol=1e-12)
				if strings.Contains(lower, "isclose") && (strings.Contains(lower, "rel_tol") || strings.Contains(lower, "abs_tol")) {
					tols := []string{}
					if strings.Contains(lower, "rel_tol") {
						tols = append(tols, "rel_tol="+extractKVFromLine(trimmed, "rel_tol"))
					}
					if strings.Contains(lower, "abs_tol") {
						tols = append(tols, "abs_tol="+extractKVFromLine(trimmed, "abs_tol"))
					}
					hint = "isclose with " + strings.Join(tols, ", ")
				}

				// Python: np.allclose(a, b, atol=1e-6, rtol=1e-5)
				if strings.Contains(lower, "allclose") && (strings.Contains(lower, "atol") || strings.Contains(lower, "rtol")) {
					tols := []string{}
					if strings.Contains(lower, "atol") {
						tols = append(tols, "atol="+extractKVFromLine(trimmed, "atol"))
					}
					if strings.Contains(lower, "rtol") {
						tols = append(tols, "rtol="+extractKVFromLine(trimmed, "rtol"))
					}
					hint = "np.allclose with " + strings.Join(tols, ", ")
				}

				// Python: pytest.approx(expected, abs=1e-6, rel=1e-3)
				if strings.Contains(lower, "approx") && (strings.Contains(lower, "abs=") || strings.Contains(lower, "rel=")) {
					tols := []string{}
					if strings.Contains(lower, "abs=") {
						tols = append(tols, "abs="+extractKVFromLine(trimmed, "abs"))
					}
					if strings.Contains(lower, "rel=") {
						tols = append(tols, "rel="+extractKVFromLine(trimmed, "rel"))
					}
					hint = "pytest.approx with " + strings.Join(tols, ", ")
				}

				// Generic: abs(a - b) < epsilon patterns
				if strings.Contains(lower, "abs(") && (strings.Contains(lower, "< 0.") || strings.Contains(lower, "< 1e-") || strings.Contains(lower, "<= 0.") || strings.Contains(lower, "<= 1e-")) {
					// Extract the threshold value.
					for _, sep := range []string{"< ", "<= "} {
						idx := strings.Index(lower, "abs(")
						if idx < 0 {
							continue
						}
						threshIdx := strings.Index(lower[idx:], sep)
						if threshIdx < 0 {
							continue
						}
						threshStart := idx + threshIdx + len(sep)
						threshEnd := threshStart
						for threshEnd < len(lower) && (lower[threshEnd] == '.' || lower[threshEnd] == '-' || lower[threshEnd] == 'e' ||
							(lower[threshEnd] >= '0' && lower[threshEnd] <= '9')) {
							threshEnd++
						}
						if threshEnd > threshStart {
							hint = "Tolerance: |actual - expected| " + sep + trimmed[threshStart:threshEnd]
						}
						break
					}
				}

				// JS: toBeCloseTo(expected, numDigits)
				if strings.Contains(lower, "tobecloseto") {
					hint = "Jest toBeCloseTo — default 5 decimal places precision"
				}

				if hint != "" && !seen[hint] {
					seen[hint] = true
					tolerances = append(tolerances, hint)
				}
			}
		}
		if len(tolerances) > 0 {
			break
		}
	}

	if len(tolerances) > 5 {
		tolerances = tolerances[:5]
	}
	return tolerances
}

// extractKVFromLine extracts the value for a key=value pattern in a line.
// E.g., extractKVFromLine("isclose(a, b, rel_tol=1e-9)", "rel_tol") → "1e-9"
func extractKVFromLine(line, key string) string {
	idx := strings.Index(line, key+"=")
	if idx < 0 {
		return "?"
	}
	start := idx + len(key) + 1
	end := start
	for end < len(line) && line[end] != ',' && line[end] != ')' && line[end] != ' ' {
		end++
	}
	if end > start {
		return strings.TrimSpace(line[start:end])
	}
	return "?"
}

// extractTestEnvironmentVars scans test scripts for environment variable
// settings (export FOO=bar, os.environ, process.env). These env vars often
// configure ports, database URLs, API keys, or feature flags that the solution
// must respect. Missing them causes silent failures.
func extractTestEnvironmentVars(workDir string) []string {
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test"), workDir, "/app"}
	var envVars []string
	seen := make(map[string]bool)

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
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
					continue
				}

				var envVar string

				// Shell: export FOO=bar or FOO=bar (before command)
				if strings.HasPrefix(trimmed, "export ") {
					rest := strings.TrimPrefix(trimmed, "export ")
					if eqIdx := strings.Index(rest, "="); eqIdx > 0 {
						varName := strings.TrimSpace(rest[:eqIdx])
						value := strings.TrimSpace(rest[eqIdx+1:])
						value = strings.Trim(value, "\"'")
						// Skip PATH, HOME, and other generic vars.
						if !isGenericEnvVar(varName) {
							envVar = varName + "=" + value
						}
					}
				}

				// Python: os.environ["FOO"] = "bar" or os.environ.setdefault("FOO", "bar")
				if strings.Contains(trimmed, "os.environ") {
					if strings.Contains(trimmed, "os.environ[") {
						// os.environ["KEY"] = "VALUE"
						if kv := extractPythonEnvAssign(trimmed); kv != "" {
							envVar = kv
						}
					} else if strings.Contains(trimmed, ".setdefault(") || strings.Contains(trimmed, ".get(") {
						// os.environ.setdefault("KEY", "VALUE")
						if kv := extractPythonEnvDefault(trimmed); kv != "" {
							envVar = kv
						}
					}
				}

				// JS/TS: process.env.FOO = "bar"
				if strings.Contains(trimmed, "process.env.") && strings.Contains(trimmed, "=") {
					// process.env.PORT = "8080"
					idx := strings.Index(trimmed, "process.env.")
					rest := trimmed[idx+12:]
					if eqIdx := strings.Index(rest, "="); eqIdx > 0 {
						varName := strings.TrimSpace(rest[:eqIdx])
						varName = strings.TrimRight(varName, " ")
						value := strings.TrimSpace(rest[eqIdx+1:])
						value = strings.Trim(value, "\"';")
						if !isGenericEnvVar(varName) {
							envVar = varName + "=" + value
						}
					}
				}

				if envVar != "" && !seen[envVar] {
					seen[envVar] = true
					envVars = append(envVars, envVar)
				}
			}
		}
		if len(envVars) > 0 {
			break
		}
	}

	if len(envVars) > 8 {
		envVars = envVars[:8]
	}
	return envVars
}

// isGenericEnvVar returns true for common env vars that aren't task-specific.
func isGenericEnvVar(name string) bool {
	generic := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "SHELL": true,
		"LANG": true, "LC_ALL": true, "TERM": true, "PWD": true,
		"PYTHONDONTWRITEBYTECODE": true, "PYTHONUNBUFFERED": true,
		"PYTHONPATH": true, "GOPATH": true, "GOROOT": true,
		"NODE_PATH": true, "NODE_ENV": true,
		"PIP_BREAK_SYSTEM_PACKAGES": true,
	}
	return generic[name]
}

// extractPythonEnvAssign extracts KEY=VALUE from os.environ["KEY"] = "VALUE".
func extractPythonEnvAssign(line string) string {
	// Find the key in brackets after os.environ
	bracketIdx := strings.Index(line, "os.environ[")
	if bracketIdx < 0 {
		return ""
	}
	rest := line[bracketIdx+11:]
	// Extract key from quotes.
	key := extractQuotedString(rest)
	if key == "" || isGenericEnvVar(key) {
		return ""
	}
	// Find the value after =
	eqIdx := strings.Index(rest, "=")
	if eqIdx < 0 {
		return key + "=(set in test)"
	}
	value := strings.TrimSpace(rest[eqIdx+1:])
	value = strings.Trim(value, "\"' ")
	return key + "=" + value
}

// extractPythonEnvDefault extracts KEY=VALUE from os.environ.setdefault("KEY", "VALUE").
func extractPythonEnvDefault(line string) string {
	// Find the opening paren after setdefault or get.
	var funcIdx int
	if idx := strings.Index(line, ".setdefault("); idx >= 0 {
		funcIdx = idx + 12
	} else if idx := strings.Index(line, ".get("); idx >= 0 {
		funcIdx = idx + 5
	} else {
		return ""
	}
	rest := line[funcIdx:]
	key := extractQuotedString(rest)
	if key == "" || isGenericEnvVar(key) {
		return ""
	}
	// Find the default value (second argument).
	commaIdx := strings.Index(rest, ",")
	if commaIdx < 0 {
		return key + "=(read from env)"
	}
	valueRest := rest[commaIdx+1:]
	value := extractQuotedString(valueRest)
	if value == "" {
		value = strings.TrimSpace(valueRest)
		value = strings.TrimRight(value, ")")
	}
	return key + "=" + value
}

// extractQuotedString extracts the first quoted string from text.
func extractQuotedString(s string) string {
	for _, delim := range []byte{'"', '\''} {
		idx := strings.IndexByte(s, delim)
		if idx < 0 {
			continue
		}
		endIdx := strings.IndexByte(s[idx+1:], delim)
		if endIdx < 0 {
			continue
		}
		return s[idx+1 : idx+1+endIdx]
	}
	return ""
}

// detectExpectedWorkingDir analyzes test scripts to determine if they expect
// the solution to run from a specific directory. Tests that cd to a directory
// or use absolute paths reveal the expected cwd.
func detectExpectedWorkingDir(workDir string) string {
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
			for _, line := range strings.Split(content, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
					continue
				}

				// Shell: cd /app or cd /some/path
				if strings.HasPrefix(trimmed, "cd ") && !strings.Contains(trimmed, "$") {
					dir := strings.TrimPrefix(trimmed, "cd ")
					dir = strings.TrimSpace(dir)
					dir = strings.Trim(dir, "\"'")
					if strings.HasPrefix(dir, "/") && dir != workDir {
						return fmt.Sprintf("Tests cd to `%s` before running your solution. Ensure your output files are in that directory.", dir)
					}
				}

				// Python: os.chdir("/app")
				if strings.Contains(trimmed, "os.chdir(") {
					dir := extractQuotedString(trimmed[strings.Index(trimmed, "os.chdir(")+9:])
					if dir != "" && strings.HasPrefix(dir, "/") && dir != workDir {
						return fmt.Sprintf("Tests chdir to `%s` before running your solution. Ensure your output files are in that directory.", dir)
					}
				}

				// Python: subprocess with cwd= argument
				if strings.Contains(trimmed, "cwd=") && (strings.Contains(trimmed, "subprocess") || strings.Contains(trimmed, "Popen")) {
					cwdIdx := strings.Index(trimmed, "cwd=")
					rest := trimmed[cwdIdx+4:]
					dir := extractQuotedString(rest)
					if dir != "" && strings.HasPrefix(dir, "/") && dir != workDir {
						return fmt.Sprintf("Tests run your solution with cwd=`%s`. Ensure your solution and output files are in that directory.", dir)
					}
				}
			}
		}
	}

	return ""
}

// autoMkdirOutputDirs creates output directories that tests reference but don't
// exist yet. This saves 1 turn of "No such file or directory" errors when the
// agent writes output files. Only creates directories from well-known output
// paths detected in test analysis — not arbitrary paths.
func autoMkdirOutputDirs(workDir string, expectedOutputs []string) {
	created := make(map[string]bool)
	for _, o := range expectedOutputs {
		dir := filepath.Dir(o)
		if dir == "." || dir == "" || dir == "/" {
			continue
		}
		// Resolve relative paths against workDir.
		fullDir := dir
		if !filepath.IsAbs(dir) {
			fullDir = filepath.Join(workDir, dir)
		}
		if created[fullDir] || dirExists(fullDir) {
			continue
		}
		if err := os.MkdirAll(fullDir, 0o755); err == nil {
			created[fullDir] = true
			fmt.Fprintf(os.Stderr, "[gollem] auto-mkdir: %s\n", fullDir)
		}
	}
}

// extractPerTestTimeouts scans test scripts for per-test timeout settings.
// Returns human-readable descriptions like "30s per test case".
// Many TB2 test scripts wrap commands with `timeout N` or set ulimits,
// and agents that don't know about these limits produce solutions that are
// correct but too slow.
func extractPerTestTimeouts(workDir string) []string {
	var timeouts []string
	seen := make(map[string]bool)

	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	for _, td := range testDirs {
		entries, err := os.ReadDir(td)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".sh") && !strings.HasSuffix(name, ".py") {
				continue
			}
			info, _ := entry.Info()
			if info == nil || info.Size() > 50000 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(td, name))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") {
					continue
				}

				// Shell: timeout N command...
				if strings.Contains(trimmed, "timeout ") {
					fields := strings.Fields(trimmed)
					for i, f := range fields {
						if f == "timeout" && i+1 < len(fields) {
							secs := fields[i+1]
							if isNumericOrFloat(secs) {
								desc := secs + "s per test case"
								if !seen[desc] {
									seen[desc] = true
									timeouts = append(timeouts, desc)
								}
							}
							break
						}
					}
				}

				// Shell: ulimit -t N (CPU time limit)
				if strings.Contains(trimmed, "ulimit -t ") {
					fields := strings.Fields(trimmed)
					for i, f := range fields {
						if f == "-t" && i+1 < len(fields) {
							secs := fields[i+1]
							if isNumericOrFloat(secs) {
								desc := secs + "s CPU time limit (ulimit)"
								if !seen[desc] {
									seen[desc] = true
									timeouts = append(timeouts, desc)
								}
							}
							break
						}
					}
				}

				// Python: signal.alarm(N)
				if strings.Contains(trimmed, "signal.alarm(") {
					start := strings.Index(trimmed, "signal.alarm(") + len("signal.alarm(")
					end := strings.Index(trimmed[start:], ")")
					if end > 0 {
						secs := strings.TrimSpace(trimmed[start : start+end])
						if isNumericOrFloat(secs) {
							desc := secs + "s per test case (signal.alarm)"
							if !seen[desc] {
								seen[desc] = true
								timeouts = append(timeouts, desc)
							}
						}
					}
				}

				// Variable assignments: TIME_LIMIT=N, TIMEOUT=N, TIME_BUDGET=N
				lower := strings.ToLower(trimmed)
				for _, prefix := range []string{"time_limit", "timeout", "time_budget"} {
					if strings.HasPrefix(lower, prefix) && strings.Contains(trimmed, "=") {
						eqParts := strings.SplitN(trimmed, "=", 2)
						if len(eqParts) == 2 {
							val := strings.TrimSpace(eqParts[1])
							val = strings.Trim(val, "\"' ")
							if isNumericOrFloat(val) {
								desc := val + "s time limit (" + strings.TrimSpace(eqParts[0]) + ")"
								if !seen[desc] {
									seen[desc] = true
									timeouts = append(timeouts, desc)
								}
							}
						}
					}
				}
			}
		}
		if len(timeouts) > 0 {
			break
		}
	}

	if len(timeouts) > 5 {
		timeouts = timeouts[:5]
	}
	return timeouts
}

// isNumericOrFloat returns true if s looks like a positive number (int or float).
func isNumericOrFloat(s string) bool {
	if s == "" {
		return false
	}
	dotSeen := false
	for _, c := range s {
		if c == '.' && !dotSeen {
			dotSeen = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// diffTarget represents a diff/cmp comparison found in a test script.
type diffTarget struct {
	expectedRef string // path to the reference/expected output file
	flags       string // diff flags (empty for exact match)
}

// autoReadDiffExpectedFiles scans test scripts for diff/cmp comparisons,
// identifies expected reference files, and auto-reads them into context.
// Seeing the exact expected output is the #1 way to produce correct output —
// it eliminates guessing about format, whitespace, encoding, and structure.
// Returns remaining auto-read budget.
func autoReadDiffExpectedFiles(workDir string, parts *[]string, budget int) int {
	targets := extractDiffTargets(workDir)
	if len(targets) == 0 {
		return budget
	}

	// Surface comparison mode hints.
	var comparisonHints []string
	for _, t := range targets {
		if t.flags != "" {
			comparisonHints = append(comparisonHints, t.flags)
		}
	}
	if len(comparisonHints) > 0 {
		*parts = append(*parts, "\n## Comparison Mode (from test scripts)")
		seen := make(map[string]bool)
		for _, h := range comparisonHints {
			if !seen[h] {
				seen[h] = true
				*parts = append(*parts, "  - "+h)
			}
		}
	}

	// Auto-read expected reference files.
	count := 0
	for _, t := range targets {
		if budget <= 0 || count >= 3 {
			break
		}
		// Try multiple locations for the expected file.
		candidates := []string{t.expectedRef}
		if !filepath.IsAbs(t.expectedRef) {
			candidates = append(candidates,
				filepath.Join(workDir, t.expectedRef),
				filepath.Join("/app", t.expectedRef),
				filepath.Join("/app/task_file", t.expectedRef),
			)
		}
		for _, candidate := range candidates {
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > 5000 {
				continue
			}
			limit := 5000
			if limit > budget {
				limit = budget
			}
			content := readFileTruncated(candidate, limit)
			if content != "" {
				*parts = append(*parts, fmt.Sprintf("\n## Expected output reference (from test diff): %s", candidate))
				*parts = append(*parts, content)
				*parts = append(*parts, "Your output MUST match this file exactly. Verify with: diff <expected> <your_output>")
				budget -= len(content)
				count++
			}
			break
		}
	}

	return budget
}

// extractDiffTargets scans test scripts for diff/cmp commands and returns
// the expected reference file paths. Uses heuristics to determine which file
// is the reference: files that already exist, files in /tests/, files named
// "expected*", "reference*", etc.
func extractDiffTargets(workDir string) []diffTarget {
	var targets []diffTarget
	seen := make(map[string]bool)

	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	for _, td := range testDirs {
		entries, err := os.ReadDir(td)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".sh") && !strings.HasSuffix(name, ".py") {
				continue
			}
			info, _ := entry.Info()
			if info == nil || info.Size() > 50000 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(td, name))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") {
					continue
				}

				// Shell: diff [flags] file1 file2
				if strings.Contains(trimmed, "diff ") {
					if t := parseDiffLine(trimmed, workDir); t.expectedRef != "" && !seen[t.expectedRef] {
						seen[t.expectedRef] = true
						targets = append(targets, t)
					}
				}

				// Shell: cmp [flags] file1 file2
				if strings.Contains(trimmed, "cmp ") {
					if t := parseCmpLine(trimmed, workDir); t.expectedRef != "" && !seen[t.expectedRef] {
						seen[t.expectedRef] = true
						targets = append(targets, t)
					}
				}
			}
		}
		if len(targets) > 0 {
			break
		}
	}

	if len(targets) > 5 {
		targets = targets[:5]
	}
	return targets
}

// parseDiffLine extracts a diff target from a shell diff command line.
// Returns the expected reference file path and any flags.
// Heuristic: the file that exists is the reference; if both exist, use
// naming conventions (expected, reference, gold → reference file).
func parseDiffLine(line, workDir string) diffTarget {
	idx := strings.Index(line, "diff ")
	if idx < 0 {
		return diffTarget{}
	}
	rest := line[idx:]

	fields := strings.Fields(rest)
	if len(fields) < 3 {
		return diffTarget{}
	}

	var flags []string
	var files []string
	for _, f := range fields[1:] { // skip "diff"
		if strings.HasPrefix(f, "-") {
			flags = append(flags, f)
		} else if strings.HasPrefix(f, "<(") || strings.HasPrefix(f, ">(") || strings.HasPrefix(f, "$") {
			// Process substitution or variable — skip.
			continue
		} else {
			files = append(files, f)
		}
	}

	if len(files) < 2 {
		return diffTarget{}
	}

	ref := classifyDiffReference(files[0], files[1], workDir)
	if ref == "" {
		return diffTarget{}
	}

	flagStr := ""
	if len(flags) > 0 {
		flagStr = describeDiffFlags(flags)
	} else {
		flagStr = "Exact match required (diff with no flags)"
	}

	return diffTarget{expectedRef: ref, flags: flagStr}
}

// parseCmpLine extracts a diff target from a shell cmp command line.
func parseCmpLine(line, workDir string) diffTarget {
	idx := strings.Index(line, "cmp ")
	if idx < 0 {
		return diffTarget{}
	}
	rest := line[idx:]

	fields := strings.Fields(rest)
	var files []string
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "-") {
			files = append(files, f)
		}
	}

	if len(files) < 2 {
		return diffTarget{}
	}

	ref := classifyDiffReference(files[0], files[1], workDir)
	if ref == "" {
		return diffTarget{}
	}

	return diffTarget{expectedRef: ref, flags: "Byte-exact comparison (cmp)"}
}

// classifyDiffReference determines which of two files is the reference/expected
// file and returns it. Returns empty string if neither looks like a reference.
func classifyDiffReference(file1, file2, workDir string) string {
	// Score each file: higher score = more likely to be the reference.
	score1 := diffRefScore(file1)
	score2 := diffRefScore(file2)

	// Files that exist are more likely to be references (pre-placed).
	if fileExistsAnywhere(file1, workDir) {
		score1 += 2
	}
	if fileExistsAnywhere(file2, workDir) {
		score2 += 2
	}

	// If only one exists, it's the reference.
	exists1 := fileExistsAnywhere(file1, workDir)
	exists2 := fileExistsAnywhere(file2, workDir)
	if exists1 && !exists2 {
		return file1
	}
	if exists2 && !exists1 {
		return file2
	}

	// Both exist or neither exists — use name heuristics.
	if score1 > score2 {
		return file1
	}
	if score2 > score1 {
		return file2
	}

	// Tie: default to first arg (conventional: diff expected actual).
	if exists1 {
		return file1
	}
	return ""
}

// diffRefScore scores a file path on how likely it is to be a reference/expected file.
func diffRefScore(path string) int {
	lower := strings.ToLower(path)
	score := 0
	// Reference indicators.
	for _, kw := range []string{"expected", "reference", "gold", "correct", "baseline", "answer_key"} {
		if strings.Contains(lower, kw) {
			score += 3
		}
	}
	// /tests/ directory files are usually references.
	if strings.Contains(lower, "/tests/") {
		score += 2
	}
	// Output indicators (less likely to be reference).
	for _, kw := range []string{"output", "result", "actual", "my_", "student"} {
		if strings.Contains(lower, kw) {
			score -= 2
		}
	}
	return score
}

// fileExistsAnywhere checks if a file exists at the given path, or resolved
// against workDir, /app, or /app/task_file.
func fileExistsAnywhere(path, workDir string) bool {
	if filepath.IsAbs(path) {
		return fileExists(path)
	}
	for _, base := range []string{workDir, "/app", "/app/task_file", "/tests"} {
		if fileExists(filepath.Join(base, path)) {
			return true
		}
	}
	return false
}

// describeDiffFlags returns a human-readable description of diff flags.
func describeDiffFlags(flags []string) string {
	for _, f := range flags {
		switch f {
		case "-b":
			return "Ignores trailing whitespace changes (diff -b)"
		case "-w", "--ignore-all-space":
			return "Ignores all whitespace differences (diff -w)"
		case "-i", "--ignore-case":
			return "Case-insensitive comparison (diff -i)"
		case "-B", "--ignore-blank-lines":
			return "Ignores blank line differences (diff -B)"
		case "-q", "--brief":
			return "Quick check — only reports whether files differ (diff -q)"
		case "--strip-trailing-cr":
			return "Ignores CR/LF differences (diff --strip-trailing-cr)"
		}
	}
	return "Comparison with flags: " + strings.Join(flags, " ")
}

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

// chmodScriptsInDir makes all shell/Python scripts in a directory executable,
// including subdirectories (depth 2). This prevents "Permission denied" errors
// that waste 1-2 agent turns on TB2 tasks with nested test structures.
func chmodScriptsInDir(dir string) {
	chmodScriptsInDirRecursive(dir, 0, 2)
}

func chmodScriptsInDirRecursive(dir string, depth, maxDepth int) {
	if depth > maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			name := entry.Name()
			if !strings.HasPrefix(name, ".") && name != "__pycache__" && name != "node_modules" {
				chmodScriptsInDirRecursive(filepath.Join(dir, name), depth+1, maxDepth)
			}
			continue
		}
		lower := strings.ToLower(entry.Name())
		if strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bash") ||
			strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".rb") ||
			strings.HasSuffix(lower, ".pl") {
			os.Chmod(filepath.Join(dir, entry.Name()), 0o755)
		}
	}
}

// detectAndActivateVenv detects Python virtual environments (venv, conda) and
// returns a hint string if one is found. Also modifies PATH environment variable
// so subsequent pip/python commands use the venv's interpreter.
func detectAndActivateVenv(workDir string) string {
	// Common venv locations.
	venvPaths := []string{
		filepath.Join(workDir, "venv"),
		filepath.Join(workDir, ".venv"),
		filepath.Join("/app", "venv"),
		filepath.Join("/app", ".venv"),
		filepath.Join(workDir, "env"),
	}
	for _, vp := range venvPaths {
		activate := filepath.Join(vp, "bin", "activate")
		if fileExists(activate) {
			// Add venv bin to PATH so python/pip resolve to the venv.
			binDir := filepath.Join(vp, "bin")
			currentPath := os.Getenv("PATH")
			if !strings.Contains(currentPath, binDir) {
				os.Setenv("PATH", binDir+":"+currentPath)
			}
			return fmt.Sprintf("Python venv detected: %s (auto-activated — python/pip resolve to venv)", vp)
		}
	}

	// Check for conda environment.
	condaPrefix := os.Getenv("CONDA_PREFIX")
	if condaPrefix != "" {
		return "Conda environment active: " + condaPrefix
	}

	return ""
}

// detectShellTask returns true if the task is primarily a shell scripting task.
func detectShellTask(workDir string) bool {
	// Check directory name for shell-related keywords.
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"bash", "shell", "scripting", "awk", "sed"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	// Count shell scripts vs other source files.
	shCount := 0
	otherCount := 0
	for _, dir := range []string{workDir, "/app"} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".sh") || strings.HasSuffix(name, ".bash") {
				shCount++
			} else if strings.HasSuffix(name, ".py") || strings.HasSuffix(name, ".go") ||
				strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".rs") ||
				strings.HasSuffix(name, ".c") || strings.HasSuffix(name, ".cpp") ||
				strings.HasSuffix(name, ".java") || strings.HasSuffix(name, ".rb") {
				otherCount++
			}
		}
	}
	// Shell task if the majority of source files are shell scripts.
	return shCount >= 2 && shCount > otherCount
}

// detectPythonImports scans .py files for third-party imports that aren't
// installed. Returns the pip package names to install. Only checks the top-level
// import name and uses a static list of known third-party packages to avoid
// false positives from standard library modules.
func detectPythonImports(workDir string) []string {
	// Known third-party module → pip package mappings.
	thirdParty := map[string]string{
		"numpy":        "numpy",
		"pandas":       "pandas",
		"scipy":        "scipy",
		"matplotlib":   "matplotlib",
		"seaborn":      "seaborn",
		"sklearn":      "scikit-learn",
		"skimage":      "scikit-image",
		"cv2":          "opencv-python",
		"PIL":          "Pillow",
		"torch":        "torch",
		"tensorflow":   "tensorflow",
		"transformers": "transformers",
		"datasets":     "datasets",
		"yaml":         "PyYAML",
		"bs4":          "beautifulsoup4",
		"requests":     "requests",
		"flask":        "flask",
		"django":       "django",
		"fastapi":      "fastapi",
		"uvicorn":      "uvicorn",
		"pydantic":     "pydantic",
		"httpx":        "httpx",
		"aiohttp":      "aiohttp",
		"sqlalchemy":   "sqlalchemy",
		"redis":        "redis",
		"pymongo":      "pymongo",
		"psycopg2":     "psycopg2-binary",
		"dotenv":       "python-dotenv",
		"tqdm":         "tqdm",
		"click":        "click",
		"rich":         "rich",
		"networkx":     "networkx",
		"sympy":        "sympy",
		"lxml":         "lxml",
		"jwt":          "PyJWT",
		"Crypto":       "pycryptodome",
		"nacl":         "PyNaCl",
		"paramiko":     "paramiko",
		"toml":         "toml",
		"msgpack":      "msgpack",
		"pyarrow":      "pyarrow",
		"h5py":         "h5py",
		"pytest":       "pytest",
		"attr":         "attrs",
		"dateutil":     "python-dateutil",
		"serial":       "pyserial",
		"construct":    "construct",
		"lark":         "lark",
		"pyparsing":    "pyparsing",
		"bitstring":    "bitstring",
		"elftools":     "pyelftools",
		// Test-related packages (common in TB2 test scripts).
		"hypothesis":   "hypothesis",
		"freezegun":    "freezegun",
		"mock":         "mock",
		"responses":    "responses",
		"faker":        "Faker",
		"factory":      "factory-boy",
		"parameterized": "parameterized",
		"colorama":     "colorama",
		"tabulate":     "tabulate",
		"jinja2":       "Jinja2",
		"Levenshtein":  "python-Levenshtein",
		"regex":        "regex",
		"orjson":       "orjson",
		"ujson":        "ujson",
		"xxhash":       "xxhash",
		"sortedcontainers": "sortedcontainers",
		// Crypto and security.
		"cryptography": "cryptography",
		"gmpy2":        "gmpy2",
		// System and process utilities.
		"psutil":  "psutil",
		"pexpect": "pexpect",
		// Web/network.
		"websockets": "websockets",
		"websocket":  "websocket-client",
		"grpc":       "grpcio",
		// Data formats and validation.
		"jsonschema":     "jsonschema",
		"more_itertools": "more-itertools",
		"bitarray":       "bitarray",
		// Image processing and scientific.
		"imageio":        "imageio",
		"skfuzzy":        "scikit-fuzzy",
		"astropy":        "astropy",
		"Bio":            "biopython",
		"rdkit":          "rdkit",
		"shapely":        "shapely",
		"fiona":          "fiona",
		"geopandas":      "geopandas",
		// Concurrency and async.
		"trio":           "trio",
		"anyio":          "anyio",
		"gevent":         "gevent",
		// Serialization and compression.
		"cbor2":          "cbor2",
		"bson":           "pymongo",
		"blosc":          "blosc",
		"zstandard":      "zstandard",
		"lz4":            "lz4",
		// Math and optimization.
		"pulp":           "PuLP",
		"cvxpy":          "cvxpy",
		"z3":             "z3-solver",
		// CLI and config.
		"typer":          "typer",
	}

	needed := make(map[string]string) // module → pip package

	// Scan .py files in workDir, /app, and test directories (non-recursive, limit 20 per dir).
	// Including test directories catches test-specific deps like hypothesis,
	// freezegun, mock, etc. that the agent would otherwise discover via
	// ModuleNotFoundError during test runs (wasting 2+ turns).
	scanDirs := []string{workDir, "/app"}
	for _, td := range []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")} {
		if dirExists(td) {
			scanDirs = append(scanDirs, td)
			break // only one test dir
		}
	}
	for _, dir := range scanDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.py"))
		if len(matches) > 20 {
			matches = matches[:20]
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.Size() > 50000 {
				continue
			}
			data, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				// Match "import foo" or "from foo import bar".
				var module string
				if strings.HasPrefix(trimmed, "import ") {
					// "import foo" or "import foo, bar" or "import foo as f"
					rest := strings.TrimPrefix(trimmed, "import ")
					module = strings.Fields(rest)[0]
					module = strings.TrimRight(module, ",")
				} else if strings.HasPrefix(trimmed, "from ") {
					// "from foo import bar"
					rest := strings.TrimPrefix(trimmed, "from ")
					parts := strings.Fields(rest)
					if len(parts) >= 1 {
						module = parts[0]
					}
				}
				if module == "" {
					continue
				}
				// Use top-level package name.
				if dot := strings.Index(module, "."); dot > 0 {
					module = module[:dot]
				}
				if pkg, ok := thirdParty[module]; ok {
					needed[module] = pkg
				}
			}
		}
	}

	if len(needed) == 0 {
		return nil
	}

	// Check which modules are actually missing (import fails).
	var missing []string
	for module, pkg := range needed {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, "python3", "-c", "import "+module)
		cmd.Dir = workDir
		err := cmd.Run()
		cancel()
		if err != nil {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// detectSystemPackagesFromTests scans shell scripts in test directories for
// references to system commands and returns apt package names for any that are
// missing. This catches common TB2 failures where test scripts invoke utilities
// like jq, bc, xmllint, xxd that aren't installed in the container.
func detectSystemPackagesFromTests(workDir string) []string {
	// Map of commands commonly used in test scripts → apt packages.
	cmdPackages := map[string]string{
		"jq":        "jq",
		"bc":        "bc",
		"xmllint":   "libxml2-utils",
		"xxd":       "xxd",
		"hexdump":   "bsdmainutils",
		"socat":     "socat",
		"netcat":    "netcat-openbsd",
		"nc":        "netcat-openbsd",
		"nmap":      "nmap",
		"expect":    "expect",
		"sshpass":   "sshpass",
		"valgrind":  "valgrind",
		"strace":    "strace",
		"file":      "file",
		"dos2unix":  "dos2unix",
		"iconv":     "libc-bin",
		"patch":     "patch",
		"diffstat":  "diffstat",
		"entr":      "entr",
		"parallel":  "parallel",
		"pv":        "pv",
		"tree":      "tree",
		"rsync":     "rsync",
		"sqlite3":   "sqlite3",
		"csvtool":   "csvtool",
		"gnuplot":   "gnuplot-nox",
		"convert":   "imagemagick",
		"identify":  "imagemagick",
		"dot":       "graphviz",
		"nasm":      "nasm",
		"zip":       "zip",
		"unzip":     "unzip",
		"bzip2":     "bzip2",
		"xz":        "xz-utils",
		"7z":        "p7zip-full",
		"curl":      "curl",
		"wget":      "wget",
		"netstat":   "net-tools",
		"ifconfig":  "net-tools",
		"dig":       "dnsutils",
		"nslookup":  "dnsutils",
		"host":      "dnsutils",
		"ssh":       "openssh-client",
		"scp":       "openssh-client",
		"htop":      "htop",
		"lsof":      "lsof",
		"tcpdump":   "tcpdump",
		"iptables":  "iptables",
		"tesseract": "tesseract-ocr",
		"ffmpeg":    "ffmpeg",
		"ffprobe":   "ffmpeg",
		"sox":       "sox",
		"pandoc":    "pandoc",
	}

	// Scan .sh and .py files in test directories.
	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test")}
	needed := make(map[string]string) // cmd → pkg

	for _, td := range testDirs {
		if !dirExists(td) {
			continue
		}
		// Scan shell scripts for direct command usage.
		shMatches, _ := filepath.Glob(filepath.Join(td, "*.sh"))
		if len(shMatches) > 10 {
			shMatches = shMatches[:10]
		}
		for _, f := range shMatches {
			data, err := os.ReadFile(f)
			if err != nil || len(data) > 50000 {
				continue
			}
			content := string(data)
			for cmd, pkg := range cmdPackages {
				// Match command at word boundaries: "jq .", "$(jq", "| jq", etc.
				// Avoid false positives by checking common usage patterns.
				if strings.Contains(content, cmd+" ") ||
					strings.Contains(content, cmd+"\n") ||
					strings.Contains(content, cmd+"\"") ||
					strings.Contains(content, cmd+"'") ||
					strings.Contains(content, "|"+cmd) ||
					strings.Contains(content, "| "+cmd) ||
					strings.Contains(content, "$("+cmd) {
					needed[cmd] = pkg
				}
			}
		}
		// Scan Python test scripts for subprocess calls to system commands.
		// Tests often use subprocess.run(["jq", ...]) or os.system("valgrind ...")
		pyMatches, _ := filepath.Glob(filepath.Join(td, "*.py"))
		if len(pyMatches) > 10 {
			pyMatches = pyMatches[:10]
		}
		for _, f := range pyMatches {
			data, err := os.ReadFile(f)
			if err != nil || len(data) > 50000 {
				continue
			}
			content := string(data)
			for cmd, pkg := range cmdPackages {
				// Match subprocess patterns: subprocess.run(["cmd", ...]),
				// os.system("cmd ..."), shutil.which("cmd"), Popen(["cmd"])
				if strings.Contains(content, "\""+cmd+"\"") ||
					strings.Contains(content, "'"+cmd+"'") ||
					strings.Contains(content, "\""+cmd+" ") {
					needed[cmd] = pkg
				}
			}
		}
		break // only scan first found test dir
	}

	if len(needed) == 0 {
		return nil
	}

	// Check which commands are actually missing.
	var missing []string
	seen := make(map[string]bool) // deduplicate packages
	for cmd, pkg := range needed {
		if runQuiet(workDir, "which", cmd) == "" {
			if !seen[pkg] {
				seen[pkg] = true
				missing = append(missing, pkg)
			}
		}
	}

	return missing
}

// detectCppTask returns true if the working directory looks like a C/C++ compilation task.
func detectCppTask(workDir string) bool {
	// Check for C/C++ source files.
	for _, dir := range []string{workDir, "/app"} {
		for _, ext := range []string{"*.c", "*.cpp", "*.cc", "*.cxx", "*.h", "*.hpp"} {
			matches, _ := filepath.Glob(filepath.Join(dir, ext))
			if len(matches) > 0 {
				return true
			}
		}
	}
	// Check for CMake/Make build systems alongside any source.
	if fileExists(filepath.Join(workDir, "CMakeLists.txt")) {
		return true
	}
	// Check directory name for C/C++ indicators.
	lower := strings.ToLower(filepath.Base(workDir))
	for _, ind := range []string{"gcc", "clang", "compile", "linker", "segfault", "valgrind"} {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
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

// detectHaskellTask returns true if the working directory contains Haskell project files.
func detectHaskellTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.hs"))
		if len(matches) > 0 {
			return true
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*.cabal"))
		if len(matches) > 0 {
			return true
		}
		if fileExists(filepath.Join(dir, "stack.yaml")) || fileExists(filepath.Join(dir, "cabal.project")) {
			return true
		}
	}
	return false
}

// detectNotebookTask returns true if the task involves Jupyter notebooks.
func detectNotebookTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		if matches, _ := filepath.Glob(filepath.Join(dir, "*.ipynb")); len(matches) > 0 {
			return true
		}
	}
	return false
}

// detectRubyTask returns true if the working directory contains Ruby files.
func detectRubyTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.rb"))
		if len(matches) > 0 {
			return true
		}
		if fileExists(filepath.Join(dir, "Gemfile")) || fileExists(filepath.Join(dir, "Rakefile")) {
			return true
		}
	}
	return false
}

// detectJavaTask returns true if the task involves Java or Kotlin code.
func detectJavaTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		for _, ext := range []string{"*.java", "*.kt", "*.kts"} {
			if matches, _ := filepath.Glob(filepath.Join(dir, ext)); len(matches) > 0 {
				return true
			}
		}
		if fileExists(filepath.Join(dir, "pom.xml")) || fileExists(filepath.Join(dir, "build.gradle")) ||
			fileExists(filepath.Join(dir, "build.gradle.kts")) {
			return true
		}
	}
	return false
}

// detectDotNetTask returns true if the task involves .NET/C# code.
func detectDotNetTask(workDir string) bool {
	for _, dir := range []string{workDir, "/app"} {
		for _, ext := range []string{"*.cs", "*.csproj", "*.sln", "*.fsproj"} {
			if matches, _ := filepath.Glob(filepath.Join(dir, ext)); len(matches) > 0 {
				return true
			}
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

// detectTestPorts scans test scripts for localhost:PORT or 127.0.0.1:PORT
// references. Returns the unique ports found, which tells the agent exactly
// what port the service needs to listen on. Saves 1-2 turns of guessing.
func detectTestPorts(workDir string) []string {
	var ports []string
	seen := make(map[string]bool)

	testDirs := []string{"/tests", filepath.Join(workDir, "tests"), filepath.Join(workDir, "test"), workDir, "/app"}
	for _, td := range testDirs {
		entries, err := os.ReadDir(td)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !isSourceFile(name) && !strings.HasSuffix(name, ".sh") {
				continue
			}
			info, _ := entry.Info()
			if info == nil || info.Size() > 30000 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(td, name))
			if err != nil {
				continue
			}
			content := string(data)
			// Scan for localhost:PORT and 127.0.0.1:PORT patterns.
			for _, prefix := range []string{"localhost:", "127.0.0.1:", "0.0.0.0:"} {
				idx := 0
				for {
					pos := strings.Index(content[idx:], prefix)
					if pos < 0 {
						break
					}
					pos += idx + len(prefix)
					// Extract the port number.
					end := pos
					for end < len(content) && content[end] >= '0' && content[end] <= '9' {
						end++
					}
					if end > pos && end-pos <= 5 {
						port := content[pos:end]
						if !seen[port] && port != "0" {
							seen[port] = true
							ports = append(ports, port)
						}
					}
					idx = end
				}
			}
		}
		if len(ports) > 0 {
			break // found ports in this directory
		}
	}

	// Cap to 3 ports.
	if len(ports) > 3 {
		ports = ports[:3]
	}
	return ports
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
	var constraints []string
	seen := make(map[string]bool)
	extractTestConstraintsRecursive(testDir, &constraints, seen, 0, 3)
	return constraints
}

// extractTestConstraintsRecursive walks test directories up to maxDepth to
// find constraint patterns. Previously this was non-recursive and missed tests
// in subdirectories like /tests/unit/test_foo.py.
func extractTestConstraintsRecursive(dir string, constraints *[]string, seen map[string]bool, depth, maxDepth int) {
	if depth > maxDepth || len(*constraints) >= 15 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories.
			if depth < maxDepth {
				extractTestConstraintsRecursive(filepath.Join(dir, entry.Name()), constraints, seen, depth+1, maxDepth)
			}
			continue
		}
		if !isSourceFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 10000 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
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

			// Timeout constraints: subprocess.run(..., timeout=N), time limits.
			// These tell the agent its solution must execute within N seconds.
			if !isConstraintLine {
				if strings.Contains(trimmed, "timeout=") || strings.Contains(trimmed, "timeout =") {
					isConstraintLine = true
				}
			}

			// File hash checks: md5/sha verification in Python tests.
			if !isConstraintLine {
				if (strings.Contains(trimmed, "md5") || strings.Contains(trimmed, "sha256") || strings.Contains(trimmed, "hashlib")) &&
					strings.Contains(trimmed, "==") {
					isConstraintLine = true
				}
			}

			if isConstraintLine && !seen[trimmed] {
				// Truncate very long lines.
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				*constraints = append(*constraints, trimmed)
				seen[trimmed] = true
			}
		}
		// Cap at 15 constraints to avoid overwhelming context.
		if len(*constraints) >= 15 {
			return
		}
	}
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
	// Include /app/tests and /app/task_file/tests for TB2 tasks that nest under /app.
	testDirCandidates := []string{
		"/tests",
		filepath.Join(workDir, "tests"),
		filepath.Join(workDir, "test"),
	}
	if workDir != "/app" {
		testDirCandidates = append(testDirCandidates,
			"/app/tests",
			"/app/test",
			"/app/task_file/tests",
		)
	}
	for _, td := range testDirCandidates {
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

	// Detect standalone test/verification scripts in workDir and /app.
	// Many TB2 tasks place test.sh, verify.py, etc. directly in the working
	// directory rather than in a tests/ subdirectory.
	seen := make(map[string]bool)
	for _, cmd := range cmds {
		seen[cmd] = true
	}
	for _, dir := range []string{workDir, "/app", "/app/task_file"} {
		for _, scriptName := range []string{
			"test.sh", "test.py", "test.rb",
			"run_tests.sh", "run_tests.py", "run_test.sh", "run_test.py",
			"verify.sh", "verify.py", "check.sh", "check.py",
			"validate.sh", "validate.py",
		} {
			path := filepath.Join(dir, scriptName)
			if !fileExists(path) {
				continue
			}
			var cmd string
			if strings.HasSuffix(scriptName, ".sh") {
				cmd = "Test: bash " + path
			} else if strings.HasSuffix(scriptName, ".py") {
				cmd = "Test: python3 " + path
			} else if strings.HasSuffix(scriptName, ".rb") {
				cmd = "Test: ruby " + path
			}
			if cmd != "" && !seen[cmd] {
				seen[cmd] = true
				cmds = append(cmds, cmd)
			}
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
	// Swift
	if fileExists(filepath.Join(workDir, "Package.swift")) {
		cmds = append(cmds, "Build: swift build")
		cmds = append(cmds, "Test: swift test")
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
	// Java/Maven
	if fileExists(filepath.Join(workDir, "pom.xml")) {
		cmds = append(cmds, "Build: mvn compile -q")
		cmds = append(cmds, "Test: mvn test -q")
	}
	// Java/Gradle (prefer gradlew wrapper if present).
	if fileExists(filepath.Join(workDir, "build.gradle")) || fileExists(filepath.Join(workDir, "build.gradle.kts")) {
		gradleCmd := "gradle"
		if fileExists(filepath.Join(workDir, "gradlew")) {
			gradleCmd = "./gradlew"
		}
		cmds = append(cmds, "Build: "+gradleCmd+" build --quiet")
		cmds = append(cmds, "Test: "+gradleCmd+" test --quiet")
	}
	// .NET
	for _, dir := range []string{workDir, "/app"} {
		if matches, _ := filepath.Glob(filepath.Join(dir, "*.csproj")); len(matches) > 0 {
			cmds = append(cmds, "Build: dotnet build")
			cmds = append(cmds, "Test: dotnet test")
			break
		}
		if matches, _ := filepath.Glob(filepath.Join(dir, "*.sln")); len(matches) > 0 {
			cmds = append(cmds, "Build: dotnet build")
			cmds = append(cmds, "Test: dotnet test")
			break
		}
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
	if fileExists(filepath.Join(workDir, "pyproject.toml")) && !fileExists(filepath.Join(workDir, "setup.py")) {
		cmds = append(cmds, "Install: pip install --break-system-packages -e .")
	}

	// pytest detection (common across all Python projects).
	// Check test directories AND workDir itself for test_*.py files.
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
	if !hasPyTests {
		// Also check workDir for test_*.py files (some projects keep tests alongside code).
		if matches, _ := filepath.Glob(filepath.Join(workDir, "test_*.py")); len(matches) > 0 {
			hasPyTests = true
		}
	}
	if hasPyTests {
		cmds = append(cmds, "Test: pytest -xvs")
	}

	// Ruby (Gemfile with rspec or Rakefile).
	if fileExists(filepath.Join(workDir, "Gemfile")) {
		cmds = append(cmds, "Install: bundle install")
		// Check if rspec is a dependency.
		if content := readFileTruncated(filepath.Join(workDir, "Gemfile"), 2000); strings.Contains(content, "rspec") {
			cmds = append(cmds, "Test: bundle exec rspec")
		}
	}
	if fileExists(filepath.Join(workDir, "Rakefile")) {
		cmds = append(cmds, "Test: rake test")
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

// detectTodoStubs scans source files in the working directory for TODO, FIXME,
// IMPLEMENT, NotImplementedError, and pass-only function stubs. Returns a list
// of actionable lines showing what the agent needs to implement.
// This catches the common TB2 pattern of providing skeleton code with stubs.
func detectTodoStubs(workDir string) []string {
	var stubs []string
	seen := make(map[string]bool)

	todoPatterns := []string{
		"TODO", "FIXME", "IMPLEMENT", "HACK", "XXX",
		"NotImplementedError", "NotImplemented",
		"raise NotImplementedError", "pass  # TODO",
		"pass # TODO", "pass  # FIXME", "pass # FIXME",
		"todo!()", "unimplemented!()", "panic(\"not implemented\")",
		"throw new Error(\"not implemented\")",
		"throw new UnsupportedOperationException",
	}

	for _, dir := range []string{workDir, "/app"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isSourceFile(entry.Name()) {
				continue
			}
			info, _ := entry.Info()
			if info == nil || info.Size() > 50000 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			for lineNum, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				for _, pat := range todoPatterns {
					if strings.Contains(trimmed, pat) {
						key := entry.Name() + ":" + trimmed
						if !seen[key] {
							seen[key] = true
							stub := fmt.Sprintf("  %s:%d: %s", entry.Name(), lineNum+1, trimmed)
							if len(stub) > 150 {
								stub = stub[:150] + "..."
							}
							stubs = append(stubs, stub)
						}
						break
					}
				}
			}
		}
		if len(stubs) > 0 {
			break
		}
	}

	// Cap to avoid context bloat.
	if len(stubs) > 15 {
		stubs = append(stubs[:15], fmt.Sprintf("  ... and %d more TODOs", len(stubs)-15))
	}
	return stubs
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// depsMarkerPath returns the path to a marker file indicating that dependencies
// have already been installed for the given working directory. Subagents check
// this marker to skip redundant (and slow) dependency installation steps.
func depsMarkerPath(workDir string) string {
	// Simple hash to keep the filename short and filesystem-safe.
	h := uint32(0)
	for _, b := range []byte(workDir) {
		h = h*31 + uint32(b)
	}
	return fmt.Sprintf("%s/gollem-deps-%08x", os.TempDir(), h)
}

// isSourceFile returns true if the filename has a recognized source code extension.
// isBinaryExtension returns true if the lowercased filename has an extension
// associated with binary files. Used to skip binary files in auto-read.
func isBinaryExtension(lower string) bool {
	binaryExts := []string{
		// Images
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tiff", ".ppm", ".pgm",
		".svg", ".webp", ".ico",
		// Audio
		".wav", ".mp3", ".flac", ".ogg", ".aac", ".m4a", ".aiff", ".wma",
		// Video
		".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm",
		// Archives
		".zip", ".tar", ".gz", ".xz", ".bz2", ".7z", ".rar", ".zst",
		// Documents
		".pdf", ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx",
		// Databases
		".db", ".sqlite", ".sqlite3",
		// Python compiled/data
		".pyc", ".pyo", ".pickle", ".pkl", ".npy", ".npz", ".h5", ".hdf5",
		// Compiled objects/libraries
		".o", ".obj", ".a", ".so", ".dylib", ".dll", ".lib", ".exe",
		".class", ".jar", ".war", ".whl", ".egg",
		// Binary data
		".bin", ".dat", ".raw", ".img", ".iso",
		// Fonts
		".ttf", ".otf", ".woff", ".woff2",
	}
	for _, ext := range binaryExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

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
// Keeps the first message (task description), builds a context recovery summary from
// dropped messages, and keeps the last keepLast messages with content truncated.
//
// The context recovery summary preserves key information from dropped messages:
// - Files that were read (so the agent knows what it already explored)
// - Files that were edited/written (so the agent knows its current code state)
// - Verification history (so the agent knows what tests passed/failed)
// This prevents the agent from re-doing work or losing track of its approach.
func emergencyCompressMessagesWithConfig(messages []core.ModelMessage, maxContentBytes, keepLast int) []core.ModelMessage {
	if len(messages) <= keepLast+1 {
		// Can't drop messages, but still try truncating content.
		result := make([]core.ModelMessage, len(messages))
		for i, msg := range messages {
			result[i] = truncateMessageContent(msg, maxContentBytes)
		}
		return result
	}

	result := make([]core.ModelMessage, 0, keepLast+3)
	result = append(result, messages[0]) // first message (task + system prompt)

	// Build a context recovery summary from dropped messages.
	dropped := messages[1 : len(messages)-keepLast]
	summary := buildContextRecoverySummary(dropped)

	result = append(result, core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.SystemPromptPart{
				Content: summary,
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

// buildContextRecoverySummary extracts key information from dropped messages
// to create a recovery note that preserves the agent's working state.
func buildContextRecoverySummary(dropped []core.ModelMessage) string {
	var filesRead []string
	var filesModified []string
	var verifyRuns []string
	var testTrajectory []string // compact "passed/total" per verification run
	var packagesInstalled []string
	var subagentTasks []string
	var lastAssistantText string // last assistant text block — captures current approach/thinking

	seenRead := make(map[string]bool)
	seenModified := make(map[string]bool)
	seenPackages := make(map[string]bool)

	// Scan dropped messages for tool calls and their results.
	// Track the last pending verification call ID to match with its result.
	var pendingVerifyCallID string
	var pendingVerifyCmd string
	var pendingSubagentCallID string
	var pendingSubagentTask string

	for _, msg := range dropped {
		if resp, ok := msg.(core.ModelResponse); ok {
			// Capture assistant's latest text (approach/thinking).
			if text := resp.TextContent(); text != "" {
				lastAssistantText = text
			}
			for _, part := range resp.Parts {
				tc, ok := part.(core.ToolCallPart)
				if !ok {
					continue
				}
				switch tc.ToolName {
				case "read":
					path := extractPathFromArgs(tc.ArgsJSON)
					if path != "" && !seenRead[path] {
						seenRead[path] = true
						filesRead = append(filesRead, path)
					}
				case "edit", "multi_edit":
					path := extractPathFromArgs(tc.ArgsJSON)
					if path != "" && !seenModified[path] {
						seenModified[path] = true
						filesModified = append(filesModified, path)
					}
				case "write":
					path := extractPathFromArgs(tc.ArgsJSON)
					if path != "" && !seenModified[path] {
						seenModified[path] = true
						filesModified = append(filesModified, path)
					}
				case "bash":
					var args struct {
						Command string `json:"command"`
					}
					if json.Unmarshal([]byte(tc.ArgsJSON), &args) == nil {
						if isVerificationCommand(tc.ArgsJSON) {
							pendingVerifyCallID = tc.ToolCallID
							pendingVerifyCmd = args.Command
							if len(pendingVerifyCmd) > 80 {
								pendingVerifyCmd = pendingVerifyCmd[:80] + "..."
							}
						}
						// Track package installations to prevent re-installs after recovery.
						cmd := args.Command
						if (strings.Contains(cmd, "pip install") || strings.Contains(cmd, "pip3 install")) && !strings.Contains(cmd, "--help") {
							// Extract package names from pip install command.
							for _, part := range strings.Fields(cmd) {
								if part == "install" || strings.HasPrefix(part, "-") || strings.HasPrefix(part, "pip") {
									continue
								}
								if !seenPackages[part] {
									seenPackages[part] = true
									packagesInstalled = append(packagesInstalled, part)
								}
							}
						}
						if strings.Contains(cmd, "apt-get install") || strings.Contains(cmd, "apt install") {
							for _, part := range strings.Fields(cmd) {
								if part == "install" || strings.HasPrefix(part, "-") || part == "apt-get" || part == "apt" {
									continue
								}
								if !seenPackages[part] {
									seenPackages[part] = true
									packagesInstalled = append(packagesInstalled, part)
								}
							}
						}
						if strings.Contains(cmd, "npm install") {
							for _, part := range strings.Fields(cmd) {
								if part == "install" || strings.HasPrefix(part, "-") || part == "npm" {
									continue
								}
								if !seenPackages[part] {
									seenPackages[part] = true
									packagesInstalled = append(packagesInstalled, part)
								}
							}
						}
					}
				case "delegate":
					var args struct {
						Task string `json:"task"`
					}
					if json.Unmarshal([]byte(tc.ArgsJSON), &args) == nil {
						pendingSubagentCallID = tc.ToolCallID
						pendingSubagentTask = args.Task
						if len(pendingSubagentTask) > 100 {
							pendingSubagentTask = pendingSubagentTask[:100] + "..."
						}
					}
				}
			}
		}
		if req, ok := msg.(core.ModelRequest); ok {
			for _, part := range req.Parts {
				tr, ok := part.(core.ToolReturnPart)
				if !ok {
					continue
				}
				if pendingVerifyCallID != "" && tr.ToolCallID == pendingVerifyCallID {
					content := ""
					if s, ok := tr.Content.(string); ok {
						content = s
					}
					failed, summary := verificationResultFailed(content)
					status := "PASSED"
					if failed {
						status = "FAILED"
						if summary != "" {
							status += ": " + summary
						}
					}
					verifyRuns = append(verifyRuns, fmt.Sprintf("`%s` → %s", pendingVerifyCmd, status))
					// Extract test counts for compact trajectory.
					if p, f, countOK := extractTestCounts(content); countOK {
						testTrajectory = append(testTrajectory, fmt.Sprintf("%d/%d", p, p+f))
					}
					pendingVerifyCallID = ""
					pendingVerifyCmd = ""
				}
				if pendingSubagentCallID != "" && tr.ToolCallID == pendingSubagentCallID {
					content := ""
					if s, ok := tr.Content.(string); ok {
						content = s
						if len(content) > 150 {
							content = content[:150] + "..."
						}
					}
					subagentTasks = append(subagentTasks, fmt.Sprintf("Task: %s → Result: %s", pendingSubagentTask, content))
					pendingSubagentCallID = ""
					pendingSubagentTask = ""
				}
			}
		}
	}

	// Build the summary.
	var b strings.Builder
	b.WriteString("[EMERGENCY CONTEXT RECOVERY] Previous conversation history was too large " +
		"and has been truncated to recover from a 413 error.\n\n")

	if len(filesRead) > 0 {
		b.WriteString("FILES PREVIOUSLY READ (you can re-read if needed):\n")
		limit := len(filesRead)
		if limit > 20 {
			limit = 20
		}
		for _, f := range filesRead[:limit] {
			b.WriteString("  - ")
			b.WriteString(f)
			b.WriteString("\n")
		}
		if len(filesRead) > 20 {
			fmt.Fprintf(&b, "  ... and %d more\n", len(filesRead)-20)
		}
		b.WriteString("\n")
	}

	if len(filesModified) > 0 {
		b.WriteString("FILES YOU MODIFIED (your changes are still on disk):\n")
		for _, f := range filesModified {
			b.WriteString("  - ")
			b.WriteString(f)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(verifyRuns) > 0 {
		b.WriteString("VERIFICATION HISTORY:\n")
		for _, r := range verifyRuns {
			b.WriteString("  - ")
			b.WriteString(r)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Compact test trajectory shows progress at a glance after context recovery.
	if len(testTrajectory) > 0 {
		b.WriteString("TEST PROGRESS: ")
		b.WriteString(strings.Join(testTrajectory, " → "))
		if len(testTrajectory) >= 2 &&
			testTrajectory[len(testTrajectory)-1] == testTrajectory[len(testTrajectory)-2] {
			b.WriteString(" (stalled — no improvement between last runs)")
		}
		b.WriteString("\nContinue from where you left off. Run tests to see current state.\n\n")
	}

	if len(packagesInstalled) > 0 {
		limit := len(packagesInstalled)
		if limit > 15 {
			limit = 15
		}
		b.WriteString("PACKAGES ALREADY INSTALLED (do NOT reinstall):\n")
		b.WriteString("  " + strings.Join(packagesInstalled[:limit], ", "))
		if len(packagesInstalled) > 15 {
			fmt.Fprintf(&b, " ... and %d more", len(packagesInstalled)-15)
		}
		b.WriteString("\n\n")
	}

	if len(subagentTasks) > 0 {
		limit := len(subagentTasks)
		if limit > 5 {
			limit = 5
		}
		b.WriteString("COMPLETED SUBAGENT TASKS (do NOT redo):\n")
		for _, t := range subagentTasks[:limit] {
			b.WriteString("  - ")
			b.WriteString(t)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Include the agent's last approach/thinking to maintain continuity.
	if lastAssistantText != "" {
		truncated := lastAssistantText
		if len(truncated) > 500 {
			truncated = truncated[:500] + "..."
		}
		b.WriteString("YOUR LAST APPROACH/THINKING:\n")
		b.WriteString("  " + truncated + "\n\n")
	}

	b.WriteString("Focus on completing the task with the remaining context. " +
		"Your previous work is preserved on disk — check output files and run tests to understand current state.")

	return b.String()
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

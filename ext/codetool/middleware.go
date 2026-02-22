package codetool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
			// Inject a guidance message.
			loopMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content: "WARNING: You appear to be stuck in a loop, repeatedly editing " +
							strings.Join(loopedFiles, ", ") + ". " +
							"Step back and reconsider your approach. Try a FUNDAMENTALLY DIFFERENT strategy: " +
							"(1) re-read the FULL error output, (2) consider if your approach is wrong, " +
							"(3) try a completely different algorithm or implementation. " +
							"Do NOT keep making small tweaks to the same failing approach.",
					},
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

	// Top-level directory listing (first 30 entries, one level deep).
	if ls := runQuiet(workDir, "ls", "-1"); ls != "" {
		entries := strings.Split(strings.TrimSpace(ls), "\n")
		if len(entries) > 30 {
			entries = append(entries[:30], "... (truncated)")
		}
		parts = append(parts, "Top-level files:\n"+strings.Join(entries, "\n"))
	}

	// Read README if present — many tasks embed critical requirements here.
	readmePaths := []string{
		filepath.Join(workDir, "README.md"),
		filepath.Join(workDir, "README.txt"),
		filepath.Join(workDir, "README"),
		filepath.Join(workDir, "readme.md"),
	}
	for _, rp := range readmePaths {
		if content := readFileTruncated(rp, 3000); content != "" {
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
	// Cap at 30KB total (~8000 tokens) to leave room for the actual conversation.
	autoReadBudget := 30000

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
			// Auto-read test files (up to 5KB each, up to 3 files).
			autoReadBudget = autoReadDirBudget(td, &parts, "Test", 5000, 3, autoReadBudget)
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

	// Auto-read small source files in /app/ — now recursive (depth 3).
	// Reads files < 5KB to avoid overwhelming context, up to 8 files total.
	if autoReadBudget > 0 {
		appSourceDirs := []string{"/app", workDir}
		for _, ad := range appSourceDirs {
			autoReadSourceFilesBudget(ad, &parts, 5000, 8, autoReadBudget)
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
	}

	// Check for output directories that need to be populated.
	outputDirs := []string{
		filepath.Join(workDir, "output_data"),
		"/app/task_file/output_data",
		filepath.Join(workDir, "output"),
	}
	for _, od := range outputDirs {
		if info, err := os.Stat(od); err == nil && info.IsDir() {
			parts = append(parts, "\nOutput directory: "+od+" (your deliverables go here)")
			break
		}
	}

	// Task-type specific guidance based on detected patterns.
	parts = append(parts, detectTaskGuidance(workDir))

	// Available tools.
	parts = append(parts, "\nAvailable tools: bash, view, edit, multi_edit, write, grep, glob, ls, planning, delegate")
	parts = append(parts, "Source files are pre-loaded above. For complex tasks, create a plan first using the planning tool, then proceed.")

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
	return language, buildSystem
}

// runQuiet runs a command in workDir and returns trimmed stdout, or empty
// string on any error. It has a short timeout to avoid blocking agent startup.
func runQuiet(workDir string, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
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

// autoReadSourceFilesBudget reads small source files in a directory recursively
// (up to depth 3), respecting a total byte budget. This ensures the agent sees
// code in subdirectories like src/, lib/, utils/ without wasting turns.
func autoReadSourceFilesBudget(dir string, parts *[]string, maxBytes, maxFiles, budget int) {
	autoReadSourceRecursive(dir, parts, maxBytes, &maxFiles, &budget, 0, 3)
}

// autoReadSourceRecursive walks a directory tree reading source files.
func autoReadSourceRecursive(dir string, parts *[]string, maxBytes int, remaining *int, budget *int, depth, maxDepth int) {
	if depth > maxDepth || *remaining <= 0 || *budget <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Read files first, then recurse into subdirectories.
	for _, entry := range entries {
		if *remaining <= 0 || *budget <= 0 {
			return
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSourceFile(name) {
			continue
		}
		// Skip common non-task files.
		lower := strings.ToLower(name)
		if lower == "readme.md" || lower == "readme.txt" || lower == "readme" {
			continue // Already auto-read separately.
		}
		info, err := entry.Info()
		if err != nil || info.Size() > int64(maxBytes) || info.Size() == 0 {
			continue
		}
		limit := maxBytes
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
		autoReadSourceRecursive(filepath.Join(dir, name), parts, maxBytes, remaining, budget, depth+1, maxDepth)
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
		hints = append(hints, "Strategy: (1) Read and understand the filter code thoroughly, (2) identify what it blocks vs allows, (3) craft payloads that exploit gaps, (4) test each payload against the filter before writing to output.")
	}
	if hasTests && !hasInputData {
		hints = append(hints, "\n## Task Type: Code Implementation")
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

	// Detect Dockerfile/container tasks.
	if fileExists(filepath.Join(workDir, "Dockerfile")) || fileExists(filepath.Join(workDir, "docker-compose.yml")) {
		hints = append(hints, "\n## Note: Docker files detected")
		hints = append(hints, "If the task involves Docker: build and test locally first, then containerize.")
		hints = append(hints, "Don't waste turns debugging Docker networking or GPU passthrough — focus on the core task.")
	}

	if len(hints) > 0 {
		return strings.Join(hints, "\n")
	}
	return ""
}

// discoverVerificationScripts finds test/verification scripts outside of standard test directories.
func discoverVerificationScripts(workDir string) []string {
	var scripts []string
	seen := make(map[string]bool)

	// Check these directories for verification scripts.
	searchDirs := []string{workDir, "/app", "/app/task_file"}
	verifyPatterns := []string{
		"verify*", "check*", "validate*", "test_*", "run_test*",
		"score*", "eval*", "grade*",
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
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			// Look for assertion lines with numeric comparisons.
			if !strings.Contains(trimmed, "assert") {
				continue
			}
			// Skip comments and imports.
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "import") {
				continue
			}
			// Extract constraints with numeric values (size limits, thresholds, etc.)
			hasConstraint := false
			for _, indicator := range []string{"<", ">", "<=", ">=", "==", "size", "bytes", "length", "len(", "count"} {
				if strings.Contains(trimmed, indicator) {
					hasConstraint = true
					break
				}
			}
			if hasConstraint && !seen[trimmed] {
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
		".py", ".js", ".ts", ".go", ".rs", ".c", ".cpp", ".h", ".hpp",
		".java", ".rb", ".sh", ".bash", ".pl", ".lua", ".r", ".R",
		".sql", ".html", ".css", ".json", ".yaml", ".yml", ".toml",
		".xml", ".md", ".txt", ".cfg", ".ini", ".conf",
		".csv", ".tsv", ".jsonl", ".env", ".dockerfile",
		".jsx", ".tsx", ".vue", ".svelte", ".zig", ".nim",
		".kt", ".kts", ".scala", ".ex", ".exs", ".erl", ".hs",
		".jl", ".m", ".swift", ".f90", ".f95",
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
func ProgressTrackingMiddleware(workDir string) core.AgentMiddleware {
	var mu sync.Mutex
	turn := 0
	hasWritten := false
	warned7 := false
	warned15 := false

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
							if tc.ToolName == "write" || tc.ToolName == "multi_edit" || tc.ToolName == "edit" || tc.ToolName == "execute_code" {
								hasWritten = true
								break
							}
							// Also check bash for redirects/tee that create files.
							if tc.ToolName == "bash" {
								var args struct {
									Command string `json:"command"`
								}
								if json.Unmarshal([]byte(tc.ArgsJSON), &args) == nil {
									cmd := args.Command
									if strings.Contains(cmd, " > ") ||
										strings.Contains(cmd, " >> ") ||
										strings.Contains(cmd, " tee ") ||
										strings.Contains(cmd, "echo ") && strings.Contains(cmd, ">") {
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
		w7 := warned7
		w15 := warned15
		if needsWarning && currentTurn >= 7 && !w7 {
			warned7 = true
		}
		if needsWarning && currentTurn >= 15 && !w15 {
			warned15 = true
		}
		mu.Unlock()

		if needsWarning && currentTurn >= 15 && !w15 {
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
		} else if needsWarning && currentTurn >= 7 && !w7 {
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

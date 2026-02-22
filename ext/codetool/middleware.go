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

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		// Track edits by parsing tool call ArgsJSON directly (more reliable
		// than parsing return strings).
		mu.Lock()
		for _, msg := range messages[max(0, len(messages)-2):] {
			if resp, ok := msg.(core.ModelResponse); ok {
				for _, part := range resp.Parts {
					if tc, ok := part.(core.ToolCallPart); ok {
						if tc.ToolName == "edit" || tc.ToolName == "multi_edit" || tc.ToolName == "write" {
							path := extractPathFromArgs(tc.ArgsJSON)
							if path != "" {
								editCounts[path]++
							}
						}
					}
				}
			}
		}

		// Check if any file has been edited too many times.
		var loopedFiles []string
		for path, count := range editCounts {
			if count >= threshold {
				loopedFiles = append(loopedFiles, path)
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

// ContextInjectionMiddleware injects environment context at the start of the
// conversation. It runs a set of bash commands to discover the environment
// (e.g., directory structure, language, available tools) and prepends the
// results as a system-level context message.
func ContextInjectionMiddleware(workDir string) core.AgentMiddleware {
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

	// Auto-read small source files in /app/ — saves 3-5 turns of manual file reading.
	// Only reads files < 5KB to avoid overwhelming context.
	if autoReadBudget > 0 {
		appSourceDirs := []string{"/app", workDir}
		for _, ad := range appSourceDirs {
			autoReadSourceFilesBudget(ad, &parts, 5000, 5, autoReadBudget)
			break // only read from one source directory
		}
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

	// Available tools.
	parts = append(parts, "\nAvailable tools: bash, view, edit, multi_edit, write, grep, glob, ls, planning, delegate")
	parts = append(parts, "Start by reading the task-relevant files. For complex tasks, create a plan first using the planning tool, then proceed with changes.")

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

// autoReadSourceFilesBudget reads small source files in a directory (non-recursive),
// respecting a total byte budget.
func autoReadSourceFilesBudget(dir string, parts *[]string, maxBytes, maxFiles, budget int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
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
		if limit > budget {
			limit = budget
		}
		content := readFileTruncated(filepath.Join(dir, name), limit)
		if content != "" {
			*parts = append(*parts, fmt.Sprintf("\n## Source file auto-read: %s/%s", dir, name))
			*parts = append(*parts, content)
			budget -= len(content)
			count++
		}
	}
}

// isSourceFile returns true if the filename has a recognized source code extension.
func isSourceFile(name string) bool {
	sourceExts := []string{
		".py", ".js", ".ts", ".go", ".rs", ".c", ".cpp", ".h", ".hpp",
		".java", ".rb", ".sh", ".bash", ".pl", ".lua", ".r", ".R",
		".sql", ".html", ".css", ".json", ".yaml", ".yml", ".toml",
		".xml", ".md", ".txt", ".cfg", ".ini", ".conf",
	}
	lower := strings.ToLower(name)
	for _, ext := range sourceExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
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

		// Track whether the agent has created any files via write tool.
		if !hasWritten {
			for _, msg := range messages {
				if resp, ok := msg.(core.ModelResponse); ok {
					for _, part := range resp.Parts {
						if tc, ok := part.(core.ToolCallPart); ok {
							if tc.ToolName == "write" || tc.ToolName == "multi_edit" {
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

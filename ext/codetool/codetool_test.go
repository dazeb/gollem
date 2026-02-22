package codetool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestFile(t, dir, "hello.go", `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`)
	writeTestFile(t, dir, "lib/utils.go", `package lib

func Add(a, b int) int {
	return a + b
}

func Multiply(a, b int) int {
	return a * b
}
`)
	writeTestFile(t, dir, "lib/utils_test.go", `package lib

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("expected 5")
	}
}
`)
	writeTestFile(t, dir, "README.md", `# Test Project

This is a test project.
`)
	return dir
}

func writeTestFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func call(t *testing.T, tool core.Tool, argsJSON string) string {
	t.Helper()
	ctx := context.Background()
	rc := &core.RunContext{}
	result, err := tool.Handler(ctx, rc, argsJSON)
	if err != nil {
		t.Fatalf("tool call failed: %v", err)
	}
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	return s
}

func callErr(t *testing.T, tool core.Tool, argsJSON string) error {
	t.Helper()
	ctx := context.Background()
	rc := &core.RunContext{}
	_, err := tool.Handler(ctx, rc, argsJSON)
	return err
}

func callBashStr(t *testing.T, tool core.Tool, argsJSON string) string {
	t.Helper()
	return call(t, tool, argsJSON)
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected %q NOT to contain %q", s, substr)
	}
}

// --- Truncation Tests ---

func TestTruncateOutput_Short(t *testing.T) {
	result := truncateOutput("short output", 1000)
	if result != "short output" {
		t.Errorf("expected no truncation, got %q", result)
	}
}

func TestTruncateOutput_Long(t *testing.T) {
	// Create a long string with identifiable head and tail.
	input := strings.Repeat("HEAD", 100) + strings.Repeat("MIDDLE", 500) + strings.Repeat("TAIL", 100)
	result := truncateOutput(input, 500)

	if len(result) > 600 { // some slack for the separator
		t.Errorf("result too long: %d bytes", len(result))
	}
	assertContains(t, result, "HEAD")
	assertContains(t, result, "TAIL")
	assertContains(t, result, "truncated")
}

func TestTruncateOutput_ZeroMax(t *testing.T) {
	result := truncateOutput("anything", 0)
	if result != "anything" {
		t.Errorf("expected no truncation with maxLen=0, got %q", result)
	}
}

// --- Bash Tests ---

func TestBash_Echo(t *testing.T) {
	dir := setupTestDir(t)
	tool := Bash(WithWorkDir(dir))
	result := callBashStr(t, tool, `{"command": "echo hello world"}`)
	assertContains(t, result, "hello world")
	// Success: no exit code shown.
	assertNotContains(t, result, "exit code")
}

func TestBash_ExitCode(t *testing.T) {
	tool := Bash()
	result := callBashStr(t, tool, `{"command": "exit 42"}`)
	assertContains(t, result, "[exit code: 42]")
}

func TestBash_Timeout(t *testing.T) {
	tool := Bash(WithBashTimeout(1 * time.Second))
	result := callBashStr(t, tool, `{"command": "sleep 10"}`)
	assertContains(t, result, "timed out")
}

func TestBash_EmptyCommand(t *testing.T) {
	tool := Bash()
	err := callErr(t, tool, `{"command": ""}`)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestBash_WorkDir(t *testing.T) {
	dir := setupTestDir(t)
	tool := Bash(WithWorkDir(dir))
	result := callBashStr(t, tool, `{"command": "ls hello.go"}`)
	assertContains(t, result, "hello.go")
}

func TestBash_Stderr(t *testing.T) {
	tool := Bash()
	result := callBashStr(t, tool, `{"command": "echo err >&2"}`)
	assertContains(t, result, "err")
}

func TestBash_CustomTimeout(t *testing.T) {
	tool := Bash(WithBashTimeout(60 * time.Second))
	result := callBashStr(t, tool, `{"command": "sleep 10", "timeout": 1}`)
	assertContains(t, result, "timed out")
}

func TestBash_BuildTimeout(t *testing.T) {
	// Verify build commands get auto-extended timeout.
	if !isBuildCommand("make -j4") {
		t.Error("expected make to be detected as build command")
	}
	if !isBuildCommand("cargo build --release") {
		t.Error("expected cargo build to be detected as build command")
	}
	if !isBuildCommand("pip install numpy") {
		t.Error("expected pip install to be detected as build command")
	}
	if isBuildCommand("echo hello") {
		t.Error("expected echo NOT to be detected as build command")
	}
}

func TestFormatBashOutput(t *testing.T) {
	// Success with stdout only.
	result := formatBashOutput("hello\n", "", 0, false, 0)
	if result != "hello\n" {
		t.Errorf("stdout only: got %q", result)
	}

	// Success with stderr.
	result = formatBashOutput("out\n", "warn\n", 0, false, 0)
	assertContains(t, result, "out")
	assertContains(t, result, "[stderr]")
	assertContains(t, result, "warn")

	// Error with no output.
	result = formatBashOutput("", "", 1, false, 0)
	assertContains(t, result, "[exit code: 1]")
	assertContains(t, result, "(no output)")

	// Timeout.
	result = formatBashOutput("partial\n", "", 124, true, 120*time.Second)
	assertContains(t, result, "partial")
	assertContains(t, result, "[timed out after")

	// No output, success.
	result = formatBashOutput("", "", 0, false, 0)
	if result != "(no output)" {
		t.Errorf("empty success: got %q", result)
	}
}

func TestModuleNotFoundHint(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"simple module", "ModuleNotFoundError: No module named 'numpy'", "[hint: try: pip install --break-system-packages numpy]"},
		{"aliased module", "ModuleNotFoundError: No module named 'cv2'", "[hint: try: pip install --break-system-packages opencv-python]"},
		{"submodule", "ModuleNotFoundError: No module named 'sklearn.ensemble'", "[hint: try: pip install --break-system-packages scikit-learn]"},
		{"double quotes", `ModuleNotFoundError: No module named "yaml"`, "[hint: try: pip install --break-system-packages PyYAML]"},
		{"no match", "some random error output", ""},
		{"PIL alias", "ModuleNotFoundError: No module named 'PIL'", "[hint: try: pip install --break-system-packages Pillow]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := moduleNotFoundHint(tt.output)
			if got != tt.want {
				t.Errorf("moduleNotFoundHint(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestTransientErrorHint(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		want     string
	}{
		{"externally managed", "error: externally-managed-environment\n× pip install failed", 1, "[hint: add --break-system-packages flag to pip install]"},
		{"dpkg lock", "E: Could not get lock /var/lib/dpkg/lock", 100, "[hint: try: dpkg --configure -a && apt-get install -f]"},
		{"network error", "Temporary failure resolving 'archive.ubuntu.com'", 100, "[hint: network error — this container may not have internet access. Use only locally available packages and tools. For Python: check if the package is already installed with 'python3 -c \"import <module>\"'. For apt: try 'dpkg -l | grep <package>' to check installed packages]"},
		{"permission denied /usr", "bash: /usr/local/bin/foo: Permission denied", 126, "[hint: try running with sudo or use --user flag for pip]"},
		{"no match", "some other error", 1, ""},
		{"success ignores", "externally-managed-environment", 0, "[hint: add --break-system-packages flag to pip install]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transientErrorHint(tt.output, tt.exitCode)
			if got != tt.want {
				t.Errorf("transientErrorHint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignalHint(t *testing.T) {
	tests := []struct {
		exitCode int
		contains string
	}{
		{137, "SIGKILL"},
		{139, "SIGSEGV"},
		{134, "SIGABRT"},
		{1, ""},
		{0, ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("exit_%d", tt.exitCode), func(t *testing.T) {
			got := signalHint(tt.exitCode)
			if tt.contains == "" && got != "" {
				t.Errorf("signalHint(%d) = %q, want empty", tt.exitCode, got)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("signalHint(%d) = %q, want containing %q", tt.exitCode, got, tt.contains)
			}
		})
	}
}

func TestTestResultSummary(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		contains string
	}{
		{
			"pytest summary",
			"test_foo.py::test_a PASSED\ntest_foo.py::test_b FAILED\n======= 1 passed, 1 failed =======",
			"1 passed, 1 failed",
		},
		{
			"go test failures",
			"--- FAIL: TestFoo (0.01s)\n--- FAIL: TestBar (0.02s)\nFAIL\tgithub.com/example",
			"2 test(s) FAILED",
		},
		{
			"no test output",
			"hello world\nsome random output\ndone",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := testResultSummary(tt.output)
			if tt.contains == "" && got != "" {
				t.Errorf("testResultSummary() = %q, want empty", got)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("testResultSummary() = %q, want containing %q", got, tt.contains)
			}
		})
	}
}

func TestCompilationErrorSummary(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		contains string
	}{
		{
			"gcc errors",
			"main.c:10:5: error: expected ';' after expression\nmain.c:15:1: error: unknown type name 'foo'",
			1,
			"2 error(s) found",
		},
		{
			"success output",
			"main.c:10:5: error: something",
			0,
			"",
		},
		{
			"no errors",
			"Building project...\nDone.",
			1,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compilationErrorSummary(tt.output, tt.exitCode)
			if tt.contains == "" && got != "" {
				t.Errorf("compilationErrorSummary() = %q, want empty", got)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("compilationErrorSummary() = %q, want containing %q", got, tt.contains)
			}
		})
	}
}

// --- View Tests ---

func TestView_ReadFile(t *testing.T) {
	dir := setupTestDir(t)
	tool := View(WithWorkDir(dir))
	result := call(t, tool, `{"path": "hello.go"}`)
	assertContains(t, result, "Hello, World!")
	assertContains(t, result, "package main")
}

func TestView_LineNumbers(t *testing.T) {
	dir := setupTestDir(t)
	tool := View(WithWorkDir(dir))
	result := call(t, tool, `{"path": "hello.go"}`)
	assertContains(t, result, "1\t")
}

func TestView_Offset(t *testing.T) {
	dir := setupTestDir(t)
	tool := View(WithWorkDir(dir))
	result := call(t, tool, `{"path": "hello.go", "offset": 5}`)
	assertNotContains(t, result, "package main")
	assertContains(t, result, "Println")
}

func TestView_Limit(t *testing.T) {
	dir := setupTestDir(t)
	tool := View(WithWorkDir(dir))
	result := call(t, tool, `{"path": "hello.go", "limit": 2}`)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	// Should have 2 content lines (possibly + a truncation message)
	contentLines := 0
	for _, l := range lines {
		if !strings.HasPrefix(l, "...") {
			contentLines++
		}
	}
	if contentLines != 2 {
		t.Errorf("expected 2 content lines, got %d", contentLines)
	}
}

func TestView_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	tool := View(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "nonexistent.go"}`)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestView_Directory(t *testing.T) {
	dir := setupTestDir(t)
	tool := View(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "lib"}`)
	if err == nil {
		t.Error("expected error for directory")
	}
}

func TestView_EmptyPath(t *testing.T) {
	tool := View()
	err := callErr(t, tool, `{"path": ""}`)
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestView_BinaryFile(t *testing.T) {
	dir := setupTestDir(t)
	// Write a binary file with null bytes.
	binPath := filepath.Join(dir, "binary.dat")
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG header
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 pixel
	}
	os.WriteFile(binPath, data, 0o644)

	tool := View(WithWorkDir(dir))
	result := call(t, tool, `{"path":"binary.dat"}`)
	assertContains(t, result, "Binary file")
}

func TestView_TotalLineCount(t *testing.T) {
	dir := setupTestDir(t)
	// Create a file with 20 lines.
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	os.WriteFile(filepath.Join(dir, "long.txt"), []byte(strings.Join(lines, "\n")), 0o644)

	tool := View(WithWorkDir(dir))
	// Read only first 5 lines.
	result := call(t, tool, `{"path":"long.txt","limit":5}`)
	assertContains(t, result, "line 1")
	assertContains(t, result, "line 5")
	// Should show total line count.
	assertContains(t, result, "20 total lines")
}

// --- Write Tests ---

func TestWrite_NewFile(t *testing.T) {
	dir := setupTestDir(t)
	tool := Write(WithWorkDir(dir))
	result := call(t, tool, `{"path": "new.txt", "content": "hello new file"}`)
	assertContains(t, result, "Wrote")

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello new file" {
		t.Errorf("file content mismatch: %q", data)
	}
}

func TestWrite_CreatesDirs(t *testing.T) {
	dir := setupTestDir(t)
	tool := Write(WithWorkDir(dir))
	call(t, tool, `{"path": "deep/nested/file.txt", "content": "deep content"}`)

	data, err := os.ReadFile(filepath.Join(dir, "deep/nested/file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "deep content" {
		t.Errorf("file content mismatch: %q", data)
	}
}

func TestWrite_Overwrite(t *testing.T) {
	dir := setupTestDir(t)
	tool := Write(WithWorkDir(dir))
	call(t, tool, `{"path": "hello.go", "content": "overwritten"}`)

	data, err := os.ReadFile(filepath.Join(dir, "hello.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "overwritten" {
		t.Errorf("file content mismatch: %q", data)
	}
}

func TestWrite_EmptyPath(t *testing.T) {
	tool := Write()
	err := callErr(t, tool, `{"path": ""}`)
	if err == nil {
		t.Error("expected error for empty path")
	}
}

// --- Edit Tests ---

func TestEdit_SimpleReplace(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	result := call(t, tool, `{"path": "hello.go", "old_string": "Hello, World!", "new_string": "Hello, Gollem!"}`)
	assertContains(t, result, "Replaced 1")
	// Should show context around the edit.
	assertContains(t, result, "Hello, Gollem!")
	assertContains(t, result, "Context:")

	data, _ := os.ReadFile(filepath.Join(dir, "hello.go"))
	assertContains(t, string(data), "Hello, Gollem!")
}

func TestEdit_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "hello.go", "old_string": "DOES NOT EXIST", "new_string": "replacement"}`)
	if err == nil {
		t.Error("expected error when old_string not found")
	}
}

func TestEdit_WhitespaceAutoCorrect(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	// hello.go uses tabs, but we'll try with spaces — should auto-correct.
	result := call(t, tool, `{"path": "hello.go", "old_string": "  fmt.Println(\"Hello, World!\")", "new_string": "  fmt.Println(\"Hi\")"}`)
	assertContains(t, result, "auto-corrected whitespace")
	// Verify the edit was applied with the file's tab indentation.
	content, err := os.ReadFile(filepath.Join(dir, "hello.go"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(content), "\tfmt.Println(\"Hi\")")
	assertNotContains(t, string(content), "Hello, World!")
}

func TestDetectWhitespaceMismatch(t *testing.T) {
	content := "func main() {\n\tfmt.Println(\"hello\")\n}\n"

	// Spaces instead of tab — should detect mismatch.
	search := "    fmt.Println(\"hello\")"
	hint := detectWhitespaceMismatch(content, search)
	if hint == "" {
		t.Error("expected whitespace mismatch hint for spaces vs tab")
	}
	assertContains(t, hint, "Whitespace mismatch")
	assertContains(t, hint, "fmt.Println")

	// Exact match — should return empty (no mismatch).
	search2 := "\tfmt.Println(\"hello\")"
	hint2 := detectWhitespaceMismatch(content, search2)
	if hint2 != "" {
		t.Errorf("expected empty hint for exact match, got %q", hint2)
	}

	// Totally different content — should return empty.
	hint3 := detectWhitespaceMismatch(content, "completely different")
	if hint3 != "" {
		t.Errorf("expected empty hint for non-matching content, got %q", hint3)
	}
}

func TestAutoCorrectWhitespace(t *testing.T) {
	// Tab-indented content, space-indented search.
	content := "func main() {\n\tfmt.Println(\"hello\")\n\tfmt.Println(\"world\")\n}\n"

	t.Run("spaces_to_tabs", func(t *testing.T) {
		oldStr := "    fmt.Println(\"hello\")\n    fmt.Println(\"world\")"
		newStr := "    fmt.Println(\"HI\")\n    fmt.Println(\"WORLD\")"
		actualOld, adjustedNew, ok := autoCorrectWhitespace(content, oldStr, newStr)
		if !ok {
			t.Fatal("expected auto-correct to succeed")
		}
		assertContains(t, actualOld, "\tfmt.Println(\"hello\")")
		assertContains(t, adjustedNew, "\tfmt.Println(\"HI\")")
		assertContains(t, adjustedNew, "\tfmt.Println(\"WORLD\")")
	})

	t.Run("exact_match_returns_false", func(t *testing.T) {
		oldStr := "\tfmt.Println(\"hello\")\n\tfmt.Println(\"world\")"
		newStr := "\tfmt.Println(\"HI\")"
		_, _, ok := autoCorrectWhitespace(content, oldStr, newStr)
		if ok {
			t.Error("expected no auto-correct for exact match")
		}
	})

	t.Run("no_match_returns_false", func(t *testing.T) {
		_, _, ok := autoCorrectWhitespace(content, "completely different", "new")
		if ok {
			t.Error("expected no auto-correct for non-matching content")
		}
	})

	t.Run("ambiguous_match_returns_false", func(t *testing.T) {
		// Content with duplicate lines when normalized.
		dupContent := "if true {\n\tfoo()\n}\nif false {\n\tfoo()\n}\n"
		_, _, ok := autoCorrectWhitespace(dupContent, "    foo()", "    bar()")
		if ok {
			t.Error("expected no auto-correct for ambiguous match")
		}
	})

	t.Run("indent_change_preserved", func(t *testing.T) {
		// Model increases indent: 4 spaces → 8 spaces (old) should map to tab → double tab.
		oldStr := "    fmt.Println(\"hello\")"
		newStr := "        if true {\n            fmt.Println(\"hello\")\n        }"
		_, adjustedNew, ok := autoCorrectWhitespace(content, oldStr, newStr)
		if !ok {
			t.Fatal("expected auto-correct to succeed")
		}
		// The new string should have tab-based indentation, not spaces.
		if strings.Contains(adjustedNew, "    ") {
			t.Errorf("expected tab indentation in adjusted new, got: %q", adjustedNew)
		}
	})
}

func TestEdit_AmbiguousMatch(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "lib/utils.go", "old_string": "return a", "new_string": "return x"}`)
	if err == nil {
		t.Error("expected error for ambiguous match")
	}
}

func TestEdit_ReplaceAll(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	result := call(t, tool, `{"path": "lib/utils.go", "old_string": "return a", "new_string": "return x", "replace_all": true}`)
	assertContains(t, result, "Replaced 2")
}

func TestEdit_Delete(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	call(t, tool, `{"path": "hello.go", "old_string": "import \"fmt\"\n\n", "new_string": ""}`)

	data, _ := os.ReadFile(filepath.Join(dir, "hello.go"))
	assertNotContains(t, string(data), "import")
}

func TestEdit_FileNotExist(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "nope.go", "old_string": "a", "new_string": "b"}`)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEdit_SameStrings(t *testing.T) {
	dir := setupTestDir(t)
	tool := Edit(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "hello.go", "old_string": "main", "new_string": "main"}`)
	if err == nil {
		t.Error("expected error when old_string equals new_string")
	}
}

// --- MultiEdit Tests ---

func TestMultiEdit(t *testing.T) {
	dir := setupTestDir(t)
	tool := MultiEdit(WithWorkDir(dir))
	args := `{"edits": [
		{"path": "hello.go", "old_string": "Hello, World!", "new_string": "Hi!"},
		{"path": "lib/utils.go", "old_string": "func Add", "new_string": "func Sum"}
	]}`
	result := call(t, tool, args)
	// Multi-edit now shows per-edit context.
	assertContains(t, result, "Replaced 1 occurrence(s) in hello.go")
	assertContains(t, result, "Replaced 1 occurrence(s) in lib/utils.go")
	assertContains(t, result, "Hi!")
	assertContains(t, result, "func Sum")

	data1, _ := os.ReadFile(filepath.Join(dir, "hello.go"))
	assertContains(t, string(data1), "Hi!")

	data2, _ := os.ReadFile(filepath.Join(dir, "lib/utils.go"))
	assertContains(t, string(data2), "func Sum")
}

func TestMultiEdit_Empty(t *testing.T) {
	tool := MultiEdit()
	err := callErr(t, tool, `{"edits": []}`)
	if err == nil {
		t.Error("expected error for empty edits")
	}
}

// --- Grep Tests ---

func TestGrep_FindPattern(t *testing.T) {
	dir := setupTestDir(t)
	tool := Grep(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "func.*Add"}`)
	assertContains(t, result, "utils.go")
	assertContains(t, result, "func Add")
}

func TestGrep_WithInclude(t *testing.T) {
	dir := setupTestDir(t)
	tool := Grep(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "[Tt]est", "include": "*.md"}`)
	assertContains(t, result, "README.md")
	assertNotContains(t, result, "utils_test.go")
}

func TestGrep_NoMatch(t *testing.T) {
	dir := setupTestDir(t)
	tool := Grep(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "ZZZZNOTEXIST"}`)
	assertContains(t, result, "No matches")
}

func TestGrep_InvalidRegex(t *testing.T) {
	dir := setupTestDir(t)
	tool := Grep(WithWorkDir(dir))
	err := callErr(t, tool, `{"pattern": "[invalid"}`)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestGrep_ContextLines(t *testing.T) {
	dir := setupTestDir(t)
	tool := Grep(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "func Add", "context_lines": 1}`)
	assertContains(t, result, "return a + b")
}

func TestGrep_EmptyPattern(t *testing.T) {
	tool := Grep()
	err := callErr(t, tool, `{"pattern": ""}`)
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

// --- Glob Tests ---

func TestGlob_FindGoFiles(t *testing.T) {
	dir := setupTestDir(t)
	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "**/*.go"}`)
	assertContains(t, result, "hello.go")
	assertContains(t, result, "utils.go")
}

func TestGlob_SubdirOnly(t *testing.T) {
	dir := setupTestDir(t)
	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "lib/*.go"}`)
	assertContains(t, result, "utils.go")
	assertNotContains(t, result, "hello.go")
}

func TestGlob_NoMatch(t *testing.T) {
	dir := setupTestDir(t)
	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "**/*.rs"}`)
	assertContains(t, result, "No files matched")
}

func TestGlob_SimplePattern(t *testing.T) {
	dir := setupTestDir(t)
	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "*.go"}`)
	assertContains(t, result, "hello.go")
	assertNotContains(t, result, "utils.go") // Not in root
}

func TestGlob_EmptyPattern(t *testing.T) {
	tool := Glob()
	err := callErr(t, tool, `{"pattern": ""}`)
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

// --- Ls Tests ---

func TestLs_RootDir(t *testing.T) {
	dir := setupTestDir(t)
	tool := Ls(WithWorkDir(dir))
	result := call(t, tool, `{}`)
	assertContains(t, result, "hello.go")
	assertContains(t, result, "lib/")
	assertContains(t, result, "README.md")
}

func TestLs_Depth2(t *testing.T) {
	dir := setupTestDir(t)
	tool := Ls(WithWorkDir(dir))
	result := call(t, tool, `{"depth": 2}`)
	assertContains(t, result, "lib/utils.go")
}

func TestLs_SubDir(t *testing.T) {
	dir := setupTestDir(t)
	tool := Ls(WithWorkDir(dir))
	result := call(t, tool, `{"path": "lib"}`)
	assertContains(t, result, "utils.go")
	assertNotContains(t, result, "hello.go")
}

func TestLs_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	tool := Ls(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "nonexistent"}`)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestLs_File(t *testing.T) {
	dir := setupTestDir(t)
	tool := Ls(WithWorkDir(dir))
	err := callErr(t, tool, `{"path": "hello.go"}`)
	if err == nil {
		t.Error("expected error for file path")
	}
}

// --- Toolset Tests ---

func TestToolset_AllTools(t *testing.T) {
	ts := Toolset()
	if ts.Name != "codetool" {
		t.Errorf("expected toolset name 'codetool', got %q", ts.Name)
	}
	if len(ts.Tools) != 8 {
		t.Errorf("expected 8 tools, got %d", len(ts.Tools))
	}

	names := make(map[string]bool)
	for _, tool := range ts.Tools {
		names[tool.Definition.Name] = true
	}

	expected := []string{"bash", "view", "write", "edit", "multi_edit", "grep", "glob", "ls"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestAllTools_Count(t *testing.T) {
	tools := AllTools()
	if len(tools) != 8 {
		t.Errorf("expected 8 tools, got %d", len(tools))
	}
}

func TestToolset_WithOptions(t *testing.T) {
	dir := setupTestDir(t)
	ts := Toolset(WithWorkDir(dir))
	// Verify tools work with the configured workdir.
	for _, tool := range ts.Tools {
		if tool.Definition.Name == "ls" {
			ctx := context.Background()
			rc := &core.RunContext{}
			result, err := tool.Handler(ctx, rc, `{}`)
			if err != nil {
				t.Fatalf("ls failed: %v", err)
			}
			s, ok := result.(string)
			if !ok {
				t.Fatalf("expected string result, got %T", result)
			}
			assertContains(t, s, "hello.go")
		}
	}
}

// --- Doublestar matching tests ---

func TestMatchDoublestar(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.go", "hello.go", true},
		{"**/*.go", "lib/utils.go", true},
		{"**/*.go", "a/b/c/d.go", true},
		{"**/*.go", "hello.py", false},
		{"*.go", "hello.go", true},
		{"*.go", "lib/hello.go", false},
		{"lib/**/*.go", "lib/utils.go", true},
		{"lib/**/*.go", "lib/sub/deep.go", true},
		{"lib/**/*.go", "hello.go", false},
		{"src/**/test_*.py", "src/test_main.py", true},
		{"src/**/test_*.py", "src/sub/test_utils.py", true},
	}

	for _, tt := range tests {
		got := matchDoublestar(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchDoublestar(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// --- Verification Checkpoint Tests ---

func TestIsVerificationCommand(t *testing.T) {
	tests := []struct {
		name     string
		argsJSON string
		want     bool
	}{
		{"go test", `{"command":"go test ./..."}`, true},
		{"go build", `{"command":"go build ./..."}`, true},
		{"go vet", `{"command":"go vet ./..."}`, true},
		{"pytest", `{"command":"pytest tests/"}`, true},
		{"npm test", `{"command":"npm test"}`, true},
		{"yarn test", `{"command":"yarn test"}`, true},
		{"cargo test", `{"command":"cargo test"}`, true},
		{"cargo clippy", `{"command":"cargo clippy"}`, true},
		{"make test", `{"command":"make test"}`, true},
		{"make (build)", `{"command":"make"}`, true},
		{"eslint", `{"command":"npx eslint src/"}`, true},
		{"golangci-lint", `{"command":"golangci-lint run"}`, true},
		{"tsc", `{"command":"tsc --noEmit"}`, true},
		{"mypy", `{"command":"mypy src/"}`, true},
		{"mvn test", `{"command":"mvn test"}`, true},
		{"gradle test", `{"command":"gradle test"}`, true},
		{"dotnet test", `{"command":"dotnet test"}`, true},
		{"mixed case", `{"command":"Go Test ./..."}`, true},

		// Non-verification commands.
		{"echo", `{"command":"echo hello"}`, false},
		{"ls", `{"command":"ls -la"}`, false},
		{"cat", `{"command":"cat file.txt"}`, false},
		{"git status", `{"command":"git status"}`, false},
		{"cd", `{"command":"cd /tmp"}`, false},
		{"curl", `{"command":"curl https://example.com"}`, false},
		{"invalid json", `not json`, false},
		{"empty command", `{"command":""}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVerificationCommand(tt.argsJSON)
			if got != tt.want {
				t.Errorf("isVerificationCommand(%s) = %v, want %v", tt.argsJSON, got, tt.want)
			}
		})
	}
}

func TestVerificationCheckpoint_RejectsWithoutVerification(t *testing.T) {
	_, validator := VerificationCheckpoint()

	ctx := context.Background()
	rc := &core.RunContext{}

	_, err := validator(ctx, rc, "I'm done with the task.")
	if err == nil {
		t.Fatal("expected error when no verification was done")
	}

	var retryErr *core.ModelRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected ModelRetryError, got %T: %v", err, err)
	}
	assertContains(t, retryErr.Message, "verify")
}

func TestVerificationCheckpoint_AcceptsAfterVerification(t *testing.T) {
	mw, validator := VerificationCheckpoint()

	ctx := context.Background()

	// Simulate a conversation where the model called bash with "go test".
	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Fix the bug"},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{
					ToolName:   "bash",
					ArgsJSON:   `{"command":"go test ./..."}`,
					ToolCallID: "call1",
				},
			},
		},
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.ToolReturnPart{
					ToolName:   "bash",
					Content:    "ok\nPASS",
					ToolCallID: "call1",
				},
			},
		},
	}

	// Run the middleware so it scans the messages.
	nextCalled := false
	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		nextCalled = true
		return &core.ModelResponse{}, nil
	}
	_, err := mw(ctx, messages, nil, nil, next)
	if err != nil {
		t.Fatalf("middleware error: %v", err)
	}
	if !nextCalled {
		t.Fatal("middleware did not call next")
	}

	// First validator call should trigger pre-completion checklist (retry).
	rc := &core.RunContext{}
	_, err = validator(ctx, rc, "Done! All tests pass.")
	if err == nil {
		t.Fatal("first validator call should trigger pre-completion checklist retry")
	}
	var retryErr *core.ModelRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected ModelRetryError for checklist, got: %v", err)
	}

	// Second validator call should accept.
	output, err := validator(ctx, rc, "Done! All tests pass.")
	if err != nil {
		t.Fatalf("validator should accept on second call after verification, got: %v", err)
	}
	if output != "Done! All tests pass." {
		t.Errorf("validator modified output: %q", output)
	}
}

func TestVerificationCheckpoint_IgnoresNonBashTools(t *testing.T) {
	mw, validator := VerificationCheckpoint()

	ctx := context.Background()

	// Simulate a conversation with edit and view calls but no bash or execute_code.
	messages := []core.ModelMessage{
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{
					ToolName:   "edit",
					ArgsJSON:   `{"path":"main.go","old_string":"foo","new_string":"bar"}`,
					ToolCallID: "call1",
				},
				core.ToolCallPart{
					ToolName:   "view",
					ArgsJSON:   `{"path":"main.go"}`,
					ToolCallID: "call2",
				},
			},
		},
	}

	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		return &core.ModelResponse{}, nil
	}
	_, _ = mw(ctx, messages, nil, nil, next)

	// Should still reject — no bash or verification execute_code.
	rc := &core.RunContext{}
	_, err := validator(ctx, rc, "Done!")
	if err == nil {
		t.Fatal("expected error when only edit/view tools were used")
	}
}

func TestVerificationCheckpoint_AcceptsExecuteCode(t *testing.T) {
	mw, validator := VerificationCheckpoint()

	ctx := context.Background()

	// Simulate a conversation where the model used execute_code with bash() to verify.
	messages := []core.ModelMessage{
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{
					ToolName:   "execute_code",
					ArgsJSON:   `{"code":"result = bash(command='python /app/test_outputs.py')\nresult"}`,
					ToolCallID: "call1",
				},
			},
		},
	}

	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		return &core.ModelResponse{}, nil
	}
	_, _ = mw(ctx, messages, nil, nil, next)

	// First validator call triggers checklist.
	rc := &core.RunContext{}
	_, err := validator(ctx, rc, "Done!")
	if err == nil {
		t.Fatal("first call should trigger pre-completion checklist")
	}

	// Second call should accept.
	_, err = validator(ctx, rc, "Done!")
	if err != nil {
		t.Fatalf("should accept after execute_code verification, got: %v", err)
	}
}

func TestIsVerificationCode(t *testing.T) {
	tests := []struct {
		name     string
		argsJSON string
		want     bool
	}{
		{"bash call with test", `{"code":"bash(command='pytest')"}`, true},
		{"bash call with test_outputs", `{"code":"bash(command='python /app/test_outputs.py')"}`, true},
		{"assert statement", `{"code":"assert result == expected"}`, true},
		{"assertEqual", `{"code":"self.assertEqual(output, expected)"}`, true},
		{"open output file", `{"code":"with open('output.csv') as f:\n    data = f.read()"}`, true},
		{"simple math", `{"code":"x = 1 + 2\nx"}`, false},
		{"view call only", `{"code":"view(path='main.py')"}`, false},
		{"invalid json", `not json`, false},
		{"empty code", `{"code":""}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVerificationCode(tt.argsJSON)
			if got != tt.want {
				t.Errorf("isVerificationCode(%s) = %v, want %v", tt.argsJSON, got, tt.want)
			}
		})
	}
}

func TestIsPipCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"pip install numpy", true},
		{"pip3 install --break-system-packages scipy", true},
		{"python3 -m pip install torch", true},
		{"python -m pip install -r requirements.txt", true},
		{"sudo pip install pandas", true},
		{"echo hello", false},
		{"pip freeze", false},
		{"npm install", false},
		{"apt-get install python3-pip", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := isPipCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("isPipCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestIsDestructiveTestCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		// Destructive operations — should be blocked.
		{"redirect to tests", "echo hello > /tests/test.sh", true},
		{"append to tests", "echo hello >> /tests/test.sh", true},
		{"rm tests file", "rm /tests/test.sh", true},
		{"rm -rf tests", "rm -rf /tests/", true},
		{"sed -i on tests", "sed -i 's/old/new/' /tests/test.py", true},
		{"chmod tests", "chmod +x /tests/test.sh", true},
		{"tee to tests", "echo data | tee /tests/out.txt", true},
		{"truncate tests", "truncate -s 0 /tests/test.sh", true},

		// Non-destructive operations — should be allowed.
		{"run test script", "bash /tests/test.sh", false},
		{"run python test", "python3 /tests/test.py", false},
		{"cat test file", "cat /tests/test.sh", false},
		{"ls tests dir", "ls /tests/", false},
		{"head test file", "head -n 10 /tests/test.py", false},
		{"diff with tests", "diff output.txt /tests/expected.txt", false},
		{"grep in tests", "grep -r 'pattern' /tests/", false},
		{"no tests ref", "echo hello > /app/output.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDestructiveTestCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("isDestructiveTestCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestIsTransientBashFailure(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		output   string
		want     bool
	}{
		{"network error", 1, "Could not resolve host: example.com", true},
		{"connection timeout", 1, "Connection timed out", true},
		{"dpkg lock", 1, "unable to acquire the dpkg frontend lock", true},
		{"hash sum mismatch", 1, "Hash sum mismatch", true},
		{"failed to fetch", 100, "E: Failed to fetch http://archive.ubuntu.com/", true},
		{"success", 0, "all good", false},
		{"normal error", 1, "syntax error near unexpected token", false},
		{"test failure", 1, "FAILED test_something", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientBashFailure(tt.exitCode, tt.output)
			if got != tt.want {
				t.Errorf("isTransientBashFailure(%d, %q) = %v, want %v", tt.exitCode, tt.output, got, tt.want)
			}
		})
	}
}

func TestBash_BlocksDestructiveTestCommand(t *testing.T) {
	tool := Bash()
	err := callErr(t, tool, `{"command":"echo pwned > /tests/test.sh"}`)
	if err == nil {
		t.Fatal("expected error for destructive test command")
	}
	var retryErr *core.ModelRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected ModelRetryError, got %T: %v", err, err)
	}
	if !strings.Contains(retryErr.Message, "BLOCKED") {
		t.Errorf("expected BLOCKED message, got: %s", retryErr.Message)
	}
}

func TestBash_AllowsRunningTests(t *testing.T) {
	// Running tests from /tests/ should not be blocked.
	tool := Bash()
	// Use a simple echo to verify the command runs (bash /tests/... would fail
	// because /tests/ doesn't exist, but it should not be blocked by our check).
	result := call(t, tool, `{"command":"echo 'bash /tests/test.sh would run here'"}`)
	assertContains(t, result, "bash /tests/test.sh would run here")
}

func TestIsProtectedTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/tests/test.sh", true},
		{"/tests/unit/test_check.py", true},
		{"/tests/e2e/verify.sh", true},
		{"/tests", true},
		{"/app/tests/test.py", false},       // not root /tests/
		{"/home/user/tests/foo.py", false},   // not root /tests/
		{"/src/main.py", false},              // unrelated
		{"/app/solution.py", false},          // unrelated
		{"tests/test.sh", false},             // relative, not /tests/
		{"/testing/foo.py", false},           // /testing != /tests
		{"/tests/../app/foo.py", false},      // cleaned to /app/foo.py
		{"/tests/./nested/test.sh", true},    // cleaned to /tests/nested/test.sh
	}
	for _, tt := range tests {
		got := isProtectedTestFile(tt.path)
		if got != tt.want {
			t.Errorf("isProtectedTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestEdit_ProtectedTestFile(t *testing.T) {
	// Edit should block modifications to /tests/ files.
	tool := Edit()
	err := callErr(t, tool, `{"path":"/tests/test.sh","old_string":"echo hello","new_string":"echo bye"}`)
	if err == nil {
		t.Fatal("expected error for protected test file")
	}
	var retryErr *core.ModelRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected ModelRetryError, got %T: %v", err, err)
	}
	if !strings.Contains(retryErr.Message, "BLOCKED") {
		t.Errorf("expected BLOCKED message, got: %s", retryErr.Message)
	}
}

func TestWrite_ProtectedTestFile(t *testing.T) {
	// Write should block creation/overwrite of /tests/ files.
	tool := Write()
	err := callErr(t, tool, `{"path":"/tests/test_new.py","content":"print('hello')"}`)
	if err == nil {
		t.Fatal("expected error for protected test file")
	}
	var retryErr *core.ModelRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected ModelRetryError, got %T: %v", err, err)
	}
	if !strings.Contains(retryErr.Message, "BLOCKED") {
		t.Errorf("expected BLOCKED message, got: %s", retryErr.Message)
	}
}

func TestMultiEdit_ProtectedTestFile(t *testing.T) {
	dir := setupTestDir(t)
	// MultiEdit should block if any edit targets /tests/.
	tool := MultiEdit(WithWorkDir(dir))
	err := callErr(t, tool, `{"edits":[{"path":"/tests/test.sh","old_string":"echo hello","new_string":"echo bye"}]}`)
	if err == nil {
		t.Fatal("expected error for protected test file")
	}
	var retryErr *core.ModelRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected ModelRetryError, got %T: %v", err, err)
	}
	if !strings.Contains(retryErr.Message, "BLOCKED") {
		t.Errorf("expected BLOCKED message, got: %s", retryErr.Message)
	}
}

func TestVerificationCheckpoint_IgnoresNonVerificationBash(t *testing.T) {
	mw, validator := VerificationCheckpoint()

	ctx := context.Background()

	// Bash calls that are NOT verification (e.g., ls, cat).
	messages := []core.ModelMessage{
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{
					ToolName:   "bash",
					ArgsJSON:   `{"command":"ls -la"}`,
					ToolCallID: "call1",
				},
				core.ToolCallPart{
					ToolName:   "bash",
					ArgsJSON:   `{"command":"cat README.md"}`,
					ToolCallID: "call2",
				},
			},
		},
	}

	next := func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		return &core.ModelResponse{}, nil
	}
	_, _ = mw(ctx, messages, nil, nil, next)

	// Should reject — bash was used but not for verification.
	rc := &core.RunContext{}
	_, err := validator(ctx, rc, "All done!")
	if err == nil {
		t.Fatal("expected error when bash was used but not for verification")
	}
}

func TestFileSnippetForEdit(t *testing.T) {
	content := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}

func helper() {
	fmt.Println("helper")
}
`
	// Search for something that partially matches.
	search := `func main() {
	fmt.Println("Goodbye, World!")
}`
	snippet := fileSnippetForEdit(content, search)
	if snippet == "" {
		t.Fatal("expected a non-empty snippet")
	}
	if !strings.Contains(snippet, "func main()") {
		t.Errorf("snippet should contain 'func main()', got: %s", snippet)
	}
	if !strings.Contains(snippet, "Hello, World!") {
		t.Errorf("snippet should contain the actual file content, got: %s", snippet)
	}

	// Empty search should return empty.
	if s := fileSnippetForEdit(content, ""); s != "" {
		t.Errorf("expected empty snippet for empty search, got: %s", s)
	}

	// Search for something with no match at all.
	if s := fileSnippetForEdit(content, "zzzzzzz_nonexistent"); s != "" {
		t.Errorf("expected empty snippet for non-matching search, got: %s", s)
	}
}

func TestPythonErrorHint(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		want     string
	}{
		{
			name: "syntax_error",
			output: `Traceback (most recent call last):
  File "solve.py", line 42, in <module>
    x = (1 +
SyntaxError: unexpected EOF while parsing`,
			exitCode: 1,
			want:     "solve.py:42",
		},
		{
			name: "indentation_error",
			output: `  File "main.py", line 10
    print("hello")
IndentationError: unexpected indent`,
			exitCode: 1,
			want:     "main.py:10",
		},
		{
			name: "name_error_suggestion",
			output: `Traceback (most recent call last):
  File "app.py", line 5, in <module>
NameError: name 'pritn' is not defined. Did you mean: 'print'?`,
			exitCode: 1,
			want:     "Did you mean",
		},
		{
			name:     "success_no_hint",
			output:   "All tests passed!",
			exitCode: 0,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pythonErrorHint(tt.output, tt.exitCode)
			if tt.want == "" {
				if got != "" {
					t.Errorf("expected empty hint, got: %s", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("expected hint to contain %q, got: %s", tt.want, got)
			}
		})
	}
}

func TestWritePreview(t *testing.T) {
	dir := t.TempDir()
	tool := Write(WithWorkDir(dir))

	// Small file should include preview.
	result := call(t, tool, `{"path":"small.py","content":"print('hello')\nprint('world')\n"}`)
	if !strings.Contains(result, "3 lines") {
		t.Errorf("expected line count in result, got: %s", result)
	}
	if !strings.Contains(result, "print('hello')") {
		t.Errorf("expected content preview, got: %s", result)
	}
}

func TestWriteOverwriteWarning(t *testing.T) {
	dir := t.TempDir()
	tool := Write(WithWorkDir(dir))

	// Create a large file first.
	bigContent := strings.Repeat("line of content\n", 100) // ~1600 bytes
	call(t, tool, fmt.Sprintf(`{"path":"big.txt","content":%q}`, bigContent))

	// Overwrite with much smaller content — should trigger warning.
	result := call(t, tool, `{"path":"big.txt","content":"tiny\n"}`)
	if !strings.Contains(result, "warning") {
		t.Errorf("expected overwrite warning, got: %s", result)
	}
	if !strings.Contains(result, "reduction") {
		t.Errorf("expected reduction percentage in warning, got: %s", result)
	}

	// Overwrite with similar-sized content — should NOT trigger warning.
	result2 := call(t, tool, fmt.Sprintf(`{"path":"big.txt","content":%q}`, bigContent[:len(bigContent)-10]))
	if strings.Contains(result2, "warning") {
		t.Errorf("expected no warning for similar-size overwrite, got: %s", result2)
	}
}

func TestMultiEdit_Atomic(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", "hello world\ngoodbye world\n")
	writeTestFile(t, dir, "b.txt", "foo bar\nbaz qux\n")

	tool := MultiEdit(WithWorkDir(dir))

	// First edit succeeds, second should fail — verify first file is NOT modified.
	err := callErr(t, tool, `{"edits":[
		{"path":"a.txt","old_string":"hello world","new_string":"hello earth"},
		{"path":"b.txt","old_string":"nonexistent string","new_string":"replacement"}
	]}`)
	if err == nil {
		t.Fatal("expected error for second edit not found")
	}

	// Verify a.txt was NOT modified (atomic — second edit failed, so no writes).
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("expected a.txt to remain unchanged (atomic multi_edit), got: %s", data)
	}

	// Test that sequential edits to the same file work within a batch.
	result := call(t, tool, `{"edits":[
		{"path":"a.txt","old_string":"hello world","new_string":"hello earth"},
		{"path":"a.txt","old_string":"goodbye world","new_string":"goodbye earth"}
	]}`)
	if !strings.Contains(result, "hello earth") {
		t.Errorf("expected first edit result, got: %s", result)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "a.txt"))
	content := string(data)
	if !strings.Contains(content, "hello earth") || !strings.Contains(content, "goodbye earth") {
		t.Errorf("expected both edits applied, got: %s", content)
	}
}

func TestGlobShowsSizes(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "big.txt", strings.Repeat("x", 2000))
	writeTestFile(t, dir, "small.txt", "hello")

	tool := Glob(WithWorkDir(dir))

	result := call(t, tool, `{"pattern":"*.txt"}`)
	// Should show file sizes.
	if !strings.Contains(result, "(") {
		t.Errorf("expected file sizes in glob output, got: %s", result)
	}
}

func TestFindOccurrenceLines(t *testing.T) {
	content := "aaa\nbbb\nccc\nbbb\nddd\nbbb\n"
	got := findOccurrenceLines(content, "bbb")
	// "bbb" appears at lines 2, 4, 6
	if !strings.Contains(got, "2") || !strings.Contains(got, "4") || !strings.Contains(got, "6") {
		t.Errorf("findOccurrenceLines() = %q, want lines 2, 4, 6", got)
	}
}

func TestEditMultipleOccurrencesShowsLines(t *testing.T) {
	dir := t.TempDir()
	// Create a file with duplicate lines
	content := "line1\ndup\nline3\ndup\nline5\n"
	writeTestFile(t, dir, "dup.txt", content)

	tool := Edit(WithWorkDir(dir))
	err := callErr(t, tool, `{"path":"dup.txt","old_string":"dup","new_string":"unique"}`)
	if err == nil {
		t.Fatal("expected error for multiple occurrences")
	}
	errMsg := err.Error()
	// Should mention line numbers
	if !strings.Contains(errMsg, "lines") {
		t.Errorf("expected line numbers in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "2") {
		t.Errorf("expected line 2 in error, got: %s", errMsg)
	}
}

func TestCompilationErrorHint(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		contains string
	}{
		{
			name:     "gcc_error",
			output:   "main.c:42:5: error: expected ';' after expression",
			exitCode: 1,
			contains: "main.c:42",
		},
		{
			name:     "go_error",
			output:   "./main.go:15:2: undefined: fmt.Printl",
			exitCode: 2,
			contains: "main.go:15",
		},
		{
			name:     "rust_error",
			output:   "error[E0308]: mismatched types\n --> src/main.rs:10:5\n  |\n10 |     foo()\n  |     ^^^^^ expected u32",
			exitCode: 101,
			contains: "src/main.rs:10",
		},
		{
			name:     "success_no_hint",
			output:   "Build succeeded",
			exitCode: 0,
			contains: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compilationErrorHint(tt.output, tt.exitCode)
			if tt.contains == "" && got != "" {
				t.Errorf("expected no hint, got: %s", got)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("expected hint containing %q, got: %s", tt.contains, got)
			}
		})
	}
}

func TestJsonErrorHint(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		contains string
	}{
		{
			name:     "python_json_decode",
			output:   `json.decoder.JSONDecodeError: Expecting ',' delimiter: line 5 column 3 (char 42)`,
			exitCode: 1,
			contains: "line 5 column 3",
		},
		{
			name:     "node_json_error",
			output:   `SyntaxError: Unexpected token } in JSON at position 42`,
			exitCode: 1,
			contains: "position 42",
		},
		{
			name:     "success_no_hint",
			output:   `{"key": "value"}`,
			exitCode: 0,
			contains: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonErrorHint(tt.output, tt.exitCode)
			if tt.contains == "" && got != "" {
				t.Errorf("expected no hint, got: %s", got)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("expected hint containing %q, got: %s", tt.contains, got)
			}
		})
	}
}

func TestPythonErrorHintExpanded(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		contains string
	}{
		{
			name: "type_error_with_traceback",
			output: `Traceback (most recent call last):
  File "main.py", line 42, in process
    result = x + "hello"
TypeError: unsupported operand type(s) for +: 'int' and 'str'`,
			exitCode: 1,
			contains: "main.py:42",
		},
		{
			name: "value_error_with_traceback",
			output: `Traceback (most recent call last):
  File "/app/solver.py", line 15, in parse
    val = int("abc")
ValueError: invalid literal for int() with base 10: 'abc'`,
			exitCode: 1,
			contains: "solver.py:15",
		},
		{
			name: "key_error_with_traceback",
			output: `Traceback (most recent call last):
  File "data.py", line 8, in load
    x = d["missing"]
KeyError: 'missing'`,
			exitCode: 1,
			contains: "data.py:8",
		},
		{
			name: "file_not_found_no_traceback",
			output: `FileNotFoundError: [Errno 2] No such file or directory: 'output.csv'`,
			exitCode: 1,
			contains: "output.csv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pythonErrorHint(tt.output, tt.exitCode)
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("expected hint containing %q, got: %s", tt.contains, got)
			}
		})
	}
}

func TestEncodingErrorHint(t *testing.T) {
	got := encodingErrorHint("UnicodeDecodeError: 'utf-8' codec can't decode byte 0xff", 1)
	if !strings.Contains(got, "encoding='utf-8'") {
		t.Errorf("expected encoding hint, got: %s", got)
	}
	got = encodingErrorHint("UnicodeEncodeError: 'ascii' codec can't encode character", 1)
	if !strings.Contains(got, "encoding='utf-8'") {
		t.Errorf("expected encoding hint, got: %s", got)
	}
	got = encodingErrorHint("success output", 0)
	if got != "" {
		t.Errorf("expected no hint for success, got: %s", got)
	}
}

func TestPermissionHint(t *testing.T) {
	got := permissionHint("bash: ./run.sh: Permission denied", 126)
	if !strings.Contains(got, "chmod") {
		t.Errorf("expected chmod hint for exit 126, got: %s", got)
	}
	got = permissionHint("Permission denied: ./test.py", 1)
	if !strings.Contains(got, "chmod") {
		t.Errorf("expected chmod hint for script, got: %s", got)
	}
	got = permissionHint("all good", 0)
	if got != "" {
		t.Errorf("expected no hint for success, got: %s", got)
	}
}

func TestAddressInUseHint(t *testing.T) {
	got := addressInUseHint("OSError: [Errno 98] Address already in use", 1)
	if !strings.Contains(got, "port") {
		t.Errorf("expected port hint, got: %s", got)
	}
	got = addressInUseHint("Error: listen EADDRINUSE: address already in use :::3000", 1)
	if !strings.Contains(got, "port") {
		t.Errorf("expected port hint for EADDRINUSE, got: %s", got)
	}
	got = addressInUseHint("server started", 0)
	if got != "" {
		t.Errorf("expected no hint for success, got: %s", got)
	}
}

func TestFirstFailureDetail(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		contains string
	}{
		{
			name: "pytest_assertion",
			output: `test_foo.py::test_add FAILED
E       assert 5 == 4
E        +  where 5 = add(2, 3)
================ 1 failed, 2 passed ================`,
			contains: "assert 5 == 4",
		},
		{
			name: "go_test_expected_got",
			output: `--- FAIL: TestAdd (0.00s)
    main_test.go:15: expected 42, got 43
FAIL
FAIL	example.com/pkg	0.001s`,
			contains: "expected 42, got 43",
		},
		{
			name: "python_unittest_assertion",
			output: `FAIL: test_add (test_math.TestMath)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "test_math.py", line 10, in test_add
    self.assertEqual(result, 5)
AssertionError: 4 != 5
----------------------------------------------------------------------
Ran 3 tests in 0.001s

FAILED (failures=1)`,
			contains: "AssertionError: 4 != 5",
		},
		{
			name: "jest_expected_received",
			output: `FAIL src/math.test.js
  ● add › should return 5

    Expected: 5
    Received: 4

Tests: 1 failed, 2 passed, 3 total`,
			contains: "Expected: 5",
		},
		{
			name: "generic_expected_actual",
			output: `Running tests...
Test 1: PASS
Test 2: FAIL - Expected "hello" but got "world"
Test 3: PASS`,
			contains: `Expected "hello" but got "world"`,
		},
		{
			name: "classic_diff",
			output: `Comparing outputs:
1c1
< hello world
---
> hello wrold
2c2
< line two`,
			contains: `diff: expected "hello world", got "hello wrold"`,
		},
		{
			name: "unified_diff",
			output: `--- expected.txt
+++ actual.txt
@@ -1,3 +1,3 @@
 line1
-expected_line2
+actual_line2
 line3`,
			contains: `diff: expected "expected_line2", got "actual_line2"`,
		},
		{
			name: "rust_panic",
			output: `running 3 tests
test test_add ... ok
test test_multiply ... FAILED
thread 'test_multiply' panicked at 'assertion ` + "`" + `left == right` + "`" + ` failed
  left: 42
  right: 43'`,
			contains: "panicked at",
		},
		{
			name: "no_failure",
			output: `All tests passed!
3 passed, 0 failed`,
			contains: "", // empty = no match expected
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstFailureDetail(tc.output)
			if tc.contains == "" {
				if got != "" {
					t.Errorf("expected no failure detail, got: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.contains) {
				t.Errorf("expected failure detail containing %q, got: %s", tc.contains, got)
			}
		})
	}
}

func TestTestResultSummaryWithFailureDetail(t *testing.T) {
	// Verify that testResultSummary now includes first failure details.
	output := `test_calc.py::test_multiply PASSED
test_calc.py::test_add FAILED
E       assert 7 == 5
test_calc.py::test_subtract PASSED
========================= 1 failed, 2 passed =========================`

	got := testResultSummary(output)
	if !strings.Contains(got, "1 failed") {
		t.Errorf("expected summary line with '1 failed', got: %s", got)
	}
	if !strings.Contains(got, "assert 7 == 5") {
		t.Errorf("expected first failure detail 'assert 7 == 5', got: %s", got)
	}
}

func TestParseMakefileTargets(t *testing.T) {
	makefile := `# My project Makefile

CC = gcc
CFLAGS = -Wall

.PHONY: all test clean

all: build

build:
	$(CC) $(CFLAGS) -o myapp main.c

test: build
	./run_tests.sh

clean:
	rm -f myapp *.o

install: build
	cp myapp /usr/local/bin/

run: build
	./myapp

lint:
	pylint src/

# Don't include these:
%.o: %.c
	$(CC) -c $<
`
	targets := parseMakefileTargets(makefile)

	// Should include useful targets
	targetSet := make(map[string]bool)
	for _, t := range targets {
		targetSet[t] = true
	}

	for _, expected := range []string{"build", "test", "clean", "install", "run", "lint"} {
		if !targetSet[expected] {
			t.Errorf("expected target %q in parsed targets: %v", expected, targets)
		}
	}
}

func TestTestResultSummaryXOfYPattern(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "X/Y passed",
			output: `Running test suite...
Test 1: PASS
Test 2: FAIL
Test 3: PASS
3/5 tests passed`,
			want: "3/5 tests passed",
		},
		{
			name: "X out of Y",
			output: `Checking outputs...
Passed 7 out of 10 tests`,
			want: "7 out of 10",
		},
		{
			name: "Passed X of Y failed Z",
			output: `Test results: passed 3 of 5, failed 2`,
			want: "passed 3 of 5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := testResultSummary(tt.output)
			if !strings.Contains(got, tt.want) {
				t.Errorf("expected summary containing %q, got: %s", tt.want, got)
			}
		})
	}
}

func TestTestFailureFingerprint(t *testing.T) {
	// Same output should produce same fingerprint.
	output1 := `test_calc.py::test_add FAILED
E       assert 7 == 5
========================= 1 failed =========================`

	fp1 := testFailureFingerprint(output1)
	fp2 := testFailureFingerprint(output1)
	if fp1 != fp2 {
		t.Errorf("same output should produce same fingerprint: %q vs %q", fp1, fp2)
	}
	if fp1 == "" {
		t.Error("expected non-empty fingerprint for failing test")
	}

	// Different failure should produce different fingerprint.
	output2 := `test_calc.py::test_add FAILED
E       assert 7 == 6
========================= 1 failed =========================`

	fp3 := testFailureFingerprint(output2)
	if fp3 == fp1 {
		t.Errorf("different failures should produce different fingerprints: both %q", fp1)
	}

	// Passing test should produce empty fingerprint.
	output3 := `test_calc.py::test_add PASSED
========================= 1 passed =========================`
	fp4 := testFailureFingerprint(output3)
	if fp4 != "" {
		t.Errorf("passing test should have empty fingerprint, got: %q", fp4)
	}
}

func TestDetectEnvFiles(t *testing.T) {
	dir := t.TempDir()

	// No .env files — should return empty.
	got := detectEnvFiles(dir)
	if got != "" {
		t.Errorf("expected empty for no env files, got: %s", got)
	}

	// Create .env.example with some vars.
	writeTestFile(t, dir, ".env.example", `# Database config
DATABASE_URL=postgresql://localhost:5432/mydb
SECRET_KEY=changeme
PORT=8080
`)
	got = detectEnvFiles(dir)
	if !strings.Contains(got, "DATABASE_URL") {
		t.Errorf("expected DATABASE_URL in env hint, got: %s", got)
	}
	if !strings.Contains(got, ".env.example") {
		t.Errorf("expected .env.example path in hint, got: %s", got)
	}
	if !strings.Contains(got, "cp") {
		t.Errorf("expected cp hint for .env.example, got: %s", got)
	}
}

func TestSignalHintTimeout(t *testing.T) {
	hint := signalHint(124)
	if !strings.Contains(hint, "timeout") {
		t.Errorf("expected timeout hint for exit 124, got: %s", hint)
	}
	if !strings.Contains(hint, "too slow") {
		t.Errorf("expected 'too slow' in timeout hint, got: %s", hint)
	}
}

func TestNodeErrorHint(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		contains string
	}{
		{
			name:     "module_not_found",
			output:   `Error: Cannot find module 'express'\nRequire stack:\n- /app/server.js`,
			exitCode: 1,
			contains: "npm install express",
		},
		{
			name:     "reference_error_with_stack",
			output:   "ReferenceError: foo is not defined\n    at Object.<anonymous> (/app/main.js:15:3)\n    at Module._compile (node:internal/modules/cjs/loader:1254:14)",
			exitCode: 1,
			contains: "/app/main.js:15:3",
		},
		{
			name:     "success_no_hint",
			output:   "Server listening on port 3000",
			exitCode: 0,
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodeErrorHint(tt.output, tt.exitCode)
			if tt.contains == "" {
				if got != "" {
					t.Errorf("expected no hint, got: %s", got)
				}
				return
			}
			if !strings.Contains(got, tt.contains) {
				t.Errorf("expected hint containing %q, got: %s", tt.contains, got)
			}
		})
	}
}

func TestFirstFailureDetailColon(t *testing.T) {
	// Lines ending in ":" should be treated as headers, not assertions.
	output := "Comparing expected vs actual output:\n< hello\n---\n> world"
	got := firstFailureDetail(output)
	// Should match the diff pattern, not the "expected vs actual" header.
	if strings.Contains(got, "Comparing") {
		t.Errorf("expected diff pattern match, not header match, got: %s", got)
	}
	if !strings.Contains(got, "diff:") {
		t.Errorf("expected diff: pattern, got: %s", got)
	}
}

func TestDetectCoqTask(t *testing.T) {
	// Directory with _CoqProject file.
	dir := t.TempDir()
	writeTestFile(t, dir, "_CoqProject", "-R . MyProject\n")
	if !detectCoqTask(dir) {
		t.Error("expected Coq task detection with _CoqProject")
	}

	// Directory with .v files.
	dir2 := t.TempDir()
	writeTestFile(t, dir2, "Proof.v", "Theorem plus_comm : forall n m, n + m = m + n.\n")
	if !detectCoqTask(dir2) {
		t.Error("expected Coq task detection with .v files")
	}

	// Empty directory — should not detect.
	dir3 := t.TempDir()
	if detectCoqTask(dir3) {
		t.Error("expected no Coq task in empty directory")
	}
}

func TestDetectOCamlTask(t *testing.T) {
	// Directory with dune-project.
	dir := t.TempDir()
	writeTestFile(t, dir, "dune-project", "(lang dune 3.0)\n")
	if !detectOCamlTask(dir) {
		t.Error("expected OCaml task detection with dune-project")
	}

	// Directory with .ml files.
	dir2 := t.TempDir()
	writeTestFile(t, dir2, "main.ml", "let () = print_endline \"hello\"\n")
	if !detectOCamlTask(dir2) {
		t.Error("expected OCaml task detection with .ml files")
	}

	// Empty directory — should not detect.
	dir3 := t.TempDir()
	if detectOCamlTask(dir3) {
		t.Error("expected no OCaml task in empty directory")
	}
}

func TestDetectBuildFromSourceTask(t *testing.T) {
	// Directory name with "build-" prefix.
	dir := filepath.Join(t.TempDir(), "build-povray")
	os.MkdirAll(dir, 0o755)
	if !detectBuildFromSourceTask(dir) {
		t.Error("expected build task detection from directory name 'build-povray'")
	}

	// Directory with configure script.
	dir2 := t.TempDir()
	writeTestFile(t, dir2, "configure", "#!/bin/sh\n")
	if !detectBuildFromSourceTask(dir2) {
		t.Error("expected build task detection with configure script")
	}

	// Empty directory — should not detect.
	dir3 := t.TempDir()
	if detectBuildFromSourceTask(dir3) {
		t.Error("expected no build task in empty directory")
	}
}

func TestDetectImageFiles(t *testing.T) {
	// Directory with image files.
	dir := t.TempDir()
	writeTestFile(t, dir, "diagram.png", "PNG")
	writeTestFile(t, dir, "photo.jpg", "JPEG")
	writeTestFile(t, dir, "code.py", "print('hi')")

	images := detectImageFiles(dir)
	if len(images) != 2 {
		t.Errorf("expected 2 image files, got %d: %v", len(images), images)
	}

	// Empty directory — no images.
	dir2 := t.TempDir()
	images2 := detectImageFiles(dir2)
	if len(images2) != 0 {
		t.Errorf("expected 0 image files in empty dir, got %d", len(images2))
	}
}

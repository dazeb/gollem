package codetool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/fugue-labs/gollem/core"
)

// WriteParams are the parameters for the write tool.
type WriteParams struct {
	// Path is the file path to write to.
	Path string `json:"path" jsonschema:"description=File path to create or overwrite"`

	// Content is the full file content to write.
	Content string `json:"content" jsonschema:"description=The complete file content to write"`
}

// Write creates a tool that writes content to a file, creating it if needed.
func Write(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[WriteParams](
		"write",
		"Create a new file or overwrite an existing file with the provided content. "+
			"Creates parent directories if they don't exist. "+
			"Use this for creating new files. For modifying existing files, prefer the edit tool.",
		func(ctx context.Context, params WriteParams) (string, error) {
			if params.Path == "" {
				return "", &core.ModelRetryError{Message: "path must not be empty"}
			}

			path := params.Path
			if !filepath.IsAbs(path) && cfg.WorkDir != "" {
				path = filepath.Join(cfg.WorkDir, path)
			}

			if isProtectedTestFile(path) {
				return "", protectedFileError(params.Path)
			}

			// Check if overwriting an existing file — track previous size
			// for the overwrite warning that catches accidental truncation.
			// Also detect CRLF line endings so we can preserve them.
			var prevSize int64 = -1
			var prevCRLF bool
			if info, err := os.Stat(path); err == nil {
				prevSize = info.Size()
				// Sample the first 4K to detect CRLF. This is consistent
				// with the edit tool's CRLF preservation behavior — without
				// it, using write for a full-file rewrite would silently
				// convert Windows line endings to Unix.
				if data, err := os.ReadFile(path); err == nil {
					sample := string(data)
					if len(sample) > 4096 {
						sample = sample[:4096]
					}
					prevCRLF = strings.Contains(sample, "\r\n")
				}
			}

			// Create parent directories.
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", fmt.Errorf("create directories: %w", err)
			}

			// Determine file permissions. When overwriting an existing file,
			// preserve its permissions (don't strip executable bits). For new
			// files, auto-chmod scripts to executable to prevent "Permission
			// denied" errors that waste 1-2 agent turns.
			perm := os.FileMode(0o644)
			if prevSize >= 0 {
				// File exists — preserve its permissions.
				if fi, err := os.Stat(path); err == nil {
					perm = fi.Mode().Perm()
				}
			}
			// Auto-upgrade to executable for scripts (both new and existing).
			lower := strings.ToLower(filepath.Base(path))
			isScript := strings.HasPrefix(params.Content, "#!") ||
				strings.HasSuffix(lower, ".sh") ||
				strings.HasSuffix(lower, ".bash") ||
				strings.HasSuffix(lower, ".zsh") ||
				strings.HasSuffix(lower, ".fish") ||
				strings.HasSuffix(lower, ".ksh") ||
				strings.HasSuffix(lower, ".py") ||
				strings.HasSuffix(lower, ".rb") ||
				strings.HasSuffix(lower, ".pl") ||
				strings.HasSuffix(lower, ".lua") ||
				strings.HasSuffix(lower, ".tcl") ||
				strings.HasSuffix(lower, ".expect")
			if isScript && perm&0o111 == 0 {
				perm = 0o755
			}

			// Preserve CRLF line endings when overwriting an existing
			// CRLF file (consistent with the edit tool). Models generate
			// LF-only content, so convert LF→CRLF before writing.
			writeContent := params.Content
			if prevCRLF && !strings.Contains(writeContent, "\r\n") {
				writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
			}

			if err := os.WriteFile(path, []byte(writeContent), perm); err != nil {
				return "", fmt.Errorf("write file: %w", err)
			}

			// Return a summary with line count. For small files, include a
			// content preview so the agent can verify without a separate view.
			lineCount := strings.Count(params.Content, "\n") + 1
			if params.Content == "" {
				lineCount = 0
			}
			result := fmt.Sprintf("Wrote %d bytes (%d lines) to %s", len(params.Content), lineCount, params.Path)

			// Warn about missing trailing newline for source code files.
			// POSIX requires text files to end with a newline, and many
			// tools (git, linters, diff) flag or mishandle files without one.
			if len(params.Content) > 0 && !strings.HasSuffix(params.Content, "\n") && isSourceFile(filepath.Base(params.Path)) {
				result += "\n[hint: file does not end with a newline — most tools expect a trailing newline in source files]"
			}

			// Detect embedded line numbers from view tool output.
			// Agents sometimes copy-paste view output (with "     1\t" prefixes)
			// into the write content. This corrupts the file silently.
			if lineCount >= 3 && looksLikeViewOutput(params.Content) {
				result += "\n[warning: content appears to contain line number prefixes from the view tool " +
					"(e.g., '     1\\t...'). This will corrupt the file. Remove the line numbers " +
					"and write only the actual file content.]"
			}

			// Warn when overwriting reduced file size by more than 50%.
			// This catches accidental truncation — the #1 write-related bug
			// where the agent rewrites a file but forgets to include all content.
			if prevSize > 100 && int64(len(params.Content)) < prevSize/2 {
				result += fmt.Sprintf("\n[warning: file shrank from %d to %d bytes (%.0f%% reduction) — verify you included all content]",
					prevSize, len(params.Content), float64(prevSize-int64(len(params.Content)))/float64(prevSize)*100)
			}

			// Include a preview for small files (< 30 lines) to save a view call.
			if lineCount > 0 && lineCount <= 30 {
				lines := strings.Split(params.Content, "\n")
				var preview strings.Builder
				preview.WriteString("\n\nContent:\n")
				for i, line := range lines {
					if len(line) > 200 {
						n := 200
					for n > 0 && !utf8.RuneStart(line[n]) {
						n--
					}
					line = line[:n] + "..."
					}
					fmt.Fprintf(&preview, "%6d\t%s\n", i+1, line)
				}
				result += preview.String()
			}

			return result, nil
		},
	)
}

// looksLikeViewOutput checks if content looks like it was copy-pasted from
// the view tool (line numbers followed by a tab at the start of lines).
// The view tool formats lines as "     1\tcode..." — if 3+ of the first 5
// lines match this pattern, it's likely pasted view output.
func looksLikeViewOutput(content string) bool {
	lines := strings.SplitN(content, "\n", 6)
	matches := 0
	checked := 0
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		checked++
		if checked > 5 {
			break
		}
		// View output format: spaces + digits + tab
		trimmed := strings.TrimLeft(line, " ")
		if len(trimmed) > 0 && trimmed[0] >= '0' && trimmed[0] <= '9' {
			tabIdx := strings.IndexByte(trimmed, '\t')
			if tabIdx > 0 && tabIdx <= 6 {
				// Verify the part before tab is all digits.
				allDigits := true
				for _, c := range trimmed[:tabIdx] {
					if c < '0' || c > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					matches++
				}
			}
		}
	}
	return checked >= 3 && matches >= 3
}

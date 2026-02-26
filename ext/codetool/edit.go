package codetool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// isProtectedTestFile returns true if the path is inside a verifier test
// directory (e.g. /tests/). On Harbor, the verifier runs the ORIGINAL test
// files — edits are silently ignored, so allowing the agent to modify them
// wastes turns and produces false confidence that tests pass.
func isProtectedTestFile(path string) bool {
	normalized := filepath.Clean(path)
	// /tests/ is the standard Harbor verifier directory.
	if strings.HasPrefix(normalized, "/tests/") || normalized == "/tests" {
		return true
	}
	return false
}

// protectedFileError returns a ModelRetryError for protected test files.
func protectedFileError(path string) error {
	return &core.ModelRetryError{
		Message: "BLOCKED: " + path + " is a verifier test file and must NOT be modified. " +
			"The verifier runs the ORIGINAL tests — your changes will be ignored during evaluation. " +
			"Fix YOUR code to pass the tests instead.",
	}
}

// EditParams are the parameters for the edit tool.
type EditParams struct {
	// Path is the file path to edit.
	Path string `json:"path" jsonschema:"description=File path to edit"`

	// OldString is the exact string to find and replace.
	OldString string `json:"old_string" jsonschema:"description=The exact string to find in the file. Must match exactly including whitespace and indentation."`

	// NewString is the replacement string.
	NewString string `json:"new_string" jsonschema:"description=The string to replace old_string with. Use empty string to delete."`

	// ReplaceAll replaces all occurrences instead of just the first.
	ReplaceAll bool `json:"replace_all,omitempty" jsonschema:"description=If true, replace ALL occurrences. Default: false (replace first only)"`
}

// Edit creates a tool that performs exact string replacements in files.
func Edit(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[EditParams](
		"edit",
		"Make exact string replacements in a file. You must provide the exact text to find (old_string) "+
			"and the replacement (new_string). The old_string must match exactly, including whitespace and "+
			"indentation. Always read a file with the view tool before editing to ensure exact matches. "+
			"The edit will fail if old_string is not found or matches multiple locations (unless replace_all is true).",
		func(ctx context.Context, params EditParams) (string, error) {
			if params.Path == "" {
				return "", &core.ModelRetryError{Message: "path must not be empty"}
			}
			if params.OldString == "" {
				return "", &core.ModelRetryError{Message: "old_string must not be empty. To create a new file, use the write tool."}
			}
			if params.OldString == params.NewString {
				return "", &core.ModelRetryError{Message: "old_string and new_string are identical — no change needed"}
			}

			path := params.Path
			if !filepath.IsAbs(path) && cfg.WorkDir != "" {
				path = filepath.Join(cfg.WorkDir, path)
			}

			if isProtectedTestFile(path) {
				return "", protectedFileError(params.Path)
			}

			// Preserve existing file permissions so we don't strip
			// executable bits when editing scripts. The write tool
			// auto-sets 0o755 for scripts, but edit should preserve
			// whatever permissions the file already has.
			fi, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("file not found: %s. Use the write tool to create new files.", params.Path)}
				}
				return "", fmt.Errorf("stat file: %w", err)
			}
			filePerm := fi.Mode().Perm()

			data, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("read file: %w", err)
			}

			content := string(data)

			// Normalize CRLF → LF for matching. Models generate LF-only
			// strings; Windows-style files with \r\n cause silent match
			// failures without this. The original line endings are restored
			// when writing back.
			hasCRLF := strings.Contains(content, "\r\n")
			if hasCRLF {
				content = strings.ReplaceAll(content, "\r\n", "\n")
			}
			// Also normalize old_string and new_string in case the model
			// copied content from a CRLF source (e.g., view output of a
			// CRLF file before normalization was added). Without this, the
			// old_string wouldn't match the LF-normalized file content.
			oldStr := strings.ReplaceAll(params.OldString, "\r\n", "\n")
			newStr := strings.ReplaceAll(params.NewString, "\r\n", "\n")

			count := strings.Count(content, oldStr)

			if count == 0 {
				// Auto-correct whitespace mismatches: if old_string matches
				// exactly once when whitespace is normalized, automatically
				// adjust indentation and apply the edit. This saves a full
				// round-trip — whitespace mismatches are the #1 edit failure.
				if actualOld, adjustedNew, ok := autoCorrectWhitespace(content, oldStr, newStr); ok {
					newContent := strings.Replace(content, actualOld, adjustedNew, 1)
					writeContent := newContent
					if hasCRLF {
						writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
					}
					if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
						return "", fmt.Errorf("write file: %w", err)
					}
					result := editResultWithContext(newContent, adjustedNew, 1, params.Path)
					result += "\n[auto-corrected whitespace mismatch]"
					return result, nil
				}

				// Auto-correct leading/trailing blank lines: models often include
				// extra blank lines at the start or end of old_string that don't
				// exist in the file. Strip them and retry — this is the #2 edit
				// failure after whitespace mismatches.
				if trimmedOld, trimmedNew, ok := autoCorrectBlankLines(content, oldStr, newStr); ok {
					// Try exact match with stripped version first.
					if strings.Count(content, trimmedOld) == 1 {
						newContent := strings.Replace(content, trimmedOld, trimmedNew, 1)
						writeContent := newContent
						if hasCRLF {
							writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
						}
						if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
							return "", fmt.Errorf("write file: %w", err)
						}
						result := editResultWithContext(newContent, trimmedNew, 1, params.Path)
						result += "\n[auto-corrected extra blank lines in old_string]"
						return result, nil
					}
					// Also try whitespace auto-correct on the stripped version.
					if actualOld, adjustedNew, ok := autoCorrectWhitespace(content, trimmedOld, trimmedNew); ok {
						newContent := strings.Replace(content, actualOld, adjustedNew, 1)
						writeContent := newContent
						if hasCRLF {
							writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
						}
						if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
							return "", fmt.Errorf("write file: %w", err)
						}
						result := editResultWithContext(newContent, adjustedNew, 1, params.Path)
						result += "\n[auto-corrected blank lines and whitespace]"
						return result, nil
					}
				}

				// Auto-correct internal blank line count differences: models
				// commonly generate 2 blank lines between functions when the file
				// has 1 (or vice versa). Normalize runs of 2+ blank lines to 1
				// and retry — this is the #3 edit failure.
				if normalizedOld, normalizedNew, ok := autoCorrectInternalBlankLines(content, oldStr, newStr); ok {
					if strings.Count(content, normalizedOld) == 1 {
						newContent := strings.Replace(content, normalizedOld, normalizedNew, 1)
						writeContent := newContent
						if hasCRLF {
							writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
						}
						if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
							return "", fmt.Errorf("write file: %w", err)
						}
						result := editResultWithContext(newContent, normalizedNew, 1, params.Path)
						result += "\n[auto-corrected internal blank line count]"
						return result, nil
					}
				}

				// Combined internal blank line + whitespace correction: the model
				// gets both the blank line count AND indentation wrong. Neither
				// autoCorrectInternalBlankLines nor autoCorrectWhitespace alone
				// can match, but normalizing blank runs first then passing through
				// whitespace correction handles both simultaneously.
				{
					blankNormOld := normalizeBlankRuns(oldStr)
					blankNormNew := normalizeBlankRuns(newStr)
					if blankNormOld != oldStr { // blank line normalization changed something
						if actualOld, adjustedNew, ok := autoCorrectWhitespace(content, blankNormOld, blankNormNew); ok {
							newContent := strings.Replace(content, actualOld, adjustedNew, 1)
							writeContent := newContent
							if hasCRLF {
								writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
							}
							if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
								return "", fmt.Errorf("write file: %w", err)
							}
							result := editResultWithContext(newContent, adjustedNew, 1, params.Path)
							result += "\n[auto-corrected internal blank lines and whitespace]"
							return result, nil
						}
					}
				}

				// Auto-correct extra context lines: models often include an extra
				// line of context at the beginning or end of old_string that either
				// doesn't exist in the file or has slightly different content.
				// Only trims lines that are identical in old and new (pure context).
				if actualOld, adjustedNew, ok := autoCorrectLineTrim(content, oldStr, newStr); ok {
					newContent := strings.Replace(content, actualOld, adjustedNew, 1)
					writeContent := newContent
					if hasCRLF {
						writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
					}
					if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
						return "", fmt.Errorf("write file: %w", err)
					}
					result := editResultWithContext(newContent, adjustedNew, 1, params.Path)
					result += "\n[auto-corrected by trimming extra context line]"
					return result, nil
				}

				msg := fmt.Sprintf("old_string not found in %s.", params.Path)

				// Check for whitespace-only mismatch that couldn't be auto-corrected
				// (e.g., multiple normalized matches, line count mismatch).
				if wsHint := detectWhitespaceMismatch(content, oldStr); wsHint != "" {
					msg += " " + wsHint
				} else {
					msg += " Ensure exact match including whitespace and indentation."
					// Show nearby lines to help the model fix the edit without re-reading.
					if hint := findNearestLines(content, oldStr, 3); hint != "" {
						msg += "\n\nMost similar lines in the file:\n" + hint
					}
				}

				// Include a file snippet around the nearest match so the agent
				// doesn't need a separate view call before retrying the edit.
				// This saves a full turn on every edit-not-found failure.
				if snippet := fileSnippetForEdit(content, oldStr); snippet != "" {
					msg += "\n\nFile content around best match:\n" + snippet
				}

				return "", &core.ModelRetryError{Message: msg}
			}

			if count > 1 && !params.ReplaceAll {
				// Show the line numbers of each occurrence so the agent can
				// include surrounding context to disambiguate. Without this,
				// the agent wastes a turn re-reading the file.
				locations := findOccurrenceLines(content, oldStr)
				msg := fmt.Sprintf("old_string found %d times in %s (at lines %s). Provide more surrounding context to make it unique, or set replace_all=true.",
					count, params.Path, locations)
				return "", &core.ModelRetryError{Message: msg}
			}

			var newContent string
			if params.ReplaceAll {
				newContent = strings.ReplaceAll(content, oldStr, newStr)
			} else {
				newContent = strings.Replace(content, oldStr, newStr, 1)
			}

			writeContent := newContent
			if hasCRLF {
				writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
			}
			if err := os.WriteFile(path, []byte(writeContent), filePerm); err != nil {
				return "", fmt.Errorf("write file: %w", err)
			}

			replacements := 1
			if params.ReplaceAll {
				replacements = count
			}
			return editResultWithContext(newContent, newStr, replacements, params.Path), nil
		},
	)
}

// editResultWithContext returns a success message with surrounding file context
// so the model can verify the edit without a separate view call. This saves
// one turn per edit — a significant efficiency gain.
func editResultWithContext(content, newString string, replacements int, path string) string {
	header := fmt.Sprintf("Replaced %d occurrence(s) in %s", replacements, path)

	// For replace_all with many replacements, skip context — too many locations.
	if replacements > 2 {
		return header
	}

	// Find the location of the new string in the content.
	idx := strings.Index(content, newString)
	if idx < 0 || newString == "" {
		return header
	}

	// Determine the line range to show (3 before, edited lines, 3 after).
	editStartLine := strings.Count(content[:idx], "\n")
	editLines := strings.Count(newString, "\n") + 1
	allLines := strings.Split(content, "\n")

	showStart := max(0, editStartLine-3)
	showEnd := min(len(allLines), editStartLine+editLines+3)

	// Cap at 25 lines to prevent bloating the response.
	if showEnd-showStart > 25 {
		showEnd = showStart + 25
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\nContext:\n")
	for i := showStart; i < showEnd; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, allLines[i])
	}
	if showEnd < len(allLines) {
		b.WriteString("       ...\n")
	}
	return b.String()
}

// findOccurrenceLines returns a comma-separated list of line numbers where
// old_string starts in the file content. Capped at 6 locations.
func findOccurrenceLines(content, old string) string {
	var lineNums []string
	offset := 0
	for {
		idx := strings.Index(content[offset:], old)
		if idx < 0 {
			break
		}
		lineNum := strings.Count(content[:offset+idx], "\n") + 1
		lineNums = append(lineNums, strconv.Itoa(lineNum))
		offset += idx + len(old)
		if len(lineNums) >= 6 {
			lineNums = append(lineNums, "...")
			break
		}
	}
	if len(lineNums) == 0 {
		return "unknown"
	}
	return strings.Join(lineNums, ", ")
}

// findNearestLines finds lines in the file content that are most similar to
// the first line of the search string. This helps the model fix failed edits
// without needing to re-read the entire file.
func findNearestLines(content, search string, maxResults int) string {
	searchLines := strings.Split(search, "\n")
	if len(searchLines) == 0 {
		return ""
	}
	// Use the first non-trivial line as the search anchor.
	// When the first line is too short (e.g., "{", "}"), fall back
	// to subsequent lines to find a meaningful match.
	anchorLine := ""
	for _, sl := range searchLines {
		trimmed := strings.TrimSpace(sl)
		if len(trimmed) >= 3 {
			anchorLine = trimmed
			break
		}
	}
	if anchorLine == "" {
		return ""
	}

	contentLines := strings.Split(content, "\n")
	type scored struct {
		lineNum int
		line    string
		score   int
	}
	var candidates []scored

	// Score each line by counting shared words with the search line.
	searchWords := strings.Fields(strings.ToLower(anchorLine))
	for i, line := range contentLines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 3 {
			continue
		}
		lineWords := strings.Fields(strings.ToLower(trimmed))
		score := 0
		for _, sw := range searchWords {
			for _, lw := range lineWords {
				if sw == lw {
					score++
					break
				}
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{lineNum: i + 1, line: line, score: score})
		}
	}

	// Sort by score descending (simple selection for small N).
	for i := 0; i < len(candidates) && i < maxResults; i++ {
		best := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[best].score {
				best = j
			}
		}
		candidates[i], candidates[best] = candidates[best], candidates[i]
	}

	if len(candidates) > maxResults {
		candidates = candidates[:maxResults]
	}

	var result strings.Builder
	for _, c := range candidates {
		fmt.Fprintf(&result, "  L%d: %s\n", c.lineNum, c.line)
	}
	return result.String()
}

// detectWhitespaceMismatch checks if the search string matches the content
// when whitespace is normalized. If so, returns a hint with the actual content
// that the model should use. This catches the most common edit failure:
// wrong indentation (tabs vs spaces, wrong indent level).
func detectWhitespaceMismatch(content, search string) string {
	// Normalize both content and search by collapsing all whitespace runs
	// to single spaces and trimming each line. This catches:
	// - tabs vs spaces
	// - wrong indent depth
	// - trailing whitespace
	normalizeLines := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = strings.Join(strings.Fields(line), " ")
		}
		return strings.Join(lines, "\n")
	}

	normalizedSearch := normalizeLines(search)
	normalizedContent := normalizeLines(content)

	idx := strings.Index(normalizedContent, normalizedSearch)
	if idx < 0 {
		return ""
	}

	// Found a whitespace-normalized match. Extract the actual content
	// lines that the model should use.
	// Map normalized index back to the original content by counting
	// the same number of newlines.
	searchLineCount := strings.Count(search, "\n") + 1

	// Find the line in the original content that corresponds to the match.
	normalizedBefore := normalizedContent[:idx]
	matchStartLine := strings.Count(normalizedBefore, "\n")

	contentLines := strings.Split(content, "\n")
	if matchStartLine >= len(contentLines) {
		return ""
	}

	endLine := matchStartLine + searchLineCount
	if endLine > len(contentLines) {
		endLine = len(contentLines)
	}

	actualLines := contentLines[matchStartLine:endLine]
	actual := strings.Join(actualLines, "\n")

	// Only report if the actual differs from the search (confirming it's
	// a whitespace issue, not an exact match we somehow missed).
	if actual == search {
		return ""
	}

	// Show a compact hint with the actual content the model should use.
	hint := fmt.Sprintf("Whitespace mismatch — the content exists but with different indentation (line %d). Use this exact text:\n%s",
		matchStartLine+1, actual)

	// Truncate very long hints.
	if len(hint) > 1000 {
		hint = hint[:1000] + "\n..."
	}
	return hint
}

// MultiEditEntry is a single edit operation within a multi-edit batch.
type MultiEditEntry struct {
	// Path is the file to edit.
	Path string `json:"path" jsonschema:"description=File path to edit"`

	// OldString is the exact string to replace.
	OldString string `json:"old_string" jsonschema:"description=The exact string to find"`

	// NewString is the replacement.
	NewString string `json:"new_string" jsonschema:"description=The replacement string"`
}

// MultiEditParams are the parameters for the multi-edit tool.
type MultiEditParams struct {
	// Edits is the list of edit operations to perform atomically.
	Edits []MultiEditEntry `json:"edits" jsonschema:"description=List of edit operations to apply"`
}

// MultiEdit creates a tool that applies multiple edits across files.
func MultiEdit(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[MultiEditParams](
		"multi_edit",
		"Apply multiple file edits in a single operation. Each edit specifies a file, an exact string "+
			"to find (old_string), and its replacement (new_string). All edits are validated before any "+
			"files are written — if validation fails, no files are modified. "+
			"Use this when you need to make coordinated changes across multiple files.",
		func(ctx context.Context, params MultiEditParams) (string, error) {
			if len(params.Edits) == 0 {
				return "", &core.ModelRetryError{Message: "edits list must not be empty"}
			}

			// Phase 1: Validate all edits and compute new file contents.
			// No files are written until all edits pass validation.
			type pendingWrite struct {
				path       string
				relPath    string
				newContent string
				newString  string
				message    string
			}
			var pending []pendingWrite

			// Track already-modified file contents for sequential edits to the
			// same file within a single multi_edit batch.
			fileContents := make(map[string]string)
			// Track which files had CRLF line endings for restoration on write.
			fileCRLF := make(map[string]bool)
			// Track original file permissions so we don't strip executable bits.
			filePerms := make(map[string]os.FileMode)

			for i, edit := range params.Edits {
				if edit.Path == "" {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: path must not be empty", i)}
				}
				if edit.OldString == "" {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: old_string must not be empty", i)}
				}
				if edit.OldString == edit.NewString {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: old_string and new_string are identical — no change needed", i)}
				}

				// Normalize CRLF in params (same as single edit).
				oldStr := strings.ReplaceAll(edit.OldString, "\r\n", "\n")
				newStr := strings.ReplaceAll(edit.NewString, "\r\n", "\n")
				path := edit.Path
				if !filepath.IsAbs(path) && cfg.WorkDir != "" {
					path = filepath.Join(cfg.WorkDir, path)
				}

				if isProtectedTestFile(path) {
					return "", protectedFileError(edit.Path)
				}

				// Use in-memory content if we already modified this file
				// in an earlier edit within this batch.
				content, ok := fileContents[path]
				if !ok {
					fi, err := os.Stat(path)
					if err != nil {
						return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: %v", i, err)}
					}
					filePerms[path] = fi.Mode().Perm()
					data, err := os.ReadFile(path)
					if err != nil {
						return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: %v", i, err)}
					}
					content = string(data)
					// Normalize CRLF → LF for matching (same as single edit).
					if strings.Contains(content, "\r\n") {
						fileCRLF[path] = true
						content = strings.ReplaceAll(content, "\r\n", "\n")
					}
				}

				var newContent string
				var msg string
				if !strings.Contains(content, oldStr) {
					// Try auto-correcting whitespace mismatch.
					if actualOld, adjustedNew, okWs := autoCorrectWhitespace(content, oldStr, newStr); okWs {
						newContent = strings.Replace(content, actualOld, adjustedNew, 1)
						msg = fmt.Sprintf("edit[%d]: auto-corrected whitespace mismatch", i)
					} else if trimmedOld, trimmedNew, okBl := autoCorrectBlankLines(content, oldStr, newStr); okBl && strings.Count(content, trimmedOld) == 1 {
						newContent = strings.Replace(content, trimmedOld, trimmedNew, 1)
						msg = fmt.Sprintf("edit[%d]: auto-corrected extra blank lines", i)
					} else if normalizedOld, normalizedNew, okIbl := autoCorrectInternalBlankLines(content, oldStr, newStr); okIbl && strings.Count(content, normalizedOld) == 1 {
						newContent = strings.Replace(content, normalizedOld, normalizedNew, 1)
						msg = fmt.Sprintf("edit[%d]: auto-corrected internal blank line count", i)
					}
					// Combined internal blank line + whitespace correction.
					if newContent == "" {
						blankNormOld := normalizeBlankRuns(oldStr)
						blankNormNew := normalizeBlankRuns(newStr)
						if blankNormOld != oldStr {
							if actualOld2, adjustedNew2, okWs2 := autoCorrectWhitespace(content, blankNormOld, blankNormNew); okWs2 {
								newContent = strings.Replace(content, actualOld2, adjustedNew2, 1)
								msg = fmt.Sprintf("edit[%d]: auto-corrected internal blank lines and whitespace", i)
							}
						}
					}
					// If we still don't have a newContent, try line trim.
					if newContent == "" {
						if actualOld, adjustedNew, okLt := autoCorrectLineTrim(content, oldStr, newStr); okLt {
							newContent = strings.Replace(content, actualOld, adjustedNew, 1)
							msg = fmt.Sprintf("edit[%d]: auto-corrected by trimming extra context line", i)
						} else {
							errMsg := fmt.Sprintf("edit[%d]: old_string not found in %s.", i, edit.Path)
							if wsHint := detectWhitespaceMismatch(content, oldStr); wsHint != "" {
								errMsg += " " + wsHint
							} else {
								errMsg += " Ensure exact match including whitespace."
								if hint := findNearestLines(content, oldStr, 3); hint != "" {
									errMsg += "\n\nMost similar lines:\n" + hint
								}
							}
							// Include file snippet so the agent can retry without
							// a separate view call — saves a full turn.
							if snippet := fileSnippetForEdit(content, oldStr); snippet != "" {
								errMsg += "\n\nFile content around best match:\n" + snippet
							}
							return "", &core.ModelRetryError{Message: errMsg}
						}
					}
				} else {
					// Check for ambiguous matches (same safety as single edit).
					count := strings.Count(content, oldStr)
					if count > 1 {
						locations := findOccurrenceLines(content, oldStr)
						errMsg := fmt.Sprintf("edit[%d]: old_string found %d times in %s (at lines %s). Provide more surrounding context to make it unique.",
							i, count, edit.Path, locations)
						return "", &core.ModelRetryError{Message: errMsg}
					}
					newContent = strings.Replace(content, oldStr, newStr, 1)
				}
				fileContents[path] = newContent
				pending = append(pending, pendingWrite{
					path:       path,
					relPath:    edit.Path,
					newContent: newContent,
					newString:  newStr,
					message:    msg,
				})
			}

			// Phase 2: Write validated edits to disk.
			var results []string
			for i, pw := range pending {
				writeContent := pw.newContent
				if fileCRLF[pw.path] {
					writeContent = strings.ReplaceAll(writeContent, "\n", "\r\n")
				}
				perm := filePerms[pw.path]
				if perm == 0 {
					perm = 0o644
				}
				if err := os.WriteFile(pw.path, []byte(writeContent), perm); err != nil {
					return "", fmt.Errorf("edit[%d]: write file: %w", i, err)
				}
				if pw.message != "" {
					results = append(results, pw.message)
				}
				results = append(results, editResultWithContext(pw.newContent, pw.newString, 1, pw.relPath))
			}

			return strings.Join(results, "\n\n"), nil
		},
	)
}

// autoCorrectWhitespace attempts to fix a whitespace mismatch automatically.
// When old_string doesn't match exactly but matches exactly once when whitespace
// is normalized (each line trimmed of leading/trailing space, inner runs collapsed),
// it maps the indentation from old_string to the actual content and applies the
// same mapping to new_string.
//
// Returns:
//   - actualOld: the actual content in the file (to use as search string)
//   - adjustedNew: new_string with indentation adjusted to match the file
//   - ok: true if auto-correction was possible (unique normalized match found)
func autoCorrectWhitespace(content, oldStr, newStr string) (actualOld, adjustedNew string, ok bool) {
	// Normalize lines by collapsing whitespace within each line.
	normalizeLine := func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	}
	normalizeBlock := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = normalizeLine(line)
		}
		return strings.Join(lines, "\n")
	}

	normalizedOld := normalizeBlock(oldStr)
	normalizedContent := normalizeBlock(content)

	// Must match exactly once (unique).
	idx := strings.Index(normalizedContent, normalizedOld)
	if idx < 0 {
		return "", "", false
	}
	// Check for a second occurrence.
	if idx2 := strings.Index(normalizedContent[idx+1:], normalizedOld); idx2 >= 0 {
		return "", "", false // ambiguous — multiple normalized matches
	}

	// Map back to actual content lines using newline count.
	matchStartLine := strings.Count(normalizedContent[:idx], "\n")
	oldLines := strings.Split(oldStr, "\n")
	contentLines := strings.Split(content, "\n")

	if matchStartLine+len(oldLines) > len(contentLines) {
		return "", "", false
	}

	actualLines := contentLines[matchStartLine : matchStartLine+len(oldLines)]

	// Verify each line matches when normalized (sanity check).
	for i := range oldLines {
		if normalizeLine(oldLines[i]) != normalizeLine(actualLines[i]) {
			return "", "", false // lines don't correspond
		}
	}

	actualOld = strings.Join(actualLines, "\n")

	// If actual matches old exactly, no correction needed.
	if actualOld == oldStr {
		return "", "", false
	}

	// Build adjusted new_string by mapping indentation from old → actual → new.
	newLines := strings.Split(newStr, "\n")
	adjusted := make([]string, len(newLines))

	for i, newLine := range newLines {
		if i < len(oldLines) {
			oldIndent := leadingWhitespace(oldLines[i])
			actualIndent := leadingWhitespace(actualLines[i])
			newIndent := leadingWhitespace(newLine)

			if newIndent == oldIndent {
				// New line has same indent as old — swap to actual's indent.
				adjusted[i] = actualIndent + strings.TrimLeft(newLine, " \t")
			} else {
				// Model intentionally changed indent relative to old.
				// Compute the relative change and apply to actual.
				adjusted[i] = applyRelativeIndent(oldIndent, actualIndent, newIndent, newLine)
			}
		} else {
			// Extra lines in new_string (model added lines).
			// Try to match the indent pattern: use the last actual line's
			// indent mapping as a baseline.
			if len(oldLines) > 0 {
				lastOldIndent := leadingWhitespace(oldLines[len(oldLines)-1])
				lastActualIndent := leadingWhitespace(actualLines[len(actualLines)-1])
				newIndent := leadingWhitespace(newLine)
				adjusted[i] = applyRelativeIndent(lastOldIndent, lastActualIndent, newIndent, newLine)
			} else {
				adjusted[i] = newLine
			}
		}
	}

	adjustedNew = strings.Join(adjusted, "\n")
	return actualOld, adjustedNew, true
}

// leadingWhitespace returns the leading whitespace of a string.
func leadingWhitespace(s string) string {
	trimmed := strings.TrimLeft(s, " \t")
	return s[:len(s)-len(trimmed)]
}

// applyRelativeIndent computes the relative indent change from oldIndent to
// newIndent and applies the same relative change to actualIndent.
func applyRelativeIndent(oldIndent, actualIndent, newIndent string, newLine string) string {
	oldWidth := indentWidth(oldIndent)
	actualWidth := indentWidth(actualIndent)
	newWidth := indentWidth(newIndent)

	// Compute relative change in indent columns.
	delta := newWidth - oldWidth
	targetWidth := actualWidth + delta
	if targetWidth < 0 {
		targetWidth = 0
	}

	// Determine indent character from actual (preserve file convention).
	// indentUnit is the visual column width of one indentChar so that
	// Repeat(indentChar, targetWidth/indentUnit) produces the correct
	// visual width. Tabs are 4 columns (matching indentWidth), spaces are 1.
	indentChar := "\t"
	indentUnit := 4
	if strings.Contains(actualIndent, " ") && !strings.Contains(actualIndent, "\t") {
		indentChar = " "
		indentUnit = 1
	} else if actualIndent == "" && strings.Contains(newIndent, " ") {
		indentChar = " "
		indentUnit = 1
	}

	return strings.Repeat(indentChar, targetWidth/indentUnit) + strings.TrimLeft(newLine, " \t")
}

// fileSnippetForEdit returns a compact snippet of the file around the area
// most similar to the search string. This allows the agent to immediately
// retry the edit with the correct content, saving a full view + edit cycle.
func fileSnippetForEdit(content, search string) string {
	if len(content) == 0 || len(search) == 0 {
		return ""
	}

	lines := strings.Split(content, "\n")
	searchLines := strings.Split(search, "\n")
	if len(searchLines) == 0 || len(lines) == 0 {
		return ""
	}

	// Use the first non-trivial line as the search anchor.
	// When the first line is too short (e.g., "{", "}"), fall back
	// to subsequent lines to find a meaningful match.
	anchorLine := ""
	for _, sl := range searchLines {
		trimmed := strings.TrimSpace(sl)
		if len(trimmed) >= 3 {
			anchorLine = trimmed
			break
		}
	}
	if anchorLine == "" {
		return ""
	}

	bestLine := -1
	bestScore := 0
	searchWords := strings.Fields(strings.ToLower(anchorLine))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 3 {
			continue
		}
		lineWords := strings.Fields(strings.ToLower(trimmed))
		score := 0
		for _, sw := range searchWords {
			for _, lw := range lineWords {
				if sw == lw {
					score++
					break
				}
			}
		}
		// Bonus for substring match.
		if strings.Contains(strings.ToLower(trimmed), strings.ToLower(anchorLine)) {
			score += len(searchWords)
		}
		if score > bestScore {
			bestScore = score
			bestLine = i
		}
	}

	if bestLine < 0 || bestScore < 2 {
		return ""
	}

	// Show a window around the best match: enough context to retry the edit.
	contextBefore := 5
	contextAfter := len(searchLines) + 5
	start := max(0, bestLine-contextBefore)
	end := min(len(lines), bestLine+contextAfter)

	// Cap snippet at 30 lines to avoid bloating the error.
	if end-start > 30 {
		end = start + 30
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}
	if end < len(lines) {
		b.WriteString("       ...\n")
	}
	return b.String()
}

// autoCorrectBlankLines strips leading and trailing blank lines from old_string
// and corresponding lines from new_string. Models often include extra blank lines
// at boundaries that don't exist in the file. Returns the trimmed strings and
// true if any blank lines were stripped.
func autoCorrectBlankLines(content, oldStr, newStr string) (trimmedOld, trimmedNew string, ok bool) {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// Count leading blank lines in old_string.
	leadingBlanks := 0
	for _, line := range oldLines {
		if strings.TrimSpace(line) == "" {
			leadingBlanks++
		} else {
			break
		}
	}

	// Count trailing blank lines in old_string.
	trailingBlanks := 0
	for i := len(oldLines) - 1; i >= leadingBlanks; i-- {
		if strings.TrimSpace(oldLines[i]) == "" {
			trailingBlanks++
		} else {
			break
		}
	}

	if leadingBlanks == 0 && trailingBlanks == 0 {
		return "", "", false // nothing to strip
	}

	// Strip the same number of leading/trailing lines from old and new.
	oldEnd := len(oldLines) - trailingBlanks
	if oldEnd <= leadingBlanks {
		return "", "", false // all blank lines
	}
	trimmedOld = strings.Join(oldLines[leadingBlanks:oldEnd], "\n")

	// Strip corresponding blank lines from new_string. Only strip lines
	// that are actually blank — never remove content lines from new.
	newLeadingBlanks := 0
	for _, line := range newLines {
		if strings.TrimSpace(line) == "" {
			newLeadingBlanks++
		} else {
			break
		}
	}
	newTrailingBlanks := 0
	for i := len(newLines) - 1; i >= newLeadingBlanks; i-- {
		if strings.TrimSpace(newLines[i]) == "" {
			newTrailingBlanks++
		} else {
			break
		}
	}
	newStart := min(leadingBlanks, newLeadingBlanks)
	newEnd := len(newLines) - min(trailingBlanks, newTrailingBlanks)
	if newEnd <= newStart {
		trimmedNew = ""
	} else {
		trimmedNew = strings.Join(newLines[newStart:newEnd], "\n")
	}

	// Only return true if the trimmed old_string is different from the original
	// (i.e., we actually stripped something).
	if trimmedOld == oldStr {
		return "", "", false
	}
	return trimmedOld, trimmedNew, true
}

// autoCorrectInternalBlankLines normalizes runs of consecutive blank lines
// to a single blank line in both old_string and content, then tries matching.
// Models commonly generate 2 blank lines between functions when the file has 1
// (or vice versa). This is the #3 edit failure after indentation and
// leading/trailing blank lines.
//
// Returns:
//   - actualOld: the actual content region in the file (with original blank lines)
//   - adjustedNew: new_string (kept as-is — model's intended blank lines)
//   - ok: true if normalization produced a unique match
//
// normalizeBlankRuns collapses runs of 2+ consecutive blank lines to 1 blank line.
func normalizeBlankRuns(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	prevBlank := false
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		result = append(result, line)
		prevBlank = blank
	}
	return strings.Join(result, "\n")
}

func autoCorrectInternalBlankLines(content, oldStr, newStr string) (actualOld, adjustedNew string, ok bool) {
	normalizedOld := normalizeBlankRuns(oldStr)
	normalizedContent := normalizeBlankRuns(content)

	// Only proceed if normalization actually changed something.
	if normalizedOld == oldStr && normalizedContent == content {
		return "", "", false
	}

	// Must match exactly once in normalized content.
	idx := strings.Index(normalizedContent, normalizedOld)
	if idx < 0 {
		return "", "", false
	}
	// Check uniqueness.
	if strings.Contains(normalizedContent[idx+1:], normalizedOld) {
		return "", "", false
	}

	// Build a mapping from normalized line index → actual content line index.
	// This handles the case where the actual file has runs of blank lines
	// that were collapsed during normalization.
	contentLines := strings.Split(content, "\n")
	normalizedToActual := make([]int, 0, len(contentLines))
	prevBlank := false
	for i, line := range contentLines {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue // collapsed in normalization
		}
		normalizedToActual = append(normalizedToActual, i)
		prevBlank = blank
	}

	// Find the normalized line range of the match.
	normalizedStartLine := strings.Count(normalizedContent[:idx], "\n")
	normalizedEndLine := normalizedStartLine + strings.Count(normalizedOld, "\n")

	if normalizedStartLine >= len(normalizedToActual) || normalizedEndLine >= len(normalizedToActual) {
		return "", "", false
	}

	actualStartLine := normalizedToActual[normalizedStartLine]

	// The actual end line is the line AFTER the last matched normalized line,
	// plus any consecutive blank lines that were collapsed.
	actualEndLine := normalizedToActual[normalizedEndLine] + 1
	// Include any blank lines immediately after the match that were part of
	// the collapsed run (they belong to this region in the actual file).
	for actualEndLine < len(contentLines) {
		if normalizedEndLine+1 < len(normalizedToActual) && actualEndLine >= normalizedToActual[normalizedEndLine+1] {
			break // reached the next normalized line's actual position
		}
		if strings.TrimSpace(contentLines[actualEndLine]) != "" {
			break
		}
		actualEndLine++
	}

	if actualEndLine > len(contentLines) {
		actualEndLine = len(contentLines)
	}
	if actualEndLine <= actualStartLine {
		return "", "", false
	}

	actualOld = strings.Join(contentLines[actualStartLine:actualEndLine], "\n")

	// Verify the normalized versions match (sanity check).
	if normalizeBlankRuns(actualOld) != normalizedOld {
		return "", "", false
	}

	// Keep new_string as-is — the model's intended blank lines are
	// probably correct for the replacement.
	adjustedNew = newStr

	return actualOld, adjustedNew, true
}

// autoCorrectLineTrim handles the case where the model included an extra
// context line at the beginning or end of old_string that doesn't exist in
// the file (or exists with different content). This is a common failure mode:
// the model copies a nearby line for context but gets it slightly wrong.
//
// Safety: only trims if (1) old_string has 3+ lines, (2) the trimmed line is
// unchanged between old and new (pure context, not an intended edit), and
// (3) the trimmed version matches exactly once in the file.
func autoCorrectLineTrim(content, oldStr, newStr string) (actualOld, adjustedNew string, ok bool) {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// Need at least 3 lines to try trimming — too aggressive on shorter edits.
	if len(oldLines) < 3 || len(newLines) < 3 {
		return "", "", false
	}

	// Try dropping the first line (if first lines are identical context).
	if len(oldLines) > 0 && len(newLines) > 0 && oldLines[0] == newLines[0] {
		trimmedOld := strings.Join(oldLines[1:], "\n")
		if strings.Count(content, trimmedOld) == 1 {
			trimmedNew := strings.Join(newLines[1:], "\n")
			return trimmedOld, trimmedNew, true
		}
	}

	// Try dropping the last line (if last lines are identical context).
	if len(oldLines) > 0 && len(newLines) > 0 {
		lastOld := oldLines[len(oldLines)-1]
		lastNew := newLines[len(newLines)-1]
		if lastOld == lastNew {
			trimmedOld := strings.Join(oldLines[:len(oldLines)-1], "\n")
			if strings.Count(content, trimmedOld) == 1 {
				trimmedNew := strings.Join(newLines[:len(newLines)-1], "\n")
				return trimmedOld, trimmedNew, true
			}
		}
	}

	// Try dropping both first AND last line simultaneously.
	// The model sometimes includes extra context at both ends that
	// doesn't match the file (e.g., surrounding lines changed by a
	// previous edit). Requires 5+ lines to leave enough content.
	if len(oldLines) >= 5 && len(newLines) >= 5 &&
		oldLines[0] == newLines[0] &&
		oldLines[len(oldLines)-1] == newLines[len(newLines)-1] {
		trimmedOld := strings.Join(oldLines[1:len(oldLines)-1], "\n")
		if strings.Count(content, trimmedOld) == 1 {
			trimmedNew := strings.Join(newLines[1:len(newLines)-1], "\n")
			return trimmedOld, trimmedNew, true
		}
	}

	return "", "", false
}

// indentWidth computes the visual width of an indent string (tabs = 4 cols).
func indentWidth(indent string) int {
	width := 0
	for _, c := range indent {
		if c == '\t' {
			width += 4
		} else {
			width++
		}
	}
	return width
}

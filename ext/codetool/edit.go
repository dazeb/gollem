package codetool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

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

			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("file not found: %s. Use the write tool to create new files.", params.Path)}
				}
				return "", fmt.Errorf("read file: %w", err)
			}

			content := string(data)
			count := strings.Count(content, params.OldString)

			if count == 0 {
				return "", &core.ModelRetryError{
					Message: fmt.Sprintf("old_string not found in %s. Use the view tool to check the file contents and ensure exact match including whitespace.", params.Path),
				}
			}

			if count > 1 && !params.ReplaceAll {
				return "", &core.ModelRetryError{
					Message: fmt.Sprintf("old_string found %d times in %s. Provide more surrounding context to make it unique, or set replace_all=true.", count, params.Path),
				}
			}

			var newContent string
			if params.ReplaceAll {
				newContent = strings.ReplaceAll(content, params.OldString, params.NewString)
			} else {
				newContent = strings.Replace(content, params.OldString, params.NewString, 1)
			}

			if err := os.WriteFile(path, []byte(newContent), 0o600); err != nil {
				return "", fmt.Errorf("write file: %w", err)
			}

			replacements := 1
			if params.ReplaceAll {
				replacements = count
			}
			return fmt.Sprintf("Replaced %d occurrence(s) in %s", replacements, params.Path), nil
		},
	)
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
			"to find (old_string), and its replacement (new_string). Edits are applied in order. "+
			"Use this when you need to make coordinated changes across multiple files.",
		func(ctx context.Context, params MultiEditParams) (string, error) {
			if len(params.Edits) == 0 {
				return "", &core.ModelRetryError{Message: "edits list must not be empty"}
			}

			var results []string
			for i, edit := range params.Edits {
				if edit.Path == "" {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: path must not be empty", i)}
				}
				if edit.OldString == "" {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: old_string must not be empty", i)}
				}

				path := edit.Path
				if !filepath.IsAbs(path) && cfg.WorkDir != "" {
					path = filepath.Join(cfg.WorkDir, path)
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("edit[%d]: %v", i, err)}
				}

				content := string(data)
				if !strings.Contains(content, edit.OldString) {
					return "", &core.ModelRetryError{
						Message: fmt.Sprintf("edit[%d]: old_string not found in %s", i, edit.Path),
					}
				}

				newContent := strings.Replace(content, edit.OldString, edit.NewString, 1)
				if err := os.WriteFile(path, []byte(newContent), 0o600); err != nil {
					return "", fmt.Errorf("edit[%d]: write file: %w", i, err)
				}
				results = append(results, "edited "+edit.Path)
			}

			return fmt.Sprintf("Applied %d edit(s): %s", len(results), strings.Join(results, ", ")), nil
		},
	)
}

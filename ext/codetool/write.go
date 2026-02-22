package codetool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

			// Create parent directories.
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", fmt.Errorf("create directories: %w", err)
			}

			if err := os.WriteFile(path, []byte(params.Content), 0o644); err != nil {
				return "", fmt.Errorf("write file: %w", err)
			}

			return fmt.Sprintf("Wrote %d bytes to %s", len(params.Content), params.Path), nil
		},
	)
}

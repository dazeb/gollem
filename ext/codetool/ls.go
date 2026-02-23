package codetool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// LsParams are the parameters for the ls tool.
type LsParams struct {
	// Path is the directory to list.
	Path string `json:"path,omitempty" jsonschema:"description=Directory path to list. Defaults to working directory."`

	// Depth is the maximum directory depth to recurse.
	Depth *int `json:"depth,omitempty" jsonschema:"description=Maximum recursion depth. 1 = immediate children only. Default: 1"`
}

// Ls creates a tool that lists directory contents.
func Ls(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[LsParams](
		"ls",
		"List files and directories. Shows entries with type indicators (/ for directories). "+
			"Use depth > 1 to see nested directory structure. "+
			"Automatically skips common non-essential directories (.git, node_modules, etc.).",
		func(ctx context.Context, params LsParams) (string, error) {
			dir := params.Path
			if dir == "" {
				dir = "."
			}
			if !filepath.IsAbs(dir) && cfg.WorkDir != "" {
				dir = filepath.Join(cfg.WorkDir, dir)
			}

			info, err := os.Stat(dir)
			if err != nil {
				if os.IsNotExist(err) {
					return "", &core.ModelRetryError{Message: "directory not found: " + params.Path}
				}
				return "", fmt.Errorf("stat: %w", err)
			}
			if !info.IsDir() {
				return "", &core.ModelRetryError{Message: params.Path + " is a file, not a directory. Use the view tool to read it."}
			}

			depth := 1
			if params.Depth != nil && *params.Depth > 0 {
				depth = *params.Depth
			}
			if depth > 5 {
				depth = 5 // cap depth to avoid huge output
			}

			var lines []string
			listDir(ctx, dir, dir, "", depth, &lines)

			if len(lines) == 0 {
				return "(empty directory)", nil
			}

			result := strings.Join(lines, "\n")
			// Don't add count if truncated (listDir already appends a truncation notice).
			if len(lines) <= 500 {
				result += fmt.Sprintf("\n(%d entries)", len(lines))
			}
			return result, nil
		},
	)
}

// compactSize formats bytes into a compact human-readable string.
func compactSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func listDir(ctx context.Context, basePath, currentPath, prefix string, remainingDepth int, lines *[]string) {
	if ctx.Err() != nil {
		return
	}
	if remainingDepth <= 0 {
		return
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}

		name := entry.Name()

		// Skip hidden and common non-essential dirs.
		if entry.IsDir() && isSkippableDir(name) {
			continue
		}

		display := prefix + name
		if entry.IsDir() {
			display += "/"
			*lines = append(*lines, display)
			listDir(ctx, basePath, filepath.Join(currentPath, name), prefix+name+"/", remainingDepth-1, lines)
		} else {
			// Show file size to save agents from needing `ls -la` via bash.
			if info, err := entry.Info(); err == nil {
				display += fmt.Sprintf("  (%s)", compactSize(info.Size()))
			}
			*lines = append(*lines, display)
		}

		if len(*lines) > 500 {
			*lines = append(*lines, "... (truncated at 500 entries)")
			return
		}
	}
}

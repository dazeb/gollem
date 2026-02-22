package codetool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// ViewParams are the parameters for the view tool.
type ViewParams struct {
	// Path is the file path to read.
	Path string `json:"path" jsonschema:"description=Absolute or relative file path to read"`

	// Offset is the 1-based line number to start reading from.
	Offset *int `json:"offset,omitempty" jsonschema:"description=Start reading from this line number (1-based). Default: 1"`

	// Limit is the maximum number of lines to read.
	Limit *int `json:"limit,omitempty" jsonschema:"description=Maximum number of lines to return. Default: 2000"`
}

// View creates a tool that reads file contents with optional line range.
func View(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[ViewParams](
		"view",
		"Read the contents of a file. Returns the file content with line numbers. "+
			"Use offset and limit to read specific sections of large files. "+
			"Always use this to read a file before editing it.",
		func(ctx context.Context, params ViewParams) (string, error) {
			if params.Path == "" {
				return "", &core.ModelRetryError{Message: "path must not be empty"}
			}

			path := params.Path
			if !filepath.IsAbs(path) && cfg.WorkDir != "" {
				path = filepath.Join(cfg.WorkDir, path)
			}

			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return "", &core.ModelRetryError{Message: "file not found: " + params.Path}
				}
				return "", fmt.Errorf("stat file: %w", err)
			}
			if info.IsDir() {
				return "", &core.ModelRetryError{Message: params.Path + " is a directory, not a file. Use the ls tool instead."}
			}
			if cfg.MaxFileSize > 0 && info.Size() > cfg.MaxFileSize {
				return "", &core.ModelRetryError{
					Message: fmt.Sprintf("file too large (%d bytes, max %d). Use offset/limit to read sections.", info.Size(), cfg.MaxFileSize),
				}
			}

			// Quick binary file check: read the first 512 bytes and look for
			// null bytes. Binary files waste context tokens and confuse the model.
			if info.Size() > 0 {
				probe := make([]byte, 512)
				if pf, err := os.Open(path); err == nil {
					n, _ := pf.Read(probe)
					pf.Close()
					nullCount := 0
					for _, b := range probe[:n] {
						if b == 0 {
							nullCount++
						}
					}
					if nullCount > 5 {
						return fmt.Sprintf("Binary file (%d bytes). Use bash tools (hexdump, xxd, file) to inspect binary files, or write a Python script to process them.",
							info.Size()), nil
					}
				}
			}

			f, err := os.Open(path)
			if err != nil {
				return "", fmt.Errorf("open file: %w", err)
			}
			defer f.Close()

			offset := 1
			if params.Offset != nil && *params.Offset > 0 {
				offset = *params.Offset
			}
			limit := 2000
			if params.Limit != nil && *params.Limit > 0 {
				limit = *params.Limit
			}

			var lines []string
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // handle long lines
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				if lineNum < offset {
					continue
				}
				if lineNum >= offset+limit {
					// Keep counting total lines.
					for scanner.Scan() {
						lineNum++
					}
					break
				}
				line := scanner.Text()
				// Truncate very long lines.
				if len(line) > 2000 {
					line = line[:2000] + "..."
				}
				lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, line))
			}
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("reading file: %w", err)
			}

			if len(lines) == 0 {
				if lineNum == 0 {
					return "(empty file)", nil
				}
				return fmt.Sprintf("(no lines in range %d-%d, file has %d lines)", offset, offset+limit-1, lineNum), nil
			}

			result := strings.Join(lines, "\n")
			if lineNum >= offset+limit-1 {
				result += fmt.Sprintf("\n... (%d total lines, showing %d-%d)", lineNum, offset, offset+len(lines)-1)
			}
			return result, nil
		},
	)
}

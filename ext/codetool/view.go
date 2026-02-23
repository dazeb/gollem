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
	// Negative values count from the end of the file (e.g. -20 = last 20 lines).
	Offset *int `json:"offset,omitempty" jsonschema:"description=Start reading from this line number (1-based). Use negative values to read from end of file (e.g. -20 = last 20 lines). Default: 1"`

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
			"Use negative offset to read from end of file (e.g. offset=-20 for last 20 lines). "+
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

			limit := 2000
			if params.Limit != nil && *params.Limit > 0 {
				limit = *params.Limit
			}

			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // handle long lines

			// Negative offset: read from end of file. Read all lines
			// into memory and select the tail. MaxFileSize guard above
			// prevents this from blowing up on huge files.
			if params.Offset != nil && *params.Offset < 0 {
				var allLines []string
				for scanner.Scan() {
					allLines = append(allLines, scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					return "", fmt.Errorf("reading file: %w", err)
				}
				if len(allLines) == 0 {
					return "(empty file)", nil
				}

				// -N means "last N lines" (capped by limit).
				tailCount := -*params.Offset
				if tailCount > len(allLines) {
					tailCount = len(allLines)
				}
				if tailCount > limit {
					tailCount = limit
				}
				start := len(allLines) - tailCount

				var lines []string
				for i := start; i < len(allLines); i++ {
					line := allLines[i]
					if len(line) > 2000 {
						line = line[:2000] + "..."
					}
					lines = append(lines, fmt.Sprintf("%6d\t%s", i+1, line))
				}

				result := strings.Join(lines, "\n")
				result += fmt.Sprintf("\n(%d total lines, showing %d-%d)", len(allLines), start+1, len(allLines))
				return result, nil
			}

			// Positive offset: standard forward reading.
			offset := 1
			if params.Offset != nil && *params.Offset > 0 {
				offset = *params.Offset
			}

			var lines []string
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
			if lineNum >= offset+limit {
				result += fmt.Sprintf("\n... (%d total lines, showing %d-%d)", lineNum, offset, offset+len(lines)-1)
			} else if offset > 1 {
				// Show total line count even when not truncated at the end,
				// if the user started at an offset.
				result += fmt.Sprintf("\n(%d total lines, showing %d-%d)", lineNum, offset, offset+len(lines)-1)
			} else {
				// Show total line count for full file reads to help agents
				// plan reading strategies without needing `wc -l`.
				result += fmt.Sprintf("\n(%d lines)", lineNum)
			}

			// Warn about minified files: very few lines relative to file size
			// suggests the file is minified/bundled. Editing minified files is
			// usually futile — the agent should look for the source instead.
			if info.Size() > 5000 && lineNum > 0 && lineNum <= 5 {
				avgLineLen := int(info.Size()) / max(lineNum, 1)
				if avgLineLen > 500 {
					result += "\n[hint: this file appears to be minified/bundled (very long lines). " +
						"Editing minified code is error-prone. Look for the unminified source " +
						"file instead, or use a different approach.]"
				}
			}

			return result, nil
		},
	)
}

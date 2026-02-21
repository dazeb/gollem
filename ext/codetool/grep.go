package codetool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// GrepParams are the parameters for the grep tool.
type GrepParams struct {
	// Pattern is the regex pattern to search for.
	Pattern string `json:"pattern" jsonschema:"description=Regular expression pattern to search for in file contents"`

	// Path is the directory or file to search in.
	Path string `json:"path,omitempty" jsonschema:"description=Directory or file to search in. Defaults to working directory."`

	// Include is a glob pattern to filter files (e.g. '*.go', '*.py').
	Include string `json:"include,omitempty" jsonschema:"description=Glob pattern to filter files (e.g. '*.go'). Applied to filename only."`

	// MaxResults limits the number of matching lines returned.
	MaxResults *int `json:"max_results,omitempty" jsonschema:"description=Maximum number of matching lines to return. Default: 100"`

	// ContextLines is the number of lines to show before and after each match.
	ContextLines *int `json:"context_lines,omitempty" jsonschema:"description=Number of context lines before and after each match. Default: 0"`
}

// GrepMatch is a single matching line.
type GrepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// Grep creates a tool that searches file contents using regex patterns.
func Grep(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[GrepParams](
		"grep",
		"Search file contents for lines matching a regular expression pattern. "+
			"Returns matching lines with file paths and line numbers. "+
			"Use the include parameter to filter by file extension (e.g. '*.go'). "+
			"Use this to find function definitions, usages, imports, error messages, etc.",
		func(ctx context.Context, params GrepParams) (string, error) {
			if params.Pattern == "" {
				return "", &core.ModelRetryError{Message: "pattern must not be empty"}
			}

			re, err := regexp.Compile(params.Pattern)
			if err != nil {
				return "", &core.ModelRetryError{Message: fmt.Sprintf("invalid regex: %v", err)}
			}

			searchPath := params.Path
			if searchPath == "" {
				searchPath = "."
			}
			if !filepath.IsAbs(searchPath) && cfg.WorkDir != "" {
				searchPath = filepath.Join(cfg.WorkDir, searchPath)
			}

			maxResults := 100
			if params.MaxResults != nil && *params.MaxResults > 0 {
				maxResults = *params.MaxResults
			}

			contextLines := 0
			if params.ContextLines != nil && *params.ContextLines >= 0 {
				contextLines = *params.ContextLines
			}

			var matches []string
			truncated := false

			err = filepath.Walk(searchPath, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if info.IsDir() {
					base := info.Name()
					if base == ".git" || base == "node_modules" || base == "__pycache__" || base == ".tox" || base == "vendor" {
						return filepath.SkipDir
					}
					return nil
				}

				// Skip binary and large files.
				if info.Size() > 1<<20 { // 1MB
					return nil
				}

				// Apply include filter.
				if params.Include != "" {
					matched, _ := filepath.Match(params.Include, info.Name())
					if !matched {
						return nil
					}
				}

				// Skip likely binary files.
				if isBinaryFilename(info.Name()) {
					return nil
				}

				relPath, _ := filepath.Rel(cfg.WorkDir, path)
				if relPath == "" || strings.HasPrefix(relPath, "..") {
					relPath = path
				}

				return searchFile(ctx, path, relPath, re, contextLines, maxResults, &matches, &truncated)
			})

			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				return "", fmt.Errorf("searching files: %w", err)
			}

			if len(matches) == 0 {
				return "No matches found.", nil
			}

			result := strings.Join(matches, "\n")
			if truncated {
				result += fmt.Sprintf("\n... (results truncated at %d matches)", maxResults)
			}
			return result, nil
		},
	)
}

func searchFile(ctx context.Context, absPath, relPath string, re *regexp.Regexp, contextLines, maxResults int, matches *[]string, truncated *bool) error {
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var allLines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	for i, line := range allLines {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(*matches) >= maxResults {
			*truncated = true
			return filepath.SkipAll
		}
		if re.MatchString(line) {
			if contextLines > 0 {
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				end := i + contextLines + 1
				if end > len(allLines) {
					end = len(allLines)
				}
				for j := start; j < end; j++ {
					prefix := " "
					if j == i {
						prefix = ">"
					}
					*matches = append(*matches, fmt.Sprintf("%s%s:%d: %s", prefix, relPath, j+1, allLines[j]))
				}
				*matches = append(*matches, "---")
			} else {
				*matches = append(*matches, fmt.Sprintf("%s:%d: %s", relPath, i+1, line))
			}
		}
	}
	return nil
}

func isBinaryFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".svg",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z",
		".exe", ".dll", ".so", ".dylib", ".o", ".a",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".wasm", ".pyc", ".class",
		".mp3", ".mp4", ".avi", ".mov", ".mkv",
		".ttf", ".otf", ".woff", ".woff2",
		".sqlite", ".db":
		return true
	}
	return false
}

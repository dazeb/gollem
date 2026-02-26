package codetool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// GlobParams are the parameters for the glob tool.
type GlobParams struct {
	// Pattern is the glob pattern to match files against.
	Pattern string `json:"pattern" jsonschema:"description=Glob pattern to match files (e.g. '**/*.go', 'src/**/*.ts', '*.py'). Supports ** for recursive matching."`

	// Path is the directory to search in.
	Path string `json:"path,omitempty" jsonschema:"description=Directory to search in. Defaults to working directory."`

	// Exclude is a glob pattern to skip files (e.g. '*_test.go', '*.min.js').
	Exclude string `json:"exclude,omitempty" jsonschema:"description=Glob pattern to exclude files (e.g. '*_test.go'). Applied to filename only."`

	// MaxResults limits the number of results.
	MaxResults *int `json:"max_results,omitempty" jsonschema:"description=Maximum number of results to return. Default: 200"`
}

// Glob creates a tool that finds files matching glob patterns.
func Glob(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[GlobParams](
		"glob",
		"Find files matching a glob pattern. Supports ** for recursive directory matching "+
			"and {a,b} brace expansion (e.g. '**/*.{ts,tsx}'). "+
			"Returns file paths sorted by modification time (most recent first). "+
			"Use exclude to skip files (e.g. '*_test.go'). "+
			"Use this to discover files by name or extension (e.g. '**/*.go', 'src/**/*.{ts,tsx}').",
		func(ctx context.Context, params GlobParams) (string, error) {
			if params.Pattern == "" {
				return "", &core.ModelRetryError{Message: "pattern must not be empty"}
			}

			searchPath := params.Path
			if searchPath == "" {
				searchPath = "."
			}
			if !filepath.IsAbs(searchPath) && cfg.WorkDir != "" {
				searchPath = filepath.Join(cfg.WorkDir, searchPath)
			}

			maxResults := 200
			if params.MaxResults != nil && *params.MaxResults > 0 {
				maxResults = *params.MaxResults
			}

			type fileEntry struct {
				path    string
				modTime int64
				size    int64
			}

			var results []fileEntry

			// Handle ** pattern with recursive walk.
			// WalkDir is faster than Walk because it avoids Stat on every
			// entry — we only call Info() on matching files.
			if strings.Contains(params.Pattern, "**") {
				err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if d.IsDir() {
						if isSkippableDir(d.Name()) {
							return filepath.SkipDir
						}
						return nil
					}

					// Apply exclude filter (with brace expansion).
					if params.Exclude != "" {
						if matchWithBraces(params.Exclude, d.Name()) {
							return nil
						}
					}

					relPath, _ := filepath.Rel(searchPath, path)
					if matchDoublestar(params.Pattern, relPath) {
						// Only call Info() on matches — avoids Stat on
						// the vast majority of non-matching files.
						if info, err := d.Info(); err == nil {
							results = append(results, fileEntry{relPath, info.ModTime().Unix(), info.Size()})
						}
					}
					return nil
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					return "", fmt.Errorf("walking directory: %w", err)
				}
			} else {
				// Simple glob without **. Expand braces first since
				// filepath.Glob doesn't support {a,b} patterns.
				expandedPatterns := expandBraces(params.Pattern)
				seen := make(map[string]bool)
				var globMatches []string
				for _, ep := range expandedPatterns {
					p := filepath.Join(searchPath, ep)
					matches, err := filepath.Glob(p)
					if err != nil {
						return "", &core.ModelRetryError{Message: fmt.Sprintf("invalid glob pattern: %v", err)}
					}
					for _, m := range matches {
						if !seen[m] {
							seen[m] = true
							globMatches = append(globMatches, m)
						}
					}
				}
				for _, m := range globMatches {
					info, err := os.Stat(m)
					if err != nil {
						continue
					}
					if info.IsDir() {
						continue
					}
					// Apply exclude filter (with brace expansion).
					if params.Exclude != "" {
						if matchWithBraces(params.Exclude, info.Name()) {
							continue
						}
					}
					relPath, _ := filepath.Rel(searchPath, m)
					results = append(results, fileEntry{relPath, info.ModTime().Unix(), info.Size()})
				}
			}

			// Sort by modification time, most recent first.
			sort.Slice(results, func(i, j int) bool {
				return results[i].modTime > results[j].modTime
			})

			if len(results) == 0 {
				return "No files matched.", nil
			}

			truncated := false
			if len(results) > maxResults {
				results = results[:maxResults]
				truncated = true
			}

			var lines []string
			for _, r := range results {
				lines = append(lines, fmt.Sprintf("%s  (%s)", r.path, compactSize(r.size)))
			}

			result := strings.Join(lines, "\n")
			if truncated {
				result += fmt.Sprintf("\n... (truncated at %d results)", maxResults)
			} else {
				result += fmt.Sprintf("\n(%d files)", len(results))
			}
			return result, nil
		},
	)
}

// matchDoublestar implements simple ** glob matching with brace expansion.
func matchDoublestar(pattern, path string) bool {
	// Normalize separators.
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Expand brace patterns like *.{go,py} into [*.go, *.py].
	// filepath.Match doesn't support brace expansion, but models use
	// it frequently (e.g., **/*.{ts,tsx}).
	patterns := expandBraces(pattern)
	for _, p := range patterns {
		if matchSegments(strings.Split(p, "/"), strings.Split(path, "/")) {
			return true
		}
	}
	return false
}

// expandBraces expands a single brace group in a pattern.
// "*.{go,py}" → ["*.go", "*.py"]
// "src/{a,b}/*.ts" → ["src/a/*.ts", "src/b/*.ts"]
// Patterns without braces return a single-element slice.
// Only the first brace group is expanded (nested braces are not supported).
func expandBraces(pattern string) []string {
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		return []string{pattern}
	}
	close := strings.IndexByte(pattern[open:], '}')
	if close < 0 {
		return []string{pattern}
	}
	close += open
	prefix := pattern[:open]
	suffix := pattern[close+1:]
	alternatives := strings.Split(pattern[open+1:close], ",")
	var result []string
	for _, alt := range alternatives {
		result = append(result, prefix+strings.TrimSpace(alt)+suffix)
	}
	return result
}

// matchWithBraces matches a filename against a pattern that may contain
// {a,b} brace expansion. Used by grep include/exclude filters.
func matchWithBraces(pattern, name string) bool {
	for _, p := range expandBraces(pattern) {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
	}
	return false
}

func matchSegments(patternParts, pathParts []string) bool {
	if len(patternParts) == 0 {
		return len(pathParts) == 0
	}

	p := patternParts[0]

	if p == "**" {
		// ** matches zero or more path segments.
		rest := patternParts[1:]
		for i := 0; i <= len(pathParts); i++ {
			if matchSegments(rest, pathParts[i:]) {
				return true
			}
		}
		return false
	}

	if len(pathParts) == 0 {
		return false
	}

	matched, _ := filepath.Match(p, pathParts[0])
	if !matched {
		return false
	}

	return matchSegments(patternParts[1:], pathParts[1:])
}

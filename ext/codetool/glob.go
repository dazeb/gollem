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
		"Find files matching a glob pattern. Supports ** for recursive directory matching. "+
			"Returns file paths sorted by modification time (most recent first). "+
			"Use exclude to skip files (e.g. '*_test.go'). "+
			"Use this to discover files by name or extension (e.g. '**/*.go', 'src/**/*.test.ts').",
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
			if strings.Contains(params.Pattern, "**") {
				// Split pattern into directory prefix and file pattern.
				err := filepath.Walk(searchPath, func(path string, info os.FileInfo, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if info.IsDir() {
						base := info.Name()
						if isSkippableDir(base) {
							return filepath.SkipDir
						}
						return nil
					}

					// Apply exclude filter.
					if params.Exclude != "" {
						excluded, _ := filepath.Match(params.Exclude, info.Name())
						if excluded {
							return nil
						}
					}

					relPath, _ := filepath.Rel(searchPath, path)
					if matchDoublestar(params.Pattern, relPath) {
						results = append(results, fileEntry{relPath, info.ModTime().Unix(), info.Size()})
					}
					return nil
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					return "", fmt.Errorf("walking directory: %w", err)
				}
			} else {
				// Simple glob without **.
				pattern := filepath.Join(searchPath, params.Pattern)
				globMatches, err := filepath.Glob(pattern)
				if err != nil {
					return "", &core.ModelRetryError{Message: fmt.Sprintf("invalid glob pattern: %v", err)}
				}
				for _, m := range globMatches {
					info, err := os.Stat(m)
					if err != nil {
						continue
					}
					if info.IsDir() {
						continue
					}
					// Apply exclude filter.
					if params.Exclude != "" {
						excluded, _ := filepath.Match(params.Exclude, info.Name())
						if excluded {
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
			}
			return result, nil
		},
	)
}

// matchDoublestar implements simple ** glob matching.
func matchDoublestar(pattern, path string) bool {
	// Normalize separators.
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
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

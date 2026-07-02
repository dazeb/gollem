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
						// Never prune the walk root — the caller explicitly
						// asked to search this directory, even if its basename
						// is skippable (e.g. path="vendor").
						if path != searchPath && isSkippableDir(d.Name()) {
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
				// Simple glob without **. filepath.Glob would interpret
				// glob metacharacters in searchPath itself (e.g. Next.js
				// "[slug]" route directories), so walk the tree and match
				// the pattern against the path relative to searchPath
				// instead. matchDoublestar requires an exact segment count
				// for patterns without **, preserving non-recursive glob
				// semantics. Expand braces up front to validate the pattern
				// and bound the walk depth.
				maxDepth := 1
				for _, ep := range expandBraces(filepath.ToSlash(params.Pattern)) {
					for _, seg := range strings.Split(ep, "/") {
						if _, err := filepath.Match(seg, ""); err != nil {
							return "", &core.ModelRetryError{Message: fmt.Sprintf("invalid glob pattern: %v", err)}
						}
					}
					if d := strings.Count(ep, "/") + 1; d > maxDepth {
						maxDepth = d
					}
				}
				err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if d.IsDir() {
						if path == searchPath {
							return nil
						}
						// Don't descend deeper than the pattern can match.
						relDir, _ := filepath.Rel(searchPath, path)
						if strings.Count(filepath.ToSlash(relDir), "/")+1 >= maxDepth {
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
						if info, err := d.Info(); err == nil {
							results = append(results, fileEntry{relPath, info.ModTime().Unix(), info.Size()})
						}
					}
					return nil
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					return "", fmt.Errorf("walking directory: %w", err)
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
		core.WithToolConcurrencySafe(true),
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

// maxBraceExpansions bounds the cartesian product of sequential brace
// groups so a pathological pattern cannot explode memory.
const maxBraceExpansions = 64

// expandBraces expands brace groups in a pattern.
// "*.{go,py}" → ["*.go", "*.py"]
// "src/{a,b}/*.ts" → ["src/a/*.ts", "src/b/*.ts"]
// "{a,b}/*.{go,py}" → ["a/*.go", "a/*.py", "b/*.go", "b/*.py"]
// Patterns without braces return a single-element slice.
// Sequential groups expand as a cartesian product (capped at
// maxBraceExpansions); nested braces are not supported.
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
	// Recurse on the suffix so later groups expand too.
	suffixes := expandBraces(pattern[close+1:])
	alternatives := strings.Split(pattern[open+1:close], ",")
	if len(alternatives)*len(suffixes) > maxBraceExpansions {
		return []string{pattern} // pathological — leave braces literal
	}
	var result []string
	for _, alt := range alternatives {
		for _, suffix := range suffixes {
			result = append(result, prefix+strings.TrimSpace(alt)+suffix)
		}
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

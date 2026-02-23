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

	// IgnoreCase makes the search case-insensitive.
	IgnoreCase bool `json:"ignore_case,omitempty" jsonschema:"description=If true\\, search case-insensitively. Default: false"`

	// MaxResults limits the number of matching lines returned.
	MaxResults *int `json:"max_results,omitempty" jsonschema:"description=Maximum number of matching lines to return. Default: 100"`

	// ContextLines is the number of lines to show before and after each match.
	ContextLines *int `json:"context_lines,omitempty" jsonschema:"description=Number of context lines before and after each match. Default: 0"`

	// Exclude is a glob pattern to skip files (e.g. '*_test.go', '*.min.js').
	Exclude string `json:"exclude,omitempty" jsonschema:"description=Glob pattern to exclude files (e.g. '*_test.go'\\, '*.min.js'). Applied to filename only."`

	// FilesOnly returns only file paths instead of matching lines.
	FilesOnly bool `json:"files_only,omitempty" jsonschema:"description=If true\\, return only file paths that contain matches (not the matching lines). Useful for surveying which files match."`
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
			"Use include to filter by file extension (e.g. '*.go'), "+
			"exclude to skip files (e.g. '*_test.go'), "+
			"ignore_case for case-insensitive search, "+
			"files_only to get just file paths without line content. "+
			"Use this to find function definitions, usages, imports, error messages, etc.",
		func(ctx context.Context, params GrepParams) (string, error) {
			if params.Pattern == "" {
				return "", &core.ModelRetryError{Message: "pattern must not be empty"}
			}

			pattern := params.Pattern
			if params.IgnoreCase {
				// Prepend (?i) for case-insensitive matching unless the pattern
				// already contains case flags.
				if !strings.HasPrefix(pattern, "(?") || !strings.ContainsRune(pattern[2:], 'i') {
					pattern = "(?i)" + pattern
				}
			}

			re, err := regexp.Compile(pattern)
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
			matchCount := 0
			fileCount := 0
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
					if isSkippableDir(base) {
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

				// Apply exclude filter.
				if params.Exclude != "" {
					excluded, _ := filepath.Match(params.Exclude, info.Name())
					if excluded {
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

				if params.FilesOnly {
					return searchFileExists(ctx, path, relPath, re, maxResults, &matches, &matchCount, &fileCount, &truncated)
				}
				return searchFile(ctx, path, relPath, re, contextLines, maxResults, &matches, &matchCount, &fileCount, &truncated)
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
			if params.FilesOnly {
				result += fmt.Sprintf("\n(%d files matched)", matchCount)
			} else {
				result += fmt.Sprintf("\n(%d matches in %d files)", matchCount, fileCount)
			}
			return result, nil
		},
	)
}

func searchFile(ctx context.Context, absPath, relPath string, re *regexp.Regexp, contextLines, maxResults int, matches *[]string, matchCount *int, fileCount *int, truncated *bool) error {
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Quick binary check: if the first 512 bytes contain null bytes, skip.
	probe := make([]byte, 512)
	n, _ := f.Read(probe)
	for _, b := range probe[:n] {
		if b == 0 {
			return nil // binary file, skip
		}
	}
	// Seek back to start after probe.
	if _, err := f.Seek(0, 0); err != nil {
		return nil
	}

	var allLines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	// Track the last context line shown to avoid duplicating lines when
	// consecutive matches have overlapping context windows.
	lastContextEnd := -1
	fileCounted := false

	for i, line := range allLines {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Count actual regex matches, not context/separator lines.
		if *matchCount >= maxResults {
			*truncated = true
			return filepath.SkipAll
		}
		if re.MatchString(line) {
			*matchCount++
			if !fileCounted {
				*fileCount++
				fileCounted = true
			}
			if contextLines > 0 {
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				end := i + contextLines + 1
				if end > len(allLines) {
					end = len(allLines)
				}
				// Skip lines already shown by previous match's context.
				if start <= lastContextEnd {
					// Add separator only if there's a gap.
					start = lastContextEnd + 1
				} else if lastContextEnd >= 0 {
					// Non-contiguous: add separator between blocks.
					*matches = append(*matches, "---")
				}
				for j := start; j < end; j++ {
					prefix := " "
					if j == i {
						prefix = ">"
					}
					lineText := allLines[j]
					if len(lineText) > 2000 {
						lineText = lineText[:2000] + "..."
					}
					*matches = append(*matches, fmt.Sprintf("%s%s:%d: %s", prefix, relPath, j+1, lineText))
				}
				if end-1 > lastContextEnd {
					lastContextEnd = end - 1
				}
			} else {
				lineText := line
				if len(lineText) > 2000 {
					lineText = lineText[:2000] + "..."
				}
				*matches = append(*matches, fmt.Sprintf("%s:%d: %s", relPath, i+1, lineText))
			}
		}
	}
	// Add trailing separator after context blocks so matches from different
	// files are visually separated. Without this, context from two files
	// would run together with no delimiter.
	if lastContextEnd >= 0 {
		*matches = append(*matches, "---")
	}
	return nil
}

// searchFileExists checks if a file contains any match and records just the
// file path (for files_only mode). This is faster than searchFile since it
// stops at the first match and doesn't track line numbers or context.
func searchFileExists(ctx context.Context, absPath, relPath string, re *regexp.Regexp, maxResults int, matches *[]string, matchCount *int, fileCount *int, truncated *bool) error {
	if *matchCount >= maxResults {
		*truncated = true
		return filepath.SkipAll
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Quick binary check.
	probe := make([]byte, 512)
	n, _ := f.Read(probe)
	for _, b := range probe[:n] {
		if b == 0 {
			return nil
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if re.MatchString(scanner.Text()) {
			*matchCount++
			*fileCount++
			*matches = append(*matches, relPath)
			return nil // found — no need to scan further
		}
	}
	return nil
}

// isSkippableDir returns true for directories that should be skipped during
// recursive file search (grep, glob). These are build artifacts, dependency
// caches, and VCS directories that waste context tokens and search time.
func isSkippableDir(name string) bool {
	switch name {
	case ".git", ".svn", ".hg", // version control
		"node_modules", "__pycache__", ".tox", "vendor",
		// Build output directories.
		"build",     // Gradle, generic
		"_build",    // OCaml (dune), Elixir (mix)
		".build",    // Swift (swift build)
		"dist",      // JS bundlers, Python sdist/wheel
		"target",    // Rust (cargo), Maven/Gradle (Java)
		"out",       // Android, TypeScript outDir, generic
		"zig-cache", // Zig
		"zig-out",   // Zig build output
		"nimcache",  // Nim compilation cache
		".gradle",   // Gradle wrapper cache
		".dub",      // D language package cache
		"deps",      // Elixir dependencies
		"_deps",     // CMake FetchContent
		".eggs",     // Python eggs
		// Python virtual environments.
		".venv", "venv",
		// Caches.
		".cache", ".pytest_cache", ".mypy_cache", ".ruff_cache",
		".next",     // Next.js build cache
		".nuxt",     // Nuxt.js build cache
		".turbo",    // Turborepo cache
		"coverage",  // test coverage reports
		".coverage", // Python coverage
		".DS_Store", // macOS metadata (file, not dir, but harmless to check)
		// IDE and editor directories.
		".idea",        // JetBrains (IntelliJ, PyCharm, GoLand, etc.)
		".vscode",      // VS Code settings
		".vs",          // Visual Studio
		".eclipse",     // Eclipse metadata
		".settings",    // Eclipse settings
		"__snapshots__", // Jest test snapshots
		// Language-specific build/cache directories.
		".stack-work", // Haskell Stack build artifacts
		"_opam",       // OCaml opam local switch
		".dart_tool",  // Dart tool cache
		".packages",   // Dart package links
		"elm-stuff",   // Elm build artifacts
		".elixir_ls",  // Elixir language server cache
		".cargo",      // Rust cargo registry (when local)
		".cabal-sandbox", // Haskell Cabal sandbox
		"_esy",        // OCaml esy package cache
		".lake",       // Lean 4 Lake build cache
		".eunit",      // Erlang EUnit output
		".angular",    // Angular cache
		".parcel-cache", // Parcel bundler cache
		".svelte-kit",   // SvelteKit build
		"_rel",        // Erlang release directory
		// Infrastructure/deployment directories.
		".terraform",  // Terraform providers and state (can be huge)
		".serverless", // Serverless Framework
		".pulumi",     // Pulumi state
		".yarn",       // Yarn v2+ PnP cache
		".pnp",        // Yarn PnP runtime
		".expo",       // React Native Expo cache
		// Bazel build outputs.
		"bazel-bin", "bazel-out", "bazel-testlogs", "bazel-genfiles":
		return true
	}
	return false
}

func isBinaryFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".svg", ".webp", ".tiff", ".tif",
		".ppm", ".pgm", ".icns", // uncommon image formats
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".zst",
		".exe", ".dll", ".so", ".dylib", ".o", ".a", ".lib", ".obj", ".wasm",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".pyc", ".pyo", ".class", ".jar", ".war", ".whl", ".egg", // compiled/packaged
		".mp3", ".mp4", ".avi", ".mov", ".mkv", ".wav", ".flac", ".ogg",
		".aac", ".m4a", ".aiff", ".wma",                 // additional audio
		".wmv", ".flv", ".webm",                         // additional video
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		".sqlite", ".sqlite3", ".db", ".db3",
		".qcow2", ".img", ".iso", ".vmdk", ".vdi",      // disk images
		".bin", ".dat", ".raw", ".pak",                  // generic binary
		".npy", ".npz", ".pkl", ".pickle", ".pt", ".pth", // ML data/models
		".onnx", ".safetensors", ".tflite", ".gguf",    // ML model formats
		".pb",                                           // protobuf binary
		".h5", ".hdf5", ".parquet", ".feather",          // data formats
		".arrow", ".avro", ".orc",                       // columnar data formats
		".cab", ".deb", ".rpm", ".snap", ".flatpak",     // packages
		".DS_Store", ".lock":                            // system/lock files
		return true
	}
	return false
}

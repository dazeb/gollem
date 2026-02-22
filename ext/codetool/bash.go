package codetool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// truncateOutput shortens long output by keeping the head and tail, connected
// by a truncation notice. This preserves error summaries that appear at the
// end of test or build output.
//
// The split ratio adapts based on content: if the output contains error/failure
// indicators, we keep more of the tail (where error details and summaries live).
func truncateOutput(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}

	// Check if the output looks like it contains errors/test results.
	// If so, keep more of the tail where summaries and error details appear.
	headRatio := 70 // default: 70% head, 30% tail
	lower := strings.ToLower(s[max(0, len(s)-5000):])
	if strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "failure") ||
		strings.Contains(lower, "traceback") ||
		strings.Contains(lower, "panic:") ||
		strings.Contains(lower, "assertion") {
		headRatio = 30 // flip: 30% head, 70% tail for error output
	}

	headLen := maxLen * headRatio / 100
	tailLen := maxLen - headLen - 100 // reserve space for the separator
	if tailLen < 0 {
		tailLen = 0
	}
	return s[:headLen] +
		fmt.Sprintf("\n\n... [truncated %d bytes, showing first %d and last %d bytes] ...\n\n", len(s), headLen, tailLen) +
		s[len(s)-tailLen:]
}

// BashParams are the parameters for the bash tool.
type BashParams struct {
	// Command is the shell command to execute.
	Command string `json:"command" jsonschema:"description=The bash command to execute"`

	// Timeout is an optional timeout in seconds. Overrides the default.
	Timeout *int `json:"timeout,omitempty" jsonschema:"description=Optional timeout in seconds (default: 120)"`
}

// BashResult is the result of a bash command execution (used in tests).
type BashResult struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// Bash creates a tool that executes shell commands.
// Returns formatted text (not JSON) for efficient token usage and easier model parsing.
func Bash(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[BashParams](
		"bash",
		"Execute a bash command in the shell. Use this for running programs, installing packages, "+
			"compiling code, running tests, git operations, and any other terminal commands. "+
			"Commands run in a persistent working directory. "+
			"Prefer this tool for exploring the filesystem and running build/test commands.",
		func(ctx context.Context, params BashParams) (string, error) {
			if strings.TrimSpace(params.Command) == "" {
				return "", &core.ModelRetryError{Message: "command must not be empty"}
			}

			timeout := cfg.BashTimeout
			if params.Timeout != nil && *params.Timeout > 0 {
				timeout = time.Duration(*params.Timeout) * time.Second
			} else if isBuildCommand(params.Command) && timeout < 5*time.Minute {
				// Auto-extend timeout for build/compile commands which often
				// need much longer than the 120s default.
				timeout = 5 * time.Minute
			} else if isLongRunningCommand(params.Command) && timeout < 5*time.Minute {
				// Auto-extend timeout for benchmarks, model training, and data
				// processing commands that typically exceed the 120s default.
				timeout = 5 * time.Minute
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
			if cfg.WorkDir != "" {
				cmd.Dir = cfg.WorkDir
			}

			// Run in a new process group so we can kill all children on timeout.
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error {
				// Kill the entire process group (negative PID).
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			exitCode := 0
			timedOut := false
			if err != nil {
				// Check timeout first — on some platforms a killed process
				// returns ExitError with code -1, so we must check context
				// before inspecting the exit code.
				if ctx.Err() == context.DeadlineExceeded {
					timedOut = true
					exitCode = 124
				} else {
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						exitCode = exitErr.ExitCode()
					} else {
						return "", fmt.Errorf("failed to execute command: %w", err)
					}
				}
			}

			outStr := stdout.String()
			errStr := stderr.String()
			rawLen := len(outStr) + len(errStr)

			// Truncate long output, keeping head and tail so the model can
			// see error summaries at the end.
			outStr = truncateOutput(outStr, cfg.MaxOutputLen)
			errStr = truncateOutput(errStr, cfg.MaxOutputLen)

			result := formatBashOutput(outStr, errStr, exitCode, timedOut, timeout)

			// Hint when output was heavily truncated — suggest file redirect.
			if rawLen > cfg.MaxOutputLen*2 {
				result += fmt.Sprintf("\n[hint: output was %d bytes (heavily truncated). For large output, redirect to a file: cmd > /tmp/out.txt 2>&1, then use view or grep to find what you need]", rawLen)
			}

			// Add hints for common errors — saves turns of troubleshooting.
			if exitCode == 127 || strings.Contains(errStr, "command not found") || strings.Contains(errStr, "No such file or directory") {
				if hint := commandNotFoundHint(errStr); hint != "" {
					result += "\n" + hint
				}
			}
			if strings.Contains(errStr, "ModuleNotFoundError") || strings.Contains(errStr, "ImportError") || strings.Contains(outStr, "ModuleNotFoundError") {
				if hint := moduleNotFoundHint(errStr + outStr); hint != "" {
					result += "\n" + hint
				}
			}
			if hint := transientErrorHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := signalHint(exitCode); hint != "" {
				result += "\n" + hint
			}

			// Append summaries for long output to help the model focus.
			combined := outStr + errStr
			if len(combined) > 2000 {
				if summary := testResultSummary(combined); summary != "" {
					result += "\n" + summary
				} else if summary := compilationErrorSummary(combined, exitCode); summary != "" {
					result += "\n" + summary
				}
			}

			return result, nil
		},
		core.WithToolSequential(true), // bash commands should run sequentially
	)
}

// formatBashOutput combines stdout, stderr, and exit code into a clean text
// format that's efficient on tokens and easy for models to parse.
func formatBashOutput(stdout, stderr string, exitCode int, timedOut bool, timeout time.Duration) string {
	var b strings.Builder
	hasContent := stdout != "" || stderr != ""

	if stdout != "" {
		b.WriteString(stdout)
	}
	if stderr != "" {
		if b.Len() > 0 {
			b.WriteString("\n[stderr]\n")
		}
		b.WriteString(stderr)
	}

	if timedOut {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("[timed out after %s — if this is a test or benchmark, optimize YOUR code to be faster. Do NOT modify test/benchmark parameters. Use the timeout parameter for legitimately long-running commands.]", timeout))
	} else if exitCode != 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("[exit code: %d]", exitCode))
		if !hasContent {
			b.WriteString("\n(no output)")
		}
	}

	if b.Len() == 0 {
		return "(no output)"
	}

	return b.String()
}

// commandNotFoundHint generates an installation hint when a command is missing.
// This saves a turn of the model figuring out what package to install.
func commandNotFoundHint(stderr string) string {
	// Common command → package mappings for Debian/Ubuntu containers.
	packages := map[string]string{
		"python3":    "python3",
		"python":     "python3",
		"pip3":       "python3-pip",
		"pip":        "python3-pip",
		"node":       "nodejs",
		"npm":        "npm",
		"gcc":        "build-essential",
		"g++":        "build-essential",
		"cc":         "build-essential",
		"make":       "build-essential",
		"cmake":      "cmake",
		"curl":       "curl",
		"wget":       "wget",
		"git":        "git",
		"java":       "default-jdk",
		"javac":      "default-jdk",
		"perl":       "perl",
		"ruby":       "ruby",
		"gfortran":   "gfortran",
		"ffmpeg":     "ffmpeg",
		"jq":         "jq",
		"unzip":      "unzip",
		"zip":        "zip",
		"bc":         "bc",
		"flex":       "flex",
		"bison":      "bison",
		"pkg-config": "pkg-config",
		"autoconf":   "autoconf",
		"automake":   "automake",
		"libtool":    "libtool",
		"rsync":      "rsync",
		"sqlite3":    "sqlite3",
		"lsof":       "lsof",
		"netcat":     "netcat-openbsd",
		"nc":         "netcat-openbsd",
		"socat":      "socat",
		"grpc":       "protobuf-compiler",
		"protoc":     "protobuf-compiler",
		"valgrind":   "valgrind",
		"gdb":        "gdb",
		"strace":     "strace",
		"ltrace":     "ltrace",
		"nasm":       "nasm",
		"yasm":       "yasm",
		"rustc":      "rustc",
		"cargo":      "cargo",
		"ghc":        "ghc",
		"ocaml":      "ocaml",
		"racket":     "racket",
		"guile":      "guile-3.0",
		"sbcl":       "sbcl",
		"gawk":       "gawk",
		"m4":         "m4",
		"patch":      "patch",
		"diffutils":  "diffutils",
		"file":       "file",
		"xxd":        "xxd",
		"hexdump":    "bsdmainutils",
		"strings":    "binutils",
		"objdump":    "binutils",
		"readelf":    "binutils",
		"ar":         "binutils",
		"nm":         "binutils",
		"strip":      "binutils",
		"as":         "binutils",
		"ld":         "binutils",
		"expect":     "expect",
		"sshd":       "openssh-server",
		"ssh":        "openssh-client",
		"sshpass":    "sshpass",
		"postfix":    "postfix",
		"sendmail":   "sendmail",
		"tesseract":  "tesseract-ocr",
		"gnuplot":    "gnuplot-nox",
		"dot":        "graphviz",
		"neato":      "graphviz",
		"latex":      "texlive-latex-base",
		"pdflatex":   "texlive-latex-base",
		"xelatex":    "texlive-xetex",
		"convert":    "imagemagick",
		"identify":   "imagemagick",
		"sox":        "sox",
		"mplayer":    "mplayer",
		"qemu-system-x86_64":   "qemu-system-x86",
		"qemu-system-mips":     "qemu-system-mips",
		"qemu-system-arm":      "qemu-system-arm",
		"qemu-img":             "qemu-utils",
		"cobc":       "gnucobol",
		"gnat":       "gnat",
		"fpc":        "fp-compiler",
		"swipl":      "swi-prolog",
		"mono":       "mono-runtime",
		"mcs":        "mono-mcs",
		"R":          "r-base",
		"Rscript":    "r-base",
		"scala":      "scala",
		"lua":        "lua5.4",
		"luarocks":   "luarocks",
		"tcl":        "tcl",
		"wish":       "tk",
		"tclsh":      "tcl",
	}

	// Extract the missing command name from stderr.
	lower := strings.ToLower(stderr)
	for cmd, pkg := range packages {
		if strings.Contains(lower, cmd+": command not found") ||
			strings.Contains(lower, cmd+": not found") {
			return fmt.Sprintf("[hint: try: apt-get install -y %s]", pkg)
		}
	}

	// Fallback for pip/ensurepip.
	if strings.Contains(lower, "no module named pip") {
		return "[hint: try: python3 -m ensurepip || apt-get install -y python3-pip]"
	}

	return ""
}

// moduleNotFoundHint generates a pip install hint when a Python import fails.
// This saves a turn of the model figuring out which package to install.
func moduleNotFoundHint(output string) string {
	// Common module → pip package mappings where they differ.
	aliases := map[string]string{
		"cv2":         "opencv-python",
		"PIL":         "Pillow",
		"sklearn":     "scikit-learn",
		"skimage":     "scikit-image",
		"yaml":        "PyYAML",
		"bs4":         "beautifulsoup4",
		"attr":        "attrs",
		"dotenv":      "python-dotenv",
		"git":         "GitPython",
		"serial":      "pyserial",
		"usb":         "pyusb",
		"magic":       "python-magic",
		"Crypto":      "pycryptodome",
		"dateutil":    "python-dateutil",
		"jwt":         "PyJWT",
		"lxml":        "lxml",
		"wx":          "wxPython",
		"gi":          "PyGObject",
		"nacl":        "PyNaCl",
		"socks":       "PySocks",
		"zmq":         "pyzmq",
		"Levenshtein": "python-Levenshtein",
		"Bio":         "biopython",
		"torch":       "torch",
		"torchvision": "torchvision",
		"tensorflow":  "tensorflow",
		"tf":          "tensorflow",
		"scipy":       "scipy",
		"pandas":      "pandas",
		"matplotlib":  "matplotlib",
		"seaborn":     "seaborn",
		"flask":       "flask",
		"django":      "django",
		"fastapi":     "fastapi",
		"uvicorn":     "uvicorn",
		"gunicorn":    "gunicorn",
		"grpc":        "grpcio grpcio-tools",
		"google.protobuf": "protobuf",
		"pydantic":    "pydantic",
		"httpx":       "httpx",
		"aiohttp":     "aiohttp",
		"sqlalchemy":  "sqlalchemy",
		"alembic":     "alembic",
		"celery":      "celery",
		"redis":       "redis",
		"pymongo":     "pymongo",
		"psycopg2":    "psycopg2-binary",
		"MySQLdb":     "mysqlclient",
		"mysql":       "mysqlclient",
		"toml":        "toml",
		"tomli":       "tomli",
		"tomllib":     "tomli",
		"msgpack":     "msgpack",
		"protobuf":    "protobuf",
		"Cython":      "cython",
		"sympy":       "sympy",
		"networkx":    "networkx",
		"pyarrow":     "pyarrow",
		"h5py":        "h5py",
		"transformers": "transformers",
		"datasets":    "datasets",
		"tokenizers":  "tokenizers",
		"tqdm":        "tqdm",
		"click":       "click",
		"rich":        "rich",
		"paramiko":    "paramiko",
		"fabric":      "fabric",
		"pexpect":     "pexpect",
		"ply":         "ply",
		"lark":        "lark",
		"pyparsing":   "pyparsing",
		"construct":   "construct",
		"bitstring":   "bitstring",
		"elftools":    "pyelftools",
	}

	// Try to extract the module name from common error patterns.
	// Patterns: "No module named 'foo'" or "No module named 'foo.bar'"
	for _, pattern := range []string{"No module named '", "No module named \""} {
		idx := strings.Index(output, pattern)
		if idx < 0 {
			continue
		}
		start := idx + len(pattern)
		rest := output[start:]
		end := strings.IndexAny(rest, "'\"")
		if end < 0 {
			continue
		}
		module := rest[:end]
		// Use the top-level package name (e.g., "foo" from "foo.bar.baz").
		if dot := strings.Index(module, "."); dot > 0 {
			module = module[:dot]
		}
		if module == "" {
			continue
		}
		pkg := module
		if alias, ok := aliases[module]; ok {
			pkg = alias
		}
		return fmt.Sprintf("[hint: try: pip install --break-system-packages %s]", pkg)
	}

	return ""
}

// transientErrorHint detects common transient errors and suggests fixes.
// These are errors that waste turns if the model has to diagnose them itself.
func transientErrorHint(output string, exitCode int) string {
	lower := strings.ToLower(output)

	// pip: externally-managed-environment error.
	// Extremely common in Docker containers — wastes 1-2 turns without a hint.
	if strings.Contains(lower, "externally-managed-environment") ||
		strings.Contains(output, "externally managed") {
		return "[hint: add --break-system-packages flag to pip install]"
	}

	// apt/dpkg lock errors — another process is running dpkg.
	if strings.Contains(lower, "could not get lock") ||
		strings.Contains(lower, "dpkg was interrupted") ||
		strings.Contains(lower, "dpkg --configure -a") {
		return "[hint: try: dpkg --configure -a && apt-get install -f]"
	}

	// Network/download errors during package installs.
	if exitCode != 0 &&
		(strings.Contains(lower, "temporary failure resolving") ||
			strings.Contains(lower, "could not resolve") ||
			strings.Contains(lower, "connection timed out") ||
			strings.Contains(lower, "connection refused") && strings.Contains(lower, "apt") ||
			strings.Contains(lower, "failed to fetch") ||
			strings.Contains(lower, "retrying") && strings.Contains(lower, "download")) {
		return "[hint: transient network error — retry the command]"
	}

	// Permission errors in common locations.
	if strings.Contains(lower, "permission denied") && exitCode != 0 {
		if strings.Contains(lower, "/usr/") || strings.Contains(lower, "/etc/") ||
			strings.Contains(lower, "/var/") {
			return "[hint: try running with sudo or use --user flag for pip]"
		}
	}

	return ""
}

// signalHint detects when a process was killed by a signal and provides guidance.
// Exit code 137 = SIGKILL (often OOM), 139 = SIGSEGV, 134 = SIGABRT.
func signalHint(exitCode int) string {
	switch exitCode {
	case 137:
		return "[hint: process was killed (SIGKILL) — likely out of memory. " +
			"Try: reduce batch size, process data in chunks, use less memory-intensive approach, " +
			"or add swap with: dd if=/dev/zero of=/swapfile bs=1M count=1024 && mkswap /swapfile && swapon /swapfile]"
	case 139:
		return "[hint: segmentation fault (SIGSEGV) — likely a memory access bug. " +
			"Check: array bounds, null pointers, use-after-free, stack overflow from deep recursion]"
	case 134:
		return "[hint: process aborted (SIGABRT) — likely an assertion failure or double-free. " +
			"Check: assert() failures, memory corruption, C++ exception in destructor]"
	}
	return ""
}

// testResultSummary extracts a concise summary from test runner output.
// Returns empty string if the output doesn't look like test results.
// This helps the model quickly understand what passed/failed without
// parsing the entire output itself — especially valuable after truncation.
func testResultSummary(output string) string {
	lower := strings.ToLower(output)

	// pytest: "X passed", "X failed", "X error"
	if strings.Contains(lower, "passed") && (strings.Contains(lower, "failed") || strings.Contains(lower, "error")) ||
		strings.Contains(lower, "===") && strings.Contains(lower, "passed") {
		// Find the pytest summary line (usually the last line with "passed" and/or "failed")
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-10); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if (strings.Contains(lineLower, "passed") || strings.Contains(lineLower, "failed")) &&
				(strings.Contains(line, "=") || strings.Contains(lineLower, "error")) {
				return "[test summary: " + line + "]"
			}
		}
	}

	// Go test: look for "--- FAIL:" and "ok" lines
	if strings.Contains(output, "--- FAIL:") || strings.Contains(output, "FAIL\t") {
		var fails []string
		var passes []string
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--- FAIL: ") {
				// Extract test name
				name := strings.TrimPrefix(trimmed, "--- FAIL: ")
				if paren := strings.Index(name, " ("); paren > 0 {
					name = name[:paren]
				}
				fails = append(fails, name)
			} else if strings.HasPrefix(trimmed, "ok \t") || strings.HasPrefix(trimmed, "ok  \t") {
				passes = append(passes, trimmed)
			}
		}
		if len(fails) > 0 {
			summary := fmt.Sprintf("[test summary: %d test(s) FAILED", len(fails))
			if len(passes) > 0 {
				summary += fmt.Sprintf(", %d package(s) passed", len(passes))
			}
			// Show failing test names (up to 10)
			shown := fails
			if len(shown) > 10 {
				shown = shown[:10]
			}
			summary += ": " + strings.Join(shown, ", ")
			if len(fails) > 10 {
				summary += fmt.Sprintf("... and %d more", len(fails)-10)
			}
			summary += "]"
			return summary
		}
	}

	// npm/jest: "Tests: X failed, Y passed, Z total"
	if strings.Contains(lower, "tests:") && strings.Contains(lower, "total") {
		for _, line := range strings.Split(output, "\n") {
			lineLower := strings.ToLower(strings.TrimSpace(line))
			if strings.Contains(lineLower, "tests:") && strings.Contains(lineLower, "total") {
				return "[test summary: " + strings.TrimSpace(line) + "]"
			}
		}
	}

	// Generic: count PASS/FAIL lines
	if strings.Contains(upper(output), "FAIL") || strings.Contains(upper(output), "PASS") {
		passCount := strings.Count(upper(output), "\nPASS")
		failCount := strings.Count(upper(output), "\nFAIL")
		if passCount+failCount >= 3 {
			return fmt.Sprintf("[test summary: %d PASS, %d FAIL out of %d tests]",
				passCount, failCount, passCount+failCount)
		}
	}

	return ""
}

func upper(s string) string {
	return strings.ToUpper(s)
}

// compilationErrorSummary extracts key error lines from compiler output.
// Returns empty string if no compilation errors are detected.
func compilationErrorSummary(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	// Only process if it looks like compiler output
	hasCompilerError := false
	errorPatterns := []string{
		": error:", ": error[", "error:", "Error:",
		": fatal error:", "undefined reference",
		"SyntaxError:", "IndentationError:", "TypeError:",
		"cannot find symbol", "not found in scope",
	}
	for _, p := range errorPatterns {
		if strings.Contains(output, p) {
			hasCompilerError = true
			break
		}
	}
	if !hasCompilerError {
		return ""
	}

	// Extract error lines (lines containing ": error" or similar patterns)
	var errorLines []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 5 || len(trimmed) > 200 {
			continue
		}
		isError := false
		for _, p := range errorPatterns {
			if strings.Contains(trimmed, p) {
				isError = true
				break
			}
		}
		if isError && !seen[trimmed] {
			seen[trimmed] = true
			errorLines = append(errorLines, trimmed)
		}
	}

	if len(errorLines) == 0 {
		return ""
	}

	// Show up to 8 error lines
	shown := errorLines
	if len(shown) > 8 {
		shown = shown[:8]
	}

	summary := fmt.Sprintf("[compilation: %d error(s) found", len(errorLines))
	if len(errorLines) > 8 {
		summary += fmt.Sprintf(" (showing first 8)")
	}
	summary += ":\n"
	for _, line := range shown {
		summary += "  " + line + "\n"
	}
	summary += "]"
	return summary
}

// isBuildCommand detects commands that typically need longer timeouts.
// isLongRunningCommand returns true for commands that typically need more than
// the default 120s timeout: benchmarks, model training, data processing, etc.
func isLongRunningCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	longPatterns := []string{
		"benchmark", "bench.",
		"python3 train", "python train",
		"python3 benchmark", "python benchmark",
		"pytest -", // test suites with options often take longer
		"pytest /",
		"go test -bench", "go test -count", "go test -run", "go test ./...",
		"train.py", "training.py",
		"fasttext ", "qemu-system",
		"java -jar", "java -cp",     // JVM programs
		"mvn ", "gradle ",           // Build tools
		"npm run ", "npx ",          // npm scripts
		"yarn ", "pnpm run ",        // Package manager scripts
		"python3 -m ", "python -m ", // Module execution (e.g., python -m pytest)
		"docker run",                // Container execution
		"timeout ",                  // Already has own timeout, don't cut short
		"lake build",                // Lean 4 proof checking
		"dune build", "dune test",   // OCaml builds
		"stack build", "cabal build", // Haskell builds
		"cargo test",                // Rust tests
	}
	for _, p := range longPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isBuildCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	buildPatterns := []string{
		"make", "cmake", "cargo build", "cargo install",
		"go build", "go install",
		"gcc ", "g++ ", "clang ", "cc ",
		"javac ", "mvn ", "gradle ",
		"npm install", "npm ci", "yarn install", "pnpm install",
		"pip install", "pip3 install", "python3 -m pip install", "python -m pip install",
		"apt-get install", "apt install", "apk add", "yum install", "dnf install",
		"docker build",
		"lake build",   // Lean 4
		"dune build",   // OCaml
		"stack build",  // Haskell
		"cabal build",  // Haskell
		"zig build",    // Zig
		"mix compile",  // Elixir
		"./configure",
		"rustup", "cargo install",
		"gem install", "bundle install",
		"composer install", // PHP
		"dotnet restore",  // .NET
	}
	for _, p := range buildPatterns {
		if strings.HasPrefix(lower, p) || strings.Contains(lower, " && "+p) || strings.Contains(lower, "; "+p) {
			return true
		}
	}
	return false
}

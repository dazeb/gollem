package codetool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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

	// Track test failure fingerprints for stale-failure detection.
	// When the same test failure appears twice in a row, the agent's fix
	// was ineffective — warn it to try a different approach.
	// Safe because bash is WithToolSequential (no concurrent calls).
	var lastTestFailFingerprint string

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

			// Block bash commands that destructively modify verifier test files.
			// The edit/write tools already block /tests/ modifications, but
			// agents can bypass that via bash redirects, rm, sed -i, etc.
			if isDestructiveTestCommand(params.Command) {
				return "", &core.ModelRetryError{
					Message: "BLOCKED: This command would modify files in /tests/ (verifier test directory). " +
						"The verifier runs the ORIGINAL tests — your changes will be ignored. " +
						"Fix YOUR code to pass the tests instead.",
				}
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

			// Auto-set PIP_BREAK_SYSTEM_PACKAGES=1 for pip commands to prevent
			// externally-managed-environment errors in Docker containers.
			// This saves a full turn of error → hint → retry.
			if isPipCommand(params.Command) {
				cmd.Env = append(os.Environ(), "PIP_BREAK_SYSTEM_PACKAGES=1")
			}

			// Run in a new process group so we can kill all children on timeout.
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error {
				// Kill the entire process group (negative PID).
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}

			// Execute the command with one auto-retry for transient failures
			// (network errors, dpkg locks, etc.). This saves a full agent turn.
			var outStr, errStr string
			exitCode := 0
			timedOut := false
			retried := false

			for attempt := 0; attempt < 2; attempt++ {
				var stdout, stderr bytes.Buffer
				if attempt == 0 {
					cmd.Stdout = &stdout
					cmd.Stderr = &stderr
				} else {
					// Re-create cmd for retry (exec.Cmd can only be started once).
					retried = true
					cmd = exec.CommandContext(ctx, "bash", "-c", params.Command)
					if cfg.WorkDir != "" {
						cmd.Dir = cfg.WorkDir
					}
					if isPipCommand(params.Command) {
						cmd.Env = append(os.Environ(), "PIP_BREAK_SYSTEM_PACKAGES=1")
					}
					cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
					cmd.Cancel = func() error {
						return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					}
					cmd.Stdout = &stdout
					cmd.Stderr = &stderr
					fmt.Fprintf(os.Stderr, "[gollem] bash: auto-retrying transient failure\n")
					// Brief pause before retry.
					time.Sleep(2 * time.Second)
				}

				err := cmd.Run()

				exitCode = 0
				timedOut = false
				if err != nil {
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

				outStr = stdout.String()
				errStr = stderr.String()

				// Auto-retry on transient failures (first attempt only).
				if attempt == 0 && !timedOut && isTransientBashFailure(exitCode, outStr+errStr) {
					continue
				}
				break
			}
			rawLen := len(outStr) + len(errStr)

			// Truncate long output, keeping head and tail so the model can
			// see error summaries at the end.
			outStr = truncateOutput(outStr, cfg.MaxOutputLen)
			errStr = truncateOutput(errStr, cfg.MaxOutputLen)

			result := formatBashOutput(outStr, errStr, exitCode, timedOut, timeout)

			// Note when the command succeeded after auto-retry.
			if retried && exitCode == 0 {
				result += "\n[auto-retried after transient failure — succeeded on second attempt]"
			}

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
			if hint := pythonErrorHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := compilationErrorHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := jsonErrorHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := encodingErrorHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := permissionHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := addressInUseHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}
			if hint := nodeErrorHint(errStr + outStr, exitCode); hint != "" {
				result += "\n" + hint
			}

			// Append summaries for long output to help the model focus.
			combined := outStr + errStr
			if len(combined) > 2000 {
				if summary := testResultSummary(combined); summary != "" {
					result += "\n" + summary
					if exitCode != 0 && (strings.Contains(strings.ToLower(summary), "fail") || strings.Contains(strings.ToLower(summary), "error")) {
						result += "\n[hint: read the FULL test failure output above — fix one failure at a time, starting with the first]"
						// Stale failure detection: warn when the same test failure
						// appears consecutively, indicating the fix was ineffective.
						fp := testFailureFingerprint(combined)
						if fp != "" && fp == lastTestFailFingerprint {
							result += "\n[hint: this test failure is IDENTICAL to the previous run — your edit did not fix the issue. Re-read the error, verify your edit was applied correctly, and try a fundamentally different approach]"
						}
						lastTestFailFingerprint = fp
					} else {
						lastTestFailFingerprint = "" // reset on success
					}
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
		return "[hint: network error — this container may not have internet access. " +
			"Use only locally available packages and tools. " +
			"For Python: check if the package is already installed with 'python3 -c \"import <module>\"'. " +
			"For apt: try 'dpkg -l | grep <package>' to check installed packages]"
	}

	// Permission errors in common locations.
	if strings.Contains(lower, "permission denied") && exitCode != 0 {
		if strings.Contains(lower, "/usr/") || strings.Contains(lower, "/etc/") ||
			strings.Contains(lower, "/var/") {
			return "[hint: try running with sudo or use --user flag for pip]"
		}
	}

	// Disk space errors — clean up or use /tmp.
	if strings.Contains(lower, "no space left on device") {
		return "[hint: no disk space — clean up with: rm -rf /tmp/* __pycache__ *.pyc .cache build/ dist/ node_modules/.cache; or write output to /tmp/]"
	}

	// Read-only filesystem — common in some container setups.
	if strings.Contains(lower, "read-only file system") {
		return "[hint: read-only filesystem — try writing to /tmp/ or /app/ instead]"
	}

	// Segfault in Python (common with numpy/scipy compiled extensions).
	if strings.Contains(lower, "segmentation fault") && (strings.Contains(lower, "python") || strings.Contains(output, "python")) {
		return "[hint: Python segfault — likely a compiled extension issue. Try: pip install --break-system-packages --force-reinstall numpy scipy, or use pure Python alternatives]"
	}

	return ""
}

// signalHint detects when a process was killed by a signal and provides guidance.
// Exit code 137 = SIGKILL (often OOM), 139 = SIGSEGV, 134 = SIGABRT.
func signalHint(exitCode int) string {
	switch exitCode {
	case 124:
		// Exit code 124 is used by the `timeout` command when it kills a process.
		return "[hint: command was killed by timeout — your solution is too slow. " +
			"Optimize: use more efficient algorithms, avoid O(n²) when O(n log n) works, " +
			"use built-in/vectorized operations (numpy, etc.), reduce data copies, " +
			"or process data in streaming fashion instead of loading all into memory]"
	case 137:
		return "[hint: process was killed (SIGKILL) — likely out of memory. " +
			"Try: reduce batch size, process data in smaller chunks, use generators/iterators instead of loading all data into memory, " +
			"use more memory-efficient data structures, or reduce number of concurrent processes]"
	case 139:
		return "[hint: segmentation fault (SIGSEGV) — likely a memory access bug. " +
			"Check: array bounds, null pointers, use-after-free, stack overflow from deep recursion]"
	case 134:
		return "[hint: process aborted (SIGABRT) — likely an assertion failure or double-free. " +
			"Check: assert() failures, memory corruption, C++ exception in destructor]"
	}
	return ""
}

// pythonErrorHint extracts actionable information from Python errors.
// Extracts file:line from tracebacks for ALL error types (not just syntax
// errors) and provides targeted guidance based on the error category.
func pythonErrorHint(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	// Only process if it looks like a Python error.
	if !strings.Contains(output, "Error:") && !strings.Contains(output, "Error (") {
		return ""
	}

	lines := strings.Split(output, "\n")

	// Look for Python traceback patterns: "File "xxx", line N"
	// followed by the error line. Extract the LAST traceback frame
	// (innermost/most relevant).
	var lastFile, lastLine, errorType string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "File \"") {
			// Extract file and line number.
			if fileEnd := strings.Index(trimmed[6:], "\""); fileEnd > 0 {
				lastFile = trimmed[6 : 6+fileEnd]
			}
			if lineIdx := strings.Index(trimmed, "line "); lineIdx > 0 {
				rest := trimmed[lineIdx+5:]
				if commaIdx := strings.IndexAny(rest, ", \n"); commaIdx > 0 {
					lastLine = rest[:commaIdx]
				} else {
					lastLine = strings.TrimSpace(rest)
				}
			}
		}
		// Capture the error type line (the final error at the end of traceback).
		// Match common Python exception patterns.
		for _, errPrefix := range []string{
			"SyntaxError:", "IndentationError:", "TabError:",
			"TypeError:", "ValueError:", "KeyError:", "IndexError:",
			"FileNotFoundError:", "PermissionError:", "OSError:",
			"AttributeError:", "NameError:", "ImportError:",
			"ZeroDivisionError:", "RuntimeError:", "StopIteration:",
			"RecursionError:", "OverflowError:", "AssertionError:",
			"UnicodeDecodeError:", "UnicodeEncodeError:",
		} {
			if strings.Contains(trimmed, errPrefix) {
				errorType = trimmed
				break
			}
		}
	}

	if errorType != "" && lastFile != "" && lastLine != "" {
		// Skip files in standard library / site-packages — the error
		// is in user code, and the innermost user frame is more useful.
		// But if it's the only frame we found, still show it.
		hint := fmt.Sprintf("[hint: %s at %s:%s — use view tool with offset=%s to see the code, then fix with edit]",
			truncateErrorLine(errorType, 150), lastFile, lastLine, lastLine)
		return hint
	}

	// NameError/AttributeError with a suggestion (Python 3.10+).
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "NameError:") || strings.Contains(trimmed, "AttributeError:")) &&
			strings.Contains(trimmed, "Did you mean:") {
			return "[hint: " + truncateErrorLine(trimmed, 150) + "]"
		}
	}

	// FileNotFoundError without traceback location — still useful to surface.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "FileNotFoundError:") {
			hint := "[hint: " + truncateErrorLine(trimmed, 150) + " — check the file path exists and is spelled correctly"
			// Common TB2 issue: running from wrong directory.
			if strings.Contains(trimmed, "input_data") || strings.Contains(trimmed, "task_file") {
				hint += ". If running a script, try: cd /app && python3 script.py, or use absolute paths"
			}
			hint += "]"
			return hint
		}
	}

	return ""
}

// truncateErrorLine shortens an error line for hint display.
func truncateErrorLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// compilationErrorHint extracts the first file:line from C/C++, Go, and Rust
// compiler errors so the agent can jump directly to the error location.
// Saves 1-2 turns of the agent reading the full error output and figuring out
// which file and line to view/edit.
func compilationErrorHint(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	lines := strings.Split(output, "\n")

	// C/C++/clang: "file.c:42:5: error: ..."
	// Go: "./main.go:42:5: ..." or "main.go:42:5: ..."
	// Rust (cargo): " --> src/main.rs:42:5"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Rust: " --> file:line:col"
		if strings.HasPrefix(trimmed, "--> ") {
			rest := strings.TrimPrefix(trimmed, "--> ")
			parts := strings.SplitN(rest, ":", 3)
			if len(parts) >= 2 && isNumeric(parts[1]) {
				return fmt.Sprintf("[hint: error at %s:%s — use view tool with offset=%s to see the code, then fix with edit]",
					parts[0], parts[1], parts[1])
			}
		}

		// C/C++/Go: "file:line:col: error:" or "file:line: error:"
		if !strings.Contains(trimmed, ": error") && !strings.Contains(trimmed, ": fatal error") &&
			!strings.Contains(trimmed, ": cannot ") && !strings.Contains(trimmed, ": undefined") &&
			!strings.Contains(trimmed, "cannot find") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 4)
		if len(parts) >= 3 && isNumeric(parts[1]) {
			file := parts[0]
			line := parts[1]
			// Skip very long file paths (probably not real file references)
			if len(file) > 200 {
				continue
			}
			return fmt.Sprintf("[hint: error at %s:%s — use view tool with offset=%s to see the code, then fix with edit]",
				file, line, line)
		}
	}

	return ""
}

// isNumeric returns true if the string contains only digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// jsonErrorHint extracts position info from JSON decode errors. Many TB2 tasks
// involve creating JSON output files, and a decode error with "line X column Y"
// saves the agent from having to count characters manually.
func jsonErrorHint(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	// Python: json.decoder.JSONDecodeError: Expecting ',' delimiter: line 5 column 3 (char 42)
	if idx := strings.Index(output, "JSONDecodeError:"); idx >= 0 {
		rest := output[idx:]
		if end := strings.IndexAny(rest, "\n\r"); end > 0 {
			errLine := strings.TrimSpace(rest[:end])
			return "[hint: " + errLine + " — check the JSON file at that line/column for missing commas, brackets, or quotes]"
		}
		if len(rest) < 200 {
			return "[hint: " + strings.TrimSpace(rest) + " — check JSON syntax]"
		}
	}

	// Node.js: SyntaxError: Unexpected token } in JSON at position 42
	if strings.Contains(output, "SyntaxError:") && strings.Contains(output, "JSON") {
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "SyntaxError:") && strings.Contains(trimmed, "JSON") {
				return "[hint: " + trimmed + " — check JSON syntax at that position]"
			}
		}
	}

	// jq: parse error (Invalid numeric literal at line 3, column 1)
	if strings.Contains(output, "parse error") && strings.Contains(output, "jq") {
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "parse error") {
				return "[hint: " + trimmed + "]"
			}
		}
	}

	return ""
}

// encodingErrorHint detects Python UnicodeDecodeError/UnicodeEncodeError and
// provides the fix. These waste 1-2 turns as the agent figures out encoding.
func encodingErrorHint(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}
	if strings.Contains(output, "UnicodeDecodeError:") {
		return "[hint: UnicodeDecodeError — add encoding='utf-8' and errors='replace' to open() calls, " +
			"or use: open(file, 'r', encoding='utf-8', errors='replace'). " +
			"For binary files, use open(file, 'rb')]"
	}
	if strings.Contains(output, "UnicodeEncodeError:") {
		return "[hint: UnicodeEncodeError — add encoding='utf-8' to open() calls for writing, " +
			"or encode strings with .encode('utf-8', errors='replace')]"
	}
	return ""
}

// permissionHint detects "Permission denied" on executable scripts (exit code 126)
// and suggests chmod +x. Saves 1 turn of troubleshooting.
func permissionHint(output string, exitCode int) string {
	if exitCode == 126 {
		return "[hint: permission denied — the script is not executable. Run: chmod +x <script_path>, then retry]"
	}
	// Also catch "Permission denied" when trying to run a script directly.
	if exitCode != 0 && strings.Contains(output, "Permission denied") {
		lower := strings.ToLower(output)
		// Check if it's a script execution issue (not a file access issue).
		if strings.Contains(lower, ".sh") || strings.Contains(lower, ".py") ||
			strings.Contains(lower, "./") {
			return "[hint: permission denied — try: chmod +x <script>, then retry. Or run with: bash <script> or python3 <script>]"
		}
	}
	return ""
}

// addressInUseHint detects "Address already in use" errors from server processes.
func addressInUseHint(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}
	if strings.Contains(output, "Address already in use") || strings.Contains(output, "EADDRINUSE") {
		return "[hint: port already in use — find the process with: lsof -i :<port> or ss -tlnp | grep <port>, " +
			"then kill it with: kill <pid>. Or use a different port.]"
	}
	return ""
}

// nodeErrorHint extracts actionable information from Node.js errors.
// Maps MODULE_NOT_FOUND to npm install hints and extracts file:line from stacks.
func nodeErrorHint(output string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	// MODULE_NOT_FOUND — suggest npm install.
	if strings.Contains(output, "MODULE_NOT_FOUND") || strings.Contains(output, "Cannot find module") {
		// Extract the module name.
		for _, pattern := range []string{"Cannot find module '", "Cannot find module \""} {
			idx := strings.Index(output, pattern)
			if idx < 0 {
				continue
			}
			start := idx + len(pattern)
			rest := output[start:]
			end := strings.IndexAny(rest, "'\"")
			if end > 0 {
				module := rest[:end]
				// Skip relative paths — those are user code errors, not missing packages.
				if !strings.HasPrefix(module, ".") && !strings.HasPrefix(module, "/") {
					return fmt.Sprintf("[hint: missing Node module — try: npm install %s]", module)
				}
			}
		}
		return "[hint: missing Node module — try: npm install]"
	}

	// ReferenceError, TypeError with stack trace — extract file:line.
	for _, errType := range []string{"ReferenceError:", "TypeError:", "SyntaxError:"} {
		if strings.Contains(output, errType) {
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				// Node stack format: "    at functionName (file:line:col)"
				if strings.HasPrefix(trimmed, "at ") && strings.Contains(trimmed, "(") {
					parenIdx := strings.Index(trimmed, "(")
					if parenIdx > 0 {
						locEnd := strings.Index(trimmed[parenIdx:], ")")
						if locEnd > 0 {
							loc := trimmed[parenIdx+1 : parenIdx+locEnd]
							// Skip node_modules and internal paths.
							if !strings.Contains(loc, "node_modules") && !strings.HasPrefix(loc, "node:") {
								return fmt.Sprintf("[hint: %s — at %s]", errType, loc)
							}
						}
					}
				}
			}
		}
	}

	return ""
}

// testResultSummary extracts a concise summary from test runner output.
// Returns empty string if the output doesn't look like test results.
// This helps the model quickly understand what passed/failed without
// parsing the entire output itself — especially valuable after truncation.
//
// In addition to pass/fail counts, it extracts the FIRST failure's assertion
// detail (e.g., "Expected X, got Y") which is the most actionable information
// for fixing the issue.
func testResultSummary(output string) string {
	lower := strings.ToLower(output)

	// pytest: "X passed", "X failed", "X error"
	if strings.Contains(lower, "passed") && (strings.Contains(lower, "failed") || strings.Contains(lower, "error")) ||
		strings.Contains(lower, "===") && strings.Contains(lower, "passed") {
		// Find the pytest summary line (usually the last line with "passed" and/or "failed")
		lines := strings.Split(output, "\n")
		var summary string
		for i := len(lines) - 1; i >= max(0, len(lines)-10); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if (strings.Contains(lineLower, "passed") || strings.Contains(lineLower, "failed")) &&
				(strings.Contains(line, "=") || strings.Contains(lineLower, "error")) {
				summary = "[test summary: " + line + "]"
				break
			}
		}
		// Append first failure detail for actionable debugging.
		if detail := firstFailureDetail(output); detail != "" {
			if summary != "" {
				summary += "\n" + detail
			} else {
				summary = detail
			}
		}
		if summary != "" {
			return summary
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
			if detail := firstFailureDetail(output); detail != "" {
				summary += "\n" + detail
			}
			return summary
		}
	}

	// Python unittest: "Ran X tests in Y.ZZZs" + "OK" or "FAILED (failures=N, errors=M)"
	if strings.Contains(output, "Ran ") && strings.Contains(lower, "tests") {
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-5); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if strings.Contains(lineLower, "failed") && strings.Contains(lineLower, "failures") {
				var summary string
				// Find the "Ran X tests" line too.
				for j := i - 1; j >= max(0, i-3); j-- {
					if strings.HasPrefix(strings.TrimSpace(lines[j]), "Ran ") {
						summary = "[test summary: " + strings.TrimSpace(lines[j]) + " — " + line + "]"
						break
					}
				}
				if summary == "" {
					summary = "[test summary: " + line + "]"
				}
				if detail := firstFailureDetail(output); detail != "" {
					summary += "\n" + detail
				}
				return summary
			}
			if line == "OK" && i > 0 {
				prev := strings.TrimSpace(lines[i-1])
				if strings.HasPrefix(prev, "Ran ") {
					return "[test summary: " + prev + " — OK]"
				}
			}
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

	// Cargo test: "test result: ok. X passed; Y failed; Z ignored"
	if strings.Contains(output, "test result:") {
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "test result:") {
				return "[test summary: " + trimmed + "]"
			}
		}
	}

	// "X/Y tests passed" or "X out of Y" pattern (custom test scripts).
	// These are common in TB2 tasks that use custom test harnesses.
	{
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-15); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if (strings.Contains(lineLower, "passed") || strings.Contains(lineLower, "failed")) &&
				(strings.Contains(line, "/") || strings.Contains(lineLower, " out of ") || strings.Contains(lineLower, " of ")) &&
				!strings.Contains(line, "===") { // skip pytest output
				return "[test summary: " + line + "]"
			}
		}
	}

	// Shell test scripts: look for reward/score output (TB2 pattern).
	if strings.Contains(lower, "reward") || strings.Contains(lower, "score") {
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-10); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if (strings.Contains(lineLower, "reward") || strings.Contains(lineLower, "score")) &&
				(strings.Contains(line, ":") || strings.Contains(line, "=")) {
				return "[test summary: " + line + "]"
			}
		}
	}

	// RSpec: "3 examples, 1 failure" or "5 examples, 0 failures"
	if strings.Contains(lower, "example") && strings.Contains(lower, "failure") {
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-5); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if strings.Contains(lineLower, "example") && strings.Contains(lineLower, "failure") {
				return "[test summary: " + line + "]"
			}
		}
	}

	// Ruby minitest: "X runs, Y assertions, Z failures, W errors"
	if strings.Contains(lower, "runs") && strings.Contains(lower, "assertions") {
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-5); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if strings.Contains(lineLower, "runs") && strings.Contains(lineLower, "assertions") {
				return "[test summary: " + line + "]"
			}
		}
	}

	// Catch2 (C++): "X test cases - Y assertions - Z failures"
	// or "All tests passed (X assertions in Y test cases)"
	if strings.Contains(lower, "test case") && strings.Contains(lower, "assertion") {
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-5); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if strings.Contains(lineLower, "test case") && strings.Contains(lineLower, "assertion") {
				return "[test summary: " + line + "]"
			}
		}
	}

	// CTest: "X% tests passed, Y tests failed out of Z"
	if strings.Contains(lower, "tests passed") && strings.Contains(lower, "tests failed") {
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= max(0, len(lines)-5); i-- {
			line := strings.TrimSpace(lines[i])
			lineLower := strings.ToLower(line)
			if strings.Contains(lineLower, "tests passed") && strings.Contains(lineLower, "tests failed") {
				return "[test summary: " + line + "]"
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

// firstFailureDetail extracts the assertion detail from the FIRST test failure
// in the output. This is the most actionable information for the agent —
// knowing "Expected 42, got 43" or "AssertionError: lists differ" tells it
// exactly what to fix, saving 1-2 turns of reading test output.
//
// Scans for common assertion patterns across pytest, unittest, Go, Rust, Jest.
func firstFailureDetail(output string) string {
	lines := strings.Split(output, "\n")

	// Patterns that indicate an assertion/comparison failure with details.
	// We want the FIRST one — it's the most useful for "fix one at a time".
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// pytest: "AssertionError: assert X == Y" or "E       assert X == Y"
		if strings.HasPrefix(trimmed, "E ") && (strings.Contains(trimmed, "assert") ||
			strings.Contains(trimmed, "==") || strings.Contains(trimmed, "!=") ||
			strings.Contains(trimmed, "Error") || strings.Contains(trimmed, "not in") ||
			strings.Contains(trimmed, "in ")) {
			detail := strings.TrimPrefix(trimmed, "E ")
			detail = strings.TrimSpace(detail)
			if len(detail) > 200 {
				detail = detail[:200] + "..."
			}
			return "[first failure: " + detail + "]"
		}

		// pytest/unittest: "AssertionError: ..." on its own line
		if strings.HasPrefix(trimmed, "AssertionError:") || strings.HasPrefix(trimmed, "AssertionError(") {
			detail := trimmed
			if len(detail) > 200 {
				detail = detail[:200] + "..."
			}
			return "[first failure: " + detail + "]"
		}

		// Python unittest: "FAIL: test_name (module.TestClass)"
		// followed by assertion details in subsequent lines
		if strings.HasPrefix(trimmed, "FAIL: ") && strings.Contains(trimmed, "(") {
			// Look ahead for the actual assertion
			for j := i + 1; j < min(i+8, len(lines)); j++ {
				ahead := strings.TrimSpace(lines[j])
				if strings.HasPrefix(ahead, "AssertionError:") ||
					strings.Contains(ahead, "!=") && strings.Contains(ahead, "assert") ||
					strings.HasPrefix(ahead, "Expected") ||
					strings.HasPrefix(ahead, "Got") {
					if len(ahead) > 200 {
						ahead = ahead[:200] + "..."
					}
					return "[first failure: " + ahead + "]"
				}
			}
		}

		// Go test: lines right after "--- FAIL:" often have "expected X, got Y"
		// or "want X, got Y"
		if strings.HasPrefix(trimmed, "--- FAIL:") {
			for j := i + 1; j < min(i+8, len(lines)); j++ {
				ahead := strings.TrimSpace(lines[j])
				aheadLower := strings.ToLower(ahead)
				if (strings.Contains(aheadLower, "expected") || strings.Contains(aheadLower, "want ")) &&
					(strings.Contains(aheadLower, "got ") || strings.Contains(aheadLower, "actual")) {
					if len(ahead) > 200 {
						ahead = ahead[:200] + "..."
					}
					return "[first failure: " + ahead + "]"
				}
				// Go test: Error() or Errorf() messages
				if strings.Contains(ahead, "Error Trace:") {
					// Skip trace, look for the message
					for k := j + 1; k < min(j+5, len(lines)); k++ {
						msg := strings.TrimSpace(lines[k])
						if strings.HasPrefix(msg, "Error:") || strings.HasPrefix(msg, "Messages:") {
							if len(msg) > 200 {
								msg = msg[:200] + "..."
							}
							return "[first failure: " + msg + "]"
						}
					}
				}
			}
		}

		// Classic diff format: "< expected\n---\n> actual" (common in TB2 test.sh scripts)
		if strings.HasPrefix(trimmed, "< ") && i+2 < len(lines) {
			sep := strings.TrimSpace(lines[i+1])
			nextLine := strings.TrimSpace(lines[i+2])
			if sep == "---" && strings.HasPrefix(nextLine, "> ") {
				expected := strings.TrimPrefix(trimmed, "< ")
				actual := strings.TrimPrefix(nextLine, "> ")
				detail := fmt.Sprintf("diff: expected %q, got %q", expected, actual)
				if len(detail) > 200 {
					detail = detail[:200] + "..."
				}
				return "[first failure: " + detail + "]"
			}
		}

		// Unified diff header: "@@ -N,M +N,M @@" — extract first changed line pair
		if strings.HasPrefix(trimmed, "@@ ") && len(trimmed) > 3 && strings.Contains(trimmed[3:], " @@") {
			for j := i + 1; j < min(i+20, len(lines)); j++ {
				jLine := lines[j]
				if strings.HasPrefix(jLine, "-") && !strings.HasPrefix(jLine, "---") {
					expected := strings.TrimPrefix(jLine, "-")
					if j+1 < len(lines) && strings.HasPrefix(lines[j+1], "+") && !strings.HasPrefix(lines[j+1], "+++") {
						actual := strings.TrimPrefix(lines[j+1], "+")
						detail := fmt.Sprintf("diff: expected %q, got %q", strings.TrimSpace(expected), strings.TrimSpace(actual))
						if len(detail) > 200 {
							detail = detail[:200] + "..."
						}
						return "[first failure: " + detail + "]"
					}
					break
				}
			}
		}

		// Rust: thread 'test_name' panicked at 'assertion `left == right` failed'
		if strings.Contains(trimmed, "panicked at") {
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "assert") || strings.Contains(lower, "left") || strings.Contains(lower, "unwrap") {
				detail := trimmed
				for j := i + 1; j < min(i+5, len(lines)); j++ {
					ahead := strings.TrimSpace(lines[j])
					aheadLower := strings.ToLower(ahead)
					if strings.HasPrefix(aheadLower, "left:") || strings.HasPrefix(aheadLower, "right:") {
						detail += " / " + ahead
					}
				}
				if len(detail) > 200 {
					detail = detail[:200] + "..."
				}
				return "[first failure: " + detail + "]"
			}
		}

		// Generic "Expected X, got Y" pattern (many test frameworks)
		lower := strings.ToLower(trimmed)
		if (strings.Contains(lower, "expected") && strings.Contains(lower, "got")) ||
			(strings.Contains(lower, "expected") && strings.Contains(lower, "actual")) ||
			(strings.Contains(lower, "expected") && strings.Contains(lower, "received")) {
			// Make sure it's an actual assertion, not just a comment or header.
			// Headers like "Comparing expected vs actual:" don't contain values.
			if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") &&
				!strings.HasPrefix(trimmed, "*") && len(trimmed) > 10 &&
				!strings.HasSuffix(trimmed, ":") { // skip headers ending in colon
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				return "[first failure: " + trimmed + "]"
			}
		}

		// Jest/Vitest: "Expected: X" / "Received: Y" (consecutive lines)
		if strings.HasPrefix(trimmed, "Expected:") || strings.HasPrefix(trimmed, "- Expected") {
			detail := trimmed
			// Grab the "Received:" line too if it follows
			if i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextTrimmed, "Received:") || strings.HasPrefix(nextTrimmed, "+ Received") {
					detail += " / " + nextTrimmed
				}
			}
			if len(detail) > 200 {
				detail = detail[:200] + "..."
			}
			return "[first failure: " + detail + "]"
		}
	}

	return ""
}

// testFailureFingerprint creates a stable fingerprint of a test failure.
// Used to detect when the same test fails identically after edits (stale failure).
// Uses the first failure's assertion detail as the fingerprint — if the first
// failure assertion is identical, the agent's fix was ineffective.
func testFailureFingerprint(output string) string {
	return firstFailureDetail(output)
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
		"python3 /app/", "python /app/", // app scripts
		"python3 solve", "python solve", // solver scripts
		"python3 process", "python process", // data processing
		"python3 run", "python run",     // generic runner scripts
		"bash /app/", "sh /app/",        // shell scripts in /app/
		"bash /tests/", "sh /tests/",    // test scripts
	}
	for _, p := range longPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isPipCommand returns true if the command involves pip installing packages.
// Used to auto-set PIP_BREAK_SYSTEM_PACKAGES=1 for container environments.
func isPipCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	return strings.Contains(lower, "pip install") ||
		strings.Contains(lower, "pip3 install") ||
		strings.Contains(lower, "python -m pip") ||
		strings.Contains(lower, "python3 -m pip")
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

// isDestructiveTestCommand checks whether a bash command would destructively
// modify files in /tests/ (the verifier test directory). This blocks:
//   - Redirects to /tests/ files (>, >>)
//   - rm, mv, cp targeting /tests/
//   - sed -i (in-place edit) on /tests/ files
//   - chmod, chown on /tests/ files
//   - truncate on /tests/ files
//
// It does NOT block running tests (bash /tests/test.sh, python3 /tests/test.py)
// or reading tests (cat /tests/test.sh, head /tests/test.py).
func isDestructiveTestCommand(cmd string) bool {
	// Quick check: if the command doesn't reference /tests/, skip.
	if !strings.Contains(cmd, "/tests/") {
		return false
	}

	// Destructive patterns that target /tests/ files.
	lower := strings.ToLower(cmd)

	// Redirects to /tests/ files.
	if (strings.Contains(cmd, "> /tests/") || strings.Contains(cmd, ">/tests/") ||
		strings.Contains(cmd, ">> /tests/") || strings.Contains(cmd, ">>/tests/")) {
		return true
	}

	// tee writing to /tests/ files.
	if strings.Contains(lower, "tee ") && strings.Contains(cmd, "/tests/") {
		// Check if /tests/ comes after tee (output target).
		teeIdx := strings.Index(lower, "tee ")
		testsIdx := strings.Index(cmd[teeIdx:], "/tests/")
		if testsIdx > 0 {
			return true
		}
	}

	// rm targeting /tests/ files.
	if (strings.Contains(lower, "rm ") || strings.Contains(lower, "rm -")) && strings.Contains(cmd, "/tests/") {
		return true
	}

	// sed -i (in-place edit) on /tests/ files.
	if strings.Contains(lower, "sed ") && strings.Contains(lower, "-i") && strings.Contains(cmd, "/tests/") {
		return true
	}

	// chmod, chown on /tests/ files (prevents making tests non-executable, etc.).
	if (strings.Contains(lower, "chmod ") || strings.Contains(lower, "chown ")) && strings.Contains(cmd, "/tests/") {
		return true
	}

	// truncate on /tests/ files.
	if strings.Contains(lower, "truncate ") && strings.Contains(cmd, "/tests/") {
		return true
	}

	// mv/cp with /tests/ as destination (overwriting test files).
	// Only block if /tests/ is in the latter part (destination).
	if strings.Contains(lower, "cp ") || strings.Contains(lower, "mv ") {
		// Split on /tests/ and check if it appears after the first argument.
		parts := strings.SplitN(cmd, "/tests/", 2)
		if len(parts) == 2 {
			before := parts[0]
			// If /tests/ follows the source argument (i.e., it's the destination), block.
			if strings.Contains(before, "cp ") || strings.Contains(before, "mv ") {
				// But NOT if /tests/ is the source (first arg after cp/mv).
				lastSpace := strings.LastIndex(strings.TrimSpace(before), " ")
				if lastSpace > 0 {
					return true
				}
			}
		}
	}

	return false
}

// isTransientBashFailure returns true if the bash output suggests a transient
// failure that's likely to succeed on retry (network errors, lock contention).
func isTransientBashFailure(exitCode int, output string) bool {
	if exitCode == 0 {
		return false
	}
	lower := strings.ToLower(output)

	transientPatterns := []string{
		"could not resolve host",
		"connection timed out",
		"connection refused",
		"temporary failure in name resolution",
		"network is unreachable",
		"unable to fetch",
		"failed to download",
		"dpkg was interrupted",
		"unable to acquire the dpkg",
		"is another process using it",
		"hash sum mismatch",        // apt mirror inconsistency
		"failed to fetch",          // apt download failure
		"connection reset by peer",
		"ssl_error_syscall",
		"read: connection reset",
	}
	for _, p := range transientPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

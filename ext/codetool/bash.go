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

			// Truncate long output, keeping head and tail so the model can
			// see error summaries at the end.
			outStr = truncateOutput(outStr, cfg.MaxOutputLen)
			errStr = truncateOutput(errStr, cfg.MaxOutputLen)

			result := formatBashOutput(outStr, errStr, exitCode, timedOut, timeout)

			// Add hint for "command not found" errors — saves a turn of
			// the model figuring out the package name.
			if exitCode == 127 || strings.Contains(errStr, "command not found") || strings.Contains(errStr, "No such file or directory") {
				if hint := commandNotFoundHint(errStr); hint != "" {
					result += "\n" + hint
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
		b.WriteString(fmt.Sprintf("[timed out after %s]", timeout))
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

// isBuildCommand detects commands that typically need longer timeouts.
func isBuildCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	buildPatterns := []string{
		"make", "cmake", "cargo build", "cargo install",
		"go build", "go install",
		"gcc ", "g++ ", "clang ", "cc ",
		"javac ", "mvn ", "gradle ",
		"npm install", "npm ci", "yarn install", "pnpm install",
		"pip install", "pip3 install",
		"apt-get install", "apt install", "apk add", "yum install", "dnf install",
		"docker build",
		"lake build", // Lean 4
		"./configure",
	}
	for _, p := range buildPatterns {
		if strings.HasPrefix(lower, p) || strings.Contains(lower, " && "+p) || strings.Contains(lower, "; "+p) {
			return true
		}
	}
	return false
}

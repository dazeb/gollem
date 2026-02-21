package codetool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// BashParams are the parameters for the bash tool.
type BashParams struct {
	// Command is the shell command to execute.
	Command string `json:"command" jsonschema:"description=The bash command to execute"`

	// Timeout is an optional timeout in seconds. Overrides the default.
	Timeout *int `json:"timeout,omitempty" jsonschema:"description=Optional timeout in seconds (default: 120)"`
}

// BashResult is the result of a bash command execution.
type BashResult struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// Bash creates a tool that executes shell commands.
func Bash(opts ...Option) core.Tool {
	cfg := applyOpts(opts)
	return core.FuncTool[BashParams](
		"bash",
		"Execute a bash command in the shell. Use this for running programs, installing packages, "+
			"compiling code, running tests, git operations, and any other terminal commands. "+
			"Commands run in a persistent working directory. "+
			"Prefer this tool for exploring the filesystem and running build/test commands.",
		func(ctx context.Context, params BashParams) (BashResult, error) {
			if strings.TrimSpace(params.Command) == "" {
				return BashResult{}, &core.ModelRetryError{Message: "command must not be empty"}
			}

			timeout := cfg.BashTimeout
			if params.Timeout != nil && *params.Timeout > 0 {
				timeout = time.Duration(*params.Timeout) * time.Second
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
			if cfg.WorkDir != "" {
				cmd.Dir = cfg.WorkDir
			}

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			exitCode := 0
			if err != nil {
				// Check timeout first — on some platforms a killed process
				// returns ExitError with code -1, so we must check context
				// before inspecting the exit code.
				if ctx.Err() == context.DeadlineExceeded {
					return BashResult{
						Stdout:   stdout.String(),
						Stderr:   fmt.Sprintf("Command timed out after %s", timeout),
						ExitCode: 124,
					}, nil
				}
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					exitCode = exitErr.ExitCode()
				} else {
					return BashResult{}, fmt.Errorf("failed to execute command: %w", err)
				}
			}

			outStr := stdout.String()
			errStr := stderr.String()

			// Truncate output if too long.
			if cfg.MaxOutputLen > 0 && len(outStr) > cfg.MaxOutputLen {
				outStr = outStr[:cfg.MaxOutputLen] + fmt.Sprintf("\n... [truncated, %d bytes total]", len(stdout.String()))
			}
			if cfg.MaxOutputLen > 0 && len(errStr) > cfg.MaxOutputLen {
				errStr = errStr[:cfg.MaxOutputLen] + fmt.Sprintf("\n... [truncated, %d bytes total]", len(stderr.String()))
			}

			return BashResult{
				Stdout:   outStr,
				Stderr:   errStr,
				ExitCode: exitCode,
			}, nil
		},
		core.WithToolSequential(true), // bash commands should run sequentially
	)
}

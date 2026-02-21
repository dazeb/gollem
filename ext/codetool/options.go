package codetool

import (
	"time"

	montygo "github.com/fugue-labs/monty-go"
)

// Config holds shared configuration for coding tools.
type Config struct {
	// WorkDir is the working directory for shell commands and file operations.
	// Defaults to the current working directory.
	WorkDir string

	// BashTimeout is the default timeout for shell commands.
	// Defaults to 120 seconds.
	BashTimeout time.Duration

	// MaxFileSize is the maximum file size in bytes that View will read.
	// Defaults to 1MB.
	MaxFileSize int64

	// MaxOutputLen is the maximum output length in bytes from Bash.
	// Defaults to 100KB.
	MaxOutputLen int

	// Runner is an optional monty-go WASM Python runner for code mode.
	// When set, the agent gets an execute_code tool that lets it batch
	// multiple tool calls into a single Python script (N calls per round-trip).
	Runner *montygo.Runner
}

// Option configures coding tools.
type Option func(*Config)

func defaults() *Config {
	return &Config{
		BashTimeout:  120 * time.Second,
		MaxFileSize:  1 << 20, // 1MB
		MaxOutputLen: 100 * 1024,
	}
}

func applyOpts(opts []Option) *Config {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// WithWorkDir sets the working directory for tools.
func WithWorkDir(dir string) Option {
	return func(c *Config) { c.WorkDir = dir }
}

// WithBashTimeout sets the default timeout for bash commands.
func WithBashTimeout(d time.Duration) Option {
	return func(c *Config) { c.BashTimeout = d }
}

// WithMaxFileSize sets the maximum readable file size.
func WithMaxFileSize(n int64) Option {
	return func(c *Config) { c.MaxFileSize = n }
}

// WithMaxOutputLen sets the maximum output length from bash commands.
func WithMaxOutputLen(n int) Option {
	return func(c *Config) { c.MaxOutputLen = n }
}

// WithCodeMode enables code mode using a monty-go WASM Python runner.
// The agent gets an execute_code tool alongside individual tools, letting
// it batch multiple operations into a single Python script per API call.
// The caller owns the runner lifecycle (create with montygo.New(), close
// when done).
func WithCodeMode(runner *montygo.Runner) Option {
	return func(c *Config) { c.Runner = runner }
}

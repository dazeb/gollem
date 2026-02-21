package codetool

import "time"

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

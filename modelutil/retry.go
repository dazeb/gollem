package modelutil

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries     int           // maximum number of retries (default: 3)
	InitialBackoff time.Duration // initial wait time (default: 1s)
	MaxBackoff     time.Duration // maximum wait time (default: 30s)
	BackoffFactor  float64       // multiplier per retry (default: 2.0)
	Jitter         bool          // add random jitter (default: true)
	IsRetryable    func(error) bool // determines if an error should be retried
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
		Jitter:         true,
		IsRetryable:    defaultIsRetryable,
	}
}

// defaultIsRetryable checks for transient HTTP and connection errors.
func defaultIsRetryable(err error) bool {
	var httpErr *core.ModelHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			return true
		}
	}

	// Retry on connection-level errors (unexpected EOF, connection reset, timeout).
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check error message for common transient patterns.
	msg := err.Error()
	for _, pattern := range []string{"unexpected EOF", "connection reset", "connection refused", "broken pipe", "i/o timeout", "no such host", "dns", "temporary failure"} {
		if strings.Contains(strings.ToLower(msg), pattern) {
			return true
		}
	}

	return false
}

// RetryModel wraps a Model with exponential backoff retry for transient failures.
type RetryModel struct {
	model  core.Model
	config RetryConfig
}

// NewRetryModel creates a Model that retries transient failures.
func NewRetryModel(model core.Model, config RetryConfig) *RetryModel {
	if config.IsRetryable == nil {
		config.IsRetryable = defaultIsRetryable
	}
	if config.BackoffFactor == 0 {
		config.BackoffFactor = 2.0
	}
	if config.InitialBackoff == 0 {
		config.InitialBackoff = time.Second
	}
	if config.MaxBackoff == 0 {
		config.MaxBackoff = 30 * time.Second
	}
	return &RetryModel{model: model, config: config}
}

var _ core.Model = (*RetryModel)(nil)

func (r *RetryModel) ModelName() string {
	return r.model.ModelName()
}

func (r *RetryModel) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return retryLoop(ctx, r.config, func() (*core.ModelResponse, error) {
		return r.model.Request(ctx, messages, settings, params)
	})
}

func (r *RetryModel) RequestStream(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return retryLoop(ctx, r.config, func() (core.StreamedResponse, error) {
		return r.model.RequestStream(ctx, messages, settings, params)
	})
}

func retryLoop[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !cfg.IsRetryable(err) {
			return zero, err
		}

		// Don't sleep after the last attempt.
		if attempt < cfg.MaxRetries {
			wait := backoff

			// Use Retry-After from the provider if available (e.g., 429 rate limits).
			// This is more accurate than exponential backoff for rate-limited APIs.
			var httpErr *core.ModelHTTPError
			if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
				wait = httpErr.RetryAfter
			}

			if cfg.Jitter {
				// Add 0-25% jitter.
				jitter := time.Duration(float64(wait) * 0.25 * rand.Float64())
				wait += jitter
			}

			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(wait):
			}

			backoff = time.Duration(float64(backoff) * cfg.BackoffFactor)
			if backoff > cfg.MaxBackoff {
				backoff = cfg.MaxBackoff
			}
		}
	}

	return zero, lastErr
}

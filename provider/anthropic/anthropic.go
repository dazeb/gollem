// Package anthropic provides a core.Model implementation for Anthropic's
// Messages API, supporting Claude models with tool use, streaming, and
// extended thinking.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// Model constants for Anthropic Claude models.
const (
	Claude4Opus   = "claude-opus-4-6"
	Claude4Sonnet = "claude-sonnet-4-5-20250929"
	Claude4Haiku  = "claude-haiku-4-5-20251001"
)

const (
	defaultBaseURL       = "https://api.anthropic.com"
	defaultModel         = Claude4Sonnet
	defaultMaxTokens     = 4096
	anthropicVersion     = "2023-06-01"
	messagesEndpoint     = "/v1/messages"
)

// Provider implements core.Model for Anthropic's Messages API.
type Provider struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	maxTokens  int
}

// Option configures the Anthropic provider.
type Option func(*Provider)

// WithAPIKey sets the API key. If not set, reads from ANTHROPIC_API_KEY env var.
func WithAPIKey(key string) Option {
	return func(p *Provider) {
		p.apiKey = key
	}
}

// WithModel sets the model to use.
func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) {
		p.httpClient = c
	}
}

// WithMaxTokens sets the default max tokens for requests.
func WithMaxTokens(n int) Option {
	return func(p *Provider) {
		p.maxTokens = n
	}
}

// New creates a new Anthropic provider with the given options.
func New(opts ...Option) *Provider {
	p := &Provider{
		model:      defaultModel,
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
		maxTokens:  defaultMaxTokens,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		p.apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return p
}

// ModelName returns the model identifier.
func (p *Provider) ModelName() string {
	return p.model
}

// Request sends messages to Anthropic and returns a complete response.
func (p *Provider) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	req, err := buildRequest(messages, settings, params, p.model, p.maxTokens, false)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to build request: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	resp, err := p.doWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: failed to decode response: %w", err)
	}

	return parseResponse(&apiResp, p.model), nil
}

// RequestStream sends messages and returns a streaming response.
func (p *Provider) RequestStream(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (core.StreamedResponse, error) {
	req, err := buildRequest(messages, settings, params, p.model, p.maxTokens, true)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to build request: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	resp, err := p.doWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	return newStreamedResponse(resp.Body, p.model), nil
}

// doWithRetry sends an HTTP request with automatic retry for transient errors
// (429 rate limits, 500/502/503 server errors). Uses exponential backoff,
// respecting Retry-After headers when present.
func (p *Provider) doWithRetry(ctx context.Context, body []byte) (*http.Response, error) {
	const maxRetries = 3
	baseDelay := 2 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			if lastErr != nil {
				if httpErr, ok := lastErr.(*core.ModelHTTPError); ok && httpErr.RetryAfter > 0 {
					delay = httpErr.RetryAfter
				}
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+messagesEndpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("anthropic: failed to create HTTP request: %w", err)
		}
		p.setHeaders(httpReq)

		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("anthropic: HTTP request failed: %w", err)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		httpErr := &core.ModelHTTPError{
			Message:    "anthropic API error: " + string(respBody),
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			ModelName:  p.model,
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					httpErr.RetryAfter = time.Duration(secs) * time.Second
				}
			}
		}

		if !isRetryableStatus(resp.StatusCode) {
			return nil, httpErr
		}

		lastErr = httpErr
		fmt.Fprintf(os.Stderr, "[gollem] anthropic: retrying after %d (attempt %d/%d)\n",
			resp.StatusCode, attempt+1, maxRetries)
	}

	return nil, lastErr
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

func (p *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
}

// Verify Provider implements core.Model.
var _ core.Model = (*Provider)(nil)

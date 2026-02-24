// Package openai provides a core.Model implementation for OpenAI's
// Chat Completions API, supporting GPT and O-series models with tool use,
// streaming, and native structured output.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// Model constants for OpenAI models.
const (
	GPT4o     = "gpt-4o"
	GPT4oMini = "gpt-4o-mini"
	O3        = "o3"
	O3Mini    = "o3-mini"
	O4Mini    = "o4-mini"
)

const (
	defaultBaseURL          = "https://api.openai.com"
	defaultModel            = GPT4o
	defaultMaxTokens        = 4096
	chatCompletionsEndpoint = "/v1/chat/completions"
)

// Provider implements core.Model for OpenAI's Chat Completions API.
type Provider struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	maxTokens  int
}

// Option configures the OpenAI provider.
type Option func(*Provider)

// WithAPIKey sets the API key. If not set, reads from OPENAI_API_KEY env var.
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

// WithBaseURL sets a custom base URL (useful for proxies and compatible APIs).
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

// New creates a new OpenAI provider with the given options.
// Supports OPENAI_API_KEY and OPENAI_BASE_URL environment variables
// for compatibility with OpenAI-compatible APIs (xAI, Together, etc.).
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
		p.apiKey = os.Getenv("OPENAI_API_KEY")
	}
	// Support OPENAI_BASE_URL env var (standard for OpenAI-compatible APIs).
	if p.baseURL == defaultBaseURL {
		if envURL := os.Getenv("OPENAI_BASE_URL"); envURL != "" {
			p.baseURL = envURL
		}
	}
	// Strip trailing /v1 or /v1/ from the base URL. Our endpoint path
	// already includes /v1, so a base URL with /v1 (which is the convention
	// in the OpenAI Python client) would produce /v1/v1/chat/completions.
	p.baseURL = strings.TrimRight(p.baseURL, "/")
	if strings.HasSuffix(p.baseURL, "/v1") {
		p.baseURL = strings.TrimSuffix(p.baseURL, "/v1")
	}
	return p
}

// NewLiteLLM creates an OpenAI-compatible provider configured for a LiteLLM proxy.
func NewLiteLLM(baseURL string, opts ...Option) *Provider {
	allOpts := append([]Option{WithBaseURL(baseURL)}, opts...)
	p := New(allOpts...)
	// LiteLLM uses OPENAI_API_KEY by default, no special handling needed.
	return p
}

// NewOllama creates an OpenAI-compatible provider configured for a local Ollama instance.
// By default it connects to http://localhost:11434 and uses a dummy API key since
// Ollama does not require authentication. The model should be set via WithModel
// to match a model pulled in Ollama (e.g., "llama3", "mistral", "codellama").
func NewOllama(opts ...Option) *Provider {
	allOpts := append([]Option{
		WithBaseURL("http://localhost:11434"),
		WithAPIKey("ollama"),
	}, opts...)
	return New(allOpts...)
}

// ModelName returns the model identifier.
func (p *Provider) ModelName() string {
	return p.model
}

// Request sends messages to OpenAI and returns a complete response.
func (p *Provider) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	req, err := buildRequest(messages, settings, params, p.model, p.maxTokens, false)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to build request: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("openai: failed to decode response: %w", err)
	}

	return parseResponse(&apiResp, p.model), nil
}

// RequestStream sends messages and returns a streaming response.
func (p *Provider) RequestStream(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (core.StreamedResponse, error) {
	req, err := buildRequest(messages, settings, params, p.model, p.maxTokens, true)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to build request: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	return newStreamedResponse(resp.Body, p.model), nil
}

// doRequest sends a single HTTP request and returns the response or a typed
// error. Retry logic is handled at the model level by modelutil.RetryModel,
// which uses this error's RetryAfter field for backoff.
func (p *Provider) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+chatCompletionsEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create HTTP request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: HTTP request failed: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	httpErr := &core.ModelHTTPError{
		Message:    "openai API error: " + string(respBody),
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		ModelName:  p.model,
	}

	// Parse Retry-After header for 429 responses so the model-level
	// retry (modelutil.RetryModel) can use appropriate backoff.
	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				httpErr.RetryAfter = time.Duration(secs) * time.Second
			}
		}
	}

	return nil, httpErr
}

func (p *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
}

// Verify Provider implements core.Model.
var _ core.Model = (*Provider)(nil)

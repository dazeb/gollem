package openai

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/fugue-labs/gollem/modelutil"
)

const (
	// LocalEndpointBaseURLEnv configures the loopback OpenAI-compatible endpoint.
	// It intentionally accepts only loopback URLs so this profile cannot become an
	// unreviewed remote-provider escape hatch.
	LocalEndpointBaseURLEnv = "GOLLEM_LOCAL_OPENAI_BASE_URL"
	// LocalEndpointModelEnv selects the configured-model fallback when discovery
	// is unavailable from a local OpenAI-compatible server.
	LocalEndpointModelEnv = "GOLLEM_LOCAL_OPENAI_MODEL"
	// LocalEndpointAPIKeyEnv supplies an optional local proxy token. Its value is
	// kept inside the provider process and is never projected through the catalog.
	LocalEndpointAPIKeyEnv = "GOLLEM_LOCAL_OPENAI_API_KEY"

	defaultLocalEndpointBaseURL = "http://127.0.0.1:11434"
	defaultLocalEndpointModel   = "llama3"
	defaultLocalEndpointAPIKey  = "local"
)

// LocalEndpointConfig is the process-owned configuration for a trusted local
// OpenAI-compatible Chat Completions endpoint. It contains no renderer-facing
// identity and must be normalized before constructing a provider.
type LocalEndpointConfig struct {
	BaseURL string
	Model   string
	Token   string
}

// LocalEndpointConfigFromLookup builds a local profile from environment-like
// configuration. It is injectable for deterministic catalog and runtime tests.
func LocalEndpointConfigFromLookup(lookup func(string) (string, bool)) (LocalEndpointConfig, error) {
	config := LocalEndpointConfig{
		BaseURL: defaultLocalEndpointBaseURL,
		Model:   defaultLocalEndpointModel,
		Token:   defaultLocalEndpointAPIKey,
	}
	if lookup != nil {
		if value, ok := lookup(LocalEndpointBaseURLEnv); ok && strings.TrimSpace(value) != "" {
			config.BaseURL = value
		}
		if value, ok := lookup(LocalEndpointModelEnv); ok && strings.TrimSpace(value) != "" {
			config.Model = value
		}
		if value, ok := lookup(LocalEndpointAPIKeyEnv); ok && strings.TrimSpace(value) != "" {
			config.Token = value
		}
	}
	return NormalizeLocalEndpointConfig(config)
}

// NormalizeLocalEndpointConfig accepts only HTTP(S) loopback URLs, a bounded
// configured-model fallback, and a bounded optional local proxy token.
func NormalizeLocalEndpointConfig(config LocalEndpointConfig) (LocalEndpointConfig, error) {
	baseURL, err := normalizeLocalEndpointBaseURL(config.BaseURL)
	if err != nil {
		return LocalEndpointConfig{}, err
	}
	model, err := normalizeLocalEndpointText(config.Model, defaultLocalEndpointModel, "model", 256)
	if err != nil {
		return LocalEndpointConfig{}, err
	}
	token, err := normalizeLocalEndpointText(config.Token, defaultLocalEndpointAPIKey, "token", 4_096)
	if err != nil {
		return LocalEndpointConfig{}, err
	}
	return LocalEndpointConfig{BaseURL: baseURL, Model: model, Token: token}, nil
}

// NewLocalEndpoint creates a conservative OpenAI-compatible local provider.
// It forces the Chat Completions API and does not inherit OPENAI_API_KEY,
// Responses API behavior, or OpenAI-specific prompt-cache settings.
func NewLocalEndpoint(config LocalEndpointConfig, opts ...Option) (*Provider, error) {
	config, err := NormalizeLocalEndpointConfig(config)
	if err != nil {
		return nil, err
	}
	options := append([]Option(nil), opts...)
	options = append(options,
		WithBaseURL(config.BaseURL),
		WithModel(config.Model),
		WithAPIKey(config.Token),
		withLocalEndpointConstraints(),
	)
	return New(options...), nil
}

func withLocalEndpointConstraints() Option {
	return func(p *Provider) {
		p.forceChatCompletions = true
		p.redactEndpointErrors = true
		p.useResponses = false
		p.promptCacheKey = ""
		p.promptCacheRetention = ""
		p.serviceTier = ""
		p.transport = transportHTTP
		p.wsHTTPFallback = false
		p.wsHTTPFallbackSet = false
		p.wsPrevResponseID = ""
		p.wsLastInputSigs = nil
		p.reasoningSummary = ""
		p.reasoningSummaryHandler = nil
		p.textVerbosity = ""
		p.chatgptAccountID = ""
		p.tokenRefresher = nil
		p.profileOverride = &modelutil.ModelProfile{
			SupportsToolCalls: true,
			SupportsStreaming: true,
			ProviderName:      "openai-compatible-local",
		}
	}
}

func normalizeLocalEndpointBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = defaultLocalEndpointBaseURL
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", errors.New("invalid local OpenAI-compatible endpoint")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("local OpenAI-compatible endpoint must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid local OpenAI-compatible endpoint")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", errors.New("local OpenAI-compatible endpoint must use a loopback host")
		}
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65_535 {
			return "", errors.New("invalid local OpenAI-compatible endpoint port")
		}
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path != "" && path != "/v1" {
		return "", errors.New("local OpenAI-compatible endpoint path must be empty or /v1")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeLocalEndpointText(raw, fallback, name string, maxBytes int) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = fallback
	}
	if len(value) > maxBytes {
		return "", fmt.Errorf("local OpenAI-compatible %s exceeds %d bytes", name, maxBytes)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("local OpenAI-compatible %s contains a control character", name)
		}
	}
	return value, nil
}

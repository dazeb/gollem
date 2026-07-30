package anthropic

import (
	"fmt"
	"io"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// Provider error payloads are untrusted and may echo request data. Keep them
// bounded for classification and expose only fixed, source-free markers.
const maxProviderErrorBodyBytes = 64 << 10

var providerErrorMarkers = []struct {
	marker string
	terms  []string
}{
	{marker: "credits_exhausted", terms: []string{"credits", "spending limit", "billing", "quota exceeded", "plan limit", "subscription"}},
	{marker: "rate_limited", terms: []string{"rate_limit", "rate limited", "too many requests"}},
	{marker: "overloaded", terms: []string{"overloaded", "capacity"}},
	{marker: "authentication", terms: []string{"authentication", "api key", "invalid_api_key", "permission"}},
	{marker: "context_length", terms: []string{"context length", "too many tokens", "max_tokens"}},
	{marker: "not_found", terms: []string{"not found", "not_found"}},
	{marker: "bad_request", terms: []string{"invalid_request", "invalid request"}},
}

func readProviderErrorClassification(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, maxProviderErrorBodyBytes+1))
	overflow := len(body) > maxProviderErrorBodyBytes
	if overflow {
		body = body[:maxProviderErrorBodyBytes]
	}
	classification := classifyProviderErrorBody(string(body))
	if overflow {
		return classification + ",response_too_large"
	}
	return classification
}

func classifyProviderErrorBody(body string) string {
	combined := strings.ToLower(body)
	for _, candidate := range providerErrorMarkers {
		for _, term := range candidate.terms {
			if strings.Contains(combined, term) {
				return candidate.marker
			}
		}
	}
	return "provider_error"
}

func sanitizedProviderHTTPError(status int, classification, model string) *core.ModelHTTPError {
	if classification == "" {
		classification = "provider_error"
	}
	return &core.ModelHTTPError{
		Message:    fmt.Sprintf("anthropic API error (HTTP %d; %s)", status, classification),
		StatusCode: status,
		Body:       classification,
		ModelName:  model,
	}
}

// Package vertexerror provides bounded, secret-safe HTTP error normalization
// shared by the Google-backed provider adapters.
package vertexerror

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

const maxProviderErrorBodyBytes = 64 << 10

var providerErrorMarkers = []struct {
	marker string
	terms  []string
}{
	{marker: "quota_exhausted", terms: []string{"quota exceeded", "quota exhausted", "billing", "spending limit", "subscription"}},
	{marker: "rate_limited", terms: []string{"rate_limit", "rate limit", "resource_exhausted", "resource exhausted", "too many requests"}},
	{marker: "overloaded", terms: []string{"overloaded", "capacity", "temporarily unavailable"}},
	{marker: "authentication", terms: []string{"unauthenticated", "unauthorized", "authentication", "permission denied", "permission_denied", "forbidden"}},
	{marker: "context_length", terms: []string{"context length", "too many tokens", "token limit", "maximum context"}},
	{marker: "not_found", terms: []string{"not found", "not_found"}},
	{marker: "invalid_request", terms: []string{"invalid argument", "invalid_argument", "invalid request", "invalid_request", "bad request"}},
	{marker: "server_error", terms: []string{"internal error", "backend error", "service unavailable"}},
}

// NewHTTPError returns a bounded, provider-neutral error receipt. Provider
// response bodies are untrusted because they may echo request data.
func NewHTTPError(provider string, status int, body io.Reader, retryAfter, model string) *core.ModelHTTPError {
	classification := readClassification(body)
	return &core.ModelHTTPError{
		Message:    fmt.Sprintf("%s API error (HTTP %d; %s)", provider, status, classification),
		StatusCode: status,
		Body:       classification,
		ModelName:  model,
		RetryAfter: parseRetryAfter(status, retryAfter),
	}
}

func readClassification(reader io.Reader) string {
	if reader == nil {
		return "response_unreadable"
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxProviderErrorBodyBytes+1))
	if err != nil {
		return "response_unreadable"
	}
	overflow := len(body) > maxProviderErrorBodyBytes
	if overflow {
		body = body[:maxProviderErrorBodyBytes]
	}
	classification := classify(string(body))
	if overflow {
		return classification + ",response_too_large"
	}
	return classification
}

func classify(body string) string {
	value := strings.ToLower(body)
	for _, candidate := range providerErrorMarkers {
		for _, term := range candidate.terms {
			if strings.Contains(value, term) {
				return candidate.marker
			}
		}
	}
	return "provider_error"
}

func parseRetryAfter(status int, value string) time.Duration {
	if status != 429 {
		return 0
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

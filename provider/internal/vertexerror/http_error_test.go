package vertexerror

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPErrorRedactsAndClassifies(t *testing.T) {
	const secret = "vertex-provider-secret-must-not-leak"
	err := NewHTTPError("vertexai", http.StatusTooManyRequests, strings.NewReader(`{"error":{"message":"Quota exceeded: `+secret+`"}}`), "45", "gemini-test")
	if err.Body != "quota_exhausted" || err.RetryAfter != 45*time.Second || err.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Body, secret) {
		t.Fatalf("provider body leaked: %#v", err)
	}
}

func TestNewHTTPErrorBoundsOversizedBody(t *testing.T) {
	const secret = "oversized-provider-secret-must-not-leak"
	body := strings.Repeat("x", maxProviderErrorBodyBytes+1) + secret
	err := NewHTTPError("vertexai", 500, strings.NewReader(body), "", "gemini-test")
	if err.Body != "provider_error,response_too_large" {
		t.Fatalf("classification = %q", err.Body)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Body, secret) {
		t.Fatalf("oversized provider body leaked: %#v", err)
	}
}

func TestNewHTTPErrorPreservesOnlyValidRateLimitRetryAfter(t *testing.T) {
	if got := NewHTTPError("vertexai", http.StatusTooManyRequests, strings.NewReader("rate limit"), "0", "model").RetryAfter; got != 0 {
		t.Fatalf("zero Retry-After = %s, want 0", got)
	}
	if got := NewHTTPError("vertexai", http.StatusInternalServerError, strings.NewReader("server error"), "60", "model").RetryAfter; got != 0 {
		t.Fatalf("non-429 Retry-After = %s, want 0", got)
	}
}

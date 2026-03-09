package codetool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateFetchURLSafetyRejectsLocalTargets(t *testing.T) {
	cases := []string{
		"http://localhost:8080",
		"http://127.0.0.1",
		"http://192.168.1.1",
		"http://10.0.0.1/admin",
		"file:///etc/passwd",
		"ftp://example.com",
		"http://metadata.google.internal/computeMetadata/v1/",
	}
	for _, raw := range cases {
		if err := ValidateFetchURLSafety(raw); err == nil {
			t.Fatalf("expected rejection for %s", raw)
		}
	}
}

func TestValidateFetchURLSafetyRejectsDNSRebinding(t *testing.T) {
	old := lookupHost
	lookupHost = func(string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	defer func() { lookupHost = old }()
	if err := ValidateFetchURLSafety("https://evil.example"); err == nil || !strings.Contains(err.Error(), "resolves to private IP") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateFetchURLSafetyAllowsExample(t *testing.T) {
	if err := ValidateFetchURLSafety("https://example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchURLReturnsContent(t *testing.T) {
	tool := FetchURL(func(_ context.Context, rawURL string) (string, error) { return "hello from " + rawURL, nil })
	got := call(t, tool, `{"url":"https://example.com"}`)
	if !strings.Contains(got, "hello from https://example.com") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestFetchURLEmptyURLRetry(t *testing.T) {
	tool := FetchURL(func(_ context.Context, _ string) (string, error) { return "", nil })
	if err := callErr(t, tool, `{"url":" "}`); err == nil {
		t.Fatal("expected retry error")
	}
}

func TestFetchURLTruncatesLargeContent(t *testing.T) {
	tool := FetchURL(func(_ context.Context, _ string) (string, error) {
		return strings.Repeat("a", maxFetchResponseBytes+10), nil
	})
	got := call(t, tool, `{"url":"https://example.com"}`)
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation notice")
	}
}

func TestFetchURLPropagatesFetchError(t *testing.T) {
	tool := FetchURL(func(_ context.Context, _ string) (string, error) { return "", errors.New("boom") })
	if err := callErr(t, tool, `{"url":"https://example.com"}`); err == nil || !strings.Contains(err.Error(), "fetch failed") {
		t.Fatalf("err=%v", err)
	}
}

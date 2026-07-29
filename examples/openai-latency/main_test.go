package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/auth/openai"
	oai "github.com/fugue-labs/gollem/provider/openai"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV("a, b ,c,")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitCSV = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitCSV[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseIntCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []int
	}{
		{"1,5,15", []int{1, 5, 15}},
		{" 3 , ,7", []int{3, 7}},
		{"", []int{1}},     // empty -> default single run
		{"0,-1", []int{1}}, // all non-positive -> default
	}
	for _, tc := range tests {
		got := parseIntCSV(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("parseIntCSV(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("parseIntCSV(%q)[%d] = %d, want %d", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("OPENAI_LATENCY_TEST_VAR", "set-value")
	if got := envOr("OPENAI_LATENCY_TEST_VAR", "def"); got != "set-value" {
		t.Errorf("envOr set var = %q, want set-value", got)
	}
	if got := envOr("OPENAI_LATENCY_UNSET_VAR", "def"); got != "def" {
		t.Errorf("envOr unset var = %q, want def", got)
	}
	t.Setenv("OPENAI_LATENCY_BLANK", "   ")
	if got := envOr("OPENAI_LATENCY_BLANK", "def"); got != "def" {
		t.Errorf("envOr blank var = %q, want def", got)
	}
}

func TestRedacted(t *testing.T) {
	if got := redacted(""); got != "<none>" {
		t.Errorf("redacted('') = %q, want <none>", got)
	}
	if got := redacted("ab"); got != "****" {
		t.Errorf("redacted short = %q, want ****", got)
	}
	// Longer account ids keep first 2 and last 2 chars with a separator.
	if got := redacted("abcdef"); got != "ab…ef" {
		t.Errorf("redacted('abcdef') = %q, want ab…ef", got)
	}
}

func TestBuildHistoryGrowsWithTurns(t *testing.T) {
	for _, turns := range []int{1, 5, 15} {
		msgs := buildHistory(turns)
		// Each turn contributes a request + response, plus a final request.
		if len(msgs) != turns*2+1 {
			t.Errorf("turns=%d: got %d messages, want %d", turns, len(msgs), turns*2+1)
		}
	}
	if msgs := buildHistory(0); len(msgs) != 1 {
		t.Errorf("turns=0: got %d messages, want 1", len(msgs))
	}
}

func TestCredentialHolder(t *testing.T) {
	initial := &openai.Credentials{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}
	h := &credentialHolder{creds: initial}
	if h.get() != initial {
		t.Fatal("get should return the initial creds")
	}
	rotated := &openai.Credentials{AccessToken: "b", ExpiresAt: time.Now().Add(2 * time.Hour)}
	h.set(rotated)
	if h.get() != rotated {
		t.Fatal("get should return the rotated creds after set")
	}
}

func TestMeasuredProviderRecordAndLast(t *testing.T) {
	mp := &measuredProvider{}
	if _, ok := mp.last(); ok {
		t.Fatal("fresh measuredProvider should have no trace")
	}
	tr := oai.RequestTrace{Transport: "http", Model: "gpt-5"}
	mp.record(tr)
	got, ok := mp.last()
	if !ok {
		t.Fatal("last should report a trace after record")
	}
	if got.Transport != "http" || got.Model != "gpt-5" {
		t.Errorf("last() = %+v, want the recorded trace", got)
	}
}

func TestLoadCredentialsFromPath(t *testing.T) {
	// Writing a credentials file and loading via the path exercises both
	// branches of loadCredentials without touching the real home dir.
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	creds := &openai.Credentials{
		AccessToken:  "at",
		RefreshToken: "rt",
		AccountID:    "acct",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := loadCredentials(path)
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if got.AccountID != "acct" {
		t.Errorf("AccountID = %q, want acct", got.AccountID)
	}
}

func TestLoadCredentialsMissingPathErrors(t *testing.T) {
	if _, err := loadCredentials(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error loading missing file")
	}
}

// TestBuildProviderWiresObserverAndRefresher verifies buildProvider constructs a
// provider whose observer and token refresher are wired and functional, without
// needing a live account. The refresher is exercised against non-expiring
// (no-op refresh) credentials.
func TestBuildProviderWiresObserverAndRefresher(t *testing.T) {
	h := &credentialHolder{creds: &openai.Credentials{
		AccessToken:  "tok",
		RefreshToken: "rt",
		AccountID:    "acct",
		// Far future expiry so RefreshIfNeeded is a no-op (returns same creds).
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}}
	mp, err := buildProvider("gpt-5", "http", h)
	if err != nil {
		t.Fatalf("buildProvider: %v", err)
	}
	if mp.provider == nil {
		t.Fatal("provider should be non-nil")
	}
	// The observer records into mp; drive a trace through it.
	mp.record(oai.RequestTrace{Transport: "http"})
	if _, ok := mp.last(); !ok {
		t.Fatal("observer not wired into measuredProvider")
	}
}

// TestRunOnceHTTPAndStreamErrors drives runOnce against a local fake ChatGPT
// backend for both the http and stream transports, and verifies a non-EOF
// stream error is propagated (not swallowed). This covers the request-driving
// switch and stream-error handling without real credentials.
func TestRunOnceHTTPAndStreamErrors(t *testing.T) {
	sse := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n" +
		"data: [DONE]\n\n"

	// successServer serves the happy-path SSE for the "http" transport.
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	}))
	defer successServer.Close()

	mp := newTestMeasuredProvider(t, successServer.URL+"/chatgpt.com")

	// HTTP transport success.
	if err := runOnce(context.Background(), mp, "http", 1); err != nil {
		t.Fatalf("runOnce http: %v", err)
	}
	if tr, ok := mp.last(); !ok || tr.TotalDuration <= 0 {
		t.Error("expected a recorded trace after a successful http run")
	}

	// Stream transport success.
	if err := runOnce(context.Background(), mp, "stream", 1); err != nil {
		t.Fatalf("runOnce stream: %v", err)
	}

	// Now point at a server that returns a 429 so RequestStream setup fails and
	// runOnce surfaces the error rather than treating it as completion.
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"rate_limit","message":"slow down"}}`, http.StatusTooManyRequests)
	}))
	defer errServer.Close()
	mpErr := newTestMeasuredProvider(t, errServer.URL+"/chatgpt.com")
	if err := runOnce(context.Background(), mpErr, "stream", 1); err == nil {
		t.Fatal("runOnce stream should propagate the setup error, not swallow it")
	}

	// Unknown transport returns an error.
	if err := runOnce(context.Background(), mp, "carrier-pigeon", 1); err == nil ||
		!strings.Contains(err.Error(), "carrier-pigeon") {
		t.Fatalf("runOnce unknown transport err = %v, want a carrier-pigeon error", err)
	}
}

func TestRunOnceRequestErrorPropagated(t *testing.T) {
	// "http" transport against a 500 should surface the error.
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"server_error","message":"boom"}}`, http.StatusInternalServerError)
	}))
	defer errServer.Close()
	mp := newTestMeasuredProvider(t, errServer.URL+"/chatgpt.com")
	if err := runOnce(context.Background(), mp, "http", 1); err == nil {
		t.Fatal("runOnce http should propagate the request error")
	}
}

// newTestMeasuredProvider builds a measuredProvider pointed at a fake ChatGPT
// backend URL. Constructing the struct first (then the provider) avoids the
// "cannot use mp.record while mp is being assigned" ordering problem.
func newTestMeasuredProvider(t *testing.T, baseURL string) *measuredProvider {
	t.Helper()
	mp := &measuredProvider{}
	mp.provider = oai.New(
		oai.WithChatGPTAuth("tok", "acct"),
		oai.WithBaseURL(baseURL),
		oai.WithModel("gpt-5"),
		oai.WithRequestObserver(mp.record),
	)
	return mp
}

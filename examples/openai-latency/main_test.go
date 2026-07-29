package main

import (
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

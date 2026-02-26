package main

import (
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestNormalizeTeamMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "auto"},
		{"auto", "auto"},
		{"ON", "on"},
		{"true", "on"},
		{"1", "on"},
		{"off", "off"},
		{"FALSE", "off"},
		{"0", "off"},
		{"nope", ""},
	}

	for _, tt := range tests {
		if got := normalizeTeamMode(tt.in); got != tt.want {
			t.Fatalf("normalizeTeamMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEffortAboveHigh(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"xhigh", true},
		{"XHIGH", true},
		{"high", false},
		{"medium", false},
		{"low", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := effortAboveHigh(tt.in); got != tt.want {
			t.Fatalf("effortAboveHigh(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecideTeamModeWithModel_Forced(t *testing.T) {
	enabled, reason := decideTeamModeWithModel("on", "", "simple prompt", 5*time.Minute, nil)
	if !enabled || !strings.Contains(reason, "forced") {
		t.Fatalf("expected forced-on team mode, got enabled=%v reason=%q", enabled, reason)
	}

	enabled, reason = decideTeamModeWithModel("off", "", "complex prompt", 2*time.Hour, nil)
	if enabled || !strings.Contains(reason, "forced") {
		t.Fatalf("expected forced-off team mode, got enabled=%v reason=%q", enabled, reason)
	}
}

func TestDecideTeamModeWithModel_AutoUsesLLMDecision(t *testing.T) {
	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"enable_team": true,
			"complexity_score": 9,
			"confidence": "high",
			"reasons": ["multi-step", "cross-file coordination"]
		}`),
	)

	enabled, reason := decideTeamModeWithModel("auto", "", "Fix tests", 15*time.Minute, model)
	if !enabled {
		t.Fatalf("expected team mode enabled by llm classifier, got disabled (reason=%q)", reason)
	}
	if !strings.Contains(reason, "llm score=9") {
		t.Fatalf("expected llm reason, got %q", reason)
	}
}

func TestDecideTeamModeWithModel_AutoFailsClosedOnClassifierFailure(t *testing.T) {
	// Structured-output call fails with text-only response and 1-request limit;
	// router should fail closed (team mode off).
	model := core.NewTestModel(core.TextResponse("not structured"))

	enabled, reason := decideTeamModeWithModel("auto", "", "tiny fix", 5*time.Minute, model)
	if enabled {
		t.Fatalf("expected team mode disabled on classifier failure, got enabled (reason=%q)", reason)
	}
	if !strings.Contains(reason, "llm-classifier-error") {
		t.Fatalf("expected llm classifier error marker in reason, got %q", reason)
	}
}

func TestDecideTeamModeWithModel_AutoNilModelFailsClosed(t *testing.T) {
	enabled, reason := decideTeamModeWithModel("auto", "", "complex task", 30*time.Minute, nil)
	if enabled {
		t.Fatalf("expected team mode disabled when model is nil, got enabled (reason=%q)", reason)
	}
	if !strings.Contains(reason, "llm-classifier-error") {
		t.Fatalf("expected llm classifier error marker in reason, got %q", reason)
	}
}

func TestDecideTeamModeWithModel_AutoLowConfidenceHighScoreOverridesOn(t *testing.T) {
	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"enable_team": false,
			"complexity_score": 8,
			"confidence": "low",
			"reasons": ["uncertain"]
		}`),
	)

	enabled, reason := decideTeamModeWithModel("auto", "", "complex task", 30*time.Minute, model)
	if !enabled {
		t.Fatalf("expected team mode enabled for score override, got disabled (reason=%q)", reason)
	}
	if !strings.Contains(reason, "score-override") {
		t.Fatalf("expected score-override marker in reason, got %q", reason)
	}
}

func TestDecideTeamModeWithModel_AutoLowConfidenceLowScoreFailsClosed(t *testing.T) {
	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{
			"enable_team": true,
			"complexity_score": 7,
			"confidence": "low",
			"reasons": ["uncertain"]
		}`),
	)

	enabled, reason := decideTeamModeWithModel("auto", "", "complex task", 30*time.Minute, model)
	if enabled {
		t.Fatalf("expected team mode disabled on low confidence with low score, got enabled (reason=%q)", reason)
	}
	if !strings.Contains(reason, "llm-low-confidence") {
		t.Fatalf("expected low-confidence marker in reason, got %q", reason)
	}
}

func TestSanitizeTeamModeLLMDecision(t *testing.T) {
	got := sanitizeTeamModeLLMDecision(teamModeLLMDecision{
		EnableTeam:      true,
		ComplexityScore: 42,
		Confidence:      "SURE",
		Reasons:         []string{"  first  ", "", strings.Repeat("x", 140)},
	})

	if got.ComplexityScore != 10 {
		t.Fatalf("complexity score = %d, want 10", got.ComplexityScore)
	}
	if got.Confidence != "low" {
		t.Fatalf("confidence = %q, want low", got.Confidence)
	}
	if len(got.Reasons) != 2 {
		t.Fatalf("reasons len = %d, want 2", len(got.Reasons))
	}
	if len(got.Reasons[1]) != 100 {
		t.Fatalf("second reason len = %d, want 100", len(got.Reasons[1]))
	}
}

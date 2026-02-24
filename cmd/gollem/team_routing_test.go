package main

import (
	"strings"
	"testing"
	"time"
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

func TestEvaluateTeamModeAutoFromSignals_ComplexTaskEnables(t *testing.T) {
	instruction := strings.Repeat(
		"Benchmark latency threshold cost model and distributed pipeline parallel microbatch scheduling.\n",
		50,
	)
	d := evaluateTeamModeAutoFromSignals(
		"Implement a batching scheduler with output_data deliverables",
		instruction,
		30*time.Minute,
		64,
	)

	if !d.Enabled {
		t.Fatalf("expected team mode to be enabled, got disabled (score=%d reasons=%v)", d.Score, d.Reasons)
	}
	if d.Score < 4 {
		t.Fatalf("expected high complexity score, got %d", d.Score)
	}
}

func TestEvaluateTeamModeAutoFromSignals_SimpleTaskDisables(t *testing.T) {
	d := evaluateTeamModeAutoFromSignals(
		"find my lost git changes and merge to master",
		"Fix a small issue.",
		10*time.Minute,
		8,
	)
	if d.Enabled {
		t.Fatalf("expected team mode disabled for simple short task (score=%d reasons=%v)", d.Score, d.Reasons)
	}
}

func TestEvaluateTeamModeAutoFromSignals_StrongSignalsCanEnable(t *testing.T) {
	instruction := strings.Repeat(
		"Repository merge branch history sanitize vulnerability deliverables output_data summary.csv.\n",
		60,
	)
	d := evaluateTeamModeAutoFromSignals(
		"multi-step repository rewrite and audit",
		instruction,
		15*time.Minute,
		140,
	)
	if !d.Enabled {
		t.Fatalf("expected strong-signal task to enable team mode (score=%d reasons=%v)", d.Score, d.Reasons)
	}
}

func TestDecideTeamMode_Forced(t *testing.T) {
	enabled, reason := decideTeamMode("on", "", "simple prompt", 5*time.Minute)
	if !enabled || !strings.Contains(reason, "forced") {
		t.Fatalf("expected forced-on team mode, got enabled=%v reason=%q", enabled, reason)
	}

	enabled, reason = decideTeamMode("off", "", "complex prompt", 2*time.Hour)
	if enabled || !strings.Contains(reason, "forced") {
		t.Fatalf("expected forced-off team mode, got enabled=%v reason=%q", enabled, reason)
	}
}

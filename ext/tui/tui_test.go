package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

func TestTUI_ModelCreation(t *testing.T) {
	theme := DefaultTheme()
	m := newModel(theme)

	// Verify initial state.
	if m.done {
		t.Error("expected done to be false initially")
	}
	if m.stepMode {
		t.Error("expected stepMode to be false initially")
	}
	if m.quitting {
		t.Error("expected quitting to be false initially")
	}
	if len(m.steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(m.steps))
	}
	if m.scroll != 0 {
		t.Errorf("expected scroll 0, got %d", m.scroll)
	}
	if m.width != 80 {
		t.Errorf("expected width 80, got %d", m.width)
	}
	if m.height != 24 {
		t.Errorf("expected height 24, got %d", m.height)
	}

	// Verify Init returns nil (no initial command).
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected Init to return nil cmd")
	}
}

func TestTUI_StepRendering(t *testing.T) {
	// Use an unstyled theme to make testing easier (no ANSI codes to parse).
	theme := Theme{
		System: lipgloss.NewStyle(),
		User:   lipgloss.NewStyle(),
		Model:  lipgloss.NewStyle(),
		Tool:   lipgloss.NewStyle(),
		Result: lipgloss.NewStyle(),
		Status: lipgloss.NewStyle(),
	}

	tests := []struct {
		name     string
		step     step
		contains string
	}{
		{
			name:     "system prompt",
			step:     step{kind: "system", content: "You are helpful."},
			contains: "[system] You are helpful.",
		},
		{
			name:     "user prompt",
			step:     step{kind: "user", content: "Tell me about Go."},
			contains: "[user] Tell me about Go.",
		},
		{
			name:     "model response",
			step:     step{kind: "model", content: "Go is a programming language."},
			contains: "[model] Go is a programming language.",
		},
		{
			name:     "tool call",
			step:     step{kind: "tool-call", content: "search({\"query\": \"Go lang\"})"},
			contains: "[tool] search",
		},
		{
			name:     "tool result",
			step:     step{kind: "tool-result", content: "search: found 10 results"},
			contains: "[result] search: found 10 results",
		},
		{
			name:     "error",
			step:     step{kind: "error", content: "something went wrong"},
			contains: "[error] something went wrong",
		},
		{
			name:     "done",
			step:     step{kind: "done", content: "completed"},
			contains: "[done] completed",
		},
		{
			name:     "checkpoint",
			step:     step{kind: "checkpoint", content: "snapshot.created"},
			contains: "[checkpoint] snapshot.created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderStep(tt.step, theme, 80)
			if !strings.Contains(rendered, tt.contains) {
				t.Errorf("rendered step %q does not contain %q\ngot: %q", tt.name, tt.contains, rendered)
			}
			if rendered == "" {
				t.Errorf("rendered step %q is empty", tt.name)
			}
		})
	}
}

func TestTUI_UsageDisplay(t *testing.T) {
	usage := core.RunUsage{
		Usage: core.Usage{
			InputTokens:  150,
			OutputTokens: 42,
		},
		Requests:  3,
		ToolCalls: 2,
	}

	formatted := FormatUsage(usage, "auto")

	if !strings.Contains(formatted, "150 in") {
		t.Errorf("expected input tokens in formatted output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "42 out") {
		t.Errorf("expected output tokens in formatted output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "requests: 3") {
		t.Errorf("expected request count in formatted output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "tools: 2") {
		t.Errorf("expected tool call count in formatted output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "mode: auto") {
		t.Errorf("expected mode in formatted output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "n/p:step") {
		t.Errorf("expected navigation help in formatted output, got: %s", formatted)
	}

	// Test step mode.
	stepFormatted := FormatUsage(usage, "step")
	if !strings.Contains(stepFormatted, "mode: step") {
		t.Errorf("expected step mode in formatted output, got: %s", stepFormatted)
	}

	statusFormatted := FormatTraceStatus(usage, "succeeded", "USD 0.010000", "1s", "auto")
	for _, want := range []string{"status: succeeded", "elapsed: 1s", "cost: USD 0.010000", "d:diverge"} {
		if !strings.Contains(statusFormatted, want) {
			t.Errorf("expected %q in formatted output, got: %s", want, statusFormatted)
		}
	}
}

func TestTUI_TraceNavigationKeys(t *testing.T) {
	m := newModel(DefaultTheme())
	m.height = 5
	m.steps = []step{
		{kind: "system", content: "start"},
		{kind: "checkpoint", content: "snapshot.created"},
		{kind: "model", content: "ok"},
		{kind: "error", content: "failed"},
		{kind: "done", content: "done"},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = updated.(model)
	if m.scroll != len(m.steps)-1 {
		t.Fatalf("g scroll = %d, want top", m.scroll)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(model)
	if m.scroll != len(m.steps)-2 {
		t.Fatalf("n scroll = %d, want one step forward", m.scroll)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = updated.(model)
	if m.scroll < 0 || m.scroll > len(m.steps)-1 {
		t.Fatalf("e produced invalid scroll = %d", m.scroll)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(model)
	if m.scroll < 0 || m.scroll > len(m.steps)-1 {
		t.Fatalf("c produced invalid scroll = %d", m.scroll)
	}

	m.steps = append(m.steps, step{kind: "system", content: "first divergence at event 3", detail: "first divergence detail"})
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if m.scroll < 0 || m.scroll > len(m.steps)-1 {
		t.Fatalf("d produced invalid scroll = %d", m.scroll)
	}

	m.scroll = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(model)
	if m.scroll != 1 {
		t.Fatalf("p scroll = %d, want 1", m.scroll)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(model)
	if m.scroll != 2 {
		t.Fatalf("left scroll = %d, want 2", m.scroll)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated.(model)
	if m.scroll != 0 {
		t.Fatalf("G scroll = %d, want 0", m.scroll)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 1})
	m = updated.(model)
	if m.width != 40 || m.height != 1 {
		t.Fatalf("window size not applied: %+v", m)
	}
	if got := scrollToFirstKind(m.steps, 0, "missing"); got != 0 {
		t.Fatalf("missing kind scroll = %d, want 0", got)
	}
	if got := scrollToFirstContent(m.steps, 0, ""); got != 0 {
		t.Fatalf("empty content scroll = %d, want 0", got)
	}
	if got := (model{}).selectedStepIndex(); got != 0 {
		t.Fatalf("empty selectedStepIndex = %d, want 0", got)
	}
}

func TestTUI_SplitDetailView(t *testing.T) {
	m := newModel(Theme{
		System: lipgloss.NewStyle(),
		User:   lipgloss.NewStyle(),
		Model:  lipgloss.NewStyle(),
		Tool:   lipgloss.NewStyle(),
		Result: lipgloss.NewStyle(),
		Status: lipgloss.NewStyle(),
	})
	m.width = 120
	m.height = 8
	m.steps = []step{
		{kind: "system", eventKind: "run.started", content: "#001 run.started", detail: "event: run.started\npayload:\n{}"},
		{kind: "tool-call", eventKind: "approval.requested", content: "#002 approval.requested tool=write", detail: "event: approval.requested\nreplay: recorded\ntool=write"},
	}

	view := m.View()
	for _, want := range []string{"Detail: approval.requested", "approval.requested", "tokens:"} {
		if !strings.Contains(view, want) {
			t.Fatalf("split view missing %q:\n%s", want, view)
		}
	}
}

func TestTUI_TraceDiffSteps(t *testing.T) {
	diff := traceutil.DiffResult{
		BaselineID:     "run-a",
		VariantID:      "run-b",
		BaselineStatus: "succeeded",
		VariantStatus:  "failed",
		FirstDivergence: &traceutil.Divergence{
			Index:         1,
			BaselineEvent: "002 model.responded",
			VariantEvent:  "002 model.failed",
		},
		UsageDelta: traceutil.UsageDelta{TotalTokens: 12, ToolCalls: 1},
		Narrative:  []string{"status changed from succeeded to failed"},
	}
	steps := traceDiffSteps(diff)
	if len(steps) < 3 {
		t.Fatalf("expected multiple diff steps, got %+v", steps)
	}
	if !strings.Contains(steps[0].content, "diff run-a -> run-b") {
		t.Fatalf("missing diff summary: %+v", steps[0])
	}
	if !strings.Contains(steps[1].content, "first divergence") {
		t.Fatalf("missing first divergence step: %+v", steps)
	}
}

func TestTUI_TraceDiffStepsIncludeCausalSemanticAndArtifactPanels(t *testing.T) {
	baseScore := 0.4
	variantScore := 0.9
	basePassed := false
	variantPassed := true
	diff := traceutil.DiffResult{
		BaselineID:     "run-a",
		VariantID:      "run-b",
		BaselineStatus: "succeeded",
		VariantStatus:  "succeeded",
		CausalDivergence: &traceutil.CausalDivergence{
			Index:    2,
			Baseline: "tool.called:shell",
			Variant:  "tool.called:go-test",
		},
		SemanticDelta: traceutil.SemanticDelta{
			Changed:            true,
			FinalOutputChanged: true,
			Notes:              []string{"final output changed"},
		},
		EvaluatorDelta: &traceutil.EvaluatorDelta{
			BaselineScore:  &baseScore,
			VariantScore:   &variantScore,
			ScoreDelta:     &variantScore,
			BaselinePassed: &basePassed,
			VariantPassed:  &variantPassed,
		},
		TopologyDelta:    []string{"planner-v1 -> planner-v2"},
		ArtifactDelta:    []string{"main.go -> main.go"},
		FinalOutputDelta: "old -> new",
	}
	steps := traceDiffSteps(diff)
	kinds := make(map[string]bool)
	for _, step := range steps {
		kinds[step.eventKind] = true
	}
	for _, want := range []string{"diff.causal", "diff.semantic", "diff.evaluator", "diff.topology_delta", "diff.artifact_delta", "diff.final_output"} {
		if !kinds[want] {
			t.Fatalf("missing %s in steps: %+v", want, steps)
		}
	}
	if detail := traceDiffDetail(diff); !strings.Contains(detail, "causal: boundary 3") || !strings.Contains(detail, "semantic: final output changed") {
		t.Fatalf("unexpected diff detail:\n%s", detail)
	}
}

func TestTUI_TraceStepsIncludeUsageAccumulation(t *testing.T) {
	artifact, err := traceutil.FromRunTrace(&core.RunTrace{
		RunID:   "run-usage",
		Success: true,
		Usage: core.RunUsage{
			Usage:    core.Usage{InputTokens: 10, OutputTokens: 5},
			Requests: 1,
		},
		Requests: []core.RequestTrace{
			{
				RequestID:  "req-1",
				TurnNumber: 1,
				Response: &core.RequestTraceResponse{
					Usage: core.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	traceutil.WithCost(artifact, &core.RunCost{TotalCost: 0.03, Currency: "USD"})

	steps := traceSteps(artifact)
	var found bool
	for _, step := range steps {
		if step.eventKind != "usage.accumulated" {
			continue
		}
		found = true
		if !strings.Contains(step.content, "tokens=15") || !strings.Contains(step.detail, "cost: USD 0.030000") {
			t.Fatalf("unexpected accumulation step: %+v", step)
		}
	}
	if !found {
		t.Fatalf("missing usage accumulation step: %+v", steps)
	}
}

func TestTUI_TraceStepsFallBackToSummaryUsageAccumulation(t *testing.T) {
	artifact := &traceutil.Artifact{
		SchemaVersion: traceutil.SchemaVersion,
		Run:           traceutil.RunMetadata{ID: "summary-only"},
		Summary: traceutil.Summary{
			Status: "succeeded",
			Usage: core.RunUsage{
				Usage:    core.Usage{InputTokens: 11, OutputTokens: 4},
				Requests: 1,
			},
			Cost: &core.RunCost{TotalCost: 0.02},
		},
	}
	steps := traceSteps(artifact)
	for _, step := range steps {
		if step.eventKind == "usage.accumulated" {
			if !strings.Contains(step.content, "tokens=15") || !strings.Contains(step.detail, "USD 0.020000") {
				t.Fatalf("unexpected summary accumulation step: %+v", step)
			}
			return
		}
	}
	t.Fatalf("missing summary accumulation step: %+v", steps)
}

func TestTUI_TraceEventStepsCoverRuntimeKinds(t *testing.T) {
	events := []traceutil.Event{
		{Seq: 1, Kind: "run.started"},
		{Seq: 2, Kind: "run.completed"},
		{Seq: 3, Kind: "run.failed", Payload: map[string]any{"error": "boom"}},
		{Seq: 4, Kind: "model.requested", Payload: map[string]any{"model": "test-model"}},
		{Seq: 5, Kind: "model.responded", Payload: map[string]any{"finish_reason": "stop"}},
		{Seq: 6, Kind: "tool.called", Payload: map[string]any{"tool_name": "shell"}},
		{Seq: 7, Kind: "tool.completed", Payload: map[string]any{"result": "ok"}},
		{Seq: 8, Kind: "snapshot.created", Payload: map[string]any{"snapshot_id": "snap-1"}},
		{Seq: 9, Kind: "checkpoint.created", Payload: map[string]any{"checkpoint_id": "snap-1"}},
		{Seq: 10, Kind: "approval.requested", Payload: map[string]any{"reason": "write file"}},
		{Seq: 11, Kind: "topology.transitioned", Payload: map[string]any{"to": "team"}},
		{Seq: 12, Kind: "artifact.changed", Payload: map[string]any{"data": map[string]any{"tool_name": "write"}}},
	}
	for _, event := range events {
		step := traceEventStep(event)
		if step.content == "" || step.detail == "" || step.eventKind != event.Kind {
			t.Fatalf("empty trace event step for %+v: %+v", event, step)
		}
	}
}

func TestTUI_TraceHelperFormattingAndUsageCoercion(t *testing.T) {
	usage, ok := eventUsage(traceutil.Event{Payload: map[string]any{
		"input_tokens":       "6",
		"output_tokens":      float64(4),
		"cache_read_tokens":  json.Number("2"),
		"cache_write_tokens": int64(1),
	}})
	if !ok || usage.InputTokens != 6 || usage.OutputTokens != 4 || usage.CacheReadTokens != 2 || usage.CacheWriteTokens != 1 {
		t.Fatalf("top-level event usage = %+v ok=%v", usage, ok)
	}
	nestedUsage, ok := eventUsage(traceutil.Event{Payload: map[string]any{
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 2},
	}})
	if !ok || nestedUsage.TotalTokens() != 3 {
		t.Fatalf("nested event usage = %+v ok=%v", nestedUsage, ok)
	}
	if _, ok := eventUsage(traceutil.Event{Payload: map[string]any{"usage": make(chan int)}}); ok {
		t.Fatal("expected unsupported usage payload to fail")
	}
	if got := payloadInt(map[string]any{"n": json.Number("9")}, "n"); got != 9 {
		t.Fatalf("payloadInt json.Number = %d, want 9", got)
	}
	if got := accumulatedCost(nil, 1, 2); got != "<unknown>" {
		t.Fatalf("nil accumulated cost = %q", got)
	}
	if got := accumulatedCost(&core.RunCost{TotalCost: 0.04, Currency: "EUR"}, 1, 4); got != "EUR 0.010000" {
		t.Fatalf("accumulated cost = %q", got)
	}
	if got := formatTraceCost(&core.RunCost{TotalCost: 0.5}); got != "USD 0.500000" {
		t.Fatalf("formatTraceCost = %q", got)
	}
	if got := formatTraceDuration(int64((2 * time.Second) / time.Millisecond)); got != "2s" {
		t.Fatalf("formatTraceDuration = %q", got)
	}
	if got := pointerValue((*float64)(nil)); got != "<nil>" {
		t.Fatalf("nil pointer value = %v", got)
	}
	score := 0.5
	passed := true
	if detail := evaluatorDiffDetail(&traceutil.EvaluatorDelta{BaselineScore: &score, VariantScore: &score, BaselinePassed: &passed, VariantPassed: &passed}); !strings.Contains(detail, "score: 0.5 -> 0.5") {
		t.Fatalf("unexpected evaluator detail:\n%s", detail)
	}
}

func TestTUI_ToolCallDisplay(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		argsJSON string
		contains string
	}{
		{
			name:     "simple tool call",
			toolName: "search",
			argsJSON: `{"query": "hello"}`,
			contains: `search({"query": "hello"})`,
		},
		{
			name:     "empty args",
			toolName: "get_time",
			argsJSON: "{}",
			contains: "get_time({})",
		},
		{
			name:     "long args truncated",
			toolName: "analyze",
			argsJSON: strings.Repeat("x", 300),
			contains: "analyze(" + strings.Repeat("x", 200) + "...)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := FormatToolCall(tt.toolName, tt.argsJSON)
			if !strings.Contains(formatted, tt.contains) {
				t.Errorf("FormatToolCall(%q, %q) = %q, expected to contain %q",
					tt.toolName, tt.argsJSON, formatted, tt.contains)
			}
		})
	}
}

func TestTUI_ExtractSteps(t *testing.T) {
	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.SystemPromptPart{Content: "Be helpful."},
				core.UserPromptPart{Content: "Hello"},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.TextPart{Content: "Hi there!"},
			},
		},
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.ToolReturnPart{ToolName: "search", Content: "results"},
			},
		},
		core.ModelResponse{
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{ToolName: "search", ArgsJSON: `{"q":"test"}`},
			},
		},
	}

	steps := ExtractSteps(messages)

	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	expected := []struct {
		kind    string
		content string
	}{
		{"system", "Be helpful."},
		{"user", "Hello"},
		{"model", "Hi there!"},
		{"tool-result", "search: results"},
		{"tool-call", "search"},
	}

	for i, exp := range expected {
		if steps[i].kind != exp.kind {
			t.Errorf("step %d: expected kind %q, got %q", i, exp.kind, steps[i].kind)
		}
		if !strings.Contains(steps[i].content, exp.content) {
			t.Errorf("step %d: expected content containing %q, got %q", i, exp.content, steps[i].content)
		}
	}
}

func TestTUI_UpdateStepMsg(t *testing.T) {
	m := newModel(DefaultTheme())

	s := step{kind: "model", content: "Hello!"}
	updated, _ := m.Update(stepMsg{step: s})
	um := updated.(model)

	if len(um.steps) != 1 {
		t.Fatalf("expected 1 step after update, got %d", len(um.steps))
	}
	if um.steps[0].kind != "model" {
		t.Errorf("expected step kind 'model', got %q", um.steps[0].kind)
	}
	if um.steps[0].content != "Hello!" {
		t.Errorf("expected step content 'Hello!', got %q", um.steps[0].content)
	}
	// Scroll should reset to 0 on new step.
	if um.scroll != 0 {
		t.Errorf("expected scroll 0 after new step, got %d", um.scroll)
	}
}

func TestTUI_UpdateDoneMsg(t *testing.T) {
	m := newModel(DefaultTheme())

	updated, cmd := m.Update(doneMsg{err: nil})
	um := updated.(model)

	if !um.done {
		t.Error("expected done to be true after doneMsg")
	}
	if um.err != nil {
		t.Errorf("expected nil error, got %v", um.err)
	}
	// doneMsg should cause a Quit command.
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Quit) after doneMsg")
	}
}

func TestTUI_ScrollBehavior(t *testing.T) {
	m := newModel(DefaultTheme())

	// Add some steps.
	for range 5 {
		updated, _ := m.Update(stepMsg{step: step{kind: "model", content: "msg"}})
		m = updated.(model)
	}

	if m.scroll != 0 {
		t.Errorf("expected scroll 0, got %d", m.scroll)
	}

	// Scroll up.
	updated, _ := m.Update(keyMsg("up"))
	m = updated.(model)
	if m.scroll != 1 {
		t.Errorf("expected scroll 1 after up, got %d", m.scroll)
	}

	// Scroll down.
	updated, _ = m.Update(keyMsg("down"))
	m = updated.(model)
	if m.scroll != 0 {
		t.Errorf("expected scroll 0 after down, got %d", m.scroll)
	}

	// Scroll down at bottom should not go below 0.
	updated, _ = m.Update(keyMsg("down"))
	m = updated.(model)
	if m.scroll != 0 {
		t.Errorf("expected scroll to stay at 0, got %d", m.scroll)
	}
}

func TestTUI_ModeToggle(t *testing.T) {
	m := newModel(DefaultTheme())

	if m.stepMode {
		t.Error("expected initial mode to be auto (stepMode=false)")
	}

	// Switch to step mode.
	updated, _ := m.Update(keyMsg("s"))
	m = updated.(model)
	if !m.stepMode {
		t.Error("expected stepMode=true after pressing 's'")
	}

	// Switch back to auto mode.
	updated, _ = m.Update(keyMsg("a"))
	m = updated.(model)
	if m.stepMode {
		t.Error("expected stepMode=false after pressing 'a'")
	}
}

func TestTUI_ViewOutput(t *testing.T) {
	m := newModel(Theme{
		System: lipgloss.NewStyle(),
		User:   lipgloss.NewStyle(),
		Model:  lipgloss.NewStyle(),
		Tool:   lipgloss.NewStyle(),
		Result: lipgloss.NewStyle(),
		Status: lipgloss.NewStyle(),
	})
	m.width = 80
	m.height = 24

	// Add a step.
	updated, _ := m.Update(stepMsg{step: step{kind: "user", content: "Hello world"}})
	m = updated.(model)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(view, "[user] Hello world") {
		t.Errorf("view should contain user message, got: %q", view)
	}
	if !strings.Contains(view, "tokens:") {
		t.Errorf("view should contain status bar with tokens, got: %q", view)
	}
}

func TestTUI_DefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	// Just verify the theme fields are non-zero (styles were created).
	// We use renderStep to verify styles are functional.
	rendered := renderStep(step{kind: "system", content: "test"}, theme, 80)
	if rendered == "" {
		t.Error("DefaultTheme produced empty render for system step")
	}
	rendered = renderStep(step{kind: "user", content: "test"}, theme, 80)
	if rendered == "" {
		t.Error("DefaultTheme produced empty render for user step")
	}
	rendered = renderStep(step{kind: "model", content: "test"}, theme, 80)
	if rendered == "" {
		t.Error("DefaultTheme produced empty render for model step")
	}
	rendered = renderStep(step{kind: "tool-call", content: "test"}, theme, 80)
	if rendered == "" {
		t.Error("DefaultTheme produced empty render for tool step")
	}
	rendered = renderStep(step{kind: "tool-result", content: "test"}, theme, 80)
	if rendered == "" {
		t.Error("DefaultTheme produced empty render for result step")
	}
}

func TestTUI_WithTheme(t *testing.T) {
	customTheme := Theme{
		System: lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		User:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Model:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Tool:   lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
		Result: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		Status: lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	}

	cfg := &config{theme: DefaultTheme()}
	opt := WithTheme(customTheme)
	opt(cfg)

	// Verify the theme was applied (by rendering with it).
	rendered := renderStep(step{kind: "system", content: "test"}, cfg.theme, 80)
	if rendered == "" {
		t.Error("WithTheme produced empty render")
	}
}

// keyMsg is a helper to create tea.KeyMsg for testing.
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

// TraceView opens a terminal viewer for a recorded Gollem trace artifact.
func TraceView(artifact *traceutil.Artifact, opts ...Option) error {
	cfg := &config{
		theme: DefaultTheme(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	m := newModel(cfg.theme)
	m.steps = traceSteps(artifact)
	if artifact != nil {
		m.usage = artifact.Summary.Usage
		m.status = artifact.Summary.Status
		m.cost = formatTraceCost(artifact.Summary.Cost)
		m.elapsed = formatTraceDuration(artifact.Summary.DurationMillis)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("trace TUI error: %w", err)
	}
	return nil
}

// TraceCompareView opens a terminal viewer for a baseline-vs-variant trace diff.
func TraceCompareView(baseline, variant *traceutil.Artifact, opts ...Option) error {
	cfg := &config{
		theme: DefaultTheme(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	diff := traceutil.Diff(baseline, variant)
	m := newModel(cfg.theme)
	m.steps = traceDiffSteps(diff)
	if variant != nil {
		m.usage = variant.Summary.Usage
	}
	m.status = fmt.Sprintf("diff %s -> %s", diff.BaselineStatus, diff.VariantStatus)
	m.cost = formatCostDelta(diff.CostDelta)
	m.elapsed = formatMillisDelta(diff.DurationDeltaMillis)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("trace compare TUI error: %w", err)
	}
	return nil
}

func traceSteps(artifact *traceutil.Artifact) []step {
	if artifact == nil {
		return []step{{kind: "error", content: "nil trace artifact"}}
	}
	steps := make([]step, 0, len(artifact.Events)+len(artifact.Events)/2+2)
	for _, event := range artifact.Events {
		steps = append(steps, traceEventStep(event))
	}
	steps = append(steps, traceAccumulationSteps(artifact)...)
	if artifact.Summary.Status != "" {
		steps = append(steps, step{kind: "done", content: fmt.Sprintf("trace %s (%d events)", artifact.Summary.Status, len(artifact.Events))})
	}
	return steps
}

func traceAccumulationSteps(artifact *traceutil.Artifact) []step {
	if artifact == nil {
		return nil
	}
	var usage core.RunUsage
	var steps []step
	totalTokens := artifact.Summary.Usage.TotalTokens()
	for _, event := range artifact.Events {
		if event.Kind != "model.responded" {
			continue
		}
		eventUsage, ok := eventUsage(event)
		if !ok {
			continue
		}
		usage.IncrRequest(eventUsage)
		cost := accumulatedCost(artifact.Summary.Cost, usage.TotalTokens(), totalTokens)
		content := fmt.Sprintf(
			"usage accumulation step=%d tokens=%d cost=%s",
			event.Step,
			usage.TotalTokens(),
			cost,
		)
		detail := fmt.Sprintf(
			"step: %d\nrequests: %d\ninput_tokens: %d\noutput_tokens: %d\ncache_read_tokens: %d\ncache_write_tokens: %d\ntotal_tokens: %d\ncost: %s",
			event.Step,
			usage.Requests,
			usage.InputTokens,
			usage.OutputTokens,
			usage.CacheReadTokens,
			usage.CacheWriteTokens,
			usage.TotalTokens(),
			cost,
		)
		steps = append(steps, step{kind: "system", eventKind: "usage.accumulated", content: content, detail: detail})
	}
	if len(steps) == 0 && artifact.Summary.Usage.TotalTokens() > 0 {
		cost := formatTraceCost(artifact.Summary.Cost)
		if cost == "" {
			cost = "<unknown>"
		}
		detail := fmt.Sprintf(
			"requests: %d\ninput_tokens: %d\noutput_tokens: %d\ncache_read_tokens: %d\ncache_write_tokens: %d\ntotal_tokens: %d\ncost: %s",
			artifact.Summary.Usage.Requests,
			artifact.Summary.Usage.InputTokens,
			artifact.Summary.Usage.OutputTokens,
			artifact.Summary.Usage.CacheReadTokens,
			artifact.Summary.Usage.CacheWriteTokens,
			artifact.Summary.Usage.TotalTokens(),
			cost,
		)
		steps = append(steps, step{
			kind:      "system",
			eventKind: "usage.accumulated",
			content:   fmt.Sprintf("usage accumulation final tokens=%d cost=%s", artifact.Summary.Usage.TotalTokens(), cost),
			detail:    detail,
		})
	}
	return steps
}

func traceDiffSteps(diff traceutil.DiffResult) []step {
	steps := []step{
		{
			kind:      "system",
			eventKind: "diff.summary",
			content: fmt.Sprintf(
				"diff %s -> %s events=%+d tokens=%+d tools=%+d",
				diff.BaselineID,
				diff.VariantID,
				diff.EventDelta,
				diff.UsageDelta.TotalTokens,
				diff.UsageDelta.ToolCalls,
			),
			detail: traceDiffDetail(diff),
		},
	}
	if diff.FirstDivergence == nil {
		steps = append(steps, step{
			kind:      "done",
			eventKind: "diff.first_divergence",
			content:   "first divergence: none",
			detail:    "No event-level divergence was detected.",
		})
	} else {
		steps = append(steps, step{
			kind:      "error",
			eventKind: "diff.first_divergence",
			content:   fmt.Sprintf("first divergence at event %d", diff.FirstDivergence.Index+1),
			detail: fmt.Sprintf(
				"baseline: %s\nvariant:  %s",
				nonEmpty(diff.FirstDivergence.BaselineEvent, "<missing>"),
				nonEmpty(diff.FirstDivergence.VariantEvent, "<missing>"),
			),
		})
	}
	if diff.CausalDivergence != nil {
		steps = append(steps, step{
			kind:      "system",
			eventKind: "diff.causal",
			content:   fmt.Sprintf("causal divergence at boundary %d", diff.CausalDivergence.Index+1),
			detail: fmt.Sprintf(
				"baseline: %s\nvariant:  %s",
				nonEmpty(diff.CausalDivergence.Baseline, "<missing>"),
				nonEmpty(diff.CausalDivergence.Variant, "<missing>"),
			),
		})
	}
	if diff.SemanticDelta.Changed {
		steps = append(steps, step{
			kind:      "model",
			eventKind: "diff.semantic",
			content:   "semantic delta",
			detail:    strings.Join(diff.SemanticDelta.Notes, "\n"),
		})
	}
	for _, line := range diff.Narrative {
		steps = append(steps, step{kind: "system", eventKind: "diff.narrative", content: line, detail: line})
	}
	if diff.EvaluatorDelta != nil {
		steps = append(steps, step{kind: "system", eventKind: "diff.evaluator", content: "evaluator delta", detail: evaluatorDiffDetail(diff.EvaluatorDelta)})
	}
	appendDeltaLines := func(kind, title string, lines []string) {
		if len(lines) == 0 {
			return
		}
		steps = append(steps, step{kind: kind, eventKind: "diff." + strings.ReplaceAll(title, " ", "_"), content: title, detail: strings.Join(lines, "\n")})
	}
	appendDeltaLines("system", "topology delta", diff.TopologyDelta)
	appendDeltaLines("tool-result", "artifact delta", diff.ArtifactDelta)
	if diff.FinalOutputDelta != "" {
		steps = append(steps, step{kind: "model", eventKind: "diff.final_output", content: "final output changed", detail: diff.FinalOutputDelta})
	}
	steps = append(steps, step{kind: "done", eventKind: "diff.done", content: fmt.Sprintf("diff complete (%s -> %s)", diff.BaselineStatus, diff.VariantStatus), detail: traceDiffDetail(diff)})
	return steps
}

func traceEventStep(event traceutil.Event) step {
	detail := traceEventDetail(event.Kind, event.Seq, event.ReplayPolicy, event.Step, event.RequestID, event.Payload)
	switch event.Kind {
	case "run.started":
		return step{kind: "system", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s", event.Seq, event.Kind), detail: detail}
	case "run.completed":
		return step{kind: "done", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s", event.Seq, event.Kind), detail: detail}
	case "run.failed", "model.failed", "tool.failed":
		return step{kind: "error", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "model.requested":
		return step{kind: "user", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "model.responded":
		return step{kind: "model", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "tool.called":
		return step{kind: "tool-call", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "tool.completed":
		return step{kind: "tool-result", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "snapshot.created":
		return step{kind: "checkpoint", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "checkpoint.created":
		return step{kind: "checkpoint", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "approval.requested", "approval.resolved", "wait.started", "wait.resolved", "deferred.requested", "deferred.resolved":
		return step{kind: "tool-call", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	case "topology.transitioned":
		return step{kind: "system", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	default:
		return step{kind: "system", eventKind: event.Kind, content: fmt.Sprintf("#%03d %s %s", event.Seq, event.Kind, tracePayloadSummary(event)), detail: detail}
	}
}

func tracePayloadSummary(event traceutil.Event) string {
	if len(event.Payload) == 0 {
		return ""
	}
	if value, ok := event.Payload["model"]; ok {
		return fmt.Sprintf("model=%v", value)
	}
	if value, ok := event.Payload["error"]; ok {
		return fmt.Sprintf("error=%v", value)
	}
	if value, ok := event.Payload["tool_name"]; ok {
		return fmt.Sprintf("tool=%v", value)
	}
	if value, ok := event.Payload["reason"]; ok {
		return fmt.Sprintf("reason=%v", value)
	}
	if data, ok := event.Payload["data"].(map[string]any); ok {
		if tool, ok := data["tool_name"]; ok {
			return fmt.Sprintf("tool=%v", tool)
		}
	}
	return fmt.Sprintf("%v", event.Payload)
}

func eventUsage(event traceutil.Event) (core.Usage, bool) {
	raw, ok := event.Payload["usage"]
	if !ok || raw == nil {
		usage := core.Usage{
			InputTokens:      payloadInt(event.Payload, "input_tokens"),
			OutputTokens:     payloadInt(event.Payload, "output_tokens"),
			CacheReadTokens:  payloadInt(event.Payload, "cache_read_tokens"),
			CacheWriteTokens: payloadInt(event.Payload, "cache_write_tokens"),
		}
		return usage, usage.TotalTokens() > 0 || usage.CacheReadTokens > 0 || usage.CacheWriteTokens > 0
	}
	switch rawUsage := raw.(type) {
	case core.Usage:
		return rawUsage, true
	case map[string]any:
		return core.Usage{
			InputTokens:      payloadInt(rawUsage, "input_tokens"),
			OutputTokens:     payloadInt(rawUsage, "output_tokens"),
			CacheReadTokens:  payloadInt(rawUsage, "cache_read_tokens"),
			CacheWriteTokens: payloadInt(rawUsage, "cache_write_tokens"),
		}, true
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return core.Usage{}, false
		}
		var decoded core.Usage
		if err := json.Unmarshal(data, &decoded); err != nil {
			return core.Usage{}, false
		}
		return decoded, true
	}
}

func payloadInt(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		var n int
		_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &n)
		return n
	default:
		return 0
	}
}

func accumulatedCost(cost *core.RunCost, cumulativeTokens, totalTokens int) string {
	if cost == nil || cost.TotalCost == 0 || cumulativeTokens <= 0 || totalTokens <= 0 {
		return "<unknown>"
	}
	currency := cost.Currency
	if currency == "" {
		currency = "USD"
	}
	value := cost.TotalCost * (float64(cumulativeTokens) / float64(totalTokens))
	return fmt.Sprintf("%s %.6f", currency, value)
}

func traceDiffDetail(diff traceutil.DiffResult) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "baseline: %s (%s)\n", diff.BaselineID, diff.BaselineStatus)
	fmt.Fprintf(&b, "variant:  %s (%s)\n", diff.VariantID, diff.VariantStatus)
	fmt.Fprintf(&b, "events: %+d\nsnapshots: %+d\n", diff.EventDelta, diff.SnapshotDelta)
	fmt.Fprintf(&b, "duration: %s\ncost: %s\n", formatMillisDelta(diff.DurationDeltaMillis), formatCostDelta(diff.CostDelta))
	fmt.Fprintf(&b, "tokens: input=%+d output=%+d total=%+d\n", diff.UsageDelta.InputTokens, diff.UsageDelta.OutputTokens, diff.UsageDelta.TotalTokens)
	fmt.Fprintf(&b, "requests: %+d\ntools: %+d\n", diff.UsageDelta.Requests, diff.UsageDelta.ToolCalls)
	fmt.Fprintf(&b, "retry/error: retries=%+d errors=%+d failures=%+d\n", diff.RetryErrorDelta.RetryScheduled, diff.RetryErrorDelta.ErrorsRaised, diff.RetryErrorDelta.Failures)
	if diff.CausalDivergence != nil {
		fmt.Fprintf(&b, "causal: boundary %d\n", diff.CausalDivergence.Index+1)
	}
	if diff.SemanticDelta.Changed {
		fmt.Fprintf(&b, "semantic: %s\n", strings.Join(diff.SemanticDelta.Notes, "; "))
	}
	return b.String()
}

func evaluatorDiffDetail(delta *traceutil.EvaluatorDelta) string {
	if delta == nil {
		return ""
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "score: %v -> %v", pointerValue(delta.BaselineScore), pointerValue(delta.VariantScore))
	if delta.ScoreDelta != nil {
		fmt.Fprintf(&b, " (%+0.4f)", *delta.ScoreDelta)
	}
	fmt.Fprintf(&b, "\npassed: %v -> %v\n", pointerValue(delta.BaselinePassed), pointerValue(delta.VariantPassed))
	return b.String()
}

func pointerValue[T any](value *T) any {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func formatTraceCost(cost *core.RunCost) string {
	if cost == nil || cost.TotalCost == 0 {
		return ""
	}
	currency := cost.Currency
	if currency == "" {
		currency = "USD"
	}
	return fmt.Sprintf("%s %.6f", currency, cost.TotalCost)
}

func formatCostDelta(delta float64) string {
	if delta == 0 {
		return ""
	}
	return fmt.Sprintf("%+0.6f", delta)
}

func formatTraceDuration(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return (time.Duration(ms) * time.Millisecond).String()
}

func formatMillisDelta(delta int64) string {
	if delta == 0 {
		return ""
	}
	return (time.Duration(delta) * time.Millisecond).String()
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

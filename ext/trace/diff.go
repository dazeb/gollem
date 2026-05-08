package trace

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// DiffResult describes causal and quantitative differences between two traces.
type DiffResult struct {
	BaselineID          string            `json:"baseline_id"`
	VariantID           string            `json:"variant_id"`
	FirstDivergence     *Divergence       `json:"first_divergence,omitempty"`
	CausalDivergence    *CausalDivergence `json:"causal_divergence,omitempty"`
	CausalGraphDelta    *CausalGraphDelta `json:"causal_graph_delta,omitempty"`
	PathDivergence      PathDelta         `json:"path_divergence"`
	SemanticDelta       SemanticDelta     `json:"semantic_delta"`
	UsageDelta          UsageDelta        `json:"usage_delta"`
	KindDelta           map[string]int    `json:"kind_delta,omitempty"`
	TopologyDelta       []string          `json:"topology_delta,omitempty"`
	EvaluatorDelta      *EvaluatorDelta   `json:"evaluator_delta,omitempty"`
	ArtifactDelta       []string          `json:"artifact_delta,omitempty"`
	FinalOutputDelta    string            `json:"final_output_delta,omitempty"`
	RetryErrorDelta     RetryErrorDelta   `json:"retry_error_delta"`
	EventDelta          int               `json:"event_delta"`
	SnapshotDelta       int               `json:"snapshot_delta"`
	DurationDeltaMillis int64             `json:"duration_delta_ms"`
	CostDelta           float64           `json:"cost_delta,omitempty"`
	BaselineStatus      string            `json:"baseline_status"`
	VariantStatus       string            `json:"variant_status"`
	BaselineError       string            `json:"baseline_error,omitempty"`
	VariantError        string            `json:"variant_error,omitempty"`
	Narrative           []string          `json:"narrative,omitempty"`
}

// CausalDivergence identifies the first mismatch in the causal boundary path.
type CausalDivergence struct {
	Index    int    `json:"index"`
	Baseline string `json:"baseline,omitempty"`
	Variant  string `json:"variant,omitempty"`
}

// CausalGraphDelta compares inferred event-to-event causal edges rather than
// only the ordered boundary path.
type CausalGraphDelta struct {
	BaselineNodes       int               `json:"baseline_nodes"`
	VariantNodes        int               `json:"variant_nodes"`
	BaselineEdges       int               `json:"baseline_edges"`
	VariantEdges        int               `json:"variant_edges"`
	NodeDelta           int               `json:"node_delta"`
	EdgeDelta           int               `json:"edge_delta"`
	FirstEdgeDivergence *CausalDivergence `json:"first_edge_divergence,omitempty"`
	BaselineOnlyEdges   []string          `json:"baseline_only_edges,omitempty"`
	VariantOnlyEdges    []string          `json:"variant_only_edges,omitempty"`
}

// PathDelta summarizes the causal event path on both sides of a diff.
type PathDelta struct {
	Baseline []string `json:"baseline,omitempty"`
	Variant  []string `json:"variant,omitempty"`
}

// SemanticDelta compares user-observable run semantics beyond raw event order.
type SemanticDelta struct {
	Changed             bool     `json:"changed"`
	StatusChanged       bool     `json:"status_changed,omitempty"`
	FinalOutputChanged  bool     `json:"final_output_changed,omitempty"`
	ToolSequenceChanged bool     `json:"tool_sequence_changed,omitempty"`
	EvaluatorChanged    bool     `json:"evaluator_changed,omitempty"`
	BaselineOutput      string   `json:"baseline_output,omitempty"`
	VariantOutput       string   `json:"variant_output,omitempty"`
	BaselineTools       []string `json:"baseline_tools,omitempty"`
	VariantTools        []string `json:"variant_tools,omitempty"`
	Notes               []string `json:"notes,omitempty"`
}

// EvaluatorDelta compares aggregate evaluator output when both artifacts carry it.
type EvaluatorDelta struct {
	BaselineScore  *float64 `json:"baseline_score,omitempty"`
	VariantScore   *float64 `json:"variant_score,omitempty"`
	ScoreDelta     *float64 `json:"score_delta,omitempty"`
	BaselinePassed *bool    `json:"baseline_passed,omitempty"`
	VariantPassed  *bool    `json:"variant_passed,omitempty"`
}

// RetryErrorDelta compares retry and error boundaries.
type RetryErrorDelta struct {
	RetryScheduled int `json:"retry_scheduled"`
	ErrorsRaised   int `json:"errors_raised"`
	Failures       int `json:"failures"`
}

// Divergence identifies the first event-level mismatch.
type Divergence struct {
	Index         int    `json:"index"`
	BaselineSeq   int    `json:"baseline_seq,omitempty"`
	VariantSeq    int    `json:"variant_seq,omitempty"`
	BaselineKind  string `json:"baseline_kind,omitempty"`
	VariantKind   string `json:"variant_kind,omitempty"`
	BaselineEvent string `json:"baseline_event,omitempty"`
	VariantEvent  string `json:"variant_event,omitempty"`
}

// UsageDelta captures variant minus baseline usage.
type UsageDelta struct {
	Requests         int `json:"requests"`
	ToolCalls        int `json:"tool_calls"`
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Diff compares two trace artifacts.
func Diff(baseline, variant *Artifact) DiffResult {
	baselinePath := causalPath(baseline.Events)
	variantPath := causalPath(variant.Events)
	baselineGraph := buildCausalGraph(baseline.Events)
	variantGraph := buildCausalGraph(variant.Events)
	result := DiffResult{
		BaselineID:     displayRunID(baseline),
		VariantID:      displayRunID(variant),
		BaselineStatus: baseline.Summary.Status,
		VariantStatus:  variant.Summary.Status,
		BaselineError:  baseline.Summary.Error,
		VariantError:   variant.Summary.Error,
		EventDelta:     len(variant.Events) - len(baseline.Events),
		SnapshotDelta:  len(variant.Snapshots) - len(baseline.Snapshots),
		DurationDeltaMillis: variant.Summary.DurationMillis -
			baseline.Summary.DurationMillis,
		CostDelta: costTotal(variant.Summary.Cost) - costTotal(baseline.Summary.Cost),
		PathDivergence: PathDelta{
			Baseline: baselinePath,
			Variant:  variantPath,
		},
		CausalDivergence: firstCausalDivergence(baselinePath, variantPath),
		CausalGraphDelta: causalGraphDelta(baselineGraph, variantGraph),
		SemanticDelta:    semanticDelta(baseline, variant),
		KindDelta:        kindDelta(baseline.Events, variant.Events),
		TopologyDelta:    boundaryDelta("topology.transitioned", baseline.Events, variant.Events),
		EvaluatorDelta:   evaluatorDelta(baseline.Summary.Evaluator, variant.Summary.Evaluator),
		ArtifactDelta:    boundaryDelta("artifact.changed", baseline.Events, variant.Events),
		FinalOutputDelta: finalOutputDelta(baseline, variant),
		RetryErrorDelta:  retryErrorDelta(baseline.Events, variant.Events),
		UsageDelta: UsageDelta{
			Requests:         variant.Summary.Usage.Requests - baseline.Summary.Usage.Requests,
			ToolCalls:        variant.Summary.Usage.ToolCalls - baseline.Summary.Usage.ToolCalls,
			InputTokens:      variant.Summary.Usage.InputTokens - baseline.Summary.Usage.InputTokens,
			OutputTokens:     variant.Summary.Usage.OutputTokens - baseline.Summary.Usage.OutputTokens,
			CacheReadTokens:  variant.Summary.Usage.CacheReadTokens - baseline.Summary.Usage.CacheReadTokens,
			CacheWriteTokens: variant.Summary.Usage.CacheWriteTokens - baseline.Summary.Usage.CacheWriteTokens,
			TotalTokens:      variant.Summary.Usage.TotalTokens() - baseline.Summary.Usage.TotalTokens(),
		},
	}

	max := len(baseline.Events)
	if len(variant.Events) < max {
		max = len(variant.Events)
	}
	for i := range max {
		left := eventFingerprint(baseline.Events[i])
		right := eventFingerprint(variant.Events[i])
		if left != right {
			result.FirstDivergence = divergenceAt(i, &baseline.Events[i], &variant.Events[i])
			result.Narrative = buildDiffNarrative(result)
			return result
		}
	}
	if len(baseline.Events) != len(variant.Events) {
		var left, right *Event
		if len(baseline.Events) > max {
			left = &baseline.Events[max]
		}
		if len(variant.Events) > max {
			right = &variant.Events[max]
		}
		result.FirstDivergence = divergenceAt(max, left, right)
	}
	result.Narrative = buildDiffNarrative(result)
	return result
}

// WriteDiff writes a human-readable trace diff summary.
func WriteDiff(w io.Writer, diff DiffResult) error {
	fmt.Fprintf(w, "Trace diff\n")
	fmt.Fprintf(w, "baseline: %s (%s)\n", diff.BaselineID, diff.BaselineStatus)
	fmt.Fprintf(w, "variant:  %s (%s)\n", diff.VariantID, diff.VariantStatus)
	if diff.FirstDivergence == nil {
		fmt.Fprintln(w, "first divergence: none")
	} else {
		fmt.Fprintf(w, "first divergence: event %d\n", diff.FirstDivergence.Index+1)
		if diff.FirstDivergence.BaselineKind != "" {
			fmt.Fprintf(w, "  baseline: %s\n", diff.FirstDivergence.BaselineEvent)
		} else {
			fmt.Fprintln(w, "  baseline: <missing>")
		}
		if diff.FirstDivergence.VariantKind != "" {
			fmt.Fprintf(w, "  variant:  %s\n", diff.FirstDivergence.VariantEvent)
		} else {
			fmt.Fprintln(w, "  variant:  <missing>")
		}
	}
	if diff.CausalDivergence == nil {
		fmt.Fprintln(w, "causal divergence: none")
	} else {
		fmt.Fprintf(w, "causal divergence: path %d\n", diff.CausalDivergence.Index+1)
		fmt.Fprintf(w, "  baseline: %s\n", nonEmpty(diff.CausalDivergence.Baseline, "<missing>"))
		fmt.Fprintf(w, "  variant:  %s\n", nonEmpty(diff.CausalDivergence.Variant, "<missing>"))
	}
	if diff.CausalGraphDelta != nil {
		fmt.Fprintf(w, "causal graph: nodes=%d->%d edges=%d->%d edge_delta=%+d\n",
			diff.CausalGraphDelta.BaselineNodes,
			diff.CausalGraphDelta.VariantNodes,
			diff.CausalGraphDelta.BaselineEdges,
			diff.CausalGraphDelta.VariantEdges,
			diff.CausalGraphDelta.EdgeDelta,
		)
		if diff.CausalGraphDelta.FirstEdgeDivergence != nil {
			fmt.Fprintf(w, "  first edge divergence: %s -> %s\n",
				nonEmpty(diff.CausalGraphDelta.FirstEdgeDivergence.Baseline, "<missing>"),
				nonEmpty(diff.CausalGraphDelta.FirstEdgeDivergence.Variant, "<missing>"),
			)
		}
	}
	if diff.SemanticDelta.Changed {
		fmt.Fprintln(w, "semantic delta:")
		for _, line := range diff.SemanticDelta.Notes {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	fmt.Fprintf(w, "event delta: %+d\n", diff.EventDelta)
	fmt.Fprintf(w, "snapshot delta: %+d\n", diff.SnapshotDelta)
	fmt.Fprintf(w, "duration delta: %s\n", formatMillisDelta(diff.DurationDeltaMillis))
	if diff.CostDelta != 0 {
		fmt.Fprintf(w, "cost delta: %+0.6f\n", diff.CostDelta)
	}
	fmt.Fprintf(w, "request delta: %+d\n", diff.UsageDelta.Requests)
	fmt.Fprintf(w, "tool delta: %+d\n", diff.UsageDelta.ToolCalls)
	fmt.Fprintf(w, "token delta: input=%+d output=%+d total=%+d cache_read=%+d cache_write=%+d\n",
		diff.UsageDelta.InputTokens,
		diff.UsageDelta.OutputTokens,
		diff.UsageDelta.TotalTokens,
		diff.UsageDelta.CacheReadTokens,
		diff.UsageDelta.CacheWriteTokens,
	)
	fmt.Fprintf(w, "retry/error delta: retries=%+d errors=%+d failures=%+d\n",
		diff.RetryErrorDelta.RetryScheduled,
		diff.RetryErrorDelta.ErrorsRaised,
		diff.RetryErrorDelta.Failures,
	)
	if diff.EvaluatorDelta != nil {
		fmt.Fprintln(w, "evaluator:")
		if diff.EvaluatorDelta.ScoreDelta != nil {
			fmt.Fprintf(w, "  score delta: %+0.4f\n", *diff.EvaluatorDelta.ScoreDelta)
		}
		if diff.EvaluatorDelta.BaselinePassed != nil || diff.EvaluatorDelta.VariantPassed != nil {
			fmt.Fprintf(w, "  passed: %v -> %v\n", boolPointerValue(diff.EvaluatorDelta.BaselinePassed), boolPointerValue(diff.EvaluatorDelta.VariantPassed))
		}
	}
	if len(diff.TopologyDelta) > 0 {
		fmt.Fprintln(w, "topology delta:")
		for _, line := range diff.TopologyDelta {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	if len(diff.ArtifactDelta) > 0 {
		fmt.Fprintln(w, "artifact delta:")
		for _, line := range diff.ArtifactDelta {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	if diff.FinalOutputDelta != "" {
		fmt.Fprintf(w, "final output delta: %s\n", diff.FinalOutputDelta)
	}
	if len(diff.Narrative) > 0 {
		fmt.Fprintln(w, "outcome:")
		for _, line := range diff.Narrative {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	return nil
}

func divergenceAt(index int, baseline, variant *Event) *Divergence {
	d := &Divergence{Index: index}
	if baseline != nil {
		d.BaselineSeq = baseline.Seq
		d.BaselineKind = baseline.Kind
		d.BaselineEvent = fmt.Sprintf("%03d %-18s %s", baseline.Seq, baseline.Kind, eventSummary(*baseline))
	}
	if variant != nil {
		d.VariantSeq = variant.Seq
		d.VariantKind = variant.Kind
		d.VariantEvent = fmt.Sprintf("%03d %-18s %s", variant.Seq, variant.Kind, eventSummary(*variant))
	}
	return d
}

func firstCausalDivergence(baseline, variant []string) *CausalDivergence {
	max := len(baseline)
	if len(variant) < max {
		max = len(variant)
	}
	for i := range max {
		if baseline[i] != variant[i] {
			return &CausalDivergence{Index: i, Baseline: baseline[i], Variant: variant[i]}
		}
	}
	if len(baseline) == len(variant) {
		return nil
	}
	out := &CausalDivergence{Index: max}
	if len(baseline) > max {
		out.Baseline = baseline[max]
	}
	if len(variant) > max {
		out.Variant = variant[max]
	}
	return out
}

func semanticDelta(baseline, variant *Artifact) SemanticDelta {
	leftOutput := lastOutputText(baseline)
	rightOutput := lastOutputText(variant)
	leftTools := toolSequence(baseline)
	rightTools := toolSequence(variant)
	evalDelta := evaluatorDelta(baseline.Summary.Evaluator, variant.Summary.Evaluator)
	out := SemanticDelta{
		StatusChanged:       baseline.Summary.Status != variant.Summary.Status,
		FinalOutputChanged:  leftOutput != rightOutput,
		ToolSequenceChanged: !stringSlicesEqual(leftTools, rightTools),
		EvaluatorChanged:    evaluatorChanged(evalDelta),
		BaselineOutput:      truncateLine(leftOutput, 240),
		VariantOutput:       truncateLine(rightOutput, 240),
		BaselineTools:       leftTools,
		VariantTools:        rightTools,
	}
	if out.StatusChanged {
		out.Notes = append(out.Notes, fmt.Sprintf("status %s -> %s", nonEmpty(baseline.Summary.Status, "unknown"), nonEmpty(variant.Summary.Status, "unknown")))
	}
	if out.FinalOutputChanged {
		out.Notes = append(out.Notes, fmt.Sprintf("final output %s -> %s", truncateLine(nonEmpty(leftOutput, "<empty>"), 80), truncateLine(nonEmpty(rightOutput, "<empty>"), 80)))
	}
	if out.ToolSequenceChanged {
		out.Notes = append(out.Notes, fmt.Sprintf("tools %s -> %s", strings.Join(leftTools, ","), strings.Join(rightTools, ",")))
	}
	if out.EvaluatorChanged {
		out.Notes = append(out.Notes, "evaluator result changed")
	}
	out.Changed = len(out.Notes) > 0
	return out
}

func buildDiffNarrative(diff DiffResult) []string {
	var lines []string
	if diff.FirstDivergence == nil {
		lines = append(lines, "no event-level divergence detected")
	} else {
		lines = append(lines, fmt.Sprintf("first divergence at event %d", diff.FirstDivergence.Index+1))
	}
	if diff.CausalDivergence == nil {
		lines = append(lines, "no causal path divergence detected")
	} else {
		lines = append(lines, fmt.Sprintf("causal path divergence at boundary %d", diff.CausalDivergence.Index+1))
	}
	if diff.CausalGraphDelta != nil && diff.CausalGraphDelta.FirstEdgeDivergence != nil {
		lines = append(lines, fmt.Sprintf("causal graph edge divergence at edge %d", diff.CausalGraphDelta.FirstEdgeDivergence.Index+1))
	}
	if diff.SemanticDelta.Changed {
		lines = append(lines, "semantic delta: "+strings.Join(diff.SemanticDelta.Notes, "; "))
	}
	if diff.BaselineStatus == diff.VariantStatus {
		lines = append(lines, "both traces ended "+nonEmpty(diff.BaselineStatus, "unknown"))
	} else {
		lines = append(lines, fmt.Sprintf("status changed from %s to %s", nonEmpty(diff.BaselineStatus, "unknown"), nonEmpty(diff.VariantStatus, "unknown")))
	}
	if diff.UsageDelta.TotalTokens != 0 {
		lines = append(lines, fmt.Sprintf("total token delta %+d", diff.UsageDelta.TotalTokens))
	}
	if diff.UsageDelta.ToolCalls != 0 {
		lines = append(lines, fmt.Sprintf("tool-call delta %+d", diff.UsageDelta.ToolCalls))
	}
	if diff.DurationDeltaMillis != 0 {
		lines = append(lines, fmt.Sprintf("duration delta %+s", formatMillisDelta(diff.DurationDeltaMillis)))
	}
	if diff.CostDelta != 0 {
		lines = append(lines, fmt.Sprintf("cost delta %+0.6f", diff.CostDelta))
	}
	if diff.RetryErrorDelta.RetryScheduled != 0 || diff.RetryErrorDelta.ErrorsRaised != 0 || diff.RetryErrorDelta.Failures != 0 {
		lines = append(lines, fmt.Sprintf("retry/error delta retries=%+d errors=%+d failures=%+d", diff.RetryErrorDelta.RetryScheduled, diff.RetryErrorDelta.ErrorsRaised, diff.RetryErrorDelta.Failures))
	}
	if diff.EvaluatorDelta != nil && diff.EvaluatorDelta.ScoreDelta != nil {
		lines = append(lines, fmt.Sprintf("evaluator score delta %+0.4f", *diff.EvaluatorDelta.ScoreDelta))
	}
	if len(diff.TopologyDelta) > 0 {
		lines = append(lines, fmt.Sprintf("topology changed at %d boundary event(s)", len(diff.TopologyDelta)))
	}
	if len(diff.ArtifactDelta) > 0 {
		lines = append(lines, fmt.Sprintf("artifact changes differ at %d boundary event(s)", len(diff.ArtifactDelta)))
	}
	if diff.FinalOutputDelta != "" {
		lines = append(lines, "final output changed")
	}
	if diff.VariantError != "" && diff.VariantError != diff.BaselineError {
		lines = append(lines, "variant error: "+diff.VariantError)
	}
	return lines
}

func toolSequence(artifact *Artifact) []string {
	if artifact == nil {
		return nil
	}
	var tools []string
	for _, event := range artifact.Events {
		if event.Kind != "tool.called" {
			continue
		}
		tool := firstPayloadString(event, "tool_name")
		if tool == "" {
			tool = "<unknown>"
		}
		tools = append(tools, tool)
	}
	return tools
}

func evaluatorChanged(delta *EvaluatorDelta) bool {
	if delta == nil {
		return false
	}
	if delta.ScoreDelta != nil && *delta.ScoreDelta != 0 {
		return true
	}
	if delta.BaselinePassed == nil && delta.VariantPassed != nil {
		return true
	}
	if delta.BaselinePassed != nil && delta.VariantPassed == nil {
		return true
	}
	return delta.BaselinePassed != nil && delta.VariantPassed != nil && *delta.BaselinePassed != *delta.VariantPassed
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func causalPath(events []Event) []string {
	path := make([]string, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case "model.responded", "tool.called", "tool.completed", "tool.failed", "approval.requested", "deferred.requested", "retry.scheduled", "topology.transitioned", "evaluator.completed", "artifact.changed", "error.raised":
			summary := event.Kind
			if key := boundaryKey(event); key != "" {
				summary += ":" + shortID(key)
			}
			if tool := firstPayloadString(event, "tool_name"); tool != "" {
				summary += ":" + tool
			}
			if topo := firstPayloadString(event, "to"); topo != "" {
				summary += ":to=" + topo
			}
			path = append(path, summary)
		}
	}
	return path
}

type causalGraph struct {
	nodes []string
	edges []string
}

func buildCausalGraph(events []Event) causalGraph {
	graph := causalGraph{nodes: make([]string, 0, len(events))}
	lastByRequest := make(map[string]string)
	lastByToolCall := make(map[string]string)
	lastByStep := make(map[int]string)
	lastByAgent := make(map[string]string)
	var last string

	for _, event := range events {
		node := eventGraphKey(event)
		graph.nodes = append(graph.nodes, node)
		parent := ""
		if event.CausalParentID != "" {
			parent = lastByAgent[event.CausalParentID]
		}
		if parent == "" && event.RequestID != "" {
			parent = lastByRequest[event.RequestID]
		}
		if parent == "" {
			if toolCallID := firstPayloadString(event, "tool_call_id"); toolCallID != "" {
				parent = lastByToolCall[toolCallID]
			}
		}
		if parent == "" && event.Step > 0 {
			parent = lastByStep[event.Step]
		}
		if parent == "" {
			parent = last
		}
		if parent != "" && parent != node {
			graph.edges = append(graph.edges, parent+" -> "+node)
		}
		if event.RequestID != "" {
			lastByRequest[event.RequestID] = node
		}
		if toolCallID := firstPayloadString(event, "tool_call_id"); toolCallID != "" {
			lastByToolCall[toolCallID] = node
		}
		if event.Step > 0 {
			lastByStep[event.Step] = node
		}
		if event.AgentID != "" {
			lastByAgent[event.AgentID] = node
		}
		last = node
	}
	return graph
}

func eventGraphKey(event Event) string {
	key := event.Kind
	if event.Step > 0 {
		key += fmt.Sprintf("@step:%d", event.Step)
	}
	switch {
	case boundaryKey(event) != "":
		key += ":" + shortID(boundaryKey(event))
	case firstPayloadString(event, "tool_name") != "":
		key += ":" + firstPayloadString(event, "tool_name")
	case firstPayloadString(event, "to") != "":
		key += ":to=" + firstPayloadString(event, "to")
	case firstPayloadString(event, "path") != "":
		key += ":" + firstPayloadString(event, "path")
	case event.RequestID != "":
		key += ":" + shortID(event.RequestID)
	case event.ID != "":
		key += ":" + shortID(event.ID)
	default:
		key += fmt.Sprintf(":%03d", event.Seq)
	}
	return key
}

func causalGraphDelta(baseline, variant causalGraph) *CausalGraphDelta {
	out := &CausalGraphDelta{
		BaselineNodes: len(baseline.nodes),
		VariantNodes:  len(variant.nodes),
		BaselineEdges: len(baseline.edges),
		VariantEdges:  len(variant.edges),
		NodeDelta:     len(variant.nodes) - len(baseline.nodes),
		EdgeDelta:     len(variant.edges) - len(baseline.edges),
	}
	out.FirstEdgeDivergence = firstCausalDivergence(baseline.edges, variant.edges)
	out.BaselineOnlyEdges, out.VariantOnlyEdges = stringSetDelta(baseline.edges, variant.edges)
	if out.FirstEdgeDivergence == nil && out.NodeDelta == 0 && out.EdgeDelta == 0 && len(out.BaselineOnlyEdges) == 0 && len(out.VariantOnlyEdges) == 0 {
		return nil
	}
	return out
}

func stringSetDelta(baseline, variant []string) ([]string, []string) {
	leftCounts := make(map[string]int, len(baseline))
	rightCounts := make(map[string]int, len(variant))
	for _, value := range baseline {
		leftCounts[value]++
	}
	for _, value := range variant {
		rightCounts[value]++
	}
	var leftOnly, rightOnly []string
	for _, value := range baseline {
		if rightCounts[value] > 0 {
			rightCounts[value]--
			continue
		}
		leftOnly = append(leftOnly, value)
	}
	for _, value := range variant {
		if leftCounts[value] > 0 {
			leftCounts[value]--
			continue
		}
		rightOnly = append(rightOnly, value)
	}
	return leftOnly, rightOnly
}

func kindDelta(baseline, variant []Event) map[string]int {
	counts := make(map[string]int)
	for _, event := range baseline {
		counts[event.Kind]--
	}
	for _, event := range variant {
		counts[event.Kind]++
	}
	for kind, delta := range counts {
		if delta == 0 {
			delete(counts, kind)
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func boundaryDelta(kind string, baseline, variant []Event) []string {
	left := boundarySummaries(kind, baseline)
	right := boundarySummaries(kind, variant)
	max := len(left)
	if len(right) > max {
		max = len(right)
	}
	var out []string
	for i := range max {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l != r {
			out = append(out, fmt.Sprintf("%s -> %s", nonEmpty(l, "<missing>"), nonEmpty(r, "<missing>")))
		}
	}
	return out
}

func boundarySummaries(kind string, events []Event) []string {
	var out []string
	for _, event := range events {
		if event.Kind != kind {
			continue
		}
		summary := eventSummary(event)
		if summary == "" {
			summary = stableJSON(event.Payload)
		}
		out = append(out, summary)
	}
	return out
}

func evaluatorDelta(baseline, variant *EvaluatorSummary) *EvaluatorDelta {
	if baseline == nil && variant == nil {
		return nil
	}
	out := &EvaluatorDelta{}
	if baseline != nil {
		out.BaselineScore = baseline.Score
		out.BaselinePassed = baseline.Passed
	}
	if variant != nil {
		out.VariantScore = variant.Score
		out.VariantPassed = variant.Passed
	}
	if out.BaselineScore != nil && out.VariantScore != nil {
		delta := *out.VariantScore - *out.BaselineScore
		out.ScoreDelta = &delta
	}
	return out
}

func retryErrorDelta(baseline, variant []Event) RetryErrorDelta {
	return RetryErrorDelta{
		RetryScheduled: countKind(variant, "retry.scheduled") - countKind(baseline, "retry.scheduled"),
		ErrorsRaised:   countKind(variant, "error.raised") - countKind(baseline, "error.raised"),
		Failures:       countFailures(variant) - countFailures(baseline),
	}
}

func countKind(events []Event, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func countFailures(events []Event) int {
	count := 0
	for _, event := range events {
		switch event.Kind {
		case "run.failed", "model.failed", "tool.failed":
			count++
		}
	}
	return count
}

func finalOutputDelta(baseline, variant *Artifact) string {
	left := lastOutputText(baseline)
	right := lastOutputText(variant)
	if left == right {
		return ""
	}
	return fmt.Sprintf("%s -> %s", truncateLine(nonEmpty(left, "<empty>"), 120), truncateLine(nonEmpty(right, "<empty>"), 120))
}

func lastOutputText(artifact *Artifact) string {
	if artifact == nil || artifact.Trace == nil {
		return ""
	}
	for i := len(artifact.Trace.Steps) - 1; i >= 0; i-- {
		step := artifact.Trace.Steps[i]
		if step.Kind != core.TraceModelResponse {
			continue
		}
		if value := traceStepDataValue(step.Data, "text"); value != "" {
			return value
		}
	}
	return ""
}

func traceStepDataValue(data any, key string) string {
	switch value := data.(type) {
	case map[string]any:
		if got, ok := value[key]; ok {
			return fmt.Sprint(got)
		}
	case map[string]string:
		return value[key]
	}
	return ""
}

func firstPayloadString(event Event, key string) string {
	if value, ok := event.Payload[key]; ok && fmt.Sprint(value) != "" {
		return fmt.Sprint(value)
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", key); ok && fmt.Sprint(value) != "" {
		return fmt.Sprint(value)
	}
	return ""
}

func boolPointerValue(value *bool) any {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func costTotal(cost *core.RunCost) float64 {
	if cost == nil {
		return 0
	}
	return cost.TotalCost
}

func formatMillisDelta(delta int64) string {
	sign := "+"
	if delta < 0 {
		sign = "-"
		delta = -delta
	}
	return fmt.Sprintf("%s%dms", sign, delta)
}

func eventFingerprint(event Event) string {
	payload := map[string]any{}
	for _, key := range []string{"prompt", "model", "finish_reason", "message_count", "function_tool_count", "output_tool_count", "compactions", "success", "error"} {
		if value, ok := event.Payload[key]; ok {
			payload[key] = value
		}
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "tool_name"); ok {
		payload["tool_name"] = value
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "args"); ok {
		payload["args"] = value
	}
	if value, ok := nestedPayloadValue(event.Payload, "data", "error"); ok {
		payload["tool_error"] = value
	}
	for _, key := range []string{"tool_call_id", "tool_name", "args", "result", "approved", "reason"} {
		if value, ok := event.Payload[key]; ok {
			payload[key] = value
		}
	}
	if len(payload) == 0 {
		payload = event.Payload
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(fmt.Sprint(payload))
	}
	return event.Kind + ":" + string(data)
}

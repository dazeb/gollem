package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTraceArtifactProjectsRuntimeMetadataAndBoundaryKinds(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	score := 0.8
	passed := true
	snap := &RunSnapshot{
		Messages: []ModelMessage{
			ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "resume"}}},
		},
		ToolState:    map[string]any{"z": 1, "a": 2},
		RunID:        "run-rich",
		ParentRunID:  "parent-rich",
		RunStep:      3,
		RunStartTime: now,
		Prompt:       "resume",
		Timestamp:    now.Add(3 * time.Second),
	}
	trace := &RunTrace{
		RunID:     "run-rich",
		Prompt:    "trace everything",
		StartTime: now,
		EndTime:   now.Add(5 * time.Second),
		Duration:  5 * time.Second,
		Success:   true,
		Usage: RunUsage{
			Usage:     Usage{InputTokens: 11, OutputTokens: 7},
			Requests:  2,
			ToolCalls: 4,
		},
		Requests: []RequestTrace{
			{
				RequestID:         "req-1",
				TurnNumber:        1,
				Sequence:          1,
				ModelName:         "model-a",
				MessageCount:      2,
				FunctionToolCount: 1,
				OutputToolCount:   1,
				StartedAt:         now.Add(100 * time.Millisecond),
				EndedAt:           now.Add(200 * time.Millisecond),
				Duration:          100 * time.Millisecond,
				Response:          &RequestTraceResponse{ModelName: "model-a", FinishReason: "tool_calls", Usage: Usage{InputTokens: 5, OutputTokens: 2}},
			},
			{
				RequestID:  "req-2",
				TurnNumber: 2,
				Sequence:   2,
				ModelName:  "model-a",
				StartedAt:  now.Add(2 * time.Second),
				EndedAt:    now.Add(2100 * time.Millisecond),
				Duration:   100 * time.Millisecond,
				Error:      "model failed",
			},
		},
		Steps: []TraceStep{
			{Kind: TraceToolCall, Timestamp: now.Add(300 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-1", "tool_name": "shell", "turn_number": 1}},
			{Kind: TraceToolResult, Timestamp: now.Add(400 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-1", "tool_name": "shell", "error": "boom", "turn_number": 1}},
			{Kind: TraceModelDelta, Timestamp: now.Add(500 * time.Millisecond), Data: map[string]string{"turn_number": "1", "text": "delta"}},
			{Kind: TraceGuardrail, Timestamp: now.Add(600 * time.Millisecond), Data: map[string]any{"reason": "ok"}},
			{Kind: TraceCheckpointCreated, Timestamp: now.Add(700 * time.Millisecond), Data: map[string]any{"checkpoint_id": "manual"}},
			{Kind: TraceApprovalRequested, Timestamp: now.Add(800 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-2"}},
			{Kind: TraceApprovalResolved, Timestamp: now.Add(900 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-2", "approved": true}},
			{Kind: TraceDeferredRequested, Timestamp: now.Add(1000 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-3"}},
			{Kind: TraceDeferredResolved, Timestamp: now.Add(1100 * time.Millisecond), Data: map[string]any{"tool_call_id": "call-3"}},
			{Kind: TraceRunWaiting, Timestamp: now.Add(1200 * time.Millisecond), Data: map[string]any{"reason": "approval"}},
			{Kind: TraceRunResumed, Timestamp: now.Add(1300 * time.Millisecond), Data: map[string]any{"reason": "signal"}},
			{Kind: TraceRetryScheduled, Timestamp: now.Add(1400 * time.Millisecond), Data: map[string]any{"reason": "invalid"}},
			{Kind: TraceTopologyTransitioned, Timestamp: now.Add(1500 * time.Millisecond), Data: map[string]any{"from": "single", "to": "team"}},
			{Kind: TraceEvaluatorCompleted, Timestamp: now.Add(1600 * time.Millisecond), Data: map[string]any{"name": "tests", "score": 0.8, "passed": true}},
			{Kind: TraceArtifactChanged, Timestamp: now.Add(1700 * time.Millisecond), Data: map[string]any{"path": "main.go"}},
			{Kind: TraceErrorRaised, Timestamp: now.Add(1800 * time.Millisecond), Data: map[string]any{"error": "raised"}},
			{Kind: TraceStepKind("custom_kind"), Timestamp: now.Add(1900 * time.Millisecond), Data: map[string]any{"value": "custom"}},
		},
	}
	artifact, err := NewTraceArtifactWithSnapshotsAndEvents(trace, []*RunSnapshot{nil, snap}, nil, map[string]any{
		"source_topology": "single",
		"topology":        "team",
		"evaluator":       TraceEvaluatorSummary{Name: "metadata-tests", Score: &score, Passed: &passed},
	})
	if err != nil {
		t.Fatalf("NewTraceArtifactWithSnapshotsAndEvents() error = %v", err)
	}
	if artifact.Summary.Evaluator == nil || artifact.Summary.Evaluator.Name != "metadata-tests" {
		t.Fatalf("missing metadata evaluator summary: %+v", artifact.Summary.Evaluator)
	}
	for _, want := range []string{
		"run.started", "model.requested", "model.responded", "model.failed", "tool.called", "tool.failed",
		"model.delta", "guardrail.evaluated", "checkpoint.created", "approval.requested", "approval.resolved",
		"deferred.requested", "deferred.resolved", "wait.started", "wait.resolved", "retry.scheduled",
		"topology.transitioned", "evaluator.completed", "artifact.changed", "error.raised", "trace.custom.kind",
		"snapshot.created", "run.completed",
	} {
		if !traceArtifactHasKind(artifact, want) {
			t.Fatalf("missing event kind %q in %+v", want, artifact.Events)
		}
	}
	for _, event := range artifact.Events {
		if event.Kind == "snapshot.created" {
			keys, ok := event.Payload["tool_state"].([]string)
			if !ok || len(keys) != 2 || keys[0] != "a" || keys[1] != "z" {
				t.Fatalf("unexpected snapshot tool state keys: %#v", event.Payload["tool_state"])
			}
			return
		}
	}
	t.Fatal("missing snapshot.created event")
}

func TestTraceArtifactReadWriteAndHelperBranches(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if _, err := NewTraceArtifact(nil, nil); err == nil {
		t.Fatal("expected nil run trace error")
	}
	if got := WithTraceArtifactCost(nil, &RunCost{TotalCost: 1}); got != nil {
		t.Fatalf("nil WithTraceArtifactCost = %+v", got)
	}
	artifact, err := NewTraceArtifact(&RunTrace{RunID: "run-read", Prompt: "read", StartTime: now, EndTime: now.Add(time.Second), Success: true}, nil)
	if err != nil {
		t.Fatalf("NewTraceArtifact() error = %v", err)
	}
	WithTraceArtifactCost(artifact, nil)
	WithTraceArtifactCost(artifact, &RunCost{TotalCost: 0.01, Currency: "USD"})
	if artifact.Summary.Cost == nil {
		t.Fatal("cost was not attached")
	}
	if records, err := EncodeTraceSnapshotRecords([]*RunSnapshot{nil}); err != nil || len(records) != 0 {
		t.Fatalf("nil snapshot records = %+v err=%v", records, err)
	}
	if _, err := DecodeTraceSnapshotRecord(TraceSnapshotRecord{}); err == nil {
		t.Fatal("expected nil snapshot record error")
	}
	for _, input := range []string{"", "{", `{"messages":[],"run_step":1}`, `{}`} {
		if _, err := ReadTraceArtifact(strings.NewReader(input)); err == nil {
			t.Fatalf("expected read error for %q", input)
		}
	}

	canonicalWithTraceOnly := TraceArtifact{SchemaVersion: TraceArtifactSchemaVersion, Trace: artifact.Trace}
	data, err := json.Marshal(canonicalWithTraceOnly)
	if err != nil {
		t.Fatalf("marshal canonical: %v", err)
	}
	read, err := ReadTraceArtifact(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadTraceArtifact(canonical) error = %v", err)
	}
	if len(read.Events) == 0 || read.Summary.Status != "succeeded" {
		t.Fatalf("canonical repair failed: events=%d summary=%+v", len(read.Events), read.Summary)
	}
	var buf bytes.Buffer
	if err := WriteTraceArtifact(&buf, nil); err == nil {
		t.Fatal("expected nil artifact write error")
	}
	if err := WriteTraceArtifactFile("", artifact); err == nil {
		t.Fatal("expected empty output path error")
	}
	if _, err := ReadTraceArtifactFile(""); err == nil {
		t.Fatal("expected empty input path error")
	}
	if got := safeTraceFilenamePart(" ../weird run!* "); got != "weird_run" {
		t.Fatalf("safeTraceFilenamePart = %q", got)
	}
	if got := safeTraceFilenamePart("..."); got != "run" {
		t.Fatalf("empty safeTraceFilenamePart = %q", got)
	}
}

func TestTraceArtifactFileAndIOErrorBranches(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := NewTraceArtifact(&RunTrace{RunID: "run-io", Prompt: "io", StartTime: now, EndTime: now.Add(time.Second), Success: true}, nil)
	if err != nil {
		t.Fatalf("NewTraceArtifact() error = %v", err)
	}
	if _, err := ReadTraceArtifact(errorReader{}); err == nil {
		t.Fatal("expected reader error")
	}
	if err := WriteTraceArtifact(errorWriter{}, artifact); err == nil {
		t.Fatal("expected writer error")
	}
	if _, err := ReadTraceArtifactFile(filepath.Join(t.TempDir(), "missing.trace.json")); err == nil {
		t.Fatal("expected missing trace file error")
	}

	tmp := t.TempDir()
	tracePath := filepath.Join(tmp, "trace.json")
	if err := WriteTraceArtifactFile(tracePath, artifact); err != nil {
		t.Fatalf("WriteTraceArtifactFile() error = %v", err)
	}
	read, err := ReadTraceArtifactFile(tracePath)
	if err != nil {
		t.Fatalf("ReadTraceArtifactFile() error = %v", err)
	}
	if read.Run.ID != "run-io" {
		t.Fatalf("read run id = %q", read.Run.ID)
	}

	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := WriteTraceArtifactFile(filepath.Join(blocker, "trace.json"), artifact); err == nil {
		t.Fatal("expected mkdir/write error under file path")
	}

	legacy, err := json.Marshal(RunTrace{RunID: "legacy-run", Prompt: "legacy", StartTime: now, Success: true})
	if err != nil {
		t.Fatalf("marshal legacy trace: %v", err)
	}
	imported, err := ReadTraceArtifact(bytes.NewReader(legacy))
	if err != nil {
		t.Fatalf("ReadTraceArtifact(legacy) error = %v", err)
	}
	if imported.Run.ID != "legacy-run" || imported.SchemaVersion != TraceArtifactSchemaVersion {
		t.Fatalf("unexpected imported legacy trace: %+v", imported)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestTraceArtifactNormalizationAndPayloadHelpers(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	events := NormalizeTraceEvents([]TraceEvent{
		{Kind: "run.completed"},
		{Kind: "model.responded", Timestamp: now},
		{Kind: "run.started", Timestamp: now},
		{Kind: "snapshot.created", Timestamp: now},
		{Kind: "checkpoint.created", Timestamp: now},
	})
	if events[0].Kind != "run.started" || events[1].Kind != "checkpoint.created" || events[len(events)-1].Kind != "run.completed" {
		t.Fatalf("unexpected normalized order: %+v", events)
	}
	for _, event := range events {
		if event.ID == "" || event.Seq == 0 || event.ReplayPolicy == "" {
			t.Fatalf("event not normalized: %+v", event)
		}
	}
	for i := 1; i < len(events); i++ {
		if events[i].CausalParentEventID != events[i-1].ID {
			t.Fatalf("event %d parent = %q, want %q", i, events[i].CausalParentEventID, events[i-1].ID)
		}
		if len(events[i-1].CausalChildEventIDs) == 0 || events[i-1].CausalChildEventIDs[0] != events[i].ID {
			t.Fatalf("event %d children = %+v, want first child %q", i-1, events[i-1].CausalChildEventIDs, events[i].ID)
		}
	}
	data, err := json.Marshal(events[1])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if !bytes.Contains(data, []byte(`"causal_parent_event_id"`)) || !bytes.Contains(data, []byte(`"causal_child_event_ids"`)) {
		t.Fatalf("normalized event JSON missing causal event fields: %s", data)
	}
	for _, kind := range []string{
		"run.started", "checkpoint.created", "turn.started", "model.requested", "model.delta",
		"model.responded", "model.failed", "tool.called", "approval.requested", "deferred.requested",
		"wait.started", "approval.resolved", "deferred.resolved", "wait.resolved", "retry.scheduled",
		"tool.completed", "tool.failed", "turn.completed", "snapshot.created", "topology.transitioned",
		"artifact.changed", "evaluator.completed", "error.raised", "run.completed", "run.failed", "other",
	} {
		if traceEventKindSortRank(kind) < 0 {
			t.Fatalf("negative rank for %s", kind)
		}
	}
	if traceReplayPolicyForKind("checkpoint.created") != "checkpoint" || traceReplayPolicyForKind("snapshot.created") != "snapshot" || traceReplayPolicyForKind("model.delta") != "recorded" || traceReplayPolicyForKind("guardrail.evaluated") != "recorded" || traceReplayPolicyForKind("other") != "inspect" {
		t.Fatal("unexpected replay policy")
	}

	score := 0.5
	passed := true
	payload := map[string]any{
		"score_ptr": &score,
		"score32":   float32(0.25),
		"score_int": 2,
		"score_i64": int64(3),
		"score_num": json.Number("4.5"),
		"score_str": "6.5",
		"bool_ptr":  &passed,
		"bool_str":  "true",
		"map_any":   map[string]any{"a": 1},
		"map_str":   map[string]string{"b": "2"},
		"other":     "value",
	}
	for _, key := range []string{"score_ptr", "score32", "score_int", "score_i64", "score_num", "score_str"} {
		if tracePayloadFloatPointer(payload, key) == nil {
			t.Fatalf("missing float pointer for %s", key)
		}
	}
	if tracePayloadBoolPointer(payload, "bool_ptr") == nil || tracePayloadBoolPointer(payload, "bool_str") == nil {
		t.Fatal("missing bool pointer")
	}
	if tracePayloadMap(payload, "map_any")["a"] != 1 || tracePayloadMap(payload, "map_str")["b"] != "2" || tracePayloadMap(payload, "other")["value"] != "value" {
		t.Fatalf("unexpected payload map coercion")
	}
	if tracePayloadString(map[string]any{"name": " tests "}, "name") != "tests" {
		t.Fatal("payload string not trimmed")
	}
	if traceStepDataString(map[string]string{"turn_number": "7"}, "turn_number") != "7" || traceStepDataInt(map[string]any{"turn_number": "8"}, "turn_number") != 8 {
		t.Fatal("trace step data coercion failed")
	}
	if traceStepDataInt(map[string]any{"turn_number": "bad"}, "turn_number") != 0 {
		t.Fatal("invalid trace step int should return zero")
	}
	if _, ok := numericPointer("not-a-number"); ok {
		t.Fatal("numericPointer string should fail")
	}
	if got := compactTracePayload(map[string]any{"empty": "", "zero": 0, "false": false, "keep": "x"}); len(got) != 2 || got["keep"] != "x" || got["false"] != false {
		t.Fatalf("unexpected compact payload: %+v", got)
	}
	if got := compactTracePayload(map[string]any{"empty": ""}); got != nil {
		t.Fatalf("empty compact payload = %+v", got)
	}
}

func TestNormalizeTraceEventsRemapsPersistedParentsAcrossRenumbering(t *testing.T) {
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	events := NormalizeTraceEvents([]TraceEvent{
		{Kind: "run.started", Timestamp: base},
		{Kind: "model.requested", Timestamp: base.Add(time.Second), RequestID: "req-1"},
		{Kind: "error.raised", Timestamp: base.Add(2 * time.Second)},
		{Kind: "run.failed", Timestamp: base.Add(2 * time.Second)},
	})
	if events[3].Kind != "run.failed" || events[3].CausalParentEventID != events[2].ID {
		t.Fatalf("precondition: run.failed parent = %q, want error.raised %q", events[3].CausalParentEventID, events[2].ID)
	}

	// evaluator.completed sorts before error.raised and run.failed at the same
	// timestamp, displacing their positional IDs by one.
	events = NormalizeTraceEvents(append(events, TraceEvent{
		Kind:      "evaluator.completed",
		Timestamp: base.Add(2 * time.Second),
	}))
	kinds := make([]string, len(events))
	for i, event := range events {
		kinds[i] = event.Kind
	}
	if kinds[2] != "evaluator.completed" || kinds[3] != "error.raised" || kinds[4] != "run.failed" {
		t.Fatalf("unexpected event order after insertion: %v", kinds)
	}
	if events[4].CausalParentEventID != events[3].ID {
		t.Fatalf("run.failed parent = %q, want renumbered error.raised %q (stale link survived renumbering)", events[4].CausalParentEventID, events[3].ID)
	}
	if len(events[3].CausalChildEventIDs) != 1 || events[3].CausalChildEventIDs[0] != events[4].ID {
		t.Fatalf("error.raised children = %+v, want [%q]", events[3].CausalChildEventIDs, events[4].ID)
	}
	for _, childID := range events[2].CausalChildEventIDs {
		if childID == events[4].ID {
			t.Fatalf("evaluator event must not adopt run.failed: %+v", events[2].CausalChildEventIDs)
		}
	}

	// Stale references to events that no longer exist are dropped and the
	// parent is re-inferred instead of left dangling.
	events[3].CausalParentEventID = "evt_gone"
	events = NormalizeTraceEvents(events)
	if events[3].CausalParentEventID != events[2].ID {
		t.Fatalf("dangling parent reference must be re-inferred, got %q", events[3].CausalParentEventID)
	}

	// A persisted parent that inference would never choose must survive
	// renumbering: rewire run.failed to model.requested, then displace
	// model.requested's positional ID with an earlier insertion.
	events[4].CausalParentEventID = events[1].ID
	events = NormalizeTraceEvents(append(events, TraceEvent{
		Kind:      "checkpoint.created",
		Timestamp: base.Add(500 * time.Millisecond),
	}))
	if events[2].Kind != "model.requested" || events[5].Kind != "run.failed" {
		t.Fatalf("unexpected order after early insertion: %+v", events)
	}
	if events[5].CausalParentEventID != events[2].ID {
		t.Fatalf("rewired parent must survive renumbering, got %q want model.requested %q", events[5].CausalParentEventID, events[2].ID)
	}

	// References to a duplicated pre-normalization ID are ambiguous and must be
	// re-inferred (here via request grouping), not resolved to the last holder.
	dupEvents := NormalizeTraceEvents([]TraceEvent{
		{Kind: "run.started", Timestamp: base},
		{Kind: "model.requested", Timestamp: base.Add(time.Second), RequestID: "req-9"},
		{Kind: "tool.called", Timestamp: base.Add(2 * time.Second)},
		{Kind: "model.responded", Timestamp: base.Add(3 * time.Second), RequestID: "req-9"},
	})
	dupEvents[1].ID = "evt_dup"
	dupEvents[2].ID = "evt_dup"
	dupEvents[3].CausalParentEventID = "evt_dup"
	dupEvents = NormalizeTraceEvents(dupEvents)
	if dupEvents[3].CausalParentEventID != dupEvents[1].ID {
		t.Fatalf("ambiguous duplicate id must be re-inferred via request grouping, got %q want %q", dupEvents[3].CausalParentEventID, dupEvents[1].ID)
	}
}

func TestAttachTraceEventCausalityPrecedence(t *testing.T) {
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
	// Every event after the first carries Step 1, so step grouping (and the
	// previous-event fallback) always has a competing answer; each assertion
	// below pins a higher-precedence mechanism beating it.
	events := NormalizeTraceEvents([]TraceEvent{
		{Kind: "run.started", Timestamp: at(0), AgentID: "run-1"},
		{Kind: "turn.started", Timestamp: at(1), Step: 1},
		{Kind: "model.requested", Timestamp: at(2), RequestID: "req-a", Step: 1},
		{Kind: "tool.called", Timestamp: at(3), Payload: map[string]any{"tool_call_id": "tc-1"}, Step: 1},
		{Kind: "model.responded", Timestamp: at(4), RequestID: "req-a", Step: 1},
		{Kind: "tool.completed", Timestamp: at(5), Payload: map[string]any{"tool_call_id": "tc-1"}, Step: 1},
		{Kind: "checkpoint.created", Timestamp: at(6), CausalParentID: "run-1", RequestID: "req-a", Step: 1},
	})
	assertParent := func(child, parent int, reason string) {
		t.Helper()
		if events[child].CausalParentEventID != events[parent].ID {
			t.Fatalf("%s: event %d parent = %q, want %q", reason, child, events[child].CausalParentEventID, events[parent].ID)
		}
		found := false
		for _, childID := range events[parent].CausalChildEventIDs {
			if childID == events[child].ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: event %d children %+v missing %q", reason, parent, events[parent].CausalChildEventIDs, events[child].ID)
		}
	}
	assertParent(1, 0, "previous event is the fallback parent")
	assertParent(2, 1, "step grouping applies when no higher key matches")
	assertParent(4, 2, "request id beats step grouping")
	assertParent(5, 3, "tool call id beats step grouping")
	assertParent(6, 0, "agent lineage beats request and step grouping")
}

func TestTraceArtifactEvaluatorMetadataForms(t *testing.T) {
	score := 0.6
	passed := true
	for _, metadata := range []map[string]any{
		{"evaluator": &TraceEvaluatorSummary{Name: "ptr", Score: &score, Passed: &passed}},
		{"evaluator": map[string]any{"name": "map", "score": json.Number("0.7"), "passed": true, "detail": "ok"}},
		{"evaluator": "raw-value"},
	} {
		summary := traceEvaluatorSummaryFromMetadata(metadata)
		if summary == nil {
			t.Fatalf("missing evaluator summary for %+v", metadata)
		}
	}
	if traceEvaluatorSummaryFromMetadata(nil) != nil || traceEvaluatorSummaryFromMetadata(map[string]any{"other": "value"}) != nil {
		t.Fatal("unexpected evaluator summary without evaluator metadata")
	}
}

func TestTraceArtifactInfersEvaluatorSummaryFromEvents(t *testing.T) {
	score := 0.7
	passed := true
	artifact, err := NewTraceArtifactWithSnapshotsAndEvents(
		&RunTrace{RunID: "run-eval", Success: true},
		nil,
		[]TraceEvent{
			{Kind: "evaluator.completed", AgentID: "other", Payload: map[string]any{"name": "ignored", "score": 0.1}},
			{Kind: "evaluator.completed", AgentID: "run-eval", Payload: map[string]any{"name": "tests", "score": &score, "passed": &passed, "results": map[string]string{"suite": "unit"}}},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewTraceArtifactWithSnapshotsAndEvents() error = %v", err)
	}
	if artifact.Summary.Evaluator == nil || artifact.Summary.Evaluator.Name != "tests" || artifact.Summary.Evaluator.Score == nil || *artifact.Summary.Evaluator.Score != score {
		t.Fatalf("unexpected inferred evaluator: %+v", artifact.Summary.Evaluator)
	}
	if artifact.Summary.Evaluator.Results["suite"] != "unit" {
		t.Fatalf("unexpected evaluator results: %+v", artifact.Summary.Evaluator.Results)
	}
}

func traceArtifactHasKind(artifact *TraceArtifact, kind string) bool {
	for _, event := range artifact.Events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

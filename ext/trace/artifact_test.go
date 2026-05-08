package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	temporalext "github.com/fugue-labs/gollem/ext/temporal"
)

func TestFromRunTraceProjectsCanonicalArtifact(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	runTrace := sampleRunTrace(now)

	artifact, err := FromRunTrace(runTrace, map[string]any{"provider": "test"})
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}

	if artifact.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %q, want %q", artifact.SchemaVersion, SchemaVersion)
	}
	if artifact.Run.ID != "run-1" {
		t.Fatalf("run id = %q, want run-1", artifact.Run.ID)
	}
	if artifact.Summary.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", artifact.Summary.Status)
	}
	if artifact.Summary.Requests != 1 {
		t.Fatalf("requests = %d, want 1", artifact.Summary.Requests)
	}
	if artifact.Summary.ToolCalls != 1 {
		t.Fatalf("tool calls = %d, want 1", artifact.Summary.ToolCalls)
	}
	if len(artifact.Events) != 6 {
		t.Fatalf("events = %d, want 6: %+v", len(artifact.Events), artifact.Events)
	}
	kinds := make([]string, len(artifact.Events))
	for i, event := range artifact.Events {
		kinds[i] = event.Kind
		if event.Seq != i+1 {
			t.Fatalf("event %d seq = %d, want %d", i, event.Seq, i+1)
		}
		if event.ID == "" {
			t.Fatalf("event %d missing id", i)
		}
	}
	wantKinds := []string{"run.started", "model.requested", "model.responded", "tool.called", "tool.completed", "run.completed"}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("kind[%d] = %q, want %q (all=%v)", i, kinds[i], wantKinds[i], kinds)
		}
	}
}

func TestReadAcceptsLegacyRunTrace(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	data, err := json.Marshal(sampleRunTrace(now))
	if err != nil {
		t.Fatalf("marshal sample: %v", err)
	}

	artifact, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if artifact.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %q, want %q", artifact.SchemaVersion, SchemaVersion)
	}
	if artifact.Trace == nil || artifact.Trace.RunID != "run-1" {
		t.Fatalf("unexpected embedded trace: %+v", artifact.Trace)
	}
}

func TestReadRejectsSnapshotJSON(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	data, err := core.MarshalSnapshot(&core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: "hello", Timestamp: now},
				},
				Timestamp: now,
			},
		},
		RunID:     "snapshot-run",
		RunStep:   1,
		Prompt:    "hello",
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("MarshalSnapshot() error = %v", err)
	}

	_, err = Read(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "run snapshot") {
		t.Fatalf("expected snapshot rejection, got %v", err)
	}
}

func TestFromRunTraceWithSnapshotsAndForkSnapshot(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snap := &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: "original", Timestamp: now},
				},
				Timestamp: now,
			},
		},
		RunID:        "run-1",
		RunStep:      1,
		RunStartTime: now,
		Prompt:       "original",
		Timestamp:    now,
	}
	artifact, err := FromRunTraceWithSnapshots(sampleRunTrace(now), []*core.RunSnapshot{snap}, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshots() error = %v", err)
	}
	if len(artifact.Snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(artifact.Snapshots))
	}
	if !hasEventKind(artifact, "snapshot.created") {
		t.Fatalf("expected snapshot.created event, got %+v", artifact.Events)
	}
	if !hasEventKind(artifact, "checkpoint.created") {
		t.Fatalf("expected checkpoint.created event, got %+v", artifact.Events)
	}

	forked, record, err := ForkSnapshot(artifact, ForkOptions{
		FromStep:   1,
		NewRunID:   "fork-1",
		Prompt:     "fork prompt",
		AppendUser: "try another path",
	})
	if err != nil {
		t.Fatalf("ForkSnapshot() error = %v", err)
	}
	if record.ID != "snap_000001" {
		t.Fatalf("record id = %q, want snap_000001", record.ID)
	}
	if forked.RunID != "fork-1" {
		t.Fatalf("fork run id = %q, want fork-1", forked.RunID)
	}
	if forked.ParentRunID != "run-1" {
		t.Fatalf("fork parent = %q, want run-1", forked.ParentRunID)
	}
	if forked.SourceTraceRunID != "run-1" {
		t.Fatalf("fork source trace run id = %q, want run-1", forked.SourceTraceRunID)
	}
	if forked.SourceSnapshotID != "snap_000001" {
		t.Fatalf("fork source snapshot id = %q, want snap_000001", forked.SourceSnapshotID)
	}
	if forked.Prompt != "fork prompt" {
		t.Fatalf("fork prompt = %q, want fork prompt", forked.Prompt)
	}
	if len(forked.Messages) != 2 {
		t.Fatalf("fork messages = %d, want 2", len(forked.Messages))
	}
	lastReq, ok := forked.Messages[1].(core.ModelRequest)
	if !ok {
		t.Fatalf("last message = %T, want core.ModelRequest", forked.Messages[1])
	}
	if len(lastReq.Parts) != 1 {
		t.Fatalf("last request parts = %d, want 1", len(lastReq.Parts))
	}
	user, ok := lastReq.Parts[0].(core.UserPromptPart)
	if !ok || user.Content != "try another path" {
		t.Fatalf("unexpected appended user part: %#v", lastReq.Parts[0])
	}

	checkpointFork, checkpointRecord, err := ForkSnapshot(artifact, ForkOptions{
		FromCheckpoint: "snap_000001",
		NewRunID:       "fork-checkpoint",
	})
	if err != nil {
		t.Fatalf("ForkSnapshot(from checkpoint) error = %v", err)
	}
	if checkpointRecord.ID != "snap_000001" {
		t.Fatalf("checkpoint record id = %q, want snap_000001", checkpointRecord.ID)
	}
	if checkpointFork.SourceSnapshotID != "snap_000001" {
		t.Fatalf("checkpoint fork source snapshot id = %q, want snap_000001", checkpointFork.SourceSnapshotID)
	}
}

func TestForkSnapshotFromEventAndSystemPrompt(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snap1 := &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.SystemPromptPart{Content: "old system", Timestamp: now},
					core.UserPromptPart{Content: "original", Timestamp: now},
				},
				Timestamp: now,
			},
		},
		RunID:        "run-1",
		RunStep:      1,
		RunStartTime: now,
		Prompt:       "original",
		Timestamp:    now,
	}
	snap3 := snap1.Branch(func(messages []core.ModelMessage) []core.ModelMessage { return messages })
	snap3.RunStep = 3
	snap3.Timestamp = now.Add(3 * time.Second)

	artifact, err := FromRunTraceWithSnapshots(sampleRunTrace(now), []*core.RunSnapshot{snap1, snap3}, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshots() error = %v", err)
	}
	var eventID string
	for _, event := range artifact.Events {
		if event.Kind == "tool.called" {
			eventID = event.ID
			break
		}
	}
	if eventID == "" {
		t.Fatal("missing tool.called event")
	}

	forked, record, err := ForkSnapshot(artifact, ForkOptions{
		FromEventID:  eventID,
		SystemPrompt: "new system",
		NewRunID:     "fork-event",
	})
	if err != nil {
		t.Fatalf("ForkSnapshot() error = %v", err)
	}
	if record.Step != 1 {
		t.Fatalf("selected snapshot step = %d, want turn snapshot step 1", record.Step)
	}
	if forked.RunID != "fork-event" {
		t.Fatalf("fork run id = %q", forked.RunID)
	}
	req, ok := forked.Messages[0].(core.ModelRequest)
	if !ok {
		t.Fatalf("first message = %T, want core.ModelRequest", forked.Messages[0])
	}
	system, ok := req.Parts[0].(core.SystemPromptPart)
	if !ok || system.Content != "new system" {
		t.Fatalf("system prompt not replaced: %#v", req.Parts[0])
	}
}

func TestForkSnapshotSynthesizesFromRequestTraceWithoutStoredSnapshot(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	initial := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "write file", Timestamp: now}}, Timestamp: now},
	}
	assistantTool := core.ModelResponse{
		Parts:        []core.ModelResponsePart{core.ToolCallPart{ToolName: "write", ArgsJSON: `{"path":"out.txt"}`, ToolCallID: "call-1"}},
		FinishReason: core.FinishReasonToolCall,
		Timestamp:    now.Add(time.Second),
	}
	nextRequest := append(append([]core.ModelMessage(nil), initial...), assistantTool, core.ModelRequest{
		Parts:     []core.ModelRequestPart{core.ToolReturnPart{ToolName: "write", ToolCallID: "call-1", Content: "ok", Timestamp: now.Add(2 * time.Second)}},
		Timestamp: now.Add(2 * time.Second),
	})
	initialMessages, err := core.EncodeMessages(initial)
	if err != nil {
		t.Fatal(err)
	}
	nextMessages, err := core.EncodeMessages(nextRequest)
	if err != nil {
		t.Fatal(err)
	}
	response, err := core.EncodeModelResponse(&assistantTool)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := FromRunTrace(&core.RunTrace{
		RunID:     "run-synthetic-fork",
		Prompt:    "write file",
		StartTime: now,
		EndTime:   now.Add(3 * time.Second),
		Success:   true,
		Requests: []core.RequestTrace{
			{
				RequestID:  "req-1",
				TurnNumber: 1,
				Sequence:   1,
				StartedAt:  now,
				EndedAt:    now.Add(time.Second),
				Messages:   initialMessages,
				Response:   &core.RequestTraceResponse{Message: response, FinishReason: core.FinishReasonToolCall},
			},
			{
				RequestID:  "req-2",
				TurnNumber: 2,
				Sequence:   2,
				StartedAt:  now.Add(2 * time.Second),
				Messages:   nextMessages,
			},
		},
		Steps: []core.TraceStep{
			{Kind: core.TraceToolResult, Timestamp: now.Add(1500 * time.Millisecond), Data: map[string]any{"turn_number": 1, "tool_call_id": "call-1", "tool_name": "write", "result": "ok"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	var toolCompleted string
	for _, event := range artifact.Events {
		if event.Kind == "tool.completed" {
			toolCompleted = event.ID
			break
		}
	}
	if toolCompleted == "" {
		t.Fatal("missing tool.completed event")
	}

	forked, record, err := ForkSnapshot(artifact, ForkOptions{FromEventID: toolCompleted, NewRunID: "fork-synthetic"})
	if err != nil {
		t.Fatalf("ForkSnapshot() error = %v", err)
	}
	if !strings.HasPrefix(record.ID, "synthetic_step_") {
		t.Fatalf("record id = %q, want synthetic", record.ID)
	}
	if forked.RunID != "fork-synthetic" || forked.SourceSnapshotID != record.ID {
		t.Fatalf("unexpected fork metadata: snap=%+v record=%+v", forked, record)
	}
	if len(forked.Messages) != 3 {
		t.Fatalf("fork messages = %d, want post-tool history: %#v", len(forked.Messages), forked.Messages)
	}
	req, ok := forked.Messages[2].(core.ModelRequest)
	if !ok || len(req.Parts) != 1 {
		t.Fatalf("third message = %#v, want tool return request", forked.Messages[2])
	}
	if tr, ok := req.Parts[0].(core.ToolReturnPart); !ok || tr.ToolCallID != "call-1" {
		t.Fatalf("tool return not preserved: %#v", req.Parts[0])
	}
}

func TestForkSnapshotSyntheticDoesNotUseFutureSnapshotAsStateDonor(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	initial := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "inspect", Timestamp: now}}, Timestamp: now},
	}
	initialMessages, err := core.EncodeMessages(initial)
	if err != nil {
		t.Fatal(err)
	}
	donor := &core.RunSnapshot{
		Messages:        initial,
		RunID:           "run-synthetic-state",
		RunStep:         1,
		RunStartTime:    now,
		Prompt:          "inspect",
		Timestamp:       now.Add(3 * time.Second),
		LastInputTokens: 11,
		Retries:         2,
		ToolRetries:     map[string]int{"write": 1},
		ToolState:       map[string]any{"counter": "7"},
	}
	artifact, err := FromRunTraceWithSnapshots(&core.RunTrace{
		RunID:     "run-synthetic-state",
		Prompt:    "inspect",
		StartTime: now,
		EndTime:   now.Add(4 * time.Second),
		Success:   true,
		Requests: []core.RequestTrace{
			{
				RequestID:  "req-1",
				TurnNumber: 1,
				Sequence:   1,
				StartedAt:  now,
				Messages:   initialMessages,
			},
		},
	}, []*core.RunSnapshot{donor}, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshots() error = %v", err)
	}
	var requestedEvent string
	for _, event := range artifact.Events {
		if event.Kind == "model.requested" {
			requestedEvent = event.ID
			break
		}
	}
	if requestedEvent == "" {
		t.Fatal("missing model.requested event")
	}

	forked, record, err := ForkSnapshot(artifact, ForkOptions{FromEventID: requestedEvent})
	if err != nil {
		t.Fatalf("ForkSnapshot() error = %v", err)
	}
	if record.ID != "synthetic_step_000001" {
		t.Fatalf("record id = %q, want synthetic step 1", record.ID)
	}
	if forked.ToolState["counter"] == "7" || forked.LastInputTokens != 0 || forked.Retries != 0 || len(forked.ToolRetries) != 0 {
		t.Fatalf("synthetic fork imported future donor state: %+v", forked)
	}
	source, ok := forked.ToolState["_gollem_synthetic_state_source"].(map[string]any)
	if !ok || source["state"] != "messages_only" || source["reason"] != "no_snapshot_at_or_before_boundary" {
		t.Fatalf("missing messages-only synthetic state marker: %+v", forked.ToolState)
	}
}

func TestForkSnapshotAppliesPlannerOverridesAndMemoryEdits(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snap := &core.RunSnapshot{
		Messages:     []core.ModelMessage{core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "original", Timestamp: now}}, Timestamp: now}},
		RunID:        "run-1",
		RunStep:      1,
		RunStartTime: now,
		Prompt:       "original",
		Timestamp:    now,
		ToolState:    map[string]any{"counter": "1"},
	}
	artifact, err := FromRunTraceWithSnapshots(sampleRunTrace(now), []*core.RunSnapshot{snap}, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshots() error = %v", err)
	}

	forked, _, err := ForkSnapshot(artifact, ForkOptions{
		PlannerPrompt: "plan carefully",
		Model:         "gpt-next",
		Topology:      "team",
		Middleware:    "guarded",
		ToolPolicy:    "read-only",
		Evaluator:     "tests",
		MemoryEdits:   []string{"counter=5"},
	})
	if err != nil {
		t.Fatalf("ForkSnapshot() error = %v", err)
	}
	firstReq, ok := forked.Messages[0].(core.ModelRequest)
	if !ok {
		t.Fatalf("first message = %T, want ModelRequest", forked.Messages[0])
	}
	system, ok := firstReq.Parts[0].(core.SystemPromptPart)
	if !ok || !strings.Contains(system.Content, "plan carefully") {
		t.Fatalf("planner prompt not injected: %#v", firstReq.Parts[0])
	}
	if forked.ToolState["counter"] != "5" {
		t.Fatalf("memory edit not applied: %+v", forked.ToolState)
	}
	overrides, ok := forked.ToolState["_gollem_fork_overrides"].(map[string]any)
	if !ok {
		t.Fatalf("missing fork overrides: %+v", forked.ToolState)
	}
	if overrides["model"] != "gpt-next" || overrides["topology"] != "team" || overrides["tool_policy"] != "read-only" {
		t.Fatalf("unexpected fork overrides: %+v", overrides)
	}
}

func TestRuntimeRecorderCapturesApprovalWaitAndDeferredEvents(t *testing.T) {
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := NewRuntimeRecorder(bus)
	defer recorder.Close()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	core.Publish(bus, core.TurnStartedEvent{RunID: "run-1", TurnNumber: 1, StartedAt: now})
	core.Publish(bus, core.ApprovalRequestedEvent{RunID: "run-1", ToolCallID: "call-1", ToolName: "write", ArgsJSON: `{"path":"/tmp/out"}`, RequestedAt: now.Add(time.Second)})
	core.Publish(bus, core.RunWaitingEvent{RunID: "run-1", Reason: "approval", WaitingAt: now.Add(2 * time.Second)})
	core.Publish(bus, core.RunResumedEvent{RunID: "run-1", ResumedAt: now.Add(3 * time.Second)})
	core.Publish(bus, core.ApprovalResolvedEvent{RunID: "run-1", ToolCallID: "call-1", ToolName: "write", Approved: true, ResolvedAt: now.Add(4 * time.Second)})
	core.Publish(bus, core.ApprovalResolvedEvent{RunID: "run-1", ToolCallID: "call-denied", ToolName: "write", Approved: false, ResolvedAt: now.Add(4500 * time.Millisecond)})
	core.Publish(bus, core.DeferredRequestedEvent{RunID: "run-1", ToolCallID: "call-2", ToolName: "human", ArgsJSON: `{}`, RequestedAt: now.Add(5 * time.Second)})
	core.Publish(bus, core.DeferredResolvedEvent{RunID: "run-1", ToolCallID: "call-2", ToolName: "human", Content: "ok", ResolvedAt: now.Add(6 * time.Second)})
	core.Publish(bus, core.ArtifactChangedEvent{
		RunID:           "run-1",
		ToolCallID:      "call-1",
		ToolName:        "write",
		Path:            "/tmp/out",
		Operation:       "create",
		Bytes:           12,
		AfterSHA256:     "after",
		Diff:            "--- /dev/null\n+++ b/tmp/out\n@@ -0,0 +1,1 @@\n+hello\n",
		AfterContent:    "hello\n",
		ContentEncoding: "utf-8",
		ChangedAt:       now.Add(7 * time.Second),
	})

	artifact, err := FromRunTraceWithSnapshotsAndEvents(sampleRunTrace(now), nil, recorder.Events(), nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshotsAndEvents() error = %v", err)
	}
	for _, want := range []string{"turn.started", "approval.requested", "wait.started", "wait.resolved", "approval.resolved", "deferred.requested", "deferred.resolved", "artifact.changed"} {
		if !hasEventKind(artifact, want) {
			t.Fatalf("missing %s in events %+v", want, artifact.Events)
		}
	}
	for _, event := range artifact.Events {
		if event.Kind == "approval.resolved" && event.RequestID == "call-denied" {
			if approved, ok := event.Payload["approved"].(bool); !ok || approved {
				t.Fatalf("denied approval payload lost approved=false: %+v", event.Payload)
			}
		}
		if event.Kind == "artifact.changed" {
			if event.Payload["after_content"] != "hello\n" || event.Payload["content_encoding"] != "utf-8" {
				t.Fatalf("artifact content snapshot missing from payload: %+v", event.Payload)
			}
		}
	}
	for _, event := range artifact.Events {
		if event.Kind == "approval.resolved" && event.RequestID == "call-denied" {
			return
		}
	}
	t.Fatalf("missing denied approval event in %+v", artifact.Events)
}

func TestRuntimeRecorderNilBusAndLiveSinkBranches(t *testing.T) {
	recorder := NewRuntimeRecorder(nil)
	if got := recorder.Events(); got != nil {
		t.Fatalf("nil-bus recorder events = %+v, want nil", got)
	}
	if got := recorder.EventsForTrace(""); got != nil {
		t.Fatalf("nil-bus recorder EventsForTrace = %+v, want nil", got)
	}
	recorder.Close()
	var nilRecorder *RuntimeRecorder
	nilRecorder.Close()
	if got := nilRecorder.Events(); got != nil {
		t.Fatalf("nil recorder events = %+v, want nil", got)
	}
	nilRecorder.OnEvent(nil)()

	bus := core.NewEventBus()
	defer bus.Close()
	recorder = NewRuntimeRecorder(bus)
	defer recorder.Close()
	var seen []Event
	unsub := recorder.OnEvent(func(event Event) {
		seen = append(seen, event)
	})
	recorder.OnEvent(nil)()

	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	core.Publish(bus, core.RunStartedEvent{RunID: "run-sink", Prompt: "sink", StartedAt: now})
	if len(seen) != 1 || seen[0].Kind != "run.started" {
		t.Fatalf("sink events = %+v", seen)
	}
	unsub()
	core.Publish(bus, core.RunCompletedEvent{RunID: "run-sink", Success: true, StartedAt: now, CompletedAt: now.Add(time.Second)})
	if len(seen) != 1 {
		t.Fatalf("sink should be unsubscribed, saw %+v", seen)
	}
	if events := recorder.EventsForTrace("run-sink"); len(events) != 0 {
		t.Fatalf("root projected events should be filtered, got %+v", events)
	}
	if events := recorder.EventsForTrace(""); len(events) != 2 {
		t.Fatalf("unscoped recorder events = %+v, want both events", events)
	}
}

func TestTraceStepRuntimeBoundariesProjectToCanonicalEvents(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	runTrace := sampleRunTrace(now)
	runTrace.Steps = append(runTrace.Steps,
		core.TraceStep{
			Kind:      core.TraceApprovalRequested,
			Timestamp: now.Add(600 * time.Millisecond),
			Data: map[string]any{
				"tool_call_id": "call-approval",
				"tool_name":    "publish",
				"args":         `{}`,
			},
		},
		core.TraceStep{
			Kind:      core.TraceRunWaiting,
			Timestamp: now.Add(620 * time.Millisecond),
			Data:      map[string]any{"reason": "approval"},
		},
		core.TraceStep{
			Kind:      core.TraceRunResumed,
			Timestamp: now.Add(640 * time.Millisecond),
			Data:      map[string]any{"reason": "approval"},
		},
		core.TraceStep{
			Kind:      core.TraceApprovalResolved,
			Timestamp: now.Add(660 * time.Millisecond),
			Data: map[string]any{
				"tool_call_id": "call-approval",
				"tool_name":    "publish",
				"approved":     true,
			},
		},
		core.TraceStep{
			Kind:      core.TraceDeferredRequested,
			Timestamp: now.Add(1100 * time.Millisecond),
			Data: map[string]any{
				"tool_call_id": "call-deferred",
				"tool_name":    "human",
				"args":         `{}`,
			},
		},
		core.TraceStep{
			Kind:      core.TraceDeferredResolved,
			Timestamp: now.Add(1200 * time.Millisecond),
			Data: map[string]any{
				"tool_call_id": "call-deferred",
				"tool_name":    "human",
				"result":       "ok",
			},
		},
	)

	artifact, err := FromRunTrace(runTrace, nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	for _, want := range []string{"approval.requested", "wait.started", "wait.resolved", "approval.resolved", "deferred.requested", "deferred.resolved"} {
		if !hasEventKind(artifact, want) {
			t.Fatalf("missing %s in events %+v", want, artifact.Events)
		}
	}
	if err := ValidateReplay(artifact); err != nil {
		t.Fatalf("ValidateReplay() error = %v", err)
	}
}

func TestTraceStepPRDEventKindsProjectToCanonicalEvents(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	runTrace := sampleRunTrace(now)
	runTrace.Steps = append(runTrace.Steps,
		core.TraceStep{Kind: core.TraceCheckpointCreated, Timestamp: now.Add(50 * time.Millisecond), Data: map[string]any{"checkpoint_id": "ckpt-1"}},
		core.TraceStep{Kind: core.TraceModelDelta, Timestamp: now.Add(250 * time.Millisecond), Data: map[string]any{"text": "partial"}},
		core.TraceStep{Kind: core.TraceRetryScheduled, Timestamp: now.Add(950 * time.Millisecond), Data: map[string]any{"reason": "validation"}},
		core.TraceStep{Kind: core.TraceTopologyTransitioned, Timestamp: now.Add(1200 * time.Millisecond), Data: map[string]any{"from": "solo", "to": "team"}},
		core.TraceStep{Kind: core.TraceArtifactChanged, Timestamp: now.Add(1300 * time.Millisecond), Data: map[string]any{"path": "main.go", "operation": "modified"}},
		core.TraceStep{Kind: core.TraceEvaluatorCompleted, Timestamp: now.Add(1400 * time.Millisecond), Data: map[string]any{"name": "tests", "score": 1.0, "passed": true}},
		core.TraceStep{Kind: core.TraceErrorRaised, Timestamp: now.Add(1500 * time.Millisecond), Data: map[string]any{"error": "transient"}},
	)

	artifact, err := FromRunTrace(runTrace, map[string]any{
		"evaluator": map[string]any{"name": "tests", "score": 1.0, "passed": true},
	})
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	if artifact.Run.RuntimeVersion == "" {
		t.Fatal("expected runtime version")
	}
	if artifact.Summary.Evaluator == nil || artifact.Summary.Evaluator.Name != "tests" {
		t.Fatalf("expected evaluator summary, got %+v", artifact.Summary.Evaluator)
	}
	for _, want := range []string{"checkpoint.created", "model.delta", "retry.scheduled", "topology.transitioned", "artifact.changed", "evaluator.completed", "error.raised"} {
		if !hasEventKind(artifact, want) {
			t.Fatalf("missing %s in events %+v", want, artifact.Events)
		}
	}
}

func TestMetadataProjectsTopologyAndEvaluatorEvents(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	score := 0.75
	passed := true
	artifact, err := FromRunTrace(sampleRunTrace(now), map[string]any{
		"topology": "team",
		"evaluator": map[string]any{
			"name":   "tests",
			"score":  score,
			"passed": passed,
		},
	})
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	if !hasEventKind(artifact, "topology.transitioned") {
		t.Fatalf("missing topology event: %+v", artifact.Events)
	}
	if !hasEventKind(artifact, "evaluator.completed") {
		t.Fatalf("missing evaluator event: %+v", artifact.Events)
	}
}

func TestRuntimeRecorderRecordsPRDBoundaryEvents(t *testing.T) {
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := NewRuntimeRecorder(bus)
	defer recorder.Close()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	score := 0.9
	passed := true

	core.Publish(bus, core.RunStartedEvent{RunID: "run-1", StartedAt: now})
	core.Publish(bus, core.ModelDeltaEvent{RunID: "run-1", TurnNumber: 1, PartIndex: 0, DeltaKind: "text", ContentDelta: "partial", DeltaAt: now.Add(time.Millisecond)})
	core.Publish(bus, core.RetryScheduledEvent{RunID: "run-1", TurnNumber: 1, Reason: "validation", Retry: 1, MaxRetries: 3, ScheduledAt: now.Add(2 * time.Millisecond)})
	core.Publish(bus, core.CheckpointCreatedEvent{RunID: "run-1", CheckpointID: "ckpt-1", SnapshotID: "snap-1", Step: 1, CreatedAt: now.Add(3 * time.Millisecond)})
	core.Publish(bus, core.TopologyTransitionedEvent{RunID: "run-1", From: "solo", To: "team", Reason: "test", TransitionedAt: now.Add(4 * time.Millisecond)})
	core.Publish(bus, core.EvaluatorCompletedEvent{RunID: "run-1", Name: "tests", Score: &score, Passed: &passed, CompletedAt: now.Add(5 * time.Millisecond)})
	core.Publish(bus, core.ErrorRaisedEvent{RunID: "run-1", TurnNumber: 1, Error: "boom", RaisedAt: now.Add(6 * time.Millisecond)})
	core.Publish(bus, core.RunCompletedEvent{RunID: "run-1", Success: false, Error: "boom", StartedAt: now, CompletedAt: now.Add(7 * time.Millisecond)})

	events := recorder.Events()
	for _, want := range []string{"model.delta", "retry.scheduled", "checkpoint.created", "topology.transitioned", "evaluator.completed", "error.raised"} {
		if !hasEventKindIn(events, want) {
			t.Fatalf("missing %s in runtime events %+v", want, events)
		}
	}
	artifact, err := FromRunTraceWithSnapshotsAndEvents(sampleRunTrace(now), nil, events, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshotsAndEvents() error = %v", err)
	}
	if artifact.Summary.Evaluator == nil || artifact.Summary.Evaluator.Score == nil || *artifact.Summary.Evaluator.Score != score {
		t.Fatalf("expected evaluator summary from runtime events, got %+v", artifact.Summary.Evaluator)
	}
}

func TestRuntimeRecorderKeepsNestedBoundariesAndFiltersRootDuplicates(t *testing.T) {
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := NewRuntimeRecorder(bus)
	defer recorder.Close()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	core.Publish(bus, core.RunStartedEvent{RunID: "run-1", Prompt: "root", StartedAt: now})
	core.Publish(bus, core.ModelRequestStartedEvent{RunID: "run-1", TurnNumber: 1, MessageCount: 2, StartedAt: now.Add(100 * time.Millisecond)})
	core.Publish(bus, core.ModelResponseCompletedEvent{RunID: "run-1", TurnNumber: 1, FinishReason: "stop", DurationMs: 100, CompletedAt: now.Add(200 * time.Millisecond)})
	core.Publish(bus, core.ToolCalledEvent{RunID: "run-1", ToolCallID: "delegate-1", ToolName: "delegate", ArgsJSON: `{}`, CalledAt: now.Add(300 * time.Millisecond)})
	core.Publish(bus, core.RunStartedEvent{RunID: "unrelated-1", Prompt: "other", StartedAt: now.Add(350 * time.Millisecond)})
	core.Publish(bus, core.ToolCalledEvent{RunID: "unrelated-1", ToolCallID: "other-tool", ToolName: "view", ArgsJSON: `{}`, CalledAt: now.Add(360 * time.Millisecond)})

	core.Publish(bus, core.RunStartedEvent{RunID: "child-1", ParentRunID: "run-1", Prompt: "child", StartedAt: now.Add(400 * time.Millisecond)})
	core.Publish(bus, core.ModelRequestStartedEvent{RunID: "child-1", ParentRunID: "run-1", TurnNumber: 1, MessageCount: 1, StartedAt: now.Add(500 * time.Millisecond)})
	core.Publish(bus, core.ModelResponseCompletedEvent{RunID: "child-1", ParentRunID: "run-1", TurnNumber: 1, FinishReason: "stop", DurationMs: 100, CompletedAt: now.Add(600 * time.Millisecond)})
	core.Publish(bus, core.ToolCalledEvent{RunID: "child-1", ParentRunID: "run-1", ToolCallID: "child-tool-1", ToolName: "view", ArgsJSON: `{}`, CalledAt: now.Add(700 * time.Millisecond)})
	core.Publish(bus, core.ToolCompletedEvent{RunID: "child-1", ParentRunID: "run-1", ToolCallID: "child-tool-1", ToolName: "view", Result: "ok", DurationMs: 100, CompletedAt: now.Add(800 * time.Millisecond)})
	core.Publish(bus, core.RunCompletedEvent{RunID: "child-1", ParentRunID: "run-1", Success: true, StartedAt: now.Add(400 * time.Millisecond), CompletedAt: now.Add(900 * time.Millisecond)})

	core.Publish(bus, core.ToolCompletedEvent{RunID: "run-1", ToolCallID: "delegate-1", ToolName: "delegate", Result: "child done", DurationMs: 700, CompletedAt: now.Add(time.Second)})
	core.Publish(bus, core.RunCompletedEvent{RunID: "run-1", Success: true, StartedAt: now, CompletedAt: now.Add(2 * time.Second)})

	runtimeEvents := recorder.EventsForTrace("run-1")
	if hasEventForAgent(runtimeEvents, "model.requested", "run-1") {
		t.Fatalf("root model boundary should be filtered from runtime duplicate events: %+v", runtimeEvents)
	}
	if hasEventForAgent(runtimeEvents, "run.started", "unrelated-1") || hasEventForAgent(runtimeEvents, "tool.called", "unrelated-1") {
		t.Fatalf("unrelated run events should be excluded from runtime events: %+v", runtimeEvents)
	}
	for _, want := range []struct {
		kind  string
		agent string
	}{
		{"run.started", "child-1"},
		{"model.requested", "child-1"},
		{"model.responded", "child-1"},
		{"tool.called", "child-1"},
		{"tool.completed", "child-1"},
		{"run.completed", "child-1"},
	} {
		if !hasEventForAgent(runtimeEvents, want.kind, want.agent) {
			t.Fatalf("missing nested %s for %s in %+v", want.kind, want.agent, runtimeEvents)
		}
	}

	artifact, err := FromRunTraceWithSnapshotsAndEvents(sampleRunTrace(now), nil, runtimeEvents, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshotsAndEvents() error = %v", err)
	}
	if err := ValidateReplay(artifact); err != nil {
		t.Fatalf("nested runtime events should strict-replay: %v\n%+v", err, artifact.Events)
	}
}

func TestFromTemporalWorkflowStatus(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snapshot, err := core.EncodeRunSnapshot(&core.RunSnapshot{
		RunID:        "workflow-run",
		RunStep:      3,
		RunStartTime: now,
		Prompt:       "durable prompt",
		Timestamp:    now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("EncodeRunSnapshot() error = %v", err)
	}
	status := &temporalext.WorkflowStatus{
		RunID:              "workflow-run",
		RunStep:            3,
		Usage:              core.RunUsage{Requests: 2},
		WorkflowName:       "agent__demo__workflow",
		RegistrationName:   "demo",
		Version:            "v1",
		TemporalWorkflowID: "wf-status",
		TemporalRunID:      "run-status",
		TemporalRunChain:   []string{"run-a", "run-status"},
		Snapshot:           snapshot,
		Cost:               &core.RunCost{TotalCost: 1.25, Currency: "USD"},
		Completed:          true,
		TraceExport:        &temporalext.TraceExportStatus{Attempted: true, Total: 2, Succeeded: 1, Failed: 1},
	}

	artifact, err := FromTemporalWorkflowStatus(status, map[string]any{"temporal_workflow_id": "wf-1"})
	if err != nil {
		t.Fatalf("FromTemporalWorkflowStatus() error = %v", err)
	}
	if artifact.Run.Mode != "temporal" {
		t.Fatalf("mode = %q, want temporal", artifact.Run.Mode)
	}
	if artifact.Run.ID != "workflow-run" {
		t.Fatalf("run id = %q, want workflow-run", artifact.Run.ID)
	}
	if artifact.Run.Prompt != "durable prompt" {
		t.Fatalf("prompt = %q, want durable prompt", artifact.Run.Prompt)
	}
	if len(artifact.Snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(artifact.Snapshots))
	}
	if artifact.Metadata["temporal_workflow_name"] != "agent__demo__workflow" {
		t.Fatalf("metadata missing workflow name: %+v", artifact.Metadata)
	}
	if artifact.Metadata["temporal_workflow_id"] != "wf-status" || artifact.Metadata["temporal_run_id"] != "run-status" {
		t.Fatalf("metadata missing temporal run identity: %+v", artifact.Metadata)
	}
	if artifact.Metadata["temporal_trace_export_failed"] != 1 {
		t.Fatalf("metadata missing trace export failure count: %+v", artifact.Metadata)
	}
	if artifact.Summary.Cost == nil || artifact.Summary.Cost.TotalCost != 1.25 {
		t.Fatalf("cost missing from summary: %+v", artifact.Summary.Cost)
	}
}

func TestOrchestratorArtifactSpecProducesLoadableTraceArtifact(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	spec, ok, err := OrchestratorArtifactSpec(&orchestrator.Task{
		ID:      "task-1",
		Kind:    "team",
		Attempt: 2,
		Run: &orchestrator.RunRef{
			ID:       "orch-run-1",
			WorkerID: "worker-1",
			Attempt:  2,
		},
	}, &core.RunResult[string]{
		RunID: "run-1",
		Trace: sampleRunTrace(now),
		Cost:  &core.RunCost{TotalCost: 0.25, Currency: "USD"},
	}, map[string]any{"team_name": "coding-team"})
	if err != nil {
		t.Fatalf("OrchestratorArtifactSpec() error = %v", err)
	}
	if !ok {
		t.Fatal("expected artifact spec for traced result")
	}
	if spec.Kind != OrchestratorArtifactKind {
		t.Fatalf("kind = %q, want %q", spec.Kind, OrchestratorArtifactKind)
	}
	artifact, err := Read(bytes.NewReader(spec.Body))
	if err != nil {
		t.Fatalf("read artifact body: %v", err)
	}
	if artifact.Run.Mode != "orchestrator" {
		t.Fatalf("mode = %q, want orchestrator", artifact.Run.Mode)
	}
	if artifact.Metadata["orchestrator_task_id"] != "task-1" {
		t.Fatalf("missing task metadata: %+v", artifact.Metadata)
	}
	if artifact.Summary.Cost == nil || artifact.Summary.Cost.TotalCost != 0.25 {
		t.Fatalf("missing cost: %+v", artifact.Summary.Cost)
	}
}

func TestFromTemporalWorkflowStatusIncludesWaitingEvents(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snapshot, err := core.EncodeRunSnapshot(&core.RunSnapshot{
		RunID:        "workflow-run",
		RunStep:      3,
		RunStartTime: now,
		Prompt:       "durable prompt",
		Timestamp:    now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("EncodeRunSnapshot() error = %v", err)
	}
	status := &temporalext.WorkflowStatus{
		RunID:         "workflow-run",
		RunStep:       3,
		Snapshot:      snapshot,
		Waiting:       true,
		WaitingReason: "approval_and_deferred",
		PendingApprovals: []temporalext.ToolApprovalRequest{
			{ToolCallID: "approval-1", ToolName: "write", ArgsJSON: `{"path":"/tmp/out"}`},
		},
		DeferredRequests: []core.DeferredToolRequest{
			{ToolCallID: "deferred-1", ToolName: "human", ArgsJSON: `{}`},
		},
	}

	artifact, err := FromTemporalWorkflowStatus(status, nil)
	if err != nil {
		t.Fatalf("FromTemporalWorkflowStatus() error = %v", err)
	}
	for _, want := range []string{"approval.requested", "deferred.requested", "wait.started"} {
		if !hasEventKind(artifact, want) {
			t.Fatalf("missing %s in events %+v", want, artifact.Events)
		}
	}
	if artifact.Summary.Status != "waiting" {
		t.Fatalf("summary status = %q, want waiting", artifact.Summary.Status)
	}
	if err := ValidateReplay(artifact); err != nil {
		t.Fatalf("waiting approval/deferred trace should strict-replay: %v", err)
	}
}

func TestFromTemporalWorkflowStatusMarksRunningStatus(t *testing.T) {
	artifact, err := FromTemporalWorkflowStatus(&temporalext.WorkflowStatus{
		RunID:   "workflow-run",
		RunStep: 1,
	}, nil)
	if err != nil {
		t.Fatalf("FromTemporalWorkflowStatus() error = %v", err)
	}
	if artifact.Summary.Status != "running" {
		t.Fatalf("summary status = %q, want running", artifact.Summary.Status)
	}
}

func TestFromTemporalWorkflowStatusDoesNotDuplicateTraceBoundaryEvents(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	status := &temporalext.WorkflowStatus{
		RunID:         "workflow-run",
		RunStep:       3,
		Waiting:       true,
		WaitingReason: "approval",
		Trace: &core.RunTrace{
			RunID:     "workflow-run",
			Prompt:    "durable prompt",
			StartTime: now,
			EndTime:   now.Add(time.Second),
			Steps: []core.TraceStep{
				{
					Kind:      core.TraceApprovalRequested,
					Timestamp: now.Add(100 * time.Millisecond),
					Data: map[string]any{
						"tool_call_id": "approval-1",
						"tool_name":    "write",
						"args":         `{"path":"/tmp/out"}`,
					},
				},
				{
					Kind:      core.TraceRunWaiting,
					Timestamp: now.Add(200 * time.Millisecond),
					Data:      map[string]any{"reason": "approval"},
				},
			},
		},
		PendingApprovals: []temporalext.ToolApprovalRequest{
			{ToolCallID: "approval-1", ToolName: "write", ArgsJSON: `{"path":"/tmp/out"}`},
		},
	}

	artifact, err := FromTemporalWorkflowStatus(status, nil)
	if err != nil {
		t.Fatalf("FromTemporalWorkflowStatus() error = %v", err)
	}
	if got := countEventsByKind(artifact, "approval.requested"); got != 1 {
		t.Fatalf("approval.requested count = %d, want 1: %+v", got, artifact.Events)
	}
	if got := countEventsByKind(artifact, "wait.started"); got != 1 {
		t.Fatalf("wait.started count = %d, want 1: %+v", got, artifact.Events)
	}
}

func TestInspectIncludesOperationalSummary(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := FromRunTrace(sampleRunTrace(now), nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}

	var buf bytes.Buffer
	if err := Inspect(&buf, artifact, InspectOptions{EventsLimit: 3}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Trace run-1",
		"schema: gollem.trace.v1",
		"status: succeeded",
		"snapshots: 0",
		"requests: 1",
		"tools: 1",
		"model.requested",
		"... 3 more events",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, out)
		}
	}
}

func TestInspectAndReplayErrorBranches(t *testing.T) {
	if err := Inspect(&bytes.Buffer{}, nil, InspectOptions{}); err == nil {
		t.Fatal("expected nil inspect artifact error")
	}
	if err := ReplayWithOptions(&bytes.Buffer{}, nil, ReplayOptions{}); err == nil {
		t.Fatal("expected nil replay artifact error")
	}
	if _, err := BuildReplayState(nil, ReplayOptions{}); err == nil {
		t.Fatal("expected nil replay state error")
	}

	artifact := &Artifact{
		SchemaVersion: SchemaVersion,
		Run: RunMetadata{
			ID:        "run-error",
			Prompt:    strings.Repeat("x", 220),
			StartedAt: time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
		},
		Summary: Summary{
			Status:         "failed",
			Error:          "boom",
			DurationMillis: 1500,
			Cost:           &core.RunCost{TotalCost: 0.0123},
		},
		Events: []Event{{Seq: 1, Kind: "error.raised", Payload: map[string]any{"error": "boom"}}},
	}
	var buf bytes.Buffer
	if err := Inspect(&buf, artifact, InspectOptions{EventsLimit: 20}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	for _, want := range []string{"error: boom", "duration: 1.5s", "cost: 0.012300 USD", "prompt:"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("inspect output missing %q:\n%s", want, buf.String())
		}
	}
	if err := ReplayWithOptions(&bytes.Buffer{}, artifact, ReplayOptions{Mode: "unknown"}); err == nil {
		t.Fatal("expected unsupported replay mode error")
	}
	if _, err := BuildReplayState(artifact, ReplayOptions{Mode: "unknown"}); err == nil {
		t.Fatal("expected unsupported replay state mode error")
	}
	if err := Replay(&bytes.Buffer{}, artifact); err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	corruptSnapshot := *artifact
	corruptSnapshot.Snapshots = []SnapshotRecord{{ID: "snap-bad"}}
	if _, err := BuildReplayState(&corruptSnapshot, ReplayOptions{Mode: "strict"}); err == nil {
		t.Fatal("expected corrupt snapshot restore error")
	}
}

func TestDiffFindsFirstDivergenceAndUsageDelta(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	baselineTrace := sampleRunTrace(now)
	variantTrace := sampleRunTrace(now)
	variantTrace.RunID = "run-2"
	variantTrace.Requests[0].ModelName = "variant-model"
	variantTrace.Requests[0].Response.ModelName = "variant-model"
	variantTrace.Usage.InputTokens = 20
	variantTrace.Usage.OutputTokens = 10
	variantTrace.Steps[2].Data = map[string]any{"tool_name": "bash", "args": `{"cmd":"go test ./..."}`}
	variantTrace.Steps = append(variantTrace.Steps,
		core.TraceStep{Kind: core.TraceRetryScheduled, Timestamp: now.Add(1100 * time.Millisecond), Data: map[string]any{"reason": "validation"}},
		core.TraceStep{Kind: core.TraceTopologyTransitioned, Timestamp: now.Add(1200 * time.Millisecond), Data: map[string]any{"from": "solo", "to": "team"}},
		core.TraceStep{Kind: core.TraceArtifactChanged, Timestamp: now.Add(1300 * time.Millisecond), Data: map[string]any{"path": "main.go", "operation": "modified"}},
	)

	baseline, err := FromRunTrace(baselineTrace, map[string]any{"evaluator": map[string]any{"name": "tests", "score": 0.5, "passed": true}})
	if err != nil {
		t.Fatalf("FromRunTrace(baseline) error = %v", err)
	}
	WithCost(baseline, &core.RunCost{TotalCost: 2.00, Currency: "USD"})
	variant, err := FromRunTrace(variantTrace, map[string]any{"evaluator": map[string]any{"name": "tests", "score": 0.75, "passed": true}})
	if err != nil {
		t.Fatalf("FromRunTrace(variant) error = %v", err)
	}
	WithCost(variant, &core.RunCost{TotalCost: 1.50, Currency: "USD"})

	diff := Diff(baseline, variant)
	if diff.FirstDivergence == nil {
		t.Fatal("expected first divergence")
	}
	if diff.CausalDivergence == nil {
		t.Fatalf("expected causal divergence: %+v", diff)
	}
	if diff.CausalGraphDelta == nil || diff.CausalGraphDelta.FirstEdgeDivergence == nil {
		t.Fatalf("expected causal graph divergence: %+v", diff.CausalGraphDelta)
	}
	if !diff.SemanticDelta.Changed || !diff.SemanticDelta.ToolSequenceChanged {
		t.Fatalf("expected semantic tool sequence delta: %+v", diff.SemanticDelta)
	}
	if diff.FirstDivergence.BaselineKind != "model.requested" || diff.FirstDivergence.VariantKind != "model.requested" {
		t.Fatalf("unexpected divergence: %+v", diff.FirstDivergence)
	}
	if diff.UsageDelta.InputTokens != 10 {
		t.Fatalf("input delta = %d, want 10", diff.UsageDelta.InputTokens)
	}
	if diff.UsageDelta.TotalTokens != 15 {
		t.Fatalf("total delta = %d, want 15", diff.UsageDelta.TotalTokens)
	}
	if diff.CostDelta != -0.5 {
		t.Fatalf("cost delta = %f, want -0.5", diff.CostDelta)
	}
	if diff.RetryErrorDelta.RetryScheduled != 1 {
		t.Fatalf("retry delta = %d, want 1", diff.RetryErrorDelta.RetryScheduled)
	}
	if len(diff.TopologyDelta) == 0 {
		t.Fatalf("expected topology delta: %+v", diff)
	}
	if len(diff.ArtifactDelta) == 0 {
		t.Fatalf("expected artifact delta: %+v", diff)
	}
	if diff.EvaluatorDelta == nil || diff.EvaluatorDelta.ScoreDelta == nil || *diff.EvaluatorDelta.ScoreDelta != 0.25 {
		t.Fatalf("expected evaluator score delta 0.25, got %+v", diff.EvaluatorDelta)
	}
	if len(diff.Narrative) == 0 {
		t.Fatal("expected diff narrative")
	}
	var buf bytes.Buffer
	if err := WriteDiff(&buf, diff); err != nil {
		t.Fatalf("WriteDiff() error = %v", err)
	}
	for _, want := range []string{"causal divergence:", "causal graph:", "semantic delta:"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("diff output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestEvaluateTraceAddsEvaluatorEvidence(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := FromRunTrace(sampleRunTrace(now), nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	evaluated, err := EvaluateTrace(artifact, EvaluateOptions{Evaluator: "contains-output", Expected: "ok"})
	if err != nil {
		t.Fatalf("EvaluateTrace() error = %v", err)
	}
	if evaluated.Summary.Evaluator == nil || evaluated.Summary.Evaluator.Passed == nil || !*evaluated.Summary.Evaluator.Passed {
		t.Fatalf("missing passing evaluator summary: %+v", evaluated.Summary.Evaluator)
	}
	if countEventsByKind(evaluated, "evaluator.completed") != 1 {
		t.Fatalf("expected evaluator.completed event: %+v", evaluated.Events)
	}
	if err := ValidateArtifact(evaluated); err != nil {
		t.Fatalf("ValidateArtifact(evaluated) error = %v", err)
	}

	if _, err := EvaluateTrace(artifact, EvaluateOptions{Evaluator: "contains-output"}); err == nil || !strings.Contains(err.Error(), "requires --expected") {
		t.Fatalf("expected missing expected error, got %v", err)
	}
	if _, err := EvaluateTrace(artifact, EvaluateOptions{Evaluator: "missing"}); err == nil || !strings.Contains(err.Error(), "unknown trace evaluator") {
		t.Fatalf("expected unknown evaluator error, got %v", err)
	}
}

func TestRegressAppliesTraceBackedThresholds(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	baselineTrace := sampleRunTrace(now)
	variantTrace := sampleRunTrace(now)
	variantTrace.RunID = "run-variant"
	variantTrace.Usage.InputTokens = baselineTrace.Usage.InputTokens + 10
	variantTrace.Usage.OutputTokens = baselineTrace.Usage.OutputTokens + 5

	baseline, err := FromRunTrace(baselineTrace, nil)
	if err != nil {
		t.Fatalf("FromRunTrace(baseline) error = %v", err)
	}
	variant, err := FromRunTrace(variantTrace, nil)
	if err != nil {
		t.Fatalf("FromRunTrace(variant) error = %v", err)
	}
	maxTokens := 5
	report := Regress(baseline, []*Artifact{variant}, RegressionOptions{
		RequireStatus: "succeeded",
		MaxTokenDelta: &maxTokens,
	})
	if report.Passed {
		t.Fatalf("expected regression failure: %+v", report)
	}
	if len(report.Cases) != 1 || len(report.Cases[0].Failures) == 0 {
		t.Fatalf("expected case failure: %+v", report)
	}
}

func TestRegressionReportWritersIncludeFailuresAndEvaluator(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	baseScore := 0.5
	variantScore := 0.8
	baseline, err := FromRunTrace(sampleRunTrace(now), map[string]any{"evaluator": map[string]any{"score": baseScore, "passed": true}})
	if err != nil {
		t.Fatalf("baseline trace: %v", err)
	}
	variantTrace := sampleRunTrace(now)
	variantTrace.RunID = "variant-regress"
	variantTrace.Success = false
	variantTrace.Error = "failed"
	variantTrace.Usage.OutputTokens += 20
	variant, err := FromRunTrace(variantTrace, map[string]any{"evaluator": map[string]any{"score": variantScore, "passed": false}})
	if err != nil {
		t.Fatalf("variant trace: %v", err)
	}
	WithCost(baseline, &core.RunCost{TotalCost: 0.01})
	WithCost(variant, &core.RunCost{TotalCost: 0.20})
	maxTokens := 1
	maxCost := 0.01
	report := Regress(baseline, []*Artifact{variant}, RegressionOptions{
		RequireStatus: "succeeded",
		MaxTokenDelta: &maxTokens,
		MaxCostDelta:  &maxCost,
	})
	if report.Passed || len(report.Cases) != 1 || len(report.Cases[0].Failures) < 3 {
		t.Fatalf("expected failed regression report: %+v", report)
	}
	var text bytes.Buffer
	if err := WriteRegressionReport(&text, report); err != nil {
		t.Fatalf("WriteRegressionReport() error = %v", err)
	}
	for _, want := range []string{"Trace regression: failed", "failures:", "evaluator score:"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, text.String())
		}
	}
	var jsonOut bytes.Buffer
	if err := WriteRegressionReportJSON(&jsonOut, report); err != nil {
		t.Fatalf("WriteRegressionReportJSON() error = %v", err)
	}
	if !strings.Contains(jsonOut.String(), `"passed": false`) {
		t.Fatalf("json report missing failure:\n%s", jsonOut.String())
	}
}

func TestSleepyEvidenceRanksCandidatesAndPreservesReplayLineage(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	baseline, err := FromRunTrace(sampleRunTrace(now), map[string]any{"evaluator": map[string]any{"name": "tests", "score": 0.5, "passed": true}})
	if err != nil {
		t.Fatalf("FromRunTrace(baseline) error = %v", err)
	}
	candidateTrace := sampleRunTrace(now)
	candidateTrace.RunID = "candidate-run"
	candidate, err := FromRunTrace(candidateTrace, map[string]any{
		"sleepy_candidate_id":        "cand-1",
		"resume_source_trace_run_id": baseline.Run.ID,
		"resume_source_snapshot_id":  "snap_000001",
		"resume_parent_run_id":       baseline.Run.ID,
		"evaluator":                  map[string]any{"name": "tests", "score": 0.75, "passed": true},
	})
	if err != nil {
		t.Fatalf("FromRunTrace(candidate) error = %v", err)
	}
	evidence, err := BuildSleepyEvidence(baseline, []*Artifact{candidate})
	if err != nil {
		t.Fatalf("BuildSleepyEvidence() error = %v", err)
	}
	if evidence.SchemaVersion != SleepyEvidenceSchemaVersion {
		t.Fatalf("schema = %q", evidence.SchemaVersion)
	}
	if len(evidence.Candidates) != 1 || len(evidence.Ranking) != 1 {
		t.Fatalf("unexpected evidence shape: %+v", evidence)
	}
	row := evidence.Candidates[0]
	if row.CandidateID != "cand-1" || !row.Replayable {
		t.Fatalf("unexpected candidate row: %+v", row)
	}
	if row.Lineage.SourceTraceRunID != baseline.Run.ID || row.Lineage.SourceSnapshotID != "snap_000001" {
		t.Fatalf("missing lineage: %+v", row.Lineage)
	}
	if evidence.Ranking[0].CandidateID != "cand-1" {
		t.Fatalf("unexpected ranking: %+v", evidence.Ranking)
	}
	var buf bytes.Buffer
	if err := WriteSleepyEvidence(&buf, evidence); err != nil {
		t.Fatalf("WriteSleepyEvidence() error = %v", err)
	}
	if !strings.Contains(buf.String(), SleepyEvidenceSchemaVersion) {
		t.Fatalf("serialized evidence missing schema:\n%s", buf.String())
	}
}

func TestForkSnapshotErrorBranchesAndSelectionHelpers(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if _, _, err := ForkSnapshot(nil, ForkOptions{}); err == nil {
		t.Fatal("expected nil artifact error")
	}
	empty := &Artifact{Run: RunMetadata{ID: "empty"}}
	if _, _, err := ForkSnapshot(empty, ForkOptions{FromStep: 1}); err == nil {
		t.Fatal("expected no snapshots error")
	}

	snap3 := &core.RunSnapshot{RunID: "run-fork", RunStep: 3, Prompt: "resume", Timestamp: now.Add(3 * time.Second)}
	snap5 := &core.RunSnapshot{RunID: "run-fork", RunStep: 5, Prompt: "resume", Timestamp: now.Add(5 * time.Second)}
	artifact, err := FromRunTraceWithSnapshots(&core.RunTrace{
		RunID:     "run-fork",
		StartTime: now,
		EndTime:   now.Add(6 * time.Second),
		Success:   true,
		Steps: []core.TraceStep{
			{Kind: core.TraceCheckpointCreated, Timestamp: now.Add(4 * time.Second), Data: map[string]any{"checkpoint_id": "manual", "run_step": 4}},
		},
	}, []*core.RunSnapshot{snap3, snap5}, nil)
	if err != nil {
		t.Fatalf("FromRunTraceWithSnapshots() error = %v", err)
	}
	if _, err := selectSnapshot(nil, 1); err == nil {
		t.Fatal("expected selectSnapshot empty error")
	}
	if record, err := selectSnapshot(artifact.Snapshots, 0); err != nil || record.Step != 5 {
		t.Fatalf("select latest = %+v err=%v", record, err)
	}
	if record, err := selectSnapshot(artifact.Snapshots, 4); err != nil || record.Step != 3 {
		t.Fatalf("select prior = %+v err=%v", record, err)
	}
	if _, err := selectSnapshot([]SnapshotRecord{artifact.Snapshots[1]}, 1); err == nil {
		t.Fatal("expected no prior snapshot error")
	}
	if _, err := selectSnapshotByCheckpoint(artifact, ""); err == nil {
		t.Fatal("expected empty checkpoint error")
	}
	if _, err := selectSnapshotByCheckpoint(artifact, "missing"); err == nil {
		t.Fatal("expected missing checkpoint error")
	}
	if _, err := resolveForkStep(artifact, ForkOptions{FromEventID: "missing"}); err == nil {
		t.Fatal("expected missing event error")
	}
	if _, err := resolveForkStep(artifact, ForkOptions{FromKind: "missing.kind"}); err == nil {
		t.Fatal("expected missing kind error")
	}
	if got := payloadString(map[string]any{"k": " v "}, "k"); got != "v" {
		t.Fatalf("payloadString = %q", got)
	}
	if got := payloadString(nil, "k"); got != "" {
		t.Fatalf("empty payloadString = %q", got)
	}
	if messages := replaceSystemPrompt(nil, ""); len(messages) != 0 {
		t.Fatalf("empty system prompt should not alter messages: %+v", messages)
	}
	if messages := replaceSystemPrompt([]core.ModelMessage{core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "ok"}}}}, "system"); len(messages) != 2 {
		t.Fatalf("expected prepended system prompt, got %+v", messages)
	}
	if messages := appendPlannerPrompt(nil, ""); len(messages) != 0 {
		t.Fatalf("empty planner prompt should not alter messages: %+v", messages)
	}
	if err := WriteSnapshotFile("", &core.RunSnapshot{}); err == nil {
		t.Fatal("expected empty snapshot output path error")
	}
	applyForkOverrides(nil, ForkOptions{Model: "ignored"})
}

func TestSleepyEvidenceFlagsGamingAndWritesFile(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	baseline, err := FromRunTrace(sampleRunTrace(now), map[string]any{
		"evaluator": map[string]any{"name": "tests", "score": 0.2, "passed": true},
	})
	if err != nil {
		t.Fatalf("FromRunTrace(baseline) error = %v", err)
	}

	failedTrace := sampleRunTrace(now)
	failedTrace.RunID = "failed-candidate"
	failedTrace.Success = false
	failedTrace.Error = "tests failed"
	candidate, err := FromRunTrace(failedTrace, map[string]any{
		"mutation_id": "mut-1",
		"evaluator":   map[string]any{"name": "tests", "score": 0.9, "passed": true},
	})
	if err != nil {
		t.Fatalf("FromRunTrace(candidate) error = %v", err)
	}

	evidence, err := BuildSleepyEvidence(baseline, []*Artifact{nil, candidate})
	if err != nil {
		t.Fatalf("BuildSleepyEvidence() error = %v", err)
	}
	if len(evidence.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(evidence.Candidates))
	}
	flags := strings.Join(evidence.Candidates[0].EvaluatorGamingFlags, "\n")
	for _, want := range []string{"evaluator passed but run did not succeed", "positive evaluator score despite runtime failures"} {
		if !strings.Contains(flags, want) {
			t.Fatalf("missing flag %q in %+v", want, evidence.Candidates[0].EvaluatorGamingFlags)
		}
	}
	if evidence.Ranking[0].Reason == "" || !strings.Contains(evidence.Ranking[0].Reason, "flagged") {
		t.Fatalf("expected flagged ranking reason, got %+v", evidence.Ranking[0])
	}

	out := t.TempDir() + "/sleepy.json"
	if err := WriteSleepyEvidenceFile(out, evidence); err != nil {
		t.Fatalf("WriteSleepyEvidenceFile() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read sleepy evidence: %v", err)
	}
	if !strings.Contains(string(data), "mut-1") {
		t.Fatalf("evidence file missing mutation id:\n%s", string(data))
	}

	empty, err := BuildSleepyEvidence(baseline, nil)
	if err != nil {
		t.Fatalf("BuildSleepyEvidence(empty) error = %v", err)
	}
	if len(empty.Notes) == 0 {
		t.Fatalf("expected empty-candidate note: %+v", empty)
	}
	if _, err := BuildSleepyEvidence(nil, nil); err == nil {
		t.Fatal("expected nil baseline error")
	}
	if err := WriteSleepyEvidence(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected nil evidence error")
	}
}

func TestSleepyCandidateRankingTieBreakers(t *testing.T) {
	passed := true
	failed := false
	highScore := 0.9
	midScore := 0.8
	lowScore := 0.1
	ranks := rankSleepyCandidates([]SleepyCandidateEvidence{
		{CandidateID: "failed-high", RunID: "failed-high", Passed: &failed, Score: &highScore, Tokens: 1, Cost: 0.01},
		{CandidateID: "passed-low", RunID: "passed-low", Passed: &passed, Score: &lowScore, Tokens: 1, Cost: 0.01},
		{CandidateID: "passed-mid-expensive", RunID: "passed-mid-expensive", Passed: &passed, Score: &midScore, Tokens: 3, Cost: 0.04},
		{CandidateID: "passed-mid-cheap-z", RunID: "z-run", Passed: &passed, Score: &midScore, Tokens: 3, Cost: 0.01},
		{CandidateID: "passed-mid-cheap-a", RunID: "a-run", Passed: &passed, Score: &midScore, Tokens: 3, Cost: 0.01},
		{CandidateID: "passed-mid-fewer-tokens", RunID: "passed-mid-fewer", Passed: &passed, Score: &midScore, Tokens: 2, Cost: 0.02},
		{CandidateID: "nil-score", RunID: "nil-score"},
	})
	wantOrder := []string{
		"passed-mid-fewer-tokens",
		"passed-mid-cheap-a",
		"passed-mid-cheap-z",
		"passed-mid-expensive",
		"passed-low",
		"failed-high",
		"nil-score",
	}
	if len(ranks) != len(wantOrder) {
		t.Fatalf("ranks = %+v", ranks)
	}
	for i, want := range wantOrder {
		if ranks[i].CandidateID != want || ranks[i].Rank != i+1 {
			t.Fatalf("rank[%d] = %+v, want %s", i, ranks[i], want)
		}
		if !strings.Contains(ranks[i].Reason, "ranked by evaluator") {
			t.Fatalf("unexpected rank reason: %+v", ranks[i])
		}
	}
}

func TestJSONLStreamWriterWritesCanonicalRuntimeEvents(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	stream, err := NewJSONLStreamWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLStreamWriter() error = %v", err)
	}
	stream.WriteEvent(Event{Kind: "run.started", AgentID: "run-1"})
	stream.WriteEvent(Event{Kind: "run.completed", AgentID: "run-1"})
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"id":"live_evt_000001"`, `"seq":2`, `"kind":"run.completed"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("stream missing %q:\n%s", want, text)
		}
	}
}

func TestJSONLStreamWriterLifecycleAndReplayPolicies(t *testing.T) {
	if _, err := NewJSONLStreamWriter(""); err == nil {
		t.Fatal("expected empty path error")
	}

	path := t.TempDir() + "/nested/events.jsonl"
	stream, err := NewJSONLStreamWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLStreamWriter() error = %v", err)
	}
	stream.WriteEvent(Event{ID: "evt-custom", Kind: "checkpoint.created"})
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	stream.WriteEvent(Event{Kind: "model.responded"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"id":"evt-custom"`, `"replay_policy":"checkpoint"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("stream missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "\n") != 1 {
		t.Fatalf("expected one event after close, got:\n%s", text)
	}

	for kind, want := range map[string]string{
		"snapshot.created": "snapshot",
		"run.started":      "inspect",
		"model.responded":  "recorded",
	} {
		if got := streamReplayPolicy(kind); got != want {
			t.Fatalf("streamReplayPolicy(%q) = %q, want %q", kind, got, want)
		}
	}
	var nilStream *JSONLStreamWriter
	nilStream.WriteEvent(Event{Kind: "run.started"})
	if err := nilStream.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
}

func TestReplayStrictValidatesRecordedBoundaries(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	snap := &core.RunSnapshot{
		Messages: []core.ModelMessage{
			core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: "fix the failing tests", Timestamp: now},
				},
				Timestamp: now,
			},
		},
		RunID:        "run-1",
		RunStep:      1,
		RunStartTime: now,
		Prompt:       "fix the failing tests",
		Timestamp:    now.Add(time.Second),
	}
	artifact, err := FromRunTraceWithSnapshots(sampleRunTrace(now), []*core.RunSnapshot{snap}, nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	var buf bytes.Buffer
	if err := ReplayWithOptions(&buf, artifact, ReplayOptions{Mode: "strict"}); err != nil {
		t.Fatalf("ReplayWithOptions() error = %v", err)
	}
	if !strings.Contains(buf.String(), "strict replay validation: ok") {
		t.Fatalf("missing strict validation line:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "reconstructed: boundaries=") || !strings.Contains(buf.String(), "restored_snapshot=snap_000001") {
		t.Fatalf("missing reconstructed state line:\n%s", buf.String())
	}
	state, err := BuildReplayState(artifact, ReplayOptions{Mode: "simulated"})
	if err != nil {
		t.Fatalf("BuildReplayState() error = %v", err)
	}
	if state.RestoredSnapshotID != "snap_000001" {
		t.Fatalf("restored snapshot = %q, want snap_000001", state.RestoredSnapshotID)
	}
	if len(state.Boundaries) != len(artifact.Events) {
		t.Fatalf("boundaries = %d, want %d", len(state.Boundaries), len(artifact.Events))
	}
	if len(state.Messages) == 0 {
		t.Fatalf("expected replay messages: %+v", state)
	}

	broken := *artifact
	broken.Events = append([]Event(nil), artifact.Events...)
	for i, event := range broken.Events {
		if event.Kind == "model.responded" {
			broken.Events = append(broken.Events[:i], broken.Events[i+1:]...)
			break
		}
	}
	if err := ValidateReplay(&broken); err == nil || !strings.Contains(err.Error(), "no recorded response") {
		t.Fatalf("expected strict replay validation error, got %v", err)
	}
}

func TestValidateReplayAllowsWaitingBoundariesAndRejectsOpenTools(t *testing.T) {
	waitingApproval := &Artifact{
		Summary: Summary{Status: "waiting"},
		Events: []Event{
			{Seq: 1, Kind: "approval.requested", Payload: map[string]any{"tool_call_id": "call-approval"}},
		},
	}
	if err := ValidateReplay(waitingApproval); err != nil {
		t.Fatalf("waiting approval should validate: %v", err)
	}

	waitingDeferred := &Artifact{
		Summary: Summary{Status: "waiting"},
		Events: []Event{
			{Seq: 1, Kind: "deferred.requested", Payload: map[string]any{"tool_call_id": "call-deferred"}},
		},
	}
	if err := ValidateReplay(waitingDeferred); err != nil {
		t.Fatalf("waiting deferred should validate: %v", err)
	}

	for _, tt := range []struct {
		name   string
		events []Event
		want   string
	}{
		{
			name:   "tool",
			events: []Event{{Seq: 1, Kind: "tool.called", Payload: map[string]any{"tool_call_id": "call-tool"}}},
			want:   "tool call",
		},
		{
			name:   "approval",
			events: []Event{{Seq: 1, Kind: "approval.requested", Payload: map[string]any{"tool_call_id": "call-approval"}}},
			want:   "approval request",
		},
		{
			name:   "deferred",
			events: []Event{{Seq: 1, Kind: "deferred.requested", Payload: map[string]any{"tool_call_id": "call-deferred"}}},
			want:   "deferred request",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReplay(&Artifact{Summary: Summary{Status: "running"}, Events: tt.events})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateReplay() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReplayPayloadCoercionAndBoundaryMessages(t *testing.T) {
	payload := map[string]any{
		"score":              json.Number("0.75"),
		"passed":             "yes",
		"input_tokens":       "7",
		"output_tokens":      int64(3),
		"cache_read_tokens":  float64(2),
		"cache_write_tokens": json.Number("1"),
	}
	score := payloadFloatPointer(payload, "score")
	if score == nil || *score != 0.75 {
		t.Fatalf("score = %v, want 0.75", score)
	}
	passed := payloadBoolPointer(payload, "passed")
	if passed == nil || !*passed {
		t.Fatalf("passed = %v, want true", passed)
	}
	usage, ok := payloadUsage(payload, "usage")
	if !ok || usage.InputTokens != 7 || usage.OutputTokens != 3 || usage.CacheReadTokens != 2 || usage.CacheWriteTokens != 1 {
		t.Fatalf("usage = %+v ok=%v", usage, ok)
	}
	if got := payloadInt(map[string]any{"n": "42"}, "n"); got != 42 {
		t.Fatalf("payloadInt string = %d, want 42", got)
	}

	req := core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: "system"},
			core.UserPromptPart{Content: "user"},
			core.ToolReturnPart{ToolName: "shell", Content: "ok"},
			core.RetryPromptPart{Content: "retry"},
		},
	}
	content := replayRequestContent(req)
	for _, want := range []string{"system: system", "user", "tool shell: ok", "retry: retry"} {
		if !strings.Contains(content, want) {
			t.Fatalf("replay request content missing %q:\n%s", want, content)
		}
	}

	artifact := &Artifact{
		Trace: &core.RunTrace{
			Steps: []core.TraceStep{
				{Kind: core.TraceModelResponse, Data: map[string]any{"turn_number": 1, "text": "trace text"}},
			},
		},
	}
	boundary := replayBoundaryFromEvent(artifact, Event{
		Seq:     3,
		Kind:    "tool.completed",
		Step:    1,
		Payload: map[string]any{"data": map[string]string{"tool_name": "shell", "tool_call_id": "call-1", "result": "ok"}},
	})
	if boundary.ToolName != "shell" || boundary.ToolCallID != "call-1" || boundary.Result != "ok" {
		t.Fatalf("unexpected boundary: %+v", boundary)
	}
	modelBoundary := replayBoundaryFromEvent(artifact, Event{Kind: "model.responded", Step: 1, Payload: map[string]any{}})
	if modelBoundary.Text != "trace text" {
		t.Fatalf("model text = %q, want trace text", modelBoundary.Text)
	}
	messages := applyReplayBoundaryMessage(nil, ReplayBoundary{Kind: "tool.failed", Seq: 4, Error: "boom", ToolName: "shell"})
	if len(messages) != 1 || messages[0].Role != "tool" || messages[0].Content != "boom" {
		t.Fatalf("unexpected replay messages: %+v", messages)
	}
}

func TestReplayModes(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := FromRunTrace(sampleRunTrace(now), nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	for _, mode := range []string{"inspect", "strict", "simulated", "fork", "live-reexec"} {
		if !SupportedReplayMode(mode) {
			t.Fatalf("mode %q should be supported", mode)
		}
		var buf bytes.Buffer
		opts := ReplayOptions{Mode: mode}
		if mode == "live-reexec" {
			opts.LiveReexec = func(*ReplayState) error { return nil }
		}
		if err := ReplayWithOptions(&buf, artifact, opts); err != nil {
			t.Fatalf("ReplayWithOptions(%s) error = %v", mode, err)
		}
		if !strings.Contains(buf.String(), mode) {
			t.Fatalf("replay output for %s missing mode:\n%s", mode, buf.String())
		}
	}
	var buf bytes.Buffer
	if err := ReplayWithOptions(&buf, artifact, ReplayOptions{Mode: "live-reexec"}); err == nil || !strings.Contains(err.Error(), "live runner") {
		t.Fatalf("ReplayWithOptions(live-reexec without runner) error = %v", err)
	}
}

func TestValidateArtifactChecksStructuralInvariants(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := FromRunTrace(sampleRunTrace(now), nil)
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	if err := ValidateArtifact(artifact); err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}

	withRuntimeFailures := *artifact
	withRuntimeFailures.Events = append([]Event(nil), artifact.Events...)
	withRuntimeFailures.Events = append(withRuntimeFailures.Events,
		Event{
			ID:           "event-turn-failed",
			Seq:          len(withRuntimeFailures.Events) + 1,
			Kind:         "turn.failed",
			AgentID:      artifact.Run.ID,
			ReplayPolicy: "inspect",
		},
		Event{
			ID:           "event-guardrail",
			Seq:          len(withRuntimeFailures.Events) + 2,
			Kind:         "guardrail.evaluated",
			AgentID:      artifact.Run.ID,
			ReplayPolicy: "recorded",
			Payload:      map[string]any{"name": "policy", "passed": false},
		},
	)
	if err := ValidateArtifact(&withRuntimeFailures); err != nil {
		t.Fatalf("ValidateArtifact() rejected runtime failure/policy events: %v", err)
	}

	withResumeLineage := *artifact
	withResumeLineage.Metadata = map[string]any{"resume_source_trace_run_id": "source-run"}
	withResumeLineage.Events = append([]Event(nil), artifact.Events...)
	withResumeLineage.Events[0].CausalParentID = "source-run"
	if err := ValidateArtifact(&withResumeLineage); err != nil {
		t.Fatalf("ValidateArtifact() rejected resume source lineage: %v", err)
	}

	badSeq := *artifact
	badSeq.Events = append([]Event(nil), artifact.Events...)
	badSeq.Events[0].Seq = 99
	if err := ValidateArtifact(&badSeq); err == nil || !strings.Contains(err.Error(), "event sequence") {
		t.Fatalf("bad sequence error = %v", err)
	}

	badKind := *artifact
	badKind.Events = append([]Event(nil), artifact.Events...)
	badKind.Events[0].Kind = "missing.kind"
	if err := ValidateArtifact(&badKind); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("bad kind error = %v", err)
	}

	badPolicy := *artifact
	badPolicy.Events = append([]Event(nil), artifact.Events...)
	badPolicy.Events[0].ReplayPolicy = "recorded"
	if err := ValidateArtifact(&badPolicy); err == nil || !strings.Contains(err.Error(), "replay policy") {
		t.Fatalf("bad policy error = %v", err)
	}
}

func TestRedactAndCompactPreserveLoadableArtifact(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	runTrace := sampleRunTrace(now)
	runTrace.Steps[2].Data = map[string]any{
		"tool_name": "shell",
		"args":      `{"api_key":"secret-value","cmd":"echo secret-value"}`,
	}
	artifact, err := FromRunTrace(runTrace, map[string]any{"token": "secret-value"})
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}

	redacted, err := Redact(artifact, RedactOptions{Patterns: []string{"secret-value"}, DropTrace: true})
	if err != nil {
		t.Fatalf("Redact() error = %v", err)
	}
	var redactedJSON bytes.Buffer
	if err := Write(&redactedJSON, redacted); err != nil {
		t.Fatalf("write redacted: %v", err)
	}
	if strings.Contains(redactedJSON.String(), "secret-value") {
		t.Fatalf("redacted output still contains secret:\n%s", redactedJSON.String())
	}
	if _, err := Read(bytes.NewReader(redactedJSON.Bytes())); err != nil {
		t.Fatalf("read redacted artifact: %v", err)
	}

	compacted, err := Compact(artifact, CompactOptions{DropTrace: true, EventPayloadLimit: 24, KeepSnapshots: 0})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if compacted.Trace != nil {
		t.Fatal("expected compacted trace payload to be dropped")
	}
	if _, ok := compacted.Metadata["compacted"]; !ok {
		t.Fatalf("missing compacted metadata: %+v", compacted.Metadata)
	}
}

func TestRedactAndCompactLargeSensitivePayload(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	artifact, err := FromRunTrace(sampleRunTrace(now), map[string]any{
		"authorization": "Bearer secret-token",
	})
	if err != nil {
		t.Fatalf("FromRunTrace() error = %v", err)
	}
	artifact.Events[0].Payload = map[string]any{
		"api_key": "secret-token",
		"nested": map[string]any{
			"refresh_token": "secret-token",
			"body":          strings.Repeat("large-payload-", 200),
		},
	}
	stepData, ok := artifact.Trace.Steps[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("expected trace step data map, got %T", artifact.Trace.Steps[0].Data)
	}
	stepData["authorization"] = "Bearer secret-token"

	redacted, err := Redact(artifact, RedactOptions{
		Patterns:  []string{"secret-token"},
		DropTrace: true,
	})
	if err != nil {
		t.Fatalf("Redact() error = %v", err)
	}
	var redactedJSON bytes.Buffer
	if err := Write(&redactedJSON, redacted); err != nil {
		t.Fatalf("write redacted artifact: %v", err)
	}
	if strings.Contains(redactedJSON.String(), "secret-token") {
		t.Fatalf("redacted artifact still contains secret:\n%s", redactedJSON.String())
	}
	if redacted.Trace != nil {
		t.Fatal("expected redacted artifact to drop embedded trace")
	}
	if !redacted.Events[0].Redacted {
		t.Fatal("expected redacted event flag")
	}
	if redacted.Events[0].Redaction == nil || !redacted.Events[0].Redaction.Applied || redacted.Events[0].Redaction.Patterns != 1 {
		t.Fatalf("expected redaction metadata, got %+v", redacted.Events[0].Redaction)
	}

	compacted, err := Compact(artifact, CompactOptions{
		DropTrace:         true,
		EventPayloadLimit: 64,
		KeepSnapshots:     0,
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if compacted.Trace != nil {
		t.Fatal("expected compacted artifact to drop embedded trace")
	}
	payload := compacted.Events[0].Payload
	if payload["compacted"] != true {
		t.Fatalf("expected compacted payload marker, got %+v", payload)
	}
	preview, _ := payload["preview"].(string)
	if len(preview) > 80 {
		t.Fatalf("expected bounded preview, got %d bytes", len(preview))
	}
}

func hasEventKind(artifact *Artifact, kind string) bool {
	for _, event := range artifact.Events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func hasEventKindIn(events []Event, kind string) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func hasEventForAgent(events []Event, kind, agentID string) bool {
	for _, event := range events {
		if event.Kind == kind && event.AgentID == agentID {
			return true
		}
	}
	return false
}

func countEventsByKind(artifact *Artifact, kind string) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func sampleRunTrace(now time.Time) *core.RunTrace {
	return &core.RunTrace{
		RunID:     "run-1",
		Prompt:    "fix the failing tests",
		StartTime: now,
		EndTime:   now.Add(2 * time.Second),
		Duration:  2 * time.Second,
		Success:   true,
		Usage: core.RunUsage{
			Usage:     core.Usage{InputTokens: 10, OutputTokens: 5},
			Requests:  1,
			ToolCalls: 1,
		},
		Requests: []core.RequestTrace{
			{
				RequestID:    "run-1/request-1",
				TurnNumber:   1,
				Sequence:     1,
				ModelName:    "test-model",
				StartedAt:    now.Add(100 * time.Millisecond),
				EndedAt:      now.Add(500 * time.Millisecond),
				Duration:     400 * time.Millisecond,
				MessageCount: 2,
				Response: &core.RequestTraceResponse{
					ModelName: "test-model",
					Usage:     core.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
		},
		Steps: []core.TraceStep{
			{
				Kind:      core.TraceModelRequest,
				Timestamp: now.Add(100 * time.Millisecond),
				Data:      map[string]any{"message_count": 2},
			},
			{
				Kind:      core.TraceModelResponse,
				Timestamp: now.Add(500 * time.Millisecond),
				Duration:  400 * time.Millisecond,
				Data:      map[string]any{"text": "ok", "tool_calls": 1},
			},
			{
				Kind:      core.TraceToolCall,
				Timestamp: now.Add(700 * time.Millisecond),
				Data:      map[string]any{"tool_name": "shell", "args": `{"cmd":"go test ./..."}`},
			},
			{
				Kind:      core.TraceToolResult,
				Timestamp: now.Add(900 * time.Millisecond),
				Duration:  200 * time.Millisecond,
				Data:      map[string]any{"tool_name": "shell", "result": "ok"},
			},
		},
	}
}

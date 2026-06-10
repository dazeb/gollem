package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type testSnapshotStatefulTool struct {
	count int
}

func (t *testSnapshotStatefulTool) ExportState() (any, error) {
	return map[string]any{"count": t.count}, nil
}

func (t *testSnapshotStatefulTool) RestoreState(state any) error {
	switch s := state.(type) {
	case map[string]any:
		t.count = snapshotCountFromAny(s["count"])
	case map[string]int:
		t.count = s["count"]
	case int:
		t.count = s
	}
	return nil
}

func snapshotCountFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func TestSnapshot_CaptureAndRestore(t *testing.T) {
	startTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	rc := &RunContext{
		Usage:        RunUsage{Requests: 2, ToolCalls: 1},
		RunID:        "test-run",
		ParentRunID:  "parent-run",
		RunStep:      3,
		RunStartTime: startTime,
		Prompt:       "hello",
		Messages: []ModelMessage{
			ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "hello"}}},
		},
	}

	snap := Snapshot(rc)
	if snap.RunID != "test-run" {
		t.Errorf("expected run_id 'test-run', got %q", snap.RunID)
	}
	if snap.ParentRunID != "parent-run" {
		t.Errorf("expected parent_run_id 'parent-run', got %q", snap.ParentRunID)
	}
	if snap.RunStep != 3 {
		t.Errorf("expected run_step 3, got %d", snap.RunStep)
	}
	if snap.Prompt != "hello" {
		t.Errorf("expected prompt 'hello', got %q", snap.Prompt)
	}
	if !snap.RunStartTime.Equal(startTime) {
		t.Errorf("expected run_start_time %v, got %v", startTime, snap.RunStartTime)
	}
	if len(snap.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(snap.Messages))
	}

	// Verify that modifying the original RunContext doesn't affect the snapshot.
	rc.RunStep = 99
	if snap.RunStep != 3 {
		t.Error("snapshot should be independent of RunContext")
	}
}

func TestSnapshotUsesRichGetterFallbacksAndPublishesCheckpoint(t *testing.T) {
	startTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	base := RunContext{
		Usage:        RunUsage{Requests: 2},
		RunID:        "fallback-run",
		ParentRunID:  "fallback-parent",
		RunStep:      7,
		RunStartTime: startTime,
		Prompt:       "fallback prompt",
		Messages: []ModelMessage{
			ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "hello"}}},
		},
	}
	rc := NewRunContext(base, func() map[string]any {
		return map[string]any{"tool": map[string]any{"count": 1}}
	}, func() *RunStateSnapshot {
		return &RunStateSnapshot{
			Messages: []ModelMessage{ModelResponse{Parts: []ModelResponsePart{TextPart{Content: "ok"}}}},
		}
	})
	snap := Snapshot(rc)
	if snap.RunID != "fallback-run" || snap.ParentRunID != "fallback-parent" || snap.RunStep != 7 || snap.Prompt != "fallback prompt" {
		t.Fatalf("fallback fields not applied: %+v", snap)
	}
	if !snap.RunStartTime.Equal(startTime) {
		t.Fatalf("run start = %s, want %s", snap.RunStartTime, startTime)
	}
	if snap.ToolState["tool"] == nil {
		t.Fatalf("missing cloned tool state: %+v", snap.ToolState)
	}
	if Snapshot(nil) != nil {
		t.Fatal("nil run context should not snapshot")
	}

	bus := NewEventBus()
	defer bus.Close()
	events := make(chan CheckpointCreatedEvent, 1)
	unsub := Subscribe(bus, func(event CheckpointCreatedEvent) { events <- event })
	defer unsub()
	PublishCheckpointCreated(nil, snap)
	PublishCheckpointCreated(bus, nil)
	snap.Timestamp = time.Time{}
	PublishCheckpointCreated(bus, snap)
	select {
	case event := <-events:
		if event.RunID != "fallback-run" || event.Step != 7 || event.CheckpointID == "" || event.SnapshotID != event.CheckpointID {
			t.Fatalf("unexpected checkpoint event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for checkpoint event")
	}
	if SnapshotCheckpointID(nil) != "" {
		t.Fatal("nil checkpoint id should be empty")
	}
	if id := SnapshotCheckpointID(&RunSnapshot{RunStep: 2}); !strings.Contains(id, "checkpoint:run:step-2:") {
		t.Fatalf("unexpected fallback checkpoint id: %s", id)
	}
}

func TestSnapshotPreservesRunContextMessagesWhenRichGetterIsUsed(t *testing.T) {
	unprocessed := ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "original"}}}
	processed := ModelRequest{Parts: []ModelRequestPart{SystemPromptPart{Content: "shaped"}, UserPromptPart{Content: "processed"}}}
	rc := NewRunContext(RunContext{
		Messages: []ModelMessage{processed},
		RunID:    "run-shaped",
		RunStep:  2,
		Prompt:   "processed",
	}, nil, func() *RunStateSnapshot {
		return &RunStateSnapshot{
			Messages: []ModelMessage{unprocessed},
			RunID:    "run-shaped",
			RunStep:  2,
			Prompt:   "processed",
			Retries:  3,
		}
	})
	rc.messagesOverride = true

	snap := Snapshot(rc)
	if snap == nil {
		t.Fatal("Snapshot() returned nil")
	}
	if snap.Retries != 3 {
		t.Fatalf("rich state fields were not preserved: %+v", snap)
	}
	if len(snap.Messages) != 1 {
		t.Fatalf("messages = %+v, want one processed request", snap.Messages)
	}
	req, ok := snap.Messages[0].(ModelRequest)
	if !ok {
		t.Fatalf("message type = %T, want ModelRequest", snap.Messages[0])
	}
	if len(req.Parts) != 2 {
		t.Fatalf("processed message parts = %+v", req.Parts)
	}
	if part, ok := req.Parts[0].(SystemPromptPart); !ok || part.Content != "shaped" {
		t.Fatalf("first part = %#v, want shaped system prompt", req.Parts[0])
	}
}

func TestSnapshotOnModelRequestUsesProcessedMessages(t *testing.T) {
	var captured *RunSnapshot
	processed := ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "processed prompt"}}}
	agent := NewAgent[string](
		NewTestModel(TextResponse("done")),
		WithHistoryProcessor[string](func(context.Context, []ModelMessage) ([]ModelMessage, error) {
			return []ModelMessage{processed}, nil
		}),
		WithHooks[string](Hook{
			OnModelRequest: func(_ context.Context, rc *RunContext, _ []ModelMessage) {
				captured = Snapshot(rc)
			},
		}),
	)
	if _, err := agent.Run(context.Background(), "original prompt"); err != nil {
		t.Fatal(err)
	}
	if captured == nil || len(captured.Messages) != 1 {
		t.Fatalf("captured snapshot = %+v", captured)
	}
	req, ok := captured.Messages[0].(ModelRequest)
	if !ok || len(req.Parts) != 1 {
		t.Fatalf("captured message = %#v", captured.Messages[0])
	}
	part, ok := req.Parts[0].(UserPromptPart)
	if !ok || part.Content != "processed prompt" {
		t.Fatalf("snapshot did not preserve processed outbound message: %#v", req.Parts[0])
	}
}

func TestSnapshot_Serialize(t *testing.T) {
	runStartTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	snap := &RunSnapshot{
		Messages: []ModelMessage{
			ModelRequest{
				Parts:     []ModelRequestPart{UserPromptPart{Content: "test", Timestamp: time.Now()}},
				Timestamp: time.Now(),
			},
		},
		Usage:            RunUsage{Requests: 1},
		LastInputTokens:  42,
		Retries:          2,
		ToolRetries:      map[string]int{"echo": 1},
		RunID:            "snap-1",
		ParentRunID:      "parent-1",
		RunStep:          2,
		RunStartTime:     runStartTime,
		Prompt:           "test",
		ToolState:        map[string]any{"tool": map[string]any{"count": 3}},
		Timestamp:        time.Now(),
		SourceTraceRunID: "source-run",
		SourceSnapshotID: "snap_000001",
	}

	data, err := MarshalSnapshot(snap)
	if err != nil {
		t.Fatal(err)
	}

	restored, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}

	if restored.RunID != snap.RunID {
		t.Errorf("expected RunID %q, got %q", snap.RunID, restored.RunID)
	}
	if restored.ParentRunID != snap.ParentRunID {
		t.Errorf("expected ParentRunID %q, got %q", snap.ParentRunID, restored.ParentRunID)
	}
	if restored.Prompt != snap.Prompt {
		t.Errorf("expected Prompt %q, got %q", snap.Prompt, restored.Prompt)
	}
	if restored.RunStep != snap.RunStep {
		t.Errorf("expected RunStep %d, got %d", snap.RunStep, restored.RunStep)
	}
	if restored.LastInputTokens != snap.LastInputTokens {
		t.Errorf("expected LastInputTokens %d, got %d", snap.LastInputTokens, restored.LastInputTokens)
	}
	if restored.Retries != snap.Retries {
		t.Errorf("expected Retries %d, got %d", snap.Retries, restored.Retries)
	}
	if got := restored.ToolRetries["echo"]; got != 1 {
		t.Errorf("expected ToolRetries echo=1, got %d", got)
	}
	if !restored.RunStartTime.Equal(runStartTime) {
		t.Errorf("expected RunStartTime %v, got %v", runStartTime, restored.RunStartTime)
	}
	if got := snapshotCountFromAny(restored.ToolState["tool"].(map[string]any)["count"]); got != 3 {
		t.Errorf("expected restored tool state count=3, got %d", got)
	}
	if restored.SourceTraceRunID != "source-run" {
		t.Errorf("expected source trace run id %q, got %q", "source-run", restored.SourceTraceRunID)
	}
	if restored.SourceSnapshotID != "snap_000001" {
		t.Errorf("expected source snapshot id %q, got %q", "snap_000001", restored.SourceSnapshotID)
	}
}

func TestSnapshot_Branch(t *testing.T) {
	snap := &RunSnapshot{
		Messages: []ModelMessage{
			ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "original"}}},
		},
		RunID:       "branch-test",
		ParentRunID: "branch-parent",
	}

	branched := snap.Branch(func(msgs []ModelMessage) []ModelMessage {
		// Add a new message.
		return append(msgs, ModelRequest{
			Parts: []ModelRequestPart{UserPromptPart{Content: "branched"}},
		})
	})

	// Original should be unchanged.
	if len(snap.Messages) != 1 {
		t.Errorf("original snapshot modified, has %d messages", len(snap.Messages))
	}
	// Branch should have 2 messages.
	if len(branched.Messages) != 2 {
		t.Errorf("expected 2 messages in branch, got %d", len(branched.Messages))
	}
	if branched.RunID != snap.RunID {
		t.Error("branch should preserve RunID")
	}
	if branched.ParentRunID != snap.ParentRunID {
		t.Error("branch should preserve ParentRunID")
	}
}

func TestSnapshot_WithSnapshotOption(t *testing.T) {
	// Create a snapshot with some messages.
	snap := &RunSnapshot{
		Messages: []ModelMessage{
			ModelRequest{
				Parts:     []ModelRequestPart{UserPromptPart{Content: "previous"}},
				Timestamp: time.Now(),
			},
			ModelResponse{
				Parts:     []ModelResponsePart{TextPart{Content: "prev response"}},
				Timestamp: time.Now(),
			},
		},
		RunID:  "snap-resume",
		Prompt: "test",
	}

	model := NewTestModel(TextResponse("resumed"))
	agent := NewAgent[string](model)

	result, err := agent.Run(context.Background(), "continue", WithSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "resumed" {
		t.Errorf("expected 'resumed', got %q", result.Output)
	}

	// Model should have received the snapshot messages plus the new prompt.
	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	// Total messages = 2 from snapshot + 1 new request.
	if len(calls[0].Messages) != 3 {
		t.Errorf("expected 3 messages (2 snapshot + 1 new), got %d", len(calls[0].Messages))
	}
}

func TestSnapshot_WithSnapshotRestoresRunState(t *testing.T) {
	stateful := &testSnapshotStatefulTool{}
	type Params struct{}
	counter := FuncTool[Params]("counter", "counter", func(_ context.Context, rc *RunContext, _ Params) (string, error) {
		if got, ok := rc.ToolStateByName("counter"); !ok || snapshotCountFromAny(got.(map[string]any)["count"]) != 9 {
			t.Fatalf("expected restored tool state count=9, got %v", got)
		}
		stateful.count++
		return fmt.Sprintf("%d", stateful.count), nil
	})
	counter.Stateful = stateful

	var firstRunID string
	var firstRunStep int
	var firstUsage RunUsage
	var firstRunStart time.Time

	startTime := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	snap := &RunSnapshot{
		RunID:        "snap-resume",
		RunStep:      7,
		RunStartTime: startTime,
		Usage:        RunUsage{Requests: 3, ToolCalls: 1},
		ToolState:    map[string]any{"counter": map[string]any{"count": 9}},
	}

	model := NewTestModel(
		ToolCallResponse("counter", `{}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model,
		WithTools[string](counter),
		WithHooks[string](Hook{
			OnModelRequest: func(_ context.Context, rc *RunContext, _ []ModelMessage) {
				if firstRunID == "" {
					firstRunID = rc.RunID
					firstRunStep = rc.RunStep
					firstUsage = rc.Usage
					firstRunStart = rc.RunStartTime
				}
			},
		}),
	)

	result, err := agent.Run(context.Background(), "continue", WithSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}

	if firstRunID != "snap-resume" {
		t.Errorf("expected restored RunID snap-resume, got %q", firstRunID)
	}
	if firstRunStep != 8 {
		t.Errorf("expected restored RunStep to continue at 8, got %d", firstRunStep)
	}
	if firstUsage.Requests != 3 || firstUsage.ToolCalls != 1 {
		t.Errorf("expected restored usage {Requests:3 ToolCalls:1}, got %+v", firstUsage)
	}
	if !firstRunStart.Equal(startTime) {
		t.Errorf("expected restored RunStartTime %v, got %v", startTime, firstRunStart)
	}
	if result.RunID != "snap-resume" {
		t.Errorf("expected result RunID snap-resume, got %q", result.RunID)
	}
	if result.Usage.Requests != 5 {
		t.Errorf("expected total requests 5, got %d", result.Usage.Requests)
	}
	if got := snapshotCountFromAny(result.ToolState["counter"].(map[string]any)["count"]); got != 10 {
		t.Errorf("expected final tool state count=10, got %d", got)
	}
}

func TestSnapshot_RunStreamWithSnapshotRestoresRunState(t *testing.T) {
	stateful := &testSnapshotStatefulTool{}
	type Params struct{}
	counter := FuncTool[Params]("counter", "counter", func(_ context.Context, rc *RunContext, _ Params) (string, error) {
		if got, ok := rc.ToolStateByName("counter"); !ok || snapshotCountFromAny(got.(map[string]any)["count"]) != 4 {
			t.Fatalf("expected restored tool state count=4, got %v", got)
		}
		stateful.count++
		return fmt.Sprintf("%d", stateful.count), nil
	})
	counter.Stateful = stateful

	startTime := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	snap := &RunSnapshot{
		Messages: []ModelMessage{
			ModelRequest{
				Parts:     []ModelRequestPart{UserPromptPart{Content: "previous"}},
				Timestamp: startTime,
			},
			ModelResponse{
				Parts:     []ModelResponsePart{TextPart{Content: "previous response"}},
				Timestamp: startTime.Add(time.Second),
			},
		},
		RunID:        "stream-snap-resume",
		RunStep:      3,
		RunStartTime: startTime,
		Usage:        RunUsage{Requests: 2, ToolCalls: 1},
		ToolState:    map[string]any{"counter": map[string]any{"count": 4}},
		Prompt:       "previous",
	}

	model := NewTestModel(
		ToolCallResponse("counter", `{}`),
		TextResponse("stream done"),
	)
	agent := NewAgent[string](model, WithTools[string](counter))

	stream, err := agent.RunStream(context.Background(), "continue", WithSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	result, err := stream.Result()
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "stream done" {
		t.Fatalf("expected stream output %q, got %q", "stream done", result.Output)
	}
	if result.RunID != "stream-snap-resume" {
		t.Fatalf("expected restored RunID stream-snap-resume, got %q", result.RunID)
	}
	if result.Usage.Requests != 4 || result.Usage.ToolCalls != 2 {
		t.Fatalf("expected usage to continue from snapshot, got %+v", result.Usage)
	}
	if got := snapshotCountFromAny(result.ToolState["counter"].(map[string]any)["count"]); got != 5 {
		t.Fatalf("expected final streamed tool state count=5, got %d", got)
	}

	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 streamed model calls, got %d", len(calls))
	}
	if got := len(calls[0].Messages); got != 3 {
		t.Fatalf("expected snapshot messages plus new prompt in first streamed call, got %d messages", got)
	}
}

func TestSnapshot_WithSnapshotTraceIsFreshSegment(t *testing.T) {
	oldStart := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	snap := &RunSnapshot{
		Messages: []ModelMessage{
			ModelRequest{
				Parts:     []ModelRequestPart{UserPromptPart{Content: "previous"}},
				Timestamp: oldStart,
			},
			ModelResponse{
				Parts:     []ModelResponsePart{TextPart{Content: "previous response"}},
				Timestamp: oldStart.Add(time.Second),
			},
		},
		RunID:            "fresh-trace-resume",
		RunStep:          3,
		RunStartTime:     oldStart,
		Usage:            RunUsage{Requests: 2},
		Prompt:           "previous",
		SourceTraceRunID: "source-run",
		SourceSnapshotID: "snap_000001",
	}

	model := NewTestModel(TextResponse("resumed"))
	agent := NewAgent[string](model, WithTracing[string]())

	before := time.Now()
	result, err := agent.Run(context.Background(), "continue", WithSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil {
		t.Fatal("expected trace")
	}
	if result.Trace.StartTime.Before(before) {
		t.Fatalf("expected resumed trace to start at the fresh execution segment, got %v before %v", result.Trace.StartTime, before)
	}
	if result.Trace.StartTime.Equal(oldStart) {
		t.Fatalf("resumed trace reused snapshot run start time %v", oldStart)
	}
	if len(result.Trace.Requests) != 1 {
		t.Fatalf("expected fresh resumed trace to contain only new model requests, got %d", len(result.Trace.Requests))
	}
	if result.Trace.Requests[0].Sequence != 1 {
		t.Fatalf("expected resumed trace request sequence to restart at 1, got %d", result.Trace.Requests[0].Sequence)
	}
	if result.Trace.Requests[0].MessageCount != 3 {
		t.Fatalf("expected outbound request to include restored message history plus new prompt, got %d messages", result.Trace.Requests[0].MessageCount)
	}
}

func TestSnapshot_WithSnapshotRetryMapRemainsWritable(t *testing.T) {
	type Params struct{}

	model := NewTestModel(
		ToolCallResponse("retry_tool", `{}`),
		TextResponse("done"),
	)
	tool := FuncTool[Params]("retry_tool", "retry", func(_ context.Context, _ Params) (string, error) {
		return "", NewModelRetryError("retry once")
	})

	agent := NewAgent[string](model, WithTools[string](tool))
	snap := &RunSnapshot{
		RunID:        "snap-retry",
		RunStartTime: time.Now().Add(-time.Minute).UTC().Truncate(time.Second),
		Prompt:       "resume",
	}

	result, err := agent.Run(context.Background(), "resume", WithSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "done" {
		t.Fatalf("expected resumed output %q, got %q", "done", result.Output)
	}
}

func TestEncodeRunSnapshot_Nil(t *testing.T) {
	if _, err := EncodeRunSnapshot(nil); err == nil {
		t.Fatal("expected nil snapshot error")
	}
	if _, err := MarshalSnapshot(nil); err == nil {
		t.Fatal("expected MarshalSnapshot(nil) to fail")
	}
}

func TestSnapshot_FromHook(t *testing.T) {
	var captured *RunSnapshot

	model := NewTestModel(
		ToolCallResponse("echo", `{"n":1}`),
		TextResponse("done"),
	)

	type Params struct {
		N int `json:"n"`
	}
	tool := FuncTool[Params]("echo", "echo", func(ctx context.Context, p Params) (string, error) {
		return "echoed", nil
	})

	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithHooks[string](Hook{
			OnModelResponse: func(ctx context.Context, rc *RunContext, resp *ModelResponse) {
				if captured == nil {
					captured = Snapshot(rc)
				}
			},
		}),
	)

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	if captured == nil {
		t.Fatal("snapshot not captured from hook")
	}
	if captured.Prompt != "test" {
		t.Errorf("expected prompt 'test', got %q", captured.Prompt)
	}
}

func TestSnapshot_OnTurnEndIncludesToolResults(t *testing.T) {
	var captured *RunSnapshot

	model := NewTestModel(
		ToolCallResponseWithID("echo", `{"n":1}`, "call-echo"),
		TextResponse("done"),
	)

	type Params struct {
		N int `json:"n"`
	}
	tool := FuncTool[Params]("echo", "echo", func(ctx context.Context, p Params) (string, error) {
		return "echoed", nil
	})

	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithHooks[string](Hook{
			OnTurnEnd: func(ctx context.Context, rc *RunContext, turnNumber int, resp *ModelResponse) {
				if captured == nil {
					captured = Snapshot(rc)
				}
			},
		}),
	)

	if _, err := agent.Run(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	if captured == nil {
		t.Fatal("snapshot not captured from OnTurnEnd")
	}
	if len(captured.Messages) < 3 {
		t.Fatalf("expected snapshot to include prompt, tool call, and tool result messages, got %d", len(captured.Messages))
	}
	last, ok := captured.Messages[len(captured.Messages)-1].(ModelRequest)
	if !ok {
		t.Fatalf("last snapshot message type = %T, want ModelRequest", captured.Messages[len(captured.Messages)-1])
	}
	if len(last.Parts) != 1 {
		t.Fatalf("last snapshot message parts = %d, want 1", len(last.Parts))
	}
	ret, ok := last.Parts[0].(ToolReturnPart)
	if !ok {
		t.Fatalf("last snapshot part = %T, want ToolReturnPart", last.Parts[0])
	}
	if ret.ToolCallID != "call-echo" {
		t.Fatalf("tool return id = %q, want call-echo", ret.ToolCallID)
	}
}

func TestSnapshot_RunStreamOnTurnEndIncludesToolResults(t *testing.T) {
	var captured *RunSnapshot

	model := NewTestModel(
		ToolCallResponseWithID("echo", `{"n":1}`, "call-echo"),
		TextResponse("done"),
	)

	type Params struct {
		N int `json:"n"`
	}
	tool := FuncTool[Params]("echo", "echo", func(ctx context.Context, p Params) (string, error) {
		return "echoed", nil
	})

	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithHooks[string](Hook{
			OnTurnEnd: func(ctx context.Context, rc *RunContext, turnNumber int, resp *ModelResponse) {
				if captured == nil {
					captured = Snapshot(rc)
				}
			},
		}),
	)

	stream, err := agent.RunStream(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, err := stream.Result(); err != nil {
		t.Fatal(err)
	}

	if captured == nil {
		t.Fatal("snapshot not captured from streaming OnTurnEnd")
	}
	if len(captured.Messages) < 3 {
		t.Fatalf("expected snapshot to include prompt, tool call, and tool result messages, got %d", len(captured.Messages))
	}
	last, ok := captured.Messages[len(captured.Messages)-1].(ModelRequest)
	if !ok {
		t.Fatalf("last snapshot message type = %T, want ModelRequest", captured.Messages[len(captured.Messages)-1])
	}
	if len(last.Parts) != 1 {
		t.Fatalf("last snapshot message parts = %d, want 1", len(last.Parts))
	}
	ret, ok := last.Parts[0].(ToolReturnPart)
	if !ok {
		t.Fatalf("last snapshot part = %T, want ToolReturnPart", last.Parts[0])
	}
	if ret.ToolCallID != "call-echo" {
		t.Fatalf("tool return id = %q, want call-echo", ret.ToolCallID)
	}
}

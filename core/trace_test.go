package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestTrace_BasicRun(t *testing.T) {
	model := NewTestModel(TextResponse("hello"))
	agent := NewAgent[string](model, WithTracing[string]())

	result, err := agent.Run(context.Background(), "test prompt")
	if err != nil {
		t.Fatal(err)
	}

	if result.Trace == nil {
		t.Fatal("expected trace to be non-nil")
	}

	trace := result.Trace
	if trace.RunID == "" {
		t.Error("expected non-empty run ID")
	}
	if trace.Prompt != "test prompt" {
		t.Errorf("expected prompt %q, got %q", "test prompt", trace.Prompt)
	}
	if !trace.Success {
		t.Error("expected success=true")
	}
	if trace.Duration <= 0 {
		t.Error("expected positive duration")
	}

	// Should have model request and response steps.
	hasReq := false
	hasResp := false
	for _, step := range trace.Steps {
		switch step.Kind {
		case TraceModelRequest:
			hasReq = true
		case TraceModelResponse:
			hasResp = true
		}
	}
	if !hasReq {
		t.Error("expected TraceModelRequest step")
	}
	if !hasResp {
		t.Error("expected TraceModelResponse step")
	}
}

func TestTrace_WithToolCalls(t *testing.T) {
	type AddParams struct {
		A int `json:"a"`
	}
	addTool := FuncTool[AddParams]("add", "add", func(ctx context.Context, p AddParams) (int, error) {
		return p.A + 1, nil
	})

	model := NewTestModel(
		ToolCallResponse("add", `{"a":5}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model,
		WithTools[string](addTool),
		WithTracing[string](),
	)

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	trace := result.Trace
	if trace == nil {
		t.Fatal("expected non-nil trace")
	}

	hasToolCall := false
	hasToolResult := false
	for _, step := range trace.Steps {
		switch step.Kind {
		case TraceToolCall:
			hasToolCall = true
			data := step.Data.(map[string]any)
			if data["tool_name"] != "add" {
				t.Errorf("expected tool_name 'add', got %v", data["tool_name"])
			}
		case TraceToolResult:
			hasToolResult = true
			if step.Duration <= 0 {
				t.Error("expected positive tool duration")
			}
		}
	}
	if !hasToolCall {
		t.Error("expected TraceToolCall step")
	}
	if !hasToolResult {
		t.Error("expected TraceToolResult step")
	}
}

func TestTrace_Timing(t *testing.T) {
	model := NewTestModel(TextResponse("hello"))
	agent := NewAgent[string](model, WithTracing[string]())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	trace := result.Trace
	for i, step := range trace.Steps {
		if step.Kind == TraceModelResponse && step.Duration <= 0 {
			t.Errorf("step %d (%s): expected positive duration", i, step.Kind)
		}
	}

	if trace.EndTime.Before(trace.StartTime) {
		t.Error("end time should be after start time")
	}
}

func TestTrace_JSONSerializable(t *testing.T) {
	model := NewTestModel(TextResponse("hello"))
	agent := NewAgent[string](model, WithTracing[string]())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(result.Trace)
	if err != nil {
		t.Fatalf("failed to marshal trace: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}

	// Verify it round-trips.
	var decoded RunTrace
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal trace: %v", err)
	}
	if decoded.RunID != result.Trace.RunID {
		t.Errorf("RunID mismatch after round-trip")
	}
}

func TestTrace_Disabled(t *testing.T) {
	model := NewTestModel(TextResponse("hello"))
	agent := NewAgent[string](model) // No WithTracing

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	if result.Trace != nil {
		t.Error("expected nil trace when tracing not enabled")
	}
}

func TestTrace_RequestTraceCapturesFinalRequest(t *testing.T) {
	maxTokens := 42
	model := NewTestModel(&ModelResponse{
		Parts:        []ModelResponsePart{TextPart{Content: "ok"}},
		Usage:        Usage{InputTokens: 11, OutputTokens: 7},
		ModelName:    "test-model",
		FinishReason: FinishReasonStop,
		Timestamp:    time.Now(),
	})
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithAgentMiddleware[string](MaxTokensMiddleware(maxTokens)),
	)

	result, err := agent.Run(context.Background(), "trace request")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil {
		t.Fatal("expected non-nil trace")
	}
	if len(result.Trace.Requests) != 1 {
		t.Fatalf("expected 1 request trace, got %d", len(result.Trace.Requests))
	}

	req := result.Trace.Requests[0]
	if req.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", req.Sequence)
	}
	if req.MessageCount == 0 || len(req.Messages) == 0 {
		t.Fatalf("expected traced outbound messages, got count=%d len=%d", req.MessageCount, len(req.Messages))
	}
	if req.Settings == nil || req.Settings.MaxTokens == nil || *req.Settings.MaxTokens != maxTokens {
		t.Fatalf("expected traced max_tokens=%d, got %+v", maxTokens, req.Settings)
	}
	if req.Response == nil {
		t.Fatal("expected traced response")
	}
	if req.Response.Usage.InputTokens != 11 || req.Response.Usage.OutputTokens != 7 {
		t.Fatalf("unexpected traced usage: %+v", req.Response.Usage)
	}
	if req.Response.Message == nil {
		t.Fatal("expected serialized traced response message")
	}
}

func TestTrace_RequestTraceCapturesMiddlewareModifiedMessages(t *testing.T) {
	mw := func(
		ctx context.Context,
		messages []ModelMessage,
		settings *ModelSettings,
		params *ModelRequestParameters,
		next func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error),
	) (*ModelResponse, error) {
		modified := make([]ModelMessage, 0, len(messages)+1)
		modified = append(modified, ModelRequest{
			Parts:     []ModelRequestPart{SystemPromptPart{Content: "injected by middleware"}},
			Timestamp: time.Now(),
		})
		modified = append(modified, messages...)
		return next(ctx, modified, settings, params)
	}

	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithAgentMiddleware[string](RequestOnlyMiddleware(mw)),
	)

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil || len(result.Trace.Requests) != 1 {
		t.Fatalf("expected 1 request trace, got %+v", result.Trace)
	}

	decoded, err := DecodeMessages(result.Trace.Requests[0].Messages)
	if err != nil {
		t.Fatalf("decode traced messages: %v", err)
	}
	if len(decoded) < 2 {
		t.Fatalf("expected middleware-injected message plus original request, got %d messages", len(decoded))
	}
	first, ok := decoded[0].(ModelRequest)
	if !ok {
		t.Fatalf("expected first traced message to be a request, got %T", decoded[0])
	}
	if len(first.Parts) == 0 {
		t.Fatal("expected request parts on middleware-injected message")
	}
	sys, ok := first.Parts[0].(SystemPromptPart)
	if !ok || sys.Content != "injected by middleware" {
		t.Fatalf("expected middleware system prompt, got %#v", first.Parts[0])
	}
}

func TestTrace_RequestTraceCapturesCompaction(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithHistoryProcessor[string](func(_ context.Context, messages []ModelMessage) ([]ModelMessage, error) {
			if len(messages) <= 2 {
				return messages, nil
			}
			return messages[1:], nil
		}),
	)

	history := []ModelMessage{
		ModelRequest{
			Parts:     []ModelRequestPart{UserPromptPart{Content: "old prompt"}},
			Timestamp: time.Now().Add(-2 * time.Minute),
		},
		ModelResponse{
			Parts:        []ModelResponsePart{TextPart{Content: "old answer"}},
			FinishReason: FinishReasonStop,
			ModelName:    "test-model",
			Timestamp:    time.Now().Add(-time.Minute),
		},
	}

	result, err := agent.Run(context.Background(), "new prompt", WithMessages(history...))
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil || len(result.Trace.Requests) != 1 {
		t.Fatalf("expected 1 request trace, got %+v", result.Trace)
	}

	compactions := result.Trace.Requests[0].Compactions
	if len(compactions) == 0 {
		t.Fatal("expected compaction trace on request")
	}
	if compactions[0].Strategy != CompactionStrategyHistoryProcessor {
		t.Fatalf("expected history processor compaction, got %q", compactions[0].Strategy)
	}
	if compactions[0].MessagesBefore <= compactions[0].MessagesAfter {
		t.Fatalf("expected message count reduction, got before=%d after=%d", compactions[0].MessagesBefore, compactions[0].MessagesAfter)
	}
}

func TestTrace_RequestTraceStreamingPath(t *testing.T) {
	model := NewTestModel(TextResponse("streamed"))
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithAgentMiddleware[string](StreamOnlyMiddleware(MaxTokensMiddleware(64).Stream)),
	)

	stream, err := agent.RunStream(context.Background(), "stream trace")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	if _, err := stream.GetOutput(); err != nil {
		t.Fatal(err)
	}

	result, err := stream.Result()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Trace == nil {
		t.Fatal("expected traced stream result")
	}
	if len(result.Trace.Requests) != 1 {
		t.Fatalf("expected 1 traced stream request, got %d", len(result.Trace.Requests))
	}
	req := result.Trace.Requests[0]
	if req.Settings == nil || req.Settings.MaxTokens == nil || *req.Settings.MaxTokens != 64 {
		t.Fatalf("expected traced stream max_tokens=64, got %+v", req.Settings)
	}
	if req.Response == nil {
		t.Fatal("expected traced stream response")
	}
}

func TestTrace_RequestTraceStreamingAbortIsRecorded(t *testing.T) {
	var exported *RunTrace
	exporter := testTraceExporter{
		exportFn: func(_ context.Context, trace *RunTrace) error {
			exported = trace
			return nil
		},
	}

	model := NewTestModel(TextResponse("streamed"))
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithTraceExporter[string](exporter),
	)

	stream, err := agent.RunStream(context.Background(), "abort trace")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); !errors.Is(err, errStreamClosed) {
		t.Fatalf("expected errStreamClosed, got %v", err)
	}
	if exported == nil {
		t.Fatal("expected exported trace after stream abort")
	}
	if len(exported.Requests) != 1 {
		t.Fatalf("expected 1 request trace, got %d", len(exported.Requests))
	}
	if exported.Requests[0].Error == "" {
		t.Fatal("expected request trace error for aborted stream")
	}
}

func TestTrace_RequestTraceIgnoresCountPreservingHistoryRewrite(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithHistoryProcessor[string](func(_ context.Context, messages []ModelMessage) ([]ModelMessage, error) {
			modified := append([]ModelMessage(nil), messages...)
			req, ok := modified[len(modified)-1].(ModelRequest)
			if !ok {
				return modified, nil
			}
			parts := append([]ModelRequestPart(nil), req.Parts...)
			user, ok := parts[len(parts)-1].(UserPromptPart)
			if !ok {
				return modified, nil
			}
			user.Content += " rewritten"
			parts[len(parts)-1] = user
			req.Parts = parts
			modified[len(modified)-1] = req
			return modified, nil
		}),
	)

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil || len(result.Trace.Requests) != 1 {
		t.Fatalf("expected 1 request trace, got %+v", result.Trace)
	}
	if len(result.Trace.Requests[0].Compactions) != 0 {
		t.Fatalf("expected no compaction trace for count-preserving rewrite, got %+v", result.Trace.Requests[0].Compactions)
	}
}

func TestTrace_RequestTraceCapturesExplicitHistoryProcessorCompaction(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithTracing[string](),
		WithHistoryProcessor[string](func(ctx context.Context, messages []ModelMessage) ([]ModelMessage, error) {
			if cb := CompactionCallbackFromContext(ctx); cb != nil {
				cb(ContextCompactionStats{
					Strategy:       CompactionStrategyHistoryProcessor,
					MessagesBefore: len(messages),
					MessagesAfter:  len(messages),
				})
			}
			return messages, nil
		}),
	)

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace == nil || len(result.Trace.Requests) != 1 {
		t.Fatalf("expected 1 request trace, got %+v", result.Trace)
	}
	compactions := result.Trace.Requests[0].Compactions
	if len(compactions) != 1 {
		t.Fatalf("expected 1 explicit compaction trace, got %+v", compactions)
	}
	if compactions[0].Strategy != CompactionStrategyHistoryProcessor {
		t.Fatalf("expected history processor compaction, got %q", compactions[0].Strategy)
	}
	if compactions[0].MessagesBefore != 1 || compactions[0].MessagesAfter != 1 {
		t.Fatalf("unexpected explicit compaction counts: %+v", compactions[0])
	}
}

type testTraceExporter struct {
	exportFn func(context.Context, *RunTrace) error
}

func (e testTraceExporter) Export(ctx context.Context, trace *RunTrace) error {
	if e.exportFn == nil {
		return nil
	}
	return e.exportFn(ctx, trace)
}

package temporal

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestTemporalModel_PassThrough(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Hello!"))
	tm := NewTemporalModel(model, "test-agent", DefaultActivityConfig())

	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Hello"},
			},
			Timestamp: time.Now(),
		},
	}

	// Without middleware, Request should pass through to the wrapped model.
	resp, err := tm.Request(context.Background(), messages, nil, &core.ModelRequestParameters{
		AllowTextOutput: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TextContent() != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.TextContent())
	}
}

func TestTemporalModel_Request_UsesRequestMiddleware(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("wrapped"))
	tm := NewTemporalModelWithMiddleware(
		model,
		"test-agent",
		DefaultActivityConfig(),
		[]core.RequestMiddlewareFunc{
			func(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters, next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error)) (*core.ModelResponse, error) {
				modified := make([]core.ModelMessage, 0, len(messages)+1)
				modified = append(modified, core.ModelRequest{
					Parts:     []core.ModelRequestPart{core.SystemPromptPart{Content: "injected by middleware"}},
					Timestamp: time.Now(),
				})
				modified = append(modified, messages...)
				return next(ctx, modified, settings, params)
			},
		},
		nil,
	)

	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Hello"},
			},
			Timestamp: time.Now(),
		},
	}

	resp, err := tm.Request(context.Background(), messages, nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TextContent() != "wrapped" {
		t.Fatalf("expected wrapped response, got %q", resp.TextContent())
	}

	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 model call, got %d", len(calls))
	}
	assertLeadingSystemPrompt(t, calls[0].Messages, "injected by middleware")
}

func TestTemporalModel_RequestStream_UsesStreamMiddleware(t *testing.T) {
	model := &countingStreamTestModel{
		response: &core.ModelResponse{
			Parts:     []core.ModelResponsePart{core.TextPart{Content: "wrapped stream"}},
			ModelName: "test-model",
			Usage:     core.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}

	tm := NewTemporalModelWithMiddleware(
		model,
		"test-agent",
		DefaultActivityConfig(),
		nil,
		[]core.AgentStreamMiddleware{
			func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters, _ core.AgentStreamFunc) (core.StreamedResponse, error) {
				resp := core.TextResponse("stream middleware intercepted")
				return &completedStream{response: resp}, nil
			},
		},
	)

	messages := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Hello"},
			},
			Timestamp: time.Now(),
		},
	}

	stream, err := tm.RequestStream(context.Background(), messages, nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	for {
		_, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
	}

	if got := stream.Response().TextContent(); got != "stream middleware intercepted" {
		t.Fatalf("expected intercepted stream response, got %q", got)
	}
	if model.streamCalls != 0 {
		t.Fatalf("expected underlying stream model to be skipped, got %d calls", model.streamCalls)
	}
}

func TestTemporalModel_ModelName(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Hi"))
	tm := NewTemporalModel(model, "my-agent", DefaultActivityConfig())

	if name := tm.ModelName(); name != "test-model" {
		t.Errorf("expected 'test-model', got %q", name)
	}
}

func TestTemporalModel_ActivityNames(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Hi"))
	tm := NewTemporalModel(model, "my-agent", DefaultActivityConfig())

	reqName := tm.ModelRequestActivityName()
	if reqName != "agent__my-agent__model_request" {
		t.Errorf("unexpected request activity name: %s", reqName)
	}

	streamName := tm.ModelRequestStreamActivityName()
	if streamName != "agent__my-agent__model_request_stream" {
		t.Errorf("unexpected stream activity name: %s", streamName)
	}
}

func TestTemporalModel_ModelRequestActivity(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("Activity response"))
	tm := NewTemporalModel(model, "test-agent", DefaultActivityConfig())

	messagesJSON, err := core.MarshalMessages([]core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Hello"},
			},
			Timestamp: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}

	params := ModelActivityInput{
		MessagesJSON: messagesJSON,
		Parameters: &core.ModelRequestParameters{
			AllowTextOutput: true,
		},
	}

	output, err := tm.ModelRequestActivity(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := decodeModelActivityOutput(output)
	if err != nil {
		t.Fatalf("decode model output: %v", err)
	}
	if resp.TextContent() != "Activity response" {
		t.Errorf("expected 'Activity response', got %q", resp.TextContent())
	}
}

func TestModelRequestActivity_UsesRequestMiddleware(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("activity response"))
	tm := NewTemporalModelWithMiddleware(
		model,
		"test-agent",
		DefaultActivityConfig(),
		[]core.RequestMiddlewareFunc{
			func(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters, next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error)) (*core.ModelResponse, error) {
				modified := make([]core.ModelMessage, 0, len(messages)+1)
				modified = append(modified, core.ModelRequest{
					Parts:     []core.ModelRequestPart{core.SystemPromptPart{Content: "activity middleware"}},
					Timestamp: time.Now(),
				})
				modified = append(modified, messages...)
				return next(ctx, modified, settings, params)
			},
		},
		nil,
	)

	messagesJSON, err := core.MarshalMessages([]core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.UserPromptPart{Content: "Hello"},
			},
			Timestamp: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}

	output, err := tm.ModelRequestActivity(context.Background(), ModelActivityInput{
		MessagesJSON: messagesJSON,
		Parameters:   &core.ModelRequestParameters{AllowTextOutput: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := decodeModelActivityOutput(output)
	if err != nil {
		t.Fatalf("decode model output: %v", err)
	}
	if resp.TextContent() != "activity response" {
		t.Fatalf("expected activity response, got %q", resp.TextContent())
	}

	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 model call, got %d", len(calls))
	}
	assertLeadingSystemPrompt(t, calls[0].Messages, "activity middleware")
}

func TestDefaultActivityConfig(t *testing.T) {
	config := DefaultActivityConfig()
	if config.StartToCloseTimeout != 60*time.Second {
		t.Errorf("expected 60s timeout, got %v", config.StartToCloseTimeout)
	}
	if config.MaxRetries != 0 {
		t.Errorf("expected 0 max retries, got %d", config.MaxRetries)
	}
}

func TestCompletedStream(t *testing.T) {
	resp := &core.ModelResponse{
		Parts: []core.ModelResponsePart{
			core.TextPart{Content: "Completed"},
		},
		Usage: core.Usage{InputTokens: 10, OutputTokens: 5},
	}

	stream := &completedStream{response: resp}

	// First Next should return EOF.
	_, err := stream.Next()
	if err == nil {
		t.Fatal("expected EOF")
	}

	// Response should return the wrapped response.
	got := stream.Response()
	if got.TextContent() != "Completed" {
		t.Errorf("expected 'Completed', got %q", got.TextContent())
	}

	// Usage should match.
	usage := stream.Usage()
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("unexpected usage: %+v", usage)
	}

	// Close should be nil.
	if err := stream.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

// TestModelRequestStreamActivity_MidStreamError verifies that a non-EOF error
// during stream consumption is propagated rather than silently swallowed.
func TestModelRequestStreamActivity_MidStreamError(t *testing.T) {
	streamErr := errors.New("connection reset")
	model := &errorStreamTestModel{
		response: &core.ModelResponse{
			Parts:     []core.ModelResponsePart{core.TextPart{Content: "partial"}},
			ModelName: "test-model",
			Usage:     core.Usage{InputTokens: 10, OutputTokens: 5},
		},
		streamErr: streamErr,
	}

	tm := NewTemporalModel(model, "test-agent", DefaultActivityConfig())
	messagesJSON, err := core.MarshalMessages([]core.ModelMessage{
		core.ModelRequest{
			Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: "Hello"}},
			Timestamp: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	params := ModelActivityInput{
		MessagesJSON: messagesJSON,
		Parameters:   &core.ModelRequestParameters{AllowTextOutput: true},
	}

	_, err = tm.ModelRequestStreamActivity(context.Background(), params)
	if err == nil {
		t.Fatal("expected error from mid-stream failure, got nil")
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("expected error to contain 'connection reset', got %q", err.Error())
	}
}

// errorStreamTestModel returns a stream that errors mid-consumption.
type errorStreamTestModel struct {
	response  *core.ModelResponse
	streamErr error
}

func (m *errorStreamTestModel) ModelName() string { return "test-model" }
func (m *errorStreamTestModel) Request(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return m.response, nil
}
func (m *errorStreamTestModel) RequestStream(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return &errorTestStream{response: m.response, err: m.streamErr}, nil
}

type errorTestStream struct {
	response *core.ModelResponse
	err      error
	called   int
}

func (s *errorTestStream) Next() (core.ModelResponseStreamEvent, error) {
	s.called++
	if s.called == 1 {
		return core.PartStartEvent{Index: 0, Part: s.response.Parts[0]}, nil
	}
	return nil, s.err
}
func (s *errorTestStream) Response() *core.ModelResponse { return s.response }
func (s *errorTestStream) Usage() core.Usage             { return s.response.Usage }
func (s *errorTestStream) Close() error                  { return nil }

// TestModelRequestStreamActivity_Success verifies normal stream consumption.
func TestModelRequestStreamActivity_Success(t *testing.T) {
	model := &successStreamTestModel{
		response: &core.ModelResponse{
			Parts:     []core.ModelResponsePart{core.TextPart{Content: "streamed result"}},
			ModelName: "test-model",
			Usage:     core.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}

	tm := NewTemporalModel(model, "test-agent", DefaultActivityConfig())
	messagesJSON, err := core.MarshalMessages([]core.ModelMessage{
		core.ModelRequest{
			Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: "Hello"}},
			Timestamp: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	params := ModelActivityInput{
		MessagesJSON: messagesJSON,
		Parameters:   &core.ModelRequestParameters{AllowTextOutput: true},
	}

	output, err := tm.ModelRequestStreamActivity(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := decodeModelActivityOutput(output)
	if err != nil {
		t.Fatalf("decode model output: %v", err)
	}
	if resp.TextContent() != "streamed result" {
		t.Errorf("expected 'streamed result', got %q", resp.TextContent())
	}
}

func TestModelRequestStreamActivity_StreamMiddleware(t *testing.T) {
	model := &countingStreamTestModel{
		response: &core.ModelResponse{
			Parts:     []core.ModelResponsePart{core.TextPart{Content: "should not be used"}},
			ModelName: "test-model",
			Usage:     core.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}

	tm := NewTemporalModelWithMiddleware(
		model,
		"test-agent",
		DefaultActivityConfig(),
		nil,
		[]core.AgentStreamMiddleware{
			func(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters, _ core.AgentStreamFunc) (core.StreamedResponse, error) {
				resp := core.TextResponse("stream middleware intercepted")
				return &completedStream{response: resp}, nil
			},
		},
	)
	messagesJSON, err := core.MarshalMessages([]core.ModelMessage{
		core.ModelRequest{
			Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: "Hello"}},
			Timestamp: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}

	output, err := tm.ModelRequestStreamActivity(context.Background(), ModelActivityInput{
		MessagesJSON: messagesJSON,
		Parameters:   &core.ModelRequestParameters{AllowTextOutput: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := decodeModelActivityOutput(output)
	if err != nil {
		t.Fatalf("decode model output: %v", err)
	}
	if resp.TextContent() != "stream middleware intercepted" {
		t.Fatalf("expected intercepted stream response, got %q", resp.TextContent())
	}
	if model.streamCalls != 0 {
		t.Fatalf("expected underlying stream model to be skipped, got %d calls", model.streamCalls)
	}
}

func assertLeadingSystemPrompt(t *testing.T, messages []core.ModelMessage, want string) {
	t.Helper()

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after middleware injection, got %d", len(messages))
	}
	req, ok := messages[0].(core.ModelRequest)
	if !ok {
		t.Fatalf("expected injected model request, got %T", messages[0])
	}
	if len(req.Parts) != 1 {
		t.Fatalf("expected 1 injected part, got %d", len(req.Parts))
	}
	system, ok := req.Parts[0].(core.SystemPromptPart)
	if !ok || system.Content != want {
		t.Fatalf("expected injected system prompt %q, got %#v", want, req.Parts[0])
	}
}

type successStreamTestModel struct {
	response *core.ModelResponse
}

func (m *successStreamTestModel) ModelName() string { return "test-model" }
func (m *successStreamTestModel) Request(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return m.response, nil
}
func (m *successStreamTestModel) RequestStream(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return &successTestStream{response: m.response}, nil
}

type successTestStream struct {
	response *core.ModelResponse
	called   int
}

func (s *successTestStream) Next() (core.ModelResponseStreamEvent, error) {
	s.called++
	if s.called == 1 {
		return core.PartStartEvent{Index: 0, Part: s.response.Parts[0]}, nil
	}
	return nil, io.EOF
}
func (s *successTestStream) Response() *core.ModelResponse { return s.response }
func (s *successTestStream) Usage() core.Usage             { return s.response.Usage }
func (s *successTestStream) Close() error                  { return nil }

type countingStreamTestModel struct {
	response    *core.ModelResponse
	streamCalls int
}

func (m *countingStreamTestModel) ModelName() string { return "test-model" }
func (m *countingStreamTestModel) Request(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return m.response, nil
}
func (m *countingStreamTestModel) RequestStream(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (core.StreamedResponse, error) {
	m.streamCalls++
	return &successTestStream{response: m.response}, nil
}

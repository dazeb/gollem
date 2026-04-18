package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type sessionAwareTestModel struct {
	inner          *TestModel
	sessionCounter *atomic.Int32
	sessionID      int32
}

func (m *sessionAwareTestModel) Request(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters) (*ModelResponse, error) {
	return m.inner.Request(ctx, messages, settings, params)
}

func (m *sessionAwareTestModel) RequestStream(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters) (StreamedResponse, error) {
	return m.inner.RequestStream(ctx, messages, settings, params)
}

func (m *sessionAwareTestModel) ModelName() string {
	return m.inner.ModelName()
}

func (m *sessionAwareTestModel) NewSession() Model {
	id := m.sessionCounter.Add(1)
	return &sessionAwareTestModel{
		inner:          m.inner,
		sessionCounter: m.sessionCounter,
		sessionID:      id,
	}
}

// --- Test: Simple text output ---

func TestAgentRunTextOutput(t *testing.T) {
	model := NewTestModel(TextResponse("Hello, world!"))
	agent := NewAgent[string](model)

	result, err := agent.Run(context.Background(), "Say hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Hello, world!" {
		t.Errorf("output = %q, want 'Hello, world!'", result.Output)
	}
	if result.Usage.Requests != 1 {
		t.Errorf("requests = %d, want 1", result.Usage.Requests)
	}
	if result.RunID == "" {
		t.Error("expected non-empty RunID")
	}
}

func TestNewAgentClonesSessionAwareModel(t *testing.T) {
	counter := &atomic.Int32{}
	base := &sessionAwareTestModel{
		inner:          NewTestModel(TextResponse("ok")),
		sessionCounter: counter,
	}

	agent1 := NewAgent[string](base)
	agent2 := NewAgent[string](base)

	m1, ok := agent1.GetModel().(*sessionAwareTestModel)
	if !ok {
		t.Fatalf("agent1 model type = %T, want *sessionAwareTestModel", agent1.GetModel())
	}
	m2, ok := agent2.GetModel().(*sessionAwareTestModel)
	if !ok {
		t.Fatalf("agent2 model type = %T, want *sessionAwareTestModel", agent2.GetModel())
	}
	if m1 == base || m2 == base {
		t.Fatal("expected NewAgent to use cloned model sessions, not base model")
	}
	if m1.sessionID == 0 || m2.sessionID == 0 {
		t.Fatalf("expected non-zero session IDs, got m1=%d m2=%d", m1.sessionID, m2.sessionID)
	}
	if m1.sessionID == m2.sessionID {
		t.Fatalf("expected distinct session IDs, got both %d", m1.sessionID)
	}
}

func TestAgentRunTextWithSystemPrompt(t *testing.T) {
	model := NewTestModel(TextResponse("I am helpful"))
	agent := NewAgent[string](model,
		WithSystemPrompt[string]("You are a helpful assistant."),
	)

	result, err := agent.Run(context.Background(), "Who are you?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "I am helpful" {
		t.Errorf("output = %q, want 'I am helpful'", result.Output)
	}

	// Verify the model received system prompt.
	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	messages := calls[0].Messages
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	req, ok := messages[0].(ModelRequest)
	if !ok {
		t.Fatal("expected ModelRequest")
	}
	foundSystem := false
	for _, p := range req.Parts {
		if sp, ok := p.(SystemPromptPart); ok {
			if sp.Content == "You are a helpful assistant." {
				foundSystem = true
			}
		}
	}
	if !foundSystem {
		t.Error("expected system prompt in request")
	}
}

// --- Test: Structured output via final_result tool ---

type CityInfo struct {
	Name       string   `json:"name"`
	Country    string   `json:"country"`
	Population int      `json:"population"`
	Landmarks  []string `json:"landmarks"`
}

func TestAgentRunStructuredOutput(t *testing.T) {
	model := NewTestModel(
		ToolCallResponse("final_result", `{"name":"Tokyo","country":"Japan","population":14000000,"landmarks":["Tokyo Tower","Shibuya Crossing"]}`),
	)
	agent := NewAgent[CityInfo](model)

	result, err := agent.Run(context.Background(), "Tell me about Tokyo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output.Name != "Tokyo" {
		t.Errorf("name = %q, want 'Tokyo'", result.Output.Name)
	}
	if result.Output.Country != "Japan" {
		t.Errorf("country = %q, want 'Japan'", result.Output.Country)
	}
	if result.Output.Population != 14000000 {
		t.Errorf("population = %d, want 14000000", result.Output.Population)
	}
	if len(result.Output.Landmarks) != 2 {
		t.Errorf("landmarks count = %d, want 2", len(result.Output.Landmarks))
	}
}

// --- Test: Tool calls ---

func TestAgentRunWithToolCalls(t *testing.T) {
	// Model first calls a tool, then returns text.
	model := NewTestModel(
		ToolCallResponse("get_weather", `{"city":"London"}`),
		TextResponse("The weather in London is sunny, 22C."),
	)

	type WeatherParams struct {
		City string `json:"city"`
	}

	weatherTool := FuncTool[WeatherParams]("get_weather", "Get weather",
		func(_ context.Context, params WeatherParams) (string, error) {
			return fmt.Sprintf("Sunny, 22C in %s", params.City), nil
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](weatherTool),
	)

	result, err := agent.Run(context.Background(), "What's the weather in London?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "The weather in London is sunny, 22C." {
		t.Errorf("output = %q", result.Output)
	}
	if result.Usage.Requests != 2 {
		t.Errorf("requests = %d, want 2", result.Usage.Requests)
	}
	if result.Usage.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", result.Usage.ToolCalls)
	}
}

func TestAgentRunWithMultipleToolCalls(t *testing.T) {
	// Model calls two tools concurrently, then returns structured output.
	model := NewTestModel(
		MultiToolCallResponse(
			ToolCallPart{ToolName: "tool_a", ArgsJSON: `{"x":"1"}`, ToolCallID: "call_a"},
			ToolCallPart{ToolName: "tool_b", ArgsJSON: `{"x":"2"}`, ToolCallID: "call_b"},
		),
		TextResponse("Done with both tools."),
	)

	type Params struct {
		X string `json:"x"`
	}

	var callCount atomic.Int32

	toolA := FuncTool[Params]("tool_a", "Tool A",
		func(_ context.Context, params Params) (string, error) {
			callCount.Add(1)
			return "a:" + params.X, nil
		},
		WithToolConcurrencySafe(true),
	)
	toolB := FuncTool[Params]("tool_b", "Tool B",
		func(_ context.Context, params Params) (string, error) {
			callCount.Add(1)
			return "b:" + params.X, nil
		},
		WithToolConcurrencySafe(true),
	)

	agent := NewAgent[string](model,
		WithTools[string](toolA, toolB),
	)

	result, err := agent.Run(context.Background(), "Use both tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Done with both tools." {
		t.Errorf("output = %q", result.Output)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 tool calls, got %d", callCount.Load())
	}
}

// --- Test: Structured output with tool calls ---

func TestAgentRunToolThenStructured(t *testing.T) {
	type AnalysisResult struct {
		Summary string `json:"summary"`
		Score   int    `json:"score"`
	}
	type Params struct {
		Text string `json:"text"`
	}

	model := NewTestModel(
		ToolCallResponse("analyze", `{"text":"hello world"}`),
		ToolCallResponse("final_result", `{"summary":"Greeting","score":95}`),
	)

	analyzeTool := FuncTool[Params]("analyze", "Analyze text",
		func(_ context.Context, params Params) (string, error) {
			return "analysis of: " + params.Text, nil
		},
	)

	agent := NewAgent[AnalysisResult](model,
		WithTools[AnalysisResult](analyzeTool),
	)

	result, err := agent.Run(context.Background(), "Analyze this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output.Summary != "Greeting" {
		t.Errorf("summary = %q, want 'Greeting'", result.Output.Summary)
	}
	if result.Output.Score != 95 {
		t.Errorf("score = %d, want 95", result.Output.Score)
	}
}

// --- Test: Retry on validation failure ---

func TestAgentRunRetryOnValidation(t *testing.T) {
	callCount := 0
	model := NewTestModel(
		// First attempt: model returns bad output.
		ToolCallResponse("final_result", `{"name":"","country":"Japan","population":0,"landmarks":[]}`),
		// Second attempt: model returns good output.
		ToolCallResponse("final_result", `{"name":"Tokyo","country":"Japan","population":14000000,"landmarks":["Tower"]}`),
	)

	agent := NewAgent[CityInfo](model,
		WithMaxRetries[CityInfo](3),
		WithOutputValidator[CityInfo](func(_ context.Context, _ *RunContext, output CityInfo) (CityInfo, error) {
			callCount++
			if output.Name == "" {
				return output, NewModelRetryError("name cannot be empty")
			}
			return output, nil
		}),
	)

	result, err := agent.Run(context.Background(), "Tell me about Tokyo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output.Name != "Tokyo" {
		t.Errorf("name = %q, want 'Tokyo'", result.Output.Name)
	}
	if callCount != 2 {
		t.Errorf("validator called %d times, want 2", callCount)
	}
}

// --- Test: Max retries exceeded ---

func TestAgentRunMaxRetriesExceeded(t *testing.T) {
	// Model always returns bad output.
	model := NewTestModel(
		ToolCallResponse("final_result", `{"name":"","country":"","population":0,"landmarks":[]}`),
	)

	agent := NewAgent[CityInfo](model,
		WithMaxRetries[CityInfo](1),
		WithOutputValidator[CityInfo](func(_ context.Context, _ *RunContext, output CityInfo) (CityInfo, error) {
			return output, NewModelRetryError("invalid output")
		}),
	)

	_, err := agent.Run(context.Background(), "Tell me about Tokyo")
	if err == nil {
		t.Fatal("expected error for max retries exceeded")
	}
	var unexpectedErr *UnexpectedModelBehavior
	if !errors.As(err, &unexpectedErr) {
		t.Errorf("expected UnexpectedModelBehavior, got %T: %v", err, err)
	}
}

// --- Test: Usage limits ---

func TestAgentRunUsageLimitExceeded(t *testing.T) {
	// Model returns tool calls forever, but we limit to 2 requests.
	model := NewTestModel(
		ToolCallResponse("search", `{"q":"test"}`),
	)

	type Params struct {
		Q string `json:"q"`
	}

	searchTool := FuncTool[Params]("search", "Search",
		func(_ context.Context, _ Params) (string, error) {
			return "found", nil
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](searchTool),
		WithUsageLimits[string](UsageLimits{RequestLimit: IntPtr(2)}),
	)

	_, err := agent.Run(context.Background(), "Search forever")
	if err == nil {
		t.Fatal("expected error for usage limit exceeded")
	}
	var limitErr *UsageLimitExceeded
	if !errors.As(err, &limitErr) {
		t.Errorf("expected UsageLimitExceeded, got %T: %v", err, err)
	}
}

// --- Test: Context cancellation ---

func TestAgentRunContextCancelled(t *testing.T) {
	model := NewTestModel(TextResponse("This should never be seen"))

	agent := NewAgent[string](model)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := agent.Run(ctx, "Hello")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// --- Test: Unknown tool ---

func TestAgentRunUnknownTool(t *testing.T) {
	// Model calls an unknown tool, then returns text.
	model := NewTestModel(
		ToolCallResponse("nonexistent", `{}`),
		TextResponse("OK, I'll just answer directly."),
	)

	agent := NewAgent[string](model,
		WithMaxRetries[string](3),
	)

	result, err := agent.Run(context.Background(), "Do something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "OK, I'll just answer directly." {
		t.Errorf("output = %q", result.Output)
	}
}

// --- Test: Tool returning ModelRetryError ---

func TestAgentRunToolRetry(t *testing.T) {
	callCount := 0

	type Params struct {
		Query string `json:"query"`
	}

	model := NewTestModel(
		ToolCallResponse("search", `{"query":"bad"}`),
		ToolCallResponse("search", `{"query":"good"}`),
		TextResponse("Found the answer."),
	)

	searchTool := FuncTool[Params]("search", "Search",
		func(_ context.Context, params Params) (string, error) {
			callCount++
			if params.Query == "bad" {
				return "", NewModelRetryError("query too vague, be more specific")
			}
			return "results for: " + params.Query, nil
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](searchTool),
		WithMaxRetries[string](3),
	)

	result, err := agent.Run(context.Background(), "Search for something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Found the answer." {
		t.Errorf("output = %q", result.Output)
	}
	if callCount != 2 {
		t.Errorf("tool called %d times, want 2", callCount)
	}
}

// --- Test: Tool retry off-by-one regression ---
// Regression: the retry counter was incremented before the check, allowing
// maxRetries+1 tool executions instead of maxRetries.
func TestAgentRunToolRetryExactLimit(t *testing.T) {
	callCount := 0

	type Params struct{}

	// With maxRetries=2, the tool should be called at most 3 times:
	// initial call + 2 retries. The model then sees "exceeded maximum retries".
	// We need enough model responses: each tool call needs a ToolCallResponse,
	// plus the retry messages get sent back, and the model decides what to do.
	model := NewTestModel(
		ToolCallResponse("flaky", `{}`), // call 1 -> ModelRetryError
		ToolCallResponse("flaky", `{}`), // call 2 -> ModelRetryError (retry 1)
		ToolCallResponse("flaky", `{}`), // call 3 -> ModelRetryError (retry 2) -> exceeds limit
		TextResponse("gave up"),         // model gives up
	)

	tool := FuncTool[Params]("flaky", "Always fails",
		func(_ context.Context, _ Params) (string, error) {
			callCount++
			return "", NewModelRetryError("still broken")
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithMaxRetries[string](2),
	)

	result, err := agent.Run(context.Background(), "Use flaky tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be exactly 3 calls: initial + 2 retries.
	// Before the fix, it would be 4 calls (initial + 3 retries).
	if callCount != 3 {
		t.Errorf("tool called %d times, want 3 (initial + maxRetries=2)", callCount)
	}
	_ = result
}

// --- Test: Tool retry counter resets on success ---
// Regression: toolRetries was never reset on success, so cumulative
// ModelRetryError failures across separate (unrelated) invocations would
// eventually hit the max-retries limit even when the tool recovered each time.
func TestAgentRunToolRetryResetsOnSuccess(t *testing.T) {
	callCount := 0

	type Params struct {
		Query string `json:"query"`
	}

	// Sequence: fail → succeed → fail → succeed → done.
	// With maxRetries=1, each failure should get 1 retry allowance.
	// Without the fix, the second failure would immediately hit "exceeded
	// maximum retries" because the counter accumulated from the first failure.
	model := NewTestModel(
		ToolCallResponse("search", `{"query":"bad1"}`),  // call 1: fails
		ToolCallResponse("search", `{"query":"good1"}`), // call 2: succeeds (retry)
		ToolCallResponse("search", `{"query":"bad2"}`),  // call 3: fails (new attempt)
		ToolCallResponse("search", `{"query":"good2"}`), // call 4: succeeds (retry)
		TextResponse("All done."),
	)

	searchTool := FuncTool[Params]("search", "Search",
		func(_ context.Context, params Params) (string, error) {
			callCount++
			if strings.HasPrefix(params.Query, "bad") {
				return "", NewModelRetryError("invalid query: " + params.Query)
			}
			return "results for: " + params.Query, nil
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](searchTool),
		WithMaxRetries[string](1),
	)

	result, err := agent.Run(context.Background(), "Search twice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "All done." {
		t.Errorf("output = %q, want 'All done.'", result.Output)
	}
	// All 4 tool calls should execute (2 failures + 2 successes).
	if callCount != 4 {
		t.Errorf("tool called %d times, want 4 (fail+succeed, fail+succeed)", callCount)
	}

	// Verify the model never saw "exceeded maximum retries" — a successful
	// tool call should reset the per-tool retry counter so subsequent
	// failures get a fresh retry allowance.
	for i, call := range model.Calls() {
		for _, msg := range call.Messages {
			req, ok := msg.(ModelRequest)
			if !ok {
				continue
			}
			for _, part := range req.Parts {
				switch p := part.(type) {
				case RetryPromptPart:
					if strings.Contains(p.Content, "exceeded maximum retries") {
						t.Errorf("call %d: model saw 'exceeded maximum retries' — retry counter not reset on success: %s", i, p.Content)
					}
				}
			}
		}
	}
}

// --- Test: Message history ---

func TestAgentRunWithMessageHistory(t *testing.T) {
	model := NewTestModel(TextResponse("Continuing our conversation."))
	agent := NewAgent[string](model)

	history := []ModelMessage{
		ModelRequest{Parts: []ModelRequestPart{UserPromptPart{Content: "Previous question"}}},
		ModelResponse{Parts: []ModelResponsePart{TextPart{Content: "Previous answer"}}},
	}

	result, err := agent.Run(context.Background(), "Follow up",
		WithMessages(history...),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Continuing our conversation." {
		t.Errorf("output = %q", result.Output)
	}

	// Verify the model received the full history.
	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if len(calls[0].Messages) != 3 {
		t.Errorf("expected 3 messages (2 history + 1 new), got %d", len(calls[0].Messages))
	}
}

// --- Test: Empty response triggers retry ---

func TestAgentRunEmptyResponse(t *testing.T) {
	model := NewTestModel(
		// Empty response (no parts).
		&ModelResponse{Parts: []ModelResponsePart{}, FinishReason: FinishReasonStop},
		// Then a proper response.
		TextResponse("Here you go."),
	)

	agent := NewAgent[string](model,
		WithMaxRetries[string](3),
	)

	result, err := agent.Run(context.Background(), "Say something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Here you go." {
		t.Errorf("output = %q", result.Output)
	}
}

// --- Test: Content filter error ---

func TestAgentRunContentFilter(t *testing.T) {
	model := NewTestModel(
		&ModelResponse{Parts: []ModelResponsePart{}, FinishReason: FinishReasonContentFilter},
	)

	agent := NewAgent[string](model)

	_, err := agent.Run(context.Background(), "Something inappropriate")
	if err == nil {
		t.Fatal("expected error for content filter")
	}
	var filterErr *ContentFilterError
	if !errors.As(err, &filterErr) {
		t.Errorf("expected ContentFilterError, got %T: %v", err, err)
	}
}

// --- Test: Model request parameters include tools and output tools ---

func TestAgentRequestParameters(t *testing.T) {
	model := NewTestModel(
		ToolCallResponse("final_result", `{"name":"Test","country":"US","population":100,"landmarks":[]}`),
	)

	type Params struct {
		Q string `json:"q"`
	}

	searchTool := FuncTool[Params]("search", "Search",
		func(_ context.Context, _ Params) (string, error) {
			return "found", nil
		},
	)

	agent := NewAgent[CityInfo](model,
		WithTools[CityInfo](searchTool),
	)

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := model.Calls()
	params := calls[0].Parameters

	// Should have function tools.
	if len(params.FunctionTools) != 1 {
		t.Errorf("expected 1 function tool, got %d", len(params.FunctionTools))
	}
	if params.FunctionTools[0].Name != "search" {
		t.Errorf("function tool name = %q, want 'search'", params.FunctionTools[0].Name)
	}

	// Should have output tools (final_result).
	if len(params.OutputTools) != 1 {
		t.Errorf("expected 1 output tool, got %d", len(params.OutputTools))
	}
	if params.OutputTools[0].Name != "final_result" {
		t.Errorf("output tool name = %q, want 'final_result'", params.OutputTools[0].Name)
	}

	// OutputMode should be tool.
	if params.OutputMode != OutputModeTool {
		t.Errorf("output mode = %q, want 'tool'", params.OutputMode)
	}
}

// --- Test: Streaming ---

func TestAgentRunStream(t *testing.T) {
	model := NewTestModel(TextResponse("Hello from stream!"))
	agent := NewAgent[string](model)

	stream, err := agent.RunStream(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	// Consume events.
	var events []ModelResponseStreamEvent
	for event, err := range stream.StreamEvents() {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		events = append(events, event)
	}

	if len(events) == 0 {
		t.Error("expected at least one stream event")
	}
}

func TestAgentRunStreamText(t *testing.T) {
	model := NewTestModel(TextResponse("Hello stream text"))
	agent := NewAgent[string](model)

	stream, err := agent.RunStream(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	resp, err := stream.GetOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resp.TextContent()
	if text != "Hello stream text" {
		t.Errorf("text = %q, want 'Hello stream text'", text)
	}
}

func TestAgentRunStreamExecutesTools(t *testing.T) {
	type AddParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	var toolCalls int
	addTool := FuncTool[AddParams]("add", "Add two numbers", func(_ context.Context, p AddParams) (int, error) {
		toolCalls++
		return p.A + p.B, nil
	})

	model := NewTestModel(
		ToolCallResponse("add", `{"a":1,"b":2}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model, WithTools[string](addTool))

	stream, err := agent.RunStream(context.Background(), "Use the add tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	resp, err := stream.GetOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.TextContent(); got != "done" {
		t.Fatalf("text = %q, want %q", got, "done")
	}
	if toolCalls != 1 {
		t.Fatalf("toolCalls = %d, want 1", toolCalls)
	}

	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}

	lastReq, ok := calls[1].Messages[len(calls[1].Messages)-1].(ModelRequest)
	if !ok {
		t.Fatalf("last message type = %T, want ModelRequest", calls[1].Messages[len(calls[1].Messages)-1])
	}
	if len(lastReq.Parts) != 1 {
		t.Fatalf("tool result parts = %d, want 1", len(lastReq.Parts))
	}

	toolReturn, ok := lastReq.Parts[0].(ToolReturnPart)
	if !ok {
		t.Fatalf("tool result part type = %T, want ToolReturnPart", lastReq.Parts[0])
	}
	if toolReturn.ToolName != "add" {
		t.Fatalf("tool return tool = %q, want %q", toolReturn.ToolName, "add")
	}
	if toolReturn.Content != "3" {
		t.Fatalf("tool return content = %q, want %q", toolReturn.Content, "3")
	}
}

func TestAgentRunStreamResult(t *testing.T) {
	type Answer struct {
		Answer string `json:"answer"`
	}

	model := NewTestModel(
		ToolCallResponse("final_result", `{"answer":"42"}`),
	)
	agent := NewAgent[Answer](model)

	stream, err := agent.RunStream(context.Background(), "What is the answer?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	result, err := stream.Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output.Answer != "42" {
		t.Fatalf("output = %q, want %q", result.Output.Answer, "42")
	}

	resp, err := stream.GetOutput()
	if err != nil {
		t.Fatalf("unexpected error after Result: %v", err)
	}
	if len(resp.ToolCalls()) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls()))
	}
}

func TestAgentRunStreamHooks(t *testing.T) {
	type AddParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	var runStartCount int
	var runEndCount int
	var turnStartCount int
	var turnEndCount int
	var modelRequestCount int
	var modelResponseCount int

	hook := Hook{
		OnRunStart: func(context.Context, *RunContext, string) {
			runStartCount++
		},
		OnRunEnd: func(context.Context, *RunContext, []ModelMessage, error) {
			runEndCount++
		},
		OnTurnStart: func(context.Context, *RunContext, int) {
			turnStartCount++
		},
		OnTurnEnd: func(context.Context, *RunContext, int, *ModelResponse) {
			turnEndCount++
		},
		OnModelRequest: func(context.Context, *RunContext, []ModelMessage) {
			modelRequestCount++
		},
		OnModelResponse: func(context.Context, *RunContext, *ModelResponse) {
			modelResponseCount++
		},
	}

	addTool := FuncTool[AddParams]("add", "Add two numbers", func(_ context.Context, p AddParams) (int, error) {
		return p.A + p.B, nil
	})

	model := NewTestModel(
		ToolCallResponse("add", `{"a":1,"b":2}`),
		TextResponse("done"),
	)
	agent := NewAgent[string](model,
		WithTools[string](addTool),
		WithHooks[string](hook),
	)

	stream, err := agent.RunStream(context.Background(), "Use the add tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	if _, err := stream.GetOutput(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runStartCount != 1 {
		t.Fatalf("runStartCount = %d, want 1", runStartCount)
	}
	if runEndCount != 1 {
		t.Fatalf("runEndCount = %d, want 1", runEndCount)
	}
	if turnStartCount != 2 {
		t.Fatalf("turnStartCount = %d, want 2", turnStartCount)
	}
	if turnEndCount != 2 {
		t.Fatalf("turnEndCount = %d, want 2", turnEndCount)
	}
	if modelRequestCount != 2 {
		t.Fatalf("modelRequestCount = %d, want 2", modelRequestCount)
	}
	if modelResponseCount != 2 {
		t.Fatalf("modelResponseCount = %d, want 2", modelResponseCount)
	}
}

// --- Test: Non-object output type (outer key wrapping) ---

func TestAgentRunSliceOutput(t *testing.T) {
	// For []string output, the schema wraps in {"properties":{"result":{...}}}.
	model := NewTestModel(
		ToolCallResponse("final_result", `{"result":["alpha","beta","gamma"]}`),
	)
	agent := NewAgent[[]string](model)

	result, err := agent.Run(context.Background(), "Give me a list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Output) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Output))
	}
	if result.Output[0] != "alpha" {
		t.Errorf("first item = %q, want 'alpha'", result.Output[0])
	}
}

func TestAgentRunIntOutput(t *testing.T) {
	model := NewTestModel(
		ToolCallResponse("final_result", `{"result":42}`),
	)
	agent := NewAgent[int](model)

	result, err := agent.Run(context.Background(), "Pick a number")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != 42 {
		t.Errorf("output = %d, want 42", result.Output)
	}
}

// --- Test: RunResult contains full message history ---

func TestAgentRunResultMessages(t *testing.T) {
	model := NewTestModel(TextResponse("response"))
	agent := NewAgent[string](model)

	result, err := agent.Run(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have request + response.
	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}

	// First is request.
	if _, ok := result.Messages[0].(ModelRequest); !ok {
		t.Error("expected first message to be ModelRequest")
	}

	// Second is response.
	if _, ok := result.Messages[1].(ModelResponse); !ok {
		t.Error("expected second message to be ModelResponse")
	}
}

// --- Test: Deps passed to tool via RunContext ---

func TestAgentRunDeps(t *testing.T) {
	type MyDeps struct {
		APIKey string
	}

	type Params struct {
		Query string `json:"query"`
	}

	var capturedKey string

	model := NewTestModel(
		ToolCallResponse("search", `{"query":"test"}`),
		TextResponse("Done"),
	)

	searchTool := FuncTool[Params]("search", "Search",
		func(_ context.Context, rc *RunContext, _ Params) (string, error) {
			deps := rc.Deps.(*MyDeps)
			capturedKey = deps.APIKey
			return "result", nil
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](searchTool),
	)

	_, err := agent.Run(context.Background(), "Search",
		WithRunDeps(&MyDeps{APIKey: "secret-key"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedKey != "secret-key" {
		t.Errorf("captured key = %q, want 'secret-key'", capturedKey)
	}
}

// --- Test: FinishReason length without tool calls ---

func TestAgentRunFinishReasonLength(t *testing.T) {
	model := NewTestModel(
		&ModelResponse{Parts: []ModelResponsePart{}, FinishReason: FinishReasonLength},
	)

	agent := NewAgent[string](model)

	_, err := agent.Run(context.Background(), "Something long")
	if err == nil {
		t.Fatal("expected error for finish reason length")
	}
	if !strings.Contains(err.Error(), "token limit") {
		t.Errorf("expected token limit error, got: %v", err)
	}
}

// --- Regression: RunStream missing toolset tools ---

func TestRunStream_IncludesToolsetTools(t *testing.T) {
	type AddParams struct {
		A int `json:"a"`
	}
	addTool := FuncTool[AddParams]("add", "add numbers", func(ctx context.Context, p AddParams) (int, error) {
		return p.A, nil
	})
	toolset := NewToolset("math", addTool)

	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithToolsets[string](toolset),
	)

	_, err := agent.RunStream(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected model call")
	}
	params := calls[0].Parameters
	if params == nil {
		t.Fatal("expected parameters")
	}
	found := false
	for _, td := range params.FunctionTools {
		if td.Name == "add" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected toolset tool 'add' to be included in RunStream request parameters")
	}
}

// --- Regression: RunStream missing dynamic system prompts ---

func TestRunStream_IncludesDynamicSystemPrompts(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithDynamicSystemPrompt[string](func(_ context.Context, _ *RunContext) (string, error) {
			return "You are a dynamic assistant.", nil
		}),
	)

	_, err := agent.RunStream(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected model call")
	}
	msg := calls[0].Messages[0].(ModelRequest)
	foundDynamic := false
	for _, part := range msg.Parts {
		if sp, ok := part.(SystemPromptPart); ok && sp.Content == "You are a dynamic assistant." {
			foundDynamic = true
			break
		}
	}
	if !foundDynamic {
		t.Error("expected dynamic system prompt to be included in RunStream request")
	}
}

// --- Regression: Iter missing dynamic system prompts ---

func TestIter_IncludesDynamicSystemPrompts(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithDynamicSystemPrompt[string](func(_ context.Context, _ *RunContext) (string, error) {
			return "You are a dynamic assistant.", nil
		}),
	)

	iter := agent.Iter(context.Background(), "test")
	for !iter.Done() {
		_, err := iter.Next()
		if err != nil {
			break
		}
	}

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected model call")
	}
	msg := calls[0].Messages[0].(ModelRequest)
	foundDynamic := false
	for _, part := range msg.Parts {
		if sp, ok := part.(SystemPromptPart); ok && sp.Content == "You are a dynamic assistant." {
			foundDynamic = true
			break
		}
	}
	if !foundDynamic {
		t.Error("expected dynamic system prompt to be included in Iter request")
	}
}

// --- Regression: RunStream ignoring toolChoice ---

func TestRunStream_AppliesToolChoice(t *testing.T) {
	model := NewTestModel(TextResponse("ok"))
	agent := NewAgent[string](model,
		WithToolChoice[string](ToolChoiceAuto()),
	)

	_, err := agent.RunStream(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected model call")
	}
	settings := calls[0].Settings
	if settings == nil {
		t.Fatal("expected settings to be non-nil")
	}
	if settings.ToolChoice == nil {
		t.Error("expected tool choice to be set in RunStream settings")
	}
}

// --- Regression: Concurrent tool semaphore ignoring context cancellation ---

func TestConcurrentToolSemaphore_RespectsContextCancellation(t *testing.T) {
	type P struct{}

	// Create a tool that blocks until context is cancelled.
	blockingTool := FuncTool[P]("blocker", "blocks", func(ctx context.Context, p P) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, WithToolConcurrencySafe(true))

	// Create a tool that returns immediately.
	quickTool := FuncTool[P]("quick", "returns quickly", func(ctx context.Context, p P) (string, error) {
		return "done", nil
	}, WithToolConcurrencySafe(true))

	model := NewTestModel(
		// Request both tools simultaneously.
		MultiToolCallResponse(
			ToolCallPart{ToolName: "blocker", ArgsJSON: `{}`, ToolCallID: "call_blocker"},
			ToolCallPart{ToolName: "quick", ArgsJSON: `{}`, ToolCallID: "call_quick1"},
			ToolCallPart{ToolName: "quick", ArgsJSON: `{}`, ToolCallID: "call_quick2"},
			ToolCallPart{ToolName: "quick", ArgsJSON: `{}`, ToolCallID: "call_quick3"},
		),
		TextResponse("done"),
	)

	agent := NewAgent[string](model,
		WithTools[string](blockingTool, quickTool),
		WithMaxConcurrency[string](1), // Only 1 at a time; others wait on semaphore.
	)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a brief delay.
	go func() {
		// Small delay to let goroutines start.
		for range 1000000 {
		}
		cancel()
	}()

	_, err := agent.Run(ctx, "test")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestToolExecutionBatchesRespectExclusiveBarriers(t *testing.T) {
	type Params struct {
		ID string `json:"id"`
	}

	events := make(chan string, 8)
	releaseSafe := make(chan struct{})
	releaseExclusive := make(chan struct{})

	safeTool := FuncTool[Params]("safe", "safe tool", func(ctx context.Context, p Params) (string, error) {
		events <- "safe:start:" + p.ID
		<-releaseSafe
		events <- "safe:end:" + p.ID
		return "safe:" + p.ID, nil
	}, WithToolConcurrencySafe(true))
	exclusiveTool := FuncTool[Params]("exclusive", "exclusive tool", func(ctx context.Context, p Params) (string, error) {
		events <- "exclusive:start:" + p.ID
		<-releaseExclusive
		events <- "exclusive:end:" + p.ID
		return "exclusive:" + p.ID, nil
	})

	model := NewTestModel(
		MultiToolCallResponse(
			ToolCallPart{ToolName: "safe", ArgsJSON: `{"id":"1"}`, ToolCallID: "call_safe_1"},
			ToolCallPart{ToolName: "exclusive", ArgsJSON: `{"id":"2"}`, ToolCallID: "call_exclusive_2"},
			ToolCallPart{ToolName: "safe", ArgsJSON: `{"id":"3"}`, ToolCallID: "call_safe_3"},
		),
		TextResponse("done"),
	)

	agent := NewAgent[string](model,
		WithTools[string](safeTool, exclusiveTool),
		WithMaxConcurrency[string](3),
	)

	done := make(chan error, 1)
	go func() {
		_, err := agent.Run(context.Background(), "test batches")
		done <- err
	}()

	if got := <-events; got != "safe:start:1" {
		t.Fatalf("first event = %q, want safe:start:1", got)
	}

	select {
	case got := <-events:
		t.Fatalf("unexpected event before releasing first safe batch: %q", got)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseSafe)

	if got := <-events; got != "safe:end:1" {
		t.Fatalf("expected safe:end:1 after release, got %q", got)
	}
	if got := <-events; got != "exclusive:start:2" {
		t.Fatalf("expected exclusive:start:2 after first safe batch, got %q", got)
	}

	select {
	case got := <-events:
		t.Fatalf("unexpected event before releasing exclusive batch: %q", got)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseExclusive)

	if got := <-events; got != "exclusive:end:2" {
		t.Fatalf("expected exclusive:end:2 after release, got %q", got)
	}
	if got := <-events; got != "safe:start:3" {
		t.Fatalf("expected safe:start:3 after exclusive batch, got %q", got)
	}
	if got := <-events; got != "safe:end:3" {
		t.Fatalf("expected safe:end:3 after automatic second safe release, got %q", got)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("agent.Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run completion")
	}
}

// TestMultipleUnknownToolCallsSingleRetry verifies that multiple unknown tool
// calls in a single model response only increment the retry counter once.
// Before the fix, each unknown tool call incremented retries independently,
// exhausting retries immediately when compatible APIs returned mangled names.
func TestMultipleUnknownToolCallsSingleRetry(t *testing.T) {
	greetTool := FuncTool[struct{ Name string }]("greet", "Say hello",
		func(ctx context.Context, params struct{ Name string }) (string, error) {
			return "Hello, " + params.Name + "!", nil
		},
	)

	// First response: model calls 3 unknown tools (misspelled names).
	// Second response: model corrects itself and returns text.
	model := NewTestModel(
		// Response 1: Multiple unknown tool calls in a single response.
		&ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{ToolName: "greet_user", ArgsJSON: `{"name":"Alice"}`, ToolCallID: "call_1"},
				ToolCallPart{ToolName: "say_hello", ArgsJSON: `{"name":"Bob"}`, ToolCallID: "call_2"},
				ToolCallPart{ToolName: "hello", ArgsJSON: `{"name":"Eve"}`, ToolCallID: "call_3"},
			},
			FinishReason: FinishReasonToolCall,
			ModelName:    "test-model",
			Timestamp:    time.Now(),
		},
		// Response 2: Model corrects itself with text output.
		TextResponse("Hello everyone!"),
	)

	agent := NewAgent[string](model,
		WithTools[string](greetTool),
		WithMaxRetries[string](1),
	)

	result, err := agent.Run(context.Background(), "Say hello to everyone")
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if result.Output != "Hello everyone!" {
		t.Errorf("output = %q, want 'Hello everyone!'", result.Output)
	}

	// Verify the model was called twice (initial + retry).
	calls := model.Calls()
	if len(calls) != 2 {
		t.Errorf("expected 2 model calls (initial + retry), got %d", len(calls))
	}

	// Verify the retry message includes all 3 unknown tools.
	if len(calls) >= 2 {
		lastMsgs := calls[1].Messages
		var retryParts int
		for _, msg := range lastMsgs {
			if req, ok := msg.(ModelRequest); ok {
				for _, part := range req.Parts {
					if rp, ok := part.(RetryPromptPart); ok {
						retryParts++
						if !strings.Contains(rp.Content, "unknown tool") {
							t.Errorf("expected 'unknown tool' in retry, got: %s", rp.Content)
						}
					}
				}
			}
		}
		if retryParts != 3 {
			t.Errorf("expected 3 retry parts (one per unknown tool), got %d", retryParts)
		}
	}
}

// --- Test: Global result retry counter resets after successful tool execution ---
// Regression: state.retries (global result retry counter) was never reset,
// so scattered model misbehaviors across a long run (empty responses, unknown
// tools, parse errors) would accumulate and eventually exhaust the maxRetries
// limit, even when the model self-corrected between failures.
func TestAgentRunGlobalRetryResetsOnToolSuccess(t *testing.T) {
	type Params struct {
		Q string `json:"q"`
	}
	searchTool := FuncTool[Params]("search", "search", func(_ context.Context, p Params) (string, error) {
		return "results for: " + p.Q, nil
	})

	// Sequence:
	// 1. Empty response     → retries = 1
	// 2. Tool call "search" → executes successfully, retries should reset to 0
	// 3. Empty response     → retries should go to 1 (not 2)
	// 4. Tool call "search" → executes successfully, retries should reset to 0
	// 5. Text response      → done
	//
	// With maxRetries=1 and NO reset: step 3 would set retries=2, exceeding limit → error.
	// With reset: step 3 sets retries=1, which is within limit → continues normally.
	model := NewTestModel(
		&ModelResponse{Parts: []ModelResponsePart{}, FinishReason: FinishReasonStop}, // empty
		ToolCallResponse("search", `{"q":"first"}`),
		&ModelResponse{Parts: []ModelResponsePart{}, FinishReason: FinishReasonStop}, // empty
		ToolCallResponse("search", `{"q":"second"}`),
		TextResponse("All done."),
	)

	agent := NewAgent[string](model,
		WithTools[string](searchTool),
		WithMaxRetries[string](1),
	)

	result, err := agent.Run(context.Background(), "Search twice with hiccups")
	if err != nil {
		t.Fatalf("unexpected error (retries may not be resetting): %v", err)
	}
	if result.Output != "All done." {
		t.Errorf("output = %q, want 'All done.'", result.Output)
	}
}

func TestToolOutputTruncation_LargeOutput(t *testing.T) {
	// Tool returns a large string that should be truncated.
	largeOutput := strings.Repeat("line of output\n", 5000) // ~75K bytes = ~18K tokens

	model := NewTestModel(
		ToolCallResponse("big_tool", `{}`),
		TextResponse("done"),
	)

	tool := FuncTool[struct{}]("big_tool", "Returns large output",
		func(_ context.Context, _ struct{}) (string, error) {
			return largeOutput, nil
		},
	)

	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithToolOutputTruncation[string](TruncationConfig{MaxTokens: 1000}), // 4KB budget
	)

	result, err := agent.Run(context.Background(), "run it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "done" {
		t.Errorf("output = %q", result.Output)
	}

	// Check that the tool result in history was truncated.
	for _, msg := range result.Messages {
		req, ok := msg.(ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			tr, ok := part.(ToolReturnPart)
			if !ok || tr.ToolName != "big_tool" {
				continue
			}
			content, _ := tr.Content.(string)
			if len(content) >= len(largeOutput) {
				t.Errorf("expected truncated content, got full length %d", len(content))
			}
			if !strings.Contains(content, "truncated") {
				t.Error("expected truncation marker in content")
			}
		}
	}
}

func TestToolOutputTruncation_Disabled(t *testing.T) {
	// Without truncation option, large output should be preserved.
	largeOutput := strings.Repeat("x", 100000)

	model := NewTestModel(
		ToolCallResponse("big_tool", `{}`),
		TextResponse("done"),
	)

	tool := FuncTool[struct{}]("big_tool", "Returns large output",
		func(_ context.Context, _ struct{}) (string, error) {
			return largeOutput, nil
		},
	)

	agent := NewAgent[string](model, WithTools[string](tool))

	result, err := agent.Run(context.Background(), "run it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the tool result is NOT truncated.
	for _, msg := range result.Messages {
		req, ok := msg.(ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			tr, ok := part.(ToolReturnPart)
			if !ok || tr.ToolName != "big_tool" {
				continue
			}
			content, _ := tr.Content.(string)
			if len(content) != len(largeOutput) {
				t.Errorf("expected full content length %d, got %d", len(largeOutput), len(content))
			}
		}
	}
}

func TestToolOutputTruncation_StructuredContent(t *testing.T) {
	// Tool returns a struct; serialized JSON should be truncated.
	type BigResult struct {
		Data string `json:"data"`
	}

	model := NewTestModel(
		ToolCallResponse("struct_tool", `{}`),
		TextResponse("done"),
	)

	bigData := strings.Repeat("z", 100000)
	tool := Tool{
		Definition: ToolDefinition{
			Name:        "struct_tool",
			Description: "Returns structured output",
		},
		Handler: func(_ context.Context, _ *RunContext, _ string) (any, error) {
			return BigResult{Data: bigData}, nil
		},
	}

	agent := NewAgent[string](model,
		WithTools[string](tool),
		WithToolOutputTruncation[string](TruncationConfig{MaxTokens: 1000}),
	)

	result, err := agent.Run(context.Background(), "run it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The serialized JSON should be truncated.
	for _, msg := range result.Messages {
		req, ok := msg.(ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			tr, ok := part.(ToolReturnPart)
			if !ok || tr.ToolName != "struct_tool" {
				continue
			}
			content, _ := tr.Content.(string)
			if len(content) >= len(bigData) {
				t.Errorf("expected truncated structured content, got length %d", len(content))
			}
		}
	}
}

func TestDynamicPromptCaching_Unchanged(t *testing.T) {
	// Dynamic prompt returns the same value across two Run() calls.
	model := NewTestModel(
		TextResponse("first"),
		TextResponse("second"),
	)

	callCount := 0
	agent := NewAgent[string](model,
		WithDynamicSystemPrompt[string](func(_ context.Context, _ *RunContext) (string, error) {
			callCount++
			return "static dynamic prompt", nil
		}),
	)

	result1, err := agent.Run(context.Background(), "prompt 1")
	if err != nil {
		t.Fatal(err)
	}
	if result1.Output != "first" {
		t.Errorf("expected 'first', got %q", result1.Output)
	}

	result2, err := agent.Run(context.Background(), "prompt 2")
	if err != nil {
		t.Fatal(err)
	}
	if result2.Output != "second" {
		t.Errorf("expected 'second', got %q", result2.Output)
	}

	// The dynamic prompt func should have been called twice (once per Run).
	if callCount != 2 {
		t.Errorf("expected 2 calls to dynamic prompt func, got %d", callCount)
	}

	// Both runs should have the system prompt.
	for i, msgs := range [][]ModelMessage{result1.Messages, result2.Messages} {
		found := false
		for _, msg := range msgs {
			req, ok := msg.(ModelRequest)
			if !ok {
				continue
			}
			for _, part := range req.Parts {
				if sp, ok := part.(SystemPromptPart); ok && sp.Content == "static dynamic prompt" {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("run %d: expected dynamic system prompt in messages", i+1)
		}
	}
}

func TestDynamicPromptCaching_Changed(t *testing.T) {
	// Dynamic prompt returns different values across Run() calls.
	model := NewTestModel(
		TextResponse("first"),
		TextResponse("second"),
	)

	counter := 0
	agent := NewAgent[string](model,
		WithDynamicSystemPrompt[string](func(_ context.Context, _ *RunContext) (string, error) {
			counter++
			return fmt.Sprintf("prompt version %d", counter), nil
		}),
	)

	result1, err := agent.Run(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}

	result2, err := agent.Run(context.Background(), "p2")
	if err != nil {
		t.Fatal(err)
	}

	// Verify each run got the correct version.
	getSystemPrompt := func(msgs []ModelMessage) string {
		for _, msg := range msgs {
			req, ok := msg.(ModelRequest)
			if !ok {
				continue
			}
			for _, part := range req.Parts {
				if sp, ok := part.(SystemPromptPart); ok && strings.HasPrefix(sp.Content, "prompt version") {
					return sp.Content
				}
			}
		}
		return ""
	}

	if sp := getSystemPrompt(result1.Messages); sp != "prompt version 1" {
		t.Errorf("run 1: expected 'prompt version 1', got %q", sp)
	}
	if sp := getSystemPrompt(result2.Messages); sp != "prompt version 2" {
		t.Errorf("run 2: expected 'prompt version 2', got %q", sp)
	}
}

func TestDynamicPromptCaching_Multiple(t *testing.T) {
	// Multiple dynamic prompts, each cached independently.
	model := NewTestModel(TextResponse("done"))

	callA, callB := 0, 0
	agent := NewAgent[string](model,
		WithDynamicSystemPrompt[string](func(_ context.Context, _ *RunContext) (string, error) {
			callA++
			return "prompt A", nil
		}),
		WithDynamicSystemPrompt[string](func(_ context.Context, _ *RunContext) (string, error) {
			callB++
			return "prompt B", nil
		}),
	)

	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	if callA != 1 || callB != 1 {
		t.Errorf("expected 1 call each, got callA=%d callB=%d", callA, callB)
	}
}

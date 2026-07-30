package core

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestRunStreamConsumesQueuedSteerBeforeTerminalResult(t *testing.T) {
	model := NewTestModel(TextResponse("first answer"), TextResponse("steered answer"))
	var consumed []SteerMessage
	queue := NewSteerQueue(SteerQueueHooks{
		OnConsumed: func(message SteerMessage) error {
			consumed = append(consumed, message)
			return nil
		},
	})
	agent := NewAgent[string](model)
	stream, err := agent.RunStream(context.Background(), "initial prompt", WithSteerQueue(queue))
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if err := queue.Enqueue(SteerMessage{ID: "steer-1", Text: "change direction"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	result, err := stream.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if result.Output != "steered answer" {
		t.Fatalf("output = %q, want steered answer", result.Output)
	}
	if len(consumed) != 1 || consumed[0].ID != "steer-1" {
		t.Fatalf("consumed = %#v", consumed)
	}

	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	assertSteerMessage(t, calls[1].Messages, "change direction")
	if err := queue.Enqueue(SteerMessage{ID: "late", Text: "too late"}); !errors.Is(err, ErrSteerQueueClosed) {
		t.Fatalf("late Enqueue error = %v, want ErrSteerQueueClosed", err)
	}
}

func TestRunStreamRejectsPendingSteerWhenStreamFails(t *testing.T) {
	model := &failingSteerModel{}
	var rejected []SteerMessage
	queue := NewSteerQueue(SteerQueueHooks{
		OnRejected: func(message SteerMessage, _ error) {
			rejected = append(rejected, message)
		},
	})
	agent := NewAgent[string](model)
	stream, err := agent.RunStream(context.Background(), "initial", WithSteerQueue(queue))
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if err := queue.Enqueue(SteerMessage{ID: "steer-failed", Text: "never consumed"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := stream.Result(); err == nil {
		t.Fatal("Result succeeded")
	}
	if len(rejected) != 1 || rejected[0].ID != "steer-failed" {
		t.Fatalf("rejected = %#v", rejected)
	}
}

func TestRunStreamRejectsPendingSteerWhenStartupGuardrailFails(t *testing.T) {
	var rejected []SteerMessage
	queue := NewSteerQueue(SteerQueueHooks{
		OnRejected: func(message SteerMessage, _ error) {
			rejected = append(rejected, message)
		},
	})
	if err := queue.Enqueue(SteerMessage{ID: "steer-startup", Text: "never starts"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	agent := NewAgent[string](
		NewTestModel(TextResponse("unused")),
		WithInputGuardrail[string]("reject", func(context.Context, string) (string, error) {
			return "", errors.New("startup rejected")
		}),
	)

	if _, err := agent.RunStream(context.Background(), "initial", WithSteerQueue(queue)); err == nil {
		t.Fatal("RunStream succeeded")
	}
	if len(rejected) != 1 || rejected[0].ID != "steer-startup" {
		t.Fatalf("rejected = %#v", rejected)
	}
	if err := queue.Enqueue(SteerMessage{ID: "late", Text: "too late"}); !errors.Is(err, ErrSteerQueueClosed) {
		t.Fatalf("late Enqueue error = %v, want ErrSteerQueueClosed", err)
	}
}

func TestRunStreamConsumesQueuedSteersInOrderAfterToolResults(t *testing.T) {
	type echoParams struct {
		Text string `json:"text"`
	}
	model := NewTestModel(
		ToolCallResponseWithID("echo", `{"text":"tool result"}`, "tool-call"),
		TextResponse("steered answer"),
	)
	echo := FuncTool[echoParams]("echo", "Echo text.", func(_ context.Context, params echoParams) (string, error) {
		return params.Text, nil
	})
	var consumed []string
	queue := NewSteerQueue(SteerQueueHooks{
		OnConsumed: func(message SteerMessage) error {
			consumed = append(consumed, message.ID)
			return nil
		},
	})
	agent := NewAgent[string](model, WithTools[string](echo))
	stream, err := agent.RunStream(context.Background(), "initial", WithSteerQueue(queue))
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	for _, message := range []SteerMessage{
		{ID: "first", Text: "first adjustment"},
		{ID: "second", Text: "second adjustment"},
	} {
		if err := queue.Enqueue(message); err != nil {
			t.Fatalf("Enqueue(%s): %v", message.ID, err)
		}
	}

	result, err := stream.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if result.Output != "steered answer" || !slices.Equal(consumed, []string{"first", "second"}) {
		t.Fatalf("result = %q, consumed = %v", result.Output, consumed)
	}
	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	messages := calls[1].Messages
	if len(messages) < 2 {
		t.Fatalf("second request messages = %#v", messages)
	}
	toolResults, ok := messages[len(messages)-2].(ModelRequest)
	if !ok || len(toolResults.Parts) != 1 {
		t.Fatalf("tool result message = %#v", messages[len(messages)-2])
	}
	if _, ok := toolResults.Parts[0].(ToolReturnPart); !ok {
		t.Fatalf("tool result part = %#v", toolResults.Parts[0])
	}
	steerRequest, ok := messages[len(messages)-1].(ModelRequest)
	if !ok || len(steerRequest.Parts) != 2 {
		t.Fatalf("steer request = %#v", messages[len(messages)-1])
	}
	for i, want := range []string{"first adjustment", "second adjustment"} {
		part, ok := steerRequest.Parts[i].(UserPromptPart)
		if !ok || part.Content != want {
			t.Fatalf("steer part %d = %#v, want %q", i, steerRequest.Parts[i], want)
		}
	}
}

func TestRunRejectsSteerQueueForSynchronousExecution(t *testing.T) {
	var rejected []SteerMessage
	queue := NewSteerQueue(SteerQueueHooks{
		OnRejected: func(message SteerMessage, _ error) {
			rejected = append(rejected, message)
		},
	})
	if err := queue.Enqueue(SteerMessage{ID: "sync", Text: "cannot steer sync"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	agent := NewAgent[string](NewTestModel(TextResponse("unused")))

	if _, err := agent.Run(context.Background(), "initial", WithSteerQueue(queue)); !errors.Is(err, ErrSteeringNeedsStreaming) {
		t.Fatalf("Run error = %v, want ErrSteeringNeedsStreaming", err)
	}
	if len(rejected) != 1 || rejected[0].ID != "sync" {
		t.Fatalf("rejected = %#v", rejected)
	}
}

func TestSteerQueueRejectsPendingInstructionsOverCapacity(t *testing.T) {
	queue := NewSteerQueue(SteerQueueHooks{})
	for i := range SteerQueueMaxPending {
		if err := queue.Enqueue(SteerMessage{ID: "queued", Text: "queued"}); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}
	if err := queue.Enqueue(SteerMessage{ID: "overflow", Text: "too many"}); !errors.Is(err, ErrSteerQueueFull) {
		t.Fatalf("overflow Enqueue error = %v, want ErrSteerQueueFull", err)
	}
}

func TestRunStreamConsumesSteerOnImmediateResponseInterceptorRetry(t *testing.T) {
	model := NewTestModel(TextResponse("drop this"), TextResponse("after retry"))
	dropped := false
	agent := NewAgent[string](
		model,
		WithResponseInterceptor[string](func(context.Context, *ModelResponse) InterceptResult {
			if !dropped {
				dropped = true
				return InterceptResult{Action: MessageDrop}
			}
			return InterceptResult{Action: MessageAllow}
		}),
	)
	var consumed []SteerMessage
	queue := NewSteerQueue(SteerQueueHooks{
		OnConsumed: func(message SteerMessage) error {
			consumed = append(consumed, message)
			return nil
		},
	})
	stream, err := agent.RunStream(context.Background(), "initial", WithSteerQueue(queue))
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if err := queue.Enqueue(SteerMessage{ID: "interceptor", Text: "include on retry"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	result, err := stream.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if result.Output != "after retry" || len(consumed) != 1 || consumed[0].ID != "interceptor" {
		t.Fatalf("result = %q, consumed = %#v", result.Output, consumed)
	}
	calls := model.Calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	assertSteerMessage(t, calls[1].Messages, "include on retry")
}

func TestRunStreamRejectsSteerBeforeProviderRequestWhenPreparationFails(t *testing.T) {
	model := NewTestModel(TextResponse("first answer"), TextResponse("must not start"))
	var rejected []SteerMessage
	queue := NewSteerQueue(SteerQueueHooks{
		OnPrepared: func([]SteerMessage) error {
			return errors.New("boundary unavailable")
		},
		OnRejected: func(message SteerMessage, _ error) {
			rejected = append(rejected, message)
		},
	})
	agent := NewAgent[string](model)
	stream, err := agent.RunStream(context.Background(), "initial", WithSteerQueue(queue))
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if err := queue.Enqueue(SteerMessage{ID: "prepare", Text: "must not send"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := stream.Result(); err == nil || !strings.Contains(err.Error(), "boundary unavailable") {
		t.Fatalf("Result error = %v, want preparation failure", err)
	}
	if len(model.Calls()) != 1 {
		t.Fatalf("model calls = %d, want no request after preparation failure", len(model.Calls()))
	}
	if len(rejected) != 1 || rejected[0].ID != "prepare" {
		t.Fatalf("rejected = %#v", rejected)
	}
}

func assertSteerMessage(t *testing.T, messages []ModelMessage, want string) {
	t.Helper()
	for _, message := range messages {
		request, ok := message.(ModelRequest)
		if !ok {
			continue
		}
		for _, part := range request.Parts {
			if prompt, ok := part.(UserPromptPart); ok && prompt.Content == want {
				return
			}
		}
	}
	t.Fatalf("messages = %#v, missing steer %q", messages, want)
}

type failingSteerModel struct{}

func (*failingSteerModel) Request(
	context.Context,
	[]ModelMessage,
	*ModelSettings,
	*ModelRequestParameters,
) (*ModelResponse, error) {
	return nil, errors.New("not used")
}

func (*failingSteerModel) RequestStream(
	context.Context,
	[]ModelMessage,
	*ModelSettings,
	*ModelRequestParameters,
) (StreamedResponse, error) {
	return failingSteerStream{}, nil
}

func (*failingSteerModel) ModelName() string { return "failing-steer" }

type failingSteerStream struct{}

func (failingSteerStream) Next() (ModelResponseStreamEvent, error) {
	return nil, errors.New("stream failed")
}

func (failingSteerStream) Response() *ModelResponse { return nil }
func (failingSteerStream) Usage() Usage             { return Usage{} }
func (failingSteerStream) Close() error             { return nil }

var _ = io.EOF

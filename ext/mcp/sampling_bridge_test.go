package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

type recordingModel struct {
	requestFn func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error)
	name      string
}

func (m *recordingModel) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return m.requestFn(ctx, messages, settings, params)
}

func (m *recordingModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

func (m *recordingModel) ModelName() string {
	if m.name != "" {
		return m.name
	}
	return "recording-model"
}

type recordingRequester struct {
	last *CreateMessageParams
	fn   func(context.Context, *CreateMessageParams) (*CreateMessageResult, error)
}

func (r *recordingRequester) CreateMessage(ctx context.Context, params *CreateMessageParams) (*CreateMessageResult, error) {
	r.last = params
	return r.fn(ctx, params)
}

func TestModelSamplingHandlerReturnsToolUse(t *testing.T) {
	model := &recordingModel{
		name: "mock-client-model",
		requestFn: func(_ context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
			if len(messages) != 2 {
				t.Fatalf("expected system + user message, got %d", len(messages))
			}
			req0, ok := messages[0].(core.ModelRequest)
			if !ok || len(req0.Parts) != 1 {
				t.Fatalf("message[0] = %#v", messages[0])
			}
			if got := req0.Parts[0].(core.SystemPromptPart).Content; got != "Be terse" {
				t.Fatalf("unexpected system prompt: %q", got)
			}
			req1, ok := messages[1].(core.ModelRequest)
			if !ok || len(req1.Parts) != 1 {
				t.Fatalf("message[1] = %#v", messages[1])
			}
			if got := req1.Parts[0].(core.UserPromptPart).Content; got != "weather?" {
				t.Fatalf("unexpected user prompt: %q", got)
			}
			if settings == nil || settings.MaxTokens == nil || *settings.MaxTokens != 128 {
				t.Fatalf("unexpected settings: %+v", settings)
			}
			if params == nil || len(params.FunctionTools) != 1 || params.FunctionTools[0].Name != "lookup" {
				t.Fatalf("unexpected params: %+v", params)
			}
			return &core.ModelResponse{
				ModelName:    "mock-client-model",
				FinishReason: core.FinishReasonToolCall,
				Parts: []core.ModelResponsePart{
					core.ToolCallPart{
						ToolName:   "lookup",
						ToolCallID: "call_1",
						ArgsJSON:   `{"city":"Paris"}`,
					},
				},
			}, nil
		},
	}

	handler := ModelSamplingHandler(model)
	result, err := handler(context.Background(), &CreateMessageParams{
		SystemPrompt: "Be terse",
		Messages: []SamplingMessage{
			{
				Role:    "user",
				Content: MarshalSamplingContent(Content{Type: "text", Text: "weather?"}),
			},
		},
		MaxTokens: 128,
		Tools: []SamplingTool{
			{
				Name:        "lookup",
				Description: "Lookup city weather",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolChoice: &SamplingToolChoice{Mode: "auto"},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.StopReason != "toolUse" {
		t.Fatalf("unexpected stop reason: %q", result.StopReason)
	}
	blocks, err := ParseSamplingContent(result.Content)
	if err != nil {
		t.Fatalf("failed to parse sampling result content: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Type != "tool_use" || blocks[0].Name != "lookup" || blocks[0].ID != "call_1" {
		t.Fatalf("unexpected content blocks: %+v", blocks)
	}
}

func TestMCPModelConvertsHistoryAndToolUse(t *testing.T) {
	requester := &recordingRequester{
		fn: func(_ context.Context, params *CreateMessageParams) (*CreateMessageResult, error) {
			if params.SystemPrompt != "sys" {
				t.Fatalf("unexpected system prompt: %q", params.SystemPrompt)
			}
			if len(params.Messages) != 3 {
				t.Fatalf("expected 3 sampling messages, got %d", len(params.Messages))
			}
			assistantBlocks, err := ParseSamplingContent(params.Messages[1].Content)
			if err != nil {
				t.Fatalf("failed to parse assistant history: %v", err)
			}
			if len(assistantBlocks) != 1 || assistantBlocks[0].Type != "tool_use" || assistantBlocks[0].ID != "call_prev" {
				t.Fatalf("unexpected assistant history: %+v", assistantBlocks)
			}
			userBlocks, err := ParseSamplingContent(params.Messages[2].Content)
			if err != nil {
				t.Fatalf("failed to parse tool result history: %v", err)
			}
			if len(userBlocks) != 1 || userBlocks[0].Type != "tool_result" || userBlocks[0].ToolUseID != "call_prev" {
				t.Fatalf("unexpected user tool result history: %+v", userBlocks)
			}
			return &CreateMessageResult{
				Role: "assistant",
				Content: MarshalSamplingContentArray([]Content{
					{Type: "text", Text: "Need a tool"},
					{Type: "tool_use", Name: "lookup", ID: "call_next", Input: json.RawMessage(`{"city":"Rome"}`)},
				}),
				Model:      "client-model",
				StopReason: "toolUse",
			}, nil
		},
	}

	model := NewMCPModel(requester, WithMCPModelName("client-model"))
	maxTokens := 96
	resp, err := model.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.SystemPromptPart{Content: "sys"},
				core.UserPromptPart{Content: "Question"},
			},
		},
		core.ModelResponse{
			FinishReason: core.FinishReasonToolCall,
			Parts: []core.ModelResponsePart{
				core.ToolCallPart{ToolName: "lookup", ToolCallID: "call_prev", ArgsJSON: `{"city":"Paris"}`},
			},
		},
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.ToolReturnPart{ToolCallID: "call_prev", Content: map[string]any{"temp": 72}},
			},
		},
	}, &core.ModelSettings{MaxTokens: &maxTokens}, &core.ModelRequestParameters{
		FunctionTools: []core.ToolDefinition{
			{
				Name:             "lookup",
				Description:      "Lookup city weather",
				ParametersSchema: core.Schema{"type": "object"},
				Kind:             core.ToolKindFunction,
			},
		},
	})
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}
	if resp.FinishReason != core.FinishReasonToolCall {
		t.Fatalf("unexpected finish reason: %s", resp.FinishReason)
	}
	if resp.ModelName != "client-model" {
		t.Fatalf("unexpected model name: %q", resp.ModelName)
	}
	if model.ModelName() != "client-model" {
		t.Fatalf("ModelName did not update: %q", model.ModelName())
	}
	if len(resp.Parts) != 2 {
		t.Fatalf("unexpected response parts: %+v", resp.Parts)
	}
	if _, ok := resp.Parts[0].(core.TextPart); !ok {
		t.Fatalf("expected text part, got %T", resp.Parts[0])
	}
	if tc, ok := resp.Parts[1].(core.ToolCallPart); !ok || tc.ToolCallID != "call_next" || tc.ToolName != "lookup" {
		t.Fatalf("unexpected tool call part: %+v", resp.Parts[1])
	}
}

func TestClientHandlesServerRequestsOverStdio(t *testing.T) {
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	client := &Client{
		clientState: newClientState(ClientConfig{
			RootsProvider: StaticRoots(Root{URI: "file:///tmp/project", Name: "project"}),
			SamplingHandler: func(_ context.Context, params *CreateMessageParams) (*CreateMessageResult, error) {
				if len(params.Messages) != 1 {
					t.Fatalf("unexpected sampling params: %+v", params)
				}
				return &CreateMessageResult{
					Role:       "assistant",
					Content:    MarshalSamplingContent(Content{Type: "text", Text: "sampled"}),
					Model:      "local",
					StopReason: "endTurn",
				}, nil
			},
			ElicitationHandler: func(_ context.Context, params *ElicitationParams) (*ElicitationResult, error) {
				if params.Message == "" {
					t.Fatal("expected elicitation message")
				}
				return &ElicitationResult{
					Action:  "accept",
					Content: map[string]any{"name": "Trevor"},
				}, nil
			},
		}),
		stdin:  &pipeWriteCloser{w: clientWriter},
		stdout: bufio.NewReader(clientReader),
	}
	go client.readLoop()
	t.Cleanup(func() {
		client.shutdown()
		_ = clientWriter.Close()
		_ = serverWriter.Close()
		_ = serverReader.Close()
	})

	serverOut := bufio.NewReader(serverReader)
	sendAndRead := func(request jsonRPCMessage) jsonRPCResponse {
		data, _ := json.Marshal(request)
		_, _ = fmt.Fprintf(serverWriter, "%s\n", data)
		line, err := serverOut.ReadBytes('\n')
		if err != nil {
			t.Fatalf("failed to read client response: %v", err)
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("failed to decode client response: %v", err)
		}
		return resp
	}

	resp := sendAndRead(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "roots/list",
	})
	if resp.Error != nil {
		t.Fatalf("roots/list returned error: %+v", resp.Error)
	}
	var roots ListRootsResult
	data, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(data, &roots); err != nil {
		t.Fatalf("failed to parse roots result: %v", err)
	}
	if len(roots.Roots) != 1 || roots.Roots[0].URI != "file:///tmp/project" {
		t.Fatalf("unexpected roots: %+v", roots.Roots)
	}

	resp = sendAndRead(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(2),
		Method:  "sampling/createMessage",
		Params: mustRawJSON(tMarshal(CreateMessageParams{
			Messages: []SamplingMessage{
				{Role: "user", Content: MarshalSamplingContent(Content{Type: "text", Text: "hello"})},
			},
			MaxTokens: 32,
		})),
	})
	if resp.Error != nil {
		t.Fatalf("sampling/createMessage returned error: %+v", resp.Error)
	}

	resp = sendAndRead(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(3),
		Method:  "elicitation/create",
		Params: mustRawJSON(tMarshal(ElicitationParams{
			Message:         "Who are you?",
			RequestedSchema: json.RawMessage(`{"type":"object"}`),
		})),
	})
	if resp.Error != nil {
		t.Fatalf("elicitation/create returned error: %+v", resp.Error)
	}
}

func TestNestedSamplingRoundTripOverStdio(t *testing.T) {
	serverToClientReader, serverToClientWriter := io.Pipe()
	clientToServerReader, clientToServerWriter := io.Pipe()

	server := NewServer(WithServerInfo(ServerInfo{Name: "test-server", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "ask_client",
		Description: "Ask the connected client model a question",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"]}`),
	}, func(ctx context.Context, rc *RequestContext, args map[string]any) (*ToolResult, error) {
		prompt, _ := args["prompt"].(string)
		model := NewMCPModel(rc)
		resp, err := model.Request(ctx, []core.ModelMessage{
			core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{Content: prompt},
				},
			},
		}, &core.ModelSettings{MaxTokens: intPtr(32)}, nil)
		if err != nil {
			return nil, err
		}
		return textToolResult(resp.TextContent()), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := NewStdioServerTransport(server, clientToServerReader, nopPipeWriteCloser{PipeWriter: serverToClientWriter})
	go func() {
		_ = transport.Run(ctx)
	}()

	model := &recordingModel{
		requestFn: func(_ context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
			if len(messages) != 1 {
				t.Fatalf("unexpected nested sampling messages: %+v", messages)
			}
			req := messages[0].(core.ModelRequest)
			if got := req.Parts[0].(core.UserPromptPart).Content; got != "hello from tool" {
				t.Fatalf("unexpected nested sampling prompt: %q", got)
			}
			return &core.ModelResponse{
				ModelName:    "nested-model",
				FinishReason: core.FinishReasonStop,
				Parts: []core.ModelResponsePart{
					core.TextPart{Content: "client says hi"},
				},
			}, nil
		},
	}

	client := &Client{
		clientState: newClientState(ClientConfig{
			SamplingHandler: ModelSamplingHandler(model),
		}),
		stdin:  &pipeWriteCloser{w: clientToServerWriter},
		stdout: bufio.NewReader(serverToClientReader),
	}
	go client.readLoop()
	t.Cleanup(func() {
		client.shutdown()
		_ = clientToServerWriter.Close()
		_ = clientToServerReader.Close()
		_ = serverToClientWriter.Close()
		_ = serverToClientReader.Close()
	})

	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer initCancel()
	if err := client.initialize(initCtx); err != nil {
		t.Fatalf("client initialize failed: %v", err)
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer callCancel()
	result, err := client.CallTool(callCtx, "ask_client", map[string]any{"prompt": "hello from tool"})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if got := result.TextContent(); got != "client says hi" {
		t.Fatalf("unexpected tool result: %q", got)
	}
}

type nopPipeWriteCloser struct {
	*io.PipeWriter
}

func textToolResult(text string) *ToolResult {
	return &ToolResult{
		Content: []Content{{Type: "text", Text: text}},
	}
}

func intPtr(v int) *int { return &v }

package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/appserver/catalog"
	"github.com/fugue-labs/gollem/appserver/protocol"
)

func TestInteractionUserInputRequestResolvesFromJSONRPCResponse(t *testing.T) {
	server := readyServer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resultCh := make(chan interactionResult, 1)
	go func() {
		resp, err := server.interact.RequestUserInput(ctx, UserInputRequest{
			ThreadID: "thread-1",
			TurnID:   "turn-1",
			Prompt:   "Need a value",
			Options:  []string{"one", "two"},
			Required: true,
		})
		resultCh <- interactionResult{resp: resp, err: err}
	}()

	req := waitForServerRequest(t, server)
	if req.Method != InteractionRequestUserInput {
		t.Fatalf("request method = %q, want %s", req.Method, InteractionRequestUserInput)
	}
	var params map[string]any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("decode request params: %v", err)
	}
	if params["prompt"] != "Need a value" || params["threadId"] != "thread-1" || params["turnId"] != "turn-1" {
		t.Fatalf("request params = %#v", params)
	}
	if err := server.HandleResponse(ctx, protocol.Response{
		ID:     req.ID,
		Result: json.RawMessage(`{"text":"one"}`),
	}); err != nil {
		t.Fatalf("HandleResponse: %v", err)
	}

	got := <-resultCh
	if got.err != nil {
		t.Fatalf("interaction result error: %v", got.err)
	}
	if string(got.resp.Result) != `{"text":"one"}` || got.resp.ThreadID != "thread-1" {
		t.Fatalf("interaction response = %#v", got.resp)
	}
	notification := waitForNotification(t, server, "serverRequest/resolved")
	var resolved serverRequestResolvedParams
	if err := json.Unmarshal(notification.Params, &resolved); err != nil {
		t.Fatalf("decode resolved notification: %v", err)
	}
	requestID, _ := req.ID.Value().(string)
	if resolved.RequestID != requestID || resolved.ThreadID != "thread-1" {
		t.Fatalf("resolved params = %#v, requestID=%q", resolved, requestID)
	}
}

func TestInteractionToolCallErrorResponse(t *testing.T) {
	server := readyServer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resultCh := make(chan interactionResult, 1)
	go func() {
		resp, err := server.interact.RequestToolCall(ctx, DynamicToolCallRequest{
			ThreadID:  "thread-2",
			TurnID:    "turn-2",
			CallID:    "call-2",
			ToolName:  "client.search",
			Arguments: json.RawMessage(`{"query":"gollem"}`),
		})
		resultCh <- interactionResult{resp: resp, err: err}
	}()

	req := waitForServerRequest(t, server)
	if req.Method != InteractionToolCall {
		t.Fatalf("request method = %q, want %s", req.Method, InteractionToolCall)
	}
	var params protocol.DynamicToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("decode dynamic tool params: %v", err)
	}
	if params.ThreadID != "thread-2" || params.TurnID != "turn-2" || params.CallID != "call-2" ||
		params.Namespace != nil || params.Tool != "client.search" || params.ToolName != params.Tool {
		t.Fatalf("dynamic tool params = %#v", params)
	}
	if err := server.HandleResponse(ctx, protocol.Response{
		ID:    req.ID,
		Error: &protocol.Error{Code: protocol.CodeInvalidRequest, Message: "tool refused"},
	}); err != nil {
		t.Fatalf("HandleResponse: %v", err)
	}
	got := <-resultCh
	if !errors.Is(got.err, ErrInteractionRequestFailed) {
		t.Fatalf("interaction error = %v, want ErrInteractionRequestFailed", got.err)
	}
	if got.resp.Error == nil || got.resp.Error.Message != "tool refused" {
		t.Fatalf("interaction response = %#v", got.resp)
	}
}

func TestInteractionToolCallResponseUsesExactBoundedContract(t *testing.T) {
	tests := []struct {
		name     string
		result   string
		wantFail bool
	}{
		{
			name:   "text and image",
			result: `{"contentItems":[{"type":"inputText","text":"match"},{"type":"inputImage","imageUrl":"data:image/png;base64,AA=="}],"success":true}`,
		},
		{name: "missing fields", result: `{}`, wantFail: true},
		{name: "null content", result: `{"contentItems":null,"success":true}`, wantFail: true},
		{name: "invalid variant", result: `{"contentItems":[{"type":"inputText","imageUrl":"bad"}],"success":true}`, wantFail: true},
		{name: "invalid success", result: `{"contentItems":[],"success":"yes"}`, wantFail: true},
		{
			name:     "oversized",
			result:   `{"contentItems":[{"type":"inputText","text":"` + strings.Repeat("x", runtimeInteractionPayloadMaxBytes) + `"}],"success":true}`,
			wantFail: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := readyServer()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resultCh := make(chan interactionResult, 1)
			go func() {
				resp, err := server.interact.RequestToolCall(ctx, DynamicToolCallRequest{
					ThreadID:  "thread-contract",
					TurnID:    "turn-contract",
					ItemID:    "item-contract",
					ToolName:  "client.search",
					Arguments: json.RawMessage(`{"query":"gollem"}`),
				})
				resultCh <- interactionResult{resp: resp, err: err}
			}()
			req := waitForServerRequest(t, server)
			var params protocol.DynamicToolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil || params.CallID != "item-contract" {
				t.Fatalf("dynamic tool fallback params = %#v, error %v", params, err)
			}
			err := server.HandleResponse(ctx, protocol.Response{ID: req.ID, Result: json.RawMessage(tt.result)})
			got := <-resultCh
			if tt.wantFail {
				if err == nil || !errors.Is(got.err, ErrInteractionRequestFailed) || got.resp.Error == nil {
					t.Fatalf("HandleResponse error = %v, interaction = %#v/%v", err, got.resp, got.err)
				}
				return
			}
			if err != nil || got.err != nil || string(got.resp.Result) != tt.result {
				t.Fatalf("HandleResponse error = %v, interaction = %#v/%v", err, got.resp, got.err)
			}
		})
	}
}

func TestServeJSONLinesResolvesInteractionResponse(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := ServeJSONLines(ctx, server, inR, outW)
		if err != nil {
			_ = outW.CloseWithError(err)
		} else {
			_ = outW.Close()
		}
		errCh <- err
	}()
	scanner := bufio.NewScanner(outR)
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"interaction-jsonl","version":"1.0.0"}}}`)
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &initResp); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}
	writeInputLine(t, inW, `{"method":"initialized"}`)

	resultCh := make(chan interactionResult, 1)
	go func() {
		resp, err := server.interact.RequestMCPElicitation(ctx, MCPElicitationRequest{
			ThreadID: "thread-3",
			TurnID:   "turn-3",
			ServerID: "mcp-test",
			Message:  "choose",
		})
		resultCh <- interactionResult{resp: resp, err: err}
	}()

	var serverReq protocol.Request
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &serverReq); err != nil {
		t.Fatalf("decode server request: %v", err)
	}
	if serverReq.Method != InteractionMCPElicitation {
		t.Fatalf("server request method = %q, want %s", serverReq.Method, InteractionMCPElicitation)
	}
	requestID, _ := serverReq.ID.Value().(string)
	writeInputLine(t, inW, `{"id":"`+requestID+`","result":{"accepted":true}}`)
	resolvedLine := readOutputLine(t, scanner)
	var resolved protocol.Notification
	if err := json.Unmarshal([]byte(resolvedLine), &resolved); err != nil {
		t.Fatalf("decode resolved notification: %v", err)
	}
	if resolved.Method != "serverRequest/resolved" {
		t.Fatalf("resolved method = %q", resolved.Method)
	}
	got := <-resultCh
	if got.err != nil {
		t.Fatalf("interaction result error: %v", got.err)
	}
	if strings.TrimSpace(string(got.resp.Result)) != `{"accepted":true}` {
		t.Fatalf("interaction result = %s", got.resp.Result)
	}
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
}

func TestServeJSONLinesResolvesExactDynamicToolCall(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := ServeJSONLines(ctx, server, inR, outW)
		if err != nil {
			_ = outW.CloseWithError(err)
		} else {
			_ = outW.Close()
		}
		errCh <- err
	}()
	scanner := bufio.NewScanner(outR)
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"dynamic-tool-jsonl","version":"1.0.0"}}}`)
	var initResp protocol.Response
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &initResp); err != nil || initResp.Error != nil {
		t.Fatalf("initialize response = %#v, error %v", initResp, err)
	}
	writeInputLine(t, inW, `{"method":"initialized"}`)

	resultCh := make(chan interactionResult, 1)
	go func() {
		resp, err := server.interact.RequestToolCall(ctx, DynamicToolCallRequest{
			ThreadID:  "thread-jsonl",
			TurnID:    "turn-jsonl",
			ItemID:    "item-jsonl",
			CallID:    "call-jsonl",
			ToolName:  "client.search",
			Arguments: json.RawMessage(`{"query":"gollem"}`),
		})
		resultCh <- interactionResult{resp: resp, err: err}
	}()

	var serverReq protocol.Request
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &serverReq); err != nil {
		t.Fatalf("decode server request: %v", err)
	}
	var params protocol.DynamicToolCallParams
	if err := json.Unmarshal(serverReq.Params, &params); err != nil {
		t.Fatalf("decode dynamic tool params: %v", err)
	}
	if serverReq.Method != InteractionToolCall || params.CallID != "call-jsonl" || params.Namespace != nil ||
		params.Tool != "client.search" || params.ThreadID != "thread-jsonl" || params.TurnID != "turn-jsonl" {
		t.Fatalf("dynamic tool request = %s/%#v", serverReq.Method, params)
	}
	responseLine, err := json.Marshal(protocol.Response{
		ID: serverReq.ID,
		Result: mustInteractionJSON(t, protocol.DynamicToolCallResponse{
			ContentItems: []protocol.DynamicToolCallOutputContentItem{{Type: "inputText", Text: "match"}},
			Success:      true,
		}),
	})
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	writeInputLine(t, inW, string(responseLine))
	var resolved protocol.Notification
	if err := json.Unmarshal([]byte(readOutputLine(t, scanner)), &resolved); err != nil || resolved.Method != "serverRequest/resolved" {
		t.Fatalf("resolved notification = %#v, error %v", resolved, err)
	}
	got := <-resultCh
	if got.err != nil {
		t.Fatalf("dynamic tool result: %v", got.err)
	}
	var response protocol.DynamicToolCallResponse
	if err := json.Unmarshal(got.resp.Result, &response); err != nil || !response.Success || len(response.ContentItems) != 1 {
		t.Fatalf("dynamic tool response = %#v, error %v", response, err)
	}
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ServeJSONLines: %v", err)
	}
}

func TestToolListIncludesInteractions(t *testing.T) {
	server := readyServer()
	resp := server.HandleRequest(context.Background(), request("tool/list", map[string]any{"includeUnavailable": true}))
	if resp.Error != nil {
		t.Fatalf("tool/list error: %v", resp.Error)
	}
	var tools catalog.ToolListResponse
	if err := json.Unmarshal(resp.Result, &tools); err != nil {
		t.Fatalf("decode tool/list: %v", err)
	}
	if !containsCatalogTool(tools.Data, "interactions") {
		t.Fatalf("tool/list missing interactions tool: %#v", tools.Data)
	}
}

type interactionResult struct {
	resp InteractionResponse
	err  error
}

func mustInteractionJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

func containsCatalogTool(tools []catalog.Tool, id string) bool {
	for _, tool := range tools {
		if tool.ID == id {
			return true
		}
	}
	return false
}

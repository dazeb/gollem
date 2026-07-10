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
			ToolName:  "client.search",
			Arguments: json.RawMessage(`{"query":"gollem"}`),
		})
		resultCh <- interactionResult{resp: resp, err: err}
	}()

	req := waitForServerRequest(t, server)
	if req.Method != InteractionToolCall {
		t.Fatalf("request method = %q, want %s", req.Method, InteractionToolCall)
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
	writeInputLine(t, inW, `{"id":"init","method":"initialize","params":{"clientInfo":{"name":"interaction-jsonl"}}}`)
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

func containsCatalogTool(tools []catalog.Tool, id string) bool {
	for _, tool := range tools {
		if tool.ID == id {
			return true
		}
	}
	return false
}

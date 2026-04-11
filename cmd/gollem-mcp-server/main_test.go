package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
)

// sendRequest formats a JSON-RPC request line.
func sendRequest(t *testing.T, id interface{}, method string, params interface{}) string {
	t.Helper()
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if id != nil {
		req["id"] = id
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(data) + "\n"
}

// readResponse reads a single JSON-RPC response from the output buffer.
func readResponse(t *testing.T, output string) jsonRPCResponse {
	t.Helper()
	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", output, err)
	}
	return resp
}

// runServer runs the MCP server with the given input lines and returns all output lines.
func runServer(t *testing.T, lines ...string) []string {
	t.Helper()
	input := strings.Join(lines, "")
	reader := strings.NewReader(input)
	var output bytes.Buffer
	logBuf := io.Discard

	srv := NewServer(reader, &output, logBuf)
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("server run: %v", err)
	}

	var results []string
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			results = append(results, line)
		}
	}
	return results
}

func TestInitializeHandshake(t *testing.T) {
	initReq := sendRequest(t, 1, "initialize", map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	})
	notifReq := sendRequest(t, nil, "notifications/initialized", nil)

	lines := runServer(t, initReq, notifReq)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}

	resp := readResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Parse the result.
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var initResult initializeResult
	if err := json.Unmarshal(resultJSON, &initResult); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}

	if initResult.ProtocolVersion != protocolVersion {
		t.Errorf("protocol version = %q, want %q", initResult.ProtocolVersion, protocolVersion)
	}
	if initResult.ServerInfo.Name != "gollem-mcp-server" {
		t.Errorf("server name = %q, want %q", initResult.ServerInfo.Name, "gollem-mcp-server")
	}
	if initResult.Capabilities.Tools == nil {
		t.Error("expected tools capability to be set")
	}
}

func TestToolsList(t *testing.T) {
	listReq := sendRequest(t, 2, "tools/list", nil)
	lines := runServer(t, listReq)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	resp := readResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var listResult toolsListResult
	if err := json.Unmarshal(resultJSON, &listResult); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}

	if len(listResult.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(listResult.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range listResult.Tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range []string{"run_agent", "execute_python", "list_providers"} {
		if !toolNames[expected] {
			t.Errorf("expected tool %q in list", expected)
		}
	}
}

func TestToolsListSchema(t *testing.T) {
	listReq := sendRequest(t, 1, "tools/list", nil)
	lines := runServer(t, listReq)

	resp := readResponse(t, lines[0])
	resultJSON, _ := json.Marshal(resp.Result)

	var listResult toolsListResult
	json.Unmarshal(resultJSON, &listResult)

	// Verify run_agent has required prompt field.
	for _, tool := range listResult.Tools {
		if tool.Name == "run_agent" {
			schemaJSON, _ := json.Marshal(tool.InputSchema)
			var schema map[string]interface{}
			json.Unmarshal(schemaJSON, &schema)

			required, ok := schema["required"].([]interface{})
			if !ok {
				t.Fatal("run_agent schema missing required field")
			}
			if len(required) != 1 || required[0] != "prompt" {
				t.Errorf("run_agent required = %v, want [prompt]", required)
			}

			props, ok := schema["properties"].(map[string]interface{})
			if !ok {
				t.Fatal("run_agent schema missing properties")
			}
			if _, ok := props["prompt"]; !ok {
				t.Error("run_agent schema missing prompt property")
			}
			if _, ok := props["model"]; !ok {
				t.Error("run_agent schema missing model property")
			}
			if _, ok := props["provider"]; !ok {
				t.Error("run_agent schema missing provider property")
			}
		}
	}
}

func TestResolveProviderNameDefaultsToOpenAI(t *testing.T) {
	if got := resolveProviderName(map[string]any{}); got != "openai" {
		t.Fatalf("resolveProviderName() = %q, want openai", got)
	}
	if got := resolveProviderName(map[string]any{"provider": "   "}); got != "openai" {
		t.Fatalf("resolveProviderName(whitespace) = %q, want openai", got)
	}
}

func TestResolveProviderNamePreservesExplicitProvider(t *testing.T) {
	if got := resolveProviderName(map[string]any{"provider": "mcp"}); got != "mcp" {
		t.Fatalf("resolveProviderName(mcp) = %q, want mcp", got)
	}
}

func TestCallListProviders(t *testing.T) {
	callReq := sendRequest(t, 3, "tools/call", map[string]interface{}{
		"name":      "list_providers",
		"arguments": map[string]interface{}{},
	})

	lines := runServer(t, callReq)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	resp := readResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result toolCallResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}

	if result.IsError {
		t.Error("expected isError to be false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("content type = %q, want %q", result.Content[0].Type, "text")
	}

	// Verify the text contains provider info.
	text := result.Content[0].Text
	for _, name := range []string{"mcp", "openai", "anthropic", "ollama"} {
		if !strings.Contains(text, name) {
			t.Errorf("provider list should contain %q", name)
		}
	}
}

func TestCallExecutePython(t *testing.T) {
	callReq := sendRequest(t, 4, "tools/call", map[string]interface{}{
		"name": "execute_python",
		"arguments": map[string]interface{}{
			"code":            "1 + 2",
			"timeout_seconds": 10,
		},
	})

	lines := runServer(t, callReq)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	resp := readResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result toolCallResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}

	if result.IsError {
		t.Errorf("expected isError=false, got true: %s", result.Content[0].Text)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}

	// 1+2 = 3
	if !strings.Contains(result.Content[0].Text, "3") {
		t.Errorf("expected result to contain '3', got %q", result.Content[0].Text)
	}
}

func TestCallExecutePythonWithPrint(t *testing.T) {
	callReq := sendRequest(t, 5, "tools/call", map[string]interface{}{
		"name": "execute_python",
		"arguments": map[string]interface{}{
			"code": `print("hello world")
42`,
		},
	})

	lines := runServer(t, callReq)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	resp := readResponse(t, lines[0])
	resultJSON, _ := json.Marshal(resp.Result)
	var result toolCallResult
	json.Unmarshal(resultJSON, &result)

	if result.IsError {
		t.Errorf("expected isError=false: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", text)
	}
	if !strings.Contains(text, "42") {
		t.Errorf("expected result to contain '42', got %q", text)
	}
}

func TestCallExecutePythonMissingCode(t *testing.T) {
	callReq := sendRequest(t, 6, "tools/call", map[string]interface{}{
		"name":      "execute_python",
		"arguments": map[string]interface{}{},
	})

	lines := runServer(t, callReq)
	resp := readResponse(t, lines[0])
	resultJSON, _ := json.Marshal(resp.Result)
	var result toolCallResult
	json.Unmarshal(resultJSON, &result)

	if !result.IsError {
		t.Error("expected isError=true for missing code")
	}
	if !strings.Contains(result.Content[0].Text, "code is required") {
		t.Errorf("expected error about missing code, got %q", result.Content[0].Text)
	}
}

func TestHandleExecutePythonConcurrent(t *testing.T) {
	srv := &Server{}

	const workers = 4
	start := make(chan struct{})
	errCh := make(chan error, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			result, err := srv.handleExecutePython(context.Background(), nil, map[string]any{
				"code":            "1 + 2",
				"timeout_seconds": 10.0,
			})
			if err != nil {
				errCh <- err
				return
			}
			if result == nil || result.IsError || len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "3") {
				errCh <- io.ErrUnexpectedEOF
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent execute_python failed: %v", err)
		}
	}
}

func TestCallRunAgentMissingPrompt(t *testing.T) {
	callReq := sendRequest(t, 7, "tools/call", map[string]interface{}{
		"name":      "run_agent",
		"arguments": map[string]interface{}{},
	})

	lines := runServer(t, callReq)
	resp := readResponse(t, lines[0])
	resultJSON, _ := json.Marshal(resp.Result)
	var result toolCallResult
	json.Unmarshal(resultJSON, &result)

	if !result.IsError {
		t.Error("expected isError=true for missing prompt")
	}
	if !strings.Contains(result.Content[0].Text, "prompt is required") {
		t.Errorf("expected error about missing prompt, got %q", result.Content[0].Text)
	}
}

func TestUnknownTool(t *testing.T) {
	callReq := sendRequest(t, 8, "tools/call", map[string]interface{}{
		"name":      "nonexistent_tool",
		"arguments": map[string]interface{}{},
	})

	lines := runServer(t, callReq)
	resp := readResponse(t, lines[0])

	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if resp.Error.Code != codeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, codeMethodNotFound)
	}
}

func TestUnknownMethod(t *testing.T) {
	callReq := sendRequest(t, 9, "nonexistent/method", nil)

	lines := runServer(t, callReq)
	resp := readResponse(t, lines[0])

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != codeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, codeMethodNotFound)
	}
}

func TestMultipleRequests(t *testing.T) {
	initReq := sendRequest(t, 1, "initialize", map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0.0"},
	})
	notifReq := sendRequest(t, nil, "notifications/initialized", nil)
	listReq := sendRequest(t, 2, "tools/list", nil)
	provReq := sendRequest(t, 3, "tools/call", map[string]interface{}{
		"name":      "list_providers",
		"arguments": map[string]interface{}{},
	})

	lines := runServer(t, initReq, notifReq, listReq, provReq)

	// Should get 3 responses: initialize, tools/list, tools/call.
	// notifications/initialized has no id so no response.
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d: %v", len(lines), lines)
	}

	seen := make(map[float64]bool, len(lines))
	for i, line := range lines {
		resp := readResponse(t, line)
		if resp.Error != nil {
			t.Errorf("response %d: unexpected error: %v", i, resp.Error)
		}
		id, ok := resp.ID.(float64)
		if !ok {
			t.Errorf("response %d: expected numeric ID, got %T", i, resp.ID)
			continue
		}
		seen[id] = true
	}
	for _, expectedID := range []float64{1, 2, 3} {
		if !seen[expectedID] {
			t.Errorf("missing response id %v in %v", expectedID, lines)
		}
	}
}

func TestStringID(t *testing.T) {
	callReq := sendRequest(t, "abc-123", "tools/list", nil)
	lines := runServer(t, callReq)
	resp := readResponse(t, lines[0])

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.ID != "abc-123" {
		t.Errorf("id = %v, want %q", resp.ID, "abc-123")
	}
}

func TestNormalizeID(t *testing.T) {
	// Test integer ID.
	intRaw := json.RawMessage(`42`)
	result := normalizeID(&intRaw)
	if v, ok := result.(int64); !ok || v != 42 {
		t.Errorf("normalizeID(42) = %v (%T), want int64(42)", result, result)
	}

	// Test string ID.
	strRaw := json.RawMessage(`"hello"`)
	result = normalizeID(&strRaw)
	if v, ok := result.(string); !ok || v != "hello" {
		t.Errorf("normalizeID(\"hello\") = %v (%T), want string(hello)", result, result)
	}

	// Test nil ID.
	result = normalizeID(nil)
	if result != nil {
		t.Errorf("normalizeID(nil) = %v, want nil", result)
	}
}

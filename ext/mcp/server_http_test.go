package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPServerTransportDiscover(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "disc-test", Version: "1.0.0"}))
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	resp := postJSON(t, httpServer.URL, jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "server/discover",
		Params:  mustRawJSON(statelessParams(nil)),
	})

	if resp.Error != nil {
		t.Fatalf("discover error: %+v", resp.Error)
	}
	var result DiscoverResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to parse discover result: %v", err)
	}
	if len(result.SupportedVersions) != 1 || result.SupportedVersions[0] != ProtocolVersion {
		t.Fatalf("unexpected protocol versions: %+v", result.SupportedVersions)
	}
	if result.ServerInfo == nil || result.ServerInfo.Name != "disc-test" {
		t.Fatalf("unexpected server info: %+v", result.ServerInfo)
	}
}

func TestHTTPServerTransportResultTypeAndCache(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "cache-test", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "echo",
		Description: "echo",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ *RequestContext, _ map[string]any) (*ToolResult, error) {
		return textToolResult("hi"), nil
	})
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	// tools/list stamps resultType "complete" and cacheable fields.
	listResp := postJSON(t, httpServer.URL, jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/list",
		Params:  mustRawJSON(statelessParams(nil)),
	})
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %+v", listResp.Error)
	}
	var raw map[string]any
	if err := json.Unmarshal(listResp.Result, &raw); err != nil {
		t.Fatalf("failed to parse tools/list result: %v", err)
	}
	if rt, _ := raw["resultType"].(string); rt != ResultTypeComplete {
		t.Fatalf("resultType = %v, want %q", raw["resultType"], ResultTypeComplete)
	}
	if _, ok := raw["ttlMs"]; !ok {
		t.Fatalf("expected ttlMs on tools/list result, got %+v", raw)
	}
	if scope, _ := raw["cacheScope"].(string); scope != CacheScopePublic {
		t.Fatalf("cacheScope = %v, want %q", raw["cacheScope"], CacheScopePublic)
	}

	meta, _ := raw["_meta"].(map[string]any)
	if si, _ := meta[MetaServerInfo].(map[string]any); si == nil || si["name"] != "cache-test" {
		t.Fatalf("expected _meta.serverInfo on tools/list result, got %+v", meta)
	}
}

func TestHTTPServerTransportMissingProtocolVersion(t *testing.T) {
	server := NewServer()
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	// Params with _meta lacking protocolVersion.
	params := mustRawJSON([]byte(`{"_meta":{"io.modelcontextprotocol/clientCapabilities":{}}}`))
	resp := postJSON(t, httpServer.URL, jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/list",
		Params:  params,
	})
	if resp.Error == nil || resp.Error.Code != jsonRPCCodeInvalidParams {
		t.Fatalf("expected -32602 for missing protocolVersion, got %+v", resp.Error)
	}
}

func TestHTTPServerTransportUnsupportedProtocolVersion(t *testing.T) {
	server := NewServer()
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	params := mustRawJSON([]byte(`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25","io.modelcontextprotocol/clientCapabilities":{}}}`))
	resp := postJSON(t, httpServer.URL, jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/list",
		Params:  params,
	})
	if resp.Error == nil || resp.Error.Code != jsonRPCCodeUnsupportedProtocolVersion {
		t.Fatalf("expected -32022 for unsupported protocol version, got %+v", resp.Error)
	}
}

func TestHTTPServerTransportMissingClientCapabilities(t *testing.T) {
	server := NewServer()
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	params := mustRawJSON([]byte(`{"_meta":{"io.modelcontextprotocol/protocolVersion":"` + ProtocolVersion + `"}}`))
	resp := postJSON(t, httpServer.URL, jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/list",
		Params:  params,
	})
	if resp.Error == nil || resp.Error.Code != jsonRPCCodeInvalidParams {
		t.Fatalf("expected -32602 for missing clientCapabilities, got %+v", resp.Error)
	}
}

func TestHTTPServerTransportHeaderMismatchMethod(t *testing.T) {
	server := NewServer()
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	body, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/list",
		Params:  mustRawJSON(statelessParams(nil)),
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, httpServer.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Method", "tools/call")
	resp := doHTTP(t, req)
	if resp.Error == nil || resp.Error.Code != jsonRPCCodeHeaderMismatch {
		t.Fatalf("expected -32020 HeaderMismatch, got %+v", resp.Error)
	}
}

func TestHTTPServerTransportHeaderMismatchName(t *testing.T) {
	server := NewServer()
	server.AddTool(Tool{
		Name:        "greet",
		Description: "greet",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ *RequestContext, _ map[string]any) (*ToolResult, error) {
		return textToolResult("hi"), nil
	})
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	callParams := mustRawJSON(statelessParams(map[string]any{"name": "greet", "arguments": map[string]any{}}))
	body, _ := json.Marshal(jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/call",
		Params:  callParams,
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, httpServer.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "wrong_name")
	resp := doHTTP(t, req)
	if resp.Error == nil || resp.Error.Code != jsonRPCCodeHeaderMismatch {
		t.Fatalf("expected -32020 HeaderMismatch for Mcp-Name, got %+v", resp.Error)
	}
}

func TestHTTPServerTransportMRTRRoundTrip(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "mrtr-test", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "ask_name",
		Description: "Ask the client for their name via elicitation",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, rc *RequestContext, _ map[string]any) (*ToolResult, error) {
		responses := rc.InputResponses()
		if len(responses) == 0 {
			id, req := BuildElicitationInputRequest(&ElicitationParams{
				Message:         "What is your name?",
				RequestedSchema: json.RawMessage(`{"type":"object"}`),
			})
			rc.NeedInput(InputRequests{id: req}, "opaque-state-1")
			return nil, nil
		}
		// Retry: consume the elicitation answer.
		var name string
		for _, raw := range responses {
			el, err := ParseElicitationResult(raw)
			if err != nil {
				return nil, err
			}
			if v, ok := el.Content["name"].(string); ok {
				name = v
			}
		}
		if rc.RequestState() != "opaque-state-1" {
			return nil, &jsonRPCError{Code: jsonRPCCodeInvalidParams, Message: "requestState not echoed verbatim"}
		}
		return textToolResult("hello " + name), nil
	})

	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{
		ElicitationHandler: func(_ context.Context, params *ElicitationParams) (*ElicitationResult, error) {
			if params.Message != "What is your name?" {
				t.Fatalf("unexpected elicitation message: %q", params.Message)
			}
			return &ElicitationResult{Action: "accept", Content: map[string]any{"name": "Trevor"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(ctx, "ask_name", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if got := result.TextContent(); got != "hello Trevor" {
		t.Fatalf("unexpected tool result: %q", got)
	}
}

func TestHTTPServerTransportMissingRequiredClientCapability(t *testing.T) {
	server := NewServer()
	server.AddTool(Tool{
		Name:        "need_sampling",
		Description: "Requires sampling capability the client did not declare",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, rc *RequestContext, _ map[string]any) (*ToolResult, error) {
		if len(rc.InputResponses()) > 0 {
			return textToolResult("done"), nil
		}
		id, req := BuildSamplingInputRequest(&CreateMessageParams{
			Messages:  []SamplingMessage{{Role: "user", Content: MarshalSamplingContent(Content{Type: "text", Text: "hi"})}},
			MaxTokens: 16,
		})
		rc.NeedInput(InputRequests{id: req}, "")
		return nil, nil
	})

	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Client advertises no sampling capability.
	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{})
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}
	defer client.Close()

	_, err = client.CallTool(ctx, "need_sampling", map[string]any{})
	if err == nil {
		t.Fatal("expected MissingRequiredClientCapability error")
	}
	var rpcErr *jsonRPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *jsonRPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != jsonRPCCodeMissingRequiredClientCapability {
		t.Fatalf("expected -32021, got %d: %s", rpcErr.Code, rpcErr.Message)
	}
	var data struct {
		RequiredCapabilities []string `json:"requiredCapabilities"`
	}
	if err := json.Unmarshal(rpcErr.Data, &data); err != nil {
		t.Fatalf("failed to parse error data: %v", err)
	}
	if len(data.RequiredCapabilities) != 1 || data.RequiredCapabilities[0] != "sampling" {
		t.Fatalf("expected requiredCapabilities=[sampling], got %+v", data.RequiredCapabilities)
	}
}

func TestHTTPServerTransportGETIsSubscriptionsSeam(t *testing.T) {
	transport := NewHTTPServerTransport(NewServer())
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 for subscriptions seam, got %d", resp.StatusCode)
	}
}

// statelessParams builds request params carrying the required _meta envelope.
func statelessParams(extra map[string]any) []byte {
	meta := map[string]any{
		MetaProtocolVersion:    ProtocolVersion,
		MetaClientCapabilities: map[string]any{},
	}
	obj := map[string]any{"_meta": meta}
	for k, v := range extra {
		obj[k] = v
	}
	data, _ := json.Marshal(obj)
	return data
}

func postJSON(t *testing.T, url string, msg jsonRPCMessage) *jsonRPCResponse {
	t.Helper()
	body, _ := json.Marshal(msg)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return doHTTP(t, req)
}

func doHTTP(t *testing.T, req *http.Request) *jsonRPCResponse {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	var out jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return &out
}

func TestHTTPServerTransportToolSchemaRootTypeCompletion(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "schema-test", Version: "1.0.0"}))
	server.AddTool(Tool{
		Name:        "sloppy",
		Description: "schema whose root omits type",
		InputSchema: json.RawMessage(`{"properties":{"run_id":{"type":"string"}},"required":["run_id"]}`),
	}, func(_ context.Context, _ *RequestContext, _ map[string]any) (*ToolResult, error) {
		return textToolResult("ok"), nil
	})
	transport := NewHTTPServerTransport(server)
	httpServer := httptest.NewServer(transport)
	defer httpServer.Close()

	listResp := postJSON(t, httpServer.URL, jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      rawJSONID(1),
		Method:  "tools/list",
		Params:  mustRawJSON(statelessParams(nil)),
	})
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %+v", listResp.Error)
	}
	var list struct {
		Tools []struct {
			InputSchema map[string]json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(listResp.Result, &list); err != nil {
		t.Fatalf("failed to parse tools/list result: %v", err)
	}
	if len(list.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(list.Tools))
	}
	if got := string(list.Tools[0].InputSchema["type"]); got != `"object"` {
		t.Fatalf(`expected root schema type "object", got %s`, got)
	}
	if got := string(list.Tools[0].InputSchema["properties"]); !strings.Contains(got, "run_id") {
		t.Fatalf("expected original properties preserved, got %s", got)
	}
}

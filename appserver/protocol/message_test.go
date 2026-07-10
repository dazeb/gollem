package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessagesOmitJSONRPCOnMarshal(t *testing.T) {
	req := Request{
		JSONRPC: "2.0",
		ID:      NewNumberID(1),
		Method:  "initialize",
		Params:  json.RawMessage(`{"clientInfo":{"name":"test"}}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if strings.Contains(string(data), "jsonrpc") {
		t.Fatalf("request should omit jsonrpc, got %s", data)
	}
	assertJSONContains(t, data, `"method":"initialize"`)
	assertJSONContains(t, data, `"id":1`)

	notif := Notification{JSONRPC: "2.0", Method: "initialized"}
	data, err = json.Marshal(notif)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	if strings.Contains(string(data), "jsonrpc") {
		t.Fatalf("notification should omit jsonrpc, got %s", data)
	}
	assertJSONContains(t, data, `"method":"initialized"`)

	resp := Response{JSONRPC: "2.0", ID: NewStringID("req-1"), Result: json.RawMessage(`{"ok":true}`)}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(data), "jsonrpc") {
		t.Fatalf("response should omit jsonrpc, got %s", data)
	}
	assertJSONContains(t, data, `"id":"req-1"`)
	assertJSONContains(t, data, `"ok":true`)
}

func TestMessagesAcceptJSONRPCOnUnmarshal(t *testing.T) {
	var req Request
	if err := json.Unmarshal([]byte(`{"jsonrpc":"2.0","id":"r1","method":"thread/start","params":{"prompt":"hi"}}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if req.JSONRPC != "2.0" || req.Method != "thread/start" {
		t.Fatalf("request = %+v", req)
	}

	var notif Notification
	if err := json.Unmarshal([]byte(`{"jsonrpc":"2.0","method":"initialized"}`), &notif); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notif.JSONRPC != "2.0" || notif.Method != "initialized" {
		t.Fatalf("notification = %+v", notif)
	}

	var resp Response
	if err := json.Unmarshal([]byte(`{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.JSONRPC != "2.0" || resp.ID.IsZero() {
		t.Fatalf("response = %+v", resp)
	}
}

func TestResponseRejectsResultAndError(t *testing.T) {
	var resp Response
	err := json.Unmarshal([]byte(`{"id":1,"result":{},"error":{"code":-32603,"message":"boom"}}`), &resp)
	if err == nil {
		t.Fatal("expected response with result and error to be rejected")
	}
}

func assertJSONContains(t *testing.T, data []byte, want string) {
	t.Helper()
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected JSON %s to contain %s", data, want)
	}
}

package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMethodUnavailableErrorForKnownMethod(t *testing.T) {
	err := MethodUnavailableError("thread/start")
	if err.Code != CodeMethodUnavailable {
		t.Fatalf("code = %d, want %d", err.Code, CodeMethodUnavailable)
	}
	var data UnavailableData
	if jsonErr := json.Unmarshal(err.Data, &data); jsonErr != nil {
		t.Fatalf("unmarshal data: %v", jsonErr)
	}
	if data.Method != "thread/start" || data.Surface != SurfaceClientRequest || data.Status != MethodBlocked {
		t.Fatalf("data = %+v", data)
	}
}

func TestMethodUnavailableErrorForUnknownMethod(t *testing.T) {
	err := MethodUnavailableError("not/a/method")
	if err.Code != CodeMethodNotFound {
		t.Fatalf("code = %d, want method-not-found", err.Code)
	}
	if len(err.Data) != 0 {
		t.Fatalf("unknown method should not have unavailable data: %s", err.Data)
	}
}

func TestUnavailableResponseOmitsJSONRPC(t *testing.T) {
	resp := UnavailableResponse(NewStringID("r1"), "thread/start")
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	assertJSONContains(t, data, `"id":"r1"`)
	assertJSONContains(t, data, `"code":-32004`)
	if got := string(data); strings.Contains(got, `"jsonrpc"`) {
		t.Fatalf("response should omit jsonrpc, got %s", got)
	}
}

func TestOverloadedError(t *testing.T) {
	err := OverloadedError("fs/writeFile", 1, 1, "")
	if err.Code != CodeOverloaded {
		t.Fatalf("code = %d, want %d", err.Code, CodeOverloaded)
	}
	if err.Message != "app-server overloaded" {
		t.Fatalf("message = %q", err.Message)
	}
	var data OverloadedData
	if jsonErr := json.Unmarshal(err.Data, &data); jsonErr != nil {
		t.Fatalf("unmarshal data: %v", jsonErr)
	}
	if data.Method != "fs/writeFile" || data.Limit != 1 || data.Pending != 1 || !data.Retryable || data.RetryAfterMillis == 0 {
		t.Fatalf("data = %+v", data)
	}
}

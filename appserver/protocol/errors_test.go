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

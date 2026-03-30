package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
)

func TestActionHandler_ApproveToolCall(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	approvedCh := waitForApprovalResult(t, bridge, "tc_1")

	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {ApprovalBridge: bridge},
		},
	})

	resp := doActionRequest(t, h, http.MethodPost, `{"type":"approve_tool_call","session_id":"ses_1","tool_call_id":"tc_1","message":"ok"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	select {
	case approved := <-approvedCh:
		if !approved {
			t.Fatal("expected approved=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval resolution")
	}

	var body successResponse
	decodeJSONBody(t, resp, &body)
	if !body.OK || body.Action != agui.ActionApproveToolCall || body.ToolCallID != "tc_1" || body.Message != "ok" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestActionHandler_DenyToolCall(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	approvedCh := waitForApprovalResult(t, bridge, "tc_2")

	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {ApprovalBridge: bridge},
		},
	})

	resp := doActionRequest(t, h, http.MethodPost, `{"type":"deny_tool_call","session_id":"ses_1","tool_call_id":"tc_2","message":"no"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	select {
	case approved := <-approvedCh:
		if approved {
			t.Fatal("expected approved=false")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for denial resolution")
	}

	var body successResponse
	decodeJSONBody(t, resp, &body)
	if body.Action != agui.ActionDenyToolCall || body.Message != "no" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestActionHandler_AbortSession(t *testing.T) {
	session := agui.NewSession(agui.SessionModeCoreRun)
	var cancelled atomic.Int32
	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {
				Session: session,
				Cancel: func() {
					cancelled.Add(1)
				},
			},
		},
	})

	resp := doActionRequest(t, h, http.MethodPost, `{"type":"abort_session","session_id":"ses_1"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if cancelled.Load() != 1 {
		t.Fatalf("cancel called %d times, want 1", cancelled.Load())
	}
	if got := session.GetStatus(); got != agui.SessionStatusAborted {
		t.Fatalf("session status = %q, want %q", got, agui.SessionStatusAborted)
	}

	var body successResponse
	decodeJSONBody(t, resp, &body)
	if body.Action != agui.ActionAbortSession || body.Message != "session aborted" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestActionHandler_ApproveViaSessionRegistry(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	approvedCh := waitForApprovalResult(t, bridge, "tc_reg")
	registry := NewMapSessionRegistry()
	registry.Set("ses_reg", &SessionRuntime{ApprovalBridge: bridge})

	h := NewActionHandler(ActionHandlerConfig{Sessions: registry})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"approve_tool_call","session_id":"ses_reg","tool_call_id":"tc_reg"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	select {
	case approved := <-approvedCh:
		if !approved {
			t.Fatal("expected approved=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval resolution")
	}
}

func TestActionHandler_AbortViaCancelStore(t *testing.T) {
	cancels := NewMapCancelStore()
	var cancelled atomic.Int32
	cancels.SetCancel("ses_cancel", func() { cancelled.Add(1) })

	h := NewActionHandler(ActionHandlerConfig{Cancels: cancels})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"abort_session","session_id":"ses_cancel"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if cancelled.Load() != 1 {
		t.Fatalf("cancel called %d times, want 1", cancelled.Load())
	}
}

func TestActionHandler_BadJSON(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":`)
	assertErrorResponse(t, resp, http.StatusBadRequest, "invalid request body")
}

func TestActionHandler_RejectsTrailingJSON(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"abort_session","session_id":"ses_1"} {}`)
	assertErrorResponse(t, resp, http.StatusBadRequest, "multiple JSON values")
}

func TestActionHandler_RejectsWrongMethod(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	resp := doActionRequest(t, h, http.MethodGet, "")
	assertErrorResponse(t, resp, http.StatusMethodNotAllowed, "method not allowed")
}

func TestActionHandler_UnsupportedAction(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"resume_session","session_id":"ses_1"}`)
	assertErrorResponse(t, resp, http.StatusBadRequest, "unsupported action type")
}

func TestActionHandler_MissingIDs(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})

	resp := doActionRequest(t, h, http.MethodPost, `{"type":"abort_session"}`)
	assertErrorResponse(t, resp, http.StatusBadRequest, "session_id is required")

	resp = doActionRequest(t, h, http.MethodPost, `{"type":"approve_tool_call","session_id":"ses_1"}`)
	assertErrorResponse(t, resp, http.StatusBadRequest, "tool_call_id is required")
}

func TestActionHandler_MissingType(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	resp := doActionRequest(t, h, http.MethodPost, `{"session_id":"ses_1"}`)
	assertErrorResponse(t, resp, http.StatusBadRequest, "type is required")
}

func TestActionHandler_UnknownSession(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{Runtimes: map[string]*SessionRuntime{}})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"abort_session","session_id":"missing"}`)
	assertErrorResponse(t, resp, http.StatusNotFound, "unknown session: missing")
}

func TestActionHandler_UnknownToolCall(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {ApprovalBridge: bridge},
		},
	})

	resp := doActionRequest(t, h, http.MethodPost, `{"type":"approve_tool_call","session_id":"ses_1","tool_call_id":"missing"}`)
	assertErrorResponse(t, resp, http.StatusNotFound, "unknown tool call: missing")
}

func TestActionHandler_SharedApprovalBridgeDoesNotProveSessionExists(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	h := NewActionHandler(ActionHandlerConfig{ApprovalBridge: bridge})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"approve_tool_call","session_id":"ses_missing","tool_call_id":"tc_1"}`)
	assertErrorResponse(t, resp, http.StatusNotFound, "unknown session: ses_missing")
}

func TestActionHandler_AbortWithoutCancelReturnsNotFound(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {Session: agui.NewSession(agui.SessionModeCoreRun)},
		},
	})
	resp := doActionRequest(t, h, http.MethodPost, `{"type":"abort_session","session_id":"ses_1"}`)
	assertErrorResponse(t, resp, http.StatusNotFound, "unknown session: ses_1")
}

func waitForApprovalResult(t *testing.T, bridge *agui.ApprovalBridge, toolCallID string) <-chan bool {
	t.Helper()
	approvedCh := make(chan bool, 1)
	go func() {
		ctx := core.ContextWithToolCallID(context.Background(), toolCallID)
		approved, err := bridge.ToolApprovalFunc()(ctx, "search", `{}`)
		if err != nil {
			t.Errorf("approval wait returned error: %v", err)
			return
		}
		approvedCh <- approved
	}()
	eventuallyAction(t, func() bool { return bridge.PendingCount() == 1 })
	return approvedCh
}

func doActionRequest(t *testing.T, h http.Handler, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/action", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	return rec
}

func decodeJSONBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("unmarshal body %q: %v", rec.Body.String(), err)
	}
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMessage string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var body errorResponse
	decodeJSONBody(t, rec, &body)
	if body.OK {
		t.Fatalf("expected ok=false, got %+v", body)
	}
	if !strings.Contains(body.Error, wantMessage) {
		t.Fatalf("error = %q, want substring %q", body.Error, wantMessage)
	}
}

func eventuallyAction(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

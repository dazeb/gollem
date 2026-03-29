package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
)

func TestActionHandler_ApproveToolCall(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	approvedCh := make(chan bool, 1)
	go func() {
		ctx := core.ContextWithToolCallID(context.Background(), "tc_1")
		approved, err := bridge.ToolApprovalFunc()(ctx, "search", `{}`)
		if err != nil {
			t.Errorf("approval wait returned error: %v", err)
			return
		}
		approvedCh <- approved
	}()
	time.Sleep(10 * time.Millisecond)

	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {ApprovalBridge: bridge},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"approve_tool_call","session_id":"ses_1","tool_call_id":"tc_1","message":"ok"}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case approved := <-approvedCh:
		if !approved {
			t.Fatal("expected approved=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval resolution")
	}

	var resp successResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK || resp.ToolCallID != "tc_1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestActionHandler_DenyToolCall(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	approvedCh := make(chan bool, 1)
	go func() {
		ctx := core.ContextWithToolCallID(context.Background(), "tc_2")
		approved, err := bridge.ToolApprovalFunc()(ctx, "search", `{}`)
		if err != nil {
			t.Errorf("approval wait returned error: %v", err)
			return
		}
		approvedCh <- approved
	}()
	time.Sleep(10 * time.Millisecond)

	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {ApprovalBridge: bridge},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"deny_tool_call","session_id":"ses_1","tool_call_id":"tc_2","message":"no"}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case approved := <-approvedCh:
		if approved {
			t.Fatal("expected approved=false")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for denial resolution")
	}
}

func TestActionHandler_AbortSession(t *testing.T) {
	session := agui.NewSession(agui.SessionModeCoreRun)
	cancelled := false
	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {
				Session: session,
				Cancel: func() {
					cancelled = true
				},
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"abort_session","session_id":"ses_1"}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !cancelled {
		t.Fatal("expected cancel function to be called")
	}
	if got := session.GetStatus(); got != agui.SessionStatusAborted {
		t.Fatalf("session status = %q, want %q", got, agui.SessionStatusAborted)
	}
}

func TestActionHandler_BadJSON(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestActionHandler_RejectsTrailingJSON(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"abort_session","session_id":"ses_1"} {}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestActionHandler_UnsupportedAction(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"resume_session","session_id":"ses_1"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestActionHandler_MissingIDs(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"abort_session"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing session_id, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"approve_tool_call","session_id":"ses_1"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing tool_call_id, got %d", rec.Code)
	}
}

func TestActionHandler_UnknownSession(t *testing.T) {
	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"abort_session","session_id":"missing"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestActionHandler_UnknownToolCall(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	h := NewActionHandler(ActionHandlerConfig{
		Runtimes: map[string]*SessionRuntime{
			"ses_1": {ApprovalBridge: bridge},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"approve_tool_call","session_id":"ses_1","tool_call_id":"missing"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestActionHandler_SharedApprovalBridgeDoesNotProveSessionExists(t *testing.T) {
	bridge := agui.NewApprovalBridge()
	h := NewActionHandler(ActionHandlerConfig{ApprovalBridge: bridge})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"type":"approve_tool_call","session_id":"ses_missing","tool_call_id":"tc_1"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

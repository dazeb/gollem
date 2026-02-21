package chi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chihandler "github.com/fugue-labs/gollem/contrib/chi"
	"github.com/fugue-labs/gollem/core"
)

type mockRunner struct {
	response string
	err      error
}

func (m *mockRunner) Run(_ context.Context, prompt string) (*core.RunResult[string], error) {
	if m.err != nil {
		return nil, m.err
	}
	return &core.RunResult[string]{
		Output: m.response,
		Usage: core.RunUsage{
			Usage: core.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}, nil
}

func (m *mockRunner) RunStream(_ context.Context, prompt string) (*core.StreamResult[string], error) {
	return nil, errors.New("streaming not implemented in mock")
}

func TestHandler_Success(t *testing.T) {
	runner := &mockRunner{response: "Hello, world!"}
	handler := chihandler.Handler(runner)

	body, _ := json.Marshal(chihandler.Request{Prompt: "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp chihandler.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Response != "Hello, world!" {
		t.Fatalf("expected 'Hello, world!', got %q", resp.Response)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestHandler_EmptyPrompt(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	handler := chihandler.Handler(runner)

	body, _ := json.Marshal(chihandler.Request{Prompt: ""})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	handler := chihandler.Handler(runner)

	req := httptest.NewRequest(http.MethodPost, "/agent", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	handler := chihandler.Handler(runner)

	req := httptest.NewRequest(http.MethodGet, "/agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandler_AgentError(t *testing.T) {
	runner := &mockRunner{err: errors.New("model failed")}
	handler := chihandler.Handler(runner)

	body, _ := json.Marshal(chihandler.Request{Prompt: "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

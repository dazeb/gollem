package echohandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/contrib/echohandler"
	"github.com/fugue-labs/gollem/core"
	"github.com/labstack/echo/v4"
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
	e := echo.New()

	body, _ := json.Marshal(echohandler.Request{Prompt: "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	handler := echohandler.Handler(runner)
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp echohandler.Response
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
	e := echo.New()

	body, _ := json.Marshal(echohandler.Request{Prompt: ""})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	handler := echohandler.Handler(runner)
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/agent", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	handler := echohandler.Handler(runner)
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_AgentError(t *testing.T) {
	runner := &mockRunner{err: errors.New("model failed")}
	e := echo.New()

	body, _ := json.Marshal(echohandler.Request{Prompt: "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	handler := echohandler.Handler(runner)
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

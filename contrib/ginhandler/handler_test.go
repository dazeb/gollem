package ginhandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/contrib/ginhandler"
	"github.com/fugue-labs/gollem/core"
	"github.com/gin-gonic/gin"
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

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(runner ginhandler.AgentRunner) *gin.Engine {
	r := gin.New()
	r.POST("/agent", ginhandler.Handler(runner))
	return r
}

func TestHandler_Success(t *testing.T) {
	runner := &mockRunner{response: "Hello, world!"}
	router := setupRouter(runner)

	body, _ := json.Marshal(ginhandler.Request{Prompt: "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ginhandler.Response
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
	router := setupRouter(runner)

	body, _ := json.Marshal(ginhandler.Request{Prompt: ""})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	router := setupRouter(runner)

	req := httptest.NewRequest(http.MethodPost, "/agent", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_AgentError(t *testing.T) {
	runner := &mockRunner{err: errors.New("model failed")}
	router := setupRouter(runner)

	body, _ := json.Marshal(ginhandler.Request{Prompt: "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

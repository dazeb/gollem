package fiberhandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/contrib/fiberhandler"
	"github.com/fugue-labs/gollem/core"
	"github.com/gofiber/fiber/v2"
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

func setupApp(runner fiberhandler.AgentRunner) *fiber.App {
	app := fiber.New()
	app.Post("/agent", fiberhandler.Handler(runner))
	return app
}

func TestHandler_Success(t *testing.T) {
	runner := &mockRunner{response: "Hello, world!"}
	app := setupApp(runner)

	body, _ := json.Marshal(fiberhandler.Request{Prompt: "say hi"})
	req, _ := http.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var result fiberhandler.Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Response != "Hello, world!" {
		t.Fatalf("expected 'Hello, world!', got %q", result.Response)
	}
	if result.Usage == nil || result.Usage.InputTokens != 10 {
		t.Fatalf("unexpected usage: %+v", result.Usage)
	}
}

func TestHandler_EmptyPrompt(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	app := setupApp(runner)

	body, _ := json.Marshal(fiberhandler.Request{Prompt: ""})
	req, _ := http.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	app := setupApp(runner)

	req, _ := http.NewRequest(http.MethodPost, "/agent", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_AgentError(t *testing.T) {
	runner := &mockRunner{err: errors.New("model failed")}
	app := setupApp(runner)

	body, _ := json.Marshal(fiberhandler.Request{Prompt: "say hi"})
	req, _ := http.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

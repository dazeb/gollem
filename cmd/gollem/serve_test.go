package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
	"github.com/fugue-labs/gollem/modelutil"
	"github.com/fugue-labs/gollem/pkg/ui"
)

func TestParseServeFlags(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		got, err := parseServeFlags(nil)
		if err != nil {
			t.Fatalf("parseServeFlags() error = %v", err)
		}
		if got.port != defaultServePort {
			t.Fatalf("port = %d, want %d", got.port, defaultServePort)
		}
		if got.tools {
			t.Fatal("tools = true, want false")
		}
		if got.openBrowser {
			t.Fatal("openBrowser = true, want false")
		}
	})

	t.Run("explicit flags", func(t *testing.T) {
		got, err := parseServeFlags([]string{
			"--port", "9090",
			"--provider", "openai",
			"--model", "gpt-5.3",
			"--tools",
			"--open=false",
		})
		if err != nil {
			t.Fatalf("parseServeFlags() error = %v", err)
		}
		if got.port != 9090 {
			t.Fatalf("port = %d, want 9090", got.port)
		}
		if got.provider != "openai" {
			t.Fatalf("provider = %q, want openai", got.provider)
		}
		if got.modelName != "gpt-5.3" {
			t.Fatalf("modelName = %q, want gpt-5.3", got.modelName)
		}
		if !got.tools {
			t.Fatal("tools = false, want true")
		}
		if got.openBrowser {
			t.Fatal("openBrowser = true, want false")
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		_, err := parseServeFlags([]string{"--port", "0"})
		if err == nil || !strings.Contains(err.Error(), "invalid --port") {
			t.Fatalf("expected invalid port error, got %v", err)
		}
	})
}

func TestApplyRunStartDefaultsJSONOverwritesProviderAndModel(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/runs/start", strings.NewReader(`{"prompt":"hello","provider":"wrong","model":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")

	updated, err := applyRunStartDefaults(req, ui.RunStartRequest{Provider: "anthropic", Model: "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("applyRunStartDefaults() error = %v", err)
	}

	var got ui.RunStartRequest
	if err := json.NewDecoder(updated.Body).Decode(&got); err != nil {
		t.Fatalf("decode updated body: %v", err)
	}
	if got.Provider != "anthropic" {
		t.Fatalf("provider = %q, want anthropic", got.Provider)
	}
	if got.Model != "claude-opus-4-6" {
		t.Fatalf("model = %q, want claude-opus-4-6", got.Model)
	}
	if got.Prompt != "hello" {
		t.Fatalf("prompt = %q, want hello", got.Prompt)
	}
}

func TestApplyRunStartDefaultsFormOverwritesProviderAndModel(t *testing.T) {
	form := url.Values{
		"prompt":   {"hello"},
		"provider": {"wrong"},
		"model":    {"wrong"},
	}
	req := httptest.NewRequest(http.MethodPost, "/runs/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	updated, err := applyRunStartDefaults(req, ui.RunStartRequest{Provider: "openai", Model: "gpt-5.3"})
	if err != nil {
		t.Fatalf("applyRunStartDefaults() error = %v", err)
	}
	if err := updated.ParseForm(); err != nil {
		t.Fatalf("updated.ParseForm() error = %v", err)
	}
	if got := updated.FormValue("provider"); got != "openai" {
		t.Fatalf("provider = %q, want openai", got)
	}
	if got := updated.FormValue("model"); got != "gpt-5.3" {
		t.Fatalf("model = %q, want gpt-5.3", got)
	}
	if got := updated.FormValue("prompt"); got != "hello" {
		t.Fatalf("prompt = %q, want hello", got)
	}
}

func TestNewServeRunStarterClosesPerRunModels(t *testing.T) {
	base := &serveCountingModel{response: core.TextResponse("done")}
	cfg := serveRunConfig{
		provider:  "openai",
		modelName: "serve-test",
		timeout:   time.Minute,
		model:     modelutil.NewRetryModel(base, modelutil.DefaultRetryConfig()),
	}

	runtime := &ui.RunRuntime{
		RunID:          "run-1",
		EventBus:       core.NewEventBus(),
		Session:        agui.NewSession(agui.SessionModeCoreStream),
		ApprovalBridge: agui.NewApprovalBridge(),
		Adapter:        agui.NewAdapter("run-1"),
	}
	defer runtime.Adapter.Close()

	starter := newServeRunStarter(cfg)
	if err := starter.StartRun(context.Background(), runtime, ui.RunStartRequest{Prompt: "hello"}); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	if got := base.newSessionCalls.Load(); got != 2 {
		t.Fatalf("newSessionCalls = %d, want 2", got)
	}
	if got := base.closeCalls.Load(); got != 2 {
		t.Fatalf("closeCalls = %d, want 2", got)
	}
}

type serveCountingModel struct {
	response        *core.ModelResponse
	newSessionCalls atomic.Int32
	closeCalls      atomic.Int32
}

func (m *serveCountingModel) Request(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return nil, errors.New("unexpected Request call")
}

func (m *serveCountingModel) RequestStream(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("unexpected RequestStream call")
}

func (m *serveCountingModel) ModelName() string { return "serve-counting-model" }

func (m *serveCountingModel) NewSession() core.Model {
	m.newSessionCalls.Add(1)
	return &serveCountingSession{
		response:        m.response,
		newSessionCalls: &m.newSessionCalls,
		closeCalls:      &m.closeCalls,
	}
}

type serveCountingSession struct {
	response        *core.ModelResponse
	newSessionCalls *atomic.Int32
	closeCalls      *atomic.Int32
}

func (m *serveCountingSession) Request(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return m.response, nil
}

func (m *serveCountingSession) RequestStream(_ context.Context, _ []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return &serveStream{response: m.response}, nil
}

func (m *serveCountingSession) ModelName() string { return "serve-counting-session" }

func (m *serveCountingSession) NewSession() core.Model {
	m.newSessionCalls.Add(1)
	return &serveCountingSession{
		response:        m.response,
		newSessionCalls: m.newSessionCalls,
		closeCalls:      m.closeCalls,
	}
}

func (m *serveCountingSession) Close() error {
	m.closeCalls.Add(1)
	return nil
}

type serveStream struct {
	response *core.ModelResponse
	stage    int
}

func (s *serveStream) Next() (core.ModelResponseStreamEvent, error) {
	switch s.stage {
	case 0:
		s.stage = 1
		return core.PartStartEvent{Index: 0, Part: core.TextPart{Content: s.response.TextContent()}}, nil
	case 1:
		s.stage = 2
		return core.PartEndEvent{Index: 0}, nil
	default:
		return nil, io.EOF
	}
}

func (s *serveStream) Response() *core.ModelResponse { return s.response }
func (s *serveStream) Usage() core.Usage             { return s.response.Usage }
func (s *serveStream) Close() error                  { return nil }

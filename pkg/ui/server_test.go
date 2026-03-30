package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/agui"
)

func TestHandleStartRunCreatesRunAndRedirects(t *testing.T) {
	store := NewRunStateStore()
	started := make(chan string, 1)
	server := MustNewServer(
		WithRunStore(store),
		WithRunStarter(RunStarterFunc(func(_ context.Context, runtime *RunRuntime, req RunStartRequest) error {
			started <- runtime.RunID + ":" + req.Prompt
			core.Publish(runtime.EventBus, core.RunStartedEvent{
				RunID:     runtime.RunID,
				Prompt:    req.Prompt,
				StartedAt: time.Now(),
			})
			return nil
		})),
	)

	form := url.Values{
		"title":  {"Test run"},
		"prompt": {"hello world"},
	}
	req := httptest.NewRequest(http.MethodPost, "/runs/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/runs/") {
		t.Fatalf("location = %q, want /runs/<id>", location)
	}
	runID := strings.TrimPrefix(location, "/runs/")
	if runID == "" {
		t.Fatal("expected non-empty run id")
	}
	if _, ok := store.get(runID); !ok {
		t.Fatalf("expected run %q in store", runID)
	}

	select {
	case got := <-started:
		if got != runID+":hello world" {
			t.Fatalf("starter payload = %q, want %q", got, runID+":hello world")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async starter")
	}
}

func TestSidebarRendersLiveStatusUsageAndApprovals(t *testing.T) {
	store := NewRunStateStore()
	run := store.create(RunStartRequest{Prompt: "summarize", Provider: "test", Model: "gpt-test"})
	now := time.Now()

	core.Publish(run.EventBus(), core.RunStartedEvent{RunID: run.ID(), Prompt: "summarize", StartedAt: now})
	core.Publish(run.EventBus(), core.ModelResponseCompletedEvent{RunID: run.ID(), InputTokens: 5, OutputTokens: 2, CompletedAt: now.Add(time.Second)})
	core.Publish(run.EventBus(), core.ToolCalledEvent{RunID: run.ID(), ToolCallID: "tool_1", ToolName: "danger", CalledAt: now.Add(2 * time.Second)})
	core.Publish(run.EventBus(), core.ApprovalRequestedEvent{RunID: run.ID(), ToolCallID: "tool_1", ToolName: "danger", ArgsJSON: `{"x":1}`, RequestedAt: now.Add(3 * time.Second)})

	server := MustNewServer(WithRunStore(store))
	req := httptest.NewRequest(http.MethodGet, "/runs/"+run.ID()+"/sidebar", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "ui:fragment-loaded" {
		t.Fatalf("HX-Trigger = %q, want %q", got, "ui:fragment-loaded")
	}
	body := rec.Body.String()
	for _, want := range []string{"waiting", "danger", "tool_1", ">5<", ">2<", ">1<"} {
		if !strings.Contains(body, want) {
			t.Fatalf("sidebar body missing %q:\n%s", want, body)
		}
	}
}

func TestHandleRunRendersSpatialRendererScaffold(t *testing.T) {
	store := NewRunStateStore()
	run := store.create(RunStartRequest{
		Title:    "Renderer run",
		Summary:  "stream into the canvas",
		Prompt:   "show live output",
		Provider: "test",
		Model:    "gpt-test",
	})
	server := MustNewServer(WithRunStore(store))

	req := httptest.NewRequest(http.MethodGet, "/runs/"+run.ID(), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Baseline spatial renderer",
		"data-run-scene",
		"data-run-canvas",
		"data-run-event-log",
		"/runs/" + run.ID() + "/events",
		"Renderer run",
		"stream into the canvas",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("run page missing %q:\n%s", want, body)
		}
	}
}

func TestHandleActionAbortUsesRunSpecificSessionWithoutMutatingStatus(t *testing.T) {
	store := NewRunStateStore()
	run := store.create(RunStartRequest{Prompt: "abort me"})
	core.Publish(run.EventBus(), core.RunStartedEvent{RunID: run.ID(), Prompt: "abort me", StartedAt: time.Now()})

	cancelled := false
	run.setCancel(func() { cancelled = true })
	server := MustNewServer(WithRunStore(store))

	req := httptest.NewRequest(http.MethodPost, "/runs/"+run.ID()+"/action", strings.NewReader(`{"type":"abort_session","session_id":"wrong-session"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !cancelled {
		t.Fatal("expected cancel func to be called")
	}
	if run.Snapshot().Status != "running" {
		t.Fatalf("status = %q, want running before runtime completion event", run.Snapshot().Status)
	}
	if run.Session().GetStatus() != agui.SessionStatusAborted {
		t.Fatalf("session status = %q, want %q", run.Session().GetStatus(), agui.SessionStatusAborted)
	}
	if !strings.Contains(rec.Body.String(), run.Session().ID) {
		t.Fatalf("response body %q should mention run session %q", rec.Body.String(), run.Session().ID)
	}
}

func TestRunCompletedDeferredPreservesExistingWaitingReason(t *testing.T) {
	store := NewRunStateStore()
	run := store.create(RunStartRequest{Prompt: "wait"})
	now := time.Now()

	core.Publish(run.EventBus(), core.RunWaitingEvent{RunID: run.ID(), Reason: "approval_and_deferred", WaitingAt: now})
	core.Publish(run.EventBus(), core.RunCompletedEvent{RunID: run.ID(), Deferred: true, CompletedAt: now.Add(time.Second)})

	snap := run.Snapshot()
	if snap.Status != "waiting" {
		t.Fatalf("status = %q, want waiting", snap.Status)
	}
	if snap.WaitingReason != "approval_and_deferred" {
		t.Fatalf("waiting reason = %q, want approval_and_deferred", snap.WaitingReason)
	}
}

func TestRunStateStoreCreateRejectsDuplicateIDs(t *testing.T) {
	store := NewRunStateStore()
	calls := 0
	store.nextID = func() string {
		calls++
		switch calls {
		case 1, 2:
			return "dup"
		default:
			return fmt.Sprintf("dup-%d", calls)
		}
	}

	first := store.create(RunStartRequest{Prompt: "first"})
	second := store.create(RunStartRequest{Prompt: "second"})

	if first.ID() != "dup" {
		t.Fatalf("first id = %q, want dup", first.ID())
	}
	if second.ID() != "dup-3" {
		t.Fatalf("second id = %q, want dup-3", second.ID())
	}
	if first.ID() == second.ID() {
		t.Fatal("expected unique run ids")
	}
}

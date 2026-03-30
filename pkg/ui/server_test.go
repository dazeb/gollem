package ui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
				StartedAt: time.Now().UTC(),
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

func TestServerServeRoutesAssetsSSEAndApproveFlow(t *testing.T) {
	store := NewRunStateStore()
	server := MustNewServer(
		WithRunStore(store),
		WithRunStarter(RunStarterFunc(func(ctx context.Context, runtime *RunRuntime, req RunStartRequest) error {
			now := time.Now().UTC()
			core.Publish(runtime.EventBus, core.RunStartedEvent{RunID: runtime.RunID, Prompt: req.Prompt, StartedAt: now})
			core.Publish(runtime.EventBus, core.TurnStartedEvent{RunID: runtime.RunID, TurnNumber: 1, StartedAt: now.Add(10 * time.Millisecond)})
			runtime.Adapter.EmitTextDelta("msg_before_approval", "stream before approval")
			core.Publish(runtime.EventBus, core.ModelResponseCompletedEvent{RunID: runtime.RunID, InputTokens: 11, OutputTokens: 5, CompletedAt: now.Add(20 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.ToolCalledEvent{RunID: runtime.RunID, ToolCallID: "tool_approve", ToolName: "dangerous_write", ArgsJSON: `{"path":"/tmp/out.txt"}`, CalledAt: now.Add(30 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.ApprovalRequestedEvent{RunID: runtime.RunID, ToolCallID: "tool_approve", ToolName: "dangerous_write", ArgsJSON: `{"path":"/tmp/out.txt"}`, RequestedAt: now.Add(40 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.RunWaitingEvent{RunID: runtime.RunID, Reason: "approval", WaitingAt: now.Add(50 * time.Millisecond)})

			approved, err := runtime.ApprovalBridge.ToolApprovalFunc()(core.ContextWithToolCallID(ctx, "tool_approve"), "dangerous_write", `{"path":"/tmp/out.txt"}`)
			if err != nil {
				return err
			}

			resolvedAt := time.Now().UTC()
			core.Publish(runtime.EventBus, core.RunResumedEvent{RunID: runtime.RunID, ResumedAt: resolvedAt})
			core.Publish(runtime.EventBus, core.ApprovalResolvedEvent{RunID: runtime.RunID, ToolCallID: "tool_approve", ToolName: "dangerous_write", Approved: approved, ResolvedAt: resolvedAt})
			if !approved {
				core.Publish(runtime.EventBus, core.ToolFailedEvent{RunID: runtime.RunID, ToolCallID: "tool_approve", ToolName: "dangerous_write", Error: "denied by user", FailedAt: resolvedAt})
				core.Publish(runtime.EventBus, core.TurnCompletedEvent{RunID: runtime.RunID, TurnNumber: 1, CompletedAt: resolvedAt.Add(10 * time.Millisecond)})
				core.Publish(runtime.EventBus, core.RunCompletedEvent{RunID: runtime.RunID, Error: "denied by user", CompletedAt: resolvedAt.Add(20 * time.Millisecond)})
				return nil
			}

			core.Publish(runtime.EventBus, core.ToolCompletedEvent{RunID: runtime.RunID, ToolCallID: "tool_approve", ToolName: "dangerous_write", Result: "ok", CompletedAt: resolvedAt.Add(10 * time.Millisecond)})
			runtime.Adapter.EmitTextDelta("msg_after_approval", "stream after approval")
			core.Publish(runtime.EventBus, core.TurnCompletedEvent{RunID: runtime.RunID, TurnNumber: 1, HasText: true, HasToolCalls: true, CompletedAt: resolvedAt.Add(20 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.RunCompletedEvent{RunID: runtime.RunID, Success: true, StartedAt: now, CompletedAt: resolvedAt.Add(30 * time.Millisecond)})
			return nil
		})),
	)

	ts := httptest.NewServer(server)
	defer ts.Close()
	client := ts.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	startForm := url.Values{
		"title":    {"Approve flow"},
		"summary":  {"exercise live dashboard flow"},
		"prompt":   {"please verify the UI"},
		"provider": {"test"},
		"model":    {"serve-test"},
	}
	startResp := mustPOSTForm(t, client, ts.URL+"/runs/start", startForm)
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("start status = %d, want %d", startResp.StatusCode, http.StatusSeeOther)
	}
	runID := strings.TrimPrefix(startResp.Header.Get("Location"), "/runs/")
	if runID == "" {
		t.Fatalf("redirect location = %q, want /runs/<id>", startResp.Header.Get("Location"))
	}

	run := waitForRun(t, store, runID)
	waitForRunStatus(t, store, runID, "waiting")
	waitForPendingApprovals(t, run, 1)

	assertHTMLContains(t, mustGETBody(t, client, ts.URL+"/"),
		"Dashboard",
		"Approve flow",
		"/runs/"+runID,
		"test / serve-test",
	)
	assertHTMLContains(t, mustGETBody(t, client, ts.URL+"/runs/"+runID),
		"Approve flow",
		"exercise live dashboard flow",
		"data-run-scene",
		"data-run-canvas",
		"data-run-event-log",
		"1 pending tool approval",
		"Run started",
		"/runs/"+runID+"/events",
		"data-run-status=\"waiting\"",
		"data-run-status-label=\"Waiting\"",
		"data-run-waiting-reason=\"approval\"",
		"data-run-waiting-label=\"Waiting for approval\"",
		"data-run-last-event-label=\"Waiting for approval\"",
		"data-run-last-event-summary=\"Waiting for approval · 1 pending tool approval.\"",
	)
	sidebarBody := mustGETBody(t, client, ts.URL+"/runs/"+runID+"/sidebar")
	assertHTMLContains(t, sidebarBody,
		"waiting",
		"dangerous_write",
		"tool_approve",
		"Review approvals",
		"Waiting for approval",
		"data-run-status=\"waiting\"",
		"data-run-waiting-reason=\"approval\"",
		"data-run-last-event-label=\"Waiting for approval\"",
		">11<",
		">5<",
		">1<",
	)
	sidebarResp := mustGET(t, client, ts.URL+"/runs/"+runID+"/sidebar")
	hxTrigger := sidebarResp.Header.Get("HX-Trigger")
	sidebarResp.Body.Close()
	assertStringContains(t, hxTrigger,
		"\"ui:fragment-loaded\"",
		"\"runId\":\""+runID+"\"",
		"\"scene\"",
		"\"status\":{",
		"\"code\":\"waiting\"",
		"\"detail\":\"1 pending tool approval.\"",
		"\"isTerminal\":false",
		"\"isWaiting\":true",
		"\"label\":\"Waiting\"",
		"\"tone\":\"approval\"",
		"\"waiting\":{",
		"\"reason\":\"approval\"",
		"\"label\":\"Waiting for approval\"",
		"\"summary\":\"Waiting for approval · 1 pending tool approval.\"",
		"\"lastEvent\":{",
		"\"label\":\"Waiting for approval\"",
		"\"summary\":\"Waiting for approval · 1 pending tool approval.\"",
		"\"pendingApprovals\":{",
	)

	assetReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/static/style.css", nil)
	if err != nil {
		t.Fatalf("new static asset request: %v", err)
	}
	assetResp, err := client.Do(assetReq)
	if err != nil {
		t.Fatalf("GET static asset: %v", err)
	}
	assetBody, err := io.ReadAll(assetResp.Body)
	assetResp.Body.Close()
	if err != nil {
		t.Fatalf("read static asset: %v", err)
	}
	if assetResp.StatusCode != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", assetResp.StatusCode, http.StatusOK)
	}
	if got := assetResp.Header.Get("Content-Type"); !strings.Contains(got, "text/css") {
		t.Fatalf("asset content type = %q, want text/css", got)
	}
	if len(assetBody) == 0 {
		t.Fatal("expected embedded asset body")
	}

	eventResp := mustOpenUISSE(t, client, ts.URL+"/runs/"+runID+"/events")
	defer eventResp.Body.Close()
	reader := newUISSEStreamReader(t, eventResp.Body)
	preApprovalFrames := readUISSEFrames(t, reader, 10)
	assertUISSEFrameTypes(t, preApprovalFrames,
		agui.AGUIRunStarted,
		agui.AGUIStepStarted,
		agui.AGUITextMessageStart,
		agui.AGUITextMessageContent,
		agui.AGUITextMessageEnd,
		agui.AGUIToolCallStart,
		agui.AGUIToolCallArgs,
		agui.AGUIToolCallEnd,
		agui.AGUICustom,
	)
	assertUISSEFrameContains(t, preApprovalFrames, agui.AGUITextMessageContent, "delta", "stream before approval")
	assertUISSECustomEvent(t, preApprovalFrames, "gollem.approval.requested")
	assertUISSECustomEvent(t, preApprovalFrames, "gollem.run.waiting")

	actionResp := mustPOSTJSON(t, client, ts.URL+"/runs/"+runID+"/action", `{"type":"approve_tool_call","session_id":"wrong-session","tool_call_id":"tool_approve","message":"ship it"}`)
	defer actionResp.Body.Close()
	if actionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(actionResp.Body)
		t.Fatalf("approve status = %d, body=%s", actionResp.StatusCode, string(body))
	}
	var actionBody struct {
		OK        bool   `json:"ok"`
		Action    string `json:"action"`
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(actionResp.Body).Decode(&actionBody); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	if !actionBody.OK || actionBody.Action != agui.ActionApproveToolCall {
		t.Fatalf("unexpected approve response: %+v", actionBody)
	}
	if actionBody.SessionID != run.Session().ID {
		t.Fatalf("approve session_id = %q, want %q", actionBody.SessionID, run.Session().ID)
	}
	if actionBody.Message != "ship it" {
		t.Fatalf("approve message = %q, want ship it", actionBody.Message)
	}

	waitForRunStatus(t, store, runID, "completed")
	postApprovalFrames := readUISSEFrames(t, reader, 8)
	assertUISSEFrameTypes(t, postApprovalFrames,
		agui.AGUICustom,
		agui.AGUIToolCallResult,
		agui.AGUITextMessageStart,
		agui.AGUITextMessageContent,
		agui.AGUITextMessageEnd,
		agui.AGUIStepFinished,
		agui.AGUIRunFinished,
	)
	assertUISSECustomEvent(t, postApprovalFrames, "gollem.run.resumed")
	assertUISSECustomEvent(t, postApprovalFrames, "gollem.approval.resolved")
	assertUISSEFrameContains(t, postApprovalFrames, agui.AGUITextMessageContent, "delta", "stream after approval")
	assertHTMLContains(t, mustGETBody(t, client, ts.URL+"/runs/"+runID+"/sidebar"),
		"completed",
		"Review activity",
		"Run completed",
		"data-run-status=\"completed\"",
		"data-run-last-event-label=\"Run completed\"",
		"No pending approvals.",
		">11<",
		">5<",
		">1<",
	)
}

func TestHandleActionDenyFlow(t *testing.T) {
	store := NewRunStateStore()
	server := MustNewServer(
		WithRunStore(store),
		WithRunStarter(RunStarterFunc(func(ctx context.Context, runtime *RunRuntime, req RunStartRequest) error {
			now := time.Now().UTC()
			core.Publish(runtime.EventBus, core.RunStartedEvent{RunID: runtime.RunID, Prompt: req.Prompt, StartedAt: now})
			core.Publish(runtime.EventBus, core.ToolCalledEvent{RunID: runtime.RunID, ToolCallID: "tool_deny", ToolName: "rm", ArgsJSON: `{"path":"/tmp/nope"}`, CalledAt: now.Add(10 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.ApprovalRequestedEvent{RunID: runtime.RunID, ToolCallID: "tool_deny", ToolName: "rm", ArgsJSON: `{"path":"/tmp/nope"}`, RequestedAt: now.Add(20 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.RunWaitingEvent{RunID: runtime.RunID, Reason: "approval", WaitingAt: now.Add(30 * time.Millisecond)})

			approved, err := runtime.ApprovalBridge.ToolApprovalFunc()(core.ContextWithToolCallID(ctx, "tool_deny"), "rm", `{"path":"/tmp/nope"}`)
			if err != nil {
				return err
			}

			resolvedAt := time.Now().UTC()
			core.Publish(runtime.EventBus, core.RunResumedEvent{RunID: runtime.RunID, ResumedAt: resolvedAt})
			core.Publish(runtime.EventBus, core.ApprovalResolvedEvent{RunID: runtime.RunID, ToolCallID: "tool_deny", ToolName: "rm", Approved: approved, ResolvedAt: resolvedAt})
			if approved {
				return fmt.Errorf("expected deny flow")
			}
			core.Publish(runtime.EventBus, core.ToolFailedEvent{RunID: runtime.RunID, ToolCallID: "tool_deny", ToolName: "rm", Error: "denied by user", FailedAt: resolvedAt.Add(10 * time.Millisecond)})
			core.Publish(runtime.EventBus, core.RunCompletedEvent{RunID: runtime.RunID, Error: "denied by user", CompletedAt: resolvedAt.Add(20 * time.Millisecond)})
			return nil
		})),
	)

	ts := httptest.NewServer(server)
	defer ts.Close()
	client := ts.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	startResp := mustPOSTJSON(t, client, ts.URL+"/runs/start", `{"title":"Deny flow","prompt":"deny the tool"}`)
	defer startResp.Body.Close()
	runID := strings.TrimPrefix(startResp.Header.Get("Location"), "/runs/")
	if runID == "" {
		t.Fatalf("redirect location = %q, want /runs/<id>", startResp.Header.Get("Location"))
	}

	run := waitForRun(t, store, runID)
	waitForRunStatus(t, store, runID, "waiting")
	waitForPendingApprovals(t, run, 1)

	actionResp := mustPOSTJSON(t, client, ts.URL+"/runs/"+runID+"/action", `{"type":"deny_tool_call","session_id":"wrong-session","tool_call_id":"tool_deny","message":"do not run it"}`)
	defer actionResp.Body.Close()
	if actionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(actionResp.Body)
		t.Fatalf("deny status = %d, body=%s", actionResp.StatusCode, string(body))
	}
	var actionBody struct {
		Action    string `json:"action"`
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(actionResp.Body).Decode(&actionBody); err != nil {
		t.Fatalf("decode deny response: %v", err)
	}
	if actionBody.Action != agui.ActionDenyToolCall {
		t.Fatalf("deny action = %q, want %q", actionBody.Action, agui.ActionDenyToolCall)
	}
	if actionBody.SessionID != run.Session().ID {
		t.Fatalf("deny session_id = %q, want %q", actionBody.SessionID, run.Session().ID)
	}
	if actionBody.Message != "do not run it" {
		t.Fatalf("deny message = %q, want %q", actionBody.Message, "do not run it")
	}

	waitForRunStatus(t, store, runID, "failed")
	assertHTMLContains(t, mustGETBody(t, client, ts.URL+"/runs/"+runID+"/sidebar"),
		"failed",
		"data-run-status=\"failed\"",
		"data-run-last-event-label=\"Run failed\"",
		"No pending approvals.",
	)
}

func TestHandleActionAbortUsesRunSpecificSessionAndMarksRunAborted(t *testing.T) {
	store := NewRunStateStore()
	server := MustNewServer(
		WithRunStore(store),
		WithRunStarter(RunStarterFunc(func(ctx context.Context, runtime *RunRuntime, req RunStartRequest) error {
			core.Publish(runtime.EventBus, core.RunStartedEvent{RunID: runtime.RunID, Prompt: req.Prompt, StartedAt: time.Now().UTC()})
			<-ctx.Done()
			return ctx.Err()
		})),
	)

	ts := httptest.NewServer(server)
	defer ts.Close()
	client := ts.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	startResp := mustPOSTJSON(t, client, ts.URL+"/runs/start", `{"prompt":"abort me"}`)
	defer startResp.Body.Close()
	runID := strings.TrimPrefix(startResp.Header.Get("Location"), "/runs/")
	if runID == "" {
		t.Fatalf("redirect location = %q, want /runs/<id>", startResp.Header.Get("Location"))
	}

	run := waitForRun(t, store, runID)
	waitForRunStatus(t, store, runID, "running")
	actionResp := mustPOSTJSON(t, client, ts.URL+"/runs/"+runID+"/action", `{"type":"abort_session","session_id":"wrong-session"}`)
	defer actionResp.Body.Close()
	if actionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(actionResp.Body)
		t.Fatalf("abort status = %d, body=%s", actionResp.StatusCode, string(body))
	}
	var actionBody struct {
		Action    string `json:"action"`
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(actionResp.Body).Decode(&actionBody); err != nil {
		t.Fatalf("decode abort response: %v", err)
	}
	if actionBody.Action != agui.ActionAbortSession {
		t.Fatalf("abort action = %q, want %q", actionBody.Action, agui.ActionAbortSession)
	}
	if actionBody.SessionID != run.Session().ID {
		t.Fatalf("abort session_id = %q, want %q", actionBody.SessionID, run.Session().ID)
	}
	if actionBody.Message != "session aborted" {
		t.Fatalf("abort message = %q, want session aborted", actionBody.Message)
	}

	waitForRunStatus(t, store, runID, "aborted")
	if got := run.Session().GetStatus(); got != agui.SessionStatusAborted {
		t.Fatalf("session status = %q, want %q", got, agui.SessionStatusAborted)
	}
}

func TestRunSnapshotStructuredActivityAndWaitingAfterResumeBeforeApprovalResolved(t *testing.T) {
	store := NewRunStateStore()
	run := store.create(RunStartRequest{Prompt: "review the tool call"})
	now := time.Now().UTC()

	core.Publish(run.EventBus(), core.RunStartedEvent{RunID: run.ID(), Prompt: "review the tool call", StartedAt: now})
	core.Publish(run.EventBus(), core.ToolCalledEvent{RunID: run.ID(), ToolCallID: "tool_approve", ToolName: "dangerous_write", ArgsJSON: `{"path":"/tmp/out.txt"}`, CalledAt: now.Add(10 * time.Millisecond)})
	core.Publish(run.EventBus(), core.ApprovalRequestedEvent{RunID: run.ID(), ToolCallID: "tool_approve", ToolName: "dangerous_write", ArgsJSON: `{"path":"/tmp/out.txt"}`, RequestedAt: now.Add(20 * time.Millisecond)})
	core.Publish(run.EventBus(), core.RunWaitingEvent{RunID: run.ID(), Reason: "approval", WaitingAt: now.Add(30 * time.Millisecond)})
	core.Publish(run.EventBus(), core.RunResumedEvent{RunID: run.ID(), ResumedAt: now.Add(40 * time.Millisecond)})

	snap := run.Snapshot()
	if snap.Status != "running" {
		t.Fatalf("status = %q, want running", snap.Status)
	}
	if snap.Waiting.Active {
		t.Fatalf("waiting active = %v, want false", snap.Waiting.Active)
	}
	if snap.WaitingReason != "" {
		t.Fatalf("waiting reason = %q, want empty", snap.WaitingReason)
	}
	if !snap.Controls.CanApproveTools {
		t.Fatal("expected controls to keep approval action available")
	}
	if snap.Controls.PendingApprovalCount != 1 {
		t.Fatalf("pending approvals = %d, want 1", snap.Controls.PendingApprovalCount)
	}
	if snap.Controls.PrimaryActionLabel != "Review approvals" {
		t.Fatalf("primary action = %q, want %q", snap.Controls.PrimaryActionLabel, "Review approvals")
	}
	if len(snap.RecentActivity) == 0 {
		t.Fatal("expected structured recent activity")
	}
	last := snap.RecentActivity[len(snap.RecentActivity)-1]
	if last.Label != "Run resumed" {
		t.Fatalf("last activity label = %q, want %q", last.Label, "Run resumed")
	}
	if last.Summary == "" || last.OccurredLabel == "" {
		t.Fatalf("expected structured activity summary/time labels, got %+v", last)
	}
	if got := run.Session().GetStatus(); got != agui.SessionStatusRunning {
		t.Fatalf("session status = %q, want %q", got, agui.SessionStatusRunning)
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

func TestRendererFragmentLoadedPreservesStructuredSceneLabels(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	data := structuredHydrationPageData()
	runHTML, err := os.ReadFile(filepath.Join("templates", "run.html"))
	if err != nil {
		t.Fatalf("read run template: %v", err)
	}
	sidebarHTML, err := os.ReadFile(filepath.Join("templates", "sidebar.html"))
	if err != nil {
		t.Fatalf("read sidebar template: %v", err)
	}
	rendererJS, err := os.ReadFile(filepath.Join("static", "renderer.js"))
	if err != nil {
		t.Fatalf("read renderer script: %v", err)
	}

	runSceneHTML := renderTemplateFragment(t, extractSingleTag(t, string(runHTML), "section", `data-run-scene`), data)
	runStatusHTML := renderTemplateFragment(t, extractSingleTag(t, string(runHTML), "strong", `data-scene-stream-state`), data)
	runWaitingHTML := renderTemplateFragment(t, extractSingleTag(t, string(runHTML), "strong", `data-scene-waiting-reason`), data)
	runLastEventHTML := renderTemplateFragment(t, extractSingleTag(t, string(runHTML), "strong", `data-run-last-event`), data)
	sidebarStatusHTML := renderTemplateFragment(t, extractSingleTag(t, string(sidebarHTML), "span", `status status--{{.Run.StatusView.Code}}`), data)
	sidebarBodyHTML := renderTemplateFragment(t, extractSingleTag(t, string(sidebarHTML), "div", `sidebar-fragment`), data)
	assertStringContains(t, sidebarBodyHTML, "Waiting for approval")

	runSceneHTML = strings.ReplaceAll(runSceneHTML, `data-events-url="/runs/{{.Run.ID}}/events"`, `data-events-url=""`)
	payload, err := json.Marshal(buildSnapshotPayload(data.Run))
	if err != nil {
		t.Fatalf("marshal snapshot payload: %v", err)
	}

	script := fmt.Sprintf(`
const listeners = {};
const bodyListeners = {};
const body = {
  dataset: { route: '/runs/run_test' },
  addEventListener(type, cb) { (bodyListeners[type] ||= []).push(cb); },
  querySelectorAll() { return []; },
  matches() { return false; },
  setAttribute() {},
};
const document = {
  body,
  addEventListener(type, cb) { (listeners[type] ||= []).push(cb); },
  querySelectorAll(selector) {
    if (selector === '[data-run-scene]') return [sceneRoot];
    if (selector === '[data-run-status-badge]' || selector === '.panel__header--run .status') return [runStatusBadge, sidebarStatusBadge];
    if (selector === '.shell__nav a') return [];
    return [];
  },
  querySelector() { return null; },
};
const window = {
  document,
  Pretext: {},
  performance: { now: () => 0 },
  devicePixelRatio: 1,
  requestAnimationFrame: () => 1,
  cancelAnimationFrame: () => {},
  addEventListener() {},
  removeEventListener() {},
};
globalThis.window = window;
globalThis.document = document;
globalThis.ResizeObserver = function ResizeObserver() { this.observe = () => {}; this.disconnect = () => {}; };
globalThis.EventSource = function EventSource() {};

const datasetFromHTML = (tagHTML) => {
  const dataset = {};
  const regex = /([:\w-]+)(?:="([^"]*)")?/g;
  let match;
  while ((match = regex.exec(tagHTML)) !== null) {
    const name = match[1];
    if (!name || !name.startsWith('data-')) continue;
    dataset[name.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase())] = match[2] || '';
  }
  return dataset;
};
const classListFromHTML = (tagHTML) => ((tagHTML.match(/class="([^"]*)"/) || [,''])[1].split(/\s+/).filter(Boolean));
const textFromHTML = (tagHTML) => String((tagHTML.match(/>([\s\S]*)<\//) || [,''])[1]).replace(/<[^>]+>/g, '').replace(/\s+/g, ' ').trim();
const makeElement = (tagHTML) => ({
  dataset: datasetFromHTML(tagHTML),
  classList: classListFromHTML(tagHTML),
  className: (tagHTML.match(/class="([^"]*)"/) || [,''])[1],
  textContent: textFromHTML(tagHTML),
  title: '',
  setAttribute() {},
  matches() { return false; },
  querySelectorAll() { return []; },
});

const sceneRoot = {
  dataset: datasetFromHTML(%q),
  ownerDocument: document,
  querySelector(selector) {
    if (selector === '[data-scene-stream-state]') return runStatusBadge;
    if (selector === '[data-scene-waiting-reason]') return runWaitingBadge;
    if (selector === '[data-run-last-event]') return runLastEventBadge;
    return null;
  },
  querySelectorAll() { return []; },
  matches(selector) { return selector === '[data-run-scene]'; },
  setAttribute() {},
};
const dummyTarget = { matches() { return false; }, querySelectorAll() { return []; }, setAttribute() {} };
const runStatusBadge = makeElement(%q);
const runWaitingBadge = makeElement(%q);
const runLastEventBadge = makeElement(%q);
const sidebarStatusBadge = makeElement(%q);
%s
const renderer = {
  root: sceneRoot,
  scene: {
    runStatus: sceneRoot.dataset.runStatus || 'starting',
    waitingReason: sceneRoot.dataset.runWaitingReason || '',
    waitingSnapshot: { pending_approvals: { tool_approve: { ToolCallID: 'tool_approve' } } },
    customEventState: { waiting: true, pendingApproval: true, pendingDeferred: false },
    lastSeq: 0,
  },
  statusTargets: [runStatusBadge, sidebarStatusBadge],
  streamStateTarget: runStatusBadge,
  waitingReasonTarget: runWaitingBadge,
  lastEventTarget: runLastEventBadge,
  invalidateLayout() {},
  scheduleRender() {},
  connectionTarget: null,
};
renderer.updateStatus = RunSceneRenderer.prototype.updateStatus;
renderer.updateWaitingState = RunSceneRenderer.prototype.updateWaitingState;
renderer.setLastEventMeta = RunSceneRenderer.prototype.setLastEventMeta;
runScenes.set(sceneRoot, renderer);
const fragmentLoaded = bodyListeners['ui:fragment-loaded'][0];
fragmentLoaded({ detail: { runId: 'run_test', scene: %s }, target: dummyTarget });
console.log('RUN_STREAM:' + runStatusBadge.textContent);
console.log('RUN_WAITING:' + runWaitingBadge.textContent);
console.log('RUN_LAST:' + runLastEventBadge.textContent);
console.log('RUN_LAST_TITLE:' + runLastEventBadge.title);
console.log('SIDEBAR_STATUS:' + sidebarStatusBadge.textContent);
`, runSceneHTML, runStatusHTML, runWaitingHTML, runLastEventHTML, sidebarStatusHTML, string(rendererJS), string(payload))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node")
	cmd.Dir = "."
	cmd.Stdin = strings.NewReader(script)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("node renderer harness failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	assertStringContains(t, stdout.String(),
		"RUN_STREAM:Waiting",
		"RUN_WAITING:Waiting for approval",
		"RUN_LAST:live · Waiting for approval",
		"RUN_LAST_TITLE:Waiting for approval · 1 pending tool approval. · 15:06:05 UTC",
		"SIDEBAR_STATUS:Waiting",
	)
}

func structuredHydrationPageData() pageData {
	startedAt := time.Date(2025, time.January, 2, 15, 4, 5, 0, time.UTC)
	updatedAt := startedAt.Add(3 * time.Minute)
	lastAt := startedAt.Add(2 * time.Minute)
	activity := RunActivityView{
		Type:          core.RuntimeEventTypeApprovalRequested,
		Kind:          "approval",
		Tone:          "waiting",
		Label:         "Waiting for approval",
		Detail:        "1 pending tool approval.",
		Summary:       "Waiting for approval · 1 pending tool approval.",
		OccurredAt:    lastAt,
		OccurredLabel: lastAt.Format("15:04:05 MST"),
		IsWaiting:     true,
		ToolCallID:    "tool_approve",
		ToolName:      "dangerous_write",
	}
	waiting := RunWaitingView{
		Active:               true,
		Reason:               "approval",
		Label:                "Waiting for approval",
		Detail:               "1 pending tool approval.",
		Summary:              "Waiting for approval · 1 pending tool approval.",
		PendingKind:          "approval",
		ApprovalPendingCount: 1,
		StatusLabel:          "Waiting",
	}
	status := RunStatusView{
		Code:       "waiting",
		Label:      "Waiting",
		Tone:       "approval",
		Detail:     "1 pending tool approval.",
		IsWaiting:  true,
		IsTerminal: false,
	}
	controls := RunControlsView{
		CanAbort:              true,
		CanApproveTools:       true,
		PendingApprovalCount:  1,
		PendingApprovalLabel:  "1 pending tool approval",
		HasRecentActivity:     true,
		LastEventType:         activity.Type,
		LastEventLabel:        activity.Label,
		LastEventSummary:      activity.Summary,
		LastActivitySummary:   activity.Summary,
		LastEventTimeLabel:    activity.OccurredLabel,
		LastActivityTimeLabel: activity.OccurredLabel,
		StatusLabel:           status.Label,
		Summary:               "1 pending tool approval.",
		PrimaryActionLabel:    "Review approvals",
	}
	return pageData{Run: RunView{
		ID:               "run_test",
		Title:            "Approve flow",
		Status:           "waiting",
		StatusView:       status,
		Scene:            RunSceneStateView{Status: status, Waiting: waiting, LastEvent: RunEventStateView{Type: activity.Type, Label: activity.Label, Summary: activity.Summary, Detail: activity.Detail, OccurredAt: activity.OccurredAt, OccurredLabel: activity.OccurredLabel, Tone: activity.Tone}},
		Provider:         "test",
		Model:            "serve-test",
		Summary:          "exercise live dashboard flow",
		Prompt:           "please verify the UI",
		StartedAt:        startedAt,
		UpdatedAt:        updatedAt,
		RecentActivity:   []RunActivityView{activity},
		LastActivity:     activity,
		WaitingReason:    waiting.Reason,
		Waiting:          waiting,
		PendingApprovals: []PendingApprovalView{{ToolCallID: "tool_approve", ToolName: "dangerous_write", ArgsJSON: `{"path":"/tmp/out.txt"}`, RequestedAt: lastAt}},
		Controls:         controls,
	}}
}

func renderTemplateFragment(t *testing.T, source string, data any) string {
	t.Helper()
	tmpl, err := template.New("fragment").Parse(source)
	if err != nil {
		t.Fatalf("parse template fragment: %v\n%s", err, source)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute template fragment: %v", err)
	}
	return strings.TrimSpace(buf.String())
}

func extractSingleTag(t *testing.T, source, tagName, needle string) string {
	t.Helper()
	pattern := fmt.Sprintf(`(?s)<%s\b[^>]*%s[^>]*>.*?</%s>`, regexp.QuoteMeta(tagName), regexp.QuoteMeta(needle), regexp.QuoteMeta(tagName))
	re := regexp.MustCompile(pattern)
	match := strings.TrimSpace(re.FindString(source))
	if match == "" {
		t.Fatalf("could not find %s containing %q", tagName, needle)
	}
	return match
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

func mustPOSTForm(t *testing.T, client *http.Client, target string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new POST form request %s: %v", target, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST form %s: %v", target, err)
	}
	return resp
}

func mustPOSTJSON(t *testing.T, client *http.Client, target, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new POST json request %s: %v", target, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST json %s: %v", target, err)
	}
	return resp
}

func mustGET(t *testing.T, client *http.Client, target string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("new GET request %s: %v", target, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GET %s status = %d, body=%s", target, resp.StatusCode, string(body))
	}
	return resp
}

func mustGETBody(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	resp := mustGET(t, client, target)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", target, err)
	}
	return string(body)
}

func assertHTMLContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func assertStringContains(t *testing.T, value string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(value, want) {
			t.Fatalf("string missing %q:\n%s", want, value)
		}
	}
}

func waitForRun(t *testing.T, store *RunStateStore, runID string) *RunRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if run, ok := store.get(runID); ok {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %q not found", runID)
	return nil
}

func waitForRunStatus(t *testing.T, store *RunStateStore, runID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, ok := store.get(runID)
		if ok && run.Snapshot().Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, ok := store.get(runID)
	if !ok {
		t.Fatalf("run %q not found", runID)
	}
	t.Fatalf("run %q status = %q, want %q", runID, run.Snapshot().Status, want)
}

func waitForPendingApprovals(t *testing.T, run *RunRecord, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := run.ApprovalBridge().PendingCount(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pending approvals = %d, want %d", run.ApprovalBridge().PendingCount(), want)
}

type uiSSEFrame struct {
	id   string
	data string
}

type uiSSEStreamReader struct {
	t       *testing.T
	scanner *bufio.Scanner
}

func newUISSEStreamReader(t *testing.T, body io.Reader) *uiSSEStreamReader {
	t.Helper()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	return &uiSSEStreamReader{t: t, scanner: scanner}
}

func (r *uiSSEStreamReader) Next() uiSSEFrame {
	r.t.Helper()
	var frame uiSSEFrame
	var dataLines []string
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if line == "" {
			if frame.id != "" || len(dataLines) > 0 {
				frame.data = strings.Join(dataLines, "\n")
				return frame
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			frame.id = strings.TrimPrefix(line, "id: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if err := r.scanner.Err(); err != nil {
		r.t.Fatalf("read SSE frame: %v", err)
	}
	r.t.Fatal("no SSE frame received")
	return uiSSEFrame{}
}

func mustOpenUISSE(t *testing.T, client *http.Client, target string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("new SSE request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open SSE stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("SSE status = %d, body=%s", resp.StatusCode, string(body))
	}
	return resp
}

func readUISSEFrames(t *testing.T, reader *uiSSEStreamReader, count int) []map[string]any {
	t.Helper()
	frames := make([]map[string]any, 0, count)
	for range count {
		frame := reader.Next()
		var payload map[string]any
		if err := json.Unmarshal([]byte(frame.data), &payload); err != nil {
			t.Fatalf("unmarshal SSE frame %q: %v", frame.data, err)
		}
		frames = append(frames, payload)
	}
	return frames
}

func assertUISSEFrameTypes(t *testing.T, frames []map[string]any, wants ...string) {
	t.Helper()
	for _, want := range wants {
		found := false
		for _, frame := range frames {
			if got, _ := frame["type"].(string); got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing SSE frame type %q in %+v", want, frames)
		}
	}
}

func assertUISSECustomEvent(t *testing.T, frames []map[string]any, wantName string) {
	t.Helper()
	for _, frame := range frames {
		if gotType, _ := frame["type"].(string); gotType != agui.AGUICustom {
			continue
		}
		if gotName, _ := frame["name"].(string); gotName == wantName {
			return
		}
	}
	t.Fatalf("missing custom SSE event %q in %+v", wantName, frames)
}

func assertUISSEFrameContains(t *testing.T, frames []map[string]any, eventType, key, want string) {
	t.Helper()
	for _, frame := range frames {
		if gotType, _ := frame["type"].(string); gotType != eventType {
			continue
		}
		if got, _ := frame[key].(string); got == want {
			return
		}
	}
	t.Fatalf("missing %s frame with %s=%q in %+v", eventType, key, want, frames)
}

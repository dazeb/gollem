package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/ext/agui"
)

type pageData struct {
	AppTitle         string
	PageTitle        string
	Path             string
	CurrentYear      int
	IsRunPage        bool
	Runs             []RunView
	Run              RunView
	RunStartDefaults RunStartRequest
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.render(w, "index", pageData{
		AppTitle:         "gollem",
		PageTitle:        "Dashboard",
		Path:             r.URL.Path,
		CurrentYear:      time.Now().Year(),
		Runs:             s.runs.listViews(),
		RunStartDefaults: s.runStartDefaults,
	})
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRunStartRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	run := s.startRun(req)
	http.Redirect(w, r, "/runs/"+run.ID(), http.StatusSeeOther)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}
	view := run.Snapshot()
	s.render(w, "run", pageData{
		AppTitle:    "gollem",
		PageTitle:   view.Title,
		Path:        r.URL.Path,
		CurrentYear: time.Now().Year(),
		IsRunPage:   true,
		Runs:        s.runs.listViews(),
		Run:         view,
	})
}

func buildSnapshotPayload(view RunView) map[string]any {
	return map[string]any{
		"runId": view.ID,
		"status": map[string]any{
			"code":       view.Scene.Status.Code,
			"label":      view.Scene.Status.Label,
			"tone":       view.Scene.Status.Tone,
			"detail":     view.Scene.Status.Detail,
			"isWaiting":  view.Scene.Status.IsWaiting,
			"isTerminal": view.Scene.Status.IsTerminal,
		},
		"waiting": map[string]any{
			"active":               view.Scene.Waiting.Active,
			"reason":               view.Scene.Waiting.Reason,
			"label":                view.Scene.Waiting.Label,
			"detail":               view.Scene.Waiting.Detail,
			"summary":              view.Scene.Waiting.Summary,
			"pendingKind":          view.Scene.Waiting.PendingKind,
			"approvalPendingCount": view.Scene.Waiting.ApprovalPendingCount,
			"statusLabel":          view.Scene.Waiting.StatusLabel,
		},
		"lastEvent": map[string]any{
			"type":          view.Scene.LastEvent.Type,
			"label":         view.Scene.LastEvent.Label,
			"summary":       view.Scene.LastEvent.Summary,
			"detail":        view.Scene.LastEvent.Detail,
			"occurredLabel": view.Scene.LastEvent.OccurredLabel,
			"tone":          view.Scene.LastEvent.Tone,
		},
		"pendingApprovals": pendingApprovalsByID(view.PendingApprovals),
	}
}

func pendingApprovalsByID(items []PendingApprovalView) map[string]PendingApprovalView {
	if len(items) == 0 {
		return map[string]PendingApprovalView{}
	}
	out := make(map[string]PendingApprovalView, len(items))
	for _, item := range items {
		out[item.ToolCallID] = item
	}
	return out
}

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}
	view := run.Snapshot()
	trigger, err := json.Marshal(map[string]any{
		"ui:fragment-loaded": map[string]any{
			"runId": view.ID,
			"scene": buildSnapshotPayload(view),
		},
	})
	if err == nil {
		w.Header().Set("HX-Trigger", string(trigger))
	}
	s.render(w, "sidebar", pageData{
		AppTitle:    "gollem",
		PageTitle:   view.Title + " sidebar",
		Path:        r.URL.Path,
		CurrentYear: time.Now().Year(),
		Run:         view,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}
	run.sseHandler().ServeHTTP(w, r)
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}

	action, err := decodeAction(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action.SessionID = run.Session().ID
	before := run.Snapshot()

	body, err := json.Marshal(action)
	if err != nil {
		http.Error(w, fmt.Sprintf("encode action: %v", err), http.StatusInternalServerError)
		return
	}

	clone := r.Clone(r.Context())
	clone.Body = ioNopCloser{Reader: bytes.NewReader(body)}
	clone.ContentLength = int64(len(body))
	clone.Header.Set("Content-Type", "application/json")

	if isHTMXRequest(r) {
		rec := newBufferedResponseWriter()
		run.actionHandler().ServeHTTP(rec, clone)
		if rec.statusCode >= http.StatusBadRequest {
			copyHeader(w.Header(), rec.header)
			w.WriteHeader(rec.statusCode)
			_, _ = w.Write(rec.body.Bytes())
			return
		}
		waitForRunMutation(run, before, 750*time.Millisecond)
		s.handleSidebar(w, r)
		return
	}

	run.actionHandler().ServeHTTP(w, clone)
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	tmpl, ok := s.pages[page]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown ui page %q", page), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, page, data); err != nil {
		http.Error(w, fmt.Sprintf("render %s: %v", page, err), http.StatusInternalServerError)
	}
}

func (s *Server) lookupRun(w http.ResponseWriter, r *http.Request) (*RunRecord, bool) {
	run, ok := s.runs.get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return nil, false
	}
	return run, true
}

func decodeRunStartRequest(r *http.Request) (RunStartRequest, error) {
	var req RunStartRequest
	if requestHasJSONBody(r) {
		if err := decodeJSONRequest(r, &req); err != nil {
			return RunStartRequest{}, err
		}
		return req, nil
	}

	if err := parseRequestForm(r); err != nil {
		return RunStartRequest{}, err
	}
	req.Title = r.FormValue("title")
	req.Summary = r.FormValue("summary")
	req.Prompt = r.FormValue("prompt")
	req.Provider = r.FormValue("provider")
	req.Model = r.FormValue("model")
	return req, nil
}

func decodeAction(r *http.Request) (agui.Action, error) {
	var action agui.Action
	if requestHasJSONBody(r) {
		if err := decodeJSONRequest(r, &action); err != nil {
			return agui.Action{}, err
		}
		return action, nil
	}

	if err := parseRequestForm(r); err != nil {
		return agui.Action{}, err
	}
	action.Type = parseFormActionType(r)
	action.SessionID = r.FormValue("session_id")
	action.ToolCallID = r.FormValue("tool_call_id")
	action.ToolName = r.FormValue("tool_name")
	action.Content = r.FormValue("content")
	action.Message = firstNonEmptyFormValue(r, "message", "reason")

	approved, err := parseOptionalBoolFormValue(r, "approved")
	if err != nil {
		return agui.Action{}, err
	}
	action.Approved = approved
	if action.Type == "" && approved != nil {
		if *approved {
			action.Type = agui.ActionApproveToolCall
		} else {
			action.Type = agui.ActionDenyToolCall
		}
	}

	isError, err := parseOptionalBoolFormValue(r, "is_error")
	if err != nil {
		return agui.Action{}, err
	}
	if isError != nil {
		action.IsError = *isError
	}

	lastSeq, err := parseOptionalUint64FormValue(r, "last_seq")
	if err != nil {
		return agui.Action{}, err
	}
	if lastSeq != nil {
		action.LastSeq = *lastSeq
	}

	return action, nil
}

func requestHasJSONBody(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json")
}

func decodeJSONRequest(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return errors.New("invalid request body: multiple JSON values are not allowed")
		}
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func parseRequestForm(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("invalid form body: %w", err)
	}
	return nil
}

func parseOptionalBoolFormValue(r *http.Request, key string) (*bool, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("invalid form body: %s must be a boolean", key)
	}
	return &parsed, nil
}

func parseOptionalUint64FormValue(r *http.Request, key string) (*uint64, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid form body: %s must be an unsigned integer", key)
	}
	return &parsed, nil
}

func parseFormActionType(r *http.Request) string {
	if actionType := strings.TrimSpace(r.FormValue("type")); actionType != "" {
		return actionType
	}
	if actionType := normalizeFormActionType(firstNonEmptyFormValue(r, "decision", "action")); actionType != "" {
		return actionType
	}
	if firstNonEmptyFormValue(r, "approve") != "" {
		return agui.ActionApproveToolCall
	}
	if firstNonEmptyFormValue(r, "deny", "reject") != "" {
		return agui.ActionDenyToolCall
	}
	if firstNonEmptyFormValue(r, "abort", "abort_session") != "" {
		return agui.ActionAbortSession
	}
	return ""
}

func normalizeFormActionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case agui.ActionApproveToolCall, "approve", "approved":
		return agui.ActionApproveToolCall
	case agui.ActionDenyToolCall, "deny", "denied", "reject", "rejected":
		return agui.ActionDenyToolCall
	case agui.ActionAbortSession, "abort", "aborted", "cancel", "cancelled", "canceled":
		return agui.ActionAbortSession
	default:
		return ""
	}
}

func firstNonEmptyFormValue(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			return value
		}
	}
	return ""
}

type ioNopCloser struct {
	*bytes.Reader
}

func (ioNopCloser) Close() error { return nil }

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/ext/agui"
)

type pageData struct {
	AppTitle    string
	PageTitle   string
	Path        string
	CurrentYear int
	IsRunPage   bool
	Runs        []RunView
	Run         RunView
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.render(w, "index", pageData{
		AppTitle:    "gollem",
		PageTitle:   "Dashboard",
		Path:        r.URL.Path,
		CurrentYear: time.Now().Year(),
		Runs:        s.runs.listViews(),
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
		"status":                         view.Scene.Status.Code,
		"status_label":                   view.Scene.Status.Label,
		"status_tone":                    view.Scene.Status.Tone,
		"status_detail":                  view.Scene.Status.Detail,
		"status_is_waiting":              view.Scene.Status.IsWaiting,
		"status_is_terminal":             view.Scene.Status.IsTerminal,
		"waiting_reason":                 view.Scene.Waiting.Reason,
		"waiting_label":                  view.Scene.Waiting.Label,
		"waiting_detail":                 view.Scene.Waiting.Detail,
		"waiting_summary":                view.Scene.Waiting.Summary,
		"waiting_pending_kind":           view.Scene.Waiting.PendingKind,
		"waiting_approval_pending_count": view.Scene.Waiting.ApprovalPendingCount,
		"waiting_status_label":           view.Scene.Waiting.StatusLabel,
		"last_event_type":                view.Scene.LastEvent.Type,
		"last_event_label":               view.Scene.LastEvent.Label,
		"last_event_summary":             view.Scene.LastEvent.Summary,
		"last_event_detail":              view.Scene.LastEvent.Detail,
		"last_event_occurred_label":      view.Scene.LastEvent.OccurredLabel,
		"last_event_tone":                view.Scene.LastEvent.Tone,
		"pending_approvals":              pendingApprovalsByID(view.PendingApprovals),
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
			"scene": map[string]any{
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
			},
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

	body, err := json.Marshal(action)
	if err != nil {
		http.Error(w, fmt.Sprintf("encode action: %v", err), http.StatusInternalServerError)
		return
	}

	clone := r.Clone(r.Context())
	clone.Body = ioNopCloser{Reader: bytes.NewReader(body)}
	clone.ContentLength = int64(len(body))
	clone.Header.Set("Content-Type", "application/json")
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
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return RunStartRequest{}, fmt.Errorf("invalid request body: %w", err)
		}
		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return RunStartRequest{}, fmt.Errorf("invalid form body: %w", err)
	}
	req.Title = r.FormValue("title")
	req.Summary = r.FormValue("summary")
	req.Prompt = r.FormValue("prompt")
	req.Provider = r.FormValue("provider")
	req.Model = r.FormValue("model")
	return req, nil
}

func decodeAction(r *http.Request) (agui.Action, error) {
	defer r.Body.Close()
	var action agui.Action
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		return agui.Action{}, fmt.Errorf("invalid request body: %w", err)
	}
	return action, nil
}

type ioNopCloser struct {
	*bytes.Reader
}

func (ioNopCloser) Close() error { return nil }

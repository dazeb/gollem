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

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	run, ok := s.lookupRun(w, r)
	if !ok {
		return
	}
	view := run.Snapshot()

	w.Header().Set("HX-Trigger", "ui:fragment-loaded")
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

package ui

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RunView is placeholder scaffold data for a UI run page until live AGUI state
// is wired in.
type RunView struct {
	ID        string
	Title     string
	Status    string
	Provider  string
	Model     string
	Summary   string
	Prompt    string
	StartedAt time.Time
	UpdatedAt time.Time
	Events    []string
}

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
	s.render(w, "index", pageData{
		AppTitle:    "gollem",
		PageTitle:   "Dashboard",
		Path:        r.URL.Path,
		CurrentYear: time.Now().Year(),
		Runs:        sampleRuns(),
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	run := findRun(r.PathValue("id"))
	s.render(w, "run", pageData{
		AppTitle:    "gollem",
		PageTitle:   run.Title,
		Path:        r.URL.Path,
		CurrentYear: time.Now().Year(),
		IsRunPage:   true,
		Runs:        sampleRuns(),
		Run:         run,
	})
}

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	run := findRun(r.PathValue("id"))
	w.Header().Set("HX-Trigger", "ui:fragment-loaded")
	s.render(w, "sidebar", pageData{
		AppTitle:    "gollem",
		PageTitle:   run.Title,
		Path:        r.URL.Path,
		CurrentYear: time.Now().Year(),
		IsRunPage:   true,
		Run:         run,
	})
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

func sampleRuns() []RunView {
	now := time.Now().UTC()
	return []RunView{
		{
			ID:        "run_01",
			Title:     "AGUI Dashboard Scaffold",
			Status:    "running",
			Provider:  "openai",
			Model:     "gpt-5.4",
			Summary:   "Embedding templates, assets, and a shell layout into the binary.",
			Prompt:    "Create the UI server scaffold with embedded templates and assets.",
			StartedAt: now.Add(-8 * time.Minute),
			UpdatedAt: now.Add(-30 * time.Second),
			Events: []string{
				"session.opened",
				"run.started",
				"model.request.started",
				"model.response.completed",
			},
		},
		{
			ID:        "run_02",
			Title:     "Sidebar Fragment Demo",
			Status:    "waiting",
			Provider:  "anthropic",
			Model:     "claude-opus-4-6",
			Summary:   "Shows an htmx-loaded sidebar fragment and renderer hydration hook.",
			Prompt:    "Render the run page chrome and load the sidebar lazily.",
			StartedAt: now.Add(-27 * time.Minute),
			UpdatedAt: now.Add(-2 * time.Minute),
			Events: []string{
				"session.opened",
				"run.started",
				"session.waiting",
			},
		},
	}
}

func findRun(id string) RunView {
	id = strings.TrimSpace(id)
	for _, run := range sampleRuns() {
		if run.ID == id {
			return run
		}
	}

	now := time.Now().UTC()
	return RunView{
		ID:        id,
		Title:     "Run " + id,
		Status:    "pending",
		Provider:  "unassigned",
		Model:     "unknown",
		Summary:   "This scaffold route is live but not yet wired to persisted run data.",
		Prompt:    "Waiting for runtime data source integration.",
		StartedAt: now,
		UpdatedAt: now,
		Events: []string{
			"session.opened",
		},
	}
}

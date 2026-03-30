package ui

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/fugue-labs/gollem/core"
)

//go:embed templates/*.html static/*
var embeddedFiles embed.FS

// ServerOption customizes a UI server.
type ServerOption func(*Server)

// WithRunStarter installs the asynchronous starter used by POST /runs/start.
func WithRunStarter(starter RunStarter) ServerOption {
	return func(s *Server) {
		if starter != nil {
			s.starter = starter
		}
	}
}

// WithRunStore replaces the default in-memory run store.
func WithRunStore(store *RunStateStore) ServerOption {
	return func(s *Server) {
		if store != nil {
			s.runs = store
		}
	}
}

// Server serves the AGUI dashboard scaffold from embedded templates and assets.
type Server struct {
	mux     *http.ServeMux
	pages   map[string]*template.Template
	static  http.Handler
	runs    *RunStateStore
	starter RunStarter
}

// NewServer builds a UI server backed by embedded templates and live run state.
func NewServer(opts ...ServerOption) (*Server, error) {
	pages, err := parsePageTemplates()
	if err != nil {
		return nil, err
	}

	staticFS, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("open ui static fs: %w", err)
	}

	s := &Server{
		mux:     http.NewServeMux(),
		pages:   pages,
		static:  http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))),
		runs:    NewRunStateStore(),
		starter: RunStarterFunc(defaultRunStarter),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}

	s.routes()
	return s, nil
}

func parsePageTemplates() (map[string]*template.Template, error) {
	indexPage, err := template.ParseFS(embeddedFiles, "templates/layout.html", "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("parse index templates: %w", err)
	}

	runPage, err := template.ParseFS(embeddedFiles, "templates/layout.html", "templates/run.html")
	if err != nil {
		return nil, fmt.Errorf("parse run templates: %w", err)
	}

	sidebarFragment, err := template.ParseFS(embeddedFiles, "templates/sidebar.html")
	if err != nil {
		return nil, fmt.Errorf("parse sidebar templates: %w", err)
	}

	return map[string]*template.Template{
		"index":   indexPage,
		"run":     runPage,
		"sidebar": sidebarFragment,
	}, nil
}

// MustNewServer builds a UI server or panics if the embedded assets are invalid.
func MustNewServer(opts ...ServerOption) *Server {
	s, err := NewServer(opts...)
	if err != nil {
		panic(err)
	}
	return s
}

// Runs returns the live run registry backing the server.
func (s *Server) Runs() *RunStateStore {
	return s.runs
}

// Handler returns the configured HTTP handler tree.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ServeHTTP lets Server satisfy http.Handler directly.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.Handle("GET /", http.HandlerFunc(s.handleIndex))
	s.mux.Handle("POST /runs/start", http.HandlerFunc(s.handleStartRun))
	s.mux.Handle("GET /runs/{id}", http.HandlerFunc(s.handleRun))
	s.mux.Handle("GET /runs/{id}/sidebar", http.HandlerFunc(s.handleSidebar))
	s.mux.Handle("GET /runs/{id}/events", http.HandlerFunc(s.handleEvents))
	s.mux.Handle("POST /runs/{id}/action", http.HandlerFunc(s.handleAction))
	s.mux.Handle("GET /static/{path...}", s.static)
}

func (s *Server) startRun(req RunStartRequest) *RunRecord {
	record := s.runs.create(req)
	ctx, cancel := context.WithCancel(context.Background())
	record.setCancel(cancel)

	go func() {
		defer record.closeRuntime()
		if err := s.starter.StartRun(ctx, record.Runtime(), req); err != nil {
			if ctx.Err() != nil {
				if !record.hasRuntimeEvent(core.RuntimeEventTypeRunCompleted) {
					record.markAborted(time.Now().UTC())
				}
				return
			}
			record.failStart(err)
		}
	}()

	return record
}

func defaultRunStarter(_ context.Context, runtime *RunRuntime, req RunStartRequest) error {
	now := time.Now().UTC()
	core.Publish(runtime.EventBus, core.RunStartedEvent{
		RunID:     runtime.RunID,
		Prompt:    req.Prompt,
		StartedAt: now,
	})
	core.Publish(runtime.EventBus, core.RunCompletedEvent{
		RunID:       runtime.RunID,
		Success:     true,
		StartedAt:   now,
		CompletedAt: now,
	})
	return nil
}

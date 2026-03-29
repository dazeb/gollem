package ui

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html static/*
var embeddedFiles embed.FS

// Server serves the AGUI dashboard scaffold from embedded templates and assets.
type Server struct {
	mux       *http.ServeMux
	templates *template.Template
	static    http.Handler
}

// NewServer builds a UI server backed by embedded templates and static assets.
func NewServer() (*Server, error) {
	templates, err := template.New("ui").ParseFS(embeddedFiles, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse ui templates: %w", err)
	}

	staticFS, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("open ui static fs: %w", err)
	}

	s := &Server{
		mux:       http.NewServeMux(),
		templates: templates,
		static:    http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))),
	}
	s.routes()
	return s, nil
}

// MustNewServer builds a UI server or panics if the embedded assets are invalid.
func MustNewServer() *Server {
	s, err := NewServer()
	if err != nil {
		panic(err)
	}
	return s
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
	s.mux.Handle("GET /runs/{id}", http.HandlerFunc(s.handleRun))
	s.mux.Handle("GET /runs/{id}/sidebar", http.HandlerFunc(s.handleSidebar))
	s.mux.Handle("GET /static/{path...}", s.static)
}

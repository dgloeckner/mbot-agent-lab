// Package web serves the browser UI (team page, control room, chat wall) and
// live updates via Server-Sent Events. Templates and static files are embedded.
// Student-facing text is German; code and comments are English.
package web

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"ago": func(t time.Time) string {
		if t.IsZero() {
			return "nie"
		}
		return time.Since(t).Round(time.Second).String()
	},
}).ParseFS(templateFS, "templates/*.html"))

// Server holds the HTTP handlers.
type Server struct {
	Robots *robots.Registry
	Log    *slog.Logger
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /{$}", s.index)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// index is a temporary overview page until the control room exists.
func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "index.html", s.Robots.Snapshot()); err != nil {
		s.Log.Error("render index", "err", err)
	}
}

// Package web serves the browser UI (team page, control room, chat wall) and
// live updates via Server-Sent Events. Templates and the built team page are
// embedded. Student-facing text is German; code and comments are English.
package web

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/lab"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

// dist holds the team page built by `npx nx run web:build` in dist/app.
// dist/README.md keeps the directory present when the frontend is not built.
//
//go:embed all:dist
var distFS embed.FS

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
	Config *config.Config
	Robots *robots.Registry
	Store  *store.Store
	Hub    *live.Hub
	Lab    *lab.Service
	Agents *agent.Manager
	Log    *slog.Logger

	app     fs.FS    // built pages, nil if not built
	gzCache sync.Map // asset path -> gzipped bytes
	admins  sessions
	limiter limiter
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	if sub, err := fs.Sub(distFS, "dist/app"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			s.app = sub
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /assets/", s.assets)
	mux.HandleFunc("GET /join", s.page)
	mux.HandleFunc("POST /api/join", s.join)
	s.adminRoutes(mux)

	team := func(pattern string, h teamHandler) {
		mux.Handle(pattern, s.withTeam(h))
	}
	team("GET /t/{token}", s.teamPage)
	team("GET /t/{token}/api/session", s.session)
	team("GET /t/{token}/prompts", s.listPrompts)
	team("GET /t/{token}/prompts/{version}", s.getPrompt)
	team("POST /t/{token}/prompt", s.deployPrompt)
	team("POST /t/{token}/message", s.message)
	team("POST /t/{token}/conversation/new", s.newConversation)
	team("POST /t/{token}/start", s.start)
	team("POST /t/{token}/stop", s.stop)
	team("POST /t/{token}/tools/{name}", s.runTool)
	team("GET /t/{token}/blocks", s.blocks)
	team("GET /t/{token}/events", s.events)
	team("GET /t/{token}/notes", s.listNotes)
	team("POST /t/{token}/notes", s.addNote)
	team("PUT /t/{token}/notes/{id}", s.updateNote)
	team("DELETE /t/{token}/notes/{id}", s.deleteNote)
	team("GET /t/{token}/tasks", s.teamTasks)
	team("GET /t/{token}/tasks/{id}", s.teamTask)
	team("POST /t/{token}/tasks/{id}/accept", s.acceptTask)
	team("POST /t/{token}/tasks/{id}/activate", s.activateTask)
	team("PUT /t/{token}/name", s.renameTeam)
	team("PUT /t/{token}/model", s.setModel)
	team("PUT /t/{token}/robots/{robot}/alias", s.renameRobot)
	team("POST /t/{token}/robots/{robot}/identify", s.identifyRobot)
	return mux
}

// Run forwards robot events into the team logs until ctx ends.
func (s *Server) Run(ctx context.Context) {
	ch := make(chan robots.Event, 64)
	s.Robots.Subscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			// Finished skills are already shown as the result of the tool call.
			if k := ev.Event.Kind; k == protocol.EventSkillDone || k == protocol.EventSkillAborted {
				continue
			}
			for _, rb := range s.Robots.Snapshot() {
				if rb.ID == ev.RobotID && rb.Owner != "" {
					s.Hub.Publish(rb.Owner, live.LogEntry{
						At: ev.At, Source: live.SourceRobot, Kind: ev.Event.Kind, Robot: rb.Alias, Data: ev.Event.Data,
					})
				}
			}
		}
	}
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

// assets serves the hashed files of the built team page. Text assets are
// gzipped once and kept in memory; their names change with every build.
func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	if s.app == nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	ext := path.Ext(name)
	if (ext == ".js" || ext == ".css") && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		if gz, ok := s.gzipped(name); ok {
			h.Set("Content-Type", mime.TypeByExtension(ext))
			h.Set("Content-Encoding", "gzip")
			h.Set("Vary", "Accept-Encoding")
			_, _ = w.Write(gz)
			return
		}
	}
	http.FileServerFS(s.app).ServeHTTP(w, r)
}

func (s *Server) gzipped(name string) ([]byte, bool) {
	if v, ok := s.gzCache.Load(name); ok {
		return v.([]byte), true
	}
	raw, err := fs.ReadFile(s.app, name)
	if err != nil {
		return nil, false
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	s.gzCache.Store(name, buf.Bytes())
	return buf.Bytes(), true
}

type teamHandler func(w http.ResponseWriter, r *http.Request, team store.Team)

// securityHeaders keeps secret links out of referrers and caches.
func securityHeaders(w http.ResponseWriter) {
	hdr := w.Header()
	hdr.Set("Referrer-Policy", "no-referrer")
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
}

// withTeam resolves the secret team token from the URL.
func (s *Server) withTeam(h teamHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		t, err := s.Lab.TeamByToken(r.Context(), r.PathValue("token"))
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Diesen Team-Link gibt es nicht (mehr). Gründet auf /join ein Team oder fragt die Lab-Leitung.")
			return
		}
		if err != nil {
			s.serverError(w, err)
			return
		}
		if r.Method != http.MethodGet && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "Diese Anfrage kam nicht von der Lab-Seite.")
			return
		}
		h(w, r, t)
	})
}

// sameOrigin rejects cross-site form posts: browsers send Origin on POST/PUT/DELETE.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func (s *Server) teamPage(w http.ResponseWriter, r *http.Request, _ store.Team) {
	s.page(w, r)
}

// page serves the single-page app; it picks the view from the URL.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if s.app == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("The web pages are not built. Run `npx nx run web:build` and restart the server.\n"))
		return
	}
	http.ServeFileFS(w, r, s.app, "index.html")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes a small JSON body into v.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Die Anfrage war ungültig.")
		return false
	}
	return true
}

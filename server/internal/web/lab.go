package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/lab"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// labError turns service errors into HTTP responses.
func (s *Server) labError(w http.ResponseWriter, err error) {
	var ue *lab.UserError
	switch {
	case errors.As(err, &ue):
		status := http.StatusBadRequest
		if ue.Conflict {
			status = http.StatusConflict
		}
		writeError(w, status, ue.Msg)
	case errors.Is(err, lab.ErrUnknownTeam):
		writeError(w, http.StatusNotFound, "Dieses Team gibt es nicht mehr.")
	case errors.Is(err, lab.ErrUnknownRun):
		writeError(w, http.StatusNotFound, "Diesen Lauf gibt es nicht.")
	case errors.Is(err, lab.ErrUnknownProgram):
		writeError(w, http.StatusNotFound, "Dieses Werkzeug gibt es nicht (mehr).")
	case errors.Is(err, lab.ErrUnknownRobot):
		writeError(w, http.StatusNotFound, "Diesen Roboter gibt es nicht oder er gehört nicht zu euch.")
	default:
		s.serverError(w, err)
	}
}

// --- joining ---------------------------------------------------------------

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Diese Anfrage kam nicht von der Lab-Seite.")
		return
	}
	if !s.limiter.allow("join:"+clientIP(r), 10, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "Zu viele Versuche. Wartet eine Minute und probiert es dann noch einmal.")
		return
	}
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	t, err := s.Lab.Join(r.Context(), req.Code, req.Name)
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": t.ID, "name": t.Name, "color": t.Color, "token": t.Token})
}

// --- team actions ------------------------------------------------------------

func (s *Server) renameTeam(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.RenameTeam(r.Context(), team.ID, req.Name); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) renameRobot(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Alias string `json:"alias"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.RenameRobot(r.Context(), team.ID, r.PathValue("robot"), req.Alias); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setModel records the model the team's next runs use.
func (s *Server) setModel(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Provider string `json:"provider"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.SetTeamModel(r.Context(), team.ID, req.Provider); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// identifyRobot makes one of the team's robots blink and beep.
func (s *Server) identifyRobot(w http.ResponseWriter, r *http.Request, team store.Team) {
	id := r.PathValue("robot")
	for _, rb := range s.Robots.Snapshot() {
		if rb.ID == id && rb.Owner == team.ID {
			if _, err := s.Robots.Send(r.Context(), id, "identify", nil, 3*time.Second); err != nil {
				writeError(w, http.StatusServiceUnavailable, rb.Alias+" antwortet gerade nicht. Ist er eingeschaltet?")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	s.labError(w, lab.ErrUnknownRobot)
}

// --- admin sessions ----------------------------------------------------------

const (
	adminCookie     = "lab_admin"
	adminSessionTTL = 12 * time.Hour
)

// sessions holds logged-in admin sessions in memory; a restart logs everyone out.
type sessions struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func (ss *sessions) create() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.m == nil {
		ss.m = map[string]time.Time{}
	}
	now := time.Now()
	for k, exp := range ss.m {
		if now.After(exp) {
			delete(ss.m, k)
		}
	}
	ss.m[id] = now.Add(adminSessionTTL)
	return id
}

func (ss *sessions) valid(id string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	exp, ok := ss.m[id]
	return ok && time.Now().Before(exp)
}

func (ss *sessions) remove(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.m, id)
}

// limiter is a fixed-window rate limiter per key.
type limiter struct {
	mu sync.Mutex
	m  map[string]window
}

type window struct {
	start time.Time
	n     int
}

func (l *limiter) allow(key string, max int, per time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]window{}
	}
	now := time.Now()
	w := l.m[key]
	if now.Sub(w.start) > per {
		w = window{start: now}
	}
	w.n++
	l.m[key] = w
	if len(l.m) > 10000 { // keep memory bounded
		for k, v := range l.m {
			if now.Sub(v.start) > per {
				delete(l.m, k)
			}
		}
	}
	return w.n <= max
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Diese Anfrage kam nicht vom Leitstand.")
		return
	}
	if !s.limiter.allow("login:"+clientIP(r), 10, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "Zu viele Versuche. Wartet eine Minute.")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.Config.Server.AdminPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "Das Passwort stimmt nicht.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminCookie, Value: s.admins.create(), Path: "/admin",
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
		MaxAge: int(adminSessionTTL.Seconds()),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if c, err := r.Cookie(adminCookie); err == nil {
		s.admins.remove(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/admin", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

// withAdmin requires a valid admin session.
func (s *Server) withAdmin(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		c, err := r.Cookie(adminCookie)
		if err != nil || !s.admins.valid(c.Value) {
			writeError(w, http.StatusUnauthorized, "Bitte meldet euch im Leitstand an.")
			return
		}
		if r.Method != http.MethodGet && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "Diese Anfrage kam nicht vom Leitstand.")
			return
		}
		h(w, r)
	})
}

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/lab"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", s.page)
	mux.HandleFunc("POST /admin/api/login", s.adminLogin)
	mux.HandleFunc("POST /admin/api/logout", s.adminLogout)
	admin := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.withAdmin(h)) }
	admin("GET /admin/api/state", s.adminState)
	admin("GET /admin/api/events", s.adminEvents)
	admin("POST /admin/api/assign", s.adminAssign)
	admin("POST /admin/api/lab-code", s.adminNewLabCode)
	admin("PUT /admin/api/teams/{id}", s.adminRenameTeam)
	admin("PUT /admin/api/teams/{id}/color", s.adminTeamColor)
	admin("DELETE /admin/api/teams/{id}", s.adminDeleteTeam)
	admin("POST /admin/api/stop-all", s.adminStopAll)
	admin("PUT /admin/api/models", s.adminSetModels)
	admin("PUT /admin/api/conversation", s.adminSetConversation)
}

// AdminRobot is one robot as the control room sees it.
type AdminRobot struct {
	ID       string         `json:"id"`
	Color    string         `json:"color"`
	Online   bool           `json:"online"`
	Accepted bool           `json:"accepted"`
	Script   string         `json:"script"`
	MemFree  int            `json:"mem_free"` // free heap after connecting, -1 unknown
	Team     string         `json:"team"`     // team ID, empty if free
	Alias    string         `json:"alias"`
	State    protocol.State `json:"state"`
}

// AdminTeam is one team as the control room sees it.
type AdminTeam struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Color     string       `json:"color"`
	Link      string       `json:"link"` // path of the secret team link
	Connected int          `json:"connected"`
	CreatedAt time.Time    `json:"created_at"`
	Provider  string       `json:"provider"` // model provider of the team's next run
	Agent     agent.Status `json:"agent"`    // current or last run
	Total     store.Totals `json:"total"`    // all runs and conversations of the team
}

type adminState struct {
	LabCode string      `json:"lab_code"`
	Models  []lab.Model `json:"models"`
	// Conversation: teams may talk with their agent while no run is active.
	Conversation bool         `json:"conversation"`
	Teams        []AdminTeam  `json:"teams"`
	Robots       []AdminRobot `json:"robots"`
}

func (s *Server) buildAdminState(ctx context.Context) (adminState, error) {
	teams, err := s.Store.Teams(ctx)
	if err != nil {
		return adminState{}, err
	}
	totals, err := s.Store.TeamTotals(ctx)
	if err != nil {
		return adminState{}, err
	}
	st := adminState{LabCode: s.Lab.LabCode(), Models: s.Lab.Models(), Conversation: s.Lab.ConversationEnabled(), Teams: []AdminTeam{}, Robots: []AdminRobot{}}
	for _, t := range teams {
		st.Teams = append(st.Teams, AdminTeam{
			ID: t.ID, Name: t.Name, Color: t.Color, Link: "/t/" + t.Token,
			Connected: s.Hub.Connected(t.ID), CreatedAt: t.CreatedAt, Provider: s.Lab.Provider(t), Agent: s.Agents.Status(t.ID), Total: totals[t.ID],
		})
	}
	for _, rb := range s.Robots.Snapshot() {
		st.Robots = append(st.Robots, AdminRobot{
			ID: rb.ID, Color: rb.Color, Online: rb.Online, Accepted: rb.Accepted, Script: rb.Hello.Script, MemFree: rb.Hello.MemFree,
			Team: rb.Owner, Alias: rb.Alias, State: rb.State,
		})
	}
	return st, nil
}

func (s *Server) adminState(w http.ResponseWriter, r *http.Request) {
	st, err := s.buildAdminState(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// adminEvents streams the full state whenever it changes.
func (s *Server) adminEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Live-Verbindung nicht möglich.")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")

	var last []byte
	poll := time.NewTicker(300 * time.Millisecond)
	defer poll.Stop()
	ping := time.NewTicker(ssePingInterval)
	defer ping.Stop()
	for {
		st, err := s.buildAdminState(r.Context())
		if err != nil {
			return
		}
		b, _ := json.Marshal(st)
		if !bytes.Equal(b, last) {
			last = b
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) adminAssign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Robot string `json:"robot"`
		Team  string `json:"team"` // empty frees the robot
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.AssignRobot(r.Context(), req.Robot, req.Team); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminSetModels sets the providers teams may choose besides the default.
func (s *Server) adminSetModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Allowed []string `json:"allowed"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.SetAllowedModels(r.Context(), req.Allowed); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminSetConversation switches conversations on or off for all teams.
func (s *Server) adminSetConversation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.SetConversationEnabled(r.Context(), req.On); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminNewLabCode(w http.ResponseWriter, r *http.Request) {
	code, err := s.Lab.NewLabCode(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lab_code": code})
}

func (s *Server) adminRenameTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.RenameTeam(r.Context(), r.PathValue("id"), req.Name); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminTeamColor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Color string `json:"color"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Lab.SetTeamColor(r.Context(), r.PathValue("id"), req.Color); err != nil {
		s.labError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteTeam(w http.ResponseWriter, r *http.Request) {
	if err := s.Lab.DeleteTeam(r.Context(), r.PathValue("id")); err != nil {
		s.labError(w, err)
		return
	}
	s.Agents.RemoveTeam(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// adminStopAll is the emergency stop for every robot in the lab.
func (s *Server) adminStopAll(w http.ResponseWriter, r *http.Request) {
	s.Agents.StopAll("Leitstand")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	type outcome struct {
		Robot string `json:"robot"`
		OK    bool   `json:"ok"`
	}
	all := s.Robots.Snapshot()
	results := make([]outcome, len(all))
	var wg sync.WaitGroup
	for i, rb := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Robots.Send(ctx, rb.ID, "stop", nil, 2*time.Second)
			results[i] = outcome{Robot: rb.ID, OK: err == nil}
		}()
	}
	wg.Wait()
	notified := map[string]bool{}
	for _, rb := range all {
		if rb.Owner != "" && !notified[rb.Owner] {
			notified[rb.Owner] = true
			s.Hub.Publish(rb.Owner, live.LogEntry{Source: live.SourceSystem, Kind: "stop_all", Text: "Der Leitstand hat alle Roboter angehalten."})
		}
	}
	writeJSON(w, http.StatusOK, results)
}

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Limits for team input, also sent to the page so it can show them.
const (
	MaxPromptChars  = 20000
	MaxMessageChars = 500
	MaxNoteChars    = 1000
)

// RobotView is what a team sees of one of its robots.
type RobotView struct {
	Alias    string         `json:"alias"`
	ID       string         `json:"id"`
	Color    string         `json:"color"`
	Online   bool           `json:"online"`
	Accepted bool           `json:"accepted"`
	State    protocol.State `json:"state"`
}

func (s *Server) teamRobots(teamID string) []RobotView {
	out := []RobotView{}
	for _, rb := range s.Robots.Snapshot() {
		if rb.Owner != teamID {
			continue
		}
		out = append(out, RobotView{Alias: rb.Alias, ID: rb.ID, Color: rb.Color, Online: rb.Online, Accepted: rb.Accepted, State: rb.State})
	}
	return out
}

func (s *Server) session(w http.ResponseWriter, r *http.Request, team store.Team) {
	ctx := r.Context()
	var prompt *store.Prompt
	if p, err := s.Store.GetPrompt(ctx, team.ID, 0); err == nil {
		prompt = &p
	} else if !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, err)
		return
	}
	versions, err := s.Store.ListPrompts(ctx, team.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	notes, err := s.Store.ListNotes(ctx, team.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"team":            map[string]string{"id": team.ID, "name": team.Name, "color": team.Color},
		"robots":          s.teamRobots(team.ID),
		"agent":           s.Agents.Status(team.ID),
		"agent_available": s.Agents.Available(),
		"models":          s.Lab.ModelsFor(team),
		"prompt":          prompt,
		"versions":        versions,
		"notes":           notes,
		"tools":           tools.Catalog,
		"limits": map[string]int{
			"prompt_chars": MaxPromptChars, "message_chars": MaxMessageChars, "note_chars": MaxNoteChars,
		},
	})
}

func (s *Server) listPrompts(w http.ResponseWriter, r *http.Request, team store.Team) {
	versions, err := s.Store.ListPrompts(r.Context(), team.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *Server) getPrompt(w http.ResponseWriter, r *http.Request, team store.Team) {
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v < 1 {
		writeError(w, http.StatusBadRequest, "Unbekannte Version.")
		return
	}
	p, err := s.Store.GetPrompt(r.Context(), team.ID, v)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Version %d gibt es nicht.", v))
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deployPrompt(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Text    string `json:"text"`
		Restart bool   `json:"restart"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "Der Prompt ist leer. Schreibt zuerst, was euer Agent tun soll.")
		return
	}
	if n := utf8.RuneCountInString(req.Text); n > MaxPromptChars {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Der Prompt hat %d Zeichen, erlaubt sind %d.", n, MaxPromptChars))
		return
	}
	p, err := s.Store.SavePrompt(r.Context(), team.ID, req.Text)
	if err != nil {
		s.serverError(w, err)
		return
	}
	data, _ := json.Marshal(map[string]any{"version": p.Version, "restart": req.Restart, "chars": p.Chars})
	s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceTeam, Kind: "deploy", Data: data})
	if req.Restart || s.Agents.Status(team.ID).Running {
		if err := s.Agents.Deployed(r.Context(), team.ID, p.Version, req.Restart); err != nil {
			s.agentNotice(team.ID, err)
		}
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) message(w http.ResponseWriter, r *http.Request, team store.Team) {
	var req struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "Die Nachricht ist leer.")
		return
	}
	if n := utf8.RuneCountInString(text); n > MaxMessageChars {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Die Nachricht hat %d Zeichen, erlaubt sind %d.", n, MaxMessageChars))
		return
	}
	e := s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceTeam, Kind: "chat", Text: text})
	if !s.Agents.Deliver(team.ID, agent.Message{From: agent.SourceTeam, Kind: "chat", Text: text, Wake: true}) {
		s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceSystem, Kind: "info", Text: "Euer Agent läuft gerade nicht. Startet einen Lauf, dann liest er eure Nachrichten."})
	}
	writeJSON(w, http.StatusAccepted, e)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request, team store.Team) {
	if err := s.Agents.Start(r.Context(), team.ID); err != nil {
		var se *agent.StartError
		if errors.As(err, &se) {
			writeError(w, http.StatusConflict, se.Msg)
			return
		}
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.Agents.Status(team.ID))
}

// agentNotice tells the team why its agent could not (re)start.
func (s *Server) agentNotice(teamID string, err error) {
	var se *agent.StartError
	msg := "Der Agent konnte nicht neu starten."
	if errors.As(err, &se) {
		msg = se.Msg
	} else {
		s.Log.Error("agent", "team", teamID, "err", err)
	}
	s.Hub.Publish(teamID, live.LogEntry{Source: live.SourceSystem, Kind: "info", Text: msg})
}

// stop sends the stop command to every robot of the team in parallel.
func (s *Server) stop(w http.ResponseWriter, r *http.Request, team store.Team) {
	s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceTeam, Kind: "stop"})
	s.Agents.Stop(team.ID, "Team")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	type outcome struct {
		Alias string `json:"alias"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	views := s.teamRobots(team.ID)
	results := make([]outcome, len(views))
	var wg sync.WaitGroup
	for i, rb := range views {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Robots.Send(ctx, rb.ID, "stop", nil, 2*time.Second)
			results[i] = outcome{Alias: rb.Alias, OK: err == nil}
			if err != nil {
				results[i].Error = err.Error()
			}
		}()
	}
	wg.Wait()
	for _, o := range results {
		text := fmt.Sprintf("%s hat angehalten.", o.Alias)
		if !o.OK {
			text = fmt.Sprintf("%s hat den Stopp nicht bestätigt (%s).", o.Alias, o.Error)
		}
		s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceSystem, Kind: "info", Robot: o.Alias, Text: text})
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request, team store.Team) {
	notes, err := s.Store.ListNotes(r.Context(), team.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func noteText(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &req) {
		return "", false
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "Die Notiz ist leer.")
		return "", false
	}
	if n := utf8.RuneCountInString(text); n > MaxNoteChars {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Die Notiz hat %d Zeichen, erlaubt sind %d.", n, MaxNoteChars))
		return "", false
	}
	return text, true
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request, team store.Team) {
	text, ok := noteText(w, r)
	if !ok {
		return
	}
	n, err := s.Store.AddNote(r.Context(), team.ID, text, "team")
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request, team store.Team) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Unbekannte Notiz.")
		return
	}
	text, ok := noteText(w, r)
	if !ok {
		return
	}
	s.noteResult(w, s.Store.UpdateNote(r.Context(), team.ID, id, text))
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request, team store.Team) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Unbekannte Notiz.")
		return
	}
	s.noteResult(w, s.Store.DeleteNote(r.Context(), team.ID, id))
}

func (s *Server) noteResult(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Diese Notiz gibt es nicht mehr.")
	case err != nil:
		s.serverError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.Log.Error("team request", "err", err)
	writeError(w, http.StatusInternalServerError, "Auf dem Server ist ein Fehler passiert. Versucht es gleich noch einmal.")
}

// SSE timing.
const (
	robotPollInterval = 250 * time.Millisecond
	ssePingInterval   = 15 * time.Second
)

// events streams robot snapshots ("robots"), the agent status ("agent"), the
// model choice ("models") and log entries ("log") to the team page. Last-Event-ID lets a reconnecting browser catch up on the log.
func (s *Server) events(w http.ResponseWriter, r *http.Request, team store.Team) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Live-Verbindung nicht möglich.")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // proxies must not buffer the stream

	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	backlog, ch, cancel := s.Hub.Subscribe(team.ID, after)
	defer cancel()

	var lastRobots, lastAgent, lastModels []byte
	modelsGen := int64(-1)
	sendModels := func() error {
		gen := s.Lab.ModelsVersion()
		if gen == modelsGen {
			return nil
		}
		t, err := s.Store.Team(r.Context(), team.ID)
		if err != nil {
			return nil // deleted: the page learns it from its next request
		}
		modelsGen = gen
		b, _ := json.Marshal(s.Lab.ModelsFor(t))
		if bytes.Equal(b, lastModels) {
			return nil
		}
		lastModels = b
		_, err = fmt.Fprintf(w, "event: models\ndata: %s\n\n", b)
		return err
	}
	sendAgent := func() error {
		b, _ := json.Marshal(s.Agents.Status(team.ID))
		if bytes.Equal(b, lastAgent) {
			return nil
		}
		lastAgent = b
		_, err := fmt.Fprintf(w, "event: agent\ndata: %s\n\n", b)
		return err
	}
	sendRobots := func() error {
		b, _ := json.Marshal(s.teamRobots(team.ID))
		if bytes.Equal(b, lastRobots) {
			return nil
		}
		lastRobots = b
		_, err := fmt.Fprintf(w, "event: robots\ndata: %s\n\n", b)
		return err
	}
	sendLog := func(e live.LogEntry) error {
		b, _ := json.Marshal(e)
		_, err := fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", e.ID, b)
		return err
	}

	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	if sendRobots() != nil || sendAgent() != nil || sendModels() != nil {
		return
	}
	for _, e := range backlog {
		if sendLog(e) != nil {
			return
		}
	}
	flusher.Flush()

	poll := time.NewTicker(robotPollInterval)
	defer poll.Stop()
	ping := time.NewTicker(ssePingInterval)
	defer ping.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			err = sendLog(e)
		case <-poll.C:
			// These only send when something changed.
			if err = sendRobots(); err == nil {
				if err = sendAgent(); err == nil {
					err = sendModels()
				}
			}
		case <-ping.C:
			_, err = fmt.Fprint(w, ": ping\n\n")
		}
		if err != nil {
			return
		}
		flusher.Flush()
	}
}

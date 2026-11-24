package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/programs"
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

// activeTask returns the pair the team works in, for reading. Without one it
// answers 409 and returns false.
func (s *Server) activeTask(w http.ResponseWriter, r *http.Request, team store.Team) (store.TeamTask, bool) {
	tt, _, err := s.Lab.ActiveTask(r.Context(), team.ID)
	if err != nil {
		s.labError(w, err)
		return tt, false
	}
	return tt, true
}

// writableTask returns the pair the team works in, for a change. Without an
// active task, and in a closed task, it answers 409 and returns false.
func (s *Server) writableTask(w http.ResponseWriter, r *http.Request, team store.Team) (store.TeamTask, bool) {
	tt, err := s.Lab.WritableTask(r.Context(), team.ID)
	if err != nil {
		s.labError(w, err)
		return tt, false
	}
	return tt, true
}

func (s *Server) session(w http.ResponseWriter, r *http.Request, team store.Team) {
	ctx := r.Context()
	tasks, err := s.Lab.TeamTasks(ctx, team.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	// Without an active task the page shows the team's robots and waits.
	var task *store.TeamTaskInfo
	tt := store.TeamTask{Team: team.ID}
	for i := range tasks {
		if tasks[i].Status == store.TeamTaskActive {
			active := tasks[i]
			task, tt.Task = &active, active.ID
		}
		tasks[i].Body, tasks[i].Starter = "", ""
	}
	var prompt *store.Prompt
	if p, err := s.Store.GetPrompt(ctx, tt, 0); err == nil {
		prompt = &p
	} else if !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, err)
		return
	}
	versions, err := s.Store.ListPrompts(ctx, tt)
	if err != nil {
		s.serverError(w, err)
		return
	}
	notes, err := s.Store.ListNotes(ctx, tt)
	if err != nil {
		s.serverError(w, err)
		return
	}
	total, err := s.Store.TeamTotal(ctx, team.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"team":            map[string]string{"id": team.ID, "name": team.Name, "color": team.Color},
		"task":            task,
		"tasks":           tasks,
		"robots":          s.teamRobots(team.ID),
		"agent":           s.Agents.Status(team.ID),
		"total":           total,
		"agent_available": s.Agents.Available(),
		"models":          s.Lab.ModelsFor(team),
		"prompt":          prompt,
		"versions":        versions,
		"notes":           notes,
		"tools":           s.Agents.Tools.CatalogFor(&tools.Env{TeamID: team.ID}),
		"limits": map[string]int{
			"prompt_chars": MaxPromptChars, "message_chars": MaxMessageChars, "note_chars": MaxNoteChars,
		},
	})
}

func (s *Server) listPrompts(w http.ResponseWriter, r *http.Request, team store.Team) {
	tt, ok := s.activeTask(w, r, team)
	if !ok {
		return
	}
	versions, err := s.Store.ListPrompts(r.Context(), tt)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *Server) getPrompt(w http.ResponseWriter, r *http.Request, team store.Team) {
	tt, ok := s.activeTask(w, r, team)
	if !ok {
		return
	}
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v < 1 {
		writeError(w, http.StatusBadRequest, "Unbekannte Version.")
		return
	}
	p, err := s.Store.GetPrompt(r.Context(), tt, v)
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
	tt, ok := s.writableTask(w, r, team)
	if !ok {
		return
	}
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
	p, err := s.Store.SavePrompt(r.Context(), tt, req.Text)
	if err != nil {
		s.serverError(w, err)
		return
	}
	versionTools, err := s.Lab.VersionTools(r.Context(), p.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	toolNames := []string{}
	for _, t := range versionTools {
		toolNames = append(toolNames, t.Name)
	}
	warning := s.Lab.DeployWarning(team.ID, req.Text, versionTools)
	s.Lab.Record(r.Context(), store.Event{Kind: store.EventPromptDeployed, Actor: store.ActorTeam, Team: tt.Team, Task: tt.Task, Prompt: p.ID,
		Data: map[string]any{"version": p.Version, "chars": p.Chars, "restart": req.Restart, "tools": toolNames}})
	data, _ := json.Marshal(map[string]any{"version": p.Version, "restart": req.Restart, "chars": p.Chars, "tools": toolNames})
	s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceTeam, Kind: "deploy", Data: data})
	if req.Restart || s.Agents.Status(team.ID).Running {
		if err := s.Agents.Deployed(r.Context(), team.ID, p.Version, req.Restart); err != nil {
			s.agentNotice(team.ID, err)
		}
	}
	s.Hub.SignalRef(team.ID, live.SignalPrograms, "") // the deploy states of the team tools changed
	writeJSON(w, http.StatusCreated, map[string]any{"version": p.Version, "text": p.Text, "created_at": p.CreatedAt, "chars": p.Chars,
		"tools": versionTools, "warning": warning})
}

// restoreVersion replaces the working state with a version: the team tools
// here, the prompt text for the page's draft. 409 in a closed task.
func (s *Server) restoreVersion(w http.ResponseWriter, r *http.Request, team store.Team) {
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v < 1 {
		writeError(w, http.StatusBadRequest, "Unbekannte Version.")
		return
	}
	p, err := s.Lab.RestoreVersion(r.Context(), team.ID, v)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Version %d gibt es nicht.", v))
		return
	}
	if err != nil {
		s.labError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) message(w http.ResponseWriter, r *http.Request, team store.Team) {
	if _, ok := s.writableTask(w, r, team); !ok {
		return
	}
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
	// During a run the message goes to the run; otherwise it starts or
	// continues a conversation. The returned entry's channel tells the page
	// where the message ended up.
	if s.Agents.Status(team.ID).Running {
		e := s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceTeam, Kind: "chat", Text: text})
		if !s.Agents.Deliver(team.ID, agent.Message{From: agent.SourceTeam, Kind: "chat", Text: text, Wake: true}) {
			s.Hub.Publish(team.ID, live.LogEntry{Source: live.SourceSystem, Kind: "info", Text: "Der Lauf ist gerade zu Ende gegangen. Schreibt eure Nachricht im Gespräch noch einmal."})
		}
		writeJSON(w, http.StatusAccepted, e)
		return
	}
	e, err := s.Agents.Talk(team.ID, text)
	if err != nil {
		var te *agent.TalkError
		if !errors.As(err, &te) {
			s.serverError(w, err)
			return
		}
		status := http.StatusConflict
		if te.Limited {
			status = http.StatusTooManyRequests
		}
		writeError(w, status, te.Msg)
		return
	}
	writeJSON(w, http.StatusAccepted, e)
}

// newConversation ends the open conversation; the next message starts a new one.
func (s *Server) newConversation(w http.ResponseWriter, r *http.Request, team store.Team) {
	if _, ok := s.writableTask(w, r, team); !ok {
		return
	}
	s.Agents.NewConversation(team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request, team store.Team) {
	if _, ok := s.writableTask(w, r, team); !ok {
		return
	}
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

// runTool runs one built-in tool by hand ("Werkzeuge" tab): the body holds its
// arguments. It answers the tool result as the agent would get it; mistakes
// in the arguments come back as "ok": false. 409 without an active, open task
// and while a run is active.
func (s *Server) runTool(w http.ResponseWriter, r *http.Request, team store.Team) {
	if _, ok := s.writableTask(w, r, team); !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Die Anfrage war ungültig.")
		return
	}
	result, err := s.Agents.Manual(r.Context(), team.ID, r.PathValue("name"), string(raw))
	if err != nil {
		var se *agent.StartError
		if errors.As(err, &se) {
			writeError(w, http.StatusConflict, se.Msg)
			return
		}
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(result)
}

// blocks answers the Blockly block definitions and the toolbox of the tool
// editor, built from the tool registry. No block has a robot field.
func (s *Server) blocks(w http.ResponseWriter, _ *http.Request, _ store.Team) {
	writeJSON(w, http.StatusOK, programs.Blocks(s.Agents.Tools))
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
	tt, ok := s.activeTask(w, r, team)
	if !ok {
		return
	}
	notes, err := s.Store.ListNotes(r.Context(), tt)
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
	tt, ok := s.writableTask(w, r, team)
	if !ok {
		return
	}
	text, ok := noteText(w, r)
	if !ok {
		return
	}
	n, err := s.Store.AddNote(r.Context(), tt, text, "team")
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.Lab.Record(r.Context(), store.Event{Kind: store.EventNoteAdded, Actor: store.ActorTeam, Team: tt.Team, Task: tt.Task,
		Data: map[string]any{"note_id": n.ID, "author": "team"}})
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request, team store.Team) {
	tt, ok := s.writableTask(w, r, team)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Unbekannte Notiz.")
		return
	}
	text, ok := noteText(w, r)
	if !ok {
		return
	}
	s.noteResult(w, s.Store.UpdateNote(r.Context(), tt, id, text))
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request, team store.Team) {
	tt, ok := s.writableTask(w, r, team)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Unbekannte Notiz.")
		return
	}
	err = s.Store.DeleteNote(r.Context(), tt, id)
	if err == nil {
		s.Lab.Record(r.Context(), store.Event{Kind: store.EventNoteDeleted, Actor: store.ActorTeam, Team: tt.Team, Task: tt.Task,
			Data: map[string]any{"note_id": id, "author": "team"}})
	}
	s.noteResult(w, err)
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

// events streams robot snapshots ("robots"), the agent status ("agent") with
// the team's totals over all runs ("totals"), the model choice ("models") and
// log entries ("log") to the team page. Last-Event-ID lets a reconnecting browser catch up on the log.
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
	// The page shows one task: log entries of another task never reach it.
	task := s.Hub.Task(team.ID)

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
		if _, err := fmt.Fprintf(w, "event: agent\ndata: %s\n\n", b); err != nil {
			return err
		}
		// The totals only change with the agent status: look them up then.
		total, err := s.Store.TeamTotal(r.Context(), team.ID)
		if err != nil {
			return nil
		}
		t, _ := json.Marshal(total)
		_, err = fmt.Fprintf(w, "event: totals\ndata: %s\n\n", t)
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
		if e.Signal != "" {
			if e.Signal == live.SignalTask {
				task = e.Task
			}
			data := map[string]string{"task_id": e.Task}
			if e.Ref != "" {
				data["id"] = e.Ref
			}
			b, _ := json.Marshal(data)
			if e.Payload != nil {
				b = e.Payload
			}
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Signal, b)
			return err
		}
		if e.Task != task {
			return nil
		}
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

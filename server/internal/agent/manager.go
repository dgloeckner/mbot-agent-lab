// Package agent implements the per-team agent runtime. Every team has one
// actor: a goroutine with an inbox that calls the model only when a message
// wakes it. Everything that happens is a message: team instructions, robot
// events, other agents, system notices. See docs/handoff.md, "Agent-Runtime".
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Message sources. Other agents are SourceAgentPrefix + team name.
const (
	SourceTeam        = "team"
	SourceRobot       = "robot"
	SourceSystem      = "system"
	SourceAgentPrefix = "agent:"
)

// Message is one entry in an agent's inbox.
type Message struct {
	From  string // team | robot | agent:<name> | system
	Robot string // robot name for robot messages
	Kind  string // chat | event | info | ...
	Text  string
	Data  json.RawMessage
	At    time.Time
	Wake  bool // whether this message alone wakes the agent
}

// Agent states, also shown by the robots' LEDs.
const (
	StateWaiting  = protocol.AgentWaiting
	StateThinking = protocol.AgentThinking
	StateActing   = protocol.AgentActing
)

// Status is what the team page and the control room show about an agent.
type Status struct {
	State        string    `json:"state"`
	Running      bool      `json:"running"`
	RunID        int64     `json:"run_id,omitempty"`
	Provider     string    `json:"provider,omitempty"` // model provider of the current run
	Model        string    `json:"model,omitempty"`
	StartedAt    time.Time `json:"started_at,omitzero"`
	TokensIn     int       `json:"tokens_in"`
	TokensOut    int       `json:"tokens_out"`
	CostMicroEUR int64     `json:"cost_micro_eur"`
	Calls        int       `json:"calls"`

	// Conversation reports whether the team can talk with its agent while no
	// run is active. ConversationID is the open conversation, if any.
	Conversation       bool  `json:"conversation"`
	ConversationID     int64 `json:"conversation_id,omitempty"`
	ConversationTokens int   `json:"conversation_tokens,omitempty"`
}

// Robot events that wake the agent. Others (obstacle, color) wait in the
// inbox for the next wake-up, so routine readings do not burn tokens.
// skill_done/skill_aborted are left out: the tool call already returns them.
var wakingEvents = map[string]bool{
	protocol.EventTilted: true, protocol.EventButtonA: true, protocol.EventButtonB: true,
	protocol.EventClap: true, protocol.EventBatteryLow: true,
}

// Deps are the services the runtime uses.
type Deps struct {
	Store   *store.Store
	Robots  *robots.Registry
	Hub     *live.Hub
	LLM     *llm.Client
	Tools   *tools.Registry
	Budgets config.Budgets
	Log     *slog.Logger

	// TeamProvider names the model provider for a team's next run
	// (lab.Service.TeamProvider). Nil or "" means the default provider.
	TeamProvider func(ctx context.Context, teamID string) string

	// Conversations reports whether the control room allows conversations
	// (lab.Service.ConversationEnabled). Nil means allowed.
	Conversations func() bool
}

// Manager owns the actors. Safe for concurrent use.
type Manager struct {
	Deps
	ctx    context.Context
	mu     sync.Mutex
	actors map[string]*actor
	talks  map[string][]time.Time // team -> recent conversation messages
}

// NewManager creates the runtime. Call Run before use.
func NewManager(d Deps) *Manager {
	return &Manager{Deps: d, actors: map[string]*actor{}, talks: map[string][]time.Time{}, ctx: context.Background()}
}

// Run closes runs a previous server left open, then routes robot events to
// the agents until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	if err := m.Store.EndOpenRuns(ctx, "server_restart"); err != nil {
		m.Log.Error("close open runs", "err", err)
	}
	ch := make(chan robots.Event, 128)
	m.Robots.Subscribe(ch)
	defer m.Robots.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			kind := ev.Event.Kind
			if kind == protocol.EventSkillDone || kind == protocol.EventSkillAborted {
				continue
			}
			for _, rb := range m.Robots.Snapshot() {
				if rb.ID == ev.RobotID && rb.Owner != "" {
					m.Deliver(rb.Owner, Message{From: SourceRobot, Robot: rb.Alias, Kind: kind, Data: ev.Event.Data, At: ev.At, Wake: wakingEvents[kind]})
				}
			}
		}
	}
}

// Available reports whether a model is configured.
func (m *Manager) Available() bool { return m.LLM != nil && m.LLM.Available() }

func (m *Manager) actor(teamID string, create bool) *actor {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.actors[teamID]
	if a == nil && create {
		a = newActor(m, teamID)
		m.actors[teamID] = a
		go a.loop(m.ctx)
	}
	return a
}

// Status returns the agent status of a team.
func (m *Manager) Status(teamID string) Status {
	s := Status{State: StateWaiting}
	if a := m.actor(teamID, false); a != nil {
		s = a.getStatus()
	}
	s.Conversation = m.ConversationEnabled()
	return s
}

// StartError is a reason why a run cannot start. Its text is German.
type StartError struct{ Msg string }

func (e *StartError) Error() string { return e.Msg }

// Start begins a new run for a team with its newest prompt.
func (m *Manager) Start(ctx context.Context, teamID string) error {
	if !m.Available() {
		return &StartError{"Es ist kein Sprachmodell eingerichtet. Sagt der Lab-Leitung Bescheid."}
	}
	if _, err := m.Store.GetPrompt(ctx, teamID, 0); errors.Is(err, store.ErrNotFound) {
		return &StartError{"Deployt zuerst einen Prompt, dann kann euer Agent loslegen."}
	} else if err != nil {
		return err
	}
	a := m.actor(teamID, true)
	if a.getStatus().Running {
		return &StartError{"Euer Agent läuft schon."}
	}
	a.interrupt() // an answer in the conversation must not hold up the run
	reply := make(chan error, 1)
	a.ctrl <- control{kind: ctrlStart, reply: reply}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop ends a team's run at once: a running model call or tool is cancelled.
// by names who stopped it (team, Leitstand).
func (m *Manager) Stop(teamID, by string) {
	if a := m.actor(teamID, false); a != nil {
		a.interrupt()
		a.ctrl <- control{kind: ctrlStop, reason: "stop", by: by}
	}
}

// StopAll stops every running agent.
func (m *Manager) StopAll(by string) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.actors))
	for id := range m.actors {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id, by)
	}
}

// RemoveTeam ends the team's run and its actor.
func (m *Manager) RemoveTeam(teamID string) {
	m.mu.Lock()
	a := m.actors[teamID]
	delete(m.actors, teamID)
	m.mu.Unlock()
	if a != nil {
		a.interrupt()
		a.ctrl <- control{kind: ctrlRemove}
	}
}

// Deliver puts a message into a team's inbox. It returns false when the team
// has no running agent; such messages are not kept (team messages then go to
// Talk instead).
func (m *Manager) Deliver(teamID string, msg Message) bool {
	a := m.actor(teamID, false)
	if a == nil || !a.getStatus().Running {
		return false
	}
	if msg.At.IsZero() {
		msg.At = time.Now()
	}
	select {
	case a.inbox <- msg:
		return true
	default:
		m.Log.Warn("agent inbox full, message dropped", "team", teamID, "kind", msg.Kind)
		return false
	}
}

// Deployed tells the agent about a new prompt version. Without restart it
// takes effect at the next model call; with restart a new run begins.
func (m *Manager) Deployed(ctx context.Context, teamID string, version int, restart bool) error {
	if restart {
		a := m.actor(teamID, false)
		if a == nil || !a.getStatus().Running {
			return m.Start(ctx, teamID)
		}
		a.interrupt()
		reply := make(chan error, 1)
		a.ctrl <- control{kind: ctrlRestart, by: "Deploy", reply: reply}
		select {
		case err := <-reply:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	data, _ := json.Marshal(map[string]int{"version": version})
	m.Deliver(teamID, Message{From: SourceSystem, Kind: "deploy", Text: "Dein Team hat eine neue Prompt-Version deployt. Sie gilt ab jetzt.", Data: data})
	return nil
}

// runningAgents returns the names of teams with a running agent, except one.
func (m *Manager) runningAgents(ctx context.Context, exceptTeam string) map[string]string {
	teams, err := m.Store.Teams(ctx)
	if err != nil {
		return nil
	}
	out := map[string]string{} // name -> team ID
	for _, t := range teams {
		if t.ID != exceptTeam && m.Status(t.ID).Running {
			out[t.Name] = t.ID
		}
	}
	return out
}

// say delivers a message from one agent to another agent or to all.
func (m *Manager) say(fromTeam, fromName, to, text string) []string {
	others := m.runningAgents(m.ctx, fromTeam)
	var delivered []string
	for name, id := range others {
		if !strings.EqualFold(to, tools.AllRobots) && !strings.EqualFold(to, name) {
			continue
		}
		if m.Deliver(id, Message{From: SourceAgentPrefix + fromName, Kind: "chat", Text: text, Wake: true}) {
			m.Hub.Publish(id, live.LogEntry{Source: SourceAgentPrefix + fromName, Kind: "chat", Text: text})
			delivered = append(delivered, name)
		}
	}
	return delivered
}

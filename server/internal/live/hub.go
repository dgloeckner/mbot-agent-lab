// Package live fans out log entries to the browsers of each team (SSE) and
// counts how many browsers of a team are connected.
package live

import (
	"encoding/json"
	"sync"
	"time"
)

// Log sources shown on the team page.
const (
	SourceRobot  = "robot"
	SourceTeam   = "team"
	SourceSystem = "system"
	SourceAgent  = "agent" // the team's own agent; other agents are "agent:<team name>"
)

// ChannelConversation marks entries of the conversation between a team and
// its agent while no run is active. The team page shows them apart from the
// run protocol.
const ChannelConversation = "conversation"

// LogEntry is one line in a team's live log. Text is only set for free text
// (team messages, system notices); robot events carry Kind and Data and are
// worded by the frontend.
type LogEntry struct {
	ID     int64           `json:"id"`
	At     time.Time       `json:"at"`
	Source string          `json:"source"`
	Kind   string          `json:"kind"`
	Robot  string          `json:"robot,omitempty"` // alias of the robot
	Text   string          `json:"text,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	// Channel is empty for the run protocol, ChannelConversation for the conversation.
	Channel string `json:"channel,omitempty"`
	// Task is the team's active task when the entry was published. A page
	// only ever sees the entries of the task it shows.
	Task string `json:"task_id,omitempty"`

	// Signal is set on notices that are not log entries: they tell the
	// team's pages to reload (SignalTask, SignalTasks). Never in the backlog.
	Signal string `json:"-"`
}

// Signals to the pages of a team.
const (
	SignalTask  = "task"  // the active task changed: reload prompt, notes, log, conversation
	SignalTasks = "tasks" // a task was assigned, withdrawn, accepted or closed: reload the task list
)

// backlogSize is how many entries per team are kept for clients that
// (re)connect. The full log goes to the store once the agent runtime exists.
const backlogSize = 300

// Hub fans log entries out to the SSE connections of each team.
type Hub struct {
	mu      sync.Mutex
	seq     int64
	backlog map[string][]LogEntry
	subs    map[string]map[chan LogEntry]struct{}
	task    map[string]string // active task per team
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{backlog: map[string][]LogEntry{}, subs: map[string]map[chan LogEntry]struct{}{}, task: map[string]string{}}
}

// SetTask records the team's active task. Entries published from now on
// carry it. When it changes, the team's pages get SignalTask.
func (h *Hub) SetTask(team, task string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.task[team] == task {
		return
	}
	h.task[team] = task
	h.signal(team, LogEntry{Signal: SignalTask, Task: task})
}

// Task returns the active task the hub knows for team.
func (h *Hub) Task(team string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.task[team]
}

// Signal tells the team's pages to reload something (SignalTasks).
func (h *Hub) Signal(team, signal string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signal(team, LogEntry{Signal: signal, Task: h.task[team]})
}

func (h *Hub) signal(team string, e LogEntry) {
	for ch := range h.subs[team] {
		select {
		case ch <- e:
		default:
		}
	}
}

// Publish assigns ID, time and the team's active task and delivers e to all
// subscribers of team. Slow subscribers miss entries rather than block; they
// catch up on reconnect.
func (h *Hub) Publish(team string, e LogEntry) LogEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	e.ID = h.seq
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if e.Task == "" {
		e.Task = h.task[team]
	}
	b := append(h.backlog[team], e)
	if len(b) > backlogSize {
		b = b[len(b)-backlogSize:]
	}
	h.backlog[team] = b
	for ch := range h.subs[team] {
		select {
		case ch <- e:
		default:
		}
	}
	return e
}

// Subscribe returns the backlog of the team's active task after afterID and a
// channel for new entries and signals. Call cancel when done.
func (h *Hub) Subscribe(team string, afterID int64) (backlog []LogEntry, ch <-chan LogEntry, cancel func()) {
	c := make(chan LogEntry, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.backlog[team] {
		if e.ID > afterID && e.Task == h.task[team] {
			backlog = append(backlog, e)
		}
	}
	if h.subs[team] == nil {
		h.subs[team] = map[chan LogEntry]struct{}{}
	}
	h.subs[team][c] = struct{}{}
	return backlog, c, func() {
		h.mu.Lock()
		delete(h.subs[team], c)
		h.mu.Unlock()
	}
}

// Connected returns how many live connections a team has open.
func (h *Hub) Connected(team string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[team])
}

// Forget drops the backlog of a deleted team.
func (h *Hub) Forget(team string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.backlog, team)
	delete(h.task, team)
}

package web

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
)

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
}

// backlogSize is how many entries per team are kept for clients that
// (re)connect. The full log goes to the store once the agent runtime exists.
const backlogSize = 300

// Hub fans log entries out to the SSE connections of each team.
type Hub struct {
	mu      sync.Mutex
	seq     int64
	backlog map[string][]LogEntry
	subs    map[string]map[chan LogEntry]struct{}
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{backlog: map[string][]LogEntry{}, subs: map[string]map[chan LogEntry]struct{}{}}
}

// Publish assigns ID and time and delivers e to all subscribers of team.
// Slow subscribers miss entries rather than block; they catch up on reconnect.
func (h *Hub) Publish(team string, e LogEntry) LogEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	e.ID = h.seq
	if e.At.IsZero() {
		e.At = time.Now()
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

// Subscribe returns the backlog after afterID and a channel for new entries.
// Call cancel when done.
func (h *Hub) Subscribe(team string, afterID int64) (backlog []LogEntry, ch <-chan LogEntry, cancel func()) {
	c := make(chan LogEntry, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.backlog[team] {
		if e.ID > afterID {
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

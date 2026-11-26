// Package robots tracks every configured robot: connection status, last state,
// owner and pending commands. It sends the heartbeat and matches results to
// commands by ID.
package robots

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
)

// Bus is the part of the MQTT broker the registry needs.
type Bus interface {
	Publish(topic string, payload []byte, retain bool, qos byte) error
	Handle(filter string, subID int, fn func(topic string, payload []byte)) error
}

// HeartbeatInterval is how often the server sends lab/<id>/hb.
const HeartbeatInterval = time.Second

// Errors returned by Send.
var (
	ErrUnknownRobot = errors.New("unknown robot")
	ErrOffline      = errors.New("robot offline")
	ErrNotAccepted  = errors.New("robot speaks an unsupported protocol version")
	ErrTimeout      = errors.New("no result from robot")
	ErrNoFreshState = errors.New("no fresh state from robot")
)

// Robot is a snapshot of one robot for the UI and for tools like list_robots.
type Robot struct {
	ID        string
	Color     string
	Owner     string // team ID, empty if free
	OwnerName string // team name shown on the robot display
	TeamColor string // LED color while owned
	Alias     string
	Online    bool
	Accepted  bool // hello with a supported protocol version received
	Hello     protocol.Hello
	HelloAt   time.Time // last hello, i.e. last (re)connect of the script
	State     protocol.State
	StateAt   time.Time
	SeenAt    time.Time
	Agent     string // agent state shown on the robot
}

// Event is a robot event together with its origin and arrival time.
type Event struct {
	RobotID string
	Event   protocol.Event
	At      time.Time
}

// Registry is safe for concurrent use.
type Registry struct {
	bus    Bus
	log    *slog.Logger
	cmdSeq atomic.Uint64
	boot   string // random per server start, keeps command IDs unique across restarts

	mu        sync.Mutex
	robots    map[string]*Robot
	pending   map[string]chan protocol.Result
	listeners []chan<- Event
}

// Spec describes a configured robot.
type Spec struct {
	ID, Color string
}

// New creates a registry for the given robots.
func New(bus Bus, specs []Spec, logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	var nonce [3]byte
	_, _ = rand.Read(nonce[:])
	r := &Registry{
		boot:    hex.EncodeToString(nonce[:]),
		bus:     bus,
		log:     logger.With("component", "robots"),
		robots:  map[string]*Robot{},
		pending: map[string]chan protocol.Result{},
	}
	for _, s := range specs {
		r.robots[s.ID] = &Robot{ID: s.ID, Color: s.Color, Agent: protocol.AgentWaiting}
	}
	return r
}

// Owner describes who a robot belongs to. A zero Owner means free.
type Owner struct {
	TeamID   string
	TeamName string
	Color    string
	Alias    string
}

// nextCmdID returns a command ID that is unique across server restarts.
// Robots ignore IDs they have seen recently, so a restarted server must not
// reuse the IDs of the previous one.
func (r *Registry) nextCmdID() string {
	return fmt.Sprintf("c%s-%x", r.boot, r.cmdSeq.Add(1))
}

// Assign sets the owner of a robot. When the owner changes, the robot is
// stopped, a running command ends with reason "reassigned", and the robot
// clears its trip log when the next heartbeat names the new owner.
func (r *Registry) Assign(robotID string, o Owner) error {
	r.mu.Lock()
	rb, ok := r.robots[robotID]
	if !ok {
		r.mu.Unlock()
		return ErrUnknownRobot
	}
	changed := rb.Owner != o.TeamID
	rb.Owner, rb.OwnerName, rb.TeamColor, rb.Alias = o.TeamID, o.TeamName, o.Color, o.Alias
	if changed {
		rb.Agent = protocol.AgentWaiting
		prefix := robotID + "/"
		for key, ch := range r.pending {
			if strings.HasPrefix(key, prefix) {
				ch <- protocol.Result{ID: strings.TrimPrefix(key, prefix), OK: false, Reason: protocol.ReasonReassigned}
				delete(r.pending, key)
			}
		}
	}
	online := rb.Online
	r.mu.Unlock()

	if changed && online {
		// Fire and forget: the robot also stops on its own when the heartbeat
		// shows the new owner, this just makes it immediate.
		cmd, _ := protocol.Marshal(protocol.Cmd{ID: r.nextCmdID(), Name: "stop"})
		if err := r.bus.Publish(protocol.Topic(robotID, protocol.TopicCmd), cmd, false, 0); err != nil {
			r.log.Error("stop on reassign", "robot", robotID, "err", err)
		}
	}
	r.log.Info("robot assigned", "robot", robotID, "team", o.TeamID, "alias", o.Alias)
	return nil
}

// SetAgentState updates the agent state shown on all robots of a team.
func (r *Registry) SetAgentState(teamID, state string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rb := range r.robots {
		if rb.Owner == teamID {
			rb.Agent = state
		}
	}
}

// Subscribe registers a channel that receives every robot event. Slow
// receivers drop events instead of blocking the registry.
func (r *Registry) Subscribe(ch chan<- Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, ch)
}

// Unsubscribe removes a channel registered with Subscribe.
func (r *Registry) Unsubscribe(ch chan<- Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, l := range r.listeners {
		if l == ch {
			r.listeners = append(r.listeners[:i], r.listeners[i+1:]...)
			return
		}
	}
}

// TeamRobots returns the robots owned by a team, sorted by ID.
func (r *Registry) TeamRobots(teamID string) []Robot {
	var out []Robot
	for _, rb := range r.Snapshot() {
		if rb.Owner == teamID && teamID != "" {
			out = append(out, rb)
		}
	}
	return out
}

// Snapshot returns all robots sorted by ID.
func (r *Registry) Snapshot() []Robot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Robot, 0, len(r.robots))
	for _, rb := range r.robots {
		out = append(out, *rb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Run subscribes to robot topics and sends heartbeats until ctx ends.
func (r *Registry) Run(ctx context.Context) error {
	if err := r.bus.Handle(protocol.TopicPrefix+"/+/+", 1, r.handle); err != nil {
		return err
	}
	t := time.NewTicker(HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			r.heartbeat()
		}
	}
}

func (r *Registry) heartbeat() {
	type hb struct {
		id  string
		msg protocol.Heartbeat
	}
	r.mu.Lock()
	beats := make([]hb, 0, len(r.robots))
	for _, rb := range r.robots {
		msg := protocol.Heartbeat{Agent: rb.Agent}
		if rb.Owner != "" {
			msg.Team, msg.Owner, msg.Name, msg.Color = rb.Owner, rb.OwnerName, rb.Alias, rb.TeamColor
		}
		beats = append(beats, hb{rb.ID, msg})
	}
	r.mu.Unlock()
	for _, b := range beats {
		payload, err := protocol.Marshal(b.msg)
		if err == nil {
			err = r.bus.Publish(protocol.Topic(b.id, protocol.TopicHeartbeat), payload, false, 0)
		}
		if err != nil {
			r.log.Error("heartbeat", "robot", b.id, "err", err)
		}
	}
}

func (r *Registry) handle(topic string, payload []byte) {
	id, suffix, ok := protocol.ParseTopic(topic)
	if !ok {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	rb, known := r.robots[id]
	if !known {
		return
	}
	switch suffix {
	case protocol.TopicStatus:
		rb.Online = string(payload) == protocol.StatusOnline
		rb.SeenAt = now
		r.log.Info("robot status", "robot", id, "status", string(payload))
	case protocol.TopicHello:
		var h protocol.Hello
		if err := json.Unmarshal(payload, &h); err != nil {
			r.log.Warn("bad hello", "robot", id, "err", err)
			return
		}
		rb.Hello, rb.HelloAt, rb.SeenAt = h, now, now
		rb.Accepted = h.V == protocol.Version
		if !rb.Accepted {
			r.log.Warn("unsupported protocol version", "robot", id, "v", h.V)
		} else {
			r.log.Info("robot hello", "robot", id, "script", h.Script, "mac", h.MAC, "mem_free", h.MemFree)
		}
	case protocol.TopicState:
		var s protocol.State
		if err := json.Unmarshal(payload, &s); err != nil {
			r.log.Warn("bad state", "robot", id, "err", err)
			return
		}
		rb.State, rb.StateAt, rb.SeenAt = s, now, now
	case protocol.TopicEvent:
		var e protocol.Event
		if err := json.Unmarshal(payload, &e); err != nil {
			r.log.Warn("bad event", "robot", id, "err", err)
			return
		}
		rb.SeenAt = now
		for _, l := range r.listeners {
			select {
			case l <- Event{RobotID: id, Event: e, At: now}:
			default:
			}
		}
	case protocol.TopicResult:
		var res protocol.Result
		if err := json.Unmarshal(payload, &res); err != nil {
			r.log.Warn("bad result", "robot", id, "err", err)
			return
		}
		rb.SeenAt = now
		if ch, ok := r.pending[id+"/"+res.ID]; ok {
			ch <- res // buffered, exactly one result is delivered
			delete(r.pending, id+"/"+res.ID)
		}
	}
}

// FreshState waits until the robot reports a state received after the given
// time and returns the robot with it. A reading from before the last motion
// is never used. It fails after timeout or when ctx ends.
func (r *Registry) FreshState(ctx context.Context, robotID string, after time.Time, timeout time.Duration) (Robot, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		r.mu.Lock()
		rb, ok := r.robots[robotID]
		var snap Robot
		if ok {
			snap = *rb
		}
		r.mu.Unlock()
		switch {
		case !ok:
			return Robot{}, ErrUnknownRobot
		case snap.StateAt.After(after):
			return snap, nil
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			return snap, ErrNoFreshState
		case <-ctx.Done():
			return snap, ctx.Err()
		}
	}
}

// Send publishes a command and waits for its result or the timeout.
// The timeout should be the skill's maximum duration plus two seconds.
func (r *Registry) Send(ctx context.Context, robotID, name string, args map[string]any, timeout time.Duration) (protocol.Result, error) {
	cmd := protocol.Cmd{ID: r.nextCmdID(), Name: name, Args: args}
	payload, err := protocol.Marshal(cmd)
	if err != nil {
		return protocol.Result{}, err
	}
	key := robotID + "/" + cmd.ID
	ch := make(chan protocol.Result, 1)

	r.mu.Lock()
	rb, ok := r.robots[robotID]
	switch {
	case !ok:
		err = ErrUnknownRobot
	case !rb.Online:
		err = ErrOffline
	case !rb.Accepted:
		err = ErrNotAccepted
	default:
		r.pending[key] = ch
	}
	r.mu.Unlock()
	if err != nil {
		return protocol.Result{}, err
	}
	defer func() {
		r.mu.Lock()
		delete(r.pending, key)
		r.mu.Unlock()
	}()

	if err := r.bus.Publish(protocol.Topic(robotID, protocol.TopicCmd), payload, false, 0); err != nil {
		return protocol.Result{}, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res, nil
	case <-timer.C:
		return protocol.Result{ID: cmd.ID, OK: false, Reason: protocol.ReasonTimeout}, ErrTimeout
	case <-ctx.Done():
		return protocol.Result{}, ctx.Err()
	}
}

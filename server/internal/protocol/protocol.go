// Package protocol defines the MQTT topics and messages exchanged between the
// server and the robots. docs/protocol.md is the authoritative description;
// docs/examples/protocol holds the shared example messages used in tests.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Version is the protocol version announced in Hello.V.
const Version = 1

// MaxMessageSize is the upper bound for any payload in bytes.
const MaxMessageSize = 512

// TopicPrefix is the root of all robot topics.
const TopicPrefix = "lab"

// Topic suffixes below lab/<robot_id>/.
const (
	TopicCmd       = "cmd"
	TopicResult    = "result"
	TopicEvent     = "event"
	TopicState     = "state"
	TopicStatus    = "status"
	TopicHello     = "hello"
	TopicHeartbeat = "hb"
)

// Status payloads (plain text, retained, offline is the last will).
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// Topic builds lab/<robotID>/<suffix>.
func Topic(robotID, suffix string) string {
	return TopicPrefix + "/" + robotID + "/" + suffix
}

// ParseTopic splits lab/<robotID>/<suffix>. ok is false for foreign topics.
func ParseTopic(topic string) (robotID, suffix string, ok bool) {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 || parts[0] != TopicPrefix || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// Reason explains why a skill ended.
type Reason string

const (
	ReasonDone       Reason = "done"
	ReasonWall       Reason = "wall"
	ReasonColor      Reason = "color"
	ReasonMaxCM      Reason = "max_cm"
	ReasonTimeout    Reason = "timeout"
	ReasonStopped    Reason = "stopped"
	ReasonObstacle   Reason = "obstacle"
	ReasonBusy       Reason = "busy"
	ReasonTilted     Reason = "tilted"
	ReasonBlocked    Reason = "blocked" // held up: the wheels stand or the nose lifts
	ReasonReassigned Reason = "reassigned"
	ReasonError      Reason = "error"
	ReasonClear      Reason = "clear"     // turn_until_clear found open space
	ReasonMaxDeg     Reason = "max_deg"   // turn_until_clear turned as far as allowed
	ReasonLineLost   Reason = "line_lost" // follow_line_until: the line ended or was lost
	ReasonCrossing   Reason = "crossing"  // follow_line_until reached a crossing
	ReasonEmpty      Reason = "empty"     // retrace_path: the trip log was empty
)

// Cmd is sent from the server to a robot on lab/<id>/cmd.
type Cmd struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// Result answers a Cmd with the same ID on lab/<id>/result.
type Result struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Reason Reason          `json:"reason"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Event kinds.
const (
	EventObstacleNear  = "obstacle_near"
	EventObstacleClear = "obstacle_clear"
	EventColorChanged  = "color_changed"
	EventTilted        = "tilted"
	EventButtonA       = "button_a"
	EventButtonB       = "button_b"
	EventClap          = "clap"
	EventBatteryLow    = "battery_low"
	EventSkillDone     = "skill_done"
	EventSkillAborted  = "skill_aborted"
)

// Event is published by a robot on lab/<id>/event when its state changes.
type Event struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data,omitempty"`
	Seq  uint32          `json:"seq"`
	TMS  uint32          `json:"t_ms"`
}

// State is the retained current robot state on lab/<id>/state.
type State struct {
	Skill      string `json:"skill"`
	DistanceCM int    `json:"distance_cm"`
	Floor      string `json:"floor"` // pattern under the probes L2 L1 R1 R2, e.g. "wkkw"
	Color      string `json:"color"`
	Line       string `json:"line"`
	Yaw        int    `json:"yaw"`
	Battery    int    `json:"battery"`
	Tilted     bool   `json:"tilted"`
	Seq        uint32 `json:"seq"`
	TMS        uint32 `json:"t_ms"`
}

// Hello is published once after each (re)connect on lab/<id>/hello.
type Hello struct {
	V      int    `json:"v"`
	Script string `json:"script"`
	MAC    string `json:"mac"`
	// MemFree is the robot's free heap in bytes right after connecting, -1
	// where unknown (simulator). It shows how much room the script has left.
	MemFree int `json:"mem_free"`
}

// Agent states reported to the robot in the heartbeat.
const (
	AgentWaiting  = "waiting"
	AgentThinking = "thinking"
	AgentActing   = "acting"
)

// Heartbeat is sent once per second on lab/<id>/hb. It also carries the
// current owner and agent state so lost messages heal themselves.
type Heartbeat struct {
	Team  string `json:"team"`  // owning team ID; a change resets the robot
	Owner string `json:"owner"` // team name for the display
	Name  string `json:"name"`  // the team's name for this robot
	Color string `json:"color"` // LED color
	Agent string `json:"agent"`
}

// Marshal encodes v as compact JSON and enforces MaxMessageSize.
func Marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) >= MaxMessageSize {
		return nil, fmt.Errorf("protocol: message of %d bytes exceeds %d", len(b), MaxMessageSize)
	}
	return b, nil
}

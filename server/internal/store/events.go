package store

import (
	"context"
	"encoding/json"
	"time"
)

// Event kinds; docs/events.md lists their scope and data fields.
const (
	EventTeamJoined     = "team.joined"
	EventTaskPublished  = "task.published"
	EventTaskClosed     = "task.closed"
	EventTaskAssigned   = "task.assigned"
	EventTaskAccepted   = "task.accepted"
	EventTaskActivated  = "task.activated"
	EventPromptDeployed = "prompt.deployed"
	EventRunStarted     = "run.started"
	EventRunEnded       = "run.ended"
	EventRunFinished    = "run.finished"
	EventRobotAssigned  = "robot.assigned"
	EventRobotRenamed   = "robot.renamed"
	EventRobotReleased  = "robot.released"
	EventRobotEvent     = "robot.event"
	EventNoteAdded      = "note.added"
	EventNoteDeleted    = "note.deleted"
)

// Event actors: who caused an event.
const (
	ActorAdmin  = "admin"
	ActorTeam   = "team"
	ActorAgent  = "agent"
	ActorRobot  = "robot"
	ActorSystem = "system"
)

// Event is one fact in the event log. Scope fields are empty (or 0) when they
// do not apply; the caller passes the scope it knows and the store never
// guesses. Data is marshalled to JSON and should stay small.
type Event struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Actor  string    `json:"actor"`
	Team   string    `json:"team_id,omitempty"`
	Task   string    `json:"task_id,omitempty"`
	Robot  string    `json:"robot_id,omitempty"`
	Prompt int64     `json:"prompt_id,omitempty"`
	Run    int64     `json:"run_id,omitempty"`
	Data   any       `json:"data,omitempty"`
}

// AddEvent writes an event. It is the only writer of the event log. A (team,
// task) pair that is not assigned is refused by the database.
func (s *Store) AddEvent(ctx context.Context, e Event) error {
	var data any
	if e.Data != nil {
		raw, err := json.Marshal(e.Data)
		if err != nil {
			return err
		}
		data = string(raw)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO events (at, kind, actor, team_id, task_id, robot_id, prompt_id, run_id, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now(), e.Kind, e.Actor, nullString(e.Team), nullString(e.Task), nullString(e.Robot), nullID(e.Prompt), nullID(e.Run), data)
	return err
}

// Events returns the events of one kind ("" for all), oldest first.
func (s *Store) Events(ctx context.Context, kind string) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, at, kind, actor, COALESCE(team_id, ''), COALESCE(task_id, ''),
		COALESCE(robot_id, ''), COALESCE(prompt_id, 0), COALESCE(run_id, 0), COALESCE(data, '')
		FROM events WHERE ? = '' OR kind = ? ORDER BY id`, kind, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var at, data string
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Actor, &e.Team, &e.Task, &e.Robot, &e.Prompt, &e.Run, &data); err != nil {
			return nil, err
		}
		e.At = parseTime(at)
		if data != "" {
			e.Data = json.RawMessage(data)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

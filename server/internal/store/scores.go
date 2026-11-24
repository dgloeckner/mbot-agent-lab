package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Interventions in a run: stops by the team and by-hand calls come from the
// server, "moved by hand" is entered by the lab lead.
type Interventions struct {
	Stops  int `json:"stops"`
	ByHand int `json:"by_hand"`
	Moved  int `json:"moved"`
}

// Total sums the interventions.
func (i Interventions) Total() int { return i.Stops + i.ByHand + i.Moved }

// RunScore is one run of a task with the measures of the challenge.
type RunScore struct {
	Run       int64     `json:"run"`
	TeamID    string    `json:"team_id"`
	Version   int       `json:"version"`
	PromptID  int64     `json:"-"`
	Model     string    `json:"model"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	EndReason string    `json:"end_reason,omitempty"`
	Report    string    `json:"report,omitempty"`
	// Calls: model calls of the run, its cost.
	Calls int `json:"calls"`
	// ToGoalS: seconds from the start to finish; nil without finish.
	ToGoalS *float64 `json:"to_goal_s,omitempty"`
	// Success as the lab lead confirmed it: nil until confirmed.
	Success       *bool         `json:"success"`
	Interventions Interventions `json:"interventions"`
}

// TaskRuns returns every run of a task (not conversations, not by-hand rows),
// oldest first, with its measures.
func (s *Store) TaskRuns(ctx context.Context, task string) ([]RunScore, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id, r.team_id, COALESCE(p.version, 0), COALESCE(r.prompt_id, 0), r.model, r.started_at,
		COALESCE(r.ended_at, ''), COALESCE(r.end_reason, ''), COALESCE(r.report, ''),
		(SELECT count(*) FROM llm_calls l WHERE l.run_id = r.id),
		(SELECT data FROM events e WHERE e.run_id = r.id AND e.kind = 'run.confirmed' ORDER BY e.id DESC LIMIT 1),
		(SELECT count(*) FROM events e WHERE e.run_id = r.id AND e.kind = 'run.intervention'),
		(SELECT count(*) FROM events e WHERE e.run_id = r.id AND e.kind = 'run.ended' AND e.actor = 'team' AND r.end_reason = 'stop')
		FROM runs r LEFT JOIN prompts p ON p.id = r.prompt_id
		WHERE r.task_id = ? AND r.kind = 'run' ORDER BY r.id`, task)
	if err != nil {
		return nil, err
	}
	var out []RunScore
	for rows.Next() {
		var r RunScore
		var started, ended string
		var confirmed sql.NullString
		if err := rows.Scan(&r.Run, &r.TeamID, &r.Version, &r.PromptID, &r.Model, &started, &ended, &r.EndReason, &r.Report,
			&r.Calls, &confirmed, &r.Interventions.Moved, &r.Interventions.Stops); err != nil {
			rows.Close()
			return nil, err
		}
		r.StartedAt, r.EndedAt = parseTime(started), parseTime(ended)
		if r.EndReason == "finish" && !r.EndedAt.IsZero() {
			d := r.EndedAt.Sub(r.StartedAt).Seconds()
			d = float64(int(d*10)) / 10
			r.ToGoalS = &d
		}
		if confirmed.Valid {
			var c struct {
				Success bool `json:"success"`
			}
			if json.Unmarshal([]byte(confirmed.String), &c) == nil {
				r.Success = &c.Success
			}
		}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// By-hand calls count for the run that ended their manual row: the team
	// steered by hand before it started that run.
	mrows, err := s.DB.QueryContext(ctx, `SELECT m.team_id, m.ended_at, (SELECT count(*) FROM tool_calls t WHERE t.run_id = m.id)
		FROM runs m WHERE m.task_id = ? AND m.kind = 'manual' AND m.end_reason = 'run_start'`, task)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var team, ended string
		var n int
		if err := mrows.Scan(&team, &ended, &n); err != nil {
			return nil, err
		}
		at := parseTime(ended)
		// The first run of the team that started at or after the row ended.
		for i := range out {
			if out[i].TeamID == team && !out[i].StartedAt.Before(at.Add(-time.Second)) {
				out[i].Interventions.ByHand += n
				break
			}
		}
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Run returns the task and team of a run, for scoping events; ErrNotFound
// for a conversation, a manual row or an unknown id.
func (s *Store) RunScope(ctx context.Context, id int64) (TeamTask, int64, error) {
	var tt TeamTask
	var prompt sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT team_id, task_id, prompt_id FROM runs WHERE id = ? AND kind = 'run'`, id).Scan(&tt.Team, &tt.Task, &prompt)
	if err == sql.ErrNoRows {
		return tt, 0, ErrNotFound
	}
	return tt, prompt.Int64, err
}

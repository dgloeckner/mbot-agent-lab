package store

import (
	"context"
	"encoding/json"
	"time"
)

// Run is one agent run of a team: from start to finish, limit or stop.
type Run struct {
	ID            int64     `json:"id"`
	TeamID        string    `json:"team_id"`
	PromptVersion int       `json:"prompt_version"`
	Model         string    `json:"model"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at,omitzero"`
	EndReason     string    `json:"end_reason,omitempty"`
	TokensIn      int       `json:"tokens_in"`
	TokensOut     int       `json:"tokens_out"`
	CostMicroEUR  int64     `json:"cost_micro_eur"`
	Report        string    `json:"report,omitempty"`
}

// CreateRun starts a run with the given prompt.
func (s *Store) CreateRun(ctx context.Context, team string, promptID int64, model string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO runs (team_id, prompt_id, model, started_at) VALUES (?, ?, ?, ?)`, team, nullID(promptID), model, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndRun records why and when a run ended, and the agent's report.
func (s *Store) EndRun(ctx context.Context, id int64, reason, report string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET ended_at = ?, end_reason = ?, report = ? WHERE id = ? AND ended_at IS NULL`,
		now(), reason, nullString(report), id)
	return err
}

// EndOpenRuns closes runs left open by a server restart.
func (s *Store) EndOpenRuns(ctx context.Context, reason string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET ended_at = ?, end_reason = ? WHERE ended_at IS NULL`, now(), reason)
	return err
}

// Runs returns the latest runs of a team, newest first.
func (s *Store) Runs(ctx context.Context, team string, limit int) ([]Run, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id, r.team_id, COALESCE(p.version, 0), r.model, r.started_at, COALESCE(r.ended_at, ''),
		COALESCE(r.end_reason, ''), r.tokens_in, r.tokens_out, r.cost_microeur, COALESCE(r.report, '')
		FROM runs r LEFT JOIN prompts p ON p.id = r.prompt_id WHERE r.team_id = ? ORDER BY r.id DESC LIMIT ?`, team, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var started, ended string
		if err := rows.Scan(&r.ID, &r.TeamID, &r.PromptVersion, &r.Model, &started, &ended, &r.EndReason,
			&r.TokensIn, &r.TokensOut, &r.CostMicroEUR, &r.Report); err != nil {
			return nil, err
		}
		r.StartedAt, r.EndedAt = parseTime(started), parseTime(ended)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddMessage logs a message of a run (inbox entry, agent text, system notice).
func (s *Store) AddMessage(ctx context.Context, run int64, seq int, source, kind, text string, data json.RawMessage) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO messages (run_id, seq, source, kind, text, data, at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run, seq, source, kind, text, nullJSON(data), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddToolCall logs one tool call of a run.
func (s *Store) AddToolCall(ctx context.Context, run int64, seq int, name string, args, result json.RawMessage, d time.Duration) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tool_calls (run_id, seq, name, args, result, duration_ms, at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run, seq, name, string(args), nullJSON(result), d.Milliseconds(), now())
	return err
}

// AddLLMCall logs one model call and adds its usage to the run.
func (s *Store) AddLLMCall(ctx context.Context, run int64, provider, model string, in, out int, d time.Duration, costMicroEUR int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO llm_calls (run_id, provider, model, tokens_in, tokens_out, duration_ms, cost_microeur, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run, provider, model, in, out, d.Milliseconds(), costMicroEUR, now()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET tokens_in = tokens_in + ?, tokens_out = tokens_out + ?, cost_microeur = cost_microeur + ? WHERE id = ?`,
		in, out, costMicroEUR, run); err != nil {
		return err
	}
	return tx.Commit()
}

// RunMessage is a stored message, used to replay a run's log.
type RunMessage struct {
	Seq    int    `json:"seq"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
}

// RunMessages returns the messages of a run in order.
func (s *Store) RunMessages(ctx context.Context, run int64) ([]RunMessage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT seq, source, kind, text FROM messages WHERE run_id = ? ORDER BY seq`, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunMessage{}
	for rows.Next() {
		var m RunMessage
		if err := rows.Scan(&m.Seq, &m.Source, &m.Kind, &m.Text); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

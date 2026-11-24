package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Run is one agent run of a team: from start to finish, limit or stop.
type Run struct {
	ID            int64     `json:"id"`
	TeamID        string    `json:"team_id"`
	TaskID        string    `json:"task_id"`
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

// Kinds of rows in runs. A conversation is the talk between a team and its
// agent while no run is active; it never moves a robot.
const (
	KindRun          = "run"
	KindConversation = "conversation"
	// KindManual holds what a team did by hand in a task (console calls,
	// runs of team tools) between two runs. No tokens, no cost.
	KindManual = "manual"
)

// Totals sums the runs and conversations of a team. Tokens and cost include
// the conversations; Conversation shows their share on its own.
type Totals struct {
	Runs         int   `json:"runs"`
	TokensIn     int   `json:"tokens_in"`
	TokensOut    int   `json:"tokens_out"`
	CostMicroEUR int64 `json:"cost_micro_eur"`

	Conversation ConversationTotals `json:"conversation"`
}

// ConversationTotals sums the conversations of a team.
type ConversationTotals struct {
	Count        int   `json:"count"`
	TokensIn     int   `json:"tokens_in"`
	TokensOut    int   `json:"tokens_out"`
	CostMicroEUR int64 `json:"cost_micro_eur"`
}

const totalsQuery = `SELECT team_id,
	COALESCE(SUM(kind = 'run'), 0), COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0), COALESCE(SUM(cost_microeur), 0),
	COALESCE(SUM(kind = 'conversation'), 0),
	COALESCE(SUM(CASE WHEN kind = 'conversation' THEN tokens_in END), 0),
	COALESCE(SUM(CASE WHEN kind = 'conversation' THEN tokens_out END), 0),
	COALESCE(SUM(CASE WHEN kind = 'conversation' THEN cost_microeur END), 0)
	FROM runs`

func scanTotals(sc interface{ Scan(...any) error }, team *sql.NullString, t *Totals) error {
	c := &t.Conversation
	return sc.Scan(team, &t.Runs, &t.TokensIn, &t.TokensOut, &t.CostMicroEUR, &c.Count, &c.TokensIn, &c.TokensOut, &c.CostMicroEUR)
}

// TeamTotals sums all runs and conversations per team, the open ones included.
func (s *Store) TeamTotals(ctx context.Context) (map[string]Totals, error) {
	rows, err := s.DB.QueryContext(ctx, totalsQuery+` GROUP BY team_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Totals{}
	for rows.Next() {
		var team sql.NullString
		var t Totals
		if err := scanTotals(rows, &team, &t); err != nil {
			return nil, err
		}
		out[team.String] = t
	}
	return out, rows.Err()
}

// TeamTotal sums all runs and conversations of one team, the open ones included.
func (s *Store) TeamTotal(ctx context.Context, team string) (Totals, error) {
	var t Totals
	var id sql.NullString
	err := scanTotals(s.DB.QueryRowContext(ctx, totalsQuery+` WHERE team_id = ?`, team), &id, &t)
	return t, err
}

// CreateRun starts a run of the pair with the given prompt (the agent version).
func (s *Store) CreateRun(ctx context.Context, tt TeamTask, promptID int64, model string) (int64, error) {
	return s.create(ctx, KindRun, tt, promptID, model)
}

// CreateConversation starts a conversation. promptID is the prompt the team
// had deployed then (0 for none).
func (s *Store) CreateConversation(ctx context.Context, tt TeamTask, promptID int64, model string) (int64, error) {
	return s.create(ctx, KindConversation, tt, promptID, model)
}

func (s *Store) create(ctx context.Context, kind string, tt TeamTask, promptID int64, model string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO runs (team_id, task_id, kind, prompt_id, model, started_at) VALUES (?, ?, ?, ?, ?, ?)`,
		tt.Team, tt.Task, kind, nullID(promptID), model, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ManualRun returns the open manual row of the pair and creates one if there
// is none. It ends when a run starts or the task is switched or closed.
func (s *Store) ManualRun(ctx context.Context, tt TeamTask) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM runs WHERE team_id = ? AND task_id = ? AND kind = 'manual' AND ended_at IS NULL ORDER BY id DESC LIMIT 1`,
		tt.Team, tt.Task).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.create(ctx, KindManual, tt, 0, "")
	}
	return id, err
}

// EndManualRuns ends the open manual rows of a team (any task).
func (s *Store) EndManualRuns(ctx context.Context, team, reason string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET ended_at = ?, end_reason = ? WHERE team_id = ? AND kind = 'manual' AND ended_at IS NULL`,
		now(), reason, team)
	return err
}

// EndManualRunsOfTask ends the open manual rows of every team in a task.
func (s *Store) EndManualRunsOfTask(ctx context.Context, task, reason string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET ended_at = ?, end_reason = ? WHERE task_id = ? AND kind = 'manual' AND ended_at IS NULL`,
		now(), reason, task)
	return err
}

// NextSeq returns the next free sequence number of a run's protocol
// (messages and tool calls share it).
func (s *Store) NextSeq(ctx context.Context, run int64) (int, error) {
	var seq int
	err := s.DB.QueryRowContext(ctx, `SELECT MAX(COALESCE((SELECT MAX(seq) FROM messages WHERE run_id = ?), 0),
		COALESCE((SELECT MAX(seq) FROM tool_calls WHERE run_id = ?), 0)) + 1`, run, run).Scan(&seq)
	return seq, err
}

// EndRun records why and when a run (or conversation) ended, and the agent's report.
func (s *Store) EndRun(ctx context.Context, id int64, reason, report string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET ended_at = ?, end_reason = ?, report = ? WHERE id = ? AND ended_at IS NULL`,
		now(), reason, nullString(report), id)
	return err
}

// EndOpenRuns closes runs and conversations left open by a server restart.
func (s *Store) EndOpenRuns(ctx context.Context, reason string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET ended_at = ?, end_reason = ? WHERE ended_at IS NULL`, now(), reason)
	return err
}

const runColumns = `r.id, r.team_id, r.task_id, COALESCE(p.version, 0), r.model, r.started_at, COALESCE(r.ended_at, ''),
	COALESCE(r.end_reason, ''), r.tokens_in, r.tokens_out, r.cost_microeur, COALESCE(r.report, '')
	FROM runs r LEFT JOIN prompts p ON p.id = r.prompt_id`

func scanRun(sc interface{ Scan(...any) error }) (Run, error) {
	var r Run
	var started, ended string
	err := sc.Scan(&r.ID, &r.TeamID, &r.TaskID, &r.PromptVersion, &r.Model, &started, &ended, &r.EndReason,
		&r.TokensIn, &r.TokensOut, &r.CostMicroEUR, &r.Report)
	r.StartedAt, r.EndedAt = parseTime(started), parseTime(ended)
	return r, err
}

// Runs returns the latest runs of the pair, newest first. Conversations are left out.
func (s *Store) Runs(ctx context.Context, tt TeamTask, limit int) ([]Run, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+runColumns+` WHERE r.team_id = ? AND r.task_id = ? AND r.kind = 'run' ORDER BY r.id DESC LIMIT ?`, tt.Team, tt.Task, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastRun returns the newest ended run of the pair, or ErrNotFound.
func (s *Store) LastRun(ctx context.Context, tt TeamTask) (Run, error) {
	r, err := scanRun(s.DB.QueryRowContext(ctx, `SELECT `+runColumns+`
		WHERE r.team_id = ? AND r.task_id = ? AND r.kind = 'run' AND r.ended_at IS NOT NULL ORDER BY r.id DESC LIMIT 1`, tt.Team, tt.Task))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return r, err
}

// RobotRef names the robot a message or tool call is about: its device ID and
// what the team called it at that moment. The zero value means no robot.
type RobotRef struct {
	ID    string
	Alias string
}

// AddMessage logs a message of a run (inbox entry, agent text, system notice).
// robot is set for messages from a robot.
func (s *Store) AddMessage(ctx context.Context, run int64, seq int, source, kind, text string, data json.RawMessage, robot RobotRef) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO messages (run_id, seq, source, kind, text, data, robot_id, robot_alias, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run, seq, source, kind, text, nullJSON(data), nullString(robot.ID), nullString(robot.Alias), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddToolCall logs one tool call of a run. robot is set for robot tools.
func (s *Store) AddToolCall(ctx context.Context, run int64, seq int, name string, args, result json.RawMessage, d time.Duration, robot RobotRef) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tool_calls (run_id, seq, name, args, result, duration_ms, robot_id, robot_alias, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run, seq, name, string(args), nullJSON(result), d.Milliseconds(), nullString(robot.ID), nullString(robot.Alias), now())
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

// RunStep is one entry of a run's protocol: a message (Source set) or a tool
// call (Tool set), in the order they happened.
type RunStep struct {
	Seq    int
	Source string
	Kind   string
	Text   string
	Data   json.RawMessage
	Tool   string
	Args   json.RawMessage
	Result json.RawMessage
}

// RunSteps returns the messages and tool calls of a run in order.
func (s *Store) RunSteps(ctx context.Context, run int64) ([]RunStep, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT seq, source, kind, text, COALESCE(data, ''), '', '', '' FROM messages WHERE run_id = ?
		UNION ALL SELECT seq, '', '', '', '', name, args, COALESCE(result, '') FROM tool_calls WHERE run_id = ?
		ORDER BY 1`, run, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunStep{}
	for rows.Next() {
		var st RunStep
		var data, args, result string
		if err := rows.Scan(&st.Seq, &st.Source, &st.Kind, &st.Text, &data, &st.Tool, &args, &result); err != nil {
			return nil, err
		}
		st.Data, st.Args, st.Result = rawOrNil(data), rawOrNil(args), rawOrNil(result)
		out = append(out, st)
	}
	return out, rows.Err()
}

func rawOrNil(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
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

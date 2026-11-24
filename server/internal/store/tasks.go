package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// TeamTask is the key of everything a team produces: prompt versions, notes,
// runs and conversations belong to one (team, task) pair. Keeping both IDs in
// one type means they cannot be swapped by accident.
type TeamTask struct {
	Team string `json:"team_id"`
	Task string `json:"task_id"`
}

// Task states. Only drafts can be edited; published tasks are fixed.
const (
	TaskDraft  = "draft"
	TaskOpen   = "open"
	TaskClosed = "closed"
)

// Task is prepared by the control room and shared by all teams it is
// assigned to. Body is Markdown for the team; it never goes to the agent.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Starter     string    `json:"starter"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at,omitzero"`
	ClosedAt    time.Time `json:"closed_at,omitzero"`
	// AgentTools is AgentToolsAll or AgentToolsTeam.
	AgentTools string `json:"agent_tools"`
}

// Tool sets of a task: the agent gets every tool, or only the team's own
// deployed tools and a few that move no robot.
const (
	AgentToolsAll  = "all"
	AgentToolsTeam = "team_tools"
)

// CreateTask inserts a task. CreatedAt is set by the store.
func (s *Store) CreateTask(ctx context.Context, t Task) (Task, error) {
	at := now()
	var published, closed any
	if t.State == TaskOpen || t.State == TaskClosed {
		published = at
	}
	if t.State == TaskClosed {
		closed = at
	}
	if t.AgentTools == "" {
		t.AgentTools = AgentToolsAll
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tasks (id, title, body, starter, state, created_at, published_at, closed_at, agent_tools) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.Body, t.Starter, t.State, at, published, closed, t.AgentTools)
	if isUnique(err) {
		return Task{}, ErrConflict
	}
	if err != nil {
		return Task{}, err
	}
	return s.Task(ctx, t.ID)
}

const taskColumns = `id, title, body, starter, state, created_at, COALESCE(published_at, ''), COALESCE(closed_at, ''), agent_tools`

func scanTask(sc interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var created, published, closed string
	err := sc.Scan(&t.ID, &t.Title, &t.Body, &t.Starter, &t.State, &created, &published, &closed, &t.AgentTools)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	t.CreatedAt, t.PublishedAt, t.ClosedAt = parseTime(created), parseTime(published), parseTime(closed)
	return t, err
}

// Task returns one task.
func (s *Store) Task(ctx context.Context, id string) (Task, error) {
	return scanTask(s.DB.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
}

// AssignTask gives a task to a team. Assigning twice changes nothing; a
// withdrawn assignment becomes a new one. It reports whether the team got
// the task now.
func (s *Store) AssignTask(ctx context.Context, tt TeamTask) (bool, error) {
	at := now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO team_tasks (team_id, task_id, assigned_at) VALUES (?, ?, ?)
		ON CONFLICT DO UPDATE SET assigned_at = excluded.assigned_at, withdrawn_at = NULL WHERE withdrawn_at IS NOT NULL`,
		tt.Team, tt.Task, at)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// WithdrawTask takes back an assignment the team has not accepted.
// ErrNotFound means there is no such open assignment.
func (s *Store) WithdrawTask(ctx context.Context, tt TeamTask) error {
	return s.affectOne(ctx, `UPDATE team_tasks SET withdrawn_at = ? WHERE team_id = ? AND task_id = ? AND accepted_at IS NULL AND withdrawn_at IS NULL`,
		now(), tt.Team, tt.Task)
}

// AcceptTask records that the team accepted an assigned task. ErrNotFound
// means the task is not assigned to the team.
func (s *Store) AcceptTask(ctx context.Context, tt TeamTask) error {
	return s.affectOne(ctx, `UPDATE team_tasks SET accepted_at = COALESCE(accepted_at, ?) WHERE team_id = ? AND task_id = ? AND withdrawn_at IS NULL`,
		now(), tt.Team, tt.Task)
}

// Assignment states of a task for one team.
const (
	TeamTaskAssigned = "assigned"
	TeamTaskAccepted = "accepted"
	TeamTaskActive   = "active"
)

// TeamTaskInfo is a task as one team has it: the task and how far the team got.
type TeamTaskInfo struct {
	Task
	Status     string    `json:"status"` // assigned | accepted | active
	AssignedAt time.Time `json:"assigned_at"`
	AcceptedAt time.Time `json:"accepted_at,omitzero"`
}

// TeamTasks returns the tasks assigned to a team (withdrawn ones left out),
// in the order they were assigned.
func (s *Store) TeamTasks(ctx context.Context, team string) ([]TeamTaskInfo, error) {
	m, err := s.teamTasks(ctx, ` AND tt.team_id = ?`, team)
	return m[team], err
}

// AllTeamTasks returns TeamTasks for every team, keyed by team ID.
func (s *Store) AllTeamTasks(ctx context.Context) (map[string][]TeamTaskInfo, error) {
	return s.teamTasks(ctx, ``)
}

func (s *Store) teamTasks(ctx context.Context, filter string, args ...any) (map[string][]TeamTaskInfo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tt.team_id, tt.assigned_at, COALESCE(tt.accepted_at, ''), a.task_id IS NOT NULL,
		t.id, t.title, t.body, t.starter, t.state, t.created_at, COALESCE(t.published_at, ''), COALESCE(t.closed_at, ''), t.agent_tools
		FROM team_tasks tt JOIN tasks t ON t.id = tt.task_id
		LEFT JOIN active_tasks a ON a.team_id = tt.team_id AND a.task_id = tt.task_id
		WHERE tt.withdrawn_at IS NULL`+filter+` ORDER BY tt.assigned_at, t.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]TeamTaskInfo{}
	for rows.Next() {
		var i TeamTaskInfo
		var team, assigned, accepted, created, published, closed string
		var active bool
		if err := rows.Scan(&team, &assigned, &accepted, &active, &i.ID, &i.Title, &i.Body, &i.Starter, &i.State, &created, &published, &closed, &i.AgentTools); err != nil {
			return nil, err
		}
		i.AssignedAt, i.AcceptedAt = parseTime(assigned), parseTime(accepted)
		i.CreatedAt, i.PublishedAt, i.ClosedAt = parseTime(created), parseTime(published), parseTime(closed)
		switch {
		case active:
			i.Status = TeamTaskActive
		case accepted != "":
			i.Status = TeamTaskAccepted
		default:
			i.Status = TeamTaskAssigned
		}
		out[team] = append(out[team], i)
	}
	return out, rows.Err()
}

// TaskTeams returns the teams a task is assigned to (withdrawn ones left out).
func (s *Store) TaskTeams(ctx context.Context, task string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT team_id FROM team_tasks WHERE task_id = ? AND withdrawn_at IS NULL ORDER BY team_id`, task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ActiveTasks returns the active task of every team that has one.
func (s *Store) ActiveTasks(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT team_id, task_id FROM active_tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var team, task string
		if err := rows.Scan(&team, &task); err != nil {
			return nil, err
		}
		out[team] = task
	}
	return out, rows.Err()
}

// Tasks returns all tasks, oldest first.
func (s *Store) Tasks(ctx context.Context) ([]Task, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateDraft changes title, body and starter prompt of a draft.
// ErrNotFound means there is no draft with that ID.
func (s *Store) UpdateDraft(ctx context.Context, t Task) error {
	if t.AgentTools == "" {
		t.AgentTools = AgentToolsAll
	}
	return s.affectOne(ctx, `UPDATE tasks SET title = ?, body = ?, starter = ?, agent_tools = ? WHERE id = ? AND state = 'draft'`,
		t.Title, t.Body, t.Starter, t.AgentTools, t.ID)
}

// DeleteDraft removes a draft. ErrNotFound means there is no draft with that ID.
func (s *Store) DeleteDraft(ctx context.Context, id string) error {
	return s.affectOne(ctx, `DELETE FROM tasks WHERE id = ? AND state = 'draft'`, id)
}

// PublishTask turns a draft into an open task. ErrNotFound means there is no
// draft with that ID.
func (s *Store) PublishTask(ctx context.Context, id string) error {
	return s.affectOne(ctx, `UPDATE tasks SET state = 'open', published_at = ? WHERE id = ? AND state = 'draft'`, now(), id)
}

// CloseTask closes an open task. ErrNotFound means there is no open task
// with that ID.
func (s *Store) CloseTask(ctx context.Context, id string) error {
	return s.affectOne(ctx, `UPDATE tasks SET state = 'closed', closed_at = ? WHERE id = ? AND state = 'open'`, now(), id)
}

// ActivateTask makes an assigned task the one the team works in.
func (s *Store) ActivateTask(ctx context.Context, tt TeamTask) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO active_tasks (team_id, task_id) VALUES (?, ?)
		ON CONFLICT (team_id) DO UPDATE SET task_id = excluded.task_id`, tt.Team, tt.Task)
	return err
}

// ActiveTask returns the pair a team works in, or ErrNotFound if the team has
// no active task.
func (s *Store) ActiveTask(ctx context.Context, team string) (TeamTask, error) {
	tt := TeamTask{Team: team}
	err := s.DB.QueryRowContext(ctx, `SELECT task_id FROM active_tasks WHERE team_id = ?`, team).Scan(&tt.Task)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamTask{}, ErrNotFound
	}
	return tt, err
}

// SeeRobot records a robot the lab knows. Calling it again changes nothing.
func (s *Store) SeeRobot(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO robots (id, first_seen_at) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, now())
	return err
}

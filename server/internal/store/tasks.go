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
}

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
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tasks (id, title, body, starter, state, created_at, published_at, closed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.Body, t.Starter, t.State, at, published, closed)
	if isUnique(err) {
		return Task{}, ErrConflict
	}
	if err != nil {
		return Task{}, err
	}
	return s.Task(ctx, t.ID)
}

const taskColumns = `id, title, body, starter, state, created_at, COALESCE(published_at, ''), COALESCE(closed_at, '')`

func scanTask(sc interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var created, published, closed string
	err := sc.Scan(&t.ID, &t.Title, &t.Body, &t.Starter, &t.State, &created, &published, &closed)
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

// AssignTask gives a task to a team. Assigning twice changes nothing.
func (s *Store) AssignTask(ctx context.Context, tt TeamTask) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO team_tasks (team_id, task_id, assigned_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		tt.Team, tt.Task, now())
	return err
}

// AcceptTask records that the team accepted an assigned task. ErrNotFound
// means the task is not assigned to the team.
func (s *Store) AcceptTask(ctx context.Context, tt TeamTask) error {
	return s.affectOne(ctx, `UPDATE team_tasks SET accepted_at = COALESCE(accepted_at, ?) WHERE team_id = ? AND task_id = ?`,
		now(), tt.Team, tt.Task)
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

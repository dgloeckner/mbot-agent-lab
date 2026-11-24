package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned when a row does not exist (or belongs to another team).
var ErrNotFound = errors.New("not found")

// Prompt is one deployed version of a team prompt.
type Prompt struct {
	ID        int64     `json:"-"`
	Version   int       `json:"version"`
	Text      string    `json:"text,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Chars     int       `json:"chars"`
}

// Note is a team note. Author is "agent" or "team".
type Note struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// SavePrompt stores text as the next version of the pair and returns it.
// Versions count per (team, task), from 1.
func (s *Store) SavePrompt(ctx context.Context, tt TeamTask, text string) (Prompt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Prompt{}, err
	}
	defer tx.Rollback()
	var v int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM prompts WHERE team_id = ? AND task_id = ?`, tt.Team, tt.Task).Scan(&v); err != nil {
		return Prompt{}, err
	}
	at := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO prompts (team_id, task_id, version, text, created_at) VALUES (?, ?, ?, ?, ?)`, tt.Team, tt.Task, v, text, at)
	if err != nil {
		return Prompt{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Prompt{}, err
	}
	return Prompt{ID: id, Version: v, Text: text, CreatedAt: parseTime(at), Chars: len([]rune(text))}, nil
}

// ListPrompts returns all versions of the pair, newest first, without text.
func (s *Store) ListPrompts(ctx context.Context, tt TeamTask) ([]Prompt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT version, created_at, length(text) FROM prompts WHERE team_id = ? AND task_id = ? ORDER BY version DESC`, tt.Team, tt.Task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Prompt{}
	for rows.Next() {
		var p Prompt
		var at string
		if err := rows.Scan(&p.Version, &at, &p.Chars); err != nil {
			return nil, err
		}
		p.CreatedAt = parseTime(at)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPrompt returns one version; version 0 means the latest.
func (s *Store) GetPrompt(ctx context.Context, tt TeamTask, version int) (Prompt, error) {
	q := `SELECT id, version, text, created_at FROM prompts WHERE team_id = ? AND task_id = ? AND version = ?`
	args := []any{tt.Team, tt.Task, version}
	if version == 0 {
		q = `SELECT id, version, text, created_at FROM prompts WHERE team_id = ? AND task_id = ? ORDER BY version DESC LIMIT 1`
		args = args[:2]
	}
	var p Prompt
	var at string
	err := s.DB.QueryRowContext(ctx, q, args...).Scan(&p.ID, &p.Version, &p.Text, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return Prompt{}, ErrNotFound
	}
	if err != nil {
		return Prompt{}, err
	}
	p.CreatedAt = parseTime(at)
	p.Chars = len([]rune(p.Text))
	return p, nil
}

// ListNotes returns the notes of the pair, oldest first.
func (s *Store) ListNotes(ctx context.Context, tt TeamTask) ([]Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, text, author, created_at FROM notes WHERE team_id = ? AND task_id = ? ORDER BY id`, tt.Team, tt.Task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		var at string
		if err := rows.Scan(&n.ID, &n.Text, &n.Author, &at); err != nil {
			return nil, err
		}
		n.CreatedAt = parseTime(at)
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddNote creates a note written by the team.
func (s *Store) AddNote(ctx context.Context, tt TeamTask, text, author string) (Note, error) {
	return s.AddNoteFrom(ctx, tt, text, author, 0, 0)
}

// AddNoteFrom creates a note and records which run and message it came from,
// so a note planted by another agent can be traced. Zero IDs mean unknown.
func (s *Store) AddNoteFrom(ctx context.Context, tt TeamTask, text, author string, runID, msgID int64) (Note, error) {
	at := now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO notes (team_id, task_id, text, author, created_at, origin_run_id, origin_msg_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tt.Team, tt.Task, text, author, at, nullID(runID), nullID(msgID))
	if err != nil {
		return Note{}, err
	}
	id, _ := res.LastInsertId()
	return Note{ID: id, Text: text, Author: author, CreatedAt: parseTime(at)}, nil
}

// UpdateNote changes the text of a note of the pair.
func (s *Store) UpdateNote(ctx context.Context, tt TeamTask, id int64, text string) error {
	return s.affectOne(ctx, `UPDATE notes SET text = ? WHERE id = ? AND team_id = ? AND task_id = ?`, text, id, tt.Team, tt.Task)
}

// DeleteNote removes a note of the pair.
func (s *Store) DeleteNote(ctx context.Context, tt TeamTask, id int64) error {
	return s.affectOne(ctx, `DELETE FROM notes WHERE id = ? AND team_id = ? AND task_id = ?`, id, tt.Team, tt.Task)
}

func (s *Store) affectOne(ctx context.Context, q string, args ...any) error {
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

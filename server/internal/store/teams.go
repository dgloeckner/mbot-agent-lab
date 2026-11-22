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

// SavePrompt stores text as the next version for team and returns it.
func (s *Store) SavePrompt(ctx context.Context, team, text string) (Prompt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Prompt{}, err
	}
	defer tx.Rollback()
	var v int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM prompts WHERE team_id = ?`, team).Scan(&v); err != nil {
		return Prompt{}, err
	}
	at := now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO prompts (team_id, version, text, created_at) VALUES (?, ?, ?, ?)`, team, v, text, at); err != nil {
		return Prompt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Prompt{}, err
	}
	return Prompt{Version: v, Text: text, CreatedAt: parseTime(at), Chars: len([]rune(text))}, nil
}

// ListPrompts returns all versions of team, newest first, without text.
func (s *Store) ListPrompts(ctx context.Context, team string) ([]Prompt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT version, created_at, length(text) FROM prompts WHERE team_id = ? ORDER BY version DESC`, team)
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
func (s *Store) GetPrompt(ctx context.Context, team string, version int) (Prompt, error) {
	q := `SELECT version, text, created_at FROM prompts WHERE team_id = ? AND version = ?`
	args := []any{team, version}
	if version == 0 {
		q = `SELECT version, text, created_at FROM prompts WHERE team_id = ? ORDER BY version DESC LIMIT 1`
		args = args[:1]
	}
	var p Prompt
	var at string
	err := s.DB.QueryRowContext(ctx, q, args...).Scan(&p.Version, &p.Text, &at)
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

// ListNotes returns the notes of team, oldest first.
func (s *Store) ListNotes(ctx context.Context, team string) ([]Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, text, author, created_at FROM notes WHERE team_id = ? ORDER BY id`, team)
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

// AddNote creates a note for team.
func (s *Store) AddNote(ctx context.Context, team, text, author string) (Note, error) {
	at := now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO notes (team_id, text, author, created_at) VALUES (?, ?, ?, ?)`, team, text, author, at)
	if err != nil {
		return Note{}, err
	}
	id, _ := res.LastInsertId()
	return Note{ID: id, Text: text, Author: author, CreatedAt: parseTime(at)}, nil
}

// UpdateNote changes the text of a note of team.
func (s *Store) UpdateNote(ctx context.Context, team string, id int64, text string) error {
	return s.affectOne(ctx, `UPDATE notes SET text = ? WHERE id = ? AND team_id = ?`, text, id, team)
}

// DeleteNote removes a note of team.
func (s *Store) DeleteNote(ctx context.Context, team string, id int64) error {
	return s.affectOne(ctx, `DELETE FROM notes WHERE id = ? AND team_id = ?`, id, team)
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

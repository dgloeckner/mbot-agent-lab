package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a row does not exist (or belongs to another team).
var ErrNotFound = errors.New("not found")

// Prompt is one deployed version: the team prompt and, frozen with it, the
// team tools of the pair (prompt_programs).
type Prompt struct {
	ID        int64     `json:"-"`
	Version   int       `json:"version"`
	Text      string    `json:"text,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Chars     int       `json:"chars"`
	// Tools are the team tools that changed against the version before
	// (ListPrompts only).
	Tools []ToolChange `json:"tools,omitempty"`
	// Delta is how many characters of the prompt changed against the
	// version before (ListPrompts only; the first version: all of them).
	Delta int `json:"delta"`
	// Runs counts the runs of this version by how they ended ("" while one
	// runs; ListPrompts only).
	Runs map[string]int `json:"runs,omitempty"`
	// Unchanged: SavePrompt found the same text and team tools in the newest
	// version and created nothing; this is that version.
	Unchanged bool `json:"unchanged,omitempty"`
}

// Changes of a team tool between two versions.
const (
	ToolNew     = "neu"
	ToolChanged = "geändert"
	ToolRemoved = "entfernt"
)

// ToolChange is a team tool that is new, changed or removed in a version.
type ToolChange struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Change string `json:"change"`
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
//
// A deploy with the same text and the same team tools as the newest version
// creates nothing: it answers that version with Unchanged set.
func (s *Store) SavePrompt(ctx context.Context, tt TeamTask, text string) (Prompt, error) {
	if newest, err := s.GetPrompt(ctx, tt, 0); err == nil && newest.Text == text {
		frozen, err := s.VersionPrograms(ctx, newest.ID)
		if err != nil {
			return Prompt{}, err
		}
		cur, err := s.Programs(ctx, tt)
		if err != nil {
			return Prompt{}, err
		}
		if len(DiffTools(frozen, cur)) == 0 {
			newest.Unchanged = true
			return newest, nil
		}
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return Prompt{}, err
	}
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
	// The team tools of the pair belong to this version, valid or not.
	if _, err := tx.ExecContext(ctx, `INSERT INTO prompt_programs (prompt_id, program_id, name, description, blockly, tree, problems)
		SELECT ?, id, name, description, blockly, tree, problems FROM programs WHERE team_id = ? AND task_id = ?`, id, tt.Team, tt.Task); err != nil {
		return Prompt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Prompt{}, err
	}
	return Prompt{ID: id, Version: v, Text: text, CreatedAt: parseTime(at), Chars: len([]rune(text))}, nil
}

// ListPrompts returns all versions of the pair, newest first, without text,
// each with the team tools that changed against the version before.
func (s *Store) ListPrompts(ctx context.Context, tt TeamTask) ([]Prompt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, version, created_at, text FROM prompts WHERE team_id = ? AND task_id = ? ORDER BY version DESC`, tt.Team, tt.Task)
	if err != nil {
		return nil, err
	}
	out := []Prompt{}
	var texts []string
	for rows.Next() {
		var p Prompt
		var at, text string
		if err := rows.Scan(&p.ID, &p.Version, &at, &text); err != nil {
			rows.Close()
			return nil, err
		}
		p.CreatedAt = parseTime(at)
		p.Chars = len([]rune(text))
		out = append(out, p)
		texts = append(texts, text)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Runs of each version by their end.
	runs, err := s.DB.QueryContext(ctx, `SELECT prompt_id, COALESCE(end_reason, ''), COUNT(*) FROM runs
		WHERE team_id = ? AND task_id = ? AND kind = 'run' AND prompt_id IS NOT NULL GROUP BY prompt_id, end_reason`, tt.Team, tt.Task)
	if err != nil {
		return nil, err
	}
	byPrompt := map[int64]map[string]int{}
	for runs.Next() {
		var id int64
		var reason string
		var n int
		if err := runs.Scan(&id, &reason, &n); err != nil {
			runs.Close()
			return nil, err
		}
		if byPrompt[id] == nil {
			byPrompt[id] = map[string]int{}
		}
		byPrompt[id][reason] += n
	}
	runs.Close()
	if err := runs.Err(); err != nil {
		return nil, err
	}
	// Tool changes and prompt delta: each version against the one before (oldest first).
	var prev []Program
	prevText := ""
	for i := len(out) - 1; i >= 0; i-- {
		out[i].Runs = byPrompt[out[i].ID]
		out[i].Delta = ChangeSize(prevText, texts[i])
		prevText = texts[i]
		cur, err := s.VersionPrograms(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Tools = DiffTools(prev, cur)
		prev = cur
	}
	return out, nil
}

// DiffTools lists the team tools that are new, changed (name, description or
// blocks) or removed from one state to the next, matched by id.
func DiffTools(old, cur []Program) []ToolChange {
	before := map[string]Program{}
	for _, p := range old {
		before[p.ID] = p
	}
	var out []ToolChange
	for _, p := range cur {
		o, ok := before[p.ID]
		switch {
		case !ok:
			out = append(out, ToolChange{ID: p.ID, Name: p.Name, Change: ToolNew})
		case o.Name != p.Name || o.Description != p.Description || !sameJSON(o.Blockly, p.Blockly):
			out = append(out, ToolChange{ID: p.ID, Name: p.Name, Change: ToolChanged})
		}
		delete(before, p.ID)
	}
	for _, p := range old {
		if _, gone := before[p.ID]; gone {
			out = append(out, ToolChange{ID: p.ID, Name: p.Name, Change: ToolRemoved})
		}
	}
	return out
}

// ChangeSize is how many characters differ between a and b, counted between
// their common start and end (the page's snapshots count the same way).
func ChangeSize(a, b string) int {
	x, y := []rune(a), []rune(b)
	start := 0
	for start < len(x) && start < len(y) && x[start] == y[start] {
		start++
	}
	end := 0
	for end < len(x)-start && end < len(y)-start && x[len(x)-1-end] == y[len(y)-1-end] {
		end++
	}
	return max(len(x), len(y)) - start - end
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}

// VersionPrograms returns the team tools frozen with a prompt version, by name.
func (s *Store) VersionPrograms(ctx context.Context, promptID int64) ([]Program, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT program_id, name, description, blockly, tree, problems, '' FROM prompt_programs WHERE prompt_id = ? ORDER BY name, program_id`, promptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Program{}
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RestoreVersion replaces the team tools of the pair with those of a
// version, ids included (also tools deleted since). Nothing is deleted from
// the history. It returns the version with its prompt text.
func (s *Store) RestoreVersion(ctx context.Context, tt TeamTask, version int) (Prompt, error) {
	p, err := s.GetPrompt(ctx, tt, version)
	if err != nil {
		return p, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM programs WHERE team_id = ? AND task_id = ?`, tt.Team, tt.Task); err != nil {
		return p, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO programs (id, team_id, task_id, name, description, blockly, tree, problems, updated_at)
		SELECT program_id, ?, ?, name, description, blockly, tree, problems, ? FROM prompt_programs WHERE prompt_id = ?`, tt.Team, tt.Task, now(), p.ID); err != nil {
		return p, err
	}
	return p, tx.Commit()
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

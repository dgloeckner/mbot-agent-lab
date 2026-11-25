package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Program is a team tool of a (team, task) pair: the Blockly state the team
// edits, and the tree and problems the server derived from it.
type Program struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Blockly     json.RawMessage `json:"blockly,omitempty"`
	Tree        json.RawMessage `json:"-"`
	Problems    json.RawMessage `json:"-"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Valid reports whether the derived problems are empty.
func (p Program) Valid() bool {
	var list []any
	return json.Unmarshal(p.Problems, &list) == nil && len(list) == 0
}

const programColumns = `id, name, description, blockly, tree, problems, updated_at FROM programs`

func scanProgram(sc interface{ Scan(...any) error }) (Program, error) {
	var p Program
	var blockly, tree, problems, at string
	err := sc.Scan(&p.ID, &p.Name, &p.Description, &blockly, &tree, &problems, &at)
	p.Blockly, p.Tree, p.Problems, p.UpdatedAt = json.RawMessage(blockly), json.RawMessage(tree), json.RawMessage(problems), parseTime(at)
	return p, err
}

// Programs lists the team tools of the pair by name.
func (s *Store) Programs(ctx context.Context, tt TeamTask) ([]Program, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+programColumns+` WHERE team_id = ? AND task_id = ? ORDER BY name, id`, tt.Team, tt.Task)
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

// GetProgram returns one team tool of the pair, or ErrNotFound.
func (s *Store) GetProgram(ctx context.Context, tt TeamTask, id string) (Program, error) {
	p, err := scanProgram(s.DB.QueryRowContext(ctx, `SELECT `+programColumns+` WHERE team_id = ? AND task_id = ? AND id = ?`, tt.Team, tt.Task, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// CountPrograms counts the team tools of the pair.
func (s *Store) CountPrograms(ctx context.Context, tt TeamTask) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM programs WHERE team_id = ? AND task_id = ?`, tt.Team, tt.Task).Scan(&n)
	return n, err
}

// SaveProgram inserts a team tool or replaces it (last write wins).
// ErrConflict: the pair has another tool of that name. ErrNotFound: an
// update of a tool the pair does not have.
func (s *Store) SaveProgram(ctx context.Context, tt TeamTask, p Program, create bool) (Program, error) {
	p.UpdatedAt = time.Now().UTC()
	at := p.UpdatedAt.Format(time.RFC3339Nano)
	var res sql.Result
	var err error
	if create {
		res, err = s.DB.ExecContext(ctx, `INSERT INTO programs (id, team_id, task_id, name, description, blockly, tree, problems, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, tt.Team, tt.Task, p.Name, p.Description, string(p.Blockly), string(p.Tree), string(p.Problems), at)
	} else {
		res, err = s.DB.ExecContext(ctx, `UPDATE programs SET name = ?, description = ?, blockly = ?, tree = ?, problems = ?, updated_at = ? WHERE team_id = ? AND task_id = ? AND id = ?`,
			p.Name, p.Description, string(p.Blockly), string(p.Tree), string(p.Problems), at, tt.Team, tt.Task, p.ID)
	}
	if isUnique(err) {
		return p, ErrConflict
	}
	if err != nil {
		return p, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return p, ErrNotFound
	}
	return p, nil
}

// DeleteProgram removes a team tool of the pair, or ErrNotFound.
func (s *Store) DeleteProgram(ctx context.Context, tt TeamTask, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM programs WHERE team_id = ? AND task_id = ? AND id = ?`, tt.Team, tt.Task, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CopyPrograms copies every team tool of one pair into another, each with a
// new id from newID. Tools whose name the target has already are skipped.
func (s *Store) CopyPrograms(ctx context.Context, from, to TeamTask, newID func() string) (int, error) {
	list, err := s.Programs(ctx, from)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range list {
		p.ID = newID()
		if _, err := s.SaveProgram(ctx, to, p, true); errors.Is(err, ErrConflict) {
			continue
		} else if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

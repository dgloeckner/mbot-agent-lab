package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrConflict is returned when a unique name is already taken.
var ErrConflict = errors.New("already taken")

// Team is a group of students that joined with the lab code.
type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"-"`
	Color     string    `json:"color"`
	Provider  string    `json:"provider"` // chosen model provider, "" = lab default
	CreatedAt time.Time `json:"created_at"`
}

// Assignment says which team owns a robot and what the team calls it.
type Assignment struct {
	RobotID string `json:"robot_id"`
	TeamID  string `json:"team_id"`
	Alias   string `json:"alias"`
}

const teamOwnerPrefix = "team:"

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// CreateTeam inserts a team. ErrConflict means the name is taken.
func (s *Store) CreateTeam(ctx context.Context, t Team) (Team, error) {
	at := now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO teams (id, name, token, color, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Token, t.Color, at)
	if isUnique(err) {
		return Team{}, ErrConflict
	}
	if err != nil {
		return Team{}, err
	}
	t.CreatedAt = parseTime(at)
	return t, nil
}

func scanTeam(row interface{ Scan(...any) error }) (Team, error) {
	var t Team
	var at string
	err := row.Scan(&t.ID, &t.Name, &t.Token, &t.Color, &t.Provider, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, ErrNotFound
	}
	t.CreatedAt = parseTime(at)
	return t, err
}

// TeamByToken finds the team of a team link.
func (s *Store) TeamByToken(ctx context.Context, token string) (Team, error) {
	return scanTeam(s.DB.QueryRowContext(ctx, `SELECT id, name, token, color, provider, created_at FROM teams WHERE token = ?`, token))
}

// Team returns one team by ID.
func (s *Store) Team(ctx context.Context, id string) (Team, error) {
	return scanTeam(s.DB.QueryRowContext(ctx, `SELECT id, name, token, color, provider, created_at FROM teams WHERE id = ?`, id))
}

// Teams returns all teams in the order they joined.
func (s *Store) Teams(ctx context.Context) ([]Team, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, token, color, provider, created_at FROM teams ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RenameTeam changes a team's name. ErrConflict means the name is taken.
func (s *Store) RenameTeam(ctx context.Context, id, name string) error {
	err := s.affectOne(ctx, `UPDATE teams SET name = ? WHERE id = ?`, name, id)
	if isUnique(err) {
		return ErrConflict
	}
	return err
}

// SetTeamProvider stores the model provider a team chose ("" = lab default).
func (s *Store) SetTeamProvider(ctx context.Context, id, provider string) error {
	return s.affectOne(ctx, `UPDATE teams SET provider = ? WHERE id = ?`, provider, id)
}

// ClearTeamProvider sets every team that chose provider back to the lab
// default and returns their IDs.
func (s *Store) ClearTeamProvider(ctx context.Context, provider string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `UPDATE teams SET provider = '' WHERE provider = ? RETURNING id`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteTeam removes a team and frees its robots. Prompts and notes stay in
// the database for later evaluation but are no longer reachable.
func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM assignments WHERE owner = ?`, teamOwnerPrefix+id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM teams WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// Assignments returns all robots that have an owner.
func (s *Store) Assignments(ctx context.Context) ([]Assignment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT robot_id, owner, alias FROM assignments ORDER BY robot_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		var owner string
		if err := rows.Scan(&a.RobotID, &owner, &a.Alias); err != nil {
			return nil, err
		}
		a.TeamID = strings.TrimPrefix(owner, teamOwnerPrefix)
		out = append(out, a)
	}
	return out, rows.Err()
}

// Assign gives robot to team under alias, replacing any previous owner.
// An empty teamID frees the robot. ErrConflict means the alias is taken in that team.
func (s *Store) Assign(ctx context.Context, robotID, teamID, alias string) error {
	var err error
	if teamID == "" {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM assignments WHERE robot_id = ?`, robotID)
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO assignments (robot_id, owner, alias, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (robot_id) DO UPDATE SET owner = excluded.owner, alias = excluded.alias, updated_at = excluded.updated_at`,
		robotID, teamOwnerPrefix+teamID, alias, now())
	if isUnique(err) {
		return ErrConflict
	}
	return err
}

// Setting returns a stored value, or "" if unset.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting stores a value.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

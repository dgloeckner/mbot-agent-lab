// Package store persists teams, tasks, prompts, runs, messages, tool calls,
// LLM calls, notes and robot assignments in SQLite (pure Go driver, no cgo).
// Everything a team produces is scoped by a (team, task) pair, TeamTask.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps the database handle.
type Store struct {
	DB *sql.DB
}

// Open opens (or creates) the database at path and applies pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer; a single connection avoids "database is locked".
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

// Version returns the applied schema version.
func (s *Store) Version(ctx context.Context) (int, error) {
	var v int
	err := s.DB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

// ErrOldSchema is returned for a database from before the tasks reset. Its
// data is not migrated: the lab starts with a fresh file.
var ErrOldSchema = errors.New("store: the database is from an older version of the lab server and cannot be read; delete the file and start again")

// migrate applies every migrations/NNNN_*.sql file whose number is above user_version.
func (s *Store) migrate(ctx context.Context) error {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	current, err := s.Version(ctx)
	if err != nil {
		return err
	}
	if current > 0 {
		// Before the tasks reset, the schema had no team_tasks table.
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'team_tasks'`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrOldSchema
		}
	}
	for _, f := range files {
		var n int
		name := strings.TrimPrefix(f, "migrations/")
		if _, err := fmt.Sscanf(name, "%04d_", &n); err != nil {
			return fmt.Errorf("store: bad migration name %s", name)
		}
		if n <= current {
			continue
		}
		body, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", n)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

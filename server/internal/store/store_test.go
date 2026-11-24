package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenMigratesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lab.db")
	for i := 0; i < 2; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		v, err := s.Version(ctx)
		if err != nil || v != 2 {
			t.Fatalf("version = %d, %v", v, err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO assignments VALUES ('mbot-1','team:rot','ich','2026-11-22T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

func TestOpenRefusesOldDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lab.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// The schema before the tasks reset: teams and prompts without tasks.
	for _, q := range []string{
		`CREATE TABLE prompts (id INTEGER PRIMARY KEY, team_id TEXT NOT NULL, version INTEGER NOT NULL, text TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE teams (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`PRAGMA user_version = 4`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if _, err := Open(ctx, path); !errors.Is(err, ErrOldSchema) {
		t.Fatalf("old database: want ErrOldSchema, got %v", err)
	}
	if !strings.Contains(ErrOldSchema.Error(), "delete the file") {
		t.Errorf("message must say what to do: %q", ErrOldSchema)
	}
}

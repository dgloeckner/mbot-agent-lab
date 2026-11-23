package store

import (
	"context"
	"path/filepath"
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
		if err != nil || v != 4 {
			t.Fatalf("version = %d, %v", v, err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO assignments VALUES ('mbot-1','team:rot','ich','2026-11-22T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

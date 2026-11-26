package lab

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJoinCodeExpires(t *testing.T) {
	ctx := context.Background()
	s, _ := taskService(t)
	team, _ := s.Join(ctx, "TEST", "Rot")
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	s.joins.now = func() time.Time { return now }

	code, until := s.JoinCode(team.ID)
	if until.Sub(now) != JoinCodeTTL {
		t.Errorf("until = %v", until)
	}
	if got, err := s.TeamByJoinCode(ctx, code); err != nil || got.ID != team.ID {
		t.Fatalf("by code: %+v, %v", got, err)
	}
	// Close to its end a PC gets a new code; the old one still works until it ends.
	now = now.Add(8 * time.Minute)
	fresh, _ := s.JoinCode(team.ID)
	if fresh == code {
		t.Error("a code with two minutes left was shown again")
	}
	now = now.Add(2 * time.Minute)
	if _, err := s.TeamByJoinCode(ctx, code); !errors.Is(err, ErrJoinCode) {
		t.Errorf("expired code: %v", err)
	}
	if _, err := s.TeamByJoinCode(ctx, fresh); err != nil {
		t.Errorf("fresh code: %v", err)
	}
}

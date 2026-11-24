package lab

import (
	"context"
	"errors"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Record writes an event after the fact. A failed write is logged and never
// stops what caused the event.
func (s *Service) Record(ctx context.Context, e store.Event) {
	if err := s.Store.AddEvent(context.WithoutCancel(ctx), e); err != nil {
		s.Log.Error("record event", "kind", e.Kind, "team", e.Team, "task", e.Task, "err", err)
	}
}

// activeTaskID returns the task a team works in, or "" if it has none.
func (s *Service) activeTaskID(ctx context.Context, teamID string) string {
	tt, err := s.Store.ActiveTask(ctx, teamID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.Log.Error("active task", "team", teamID, "err", err)
	}
	return tt.Task
}

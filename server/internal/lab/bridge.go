package lab

import (
	"context"
	"errors"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Temporary bridge until teams accept tasks themselves (#42 removes it):
// every team that joins gets the default task assigned, accepted and active,
// so the team routes, the agent and the end-to-end test keep working.

// DefaultTaskID is the task every joining team works in until #42.
const DefaultTaskID = "default"

func (s *Service) bridgeDefaultTask(ctx context.Context, teamID string) error {
	if _, err := s.Store.Task(ctx, DefaultTaskID); errors.Is(err, store.ErrNotFound) {
		if _, err := s.Store.CreateTask(ctx, store.Task{
			ID: DefaultTaskID, Title: "Freies Arbeiten", State: store.TaskOpen,
			Body: "Probiert aus, was euer Agent mit den Robotern kann.",
		}); err != nil && !errors.Is(err, store.ErrConflict) {
			return err
		}
	} else if err != nil {
		return err
	}
	tt := store.TeamTask{Team: teamID, Task: DefaultTaskID}
	assign := func(ctx context.Context, tt store.TeamTask) error { _, err := s.Store.AssignTask(ctx, tt); return err }
	steps := []struct {
		do   func(context.Context, store.TeamTask) error
		kind string
	}{
		{assign, store.EventTaskAssigned},
		{s.Store.AcceptTask, store.EventTaskAccepted},
		{s.Store.ActivateTask, store.EventTaskActivated},
	}
	for _, st := range steps {
		if err := st.do(ctx, tt); err != nil {
			return err
		}
		s.Record(ctx, store.Event{Kind: st.kind, Actor: store.ActorSystem, Team: tt.Team, Task: tt.Task})
	}
	return nil
}

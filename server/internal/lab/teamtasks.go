package lab

import (
	"context"
	"errors"
	"strings"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Start prompt choices when a team accepts a task.
const (
	PromptStarter = "starter" // the task's starter prompt becomes version 1
	PromptEmpty   = "empty"   // no version yet
	PromptCopy    = "copy"    // the newest prompt of another task of the team becomes version 1
)

// ErrNoTask is returned for a team without an active task.
var ErrNoTask = &UserError{Msg: "Eurem Team ist noch keine Aufgabe zugewiesen.", Conflict: true}

// ErrTaskClosed is returned for a write in a closed task.
var ErrTaskClosed = &UserError{Msg: "Diese Aufgabe ist geschlossen. Ihr könnt alles ansehen, aber nichts mehr ändern.", Conflict: true}

// TeamTasks returns the tasks assigned to a team with their status.
func (s *Service) TeamTasks(ctx context.Context, teamID string) ([]store.TeamTaskInfo, error) {
	tasks, err := s.Store.TeamTasks(ctx, teamID)
	if tasks == nil {
		tasks = []store.TeamTaskInfo{}
	}
	return tasks, err
}

// TeamTask returns one task assigned to the team, with its text.
func (s *Service) TeamTask(ctx context.Context, teamID, taskID string) (store.TeamTaskInfo, error) {
	tasks, err := s.Store.TeamTasks(ctx, teamID)
	if err != nil {
		return store.TeamTaskInfo{}, err
	}
	for _, t := range tasks {
		if t.ID == taskID {
			return t, nil
		}
	}
	return store.TeamTaskInfo{}, ErrUnknownTask
}

// ActiveTask returns the pair the team works in and its task, or ErrNoTask.
func (s *Service) ActiveTask(ctx context.Context, teamID string) (store.TeamTask, store.Task, error) {
	tt, err := s.Store.ActiveTask(ctx, teamID)
	if errors.Is(err, store.ErrNotFound) {
		return tt, store.Task{}, ErrNoTask
	}
	if err != nil {
		return tt, store.Task{}, err
	}
	t, err := s.Store.Task(ctx, tt.Task)
	return tt, t, err
}

// WritableTask returns the active pair if the team may change something in
// it: ErrNoTask without one, ErrTaskClosed in a closed task.
func (s *Service) WritableTask(ctx context.Context, teamID string) (store.TeamTask, error) {
	tt, t, err := s.ActiveTask(ctx, teamID)
	if err != nil {
		return tt, err
	}
	if t.State == store.TaskClosed {
		return tt, ErrTaskClosed
	}
	return tt, nil
}

// AcceptTask accepts an assigned task and makes it the active one. The chosen
// start prompt becomes version 1 of the new pair: the task's starter prompt,
// nothing, or the newest prompt of another task the team accepted (fromTask).
func (s *Service) AcceptTask(ctx context.Context, teamID, taskID, choice, fromTask string) (store.TeamTaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.TeamTask(ctx, teamID, taskID)
	if err != nil {
		return info, err
	}
	switch {
	case info.Status != store.TeamTaskAssigned:
		return info, &UserError{Msg: "Diese Aufgabe habt ihr schon angenommen.", Conflict: true}
	case info.State == store.TaskClosed:
		return info, &UserError{Msg: "Diese Aufgabe ist geschlossen und kann nicht mehr angenommen werden.", Conflict: true}
	case s.RunActive != nil && s.RunActive(teamID):
		return info, &UserError{Msg: "Euer Agent ist gerade in einem Lauf. Haltet ihn erst an.", Conflict: true}
	}

	var text, source string
	switch choice {
	case PromptStarter:
		text, source = info.Starter, PromptStarter
	case PromptEmpty:
	case PromptCopy:
		from, err := s.TeamTask(ctx, teamID, fromTask)
		if err != nil || from.Status == store.TeamTaskAssigned || fromTask == taskID {
			return info, userErr("Wählt eine Aufgabe, die ihr schon angenommen habt.")
		}
		p, err := s.Store.GetPrompt(ctx, store.TeamTask{Team: teamID, Task: fromTask}, 0)
		if errors.Is(err, store.ErrNotFound) {
			return info, userErr("In der Aufgabe „%s“ habt ihr noch keinen Prompt deployt.", from.Title)
		} else if err != nil {
			return info, err
		}
		text, source = p.Text, PromptCopy
	default:
		return info, userErr("Wählt, womit euer Prompt beginnt.")
	}

	tt := store.TeamTask{Team: teamID, Task: taskID}
	if err := s.Store.AcceptTask(ctx, tt); err != nil {
		return info, err
	}
	s.Record(ctx, store.Event{Kind: store.EventTaskAccepted, Actor: store.ActorTeam, Team: teamID, Task: taskID, Data: map[string]string{"prompt": choice}})
	if choice == PromptCopy {
		// The team tools of the other task come along with its prompt.
		n, err := s.Store.CopyPrograms(ctx, store.TeamTask{Team: teamID, Task: fromTask}, tt, func() string { return "p" + randomString(8) })
		if err != nil {
			return info, err
		}
		if n > 0 {
			s.Log.Info("team tools copied", "team", teamID, "from", fromTask, "to", taskID, "count", n)
		}
	}
	if strings.TrimSpace(text) != "" {
		p, err := s.Store.SavePrompt(ctx, tt, text)
		if err != nil {
			return info, err
		}
		data := map[string]any{"version": p.Version, "chars": p.Chars, "restart": false, "source": source}
		if source == PromptCopy {
			data["from_task"] = fromTask
		}
		s.Record(ctx, store.Event{Kind: store.EventPromptDeployed, Actor: store.ActorTeam, Team: teamID, Task: taskID, Prompt: p.ID, Data: data})
	}
	if err := s.activate(ctx, tt); err != nil {
		return info, err
	}
	s.Log.Info("task accepted", "team", teamID, "task", taskID, "prompt", choice)
	s.Hub.Signal(teamID, live.SignalTasks)
	return s.TeamTask(ctx, teamID, taskID)
}

// ActivateTask switches the team to another task it accepted. It is refused
// while a run is active; an open conversation ends.
func (s *Service) ActivateTask(ctx context.Context, teamID, taskID string) (store.TeamTaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.TeamTask(ctx, teamID, taskID)
	if err != nil {
		return info, err
	}
	switch {
	case info.Status == store.TeamTaskActive:
		return info, nil
	case info.Status != store.TeamTaskAccepted:
		return info, &UserError{Msg: "Nehmt die Aufgabe zuerst an.", Conflict: true}
	case s.RunActive != nil && s.RunActive(teamID):
		return info, &UserError{Msg: "Euer Agent ist gerade in einem Lauf. Haltet ihn erst an, dann könnt ihr die Aufgabe wechseln.", Conflict: true}
	}
	if err := s.activate(ctx, store.TeamTask{Team: teamID, Task: taskID}); err != nil {
		return info, err
	}
	s.Log.Info("task activated", "team", teamID, "task", taskID)
	return s.TeamTask(ctx, teamID, taskID)
}

// activate makes tt the active pair and tells the agent and every page of
// the team. The caller holds s.mu.
func (s *Service) activate(ctx context.Context, tt store.TeamTask) error {
	if err := s.Store.ActivateTask(ctx, tt); err != nil {
		return err
	}
	s.Record(ctx, store.Event{Kind: store.EventTaskActivated, Actor: store.ActorTeam, Team: tt.Team, Task: tt.Task})
	if s.OnTaskActivated != nil {
		s.OnTaskActivated(tt.Team, tt.Task)
	}
	s.Hub.SetTask(tt.Team, tt.Task)
	return nil
}

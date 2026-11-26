package lab

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Limits for tasks written in the control room.
const (
	MaxTaskTitle   = 80
	MaxTaskBody    = 20000
	MaxTaskStarter = 20000
)

// ErrUnknownTask is returned for a task ID that does not exist.
var ErrUnknownTask = errors.New("unknown task")

// AllTeams as the target of AssignTask means every team that exists now.
const AllTeams = "all"

// TaskInput is what the control room writes into a draft.
type TaskInput struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Starter string `json:"starter"`
	// AgentTools: "all" (default) or "team_tools".
	AgentTools string `json:"agent_tools"`
}

func (in TaskInput) clean() (TaskInput, error) {
	in.Title = strings.Join(strings.Fields(in.Title), " ")
	if in.AgentTools == "" {
		in.AgentTools = store.AgentToolsAll
	}
	switch {
	case in.AgentTools != store.AgentToolsAll && in.AgentTools != store.AgentToolsTeam:
		return in, userErr("Werkzeuge des Agenten: %q gibt es nicht, nur all oder team_tools.", in.AgentTools)
	case in.Title == "":
		return in, userErr("Gebt der Aufgabe einen Titel.")
	case utf8.RuneCountInString(in.Title) > MaxTaskTitle:
		return in, userErr("Der Titel darf höchstens %d Zeichen haben.", MaxTaskTitle)
	case utf8.RuneCountInString(in.Body) > MaxTaskBody:
		return in, userErr("Der Aufgabentext darf höchstens %d Zeichen haben.", MaxTaskBody)
	case utf8.RuneCountInString(in.Starter) > MaxTaskStarter:
		return in, userErr("Der Startprompt darf höchstens %d Zeichen haben.", MaxTaskStarter)
	}
	return in, nil
}

// Tasks returns every task, oldest first.
func (s *Service) Tasks(ctx context.Context) ([]store.Task, error) {
	return s.Store.Tasks(ctx)
}

// Task returns one task.
func (s *Service) Task(ctx context.Context, id string) (store.Task, error) {
	t, err := s.Store.Task(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return t, ErrUnknownTask
	}
	return t, err
}

// CreateTask starts a new draft.
func (s *Service) CreateTask(ctx context.Context, in TaskInput) (store.Task, error) {
	in, err := in.clean()
	if err != nil {
		return store.Task{}, err
	}
	return s.Store.CreateTask(ctx, store.Task{ID: "a" + randomString(6), Title: in.Title, Body: in.Body, Starter: in.Starter, State: store.TaskDraft, AgentTools: in.AgentTools})
}

// UpdateTask changes a draft. Published tasks are fixed: all teams and the
// results refer to the same text. To change one, duplicate it.
func (s *Service) UpdateTask(ctx context.Context, id string, in TaskInput) (store.Task, error) {
	in, err := in.clean()
	if err != nil {
		return store.Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.draft(ctx, id, "bearbeiten"); err != nil {
		return store.Task{}, err
	}
	if err := s.Store.UpdateDraft(ctx, store.Task{ID: id, Title: in.Title, Body: in.Body, Starter: in.Starter, AgentTools: in.AgentTools}); err != nil {
		return store.Task{}, err
	}
	return s.Store.Task(ctx, id)
}

// DeleteTask removes a draft. Published tasks stay for the results.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.draft(ctx, id, "löschen"); err != nil {
		return err
	}
	return s.Store.DeleteDraft(ctx, id)
}

// draft returns nil if id is a draft, and otherwise the German reason why
// the control room cannot do what (bearbeiten, löschen, veröffentlichen).
func (s *Service) draft(ctx context.Context, id, what string) error {
	t, err := s.Task(ctx, id)
	if err != nil {
		return err
	}
	if t.State != store.TaskDraft {
		return &UserError{Msg: "Nur Entwürfe kann man " + what + ". Diese Aufgabe ist schon veröffentlicht; dupliziert sie, um sie zu ändern.", Conflict: true}
	}
	return nil
}

// PublishTask fixes a draft's title, text and starter prompt and makes it
// assignable.
func (s *Service) PublishTask(ctx context.Context, id string) (store.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.draft(ctx, id, "veröffentlichen"); err != nil {
		return store.Task{}, err
	}
	t, _ := s.Store.Task(ctx, id)
	if strings.TrimSpace(t.Body) == "" {
		return store.Task{}, userErr("Die Aufgabe hat noch keinen Text. Schreibt zuerst, was die Teams tun sollen.")
	}
	if err := s.Store.PublishTask(ctx, id); err != nil {
		return store.Task{}, err
	}
	s.Record(ctx, store.Event{Kind: store.EventTaskPublished, Actor: store.ActorAdmin, Task: id, Data: map[string]string{"title": t.Title}})
	s.Log.Info("task published", "task", id, "title", t.Title)
	return s.Store.Task(ctx, id)
}

// CloseTask ends a task for every team: no more deploys, runs or
// conversations in it. Running runs and open conversations in it end.
// Prompts, notes and runs stay readable, and a team whose active task it is
// keeps it selected.
func (s *Service) CloseTask(ctx context.Context, id string) (store.Task, error) {
	s.mu.Lock()
	t, err := s.Task(ctx, id)
	if err != nil {
		s.mu.Unlock()
		return store.Task{}, err
	}
	if t.State != store.TaskOpen {
		s.mu.Unlock()
		if t.State == store.TaskClosed {
			return t, nil
		}
		return store.Task{}, &UserError{Msg: "Einen Entwurf kann man nicht schließen. Löscht ihn, wenn ihr ihn nicht braucht.", Conflict: true}
	}
	if err := s.Store.CloseTask(ctx, id); err != nil {
		s.mu.Unlock()
		return store.Task{}, err
	}
	s.Record(ctx, store.Event{Kind: store.EventTaskClosed, Actor: store.ActorAdmin, Task: id})
	teams, err := s.Store.TaskTeams(ctx, id)
	if err != nil {
		s.Log.Error("teams of a closed task", "task", id, "err", err)
	}
	for _, team := range teams {
		s.Hub.Signal(team, live.SignalTasks)
		// A line in the log: the page shows it as a notice on every tab.
		data, _ := json.Marshal(map[string]string{"task": id, "title": t.Title})
		s.Hub.Publish(team, live.LogEntry{Source: live.SourceSystem, Kind: "task_closed", Text: "Die Lab-Leitung hat die Aufgabe „" + t.Title + "“ geschlossen.", Data: data})
	}
	s.mu.Unlock()
	s.Log.Info("task closed", "task", id)
	if s.OnTaskClosed != nil {
		s.OnTaskClosed(id)
	}
	return s.Store.Task(ctx, id)
}

// DuplicateTask creates a new draft with the text of any task.
func (s *Service) DuplicateTask(ctx context.Context, id string) (store.Task, error) {
	t, err := s.Task(ctx, id)
	if err != nil {
		return store.Task{}, err
	}
	title := t.Title + " (Kopie)"
	if utf8.RuneCountInString(title) > MaxTaskTitle {
		title = t.Title
	}
	return s.Store.CreateTask(ctx, store.Task{ID: "a" + randomString(6), Title: title, Body: t.Body, Starter: t.Starter, State: store.TaskDraft, AgentTools: t.AgentTools})
}

// AssignTask gives an open task to teams, or to every current team with
// AllTeams. Teams that join later get nothing automatically. Assigning twice
// changes nothing. It returns the IDs of the teams that got the task now.
func (s *Service) AssignTask(ctx context.Context, id string, teams []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.Task(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.State != store.TaskOpen {
		msg := "Veröffentlicht die Aufgabe zuerst, dann könnt ihr sie zuweisen."
		if t.State == store.TaskClosed {
			msg = "Diese Aufgabe ist geschlossen und kann nicht mehr zugewiesen werden."
		}
		return nil, &UserError{Msg: msg, Conflict: true}
	}
	existing, err := s.teamMap(ctx)
	if err != nil {
		return nil, err
	}
	if slices.Contains(teams, AllTeams) {
		teams = teams[:0:0]
		all, err := s.Store.Teams(ctx)
		if err != nil {
			return nil, err
		}
		for _, tm := range all {
			teams = append(teams, tm.ID)
		}
	}
	for _, team := range teams {
		if _, ok := existing[team]; !ok {
			return nil, ErrUnknownTeam
		}
	}
	got := []string{}
	for _, team := range teams {
		tt := store.TeamTask{Team: team, Task: id}
		added, err := s.Store.AssignTask(ctx, tt)
		if err != nil {
			return got, err
		}
		if !added {
			continue
		}
		got = append(got, team)
		s.Record(ctx, store.Event{Kind: store.EventTaskAssigned, Actor: store.ActorAdmin, Team: team, Task: id})
		s.notify(team, "task_assigned", "Ihr habt eine neue Aufgabe bekommen: „"+t.Title+"“.", map[string]string{"task": id, "title": t.Title})
		s.Hub.Signal(team, live.SignalTasks)
	}
	if len(got) > 0 {
		s.Log.Info("task assigned", "task", id, "teams", got)
	}
	return got, nil
}

// WithdrawTask takes a task back from a team that has not accepted it yet.
func (s *Service) WithdrawTask(ctx context.Context, id, team string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Task(ctx, id); err != nil {
		return err
	}
	tt := store.TeamTask{Team: team, Task: id}
	if err := s.Store.WithdrawTask(ctx, tt); errors.Is(err, store.ErrNotFound) {
		tasks, err := s.Store.TeamTasks(ctx, team)
		if err != nil {
			return err
		}
		for _, info := range tasks {
			if info.ID == id {
				return &UserError{Msg: "Das Team hat die Aufgabe schon angenommen. Zurückziehen geht nur vorher.", Conflict: true}
			}
		}
		return &UserError{Msg: "Diese Aufgabe ist dem Team nicht zugewiesen.", Conflict: true}
	} else if err != nil {
		return err
	}
	s.Record(ctx, store.Event{Kind: store.EventTaskWithdrawn, Actor: store.ActorAdmin, Team: team, Task: id})
	s.notify(team, "task_withdrawn", "Die Lab-Leitung hat eine Aufgabe zurückgenommen.", map[string]string{"task": id})
	s.Hub.Signal(team, live.SignalTasks)
	return nil
}

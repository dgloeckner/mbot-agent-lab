package lab

import (
	"context"
	"errors"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// openTask creates and publishes a task and assigns it to teams.
func openTask(t *testing.T, s *Service, title, starter string, teams ...string) store.Task {
	t.Helper()
	ctx := context.Background()
	task, err := s.CreateTask(ctx, TaskInput{Title: title, Body: "Text von " + title, Starter: starter})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = s.PublishTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignTask(ctx, task.ID, teams); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestJoinGivesNoTask(t *testing.T) {
	ctx := context.Background()
	s, _ := taskService(t)
	team, _ := s.Join(ctx, "TEST", "Rot")
	if _, _, err := s.ActiveTask(ctx, team.ID); !errors.Is(err, ErrNoTask) {
		t.Errorf("a new team has a task: %v", err)
	}
	if _, err := s.WritableTask(ctx, team.ID); !errors.Is(err, ErrNoTask) {
		t.Errorf("writable without task: %v", err)
	}
	if tasks, _ := s.TeamTasks(ctx, team.ID); len(tasks) != 0 {
		t.Errorf("tasks = %+v", tasks)
	}
}

func TestAcceptWithEachPromptChoice(t *testing.T) {
	ctx := context.Background()
	s, st := taskService(t)
	team, _ := s.Join(ctx, "TEST", "Rot")
	a := openTask(t, s, "A", "Starte bei Rot.", team.ID)
	b := openTask(t, s, "B", "", team.ID)
	c := openTask(t, s, "C", "Fahr los.", team.ID)
	d := openTask(t, s, "D", "Fahr los.", team.ID)

	// starter: the starter prompt becomes version 1.
	info, err := s.AcceptTask(ctx, team.ID, a.ID, PromptStarter, "")
	if err != nil || info.Status != store.TeamTaskActive {
		t.Fatalf("accept a = %+v, %v", info, err)
	}
	pa := store.TeamTask{Team: team.ID, Task: a.ID}
	if p, err := st.GetPrompt(ctx, pa, 0); err != nil || p.Version != 1 || p.Text != "Starte bei Rot." {
		t.Errorf("prompt of a = %+v, %v", p, err)
	}
	_, _ = st.SavePrompt(ctx, pa, "Version 2 von A")
	if _, err := s.AcceptTask(ctx, team.ID, a.ID, PromptStarter, ""); err == nil {
		t.Error("accepted twice")
	}

	// starter without a starter prompt and empty: no version yet.
	for _, task := range []store.Task{b, c} {
		choice := PromptStarter
		if task.ID == c.ID {
			choice = PromptEmpty
		}
		if _, err := s.AcceptTask(ctx, team.ID, task.ID, choice, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := st.GetPrompt(ctx, store.TeamTask{Team: team.ID, Task: task.ID}, 0); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s: a version after %s: %v", task.Title, choice, err)
		}
	}

	// copy: the newest prompt of another accepted task; only from accepted tasks with a prompt.
	if _, err := s.AcceptTask(ctx, team.ID, d.ID, PromptCopy, b.ID); err == nil {
		t.Error("copy from a task without prompt accepted")
	}
	if _, err := s.AcceptTask(ctx, team.ID, d.ID, PromptCopy, "nope"); err == nil {
		t.Error("copy from an unknown task accepted")
	}
	if _, err := s.AcceptTask(ctx, team.ID, d.ID, "irgendwas", ""); err == nil {
		t.Error("unknown prompt choice accepted")
	}
	if _, err := s.AcceptTask(ctx, team.ID, d.ID, PromptCopy, a.ID); err != nil {
		t.Fatal(err)
	}
	if p, _ := st.GetPrompt(ctx, store.TeamTask{Team: team.ID, Task: d.ID}, 0); p.Version != 1 || p.Text != "Version 2 von A" {
		t.Errorf("copied prompt = %+v", p)
	}
	if tt, _, _ := s.ActiveTask(ctx, team.ID); tt.Task != d.ID {
		t.Errorf("active after accepting d = %+v", tt)
	}

	deployed, _ := st.Events(ctx, store.EventPromptDeployed)
	if len(deployed) != 2 || deployed[0].Task != a.ID || deployed[1].Task != d.ID {
		t.Errorf("prompt.deployed events = %+v", deployed)
	}
}

func TestAcceptRefusals(t *testing.T) {
	ctx := context.Background()
	s, _ := taskService(t)
	rot, _ := s.Join(ctx, "TEST", "Rot")
	blau, _ := s.Join(ctx, "TEST", "Blau")
	other := openTask(t, s, "Nur Blau", "", blau.ID)
	if _, err := s.AcceptTask(ctx, rot.ID, other.ID, PromptEmpty, ""); !errors.Is(err, ErrUnknownTask) {
		t.Errorf("accepting a task that is not assigned: %v", err)
	}
	withdrawn := openTask(t, s, "Zurück", "", rot.ID)
	_ = s.WithdrawTask(ctx, withdrawn.ID, rot.ID)
	if _, err := s.AcceptTask(ctx, rot.ID, withdrawn.ID, PromptEmpty, ""); !errors.Is(err, ErrUnknownTask) {
		t.Errorf("accepting a withdrawn task: %v", err)
	}
	closed := openTask(t, s, "Zu", "", rot.ID)
	_, _ = s.CloseTask(ctx, closed.ID)
	if _, err := s.AcceptTask(ctx, rot.ID, closed.ID, PromptEmpty, ""); !conflict(err) {
		t.Errorf("accepting a closed task: %v", err)
	}
}

func TestSwitchTasks(t *testing.T) {
	ctx := context.Background()
	s, st := taskService(t)
	running := false
	s.RunActive = func(string) bool { return running }
	var activated []string
	s.OnTaskActivated = func(team, task string) { activated = append(activated, task) }
	team, _ := s.Join(ctx, "TEST", "Rot")
	a := openTask(t, s, "A", "", team.ID)
	b := openTask(t, s, "B", "", team.ID)
	c := openTask(t, s, "C", "", team.ID)
	_, _ = s.AcceptTask(ctx, team.ID, a.ID, PromptEmpty, "")
	_, _ = s.AcceptTask(ctx, team.ID, b.ID, PromptEmpty, "")

	if _, err := s.ActivateTask(ctx, team.ID, c.ID); !conflict(err) {
		t.Errorf("switching to a task not accepted: %v", err)
	}
	if info, err := s.ActivateTask(ctx, team.ID, a.ID); err != nil || info.Status != store.TeamTaskActive {
		t.Fatalf("switch to a = %+v, %v", info, err)
	}
	if s.Hub.Task(team.ID) != a.ID {
		t.Errorf("hub task = %q", s.Hub.Task(team.ID))
	}
	running = true
	if _, err := s.ActivateTask(ctx, team.ID, b.ID); !conflict(err) {
		t.Errorf("switch during a run: %v", err)
	}
	if _, err := s.AcceptTask(ctx, team.ID, c.ID, PromptEmpty, ""); !conflict(err) {
		t.Errorf("accept during a run: %v", err)
	}
	running = false
	if _, err := s.ActivateTask(ctx, team.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if tt, _ := st.ActiveTask(ctx, team.ID); tt.Task != b.ID {
		t.Errorf("active = %+v", tt)
	}
	if want := []string{a.ID, b.ID, a.ID, b.ID}; len(activated) != len(want) {
		t.Errorf("OnTaskActivated = %v, want %v", activated, want)
	}

	// A closed task stays selectable but is read-only.
	_, _ = s.CloseTask(ctx, b.ID)
	if _, _, err := s.ActiveTask(ctx, team.ID); err != nil {
		t.Errorf("closed active task: %v", err)
	}
	if _, err := s.WritableTask(ctx, team.ID); !errors.Is(err, ErrTaskClosed) {
		t.Errorf("writable in a closed task: %v", err)
	}
	if _, err := s.ActivateTask(ctx, team.ID, a.ID); err != nil {
		t.Errorf("switching away from a closed task: %v", err)
	}
}

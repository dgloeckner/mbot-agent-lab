package lab

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func taskService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return newService(t, st, nil), st
}

func conflict(err error) bool {
	var ue *UserError
	return errors.As(err, &ue) && ue.Conflict
}

func TestTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	s, st := taskService(t)
	if _, err := s.CreateTask(ctx, TaskInput{Title: "  "}); err == nil {
		t.Error("task without title accepted")
	}
	task, err := s.CreateTask(ctx, TaskInput{Title: " Labyrinth  lösen ", Body: "Findet den Ausgang.", Starter: "Fahr los."})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != store.TaskDraft || task.Title != "Labyrinth lösen" || len(task.ID) != 7 || task.ID[0] != 'a' {
		t.Fatalf("new task = %+v", task)
	}
	task, err = s.UpdateTask(ctx, task.ID, TaskInput{Title: "Labyrinth", Body: "Findet den Ausgang.", Starter: "Fahr vorsichtig los."})
	if err != nil || task.Starter != "Fahr vorsichtig los." {
		t.Fatalf("update draft = %+v, %v", task, err)
	}
	if _, err := s.AssignTask(ctx, task.ID, []string{AllTeams}); !conflict(err) {
		t.Errorf("assigning a draft: %v", err)
	}

	task, err = s.PublishTask(ctx, task.ID)
	if err != nil || task.State != store.TaskOpen || task.PublishedAt.IsZero() {
		t.Fatalf("publish = %+v, %v", task, err)
	}
	// Published tasks are fixed.
	if _, err := s.UpdateTask(ctx, task.ID, TaskInput{Title: "Anders"}); !conflict(err) {
		t.Errorf("editing a published task: %v", err)
	}
	if err := s.DeleteTask(ctx, task.ID); !conflict(err) {
		t.Errorf("deleting a published task: %v", err)
	}
	if _, err := s.PublishTask(ctx, task.ID); !conflict(err) {
		t.Errorf("publishing twice: %v", err)
	}

	task, err = s.CloseTask(ctx, task.ID)
	if err != nil || task.State != store.TaskClosed || task.ClosedAt.IsZero() {
		t.Fatalf("close = %+v, %v", task, err)
	}
	if _, err := s.AssignTask(ctx, task.ID, []string{AllTeams}); !conflict(err) {
		t.Errorf("assigning a closed task: %v", err)
	}

	// Duplicate works from any state and gives a new draft with the same text.
	dup, err := s.DuplicateTask(ctx, task.ID)
	if err != nil || dup.State != store.TaskDraft || dup.ID == task.ID || dup.Body != task.Body || dup.Starter != task.Starter || dup.Title != "Labyrinth (Kopie)" {
		t.Fatalf("duplicate = %+v, %v", dup, err)
	}
	if err := s.DeleteTask(ctx, dup.ID); err != nil {
		t.Errorf("deleting a draft: %v", err)
	}
	if _, err := s.Task(ctx, dup.ID); !errors.Is(err, ErrUnknownTask) {
		t.Errorf("deleted draft still there: %v", err)
	}
	if _, err := s.PublishTask(ctx, "nope"); !errors.Is(err, ErrUnknownTask) {
		t.Errorf("unknown task: %v", err)
	}

	empty, _ := s.CreateTask(ctx, TaskInput{Title: "Leer"})
	if _, err := s.PublishTask(ctx, empty.ID); err == nil {
		t.Error("published a task without text")
	}

	var kinds []string
	evs, _ := st.Events(ctx, "")
	for _, e := range evs {
		if e.Task == task.ID {
			kinds = append(kinds, e.Kind+"/"+e.Actor)
		}
	}
	if want := []string{"task.published/admin", "task.closed/admin"}; !slices.Equal(kinds, want) {
		t.Errorf("events = %v, want %v", kinds, want)
	}
}

func TestAssignAndWithdraw(t *testing.T) {
	ctx := context.Background()
	s, st := taskService(t)
	rot, _ := s.Join(ctx, "TEST", "Rot")
	blau, _ := s.Join(ctx, "TEST", "Blau")
	task, _ := s.CreateTask(ctx, TaskInput{Title: "Rennen", Body: "Fahrt im Kreis."})
	task, _ = s.PublishTask(ctx, task.ID)

	got, err := s.AssignTask(ctx, task.ID, []string{rot.ID})
	if err != nil || !slices.Equal(got, []string{rot.ID}) {
		t.Fatalf("assign rot = %v, %v", got, err)
	}
	// "all" means every current team; rot already has it.
	got, err = s.AssignTask(ctx, task.ID, []string{AllTeams})
	if err != nil || !slices.Equal(got, []string{blau.ID}) {
		t.Fatalf("assign all = %v, %v", got, err)
	}
	if got, _ := s.AssignTask(ctx, task.ID, []string{AllTeams}); len(got) != 0 {
		t.Errorf("assigning again is idempotent, got %v", got)
	}
	if _, err := s.AssignTask(ctx, task.ID, []string{"nope"}); !errors.Is(err, ErrUnknownTeam) {
		t.Errorf("unknown team: %v", err)
	}
	// Teams that join later get nothing automatically.
	gruen, _ := s.Join(ctx, "TEST", "Grün")
	if tasks, _ := st.TeamTasks(ctx, gruen.ID); slices.ContainsFunc(tasks, func(i store.TeamTaskInfo) bool { return i.ID == task.ID }) {
		t.Error("a team that joined later got the task")
	}

	// Withdraw while not accepted; afterwards the team no longer has it.
	if err := s.WithdrawTask(ctx, task.ID, blau.ID); err != nil {
		t.Fatal(err)
	}
	if tasks, _ := st.TeamTasks(ctx, blau.ID); slices.ContainsFunc(tasks, func(i store.TeamTaskInfo) bool { return i.ID == task.ID }) {
		t.Error("withdrawn task still listed")
	}
	if err := s.WithdrawTask(ctx, task.ID, blau.ID); !conflict(err) {
		t.Errorf("withdrawing twice: %v", err)
	}
	// Assigning again after a withdrawal is a new assignment.
	if got, _ := s.AssignTask(ctx, task.ID, []string{blau.ID}); !slices.Equal(got, []string{blau.ID}) {
		t.Errorf("reassign after withdrawal = %v", got)
	}

	// Withdraw after accept is refused.
	if err := st.AcceptTask(ctx, store.TeamTask{Team: rot.ID, Task: task.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.WithdrawTask(ctx, task.ID, rot.ID); !conflict(err) {
		t.Errorf("withdraw after accept: %v", err)
	}

	var kinds []string
	evs, _ := st.Events(ctx, "")
	for _, e := range evs {
		if e.Task == task.ID && e.Team != "" {
			kinds = append(kinds, e.Kind+"/"+e.Team)
		}
	}
	want := []string{"task.assigned/" + rot.ID, "task.assigned/" + blau.ID, "task.withdrawn/" + blau.ID, "task.assigned/" + blau.ID}
	if !slices.Equal(kinds, want) {
		t.Errorf("events = %v, want %v", kinds, want)
	}
}

func TestCloseTaskEndsItsRuns(t *testing.T) {
	ctx := context.Background()
	s, _ := taskService(t)
	var closed []string
	s.OnTaskClosed = func(id string) { closed = append(closed, id) }
	task, _ := s.CreateTask(ctx, TaskInput{Title: "A", Body: "Text"})
	if _, err := s.CloseTask(ctx, task.ID); !conflict(err) {
		t.Errorf("closing a draft: %v", err)
	}
	_, _ = s.PublishTask(ctx, task.ID)
	if _, err := s.CloseTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseTask(ctx, task.ID); err != nil {
		t.Errorf("closing twice: %v", err)
	}
	if !slices.Equal(closed, []string{task.ID}) {
		t.Errorf("OnTaskClosed calls = %v", closed)
	}
}

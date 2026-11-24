package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// pair creates team and task as needed and assigns the task to the team.
func pair(t *testing.T, s *Store, team, task string) TeamTask {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Team(ctx, team); errors.Is(err, ErrNotFound) {
		if _, err := s.CreateTeam(ctx, Team{ID: team, Name: team, Token: "tok-" + team, Color: "red"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Task(ctx, task); errors.Is(err, ErrNotFound) {
		if _, err := s.CreateTask(ctx, Task{ID: task, Title: task, Body: "Text", State: TaskOpen}); err != nil {
			t.Fatal(err)
		}
	}
	tt := TeamTask{Team: team, Task: task}
	if _, err := s.AssignTask(ctx, tt); err != nil {
		t.Fatal(err)
	}
	return tt
}

func TestPromptVersions(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	rot, blau := pair(t, s, "rot", "a1"), pair(t, s, "blau", "a1")
	if _, err := s.GetPrompt(ctx, rot, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty: want ErrNotFound, got %v", err)
	}
	for _, text := range []string{"eins", "zwei", "drei"} {
		if _, err := s.SavePrompt(ctx, rot, text); err != nil {
			t.Fatal(err)
		}
	}
	other, _ := s.SavePrompt(ctx, blau, "andere")
	if other.Version != 1 {
		t.Errorf("versions must be per team, got %d", other.Version)
	}
	latest, err := s.GetPrompt(ctx, rot, 0)
	if err != nil || latest.Version != 3 || latest.Text != "drei" {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
	v1, _ := s.GetPrompt(ctx, rot, 1)
	if v1.Text != "eins" {
		t.Errorf("v1 = %+v", v1)
	}
	list, _ := s.ListPrompts(ctx, rot)
	if len(list) != 3 || list[0].Version != 3 || list[0].Text != "" || list[0].Chars != 4 {
		t.Errorf("list = %+v", list)
	}
}

func TestNotesAreScopedToTeam(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	rot, blau := pair(t, s, "rot", "a1"), pair(t, s, "blau", "a1")
	n, err := s.AddNote(ctx, rot, "Wand links", "team")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateNote(ctx, blau, n.ID, "fremd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("other team may not edit: %v", err)
	}
	if err := s.UpdateNote(ctx, rot, n.ID, "Wand rechts"); err != nil {
		t.Fatal(err)
	}
	notes, _ := s.ListNotes(ctx, rot)
	if len(notes) != 1 || notes[0].Text != "Wand rechts" {
		t.Errorf("notes = %+v", notes)
	}
	if err := s.DeleteNote(ctx, blau, n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("other team may not delete: %v", err)
	}
	if err := s.DeleteNote(ctx, rot, n.ID); err != nil {
		t.Fatal(err)
	}
}

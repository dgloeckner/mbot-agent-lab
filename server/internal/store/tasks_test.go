package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestRowsOutsideAnAssignedPairAreRejected(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	pair(t, s, "rot", "a1")
	if _, err := s.CreateTask(ctx, Task{ID: "a2", Title: "Zwei", State: TaskOpen}); err != nil {
		t.Fatal(err)
	}
	// a2 exists but is not assigned to rot; "x" is no task at all.
	for _, tt := range []TeamTask{{Team: "rot", Task: "a2"}, {Team: "rot", Task: "x"}, {Team: "nope", Task: "a1"}} {
		if _, err := s.SavePrompt(ctx, tt, "p"); err == nil {
			t.Errorf("prompt for %+v accepted", tt)
		}
		if _, err := s.AddNote(ctx, tt, "n", "team"); err == nil {
			t.Errorf("note for %+v accepted", tt)
		}
		if _, err := s.CreateRun(ctx, tt, 0, "m"); err == nil {
			t.Errorf("run for %+v accepted", tt)
		}
		if _, err := s.CreateConversation(ctx, tt, 0, "m"); err == nil {
			t.Errorf("conversation for %+v accepted", tt)
		}
		if err := s.ActivateTask(ctx, tt); err == nil {
			t.Errorf("active task %+v accepted", tt)
		}
	}
	if err := s.AcceptTask(ctx, TeamTask{Team: "rot", Task: "a2"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("accepting an unassigned task: %v", err)
	}
}

func TestTasksOfOneTeamAreSeparate(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	a, b := pair(t, s, "rot", "a1"), pair(t, s, "rot", "a2")

	for _, text := range []string{"a eins", "a zwei"} {
		if _, err := s.SavePrompt(ctx, a, text); err != nil {
			t.Fatal(err)
		}
	}
	pb, _ := s.SavePrompt(ctx, b, "b eins")
	if pb.Version != 1 {
		t.Errorf("versions count per (team, task): b starts at %d", pb.Version)
	}
	if p, _ := s.GetPrompt(ctx, a, 0); p.Text != "a zwei" || p.Version != 2 {
		t.Errorf("latest of a = %+v", p)
	}
	if list, _ := s.ListPrompts(ctx, b); len(list) != 1 {
		t.Errorf("b sees %d prompts", len(list))
	}

	na, _ := s.AddNote(ctx, a, "Notiz a", "team")
	_, _ = s.AddNote(ctx, b, "Notiz b", "agent")
	if notes, _ := s.ListNotes(ctx, a); len(notes) != 1 || notes[0].Text != "Notiz a" {
		t.Errorf("notes of a = %+v", notes)
	}
	if err := s.UpdateNote(ctx, b, na.ID, "fremd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("task b may not edit a's note: %v", err)
	}
	if err := s.DeleteNote(ctx, b, na.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("task b may not delete a's note: %v", err)
	}

	pa, _ := s.GetPrompt(ctx, a, 0)
	ra, _ := s.CreateRun(ctx, a, pa.ID, "m")
	_ = s.EndRun(ctx, ra, "finish", "")
	if _, err := s.LastRun(ctx, b); !errors.Is(err, ErrNotFound) {
		t.Errorf("b sees a's run: %v", err)
	}
	if runs, _ := s.Runs(ctx, a, 5); len(runs) != 1 || runs[0].TaskID != "a1" || runs[0].PromptVersion != 2 {
		t.Errorf("runs of a = %+v", runs)
	}
}

func TestActiveTask(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	a, b := pair(t, s, "rot", "a1"), pair(t, s, "rot", "a2")
	if _, err := s.ActiveTask(ctx, "rot"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no active task yet: %v", err)
	}
	for _, tt := range []TeamTask{a, b} {
		if err := s.ActivateTask(ctx, tt); err != nil {
			t.Fatal(err)
		}
		if got, err := s.ActiveTask(ctx, "rot"); err != nil || got != tt {
			t.Errorf("active = %+v, %v; want %+v", got, err, tt)
		}
	}
	if err := s.AcceptTask(ctx, a); err != nil {
		t.Fatal(err)
	}
	if added, err := s.AssignTask(ctx, a); err != nil || added {
		t.Errorf("assigning twice: %v, %v", added, err)
	}
}

func TestSoftDeleteKeepsHistory(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	tt := pair(t, s, "rot", "a1")
	p, _ := s.SavePrompt(ctx, tt, "Los.")
	run, _ := s.CreateRun(ctx, tt, p.ID, "m")
	_ = s.EndRun(ctx, run, "finish", "")
	_, _ = s.AddNote(ctx, tt, "Wand links", "team")

	if err := s.DeleteTeam(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTeam(ctx, "rot"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
	if _, err := s.Team(ctx, "rot"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted team still found: %v", err)
	}
	if _, err := s.TeamByToken(ctx, "tok-rot"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted team's link still works: %v", err)
	}
	if teams, _ := s.Teams(ctx); len(teams) != 0 {
		t.Errorf("deleted team still listed: %+v", teams)
	}
	// History stays, and the name is free again.
	if runs, _ := s.Runs(ctx, tt, 5); len(runs) != 1 {
		t.Errorf("runs after delete = %+v", runs)
	}
	if notes, _ := s.ListNotes(ctx, tt); len(notes) != 1 {
		t.Errorf("notes after delete = %+v", notes)
	}
	if _, err := s.CreateTeam(ctx, Team{ID: "rot2", Name: "rot", Token: "tok-neu", Color: "red"}); err != nil {
		t.Errorf("name of a deleted team must be free: %v", err)
	}
}

func TestRobotReferences(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	tt := pair(t, s, "rot", "a1")
	run, _ := s.CreateRun(ctx, tt, 0, "m")
	if err := s.AddToolCall(ctx, run, 1, "drive", json.RawMessage(`{}`), nil, time.Second, RobotRef{ID: "mbot-1", Alias: "Scout"}); err == nil {
		t.Error("tool call for an unknown robot accepted")
	}
	if err := s.SeeRobot(ctx, "mbot-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeeRobot(ctx, "mbot-1"); err != nil {
		t.Errorf("seeing a robot twice: %v", err)
	}
	if err := s.AddToolCall(ctx, run, 1, "drive", json.RawMessage(`{}`), nil, time.Second, RobotRef{ID: "mbot-1", Alias: "Scout"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessage(ctx, run, 2, "robot", "tilted", "", nil, RobotRef{ID: "mbot-1", Alias: "Scout"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessage(ctx, run, 3, "team", "chat", "Los", nil, RobotRef{}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM tool_calls WHERE robot_id = 'mbot-1' AND robot_alias = 'Scout')
		+ (SELECT count(*) FROM messages WHERE robot_id = 'mbot-1' AND robot_alias = 'Scout')
		+ (SELECT count(*) FROM messages WHERE robot_id IS NULL AND robot_alias IS NULL)`).Scan(&n)
	if n != 3 {
		t.Errorf("robot references stored: %d of 3", n)
	}
}

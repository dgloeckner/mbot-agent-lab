package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAddEventKeepsScope(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	tt := pair(t, s, "rot", "a1")
	_ = s.SeeRobot(ctx, "mbot-1")
	p, _ := s.SavePrompt(ctx, tt, "Los.")
	run, _ := s.CreateRun(ctx, tt, p.ID, "m")

	if err := s.AddEvent(ctx, Event{Kind: EventTeamJoined, Actor: ActorTeam, Team: "rot", Data: map[string]string{"name": "Rot"}}); err != nil {
		t.Fatalf("team-only event: %v", err)
	}
	if err := s.AddEvent(ctx, Event{Kind: EventTaskPublished, Actor: ActorAdmin, Task: "a1"}); err != nil {
		t.Fatalf("task-only event: %v", err)
	}
	full := Event{Kind: EventRobotEvent, Actor: ActorRobot, Team: "rot", Task: "a1", Robot: "mbot-1", Prompt: p.ID, Run: run,
		Data: map[string]any{"kind": "tilted"}}
	if err := s.AddEvent(ctx, full); err != nil {
		t.Fatal(err)
	}
	evs, err := s.Events(ctx, EventRobotEvent)
	if err != nil || len(evs) != 1 {
		t.Fatalf("events = %+v, %v", evs, err)
	}
	e := evs[0]
	if e.Team != "rot" || e.Task != "a1" || e.Robot != "mbot-1" || e.Prompt != p.ID || e.Run != run || e.At.IsZero() ||
		string(e.Data.(json.RawMessage)) != `{"kind":"tilted"}` {
		t.Errorf("event = %+v", e)
	}
	if all, _ := s.Events(ctx, ""); len(all) != 3 {
		t.Errorf("all events = %d", len(all))
	}
}

func TestAddEventRefusesUnassignedPair(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	pair(t, s, "rot", "a1")
	pair(t, s, "blau", "a2")
	// Both exist, but a2 is not assigned to rot.
	if err := s.AddEvent(ctx, Event{Kind: EventTaskAccepted, Actor: ActorTeam, Team: "rot", Task: "a2"}); err == nil {
		t.Error("event for an unassigned pair accepted")
	}
	if err := s.AddEvent(ctx, Event{Kind: EventRobotAssigned, Actor: ActorAdmin, Team: "rot", Robot: "mbot-9"}); err == nil {
		t.Error("event for an unknown robot accepted")
	}
}

// Results read the event log alone: run counts per (team, task, robot,
// agent version) are a plain GROUP BY over the scope columns.
func TestRunCountsFromEvents(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	a, b := pair(t, s, "rot", "a1"), pair(t, s, "rot", "a2")
	_ = s.SeeRobot(ctx, "mbot-1")
	_ = s.SeeRobot(ctx, "mbot-2")
	pa1, _ := s.SavePrompt(ctx, a, "v1")
	pa2, _ := s.SavePrompt(ctx, a, "v2")
	pb1, _ := s.SavePrompt(ctx, b, "v1")
	start := func(tt TeamTask, robot string, prompt int64) {
		run, _ := s.CreateRun(ctx, tt, prompt, "m")
		if err := s.AddEvent(ctx, Event{Kind: EventRunStarted, Actor: ActorTeam, Team: tt.Team, Task: tt.Task, Robot: robot, Prompt: prompt, Run: run}); err != nil {
			t.Fatal(err)
		}
	}
	start(a, "mbot-1", pa1.ID)
	start(a, "mbot-1", pa1.ID)
	start(a, "mbot-1", pa2.ID)
	start(a, "mbot-2", pa2.ID)
	start(b, "mbot-1", pb1.ID)

	rows, err := s.DB.QueryContext(ctx, `SELECT team_id, task_id, robot_id, prompt_id, count(*) FROM events
		WHERE kind = 'run.started' GROUP BY task_id, team_id, robot_id, prompt_id ORDER BY task_id, robot_id, prompt_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type key struct {
		team, task, robot string
		prompt            int64
	}
	got := map[key]int{}
	for rows.Next() {
		var k key
		var n int
		if err := rows.Scan(&k.team, &k.task, &k.robot, &k.prompt, &n); err != nil {
			t.Fatal(err)
		}
		got[k] = n
	}
	want := map[key]int{
		{"rot", "a1", "mbot-1", pa1.ID}: 2,
		{"rot", "a1", "mbot-1", pa2.ID}: 1,
		{"rot", "a1", "mbot-2", pa2.ID}: 1,
		{"rot", "a2", "mbot-1", pb1.ID}: 1,
	}
	if len(got) != len(want) {
		t.Errorf("groups = %v", got)
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%+v: %d runs, want %d", k, got[k], n)
		}
	}
}

package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRunLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	rot := pair(t, s, "rot", "a1")
	p, _ := s.SavePrompt(ctx, rot, "Finde das blaue Feld.")
	if p.ID == 0 {
		t.Fatal("prompt id missing")
	}
	run, err := s.CreateRun(ctx, rot, p.ID, "testmodell")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.AddMessage(ctx, run, 1, "team", "chat", "Los!", nil, RobotRef{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddToolCall(ctx, run, 2, "drive", json.RawMessage(`{"cm":10}`), json.RawMessage(`{"ok":true}`), 1500*time.Millisecond, RobotRef{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.AddLLMCall(ctx, run, "fake", "testmodell", 100, 20, time.Second, 5); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AddNoteFrom(ctx, rot, "Wand links", "agent", run, msg); err != nil {
		t.Fatal(err)
	}
	if err := s.EndRun(ctx, run, "finish", "Feld gefunden"); err != nil {
		t.Fatal(err)
	}
	_ = s.EndRun(ctx, run, "stop", "") // a second end must not overwrite the first

	runs, _ := s.Runs(ctx, rot, 10)
	r := runs[0]
	if r.PromptVersion != 1 || r.TokensIn != 200 || r.TokensOut != 40 || r.CostMicroEUR != 10 || r.EndReason != "finish" || r.Report != "Feld gefunden" || r.EndedAt.IsZero() {
		t.Errorf("run = %+v", r)
	}
	msgs, _ := s.RunMessages(ctx, run)
	if len(msgs) != 1 || msgs[0].Text != "Los!" {
		t.Errorf("messages = %+v", msgs)
	}

	open2, _ := s.CreateRun(ctx, rot, p.ID, "testmodell")
	_ = s.EndOpenRuns(ctx, "server_restart")
	runs, _ = s.Runs(ctx, rot, 10)
	if runs[0].ID != open2 || runs[0].EndReason != "server_restart" {
		t.Errorf("open run not closed: %+v", runs[0])
	}
}

func TestTeamTotals(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	pairs := map[string]TeamTask{"rot": pair(t, s, "rot", "a1"), "blau": pair(t, s, "blau", "a1")}
	if tot, err := s.TeamTotal(ctx, "rot"); err != nil || tot != (Totals{}) {
		t.Fatalf("no runs: %+v %v", tot, err)
	}
	for _, c := range []struct {
		team    string
		in, out int
		cost    int64
	}{{"rot", 11000, 440, 37900}, {"rot", 55000, 1036, 71900}, {"blau", 100, 10, 1}} {
		run, _ := s.CreateRun(ctx, pairs[c.team], 0, "m")
		if err := s.AddLLMCall(ctx, run, "p", "m", c.in, c.out, time.Second, c.cost); err != nil {
			t.Fatal(err)
		}
	}
	conv, _ := s.CreateConversation(ctx, pairs["rot"], 0, "m")
	if err := s.AddLLMCall(ctx, conv, "p", "m", 4000, 100, time.Second, 500); err != nil {
		t.Fatal(err)
	}
	want := Totals{Runs: 2, TokensIn: 70000, TokensOut: 1576, CostMicroEUR: 110300,
		Conversation: ConversationTotals{Count: 1, TokensIn: 4000, TokensOut: 100, CostMicroEUR: 500}}
	if tot, _ := s.TeamTotal(ctx, "rot"); tot != want {
		t.Errorf("rot = %+v", tot)
	}
	all, err := s.TeamTotals(ctx)
	if err != nil || all["rot"] != want || all["blau"].Runs != 1 || len(all) != 2 {
		t.Errorf("all = %+v %v", all, err)
	}
}

func TestLastRunAndSteps(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	rot := pair(t, s, "rot", "a1")
	if _, err := s.LastRun(ctx, rot); err != ErrNotFound {
		t.Fatalf("no runs: %v", err)
	}
	p, _ := s.SavePrompt(ctx, rot, "Los.")
	run, _ := s.CreateRun(ctx, rot, p.ID, "m")
	_, _ = s.AddMessage(ctx, run, 1, "system", "start", "Der Lauf beginnt.", nil, RobotRef{})
	_ = s.AddToolCall(ctx, run, 2, "drive", json.RawMessage(`{"cm":10}`), json.RawMessage(`{"ok":true}`), time.Second, RobotRef{})
	_, _ = s.AddMessage(ctx, run, 3, "robot:rot", "obstacle_near", "", json.RawMessage(`{"distance_cm":9}`), RobotRef{})
	_ = s.AddToolCall(ctx, run, 4, "finish", json.RawMessage(`{"report":"ok"}`), nil, 0, RobotRef{})
	if _, err := s.LastRun(ctx, rot); err != ErrNotFound {
		t.Errorf("an open run is not the last run: %v", err)
	}
	_ = s.EndRun(ctx, run, "finish", "ok")
	// Conversations and open runs do not count as the last run.
	conv, _ := s.CreateConversation(ctx, rot, p.ID, "m")
	_ = s.EndRun(ctx, conv, "new", "")
	_, _ = s.CreateRun(ctx, rot, p.ID, "m")

	last, err := s.LastRun(ctx, rot)
	if err != nil || last.ID != run || last.PromptVersion != 1 || last.Report != "ok" {
		t.Fatalf("last run = %+v %v", last, err)
	}
	steps, err := s.RunSteps(ctx, run)
	if err != nil || len(steps) != 4 {
		t.Fatalf("steps = %+v %v", steps, err)
	}
	if steps[0].Source != "system" || steps[1].Tool != "drive" || string(steps[1].Args) != `{"cm":10}` ||
		string(steps[2].Data) != `{"distance_cm":9}` || steps[3].Tool != "finish" || steps[3].Result != nil {
		t.Errorf("steps = %+v", steps)
	}
	if runs, _ := s.Runs(ctx, rot, 10); len(runs) != 2 {
		t.Errorf("Runs lists %d rows, want the 2 runs without the conversation", len(runs))
	}
}

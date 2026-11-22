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
	p, _ := s.SavePrompt(ctx, "rot", "Finde das blaue Feld.")
	if p.ID == 0 {
		t.Fatal("prompt id missing")
	}
	run, err := s.CreateRun(ctx, "rot", p.ID, "testmodell")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.AddMessage(ctx, run, 1, "team", "chat", "Los!", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddToolCall(ctx, run, 2, "drive", json.RawMessage(`{"cm":10}`), json.RawMessage(`{"ok":true}`), 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.AddLLMCall(ctx, run, "fake", "testmodell", 100, 20, time.Second, 5); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AddNoteFrom(ctx, "rot", "Wand links", "agent", run, msg); err != nil {
		t.Fatal(err)
	}
	if err := s.EndRun(ctx, run, "finish", "Feld gefunden"); err != nil {
		t.Fatal(err)
	}
	_ = s.EndRun(ctx, run, "stop", "") // a second end must not overwrite the first

	runs, _ := s.Runs(ctx, "rot", 10)
	r := runs[0]
	if r.PromptVersion != 1 || r.TokensIn != 200 || r.TokensOut != 40 || r.CostMicroEUR != 10 || r.EndReason != "finish" || r.Report != "Feld gefunden" || r.EndedAt.IsZero() {
		t.Errorf("run = %+v", r)
	}
	msgs, _ := s.RunMessages(ctx, run)
	if len(msgs) != 1 || msgs[0].Text != "Los!" {
		t.Errorf("messages = %+v", msgs)
	}

	open2, _ := s.CreateRun(ctx, "rot", p.ID, "testmodell")
	_ = s.EndOpenRuns(ctx, "server_restart")
	runs, _ = s.Runs(ctx, "rot", 10)
	if runs[0].ID != open2 || runs[0].EndReason != "server_restart" {
		t.Errorf("open run not closed: %+v", runs[0])
	}
}

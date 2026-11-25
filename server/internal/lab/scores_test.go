package lab

import (
	"context"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func run(success *bool, calls int, toGoal float64) RunView {
	r := RunView{RunScore: store.RunScore{Calls: calls, Success: success}}
	if toGoal > 0 {
		r.ToGoalS = &toGoal
	}
	return r
}

func ptr(b bool) *bool { return &b }

func TestRankWithTiesOnEachLevel(t *testing.T) {
	teams := []TeamScore{
		{Name: "ohne", Runs: []RunView{run(ptr(false), 3, 10), run(nil, 2, 0)}},
		{Name: "teuer", Runs: []RunView{run(ptr(true), 20, 30)}},
		{Name: "zwei", Runs: []RunView{run(ptr(true), 30, 50), run(ptr(true), 25, 60)}},
		{Name: "langsam", Runs: []RunView{run(ptr(true), 10, 90)}},
		{Name: "schnell", Runs: []RunView{run(ptr(true), 10, 40), run(ptr(false), 5, 20)}},
		{Name: "gleich", Runs: []RunView{run(ptr(true), 10, 40)}},
	}
	Rank(teams)
	var got []string
	ranks := map[string]int{}
	for _, tm := range teams {
		got = append(got, tm.Name)
		ranks[tm.Name] = tm.Rank
	}
	// Two successes beat one; then fewer calls; then less time; exact ties share a rank.
	want := []string{"zwei", "schnell", "gleich", "langsam", "teuer", "ohne"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if ranks["schnell"] != 2 || ranks["gleich"] != 2 || ranks["langsam"] != 4 || ranks["ohne"] != 6 {
		t.Errorf("ranks = %v", ranks)
	}
	if teams[0].Successes != 2 || teams[0].Best.Calls != 25 {
		t.Errorf("best of zwei = %+v", teams[0].Best)
	}
}

func TestConfirmTwiceKeepsTheLast(t *testing.T) {
	ctx := context.Background()
	s, st := taskService(t)
	team, _ := s.Join(ctx, "TEST", "Rot")
	task := openTask(t, s, "Challenge", "", team.ID)
	if _, err := s.AcceptTask(ctx, team.ID, task.ID, PromptEmpty, ""); err != nil {
		t.Fatal(err)
	}
	tt := store.TeamTask{Team: team.ID, Task: task.ID}
	p, _ := st.SavePrompt(ctx, tt, "Los.")
	id, _ := st.CreateRun(ctx, tt, p.ID, "m")
	_ = st.AddLLMCall(ctx, id, "p", "m", 10, 5, 0, 0)
	_ = st.EndRun(ctx, id, "finish", "fertig")
	before := s.ScoresVersion()
	if err := s.ConfirmRun(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmRun(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if err := s.AddIntervention(ctx, id, "moved"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddIntervention(ctx, id, "gezaubert"); err == nil {
		t.Error("an unknown intervention was accepted")
	}
	if err := s.ConfirmRun(ctx, 999, true); err != ErrUnknownRun {
		t.Errorf("unknown run: %v", err)
	}
	if s.ScoresVersion() == before {
		t.Error("the scores version did not change")
	}
	teams, err := s.TaskScores(ctx, task.ID)
	if err != nil || len(teams) != 1 || len(teams[0].Runs) != 1 {
		t.Fatalf("scores = %+v, %v", teams, err)
	}
	r := teams[0].Runs[0]
	if r.Success == nil || *r.Success || r.Calls != 1 || r.Interventions.Moved != 1 || r.ToGoalS == nil {
		t.Errorf("run = %+v", r)
	}
	evs, _ := st.Events(ctx, store.EventRunConfirmed)
	if len(evs) != 2 || evs[1].Run != id || evs[1].Actor != store.ActorAdmin {
		t.Errorf("events = %+v", evs)
	}
}

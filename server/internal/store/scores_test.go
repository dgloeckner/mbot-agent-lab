package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskRunsCountInterventions(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, _ = st.CreateTeam(ctx, Team{ID: "rot", Name: "Rot", Token: "a", Color: "red"})
	tt := TeamTask{Team: "rot", Task: "a1"}
	_, _ = st.CreateTask(ctx, Task{ID: "a1", Title: "A", State: TaskOpen})
	_, _ = st.AssignTask(ctx, tt)
	p, _ := st.SavePrompt(ctx, tt, "Los.")

	// Two by-hand calls, then a run the team stops.
	m, _ := st.ManualRun(ctx, tt)
	for i := 1; i <= 2; i++ {
		_ = st.AddToolCall(ctx, m, i, "drive", json.RawMessage(`{}`), json.RawMessage(`{}`), time.Millisecond, RobotRef{})
	}
	// A refused call (missing argument) moved nothing: no intervention.
	_ = st.AddToolCall(ctx, m, 3, "drive", json.RawMessage(`{}`), json.RawMessage(`{"ok":false,"error":"Es fehlt das Argument \"cm\"."}`), time.Millisecond, RobotRef{})
	_ = st.EndManualRuns(ctx, "rot", "run_start")
	time.Sleep(5 * time.Millisecond)
	r1, _ := st.CreateRun(ctx, tt, p.ID, "m")
	_ = st.EndRun(ctx, r1, "stop", "")
	_ = st.AddEvent(ctx, Event{Kind: EventRunEnded, Actor: ActorTeam, Team: "rot", Task: "a1", Prompt: p.ID, Run: r1, Data: map[string]any{"reason": "stop"}})
	// A second run without by-hand calls before it.
	r2, _ := st.CreateRun(ctx, tt, p.ID, "m")
	_ = st.EndRun(ctx, r2, "finish", "fertig")

	runs, err := st.TaskRuns(ctx, "a1")
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if i := runs[0].Interventions; i.ByHand != 2 || i.Stops != 1 || i.Total() != 3 {
		t.Errorf("run 1 interventions = %+v", i)
	}
	if i := runs[1].Interventions; i.Total() != 0 || runs[1].ToGoalS == nil {
		t.Errorf("run 2 = %+v", runs[1])
	}
}

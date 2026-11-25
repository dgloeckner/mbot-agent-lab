package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// manualRows returns the manual rows of a team: id and whether it ended.
func manualRows(t *testing.T, st *store.Store, team string) map[int64]bool {
	t.Helper()
	rows, err := st.DB.Query(`SELECT id, ended_at IS NOT NULL FROM runs WHERE team_id = ? AND kind = 'manual'`, team)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		var ended bool
		_ = rows.Scan(&id, &ended)
		out[id] = ended
	}
	return out
}

func toolCallsOf(t *testing.T, st *store.Store, run int64) []string {
	t.Helper()
	rows, err := st.DB.Query(`SELECT name || ':' || COALESCE(robot_alias, '') FROM tool_calls WHERE run_id = ? ORDER BY seq`, run)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func result(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return m
}

func TestManualCallIsStoredUnderOneManualRow(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	for range 2 {
		res, err := f.m.Manual(ctx, "rot", "drive", `{"cm":20}`)
		if err != nil || result(t, res)["ok"] != true {
			t.Fatalf("drive by hand: %s %v", res, err)
		}
	}
	rows := manualRows(t, f.st, "rot")
	if len(rows) != 1 {
		t.Fatalf("manual rows = %v", rows)
	}
	var id int64
	for k := range rows {
		id = k
	}
	if got := toolCallsOf(t, f.st, id); strings.Join(got, ",") != "drive:Scout,drive:Scout" {
		t.Errorf("tool calls = %v", got)
	}
	// The live log shows each call as the team's.
	backlog, _, cancel := f.hub.Subscribe("rot", 0)
	cancel()
	n := 0
	for _, e := range backlog {
		if e.Source == "team" && e.Kind == "tool" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("team tool entries = %d, backlog %+v", n, backlog)
	}

	// A run start ends the manual row; the next by-hand call opens a new one.
	f.deploy(t, "rot", "Warte.")
	f.model.next = func(int, llm.Request) llm.Message { return llm.Message{Role: llm.RoleAssistant, Content: "Ich warte."} }
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	if rows := manualRows(t, f.st, "rot"); !rows[id] {
		t.Errorf("manual row not ended by the run start: %v", rows)
	}
	f.m.Stop("rot", "Team")
	waitFor(t, "stopped", func() bool { return !f.m.Status("rot").Running })
	if _, err := f.m.Manual(ctx, "rot", "turn", `{"degrees":90}`); err != nil {
		t.Fatal(err)
	}
	if rows := manualRows(t, f.st, "rot"); len(rows) != 2 {
		t.Errorf("manual rows after the run = %v", rows)
	}
}

func TestManualRefusals(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()

	// An agent-only tool: the registry's answer, nothing stored.
	res, err := f.m.Manual(ctx, "rot", "finish", `{"report":"fertig"}`)
	if err != nil || result(t, res)["ok"] != false {
		t.Errorf("finish by hand: %s %v", res, err)
	}
	if rows := manualRows(t, f.st, "rot"); len(rows) != 0 {
		t.Errorf("a refused call opened a manual row: %v", rows)
	}

	// During a run.
	f.deploy(t, "rot", "Denk nach.")
	f.model.block = true
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Manual(ctx, "rot", "drive", `{"cm":20}`); !errors.Is(err, ErrRunActive) {
		t.Errorf("during a run: %v", err)
	}
	f.m.Stop("rot", "Team")
	waitFor(t, "stopped", func() bool { return !f.m.Status("rot").Running })

	// Without a task.
	if _, err := f.st.CreateTeam(ctx, store.Team{ID: "gruen", Name: "Grün", Token: "g", Color: "green"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Manual(ctx, "gruen", "drive", `{"cm":20}`); !errors.Is(err, ErrNoTask) {
		t.Errorf("without a task: %v", err)
	}

	// In a closed task.
	if err := f.st.CloseTask(ctx, testTask); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Manual(ctx, "rot", "drive", `{"cm":20}`); !errors.Is(err, ErrTaskClosed) {
		t.Errorf("closed task: %v", err)
	}
}

func TestStartRefusedDuringManualCallAndStopCancelsIt(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	f.deploy(t, "rot", "Los.")
	f.bus.mu.Lock()
	f.bus.gate = true
	f.bus.mu.Unlock()
	done := make(chan json.RawMessage, 1)
	go func() {
		res, _ := f.m.Manual(ctx, "rot", "drive", `{"cm":50}`)
		done <- res
	}()
	waitFor(t, "the drive to be held", func() bool { return len(f.bus.holding()) == 1 })
	if err := f.m.Start(ctx, "rot"); !errors.Is(err, ErrManualRunning) {
		t.Errorf("start during a by-hand call: %v", err)
	}
	if !f.m.ManualBusy("rot") {
		t.Error("ManualBusy = false during the call")
	}
	f.m.Stop("rot", "Team")
	select {
	case res := <-done:
		if got := result(t, res); got["ok"] != false || got["reason"] != "stopped" {
			t.Errorf("stopped call = %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not cancel the by-hand call")
	}
	if f.m.ManualBusy("rot") {
		t.Error("ManualBusy after the call ended")
	}
	f.bus.release()
}

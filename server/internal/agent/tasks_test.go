package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

const secretTaskText = "GEHEIMER AUFGABENTEXT: Findet den blauen Ausgang."

// switchTo creates task id (if needed) with the secret text, assigns,
// accepts and activates it for team.
func (f *fixture) switchTo(t *testing.T, team, id string) store.TeamTask {
	t.Helper()
	ctx := context.Background()
	if _, err := f.st.Task(ctx, id); errors.Is(err, store.ErrNotFound) {
		if _, err := f.st.CreateTask(ctx, store.Task{ID: id, Title: id, Body: secretTaskText, Starter: "Startprompt", State: store.TaskOpen}); err != nil {
			t.Fatal(err)
		}
	}
	tt := store.TeamTask{Team: team, Task: id}
	if _, err := f.st.AssignTask(ctx, tt); err != nil {
		t.Fatal(err)
	}
	if err := f.st.AcceptTask(ctx, tt); err != nil {
		t.Fatal(err)
	}
	if err := f.st.ActivateTask(ctx, tt); err != nil {
		t.Fatal(err)
	}
	return tt
}

func allText(req llm.Request) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

// Two tasks of one team: each run sees only its own task's prompt and notes,
// never the task text; remember writes into the run's task.
func TestRunWorksInTheActiveTask(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	a := pairOf("rot") // the fixture's task
	_, _ = f.st.SavePrompt(ctx, a, "Prompt A")
	_, _ = f.st.AddNote(ctx, a, "Notiz A", "team")
	b := f.switchTo(t, "rot", "b1")
	_, _ = f.st.SavePrompt(ctx, b, "Prompt B")
	_, _ = f.st.AddNote(ctx, b, "Notiz B", "team")

	f.model.next = func(n int, req llm.Request) llm.Message {
		if n == 1 {
			return toolCall("c1", "remember", `{"text":"gelernt in B"}`)
		}
		return toolCall("c2", "finish", `{"report":"fertig"}`)
	}
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	if st := f.m.Status("rot"); st.Task != "b1" {
		t.Errorf("status task = %q", st.Task)
	}
	waitFor(t, "run in b to end", func() bool { return !f.m.Status("rot").Running })
	ctxB := allText(f.model.calls()[0])
	for _, want := range []string{"Prompt B", "Notiz B"} {
		if !strings.Contains(ctxB, want) {
			t.Errorf("context of b misses %q", want)
		}
	}
	for _, not := range []string{"Prompt A", "Notiz A", "GEHEIMER AUFGABENTEXT"} {
		if strings.Contains(ctxB, not) {
			t.Errorf("context of b holds %q", not)
		}
	}
	if notes, _ := f.st.ListNotes(ctx, b); len(notes) != 2 || notes[1].Text != "gelernt in B" {
		t.Errorf("notes of b = %+v", notes)
	}
	if notes, _ := f.st.ListNotes(ctx, a); len(notes) != 1 {
		t.Errorf("remember wrote into a: %+v", notes)
	}
	runs, _ := f.st.Runs(ctx, b, 5)
	if len(runs) != 1 || runs[0].TaskID != "b1" {
		t.Errorf("runs of b = %+v", runs)
	}

	// Back in a: a's prompt and notes, b's learning stays in b.
	if err := f.st.ActivateTask(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "run in a to end", func() bool { return !f.m.Status("rot").Running })
	ctxA := allText(f.model.calls()[2])
	if !strings.Contains(ctxA, "Prompt A") || !strings.Contains(ctxA, "Notiz A") || strings.Contains(ctxA, "gelernt in B") || strings.Contains(ctxA, "Prompt B") {
		t.Errorf("context of a:\n%s", ctxA)
	}
}

// A conversation's last run is the last run of the same task.
func TestConversationSeesTheLastRunOfItsTask(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	f.deploy(t, "rot", "Fahr in A.")
	f.model.next = func(n int, req llm.Request) llm.Message {
		if n == 1 {
			return toolCall("c1", "finish", `{"report":"Bericht aus A"}`)
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "Hallo."}
	}
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "run to end", func() bool { return !f.m.Status("rot").Running })
	b := f.switchTo(t, "rot", "b1")
	_, _ = f.st.SavePrompt(ctx, b, "Prompt B")
	if _, err := f.m.Talk("rot", "Was war los?"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "answer", func() bool { return len(f.model.calls()) == 2 && f.m.Status("rot").State == StateWaiting })
	sys := f.model.calls()[1].Messages[0].Content
	if strings.Contains(sys, "Bericht aus A") || !strings.Contains(sys, "Dein Team hatte noch keinen Lauf.") {
		t.Errorf("conversation in b sees a's run:\n%s", sys)
	}
	if strings.Contains(sys, "GEHEIMER AUFGABENTEXT") {
		t.Error("conversation context holds the task text")
	}
}

func TestStartNeedsAnOpenTask(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	f.deploy(t, "rot", "Los.")
	_, _ = f.st.DB.ExecContext(ctx, `DELETE FROM active_tasks WHERE team_id = 'rot'`)
	var se *StartError
	if err := f.m.Start(ctx, "rot"); !errors.As(err, &se) || se.Msg != "Eurem Team ist noch keine Aufgabe zugewiesen." {
		t.Errorf("start without task: %v", err)
	}
	_ = f.st.ActivateTask(ctx, pairOf("rot"))
	_ = f.st.CloseTask(ctx, testTask)
	if err := f.m.Start(ctx, "rot"); !errors.As(err, &se) || !strings.Contains(se.Msg, "geschlossen") {
		t.Errorf("start in a closed task: %v", err)
	}
}

func TestClosingTheTaskEndsTheRun(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	f.deploy(t, "rot", "Denk lange nach.")
	f.model.block = true
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "model call", func() bool { return len(f.model.calls()) == 1 })
	f.m.TaskClosed("anderes") // another task: nothing happens
	f.m.TaskClosed(testTask)
	waitFor(t, "run to end", func() bool { return !f.m.Status("rot").Running })
	runs, _ := f.st.Runs(ctx, pairOf("rot"), 1)
	if runs[0].EndReason != "task_closed" {
		t.Errorf("end reason = %q", runs[0].EndReason)
	}
}

// Robot tool calls and robot messages name the robot, and runs write their events.
func TestRobotReferencesAndRunEvents(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	f.deploy(t, "rot", "Fahr.")
	f.model.next = func(n int, req llm.Request) llm.Message {
		switch n {
		case 1:
			return toolCall("c1", "drive", `{"cm":20}`)
		case 2:
			return llm.Message{Role: llm.RoleAssistant, Content: "Ich warte."}
		}
		return toolCall("c2", "finish", `{"report":"gefahren"}`)
	}
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first answer done", func() bool { return len(f.model.calls()) == 2 && f.m.Status("rot").State == StateWaiting })
	f.bus.deliver("lab/mbot-1/event", `{"kind":"tilted","data":{},"seq":7,"t_ms":7}`)
	waitFor(t, "run to end", func() bool { return !f.m.Status("rot").Running })

	run := f.m.Status("rot").RunID
	var n int
	_ = f.st.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM tool_calls WHERE run_id = ? AND name = 'drive' AND robot_id = 'mbot-1' AND robot_alias = 'Scout')
		+ (SELECT count(*) FROM messages WHERE run_id = ? AND kind = 'tilted' AND robot_id = 'mbot-1' AND robot_alias = 'Scout')`, run, run).Scan(&n)
	if n != 2 {
		t.Errorf("robot references stored: %d of 2", n)
	}

	p, _ := f.st.GetPrompt(ctx, pairOf("rot"), 0)
	evs, _ := f.st.Events(ctx, "")
	got := map[string]store.Event{}
	for _, e := range evs {
		got[e.Kind] = e
	}
	for _, want := range []struct{ kind, actor, robot string }{
		{store.EventRunStarted, store.ActorTeam, ""},
		{store.EventRobotEvent, store.ActorRobot, "mbot-1"},
		{store.EventRunFinished, store.ActorAgent, ""},
		{store.EventRunEnded, store.ActorAgent, ""},
	} {
		e, ok := got[want.kind]
		if !ok || e.Actor != want.actor || e.Team != "rot" || e.Task != testTask || e.Prompt != p.ID || e.Run != run || e.Robot != want.robot {
			t.Errorf("%s = %+v, %v", want.kind, e, ok)
		}
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func toolNames(ts []llm.Tool) string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return strings.Join(out, ",")
}

func conversationEntries(f *fixture, team string) []live.LogEntry {
	backlog, _, cancel := f.hub.Subscribe(team, 0)
	cancel()
	var out []live.LogEntry
	for _, e := range backlog {
		if e.Channel == live.ChannelConversation {
			out = append(out, e)
		}
	}
	return out
}

// After a run, a team message gets an answer in a conversation: the agent
// sees the numbered protocol of the last run and every tool as text, but can
// only call read_sensors and remember. A motion call is refused, no robot
// moves, no run starts.
func TestConversationAfterRun(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	f.deploy(t, "rot", "Fahr ein Quadrat.")
	f.model.next = func(n int, req llm.Request) llm.Message {
		switch n {
		case 1:
			return toolCall("c1", "drive", `{"cm":12}`)
		case 2:
			return toolCall("c2", "drive", `{"cm":12}`)
		case 3:
			return toolCall("c3", "finish", `{"report":"Quadrat gefahren."}`)
		case 4:
			sys := req.Messages[0].Content
			for _, want := range []string{"Gerade läuft KEIN Lauf", "[2. drive]", "Dein Bericht (finish): Quadrat gefahren.", "- drive(", "- retrace_path(", "Fahr ein Quadrat.", "Deine Roboter: Scout."} {
				if !strings.Contains(sys, want) {
					t.Errorf("conversation system message misses %q:\n%s", want, sys)
				}
			}
			if strings.Contains(sys, platformRules) {
				t.Error("conversation uses the run's platform rules")
			}
			if got := toolNames(req.Tools); got != "read_sensors,remember" {
				t.Errorf("callable tools = %s", got)
			}
			last := req.Messages[len(req.Messages)-1].Content
			if !strings.Contains(last, `{"von":"Team"`) || !strings.Contains(last, "Fahr los!") || strings.Contains(last, "Zustand deiner Roboter") {
				t.Errorf("conversation inbox = %s", last)
			}
			return toolCall("c4", "drive", `{"cm":20}`) // not callable: must be refused
		case 5:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != llm.RoleTool || !strings.Contains(last.Content, "Im Gespräch nicht verfügbar") {
				t.Errorf("motion call not refused: %+v", last)
			}
			return llm.Message{Role: llm.RoleAssistant, Content: "Im nächsten Lauf würde ich drive nehmen."}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "?"}
	}
	if err := f.m.Start(context.Background(), "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "run to end", func() bool { return !f.m.Status("rot").Running })
	moves := len(f.bus.sent())

	if !f.m.Status("rot").Conversation {
		t.Fatal("conversation not offered")
	}
	e, err := f.m.Talk("rot", "Fahr los!")
	if err != nil || e.Channel != live.ChannelConversation || e.Source != live.SourceTeam {
		t.Fatalf("Talk = %+v, %v", e, err)
	}
	waitFor(t, "answer", func() bool { return len(f.model.calls()) == 5 && f.m.Status("rot").State == StateWaiting })

	if st := f.m.Status("rot"); st.Running || st.ConversationID == 0 || st.ConversationTokens != 220 {
		t.Errorf("status = %+v", st)
	}
	if got := len(f.bus.sent()); got != moves {
		t.Errorf("robot got %d commands in the conversation", got-moves)
	}
	var kinds []string
	for _, e := range conversationEntries(f, "rot") {
		kinds = append(kinds, e.Source+":"+e.Kind)
	}
	if got := strings.Join(kinds, ","); got != "team:chat,agent:tool,agent:text" {
		t.Errorf("conversation entries = %s", got)
	}

	ctx := context.Background()
	runs, _ := f.st.Runs(ctx, "rot", 5)
	if len(runs) != 1 {
		t.Errorf("runs = %+v, the conversation must not count as a run", runs)
	}
	total, _ := f.st.TeamTotal(ctx, "rot")
	if total.Runs != 1 || total.Conversation.Count != 1 || total.Conversation.TokensIn != 200 || total.TokensIn != 500 {
		t.Errorf("totals = %+v", total)
	}
	steps, _ := f.st.RunSteps(ctx, f.m.Status("rot").ConversationID)
	if len(steps) != 3 || steps[0].Text != "Fahr los!" || steps[1].Tool != "drive" || steps[2].Text == "" {
		t.Errorf("stored conversation = %+v", steps)
	}
}

// A note written in the conversation reaches the next run; the conversation
// itself does not. Starting the run ends the conversation.
func TestConversationOnlyNotesReachTheRun(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	f.deploy(t, "rot", "Los.")
	var runSystem, runInbox string
	f.model.next = func(n int, req llm.Request) llm.Message {
		switch n {
		case 1:
			return toolCall("m1", "remember", `{"text":"Kanten sind 20 cm lang."}`)
		case 2:
			return llm.Message{Role: llm.RoleAssistant, Content: "Gemerkt. Das Codewort ist Pinguin."}
		default:
			runSystem = req.Messages[0].Content
			runInbox = req.Messages[len(req.Messages)-1].Content
			return toolCall("f1", "finish", `{"report":"ok"}`)
		}
	}
	if _, err := f.m.Talk("rot", "Merk dir: Kanten sind 20 cm. Codewort Pinguin."); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "answer", func() bool { return len(f.model.calls()) == 2 && f.m.Status("rot").State == StateWaiting })
	conv := f.m.Status("rot").ConversationID
	notes, _ := f.st.ListNotes(context.Background(), "rot")
	if len(notes) != 1 || notes[0].Author != "agent" {
		t.Fatalf("notes = %+v", notes)
	}

	if err := f.m.Start(context.Background(), "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "run to end", func() bool { return !f.m.Status("rot").Running })
	if !strings.Contains(runSystem, "Kanten sind 20 cm lang.") {
		t.Error("note missing in the run")
	}
	if strings.Contains(runSystem+runInbox, "Pinguin") || strings.Contains(runSystem, "Gerade läuft KEIN Lauf") {
		t.Error("the conversation leaked into the run's context")
	}
	if f.m.Status("rot").ConversationID != 0 {
		t.Error("the run did not end the conversation")
	}
	told := false
	for _, e := range conversationEntries(f, "rot") {
		told = told || strings.Contains(e.Text, "Ein Lauf hat begonnen")
	}
	if !told {
		t.Error("the conversation does not say that the run ended it")
	}
	var reason string
	_ = f.st.DB.QueryRow(`SELECT end_reason FROM runs WHERE id = ?`, conv).Scan(&reason)
	if reason != "run_start" {
		t.Errorf("conversation end reason = %q", reason)
	}
}

func TestConversationLimits(t *testing.T) {
	f := newFixture(t, config.Budgets{MaxConversationMessagesPerMinute: 2})
	f.model.next = func(int, llm.Request) llm.Message { return llm.Message{Role: llm.RoleAssistant, Content: "Hallo."} }

	on := false
	f.m.Conversations = func() bool { return on }
	var te *TalkError
	if _, err := f.m.Talk("rot", "Hallo?"); !errors.As(err, &te) || te.Limited || !strings.Contains(te.Msg, "Leitstand") {
		t.Fatalf("switched off: err = %v", err)
	}
	if f.m.Status("rot").Conversation {
		t.Error("status offers a switched-off conversation")
	}
	on = true
	for i := 0; i < 2; i++ {
		if _, err := f.m.Talk("rot", "Hallo?"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.m.Talk("rot", "Hallo?"); !errors.As(err, &te) || !te.Limited {
		t.Fatalf("third message in a minute: err = %v", err)
	}
	if _, err := f.m.Talk("blau", "Hallo?"); err != nil {
		t.Errorf("the limit is per team: %v", err)
	}
	if !f.m.allowTalk("rot", time.Now().Add(time.Minute)) {
		t.Error("the limit must reset after a minute")
	}
}

// When a conversation uses up its token budget it ends; the next message
// starts a new one.
func TestConversationTokenBudget(t *testing.T) {
	f := newFixture(t, config.Budgets{MaxTokensPerConversation: 100})
	f.model.next = func(int, llm.Request) llm.Message { return llm.Message{Role: llm.RoleAssistant, Content: "Hallo."} }
	if _, err := f.m.Talk("rot", "Eins"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first answer", func() bool { return len(f.model.calls()) == 1 && f.m.Status("rot").State == StateWaiting })
	if f.m.Status("rot").ConversationID != 0 {
		t.Error("conversation over budget still open")
	}
	if _, err := f.m.Talk("rot", "Zwei"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second answer", func() bool { return len(f.model.calls()) == 2 && f.m.Status("rot").State == StateWaiting })
	if hist := f.model.calls()[1].Messages; len(hist) != 2 {
		t.Errorf("new conversation carries %d messages", len(hist))
	}
	total, _ := f.st.TeamTotal(context.Background(), "rot")
	if total.Conversation.Count != 2 || total.Runs != 0 {
		t.Errorf("totals = %+v", total)
	}
}

// The team can start a fresh conversation; the old history is gone.
func TestNewConversation(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	f.model.next = func(int, llm.Request) llm.Message { return llm.Message{Role: llm.RoleAssistant, Content: "Ja."} }
	_, _ = f.m.Talk("rot", "Eins")
	waitFor(t, "answer", func() bool { return len(f.model.calls()) == 1 && f.m.Status("rot").State == StateWaiting })
	_, _ = f.m.Talk("rot", "Zwei")
	waitFor(t, "answer", func() bool { return len(f.model.calls()) == 2 && f.m.Status("rot").State == StateWaiting })
	if n := len(f.model.calls()[1].Messages); n != 4 {
		t.Errorf("second message sees %d messages, want 4", n)
	}
	f.m.NewConversation("rot")
	waitFor(t, "conversation ended", func() bool { return f.m.Status("rot").ConversationID == 0 })
	_, _ = f.m.Talk("rot", "Drei")
	waitFor(t, "answer", func() bool { return len(f.model.calls()) == 3 && f.m.Status("rot").State == StateWaiting })
	if n := len(f.model.calls()[2].Messages); n != 2 {
		t.Errorf("new conversation sees %d messages, want 2", n)
	}
}

// Starting a run does not wait for a slow answer in the conversation.
func TestStartInterruptsConversation(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	f.deploy(t, "rot", "Los.")
	f.model.block = true
	_, _ = f.m.Talk("rot", "Denk nach.")
	waitFor(t, "thinking", func() bool { return f.m.Status("rot").State == StateThinking })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.m.Start(ctx, "rot"); err != nil {
		t.Fatal(err)
	}
	if st := f.m.Status("rot"); !st.Running || st.ConversationID != 0 {
		t.Errorf("status = %+v", st)
	}
	f.m.Stop("rot", "Team")
}

// While a run is active, the conversation is not used: Talk still works but
// the actor hands the message to the run.
func TestTalkDuringRunGoesToRun(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	f.deploy(t, "rot", "Warte.")
	f.model.next = func(n int, req llm.Request) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "Ok."}
	}
	_ = f.m.Start(context.Background(), "rot")
	waitFor(t, "first call", func() bool { return len(f.model.calls()) == 1 && f.m.Status("rot").State == StateWaiting })
	_, _ = f.m.Talk("rot", "Noch da?")
	waitFor(t, "second call", func() bool { return len(f.model.calls()) == 2 })
	sys := f.model.calls()[1].Messages[0].Content
	if !strings.Contains(sys, platformRules) {
		t.Error("message during a run did not go to the run")
	}
	f.m.Stop("rot", "Team")
}

func TestFormatRunNumbersSteps(t *testing.T) {
	start := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	r := store.Run{ID: 7, PromptVersion: 2, StartedAt: start, EndedAt: start.Add(83 * time.Second), EndReason: "finish", Report: "Fertig."}
	steps := []store.RunStep{
		{Seq: 1, Source: "system", Kind: "start", Text: "Der Lauf beginnt."},
		{Seq: 2, Tool: "drive", Args: json.RawMessage(`{"cm": 12}`), Result: json.RawMessage(`{"ok":true,"driven_cm":12.2}`)},
		{Seq: 3, Tool: "turn", Args: json.RawMessage(`{"degrees":90}`), Result: json.RawMessage(`{"ok":true}`)},
		{Seq: 4, Source: "robot:rot", Kind: "obstacle_near", Data: json.RawMessage(`{"distance_cm":12}`)},
		{Seq: 5, Tool: "drive", Args: json.RawMessage(`{"cm":12}`), Result: json.RawMessage(`{"ok":true}`)},
		{Seq: 6, Source: "agent", Kind: "text", Text: "Zweite Strecke\ngeschafft."},
		{Seq: 7, Source: "team", Kind: "chat", Text: "Weiter!"},
	}
	got := FormatRun(r, steps)
	for _, want := range []string{
		"Lauf 7 mit Prompt-Version 2, Dauer 1m23s. Der Agent hat den Lauf mit finish beendet.",
		"Dein Bericht (finish): Fertig.",
		"1. System (start): Der Lauf beginnt.",
		`2. Werkzeug [1. drive] {"cm":12} -> {"ok":true,"driven_cm":12.2}`,
		"3. Werkzeug [1. turn]",
		`4. Roboter rot (obstacle_near): {"distance_cm":12}`,
		"5. Werkzeug [2. drive]",
		"6. Du schreibst: Zweite Strecke geschafft.",
		"7. Team: Weiter!",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	var many []store.RunStep
	for i := 1; i <= 400; i++ {
		many = append(many, store.RunStep{Seq: i, Tool: "drive", Args: json.RawMessage(`{"cm":10}`), Result: json.RawMessage(`{"ok":true,"floor":"` + strings.Repeat("w", 60) + `"}`)})
	}
	long := FormatRun(r, many)
	if n := len([]rune(long)); n > lastRunChars+500 {
		t.Errorf("long run: %d characters", n)
	}
	for _, want := range []string{"1. Werkzeug [1. drive]", "10. Werkzeug [10. drive]", "sind ausgelassen.)", "400. Werkzeug [400. drive]"} {
		if !strings.Contains(long, want) {
			t.Errorf("long run misses %q", want)
		}
	}
	if strings.Contains(long, fmt.Sprintf("\n%d. Werkzeug", 11)) {
		t.Error("long run keeps step 11")
	}
}

package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// example returns the Blockly state of an example team tool.
func example(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/examples/programs/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var ex struct {
		Blockly json.RawMessage `json:"blockly"`
	}
	_ = json.Unmarshal(raw, &ex)
	return ex.Blockly
}

func TestSaveProgramRules(t *testing.T) {
	ctx := context.Background()
	s, _ := taskService(t)
	team, _ := s.Join(ctx, "TEST", "Rot")
	if _, err := s.SaveProgram(ctx, team.ID, "", "quadrat", "", example(t, "quadrat")); !errors.Is(err, ErrNoTask) {
		t.Errorf("without a task: %v", err)
	}
	a := openTask(t, s, "A", "", team.ID)
	b := openTask(t, s, "B", "", team.ID)
	if _, err := s.AcceptTask(ctx, team.ID, a.ID, PromptEmpty, ""); err != nil {
		t.Fatal(err)
	}

	p, err := s.SaveProgram(ctx, team.ID, "", "quadrat", "Fährt ein Quadrat.", example(t, "quadrat"))
	if err != nil || !p.Valid || len(p.Problems) != 0 || p.ID == "" {
		t.Fatalf("save quadrat = %+v, %v", p, err)
	}
	// Invalid tools are stored and flagged: a built-in name, a broken name, a block out of range.
	for _, c := range []struct{ name, want string }{{"drive_until", "eingebautes"}, {"Fahre!", "3 bis 30"}} {
		bad, err := s.SaveProgram(ctx, team.ID, "", c.name, "", example(t, "quadrat"))
		if err != nil || bad.Valid || !strings.Contains(bad.Problems[0].Text, c.want) {
			t.Errorf("%s: %+v, %v", c.name, bad, err)
		}
	}
	far := json.RawMessage(strings.Replace(string(example(t, "quadrat")), `"cm": 30`, `"cm": 900`, 1))
	bad, err := s.SaveProgram(ctx, team.ID, "", "zu_weit", "", far)
	if err != nil || bad.Valid || bad.Problems[0].Block != "q_drive" {
		t.Errorf("out of range: %+v, %v", bad, err)
	}
	if list, _ := s.Programs(ctx, team.ID); len(list) != 4 {
		t.Errorf("list = %+v", list)
	}
	// A second tool with the same name is refused; replacing is last write wins.
	if _, err := s.SaveProgram(ctx, team.ID, "", "quadrat", "", example(t, "quadrat")); !conflict(err) {
		t.Errorf("same name: %v", err)
	}
	if upd, err := s.SaveProgram(ctx, team.ID, p.ID, "quadrat", "Neu", example(t, "quadrat")); err != nil || upd.Description != "Neu" {
		t.Errorf("update = %+v, %v", upd, err)
	}
	// The 13th is refused.
	for i := 4; i < 12; i++ {
		if _, err := s.SaveProgram(ctx, team.ID, "", fmt.Sprintf("werkzeug_%d", i), "", example(t, "quadrat")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveProgram(ctx, team.ID, "", "werkzeug_13", "", example(t, "quadrat")); !conflict(err) || !strings.Contains(err.Error(), "12") {
		t.Errorf("13th: %v", err)
	}

	// Accepting another task with "copy" brings the tools along; the copies are new tools.
	if _, err := s.Store.SavePrompt(ctx, pairOf(team.ID, a.ID), "Fahr ein Quadrat."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptTask(ctx, team.ID, b.ID, PromptCopy, a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Programs(ctx, team.ID)
	if len(list) != 12 {
		t.Fatalf("copied %d tools", len(list))
	}
	for _, x := range list {
		if x.ID == p.ID {
			t.Error("a copy keeps the id of the original")
		}
	}
	if _, err := s.Program(ctx, team.ID, p.ID); !errors.Is(err, ErrUnknownProgram) {
		t.Errorf("a tool of the other task is visible: %v", err)
	}
}

func pairOf(team, task string) store.TeamTask { return store.TeamTask{Team: team, Task: task} }

func TestAgentToolsIsATaskOption(t *testing.T) {
	ctx := context.Background()
	s, _ := taskService(t)
	task, err := s.CreateTask(ctx, TaskInput{Title: "Challenge", Body: "x", AgentTools: "team_tools"})
	if err != nil || task.AgentTools != "team_tools" {
		t.Fatalf("create = %+v, %v", task, err)
	}
	if _, err := s.CreateTask(ctx, TaskInput{Title: "Kaputt", AgentTools: "einige"}); err == nil {
		t.Error("an unknown tool set was accepted")
	}
	if dup, _ := s.DuplicateTask(ctx, task.ID); dup.AgentTools != "team_tools" {
		t.Errorf("duplicate = %+v", dup)
	}
	if _, err := s.PublishTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask(ctx, task.ID, TaskInput{Title: "Challenge", Body: "x", AgentTools: "all"}); !conflict(err) {
		t.Errorf("change after publishing: %v", err)
	}
	if got, _ := s.Task(ctx, task.ID); got.AgentTools != "team_tools" {
		t.Errorf("after the refused change = %+v", got)
	}
}

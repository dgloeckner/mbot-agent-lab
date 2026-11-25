package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestProgramsAreScopedByTeamAndTask(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, _ = st.CreateTeam(ctx, Team{ID: "rot", Name: "Rot", Token: "a", Color: "red"})
	a, b := TeamTask{Team: "rot", Task: "a1"}, TeamTask{Team: "rot", Task: "b1"}
	for _, tt := range []TeamTask{a, b} {
		_, _ = st.CreateTask(ctx, Task{ID: tt.Task, Title: tt.Task, State: TaskOpen})
		_, _ = st.AssignTask(ctx, tt)
	}
	p := Program{ID: "p1", Name: "quadrat", Blockly: json.RawMessage(`{}`), Tree: json.RawMessage(`{}`), Problems: json.RawMessage(`[]`)}
	if _, err := st.SaveProgram(ctx, a, p, true); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetProgram(ctx, a, "p1")
	if err != nil || got.Name != "quadrat" || !got.Valid() {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := st.GetProgram(ctx, b, "p1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a tool of another task is visible: %v", err)
	}
	if list, _ := st.Programs(ctx, b); len(list) != 0 {
		t.Errorf("task b lists %+v", list)
	}
	// The same name in the pair is a conflict; in another task it is fine.
	if _, err := st.SaveProgram(ctx, a, Program{ID: "p2", Name: "quadrat", Blockly: p.Blockly, Tree: p.Tree, Problems: p.Problems}, true); !errors.Is(err, ErrConflict) {
		t.Errorf("same name: %v", err)
	}
	// Last write wins.
	p.Description, p.Problems = "Neu", json.RawMessage(`[{"text":"x"}]`)
	if _, err := st.SaveProgram(ctx, a, p, false); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.GetProgram(ctx, a, "p1"); got.Description != "Neu" || got.Valid() {
		t.Errorf("after update = %+v", got)
	}
	if _, err := st.SaveProgram(ctx, b, p, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("update from another task: %v", err)
	}
	n := 0
	if c, err := st.CopyPrograms(ctx, a, b, func() string { n++; return "copy" }); err != nil || c != 1 || n != 1 {
		t.Errorf("copy = %d, %v", c, err)
	}
	if list, _ := st.Programs(ctx, b); len(list) != 1 || list[0].ID != "copy" || list[0].Description != "Neu" {
		t.Errorf("copied = %+v", list)
	}
	if err := st.DeleteProgram(ctx, b, "p1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete from another task: %v", err)
	}
	if err := st.DeleteProgram(ctx, a, "p1"); err != nil {
		t.Fatal(err)
	}
	if c, _ := st.CountPrograms(ctx, a); c != 0 {
		t.Errorf("count = %d", c)
	}
}

func TestVersionsFreezeAndRestoreTeamTools(t *testing.T) {
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
	tool := func(id, name, blockly string) Program {
		return Program{ID: id, Name: name, Blockly: json.RawMessage(blockly), Tree: json.RawMessage(`{}`), Problems: json.RawMessage(`[]`)}
	}
	_, _ = st.SaveProgram(ctx, tt, tool("p1", "quadrat", `{"a":1}`), true)
	_, _ = st.SaveProgram(ctx, tt, tool("p2", "kreis", `{}`), true)
	v1, _ := st.SavePrompt(ctx, tt, "Version eins")
	// Change one tool, delete one, add one; deploy with the same text.
	_, _ = st.SaveProgram(ctx, tt, tool("p1", "quadrat", `{"a":2}`), false)
	_ = st.DeleteProgram(ctx, tt, "p2")
	_, _ = st.SaveProgram(ctx, tt, tool("p3", "neu", `{}`), true)
	_, _ = st.SavePrompt(ctx, tt, "Version eins")

	list, _ := st.ListPrompts(ctx, tt)
	changes := map[string]string{}
	for _, c := range list[0].Tools {
		changes[c.Name] = c.Change
	}
	if len(list) != 2 || changes["quadrat"] != ToolChanged || changes["kreis"] != ToolRemoved || changes["neu"] != ToolNew {
		t.Errorf("version 2 changes = %+v", list[0].Tools)
	}
	if len(list[1].Tools) != 2 || list[1].Tools[0].Change != ToolNew {
		t.Errorf("version 1 changes = %+v", list[1].Tools)
	}

	p, err := st.RestoreVersion(ctx, tt, v1.Version)
	if err != nil || p.Text != "Version eins" {
		t.Fatalf("restore = %+v, %v", p, err)
	}
	now, _ := st.Programs(ctx, tt)
	if len(now) != 2 || now[0].Name != "kreis" || now[1].ID != "p1" || string(now[1].Blockly) != `{"a":1}` {
		t.Errorf("after restore = %+v", now)
	}
	if after, _ := st.ListPrompts(ctx, tt); len(after) != 2 {
		t.Error("restoring must not change the history")
	}
}

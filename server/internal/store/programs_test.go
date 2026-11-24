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

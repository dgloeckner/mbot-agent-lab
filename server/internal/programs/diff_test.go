package programs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// example reads the block state of an example tool as plain JSON values.
func example(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "examples", "programs", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var ex struct {
		Blockly map[string]any `json:"blockly"`
	}
	if err := json.Unmarshal(b, &ex); err != nil {
		t.Fatal(err)
	}
	return ex.Blockly
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// find returns the block with an id anywhere in a state.
func find(v any, id string) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		if x["id"] == id {
			return x
		}
		for _, k := range sortedKeys(x) {
			if b := find(x[k], id); b != nil {
				return b
			}
		}
	case []any:
		for _, e := range x {
			if b := find(e, id); b != nil {
				return b
			}
		}
	}
	return nil
}

func clone(t *testing.T, v map[string]any) map[string]any {
	var out map[string]any
	if err := json.Unmarshal(raw(t, v), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func diff(t *testing.T, older, newer map[string]any) []Change {
	t.Helper()
	got, err := Diff(Side{Name: "x", Blockly: raw(t, older)}, Side{Name: "x", Blockly: raw(t, newer)})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func kinds(cs []Change) []string {
	var out []string
	for _, c := range cs {
		s := c.Kind + ":" + c.Block
		if c.Field != "" {
			s += "." + c.Field
		}
		out = append(out, s)
	}
	return out
}

func expect(t *testing.T, got []Change, want ...string) {
	t.Helper()
	if k := kinds(got); !reflect.DeepEqual(k, want) {
		t.Fatalf("changes %v, want %v", k, want)
	}
}

func TestDiffEqualTreesHaveNoChange(t *testing.T) {
	for _, name := range []string{"quadrat", "labyrinth", "rechts"} {
		q := example(t, name)
		expect(t, diff(t, q, clone(t, q)))
	}
	// Moving the whole tool on the workspace is no change.
	q := example(t, "quadrat")
	moved := clone(t, q)
	find(moved, "q_start")["x"] = 300.0
	expect(t, diff(t, q, moved))
}

func TestDiffChangedValue(t *testing.T) {
	q := example(t, "quadrat")
	n := clone(t, q)
	find(n, "q_drive")["fields"].(map[string]any)["cm"] = 40.0
	got := diff(t, q, n)
	expect(t, got, "value:q_drive.cm")
	if got[0].Old != 30.0 || got[0].New != 40.0 {
		t.Fatalf("values %v -> %v", got[0].Old, got[0].New)
	}
}

func TestDiffNewAndRemovedBlock(t *testing.T) {
	q := example(t, "quadrat")
	n := clone(t, q)
	// A new block after the turn, inside the loop.
	find(n, "q_turn")["next"] = map[string]any{"block": map[string]any{"type": "tool_set_leds", "id": "q_led", "fields": map[string]any{"color": "red"}}}
	expect(t, diff(t, q, n), "new:q_led")
	expect(t, diff(t, n, q), "removed:q_led")
	// Removing the first block in a chain moves nothing else.
	r := clone(t, q)
	find(r, "q_repeat")["inputs"].(map[string]any)["DO"] = map[string]any{"block": find(r, "q_turn")}
	expect(t, diff(t, q, r), "removed:q_drive")
}

func TestDiffRebuiltBlockIsRemovedAndNew(t *testing.T) {
	q := example(t, "quadrat")
	n := clone(t, q)
	find(n, "q_turn")["id"] = "q_turn2"
	expect(t, diff(t, q, n), "removed:q_turn", "new:q_turn2")
}

func TestDiffMovedBlock(t *testing.T) {
	q := example(t, "quadrat")
	// Turn first, then drive: one of the two moved, not both.
	n := clone(t, q)
	drive, turn := find(n, "q_drive"), find(n, "q_turn")
	delete(drive, "next")
	turn["next"] = map[string]any{"block": drive}
	find(n, "q_repeat")["inputs"].(map[string]any)["DO"] = map[string]any{"block": turn}
	got := diff(t, q, n)
	if len(got) != 1 || got[0].Kind != ChangeMoved {
		t.Fatalf("swap: %v", kinds(got))
	}
	// Out of the loop, after it: the turn moved.
	m := clone(t, q)
	turn = find(m, "q_turn")
	delete(find(m, "q_drive"), "next")
	find(m, "q_repeat")["next"] = map[string]any{"block": turn}
	expect(t, diff(t, q, m), "moved:q_turn")
}

func TestDiffInsideNestedLoop(t *testing.T) {
	l := example(t, "labyrinth")
	n := clone(t, l)
	// The if inside the loop: turn left instead of right, and a new
	// condition value.
	find(n, "l_turn")["fields"].(map[string]any)["dir"] = "left"
	find(n, "l_wall")["fields"].(map[string]any)["VALUE"] = "obstacle"
	expect(t, diff(t, l, n), "value:l_wall.VALUE", "value:l_turn.dir")
}

func TestDiffNameAndDescription(t *testing.T) {
	q := raw(t, example(t, "quadrat"))
	got, err := Diff(Side{Name: "quadrat", Description: "Fährt ein Quadrat.", Blockly: q}, Side{Name: "viereck", Description: "Fährt ein Quadrat mit 30 cm.", Blockly: q})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, got, "name:", "description:")
	if got[1].Old != "Fährt ein Quadrat." || got[1].New != "Fährt ein Quadrat mit 30 cm." {
		t.Fatalf("description %v", got[1])
	}
}

func TestDiffNewTool(t *testing.T) {
	got, err := Diff(Side{}, Side{Name: "quadrat", Blockly: raw(t, example(t, "quadrat"))})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, got, "name:", "new:q_start", "new:q_repeat", "new:q_drive", "new:q_turn")
	if _, err := Diff(Side{Blockly: json.RawMessage(`{`)}, Side{}); err == nil {
		t.Fatal("a broken state must be an error")
	}
}

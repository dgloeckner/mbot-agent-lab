package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The by-hand set is exactly this list (docs/tools.md, column "by hand").
var wantManual = []string{"drive", "turn", "drive_arc", "drive_until", "turn_until_clear", "scan", "follow_line_until",
	"retrace_path", "read_sensors", "wait_for", "set_leds", "show_text", "play_sound", "identify"}

func TestManualSet(t *testing.T) {
	r, _, _ := setup(t, map[string]string{"mbot-1": "Scout"})
	if got := strings.Join(r.ManualNames(), ","); got != strings.Join(wantManual, ",") {
		t.Errorf("by-hand tools = %s\nwant %s", got, strings.Join(wantManual, ","))
	}
	for _, name := range r.Names() {
		if len(r.Ends(name)) == 0 {
			t.Errorf("%s has no normal end", name)
		}
	}
	if r.Manual("fly") || r.NormalEnd("fly", json.RawMessage(`{"ok":true}`)) {
		t.Error("an unknown tool is neither by hand nor ends normally")
	}
}

func TestCallManualRefusesAgentOnlyTools(t *testing.T) {
	r, _, _ := setup(t, map[string]string{"mbot-1": "Scout"})
	finished := false
	env := &Env{TeamID: "rot", TaskID: "a1", Finish: func(string) { finished = true }, Say: func(string, string) []string { return []string{"x"} }}
	args := map[string]string{"say": `{"to":"alle","text":"Hallo"}`, "finish": `{"report":"fertig"}`, "remember": `{"text":"x"}`,
		"forget": `{"note_id":1}`, "tell_team": `{"text":"x"}`, "list_robots": `{}`, "stop": `{}`}
	for _, name := range r.Names() {
		if r.Manual(name) {
			continue
		}
		got := schemaOf(t, r.CallManual(context.Background(), env, name, args[name]))
		if got["ok"] != false || !strings.Contains(got["error"].(string), "nur euer Agent") {
			t.Errorf("CallManual(%s) = %v", name, got)
		}
	}
	if finished {
		t.Error("finish by hand ended a run")
	}
	got := schemaOf(t, r.CallManual(context.Background(), env, "drive", `{"cm":20}`))
	if got["ok"] != true || got["reason"] != "done" {
		t.Errorf("CallManual(drive) = %v", got)
	}
}

func TestNormalEnd(t *testing.T) {
	r, _, _ := setup(t, nil)
	cases := []struct {
		tool, result string
		want         bool
	}{
		{"drive", `{"ok":true,"reason":"done","driven_cm":20}`, true},
		{"drive", `{"ok":true,"reason":"obstacle","driven_cm":8}`, false},
		{"turn", `{"ok":true,"reason":"done"}`, true},
		{"turn", `{"ok":false,"reason":"tilted"}`, false},
		{"drive_arc", `{"ok":true,"reason":"color"}`, true},
		{"drive_arc", `{"ok":true,"reason":"obstacle"}`, false},
		{"drive_until", `{"ok":true,"reason":"wall"}`, true},
		{"drive_until", `{"ok":true,"reason":"obstacle"}`, true},
		{"drive_until", `{"ok":false,"reason":"stopped"}`, false},
		{"turn_until_clear", `{"ok":true,"reason":"max_deg"}`, true},
		{"turn_until_clear", `{"ok":false,"reason":"timeout"}`, false},
		{"follow_line_until", `{"ok":true,"reason":"crossing"}`, true},
		{"follow_line_until", `{"ok":true,"reason":"line_lost"}`, false},
		{"retrace_path", `{"ok":true,"reason":"empty"}`, true},
		{"retrace_path", `{"ok":true,"reason":"obstacle"}`, false},
		{"scan", `{"ok":true,"reason":"done"}`, true},
		{"scan", `{"ok":false,"reason":"offline","error":"Scout ist nicht verbunden."}`, false},
		{"wait_for", `{"ok":true,"event":"button_a","robot":"Scout"}`, true},
		{"wait_for", `{"ok":false,"reason":"timeout"}`, true},
		{"wait_for", `{"ok":false,"reason":"stopped"}`, false},
		{"read_sensors", `{"ok":true,"distance_cm":40}`, true},
		{"read_sensors", `{"ok":false,"error":"Eurem Team ist gerade kein Roboter zugeteilt."}`, false},
		{"set_leds", `{"ok":true,"reason":"done"}`, true},
		{"drive", `kein json`, false},
	}
	for _, c := range cases {
		if got := r.NormalEnd(c.tool, json.RawMessage(c.result)); got != c.want {
			t.Errorf("NormalEnd(%s, %s) = %v, want %v", c.tool, c.result, got, c.want)
		}
	}
}

func TestCatalogFor(t *testing.T) {
	r, _, _ := setup(t, map[string]string{"mbot-1": "Scout", "mbot-2": "Träger"})
	byName := map[string]Info{}
	for _, info := range r.CatalogFor(&Env{TeamID: "rot"}) {
		byName[info.Name] = info
	}
	if len(byName) != len(Catalog) {
		t.Fatalf("catalog has %d entries, want %d", len(byName), len(Catalog))
	}
	for _, name := range wantManual {
		info := byName[name]
		if !info.Manual || len(info.Schema) == 0 {
			t.Errorf("%s: manual=%v schema=%s", name, info.Manual, info.Schema)
		}
	}
	for _, name := range []string{"say", "finish", "stop", "play_melody"} {
		if info := byName[name]; info.Manual || info.Schema != nil {
			t.Errorf("%s must not be by hand: %+v", name, info)
		}
	}
	// The schema is the one the model sees, with the robot enum.
	drive := schemaOf(t, byName["drive"].Schema)
	if enum := drive["properties"].(map[string]any)["robot"].(map[string]any)["enum"].([]any); len(enum) != 2 {
		t.Errorf("robot enum = %v", enum)
	}
	if def := find(r, &Env{TeamID: "rot"}, "drive"); jsonNum(def) != jsonNum(drive) {
		t.Errorf("schema differs from the model's:\n%v\n%v", def, drive)
	}
}

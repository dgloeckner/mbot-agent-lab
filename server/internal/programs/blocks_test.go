package programs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const examples = "../../../docs/examples/programs"

func normalize(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var x any
	_ = json.Unmarshal(b, &x)
	out, _ := json.Marshal(x)
	return string(out)
}

func TestExamplesConvertToTheirTrees(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(examples, "*.json"))
	n := 0
	for _, file := range files {
		if filepath.Base(file) == "blocks.json" {
			continue
		}
		n++
		raw, _ := os.ReadFile(file)
		var ex struct {
			Blockly json.RawMessage `json:"blockly"`
			Program Program         `json:"program"`
		}
		if err := json.Unmarshal(raw, &ex); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		p, problems := Convert(ex.Program.Name, ex.Program.Description, ex.Blockly, registry())
		if len(problems) > 0 {
			t.Errorf("%s: %+v", file, problems)
			continue
		}
		if got, want := normalize(t, p), normalize(t, ex.Program); got != want {
			t.Errorf("%s:\n got  %s\n want %s", file, got, want)
		}
	}
	if n < 3 {
		t.Fatalf("only %d examples", n)
	}
}

func state(blocks string) json.RawMessage {
	return json.RawMessage(`{"blocks":{"languageVersion":0,"blocks":[` + blocks + `]}}`)
}

func TestConversionRefusals(t *testing.T) {
	startWith := func(next string) string {
		return `{"type":"program_start","id":"s","x":0,"y":0,"next":{"block":` + next + `}}`
	}
	cases := []struct {
		name, state, block, want string
	}{
		{"unknown block type", startWith(`{"type":"fly_away","id":"f"}`), "f", "kenne ich nicht"},
		{"agent-only tool", startWith(`{"type":"tool_finish","id":"f","fields":{"report":"x"}}`), "f", "nur der Agent"},
		{"value out of range", startWith(`{"type":"tool_drive","id":"d","fields":{"cm":900}}`), "d", "zwischen -300 und 300"},
		{"repeat 21", startWith(`{"type":"repeat_times","id":"r","fields":{"TIMES":21}}`), "r", "1 bis 20"},
		{"loose block", startWith(`{"type":"tool_drive","id":"d","fields":{"cm":20}}`) + `,{"type":"tool_turn","id":"lose","x":200,"y":0,"fields":{"degrees":90,"dir":"right"}}`, "lose", "hängt an nichts"},
		{"two starts", startWith(`{"type":"tool_drive","id":"d","fields":{"cm":20}}`) + `,{"type":"program_start","id":"s2","x":300,"y":0}`, "s2", "mehr als einen Start"},
		{"no start", `{"type":"tool_drive","id":"d","x":0,"y":0,"fields":{"cm":20}}`, "", "fehlt der Block"},
		{"missing condition", startWith(`{"type":"if","id":"i"}`), "i", "fehlt die Bedingung"},
	}
	for _, c := range cases {
		_, problems := Convert("test_tool", "", state(c.state), registry())
		found := false
		for _, p := range problems {
			if p.Block == c.block && strings.Contains(p.Text, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: problems %+v, want %q at %q", c.name, problems, c.want, c.block)
		}
	}
}

func TestNotAndDirection(t *testing.T) {
	s := state(`{"type":"program_start","id":"s","next":{"block":{"type":"if","id":"i","inputs":{
		"COND":{"block":{"type":"cond_not","id":"n","inputs":{"COND":{"block":{"type":"cond_sensor_floor","id":"c","fields":{"VALUE":"b"}}}}}},
		"DO":{"block":{"type":"tool_drive_arc","id":"a","fields":{"dir":"left","radius_cm":30,"degrees":180},
			"next":{"block":{"type":"tool_drive_until","id":"u","fields":{"max_cm":50,"stop_below_cm":10,"stop_on_color":"none"}}}}}}}}}`)
	p, problems := Convert("test_tool", "", s, registry())
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	want := `{"name":"test_tool","description":"","body":[{"id":"i","type":"if","cond":{"id":"c","source":"sensor","field":"floor","op":"contains","value":"b","not":true},"body":[{"id":"a","type":"call","tool":"drive_arc","args":{"degrees":-180,"radius_cm":30}},{"id":"u","type":"call","tool":"drive_until","args":{"max_cm":50,"stop_below_cm":10}}]}]}`
	if got := normalize(t, p); got != normalize(t, json.RawMessage(want)) {
		t.Errorf("got %s", got)
	}
	d := Describer(registry())
	if got := d(p.Body[0].Body[0]); got != "fahre eine Kurve nach links mit Radius 30 cm, 180 Grad weit" {
		t.Errorf("describe = %q", got)
	}
	if got := d(p.Body[0].Body[1]); got != "fahre geradeaus, höchstens 50 cm, bis eine Wand näher als 10 cm ist oder der Boden egal ist" {
		t.Errorf("describe = %q", got)
	}
}

// TestBlocksGolden pins the served definitions in docs/examples/programs/blocks.json;
// the web tests build the examples in Blockly from that file. UPDATE_GOLDEN=1 rewrites it.
func TestBlocksGolden(t *testing.T) {
	defs := Blocks(registry())
	got, _ := json.MarshalIndent(defs, "", "  ")
	got = append(got, '\n')
	file := filepath.Join(examples, "blocks.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(file, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is out of date: run UPDATE_GOLDEN=1 go test ./internal/programs", file)
	}
	for _, d := range defs.Blocks {
		if strings.Contains(normalize(t, d), `"robot"`) {
			t.Errorf("block %s has a robot field", d["type"])
		}
	}
}

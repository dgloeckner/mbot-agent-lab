package tools

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/protocol"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// robotBus answers every command like a robot would and records them.
type robotBus struct {
	mu      sync.Mutex
	handler func(string, []byte)
	cmds    []string
}

func (b *robotBus) Handle(_ string, _ int, fn func(string, []byte)) error {
	b.mu.Lock()
	b.handler = fn
	b.mu.Unlock()
	return nil
}

func (b *robotBus) Publish(topic string, payload []byte, _ bool, _ byte) error {
	id, suffix, _ := protocol.ParseTopic(topic)
	if suffix != protocol.TopicCmd {
		return nil
	}
	var cmd protocol.Cmd
	_ = json.Unmarshal(payload, &cmd)
	b.mu.Lock()
	b.cmds = append(b.cmds, id+":"+cmd.Name)
	h := b.handler
	b.mu.Unlock()
	res := protocol.Result{ID: cmd.ID, OK: true, Reason: protocol.ReasonDone}
	if cmd.Name == "drive_until" {
		res.Reason = protocol.ReasonWall
		res.Data = json.RawMessage(`{"driven_cm":42,"distance_cm":11,"color":"white"}`)
	}
	raw, _ := json.Marshal(res)
	go h(protocol.Topic(id, protocol.TopicResult), raw)
	return nil
}

func (b *robotBus) deliver(topic, payload string) {
	b.mu.Lock()
	h := b.handler
	b.mu.Unlock()
	h(topic, []byte(payload))
}

func setup(t *testing.T, aliases map[string]string) (*Registry, *robotBus, *store.Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bus := &robotBus{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := robots.New(bus, []robots.Spec{{ID: "mbot-1"}, {ID: "mbot-2"}, {ID: "mbot-3"}}, log)
	go reg.Run(ctx)
	for {
		bus.mu.Lock()
		ready := bus.handler != nil
		bus.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	for _, id := range []string{"mbot-1", "mbot-2", "mbot-3"} {
		bus.deliver("lab/"+id+"/status", "online")
		bus.deliver("lab/"+id+"/hello", `{"v":1}`)
	}
	for id, alias := range aliases {
		_ = reg.Assign(id, robots.Owner{TeamID: "rot", TeamName: "Rot", Alias: alias})
	}
	_ = reg.Assign("mbot-3", robots.Owner{TeamID: "blau", TeamName: "Blau", Alias: "fremd"})
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewRegistry(reg, st, live.NewHub(), config.Budgets{MaxAgentMessagesPerMinute: 2}), bus, st
}

func schemaOf(t *testing.T, defs []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(defs, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func find(r *Registry, env *Env, name string) map[string]any {
	for _, d := range r.Definitions(env) {
		if d.Name == name {
			var m map[string]any
			_ = json.Unmarshal(d.Parameters, &m)
			return m
		}
	}
	return nil
}

func TestRobotParameterOnlyWithSeveralRobots(t *testing.T) {
	r, _, _ := setup(t, map[string]string{"mbot-1": "Scout"})
	env := &Env{TeamID: "rot"}
	drive := find(r, env, "drive")
	if _, ok := drive["properties"].(map[string]any)["robot"]; ok {
		t.Errorf("single robot must not need a robot parameter: %v", drive)
	}
	for _, d := range r.Definitions(env) {
		if d.Name == "follow_line_until" || d.Name == "gesture" {
			t.Errorf("unfinished tool offered: %s", d.Name)
		}
	}

	r2, _, _ := setup(t, map[string]string{"mbot-1": "Scout", "mbot-2": "Träger"})
	drive = find(r2, env, "drive")
	robot := drive["properties"].(map[string]any)["robot"].(map[string]any)
	if got := robot["enum"].([]any); len(got) != 2 || got[0] != "Scout" || got[1] != "Träger" {
		t.Errorf("robot enum = %v", got)
	}
	if req := drive["required"].([]any); req[len(req)-1] != "robot" {
		t.Errorf("robot not required: %v", req)
	}
	stop := find(r2, env, "stop")
	if enum := stop["properties"].(map[string]any)["robot"].(map[string]any)["enum"].([]any); enum[2] != "alle" {
		t.Errorf("stop enum = %v", enum)
	}
}

func TestCallRobotTools(t *testing.T) {
	r, bus, _ := setup(t, map[string]string{"mbot-1": "Scout", "mbot-2": "Träger"})
	env := &Env{TeamID: "rot"}
	ctx := context.Background()
	time.Sleep(20 * time.Millisecond) // let the stops from assigning the robots pass
	bus.mu.Lock()
	bus.cmds = nil // assigning robots stops them; only count the tool calls below
	bus.mu.Unlock()

	got := schemaOf(t, r.Call(ctx, env, "drive_until", `{"max_cm":100,"robot":"träger"}`))
	if got["ok"] != true || got["reason"] != "wall" || got["driven_cm"] != 42.0 || got["robot"] != "Träger" {
		t.Errorf("drive_until = %v", got)
	}
	got = schemaOf(t, r.Call(ctx, env, "turn", `{"degrees":90}`))
	if got["ok"] != false || !strings.Contains(got["error"].(string), "Scout, Träger") {
		t.Errorf("missing robot = %v", got)
	}
	got = schemaOf(t, r.Call(ctx, env, "turn", `{"degrees":90,"robot":"fremd"}`))
	if got["ok"] != false {
		t.Errorf("another team's robot must not be reachable: %v", got)
	}
	got = schemaOf(t, r.Call(ctx, env, "drive", `{"robot":"Scout"}`))
	if got["ok"] != false || !strings.Contains(got["error"].(string), `"cm"`) {
		t.Errorf("missing argument = %v", got)
	}
	got = schemaOf(t, r.Call(ctx, env, "fly", `{}`))
	if got["ok"] != false {
		t.Errorf("unknown tool = %v", got)
	}
	got = schemaOf(t, r.Call(ctx, env, "stop", `{"robot":"alle"}`))
	if got["ok"] != true || len(got["robots"].([]any)) != 2 {
		t.Errorf("stop alle = %v", got)
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	for _, c := range bus.cmds {
		if strings.HasPrefix(c, "mbot-3") {
			t.Errorf("command reached another team's robot: %v", bus.cmds)
		}
	}
	if bus.cmds[0] != "mbot-2:drive_until" {
		t.Errorf("first command = %v", bus.cmds)
	}
}

func TestNoRobot(t *testing.T) {
	r, _, _ := setup(t, nil)
	got := schemaOf(t, r.Call(context.Background(), &Env{TeamID: "rot"}, "read_sensors", `{}`))
	if got["ok"] != false || !strings.Contains(got["error"].(string), "kein Roboter") {
		t.Errorf("read_sensors without robot = %v", got)
	}
}

func TestServerTools(t *testing.T) {
	r, bus, st := setup(t, map[string]string{"mbot-1": "Scout"})
	ctx := context.Background()
	var finished string
	var said []string
	env := &Env{TeamID: "rot", RunID: 0, Agents: []string{"Blau"},
		Finish: func(report string) { finished = report },
		Say:    func(to, text string) []string { said = append(said, to+":"+text); return []string{to} },
	}

	got := schemaOf(t, r.Call(ctx, env, "remember", `{"text":"Wand links"}`))
	if got["ok"] != true {
		t.Fatalf("remember = %v", got)
	}
	notes, _ := st.ListNotes(ctx, "rot")
	if len(notes) != 1 || notes[0].Author != "agent" {
		t.Errorf("notes = %+v", notes)
	}
	got = schemaOf(t, r.Call(ctx, env, "forget", `{"note_id":`+jsonNum(got["note_id"])+`}`))
	if got["ok"] != true {
		t.Errorf("forget = %v", got)
	}

	sayDef := find(r, env, "say")
	to := sayDef["properties"].(map[string]any)["to"].(map[string]any)["enum"].([]any)
	if len(to) != 2 || to[0] != "Blau" || to[1] != "alle" {
		t.Errorf("say enum = %v", to)
	}
	for i := 0; i < 3; i++ {
		got = schemaOf(t, r.Call(ctx, env, "say", `{"to":"Blau","text":"Hallo"}`))
	}
	if got["ok"] != false || len(said) != 2 {
		t.Errorf("say budget not enforced: %v, said %v", got, said)
	}

	_ = schemaOf(t, r.Call(ctx, env, "finish", `{"report":"Fertig"}`))
	if finished != "Fertig" {
		t.Errorf("finish report = %q", finished)
	}

	done := make(chan map[string]any)
	go func() { done <- schemaOf(t, r.Call(ctx, env, "wait_for", `{"event":"button_a","timeout_s":5}`)) }()
	time.Sleep(30 * time.Millisecond)
	bus.deliver("lab/mbot-3/event", `{"kind":"button_a","seq":1,"t_ms":1}`) // other team: ignored
	bus.deliver("lab/mbot-1/event", `{"kind":"clap","seq":2,"t_ms":2}`)     // other kind: ignored
	bus.deliver("lab/mbot-1/event", `{"kind":"button_a","seq":3,"t_ms":3}`)
	select {
	case got = <-done:
		if got["ok"] != true || got["robot"] != "Scout" {
			t.Errorf("wait_for = %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait_for did not return")
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

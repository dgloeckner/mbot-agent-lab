package programs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

type nopBus struct{}

func (nopBus) Publish(string, []byte, bool, byte) error       { return nil }
func (nopBus) Handle(string, int, func(string, []byte)) error { return nil }

// registry is the real tool registry: validation checks against the schemas the model sees.
func registry() *tools.Registry {
	return tools.NewRegistry(robots.New(nopBus{}, nil, nil), nil, nil, config.Budgets{})
}

// fake answers tool calls from a script and records them.
type fake struct {
	mu     sync.Mutex
	calls  []string
	reply  func(tool string, args map[string]any, n int) Outcome
	senses []map[string]any
	sensed int
	block  string // this tool blocks until ctx ends
}

func (f *fake) Call(ctx context.Context, tool string, args map[string]any) Outcome {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("%s%v", tool, argList(args)))
	n := len(f.calls)
	reply := f.reply
	f.mu.Unlock()
	if tool == f.block {
		<-ctx.Done()
		return Outcome{Result: json.RawMessage(`{"ok":false,"reason":"stopped"}`), Hard: true}
	}
	if reply != nil {
		return reply(tool, args, n)
	}
	return ok(`{"ok":true,"reason":"done"}`)
}

func (f *fake) Sense(context.Context) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sensed++
	if len(f.senses) == 0 {
		return map[string]any{"distance_cm": 100.0, "floor": "wwww"}, nil
	}
	s := f.senses[0]
	f.senses = f.senses[1:]
	return s, nil
}

func (f *fake) log() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, " ")
}

func argList(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	b, _ := json.Marshal(args)
	return string(b)
}

func ok(result string) Outcome { return Outcome{Result: json.RawMessage(result), Normal: true} }
func odd(result string) Outcome {
	return Outcome{Result: json.RawMessage(result)}
}

// prog builds a program from JSON nodes.
func prog(t *testing.T, body string) *Program {
	t.Helper()
	p, err := Parse([]byte(`{"name":"test_tool","description":"Test.","body":` + body + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(p, registry()); len(problems) > 0 {
		t.Fatalf("invalid test program: %+v", problems)
	}
	return p
}

func call(id, tool, args string) string {
	return fmt.Sprintf(`{"id":%q,"type":"call","tool":%q,"args":%s}`, id, tool, args)
}

func TestSequenceOfThreeCalls(t *testing.T) {
	f := &fake{}
	var events []string
	res := Run(context.Background(), prog(t, `[`+call("a", "drive", `{"cm":20}`)+`,`+call("b", "turn", `{"degrees":90}`)+`,`+call("c", "read_sensors", `{}`)+`]`), f,
		Options{Observe: func(e Event) { events = append(events, e.Phase+":"+e.Block) }})
	if res.End != EndDone || !res.OK || res.Steps != 3 {
		t.Errorf("result = %+v", res)
	}
	if got := f.log(); got != `drive{"cm":20} turn{"degrees":90} read_sensors` {
		t.Errorf("calls = %s", got)
	}
	if strings.Join(events, " ") != "before:a after:a before:b after:b before:c after:c" {
		t.Errorf("events = %v", events)
	}
	if string(res.Last) != `{"ok":true,"reason":"done"}` {
		t.Errorf("last = %s", res.Last)
	}
}

func TestRepeat(t *testing.T) {
	for _, n := range []int{1, 4, 20} {
		f := &fake{}
		var rounds []int
		res := Run(context.Background(), prog(t, fmt.Sprintf(`[{"id":"r","type":"repeat","times":%d,"body":[`+call("d", "drive", `{"cm":10}`)+`]}]`, n)), f,
			Options{Observe: func(e Event) {
				if e.Phase == PhaseAfter {
					rounds = append(rounds, e.Round)
				}
			}})
		if res.End != EndDone || res.Steps != n || len(rounds) != n || rounds[n-1] != n {
			t.Errorf("repeat %d: %+v rounds %v", n, res, rounds)
		}
	}
	p, _ := Parse([]byte(`{"name":"zu_oft","body":[{"id":"r","type":"repeat","times":21,"body":[` + call("d", "drive", `{"cm":10}`) + `]}]}`))
	if problems := Validate(p, registry()); len(problems) != 1 || problems[0].Block != "r" || !strings.Contains(problems[0].Text, "1 bis 20") {
		t.Errorf("repeat 21: %+v", problems)
	}
}

func TestRepeatUntilNeverTrueEndsAtTheRoundLimit(t *testing.T) {
	f := &fake{}
	res := Run(context.Background(), prog(t, `[{"id":"u","type":"repeat_until","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"color"},"body":[`+call("d", "drive", `{"cm":10}`)+`]}]`), f, Options{})
	if res.End != EndLimit || res.Steps != 50 || res.Block != "u" || !strings.Contains(res.Error, "50 Runden") {
		t.Errorf("result = %+v", res)
	}
}

func TestIf(t *testing.T) {
	ifProg := func(op string, not bool, withElse bool) *Program {
		els := ""
		if withElse {
			els = `,"else":[` + call("e", "turn", `{"degrees":-90}`) + `]`
		}
		return prog(t, `[`+call("d", "drive_until", `{"max_cm":100}`)+`,{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"`+op+`","value":"wall","not":`+fmt.Sprint(not)+`},"body":[`+call("t", "turn", `{"degrees":90}`)+`]`+els+`}]`)
	}
	wall := func(string, map[string]any, int) Outcome { return ok(`{"ok":true,"reason":"wall"}`) }
	for _, c := range []struct {
		name     string
		p        *Program
		want     string
		replyAll func(string, map[string]any, int) Outcome
	}{
		{"then", ifProg("eq", false, true), `drive_until{"max_cm":100} turn{"degrees":90}`, wall},
		{"else", ifProg("ne", false, true), `drive_until{"max_cm":100} turn{"degrees":-90}`, wall},
		{"no else", ifProg("ne", false, false), `drive_until{"max_cm":100}`, wall},
		{"negated", ifProg("eq", true, true), `drive_until{"max_cm":100} turn{"degrees":-90}`, wall},
	} {
		f := &fake{reply: c.replyAll}
		res := Run(context.Background(), c.p, f, Options{})
		if res.End != EndDone || f.log() != c.want {
			t.Errorf("%s: %+v calls %s", c.name, res, f.log())
		}
	}
}

func TestSensorConditionReadsOnceAndCounts(t *testing.T) {
	f := &fake{senses: []map[string]any{{"distance_cm": 12.0}, {"distance_cm": 80.0}}}
	var kinds []string
	p := prog(t, `[{"id":"r","type":"repeat","times":2,"body":[{"id":"i","type":"if","cond":{"id":"c","source":"sensor","field":"distance_cm","op":"lt","value":25},"body":[`+call("t", "turn", `{"degrees":-90}`)+`],"else":[`+call("d", "drive", `{"cm":20}`)+`]}]}]`)
	res := Run(context.Background(), p, f, Options{Observe: func(e Event) {
		if e.Phase == PhaseAfter {
			kinds = append(kinds, e.Kind+":"+e.Block)
		}
	}})
	if res.End != EndDone || res.Steps != 4 || f.sensed != 2 {
		t.Errorf("result = %+v, sensed %d", res, f.sensed)
	}
	if f.log() != `turn{"degrees":-90} drive{"cm":20}` || strings.Join(kinds, " ") != "sense:c call:t sense:c call:d" {
		t.Errorf("calls %s, events %v", f.log(), kinds)
	}
}

// obstacleOn makes one tool end unexpectedly with obstacle.
func obstacleOn(tool string) func(string, map[string]any, int) Outcome {
	return func(t string, _ map[string]any, _ int) Outcome {
		if t == tool {
			return odd(`{"ok":true,"reason":"obstacle","driven_cm":8}`)
		}
		return ok(`{"ok":true,"reason":"done"}`)
	}
}

func TestErrorRule(t *testing.T) {
	// Followed by a call: aborted, naming tool, reason and block.
	f := &fake{reply: obstacleOn("drive")}
	res := Run(context.Background(), prog(t, `[`+call("d", "drive", `{"cm":50}`)+`,`+call("t", "turn", `{"degrees":90}`)+`]`), f, Options{})
	if res.End != EndAborted || res.Block != "d" || res.Tool != "drive" || res.Reason != "obstacle" || res.At != "drive(cm: 50)" || f.log() != `drive{"cm":50}` {
		t.Errorf("followed by a call: %+v, calls %s", res, f.log())
	}
	if !strings.Contains(string(res.Last), "obstacle") {
		t.Errorf("last = %s", res.Last)
	}

	// Followed by an if that names the reason: goes on.
	f = &fake{reply: obstacleOn("drive")}
	res = Run(context.Background(), prog(t, `[`+call("d", "drive", `{"cm":50}`)+`,{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"obstacle"},"body":[`+call("t", "turn", `{"degrees":90}`)+`]},`+call("e", "drive", `{"cm":-5}`)+`]`), f, Options{})
	if res.End != EndAborted || res.Block != "e" {
		// the second drive also ends with obstacle and nothing handles it
		t.Errorf("if names it: %+v, calls %s", res, f.log())
	}
	if f.log() != `drive{"cm":50} turn{"degrees":90} drive{"cm":-5}` {
		t.Errorf("if names it: calls %s", f.log())
	}

	// An if with an else on the result handles any reason.
	f = &fake{reply: obstacleOn("drive")}
	res = Run(context.Background(), prog(t, `[`+call("d", "drive", `{"cm":50}`)+`,{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"wall"},"body":[],"else":[`+call("t", "turn", `{"degrees":180}`)+`]}]`), f, Options{})
	if res.End != EndDone || f.log() != `drive{"cm":50} turn{"degrees":180}` {
		t.Errorf("if with else: %+v, calls %s", res, f.log())
	}

	// An if on a sensor does not handle it.
	f = &fake{reply: obstacleOn("drive")}
	res = Run(context.Background(), prog(t, `[`+call("d", "drive", `{"cm":50}`)+`,{"id":"i","type":"if","cond":{"id":"c","source":"sensor","field":"distance_cm","op":"lt","value":10},"body":[],"else":[]}]`), f, Options{})
	if res.End != EndAborted || res.Block != "d" {
		t.Errorf("sensor if: %+v", res)
	}

	// Followed by a loop condition: aborted, even if the condition would test the reason.
	f = &fake{reply: obstacleOn("drive")}
	res = Run(context.Background(), prog(t, `[{"id":"u","type":"repeat_until","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"obstacle"},"body":[`+call("d", "drive", `{"cm":50}`)+`]}]`), f, Options{})
	if res.End != EndAborted || res.Block != "d" || res.Steps != 1 {
		t.Errorf("loop condition: %+v", res)
	}

	// The last block of the program: nobody handles it.
	f = &fake{reply: obstacleOn("drive")}
	if res = Run(context.Background(), prog(t, `[`+call("d", "drive", `{"cm":50}`)+`]`), f, Options{}); res.End != EndAborted {
		t.Errorf("last block: %+v", res)
	}
}

func TestHardErrorsAlwaysAbort(t *testing.T) {
	for _, reason := range []string{"offline", "timeout", "reassigned"} {
		f := &fake{reply: func(string, map[string]any, int) Outcome {
			return Outcome{Result: json.RawMessage(`{"ok":false,"reason":"` + reason + `","error":"rot ist nicht verbunden."}`), Hard: true}
		}}
		// Even an if with an else right after it does not help.
		res := Run(context.Background(), prog(t, `[`+call("d", "drive", `{"cm":50}`)+`,{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"`+reason+`"},"body":[],"else":[]}]`), f, Options{})
		if res.End != EndAborted || res.Block != "d" || res.Reason != reason || !strings.Contains(res.Error, "nicht verbunden") {
			t.Errorf("%s: %+v", reason, res)
		}
	}
}

func TestLimits(t *testing.T) {
	// 201 steps: 20 × 11 calls.
	body := strings.Repeat(call("x", "read_sensors", `{}`)+",", 10) + call("x", "read_sensors", `{}`)
	body = strings.ReplaceAll(body, `"id":"x"`, `"id":"x%d"`)
	ids := make([]any, 11)
	for i := range ids {
		ids[i] = i
	}
	p := prog(t, `[{"id":"r","type":"repeat","times":20,"body":[`+fmt.Sprintf(body, ids...)+`]}]`)
	res := Run(context.Background(), p, &fake{}, Options{})
	if res.End != EndLimit || res.Steps != 200 || !strings.Contains(res.Error, "200") {
		t.Errorf("201 steps: %+v", res)
	}

	// Time: a run longer than the limit (scaled down).
	slow := &fake{reply: func(string, map[string]any, int) Outcome {
		time.Sleep(30 * time.Millisecond)
		return ok(`{"ok":true,"reason":"done"}`)
	}}
	res = Run(context.Background(), prog(t, `[{"id":"r","type":"repeat","times":20,"body":[`+call("d", "drive", `{"cm":10}`)+`]}]`), slow, Options{MaxTime: 100 * time.Millisecond})
	if res.End != EndLimit || res.Steps >= 20 || !strings.Contains(res.Error, "Zeit") {
		t.Errorf("time limit: %+v", res)
	}

	// Depth 5 is refused, depth 4 is fine.
	nest := func(depth int) string {
		s := call("d", "drive", `{"cm":10}`)
		for i := depth; i > 0; i-- {
			s = fmt.Sprintf(`{"id":"r%d","type":"repeat","times":2,"body":[%s]}`, i, s)
		}
		return `{"name":"tief","body":[` + s + `]}`
	}
	p4, _ := Parse([]byte(nest(4)))
	p5, _ := Parse([]byte(nest(5)))
	if problems := Validate(p4, registry()); len(problems) != 0 {
		t.Errorf("depth 4: %+v", problems)
	}
	if problems := Validate(p5, registry()); len(problems) != 1 || problems[0].Block != "r5" {
		t.Errorf("depth 5: %+v", problems)
	}
}

func TestStopDuringALongCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fake{block: "drive_until"}
	done := make(chan Result)
	go func() {
		done <- Run(ctx, prog(t, `[`+call("d", "drive_until", `{"max_cm":300}`)+`,`+call("t", "turn", `{"degrees":90}`)+`]`), f, Options{})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case res := <-done:
		if res.End != EndStopped || res.OK || res.Block != "d" || f.log() != `drive_until{"max_cm":300}` {
			t.Errorf("result = %+v, calls %s", res, f.log())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the run did not stop")
	}
}

func TestSingleStepHaltsBeforeEachCall(t *testing.T) {
	step := make(chan struct{})
	waits := make(chan string, 8)
	f := &fake{}
	done := make(chan Result)
	go func() {
		done <- Run(context.Background(), prog(t, `[`+call("a", "drive", `{"cm":10}`)+`,`+call("b", "turn", `{"degrees":90}`)+`]`), f, Options{Step: step, Observe: func(e Event) {
			if e.Phase == PhaseWait {
				waits <- e.Block
			}
		}})
	}()
	for _, want := range []string{"a", "b"} {
		if got := <-waits; got != want {
			t.Fatalf("waiting at %s, want %s", got, want)
		}
		if n := len(strings.Fields(f.log())); (want == "a" && n != 0) || (want == "b" && n != 1) {
			t.Fatalf("a call ran before its step: %s", f.log())
		}
		step <- struct{}{}
	}
	if res := <-done; res.End != EndDone || res.Steps != 2 {
		t.Errorf("result = %+v", res)
	}
}

func TestDisplayToolDoesNotReplaceTheLastResult(t *testing.T) {
	f := &fake{reply: func(tool string, _ map[string]any, _ int) Outcome {
		switch tool {
		case "drive_until":
			return ok(`{"ok":true,"reason":"wall"}`)
		case "set_leds":
			o := ok(`{"ok":true,"reason":"done"}`)
			o.Quiet = true
			return o
		}
		return ok(`{"ok":true,"reason":"done"}`)
	}}
	p := prog(t, `[`+call("d", "drive_until", `{"max_cm":100}`)+`,`+call("l", "set_leds", `{"color":"red"}`)+`,{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"wall"},"body":[`+call("t", "turn", `{"degrees":90}`)+`]}]`)
	res := Run(context.Background(), p, f, Options{})
	if res.End != EndDone || !strings.HasSuffix(f.log(), `turn{"degrees":90}`) {
		t.Errorf("result %+v, calls %s", res, f.log())
	}
}

func TestReturn(t *testing.T) {
	f := &fake{reply: func(string, map[string]any, int) Outcome { return ok(`{"ok":true,"reason":"color","color":"blue"}`) }}
	res := Run(context.Background(), prog(t, `[`+call("d", "drive_until", `{"max_cm":100,"stop_on_color":"blue"}`)+`,{"id":"g","type":"return","return":{"field":"color"}},`+call("t", "turn", `{"degrees":90}`)+`]`), f, Options{})
	if res.End != EndDone || res.Value != "blue" || res.Steps != 1 || res.Block != "g" {
		t.Errorf("return field: %+v", res)
	}
	res = Run(context.Background(), prog(t, `[{"id":"g","type":"return","return":{"text":"Ziel erreicht"}}]`), &fake{}, Options{})
	if res.Value != "Ziel erreicht" {
		t.Errorf("return text: %+v", res)
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"value":"Ziel erreicht"`) || !strings.Contains(string(b), `"runtime_s"`) {
		t.Errorf("json = %s", b)
	}
}

func TestValidation(t *testing.T) {
	cat := registry()
	cases := []struct {
		name, program, block, want string
	}{
		{"agent-only tool", `{"name":"abc","body":[` + call("f", "finish", `{"report":"x"}`) + `]}`, "f", "nur der Agent"},
		{"unknown tool", `{"name":"abc","body":[` + call("f", "fly", `{}`) + `]}`, "f", "gibt es nicht"},
		{"value out of range", `{"name":"abc","body":[` + call("d", "drive", `{"cm":500}`) + `]}`, "d", "zwischen -300 und 300"},
		{"missing argument", `{"name":"abc","body":[` + call("d", "drive", `{}`) + `]}`, "d", `"cm"`},
		{"robot in a block", `{"name":"abc","body":[` + call("d", "drive", `{"cm":5,"robot":"rot"}`) + `]}`, "d", "Roboter"},
		{"text too long", `{"name":"abc","body":[` + call("s", "show_text", `{"text":"`+strings.Repeat("x", 61)+`"}`) + `]}`, "s", "60 Zeichen"},
		{"name rule", `{"name":"Fahre!","body":[` + call("d", "drive", `{"cm":5}`) + `]}`, "", "3 bis 30"},
		{"built-in name", `{"name":"drive_until","body":[` + call("d", "drive", `{"cm":5}`) + `]}`, "", "eingebautes"},
		{"planned name", `{"name":"gesture","body":[` + call("d", "drive", `{"cm":5}`) + `]}`, "", "eingebautes"},
		{"description", `{"name":"abc","description":"` + strings.Repeat("x", 201) + `","body":[` + call("d", "drive", `{"cm":5}`) + `]}`, "", "201 Zeichen"},
		{"empty", `{"name":"abc","body":[]}`, "", "leer"},
		{"contains on reason", `{"name":"abc","body":[{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"contains","value":"w"},"body":[]}]}`, "c", "Boden"},
		{"lt on text", `{"name":"abc","body":[{"id":"i","type":"if","cond":{"id":"c","source":"sensor","field":"distance_cm","op":"lt","value":"nah"},"body":[]}]}`, "c", "Zahl"},
		{"unknown sensor", `{"name":"abc","body":[{"id":"i","type":"if","cond":{"id":"c","source":"sensor","field":"light","op":"lt","value":3},"body":[]}]}`, "c", "light"},
		{"duplicate id", `{"name":"abc","body":[` + call("d", "drive", `{"cm":5}`) + `,` + call("d", "drive", `{"cm":5}`) + `]}`, "d", "zweimal"},
	}
	for _, c := range cases {
		p, err := Parse([]byte(c.program))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		problems := Validate(p, cat)
		found := false
		for _, pr := range problems {
			if pr.Block == c.block && strings.Contains(pr.Text, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: problems %+v, want %q at %q", c.name, problems, c.want, c.block)
		}
	}
	// 61 blocks are too many.
	var calls []string
	for i := range 61 {
		calls = append(calls, call(fmt.Sprintf("b%d", i), "drive", `{"cm":5}`))
	}
	p, _ := Parse([]byte(`{"name":"lang","body":[` + strings.Join(calls, ",") + `]}`))
	if problems := Validate(p, cat); len(problems) != 1 || !strings.Contains(problems[0].Text, "61 Blöcke") {
		t.Errorf("61 blocks: %+v", problems)
	}
}

func TestExamplesAreValid(t *testing.T) {
	files, _ := filepath.Glob("../../../docs/examples/programs/*.json")
	if len(files) < 3 {
		t.Fatalf("examples: %v", files)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var ex struct {
			Program json.RawMessage `json:"program"`
		}
		if err := json.Unmarshal(raw, &ex); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		p, err := Parse(ex.Program)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if problems := Validate(p, registry()); len(problems) > 0 {
			t.Errorf("%s: %+v", file, problems)
		}
		if p.Name+".json" != filepath.Base(file) && p.Name != "wand_ausweichen" {
			t.Errorf("%s: name %q", file, p.Name)
		}
		back, _ := json.Marshal(p)
		if q, _ := Parse(back); fmt.Sprint(q.Body[0].Else == nil) != fmt.Sprint(p.Body[0].Else == nil) {
			t.Errorf("%s: else lost in a round trip", file)
		}
	}
}

func TestEmptyElseSurvivesJSON(t *testing.T) {
	p, _ := Parse([]byte(`{"name":"abc","body":[{"id":"i","type":"if","cond":{"id":"c","source":"result","field":"reason","op":"eq","value":"wall"},"body":[],"else":[]}]}`))
	if !p.Body[0].HasElse() {
		t.Fatal("an empty else must count as an else")
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"else":[]`) {
		t.Errorf("json = %s", b)
	}
	q, _ := Parse(b)
	if !q.Body[0].HasElse() {
		t.Error("empty else lost in a round trip")
	}
}

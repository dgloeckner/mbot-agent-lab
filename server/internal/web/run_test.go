package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// programEvent reads the next SSE "program" event.
func programEvent(t *testing.T, events <-chan string, match func(map[string]any) bool) map[string]any {
	t.Helper()
	var got map[string]any
	waitEvent(t, events, func(e string) bool {
		if !strings.HasPrefix(e, "program ") {
			return false
		}
		got = nil
		_ = json.Unmarshal([]byte(e[len("program "):]), &got)
		return match(got)
	})
	return got
}

func phase(p string) func(map[string]any) bool {
	return func(e map[string]any) bool { return e["phase"] == p }
}

func TestRunTeamTool(t *testing.T) {
	bus := &robotBus{}
	srv := newTestServerBus(t, bus)
	_, p := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "Fährt ein Quadrat.", exampleBlockly(t, "quadrat")))
	id := p["id"].(string)
	events := sse(t, srv, "tok-rot", "")

	code, body := do(t, srv, "POST", "/t/tok-rot/programs/"+id+"/run", `{}`)
	if code != http.StatusAccepted || body["run"] == nil {
		t.Fatalf("run: %d %v", code, body)
	}
	run := body["run"].(string)
	programEvent(t, events, phase("start"))
	var blocks []string
	var end map[string]any
	for end == nil {
		e := programEvent(t, events, func(map[string]any) bool { return true })
		if e["run"] != run {
			t.Fatalf("event of another run: %v", e)
		}
		switch e["phase"] {
		case "before", "after":
			blocks = append(blocks, e["phase"].(string)[:1]+":"+e["block"].(string))
		case "end":
			end = e["result"].(map[string]any)
		}
	}
	want := strings.Repeat("b:q_drive a:q_drive b:q_turn a:q_turn ", 4)
	if strings.Join(blocks, " ")+" " != want || end["end"] != "done" || end["steps"] != 8.0 {
		t.Errorf("events %v, end %v", blocks, end)
	}
	if n := strings.Count(strings.Join(bus.sent(), ","), "mbot-1:drive"); n != 4 {
		t.Errorf("drives sent: %d (%v)", n, bus.sent())
	}
}

func TestRunTeamToolSingleStepAndStop(t *testing.T) {
	bus := &robotBus{}
	srv := newTestServerBus(t, bus)
	_, p := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "", exampleBlockly(t, "quadrat")))
	id := p["id"].(string)
	events := sse(t, srv, "tok-rot", "")

	// Single step: it halts before each call until "continue".
	_, body := do(t, srv, "POST", "/t/tok-rot/programs/"+id+"/run", `{"step":true}`)
	run := body["run"].(string)
	for i, block := range []string{"q_drive", "q_turn", "q_drive"} {
		e := programEvent(t, events, phase("wait"))
		if e["block"] != block {
			t.Fatalf("waiting at %v, want %s", e["block"], block)
		}
		if n := motions(bus.sent()); n != i {
			t.Fatalf("a call ran before continue: %v", bus.sent())
		}
		if code, _ := do(t, srv, "POST", "/t/tok-rot/program-runs/"+run+"/continue", ""); code != http.StatusNoContent {
			t.Fatalf("continue: %d", code)
		}
		programEvent(t, events, phase("after"))
	}
	// Stop during a long command: the robot gets stop, the run ends stopped.
	bus.setHold(true)
	programEvent(t, events, phase("wait"))
	do(t, srv, "POST", "/t/tok-rot/program-runs/"+run+"/continue", "")
	deadline := time.Now().Add(3 * time.Second)
	for bus.holding() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")
	end := programEvent(t, events, phase("end"))["result"].(map[string]any)
	if end["end"] != "stopped" || !strings.Contains(strings.Join(bus.sent(), ","), "mbot-1:stop") {
		t.Errorf("end %v, sent %v", end, bus.sent())
	}
	bus.setHold(false)
	if code, _ := do(t, srv, "POST", "/t/tok-rot/program-runs/"+run+"/continue", ""); code != http.StatusConflict {
		t.Errorf("continue after the end: %d", code)
	}
}

func TestRunTeamToolConflicts(t *testing.T) {
	bus := &robotBus{}
	srv := newTestServerBus(t, bus)
	_, p := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "", exampleBlockly(t, "quadrat")))
	id := p["id"].(string)
	_, bad := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("Kaputt!", "", exampleBlockly(t, "quadrat")))

	if code, got := do(t, srv, "POST", "/t/tok-rot/programs/"+bad["id"].(string)+"/run", `{}`); code != http.StatusConflict || !strings.Contains(got["error"].(string), "Fehler") {
		t.Errorf("invalid tool: %d %v", code, got)
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/programs/pnope/run", `{}`); code != http.StatusNotFound {
		t.Errorf("unknown tool: %d", code)
	}

	// While another team tool runs, and start during it.
	bus.setHold(true)
	if code, _ := do(t, srv, "POST", "/t/tok-rot/programs/"+id+"/run", `{}`); code != http.StatusAccepted {
		t.Fatal("first run refused")
	}
	if code, got := do(t, srv, "POST", "/t/tok-rot/programs/"+id+"/run", `{}`); code != http.StatusConflict || !strings.Contains(got["error"].(string), "läuft schon") {
		t.Errorf("second run: %d %v", code, got)
	}
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Los."}`)
	if code, _ := do(t, srv, "POST", "/t/tok-rot/start", ""); code != http.StatusConflict {
		t.Errorf("start during a team tool: %d", code)
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")
	bus.setHold(false)

	// During an agent run.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if code, _ := do(t, srv, "POST", "/t/tok-rot/start", ""); code == http.StatusAccepted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("start stayed refused after the team tool ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, got := do(t, srv, "POST", "/t/tok-rot/programs/"+id+"/run", `{}`); code != http.StatusConflict || got["error"] != "Erst anhalten: Euer Agent fährt gerade." {
		t.Errorf("during a run: %d %v", code, got)
	}
}

// motions counts the drive and turn commands sent.
func motions(sent []string) int {
	n := 0
	for _, c := range sent {
		if strings.HasSuffix(c, ":drive") || strings.HasSuffix(c, ":turn") {
			n++
		}
	}
	return n
}

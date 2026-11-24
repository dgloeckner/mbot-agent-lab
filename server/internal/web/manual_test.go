package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRunToolByHand(t *testing.T) {
	bus := &robotBus{}
	srv := newTestServerBus(t, bus)
	events := sse(t, srv, "tok-rot", "")

	code, res := do(t, srv, "POST", "/t/tok-rot/tools/drive", `{"cm":20}`)
	if code != http.StatusOK || res["ok"] != true || res["reason"] != "done" || res["robot"] != "rot" {
		t.Fatalf("drive by hand: %d %v", code, res)
	}
	got := waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "log ") && strings.Contains(e, `"kind":"tool"`) })
	if !strings.Contains(got, `"source":"team"`) || !strings.Contains(got, `"name":"drive"`) {
		t.Errorf("log entry = %s", got)
	}

	// Mistakes in the arguments are results, not server errors.
	for _, body := range []string{`{}`, `kein json`, `{"cm":"weit"}`} {
		code, res = do(t, srv, "POST", "/t/tok-rot/tools/drive", body)
		if code != http.StatusOK || res["ok"] != false || res["error"] == nil {
			t.Errorf("drive %s: %d %v", body, code, res)
		}
	}
	code, res = do(t, srv, "POST", "/t/tok-rot/tools/finish", `{"report":"x"}`)
	if code != http.StatusOK || res["ok"] != false || !strings.Contains(res["error"].(string), "nur euer Agent") {
		t.Errorf("finish by hand: %d %v", code, res)
	}

	// With two robots, robot is required.
	if err := srv.lab.AssignRobot(context.Background(), "mbot-3", "rot"); err != nil {
		t.Fatal(err)
	}
	if code, res = do(t, srv, "POST", "/t/tok-rot/tools/turn", `{"degrees":90}`); res["ok"] != false || !strings.Contains(res["error"].(string), "robot") {
		t.Errorf("turn without robot: %d %v", code, res)
	}
	if code, res = do(t, srv, "POST", "/t/tok-rot/tools/turn", `{"degrees":90,"robot":"grün"}`); res["ok"] != true || res["robot"] != "grün" {
		t.Errorf("turn grün: %d %v", code, res)
	}
}

func TestRunToolByHandConflicts(t *testing.T) {
	bus := &robotBus{}
	srv := newTestServerBus(t, bus)

	// While a run is active.
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Erkunde."}`)
	if code, _ := do(t, srv, "POST", "/t/tok-rot/start", ""); code != http.StatusAccepted {
		t.Fatalf("start: %d", code)
	}
	code, res := do(t, srv, "POST", "/t/tok-rot/tools/drive", `{"cm":20}`)
	if code != http.StatusConflict || res["error"] != "Erst anhalten: Euer Agent fährt gerade." {
		t.Errorf("during a run: %d %v", code, res)
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")
	waitRunning(t, srv, "tok-rot", false)

	// Start while a by-hand call runs; stop cancels the call.
	bus.setHold(true)
	done := make(chan map[string]any, 1)
	go func() {
		_, res := do(t, srv, "POST", "/t/tok-rot/tools/drive", `{"cm":50}`)
		done <- res
	}()
	deadline := time.Now().Add(3 * time.Second)
	for bus.holding() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	code, res = do(t, srv, "POST", "/t/tok-rot/start", "")
	if code != http.StatusConflict || !strings.Contains(res["error"].(string), "selbst") {
		t.Errorf("start during a by-hand call: %d %v", code, res)
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")
	select {
	case res = <-done:
		if res["ok"] != false || res["reason"] != "stopped" {
			t.Errorf("stopped call: %v", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not end the by-hand call")
	}
	bus.setHold(false)

	// Without a task, and in a closed task.
	srv.st.DB.Exec(`DELETE FROM active_tasks WHERE team_id = 'blau'`)
	if code, res = do(t, srv, "POST", "/t/tok-blau/tools/drive", `{"cm":20}`); code != http.StatusConflict || !strings.Contains(res["error"].(string), "Aufgabe") {
		t.Errorf("without a task: %d %v", code, res)
	}
	c := adminClient(t, srv)
	adminCall(t, c, srv, "POST", "/admin/api/tasks/"+testTask+"/close", "")
	if code, res = do(t, srv, "POST", "/t/tok-rot/tools/drive", `{"cm":20}`); code != http.StatusConflict || !strings.Contains(res["error"].(string), "geschlossen") {
		t.Errorf("closed task: %d %v", code, res)
	}
}

func waitRunning(t *testing.T, srv *testLab, token string, want bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, sess := do(t, srv, "GET", "/t/"+token+"/api/session", "")
		if sess["agent"].(map[string]any)["running"] == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("running did not become %v", want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

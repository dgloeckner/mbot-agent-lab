package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeRoutes change something in the active task.
var writeRoutes = []struct{ method, path, body string }{
	{"POST", "/prompt", `{"text":"Fahr los"}`},
	{"POST", "/notes", `{"text":"Wand links"}`},
	{"PUT", "/notes/1", `{"text":"Wand rechts"}`},
	{"DELETE", "/notes/1", ``},
	{"POST", "/start", ``},
	{"POST", "/message", `{"text":"Hallo"}`},
	{"POST", "/conversation/new", ``},
}

// newTask creates, publishes and assigns a task and returns its ID.
func newTask(t *testing.T, srv *testLab, c *http.Client, title, starter, teams string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"title": title, "body": "Text von " + title, "starter": starter})
	code, task := adminCall(t, c, srv, "POST", "/admin/api/tasks", string(body))
	if code != http.StatusCreated {
		t.Fatalf("create %s: %d %v", title, code, task)
	}
	id := field(task, "id").(string)
	adminCall(t, c, srv, "POST", "/admin/api/tasks/"+id+"/publish", "")
	if code, b := adminCall(t, c, srv, "POST", "/admin/api/tasks/"+id+"/assign", `{"teams":`+teams+`}`); code != http.StatusOK {
		t.Fatalf("assign %s: %d %v", title, code, b)
	}
	return id
}

func TestTeamWithoutTask(t *testing.T) {
	srv := newTestServer(t)
	_, team := do(t, srv, "POST", "/api/join", `{"code":"TEST","name":"Neulinge"}`)
	base := "/t/" + team["token"].(string)

	code, sess := do(t, srv, "GET", base+"/api/session", "")
	if code != http.StatusOK || sess["task"] != nil || len(sess["tasks"].([]any)) != 0 {
		t.Fatalf("session without task: %d %v", code, sess)
	}
	for _, r := range append(writeRoutes, struct{ method, path, body string }{"GET", "/prompts", ""}, struct{ method, path, body string }{"GET", "/notes", ""}) {
		code, body := do(t, srv, r.method, base+r.path, r.body)
		if code != http.StatusConflict || body["error"] != "Eurem Team ist noch keine Aufgabe zugewiesen." {
			t.Errorf("%s %s without task: %d %v", r.method, r.path, code, body)
		}
	}
}

func TestAcceptSwitchAndScope(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	a := newTask(t, srv, c, "Labyrinth", "Starte im Labyrinth.", `["rot"]`)
	b := newTask(t, srv, c, "Rennen", "", `"all"`)

	code, list := do(t, srv, "GET", "/t/tok-rot/tasks", "")
	if code != http.StatusOK || list["active"] != testTask || len(list["tasks"].([]any)) != 3 {
		t.Fatalf("tasks: %d %v", code, list)
	}
	if code, task := do(t, srv, "GET", "/t/tok-rot/tasks/"+a, ""); code != http.StatusOK || task["body"] != "Text von Labyrinth" || task["status"] != "assigned" {
		t.Errorf("task text: %d %v", code, task)
	}
	if code, _ := do(t, srv, "GET", "/t/tok-blau/tasks/"+a, ""); code != http.StatusNotFound {
		t.Errorf("another team's task: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/tasks/"+b+"/activate", ""); code != http.StatusConflict {
		t.Errorf("activate before accept: %d", code)
	}

	// Accept a with the starter prompt: version 1 of the new pair.
	if code, body := do(t, srv, "POST", "/t/tok-rot/tasks/"+a+"/accept", `{"prompt":"starter"}`); code != http.StatusOK || body["status"] != "active" {
		t.Fatalf("accept: %d %v", code, body)
	}
	do(t, srv, "POST", "/t/tok-rot/notes", `{"text":"Notiz in A"}`)
	_, sess := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	if field(sess["task"], "id") != a || field(sess["prompt"], "text") != "Starte im Labyrinth." || len(sess["notes"].([]any)) != 1 {
		t.Errorf("session in a = task %v prompt %v notes %v", sess["task"], sess["prompt"], sess["notes"])
	}

	// Accept b with a copy of a's prompt, then switch back and forth.
	if code, body := do(t, srv, "POST", "/t/tok-rot/tasks/"+b+"/accept", `{"prompt":"copy","from_task":"`+a+`"}`); code != http.StatusOK {
		t.Fatalf("accept copy: %d %v", code, body)
	}
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Rennen fahren"}`)
	_, sess = do(t, srv, "GET", "/t/tok-rot/api/session", "")
	if field(sess["prompt"], "version") != 2.0 || len(sess["notes"].([]any)) != 0 {
		t.Errorf("session in b = prompt %v notes %v", sess["prompt"], sess["notes"])
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/tasks/"+a+"/activate", ""); code != http.StatusOK {
		t.Fatalf("switch to a: %d", code)
	}
	_, sess = do(t, srv, "GET", "/t/tok-rot/api/session", "")
	if field(sess["prompt"], "text") != "Starte im Labyrinth." || len(sess["versions"].([]any)) != 1 {
		t.Errorf("back in a = prompt %v versions %v", sess["prompt"], sess["versions"])
	}

	// Switching is refused during a run.
	do(t, srv, "POST", "/t/tok-rot/start", "")
	code, body := do(t, srv, "POST", "/t/tok-rot/tasks/"+b+"/activate", "")
	if code != http.StatusConflict || !strings.Contains(body["error"].(string), "Lauf") {
		t.Errorf("switch during a run: %d %v", code, body)
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")
}

func TestClosedTaskIsReadOnly(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Fahr los"}`)
	do(t, srv, "POST", "/t/tok-rot/notes", `{"text":"Wand links"}`)
	adminCall(t, c, srv, "POST", "/admin/api/tasks/"+testTask+"/close", "")

	for _, path := range []string{"/api/session", "/prompts", "/prompts/1", "/notes", "/tasks", "/tasks/" + testTask} {
		if code, _ := do(t, srv, "GET", "/t/tok-rot"+path, ""); code != http.StatusOK {
			t.Errorf("GET %s in a closed task: %d", path, code)
		}
	}
	for _, r := range writeRoutes {
		code, body := do(t, srv, r.method, "/t/tok-rot"+r.path, r.body)
		if code != http.StatusConflict || !strings.Contains(body["error"].(string), "geschlossen") {
			t.Errorf("%s %s in a closed task: %d %v", r.method, r.path, code, body)
		}
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/stop", ""); code != http.StatusOK {
		t.Errorf("stop in a closed task: %d", code)
	}
}

// sse reads a team's event stream and sends "event data" lines.
func sse(t *testing.T, srv *testLab, token, lastID string) <-chan string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+"/t/"+token+"/events", nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	out := make(chan string, 256)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		event := ""
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "event: ") {
				event = line[7:]
			} else if strings.HasPrefix(line, "data: ") {
				out <- event + " " + line[6:]
			}
		}
	}()
	return out
}

func waitEvent(t *testing.T, ch <-chan string, match func(string) bool) string {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case e := <-ch:
			if match(e) {
				return e
			}
		case <-timeout:
			t.Fatal("event did not arrive")
		}
	}
}

func TestSwitchSignalsAndScopesTheLog(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	b := newTask(t, srv, c, "Rennen", "", `["rot"]`)
	do(t, srv, "POST", "/t/tok-rot/notes", `{"text":"x"}`)
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Fahr in Aufgabe A"}`)

	events := sse(t, srv, "tok-rot", "")
	waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "log ") && strings.Contains(e, `"kind":"deploy"`) })

	do(t, srv, "POST", "/t/tok-rot/tasks/"+b+"/accept", `{"prompt":"empty"}`)
	waitEvent(t, events, func(e string) bool { return e == `task {"task_id":"`+b+`"}` })
	waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "tasks ") })
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Fahr in Aufgabe B"}`)
	got := waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "log ") && strings.Contains(e, `"kind":"deploy"`) })
	if !strings.Contains(got, `"task_id":"`+b+`"`) {
		t.Errorf("log entry after the switch = %s", got)
	}

	// A page that (re)connects after the switch sees only the new task's entries.
	fresh := sse(t, srv, "tok-rot", "0")
	time.Sleep(200 * time.Millisecond)
	for len(fresh) > 0 {
		if e := <-fresh; strings.HasPrefix(e, "log ") && !strings.Contains(e, `"task_id":"`+b+`"`) {
			t.Errorf("backlog after the switch holds %s", e)
		}
	}
}

// The page tells a dead connection from silence by the named keep-alive;
// a stream it opens itself catches up with ?after= like with Last-Event-ID.
func TestEventsKeepAliveAndAfter(t *testing.T) {
	srv := newTestServer(t)
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Version eins"}`)
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Version zwei"}`)
	events := sse(t, srv, "tok-rot", "")
	if got := waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "ping ") }); got != `ping {"every_s":10}` {
		t.Errorf("keep-alive = %q", got)
	}
	first := waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "log ") && strings.Contains(e, `"kind":"deploy"`) })
	var entry struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(strings.TrimPrefix(first, "log ")), &entry)

	req, _ := http.NewRequest("GET", srv.URL+"/t/tok-rot/events?after="+strconv.FormatInt(entry.ID, 10), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	var deploys []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(deploys) < 1 && sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "data: ") && strings.Contains(line, `"kind":"deploy"`) {
			deploys = append(deploys, line)
		}
	}
	if len(deploys) != 1 || !strings.Contains(deploys[0], `"version":2`) {
		t.Errorf("after %d: %v", entry.ID, deploys)
	}
}

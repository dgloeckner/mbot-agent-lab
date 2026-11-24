package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// adminCall sends an admin request and decodes the JSON answer.
func adminCall(t *testing.T, c *http.Client, srv *testLab, method, path, body string) (int, any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func field(v any, key string) any {
	m, _ := v.(map[string]any)
	return m[key]
}

func TestTaskRoutesNeedAdmin(t *testing.T) {
	srv := newTestServer(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/admin/api/tasks"}, {"POST", "/admin/api/tasks"}, {"GET", "/admin/api/tasks/a1"},
		{"PUT", "/admin/api/tasks/a1"}, {"DELETE", "/admin/api/tasks/a1"}, {"POST", "/admin/api/tasks/a1/publish"},
		{"POST", "/admin/api/tasks/a1/close"}, {"POST", "/admin/api/tasks/a1/duplicate"},
		{"POST", "/admin/api/tasks/a1/assign"}, {"DELETE", "/admin/api/tasks/a1/assign/rot"},
	} {
		if code, _ := do(t, srv, r.method, r.path, `{}`); code != http.StatusUnauthorized {
			t.Errorf("%s %s without login: %d", r.method, r.path, code)
		}
	}
}

func TestTaskRoutes(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)

	code, body := adminCall(t, c, srv, "POST", "/admin/api/tasks", `{"title":""}`)
	if code != http.StatusBadRequest || !strings.Contains(field(body, "error").(string), "Titel") {
		t.Errorf("create without title: %d %v", code, body)
	}
	code, body = adminCall(t, c, srv, "POST", "/admin/api/tasks", `{"title":"Labyrinth","body":"Findet den **Ausgang**.","starter":"Fahr los."}`)
	if code != http.StatusCreated || field(body, "state") != "draft" {
		t.Fatalf("create: %d %v", code, body)
	}
	id := field(body, "id").(string)
	path := "/admin/api/tasks/" + id

	if code, body = adminCall(t, c, srv, "PUT", path, `{"title":"Labyrinth 2","body":"Text","starter":""}`); code != http.StatusOK || field(body, "title") != "Labyrinth 2" {
		t.Errorf("update draft: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "GET", path, ""); code != http.StatusOK || field(body, "body") != "Text" {
		t.Errorf("get: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "POST", path+"/assign", `{"teams":"all"}`); code != http.StatusConflict || !strings.Contains(field(body, "error").(string), "Veröffentlicht") {
		t.Errorf("assign a draft: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "POST", path+"/publish", ""); code != http.StatusOK || field(body, "state") != "open" {
		t.Fatalf("publish: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "PUT", path, `{"title":"Anders","body":"x"}`); code != http.StatusConflict || !strings.Contains(field(body, "error").(string), "Entwürfe") {
		t.Errorf("edit published: %d %v", code, body)
	}
	if code, _ = adminCall(t, c, srv, "DELETE", path, ""); code != http.StatusConflict {
		t.Errorf("delete published: %d", code)
	}

	// Assign to one team, then to all; a bad target is refused.
	if code, body = adminCall(t, c, srv, "POST", path+"/assign", `{"teams":["rot"]}`); code != http.StatusOK {
		t.Fatalf("assign rot: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "POST", path+"/assign", `{"teams":"all"}`); code != http.StatusOK || len(field(body, "assigned").([]any)) != 1 {
		t.Errorf("assign all: %d %v", code, body)
	}
	if code, _ = adminCall(t, c, srv, "POST", path+"/assign", `{"teams":"irgendwer"}`); code != http.StatusBadRequest {
		t.Errorf("bad target: %d", code)
	}
	if code, _ = adminCall(t, c, srv, "POST", path+"/assign", `{"teams":["nope"]}`); code != http.StatusNotFound {
		t.Errorf("unknown team: %d", code)
	}

	// The board shows per team: assigned tasks with their status.
	_, state := adminCall(t, c, srv, "GET", "/admin/api/state", "")
	status := map[string]map[string]string{}
	for _, tm := range field(state, "teams").([]any) {
		status[field(tm, "id").(string)] = map[string]string{}
		for _, tk := range field(tm, "tasks").([]any) {
			status[field(tm, "id").(string)][field(tk, "id").(string)] = field(tk, "status").(string)
		}
	}
	if status["rot"][id] != "assigned" || status["rot"][testTask] != "active" || status["blau"][id] != "assigned" {
		t.Errorf("board task status = %v", status)
	}
	if n := len(field(state, "tasks").([]any)); n != 2 {
		t.Errorf("board tasks = %d", n)
	}

	// Withdraw while not accepted; after accepting it is refused.
	if code, _ = adminCall(t, c, srv, "DELETE", path+"/assign/blau", ""); code != http.StatusNoContent {
		t.Errorf("withdraw: %d", code)
	}
	if err := srv.st.AcceptTask(t.Context(), store.TeamTask{Team: "rot", Task: id}); err != nil {
		t.Fatal(err)
	}
	if code, body = adminCall(t, c, srv, "DELETE", path+"/assign/rot", ""); code != http.StatusConflict || !strings.Contains(field(body, "error").(string), "angenommen") {
		t.Errorf("withdraw after accept: %d %v", code, body)
	}

	// Duplicate gives a draft that can be deleted; unknown tasks are 404.
	code, body = adminCall(t, c, srv, "POST", path+"/duplicate", "")
	if code != http.StatusCreated || field(body, "state") != "draft" || field(body, "body") != "Text" {
		t.Fatalf("duplicate: %d %v", code, body)
	}
	if code, _ = adminCall(t, c, srv, "DELETE", "/admin/api/tasks/"+field(body, "id").(string), ""); code != http.StatusNoContent {
		t.Errorf("delete draft: %d", code)
	}
	if code, body = adminCall(t, c, srv, "GET", "/admin/api/tasks/nope", ""); code != http.StatusNotFound || field(body, "error") != "Diese Aufgabe gibt es nicht." {
		t.Errorf("unknown task: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "POST", path+"/close", ""); code != http.StatusOK || field(body, "state") != "closed" {
		t.Errorf("close: %d %v", code, body)
	}
	if code, body = adminCall(t, c, srv, "GET", "/admin/api/tasks", ""); code != http.StatusOK || len(body.([]any)) != 2 {
		t.Errorf("list: %d %v", code, body)
	}
}

func TestClosingATaskEndsItsRun(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Erkunde die Arena."}`)
	if code, body := do(t, srv, "POST", "/t/tok-rot/start", ""); code != http.StatusAccepted {
		t.Fatalf("start: %d %v", code, body)
	}
	if code, _ := adminCall(t, c, srv, "POST", "/admin/api/tasks/"+testTask+"/close", ""); code != http.StatusOK {
		t.Fatalf("close: %d", code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		runs, _ := srv.st.Runs(t.Context(), store.TeamTask{Team: "rot", Task: testTask}, 1)
		if len(runs) == 1 && runs[0].EndReason != "" {
			if runs[0].EndReason != "task_closed" {
				t.Errorf("run ended with %q", runs[0].EndReason)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not end")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

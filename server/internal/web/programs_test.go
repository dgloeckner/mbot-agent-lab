package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func exampleBlockly(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/examples/programs/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var ex struct {
		Blockly json.RawMessage `json:"blockly"`
	}
	_ = json.Unmarshal(raw, &ex)
	var buf bytes.Buffer
	_ = json.Compact(&buf, ex.Blockly)
	return buf.String()
}

func saveBody(name, desc, blockly string) string {
	b, _ := json.Marshal(map[string]any{"name": name, "description": desc, "blockly": json.RawMessage(blockly)})
	return string(b)
}

func TestProgramRoutes(t *testing.T) {
	srv := newTestServer(t)
	events := sse(t, srv, "tok-rot", "")
	quadrat := exampleBlockly(t, "quadrat")

	code, p := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "Fährt ein Quadrat.", quadrat))
	if code != http.StatusCreated || p["valid"] != true || p["id"] == nil || p["blockly"] == nil {
		t.Fatalf("create: %d %v", code, p)
	}
	id := p["id"].(string)
	waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "programs ") && strings.Contains(e, `"id":"`+id+`"`) })

	// List and read back.
	resp, _ := http.Get(srv.URL + "/t/tok-rot/programs")
	var list []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list) != 1 || list[0]["name"] != "quadrat" || list[0]["valid"] != true || list[0]["blockly"] != nil {
		t.Errorf("list = %v", list)
	}
	if code, got := do(t, srv, "GET", "/t/tok-rot/programs/"+id, ""); code != http.StatusOK || got["blockly"] == nil {
		t.Errorf("get: %d %v", code, got)
	}

	// A second save replaces the first; an invalid tool is stored with its problems per block.
	far := strings.Replace(quadrat, `"cm":30`, `"cm":900`, 1)
	code, p = do(t, srv, "PUT", "/t/tok-rot/programs/"+id, saveBody("quadrat", "Jetzt zu weit.", far))
	problems, _ := p["problems"].([]any)
	if code != http.StatusOK || p["valid"] != false || len(problems) != 1 || problems[0].(map[string]any)["block"] != "q_drive" {
		t.Fatalf("update: %d %v", code, p)
	}
	if _, got := do(t, srv, "GET", "/t/tok-rot/programs/"+id, ""); got["description"] != "Jetzt zu weit." {
		t.Errorf("after update: %v", got)
	}

	// Refusals.
	if code, got := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "", quadrat)); code != http.StatusConflict || !strings.Contains(got["error"].(string), "habt ihr schon") {
		t.Errorf("same name: %d %v", code, got)
	}
	if code, got := do(t, srv, "GET", "/t/tok-rot/programs/pnope", ""); code != http.StatusNotFound || got["error"] != "Dieses Werkzeug gibt es nicht (mehr)." {
		t.Errorf("unknown: %d %v", code, got)
	}
	if code, _ := do(t, srv, "GET", "/t/tok-blau/programs/"+id, ""); code != http.StatusNotFound {
		t.Errorf("another team's tool: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/programs", `kein json`); code != http.StatusBadRequest {
		t.Errorf("bad body: %d", code)
	}

	// In a closed task reads work and writes answer 409.
	c := adminClient(t, srv)
	adminCall(t, c, srv, "POST", "/admin/api/tasks/"+testTask+"/close", "")
	if code, _ := do(t, srv, "GET", "/t/tok-rot/programs/"+id, ""); code != http.StatusOK {
		t.Errorf("read in a closed task: %d", code)
	}
	for _, req := range [][3]string{{"POST", "/t/tok-rot/programs", saveBody("neu", "", quadrat)}, {"PUT", "/t/tok-rot/programs/" + id, saveBody("quadrat", "", quadrat)}, {"DELETE", "/t/tok-rot/programs/" + id, ""}} {
		if code, got := do(t, srv, req[0], req[1], req[2]); code != http.StatusConflict || !strings.Contains(got["error"].(string), "geschlossen") {
			t.Errorf("%s in a closed task: %d %v", req[0], code, got)
		}
	}

	// Without an active task: 409.
	srv.st.DB.Exec(`DELETE FROM active_tasks WHERE team_id = 'blau'`)
	if code, got := do(t, srv, "GET", "/t/tok-blau/programs", ""); code != http.StatusConflict || !strings.Contains(got["error"].(string), "Aufgabe") {
		t.Errorf("without a task: %d %v", code, got)
	}
}

func TestDeleteProgram(t *testing.T) {
	srv := newTestServer(t)
	_, p := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "", exampleBlockly(t, "quadrat")))
	id := p["id"].(string)
	if code, _ := do(t, srv, "DELETE", "/t/tok-rot/programs/"+id, ""); code != http.StatusNoContent {
		t.Errorf("delete: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/t/tok-rot/programs/"+id, ""); code != http.StatusNotFound {
		t.Errorf("delete twice: %d", code)
	}
}

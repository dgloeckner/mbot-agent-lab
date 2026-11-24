package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestToolDiffRoutes(t *testing.T) {
	srv := newTestServer(t)
	quadrat := exampleBlockly(t, "quadrat")
	_, q := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "Fährt ein Quadrat.", quadrat))
	_, w := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("wand", "Fährt zur Wand.", exampleBlockly(t, "wand")))
	qid, wid := q["id"].(string), w["id"].(string)
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Version eins"}`)

	// Version 1 against the working state: a changed value in quadrat, wand deleted, a new tool.
	do(t, srv, "PUT", "/t/tok-rot/programs/"+qid, saveBody("quadrat", "Fährt ein Quadrat.", strings.Replace(quadrat, `"cm":30`, `"cm":40`, 1)))
	do(t, srv, "DELETE", "/t/tok-rot/programs/"+wid, "")
	_, n := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("neu", "Neu.", quadrat))
	nid := n["id"].(string)

	code, list := do(t, srv, "GET", "/t/tok-rot/prompts/1/tools", "")
	if code != http.StatusOK || list["left"] != 1.0 || list["right"] != 0.0 {
		t.Fatalf("tools against now: %d %v", code, list)
	}
	got := map[string]string{}
	for _, c := range list["tools"].([]any) {
		m := c.(map[string]any)
		got[m["name"].(string)] = m["change"].(string)
	}
	if got["quadrat"] != "geändert" || got["wand"] != "entfernt" || got["neu"] != "neu" || len(got) != 3 {
		t.Errorf("tool changes = %v", got)
	}

	code, d := do(t, srv, "GET", "/t/tok-rot/prompts/1/tools/"+qid+"?against=now", "")
	changes := d["changes"].([]any)
	if code != http.StatusOK || len(changes) != 1 {
		t.Fatalf("quadrat diff: %d %v", code, d)
	}
	if c := changes[0].(map[string]any); c["kind"] != "value" || c["block"] != "q_drive" || c["field"] != "cm" || c["old"] != 30.0 || c["new"] != 40.0 {
		t.Errorf("change = %v", c)
	}
	left, right := d["left"].(map[string]any), d["right"].(map[string]any)
	if left["version"] != 1.0 || right["version"] != 0.0 || left["tool"].(map[string]any)["blockly"] == nil {
		t.Errorf("sides = %v / %v", left, right)
	}

	// A tool only one side has: the other side is null.
	if _, d := do(t, srv, "GET", "/t/tok-rot/prompts/1/tools/"+wid, ""); d["right"].(map[string]any)["tool"] != nil || d["left"].(map[string]any)["tool"] == nil {
		t.Errorf("removed tool = %v", d)
	}
	if _, d := do(t, srv, "GET", "/t/tok-rot/prompts/1/tools/"+nid, ""); d["left"].(map[string]any)["tool"] != nil || d["right"].(map[string]any)["tool"] == nil {
		t.Errorf("new tool = %v", d)
	}

	// Between two versions, the older one on the left, whichever is asked.
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Version zwei"}`)
	for _, path := range []string{"/t/tok-rot/prompts/2/tools/" + qid + "?against=1", "/t/tok-rot/prompts/1/tools/" + qid + "?against=2"} {
		code, d := do(t, srv, "GET", path, "")
		if code != http.StatusOK || d["left"].(map[string]any)["version"] != 1.0 || d["right"].(map[string]any)["version"] != 2.0 || len(d["changes"].([]any)) != 1 {
			t.Errorf("%s: %d %v", path, code, d)
		}
	}
	if _, d := do(t, srv, "GET", "/t/tok-rot/prompts/2/tools", ""); len(d["tools"].([]any)) != 0 {
		t.Errorf("version 2 against now = %v", d)
	}

	// Unknown version or tool: 404.
	for _, path := range []string{"/t/tok-rot/prompts/9/tools", "/t/tok-rot/prompts/1/tools?against=9", "/t/tok-rot/prompts/x/tools",
		"/t/tok-rot/prompts/1/tools/nope", "/t/tok-rot/prompts/1/tools?against=-1"} {
		if code, _ := do(t, srv, "GET", path, ""); code != http.StatusNotFound {
			t.Errorf("%s: %d", path, code)
		}
	}

	// Reads work in a closed task; another team has none of it.
	c := adminClient(t, srv)
	adminCall(t, c, srv, "POST", "/admin/api/tasks/"+testTask+"/close", "")
	if code, _ := do(t, srv, "GET", "/t/tok-rot/prompts/1/tools/"+qid+"?against=2", ""); code != http.StatusOK {
		t.Errorf("closed task: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/t/tok-blau/prompts/1/tools/"+qid, ""); code != http.StatusNotFound {
		t.Errorf("another team: %d", code)
	}
}

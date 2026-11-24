package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDeployVersionsWithTeamTools(t *testing.T) {
	srv := newTestServer(t)
	events := sse(t, srv, "tok-rot", "")
	_, quadrat := do(t, srv, "POST", "/t/tok-rot/programs", saveBody("quadrat", "Fährt ein Quadrat.", exampleBlockly(t, "quadrat")))
	do(t, srv, "POST", "/t/tok-rot/programs", saveBody("ohne_text", "", exampleBlockly(t, "quadrat")))

	code, dep := do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Nutze `+"`quadrat`"+` und `+"`zickzack`"+`."}`)
	if code != http.StatusCreated {
		t.Fatalf("deploy: %d %v", code, dep)
	}
	tools := dep["tools"].([]any)
	if len(tools) != 2 || !strings.Contains(dep["warning"].(string), "`zickzack`") || strings.Contains(dep["warning"].(string), "quadrat") {
		t.Errorf("deploy answer = %v", dep)
	}
	waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "programs ") })

	// The list says what the agent has.
	resp, _ := http.Get(srv.URL + "/t/tok-rot/programs")
	var list []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	state := map[string]string{}
	for _, p := range list {
		who := "agent"
		if p["agent"] != true {
			who, _ = p["why"].(string)
		}
		state[p["name"].(string)] = p["deploy"].(string) + "/" + who
	}
	if state["quadrat"] != "deployed/agent" || state["ohne_text"] != "deployed/ohne Beschreibung" {
		t.Errorf("states = %v", state)
	}

	// Change the tool and the prompt, deploy again: the history names the change.
	id := quadrat["id"].(string)
	do(t, srv, "PUT", "/t/tok-rot/programs/"+id, saveBody("quadrat", "Jetzt anders.", exampleBlockly(t, "quadrat")))
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Version zwei"}`)
	resp, _ = http.Get(srv.URL + "/t/tok-rot/prompts")
	var versions []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&versions)
	resp.Body.Close()
	if len(versions) != 2 {
		t.Fatalf("versions = %v", versions)
	}
	changes, _ := versions[0]["tools"].([]any)
	if len(changes) != 1 || changes[0].(map[string]any)["change"] != "geändert" {
		t.Errorf("version 2 tool changes = %v", versions[0]["tools"])
	}

	// Restore version 1: prompt text for the page, tools in the working state, SSE programs.
	code, p := do(t, srv, "POST", "/t/tok-rot/prompts/1/restore", "")
	if code != http.StatusOK || !strings.Contains(p["text"].(string), "zickzack") {
		t.Fatalf("restore: %d %v", code, p)
	}
	waitEvent(t, events, func(e string) bool { return strings.HasPrefix(e, "programs ") })
	if _, got := do(t, srv, "GET", "/t/tok-rot/programs/"+id, ""); got["description"] != "Fährt ein Quadrat." {
		t.Errorf("after restore: %v", got)
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/prompts/9/restore", ""); code != http.StatusNotFound {
		t.Errorf("unknown version: %d", code)
	}
}

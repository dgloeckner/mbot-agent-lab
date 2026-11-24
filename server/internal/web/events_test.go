package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// kinds returns all events in the log, oldest first.
func kinds(t *testing.T, st *store.Store) []store.Event {
	t.Helper()
	evs, err := st.Events(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

// findEvent returns the newest event of kind for team.
func findEvent(evs []store.Event, kind, team string) (store.Event, bool) {
	for i := len(evs) - 1; i >= 0; i-- {
		if e := evs[i]; e.Kind == kind && e.Team == team {
			return e, true
		}
	}
	return store.Event{}, false
}

func TestHTTPRoutesLeaveEvents(t *testing.T) {
	srv := newTestServer(t)
	code, body := do(t, srv, "POST", "/api/join", `{"code":"TEST","name":"Kurvenkratzer"}`)
	if code != http.StatusCreated {
		t.Fatalf("join: %d %v", code, body)
	}
	team := body["id"].(string)
	evs := kinds(t, srv.st)
	if e, ok := findEvent(evs, store.EventTeamJoined, team); !ok || e.Task != "" || e.Actor != store.ActorTeam {
		t.Errorf("team.joined = %+v, %v", e, ok)
	}
	// Assigning in the control room and accepting on the team page.
	c := adminClient(t, srv)
	code, task := adminCall(t, c, srv, "POST", "/admin/api/tasks", `{"title":"Labyrinth","body":"Findet den Ausgang."}`)
	if code != http.StatusCreated {
		t.Fatalf("create task: %d", code)
	}
	id := field(task, "id").(string)
	adminCall(t, c, srv, "POST", "/admin/api/tasks/"+id+"/publish", "")
	if code, _ := adminCall(t, c, srv, "POST", "/admin/api/tasks/"+id+"/assign", `{"teams":["`+team+`"]}`); code != http.StatusOK {
		t.Fatalf("assign: %d", code)
	}
	if code, body := do(t, srv, "POST", "/t/"+body["token"].(string)+"/tasks/"+id+"/accept", `{"prompt":"empty"}`); code != http.StatusOK {
		t.Fatalf("accept: %d %v", code, body)
	}
	evs = kinds(t, srv.st)
	for _, c := range []struct{ kind, actor string }{
		{store.EventTaskAssigned, store.ActorAdmin}, {store.EventTaskAccepted, store.ActorTeam}, {store.EventTaskActivated, store.ActorTeam},
	} {
		if e, ok := findEvent(evs, c.kind, team); !ok || e.Task != id || e.Actor != c.actor {
			t.Errorf("%s = %+v, %v", c.kind, e, ok)
		}
	}

	if code, _ := do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Fahr los"}`); code != http.StatusCreated {
		t.Fatalf("deploy: %d", code)
	}
	code, note := do(t, srv, "POST", "/t/tok-rot/notes", `{"text":"Wand links"}`)
	if code != http.StatusCreated {
		t.Fatalf("note: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/t/tok-rot/notes/"+strconv.Itoa(int(note["id"].(float64))), ""); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("delete note: %d", code)
	}
	if code := adminDo(t, c, srv, "POST", "/admin/api/assign", `{"robot":"mbot-3","team":"rot"}`); code != http.StatusNoContent {
		t.Fatalf("assign: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-rot/robots/mbot-3/alias", `{"alias":"Scout"}`); code >= 300 {
		t.Fatalf("rename: %d", code)
	}
	if code := adminDo(t, c, srv, "POST", "/admin/api/assign", `{"robot":"mbot-3","team":""}`); code != http.StatusNoContent {
		t.Fatalf("free: %d", code)
	}

	evs = kinds(t, srv.st)
	deployed, ok := findEvent(evs, store.EventPromptDeployed, "rot")
	if !ok || deployed.Task != testTask || deployed.Prompt == 0 || deployed.Actor != store.ActorTeam {
		t.Errorf("prompt.deployed = %+v", deployed)
	}
	for _, kind := range []string{store.EventNoteAdded, store.EventNoteDeleted} {
		if e, ok := findEvent(evs, kind, "rot"); !ok || e.Task != testTask {
			t.Errorf("%s = %+v, %v", kind, e, ok)
		}
	}
	for _, c := range []struct{ kind, actor string }{
		{store.EventRobotAssigned, store.ActorAdmin}, {store.EventRobotRenamed, store.ActorTeam}, {store.EventRobotReleased, store.ActorAdmin},
	} {
		e, ok := findEvent(evs, c.kind, "rot")
		if !ok || e.Robot != "mbot-3" || e.Task != testTask || e.Actor != c.actor {
			t.Errorf("%s = %+v, %v", c.kind, e, ok)
		}
	}
}

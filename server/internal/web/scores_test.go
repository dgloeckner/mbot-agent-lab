package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func TestRunsView(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	tt := store.TeamTask{Team: "rot", Task: testTask}
	p, _ := srv.st.SavePrompt(ctx, tt, "Los.")
	id, _ := srv.st.CreateRun(ctx, tt, p.ID, "fake")
	_ = srv.st.EndRun(ctx, id, "finish", "Am Ziel.")
	run := "/admin/api/runs/" + itoa(id)

	// A team cannot confirm its own run: the routes need the control room.
	if code, _ := do(t, srv, "POST", run+"/confirm", `{"success":true}`); code != http.StatusUnauthorized {
		t.Errorf("confirm without the control room: %d", code)
	}
	c := adminClient(t, srv)
	_, st0 := adminCall(t, c, srv, "GET", "/admin/api/state", "")
	if code, _ := adminCall(t, c, srv, "POST", run+"/confirm", `{"success":true}`); code != http.StatusNoContent {
		t.Fatalf("confirm: %d", code)
	}
	if code, _ := adminCall(t, c, srv, "POST", run+"/intervention", `{"kind":"moved"}`); code != http.StatusNoContent {
		t.Fatalf("intervention: %d", code)
	}
	_, st1 := adminCall(t, c, srv, "GET", "/admin/api/state", "")
	if st0.(map[string]any)["scores"] == st1.(map[string]any)["scores"] {
		t.Error("the state does not report the change")
	}
	if code, _ := adminCall(t, c, srv, "POST", "/admin/api/runs/999/confirm", `{"success":true}`); code != http.StatusNotFound {
		t.Errorf("unknown run: %d", code)
	}
	code, view := adminCall(t, c, srv, "GET", "/admin/api/tasks/"+testTask+"/runs", "")
	teams := view.(map[string]any)["teams"].([]any)
	if code != http.StatusOK || len(teams) != 1 {
		t.Fatalf("runs view: %d %v", code, view)
	}
	team := teams[0].(map[string]any)
	r := team["runs"].([]any)[0].(map[string]any)
	if team["rank"] != 1.0 || r["success"] != true || r["interventions"].(map[string]any)["moved"] != 1.0 || r["to_goal_s"] == nil {
		t.Errorf("team = %v", team)
	}
	if tools := r["tools"].([]any); len(tools) == 0 || !strings.Contains(strings.Join(anyStrings(tools), ","), "drive") {
		t.Errorf("tools = %v", r["tools"])
	}
	// The control room starts a team's run.
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Los."}`)
	if code, _ := adminCall(t, c, srv, "POST", "/admin/api/teams/rot/start", ""); code != http.StatusAccepted {
		t.Errorf("start: %d", code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func anyStrings(xs []any) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i], _ = x.(string)
	}
	return out
}

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestJoin(t *testing.T) {
	srv := newTestServer(t)
	if code, body := do(t, srv, "POST", "/api/join", `{"code":"XXXX","name":"Neu"}`); code != http.StatusBadRequest {
		t.Errorf("wrong code: %d %v", code, body)
	}
	if code, _ := do(t, srv, "POST", "/api/join", `{"code":"test","name":"team rot"}`); code != http.StatusConflict {
		t.Errorf("duplicate name (case-insensitive): %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/join", `{"code":"TEST","name":"x"}`); code != http.StatusBadRequest {
		t.Errorf("too short name: %d", code)
	}
	code, body := do(t, srv, "POST", "/api/join", `{"code":" test ","name":"  Die   Kurvenkratzer "}`)
	if code != http.StatusCreated || body["name"] != "Die Kurvenkratzer" || body["token"] == "" {
		t.Fatalf("join: %d %v", code, body)
	}
	code, sess := do(t, srv, "GET", "/t/"+body["token"].(string)+"/api/session", "")
	if code != http.StatusOK || len(sess["robots"].([]any)) != 0 {
		t.Errorf("new team session: %d %v", code, sess)
	}
}

func TestJoinIsRateLimited(t *testing.T) {
	srv := newTestServer(t)
	var last int
	for i := 0; i < 12; i++ {
		last, _ = do(t, srv, "POST", "/api/join", `{"code":"NOPE","name":"Rater"}`)
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("12th wrong guess: %d", last)
	}
}

func TestCrossSitePostIsRejected(t *testing.T) {
	srv := newTestServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/t/tok-rot/stop", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site POST: %d", resp.StatusCode)
	}
}

// adminClient logs in and returns a client that keeps the session cookie.
func adminClient(t *testing.T, srv *testLab) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Post(srv.URL+"/admin/api/login", "application/json", strings.NewReader(`{"password":"pw"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	return c
}

func adminDo(t *testing.T, c *http.Client, srv *testLab, method, path, body string) int {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestAdminNeedsLogin(t *testing.T) {
	srv := newTestServer(t)
	if code, _ := do(t, srv, "GET", "/admin/api/state", ""); code != http.StatusUnauthorized {
		t.Errorf("state without login: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/admin/api/login", `{"password":"falsch"}`); code != http.StatusUnauthorized {
		t.Errorf("wrong password: %d", code)
	}
	c := adminClient(t, srv)
	if code := adminDo(t, c, srv, "GET", "/admin/api/state", ""); code != http.StatusOK {
		t.Errorf("state after login: %d", code)
	}
	adminDo(t, c, srv, "POST", "/admin/api/logout", "")
	if code := adminDo(t, c, srv, "GET", "/admin/api/state", ""); code != http.StatusUnauthorized {
		t.Errorf("state after logout: %d", code)
	}
}

func TestAdminAssignsSeveralRobotsToOneTeam(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	// mbot-2 moves from blau to rot, mbot-3 joins rot as well: rot now steers three robots.
	for _, robot := range []string{"mbot-2", "mbot-3"} {
		if code := adminDo(t, c, srv, "POST", "/admin/api/assign", `{"robot":"`+robot+`","team":"rot"}`); code != http.StatusNoContent {
			t.Fatalf("assign %s: %d", robot, code)
		}
	}
	_, rot := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	aliases := map[string]string{}
	for _, r := range rot["robots"].([]any) {
		m := r.(map[string]any)
		aliases[m["id"].(string)] = m["alias"].(string)
	}
	want := map[string]string{"mbot-1": "rot", "mbot-2": "blau", "mbot-3": "grün"}
	for id, a := range want {
		if aliases[id] != a {
			t.Errorf("%s alias = %q, want %q (all: %v)", id, aliases[id], a, aliases)
		}
	}
	_, blau := do(t, srv, "GET", "/t/tok-blau/api/session", "")
	if n := len(blau["robots"].([]any)); n != 0 {
		t.Errorf("blau still has %d robots", n)
	}
	if code := adminDo(t, c, srv, "POST", "/admin/api/assign", `{"robot":"mbot-9","team":"rot"}`); code != http.StatusNotFound {
		t.Errorf("unknown robot: %d", code)
	}
	if code := adminDo(t, c, srv, "POST", "/admin/api/assign", `{"robot":"mbot-3","team":""}`); code != http.StatusNoContent {
		t.Errorf("free robot: %d", code)
	}
}

func TestTeamRenamesRobotsAndItself(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	adminDo(t, c, srv, "POST", "/admin/api/assign", `{"robot":"mbot-3","team":"rot"}`)

	if code, _ := do(t, srv, "PUT", "/t/tok-rot/robots/mbot-1/alias", `{"alias":"Scout"}`); code != http.StatusNoContent {
		t.Fatalf("rename: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-rot/robots/mbot-3/alias", `{"alias":"scout"}`); code != http.StatusConflict {
		t.Errorf("duplicate alias in team: %d", code)
	}
	for _, bad := range []string{`""`, `"zwei Worte"`, `"alle"`, `"1abc"`} {
		if code, _ := do(t, srv, "PUT", "/t/tok-rot/robots/mbot-3/alias", `{"alias":`+bad+`}`); code != http.StatusBadRequest {
			t.Errorf("alias %s: %d", bad, code)
		}
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-blau/robots/mbot-1/alias", `{"alias":"Dieb"}`); code != http.StatusNotFound {
		t.Errorf("renaming another team's robot: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-rot/name", `{"name":"Team Blau"}`); code != http.StatusConflict {
		t.Errorf("taking another team's name: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-rot/name", `{"name":"Rote Raketen"}`); code != http.StatusNoContent {
		t.Errorf("rename team: %d", code)
	}
	for _, rb := range srv.reg.Snapshot() {
		if rb.ID == "mbot-1" && (rb.Alias != "Scout" || rb.OwnerName != "Rote Raketen") {
			t.Errorf("registry not updated: %+v", rb)
		}
	}
}

func TestAdminChangesTeamColor(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	if code := adminDo(t, c, srv, "PUT", "/admin/api/teams/rot/color", `{"color":"purple"}`); code != http.StatusNoContent {
		t.Fatalf("set color: %d", code)
	}
	_, rot := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	if got := rot["team"].(map[string]any)["color"]; got != "purple" {
		t.Errorf("team color = %v, want purple", got)
	}
	for _, rb := range srv.reg.Snapshot() {
		if rb.ID == "mbot-1" && rb.TeamColor != "purple" {
			t.Errorf("robot LEDs not updated: %+v", rb)
		}
	}
	// Two teams may share a color.
	if code := adminDo(t, c, srv, "PUT", "/admin/api/teams/blau/color", `{"color":"purple"}`); code != http.StatusNoContent {
		t.Errorf("shared color: %d", code)
	}
	for _, bad := range []string{`"pink"`, `""`, `"white"`} {
		if code := adminDo(t, c, srv, "PUT", "/admin/api/teams/rot/color", `{"color":`+bad+`}`); code != http.StatusBadRequest {
			t.Errorf("color %s: %d", bad, code)
		}
	}
	if code := adminDo(t, c, srv, "PUT", "/admin/api/teams/gibtsnicht/color", `{"color":"red"}`); code != http.StatusNotFound {
		t.Errorf("unknown team: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/admin/api/teams/rot/color", `{"color":"red"}`); code != http.StatusUnauthorized {
		t.Errorf("without login: %d", code)
	}
}

func TestDeletedTeamLosesLinkAndRobots(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	if code := adminDo(t, c, srv, "DELETE", "/admin/api/teams/blau", ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/t/tok-blau/api/session", ""); code != http.StatusNotFound {
		t.Errorf("old link still works: %d", code)
	}
	for _, rb := range srv.reg.Snapshot() {
		if rb.ID == "mbot-2" && rb.Owner != "" {
			t.Errorf("robot of deleted team still owned: %+v", rb)
		}
	}
}

func TestTeamChoosesReleasedModel(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	models := func(token string) (string, []string) {
		t.Helper()
		_, s := do(t, srv, "GET", "/t/"+token+"/api/session", "")
		m := s["models"].(map[string]any)
		var opts []string
		for _, o := range m["options"].([]any) {
			opts = append(opts, o.(map[string]any)["provider"].(string))
		}
		return m["selected"].(string), opts
	}

	// Only the default is released at first.
	if sel, opts := models("tok-rot"); sel != "fake" || len(opts) != 1 {
		t.Fatalf("initial: %q %v", sel, opts)
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-rot/model", `{"provider":"teuer"}`); code != http.StatusBadRequest {
		t.Errorf("unreleased model: %d", code)
	}
	if code := adminDo(t, c, srv, "PUT", "/admin/api/models", `{"allowed":["teuer","gibtsnicht"]}`); code != http.StatusBadRequest {
		t.Errorf("unknown provider: %d", code)
	}
	if code := adminDo(t, c, srv, "PUT", "/admin/api/models", `{"allowed":["teuer"]}`); code != http.StatusNoContent {
		t.Fatalf("release: %d", code)
	}
	if code, _ := do(t, srv, "PUT", "/t/tok-rot/model", `{"provider":"teuer"}`); code != http.StatusNoContent {
		t.Fatalf("choose: %d", code)
	}
	if sel, opts := models("tok-rot"); sel != "teuer" || len(opts) != 2 || opts[0] != "fake" {
		t.Errorf("after choosing: %q %v", sel, opts)
	}
	if sel, _ := models("tok-blau"); sel != "fake" {
		t.Errorf("other team: %q", sel)
	}

	// The next run uses the chosen model.
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Fahr los."}`)
	code, st := do(t, srv, "POST", "/t/tok-rot/start", "")
	if code != http.StatusAccepted || st["model"] != "teuer-modell" || st["provider"] != "teuer" {
		t.Errorf("start: %d %v", code, st)
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")

	// Session and control room count the run in the team's totals.
	_, sess := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	if total := sess["total"].(map[string]any); total["runs"] != 1.0 {
		t.Errorf("session total = %v", total)
	}
	resp, err := c.Get(srv.URL + "/admin/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Teams []struct {
			ID    string `json:"id"`
			Total struct {
				Runs int `json:"runs"`
			} `json:"total"`
		} `json:"teams"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&state)
	resp.Body.Close()
	for _, tm := range state.Teams {
		if want := map[string]int{"rot": 1, "blau": 0}[tm.ID]; tm.Total.Runs != want {
			t.Errorf("admin total of %s = %d runs, want %d", tm.ID, tm.Total.Runs, want)
		}
	}

	// Revoking the release sends the team back to the default.
	if code := adminDo(t, c, srv, "PUT", "/admin/api/models", `{"allowed":[]}`); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if sel, opts := models("tok-rot"); sel != "fake" || len(opts) != 1 {
		t.Errorf("after revoke: %q %v", sel, opts)
	}
	team, _ := srv.lab.Store.Team(context.Background(), "rot")
	if team.Provider != "" {
		t.Errorf("stored choice not cleared: %q", team.Provider)
	}
}

func TestAdminRobotMaintenance(t *testing.T) {
	srv := newTestServer(t)
	c := adminClient(t, srv)
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/admin/api/robots/mbot-9/restart", "", http.StatusNotFound},
		// The test robots never connect: the command cannot reach them.
		{"POST", "/admin/api/robots/mbot-1/restart", "", http.StatusServiceUnavailable},
		{"PUT", "/admin/api/robots/mbot-1/device-name", `{"name":"mbot rot"}`, http.StatusServiceUnavailable},
		{"PUT", "/admin/api/robots/mbot-1/device-name", `{"name":""}`, http.StatusBadRequest},
		{"PUT", "/admin/api/robots/mbot-1/device-name", `{"name":"Grün"}`, http.StatusBadRequest},
		{"PUT", "/admin/api/robots/mbot-1/device-name", `{"name":"einsehrsehrlangername1"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if code := adminDo(t, c, srv, tc.method, tc.path, tc.body); code != tc.want {
			t.Errorf("%s %s %s: %d, want %d", tc.method, tc.path, tc.body, code, tc.want)
		}
	}
}

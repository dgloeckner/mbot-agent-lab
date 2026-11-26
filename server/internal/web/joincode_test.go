package web

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Another PC of the team: a PC with the link shows a short code, /join takes it.
func TestJoinCode(t *testing.T) {
	srv := newTestServer(t)
	code, got := do(t, srv, "POST", "/t/tok-rot/join-code", "")
	c, _ := got["code"].(string)
	if code != http.StatusOK || len(c) != 6 || got["expires_at"] == nil {
		t.Fatalf("join code: %d %v", code, got)
	}
	// A second PC of the team shows the same code.
	if _, again := do(t, srv, "POST", "/t/tok-rot/join-code", ""); again["code"] != c {
		t.Errorf("second code = %v, want %s", again["code"], c)
	}
	code, team := do(t, srv, "POST", "/api/join-team", `{"code":"`+strings.ToLower(c[:3])+"-"+c[3:]+`"}`)
	if code != http.StatusOK || team["token"] != "tok-rot" {
		t.Fatalf("join with code: %d %v", code, team)
	}
	if code, body := do(t, srv, "POST", "/api/join-team", `{"code":"ZZZZZZ"}`); code != http.StatusBadRequest || !strings.Contains(body["error"].(string), "abgelaufen") {
		t.Errorf("wrong code: %d %v", code, body)
	}
}

// A wrong team link: browsers get a page with status 404, API calls JSON.
func TestUnknownTeamLinkPage(t *testing.T) {
	srv := newTestServer(t)
	get := func(accept string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", srv.URL+"/t/falsch", nil)
		req.Header.Set("Accept", accept)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	resp, body := get("text/html,application/xhtml+xml")
	if resp.StatusCode != http.StatusNotFound || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || !strings.Contains(strings.ToLower(body), "<html") {
		t.Errorf("browser: %d %s %.80s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	resp, body = get("application/json")
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, `"error"`) {
		t.Errorf("api: %d %s", resp.StatusCode, body)
	}
}

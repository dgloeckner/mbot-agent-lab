package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

type nopBus struct{}

func (nopBus) Publish(string, []byte, bool, byte) error       { return nil }
func (nopBus) Handle(string, int, func(string, []byte)) error { return nil }

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		Robots:   []config.Robot{{ID: "mbot-1", Color: "red"}, {ID: "mbot-2", Color: "blue"}},
		Teams:    []config.Team{{ID: "rot", Name: "Team Rot", Token: "tok-rot"}, {ID: "blau", Name: "Team Blau", Token: "tok-blau"}},
		Profile:  "einzel",
		Profiles: map[string]config.Profile{"einzel": {"rot": {"ich": "mbot-1"}, "blau": {"ich": "mbot-2"}}},
	}
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := robots.New(nopBus{}, []robots.Spec{{ID: "mbot-1", Color: "red"}, {ID: "mbot-2", Color: "blue"}}, nil)
	_ = reg.Assign("mbot-1", "rot", "Team Rot", "ich")
	_ = reg.Assign("mbot-2", "blau", "Team Blau", "ich")
	s := &Server{Config: cfg, Robots: reg, Store: st, Hub: NewHub(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestUnknownTokenIsRejected(t *testing.T) {
	srv := newTestServer(t)
	code, body := do(t, srv, "GET", "/t/nope/api/session", "")
	if code != http.StatusNotFound || body["error"] == nil {
		t.Fatalf("code %d body %v", code, body)
	}
	resp, _ := http.Get(srv.URL + "/t/tok-rot/api/session")
	if got := resp.Header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q", got)
	}
}

func TestDeployAndSession(t *testing.T) {
	srv := newTestServer(t)
	if code, _ := do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"  "}`); code != http.StatusBadRequest {
		t.Errorf("empty prompt: %d", code)
	}
	for i, text := range []string{"Fahr los", "Fahr vorsichtig los"} {
		code, body := do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"`+text+`"}`)
		if code != http.StatusCreated || body["version"].(float64) != float64(i+1) {
			t.Fatalf("deploy %d: %d %v", i, code, body)
		}
	}
	_, sess := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	prompt := sess["prompt"].(map[string]any)
	if prompt["version"].(float64) != 2 || prompt["text"] != "Fahr vorsichtig los" {
		t.Errorf("prompt = %v", prompt)
	}
	if team := sess["team"].(map[string]any); team["color"] != "red" {
		t.Errorf("team = %v", team)
	}
	if n := len(sess["versions"].([]any)); n != 2 {
		t.Errorf("versions = %d", n)
	}
	if robots := sess["robots"].([]any); len(robots) != 1 || robots[0].(map[string]any)["alias"] != "ich" {
		t.Errorf("robots = %v", robots)
	}
	_, other := do(t, srv, "GET", "/t/tok-blau/api/session", "")
	if other["prompt"] != nil {
		t.Errorf("team blau sees a prompt: %v", other["prompt"])
	}
	code, v1 := do(t, srv, "GET", "/t/tok-rot/prompts/1", "")
	if code != http.StatusOK || v1["text"] != "Fahr los" {
		t.Errorf("v1: %d %v", code, v1)
	}
}

func TestNotes(t *testing.T) {
	srv := newTestServer(t)
	code, n := do(t, srv, "POST", "/t/tok-rot/notes", `{"text":"Wand links"}`)
	if code != http.StatusCreated {
		t.Fatalf("add: %d", code)
	}
	id := strconv.Itoa(int(n["id"].(float64)))
	if code, _ := do(t, srv, "PUT", "/t/tok-blau/notes/"+id, `{"text":"x"}`); code != http.StatusNotFound {
		t.Errorf("foreign update: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/t/tok-rot/notes/"+id, ""); code != http.StatusNoContent {
		t.Errorf("delete: %d", code)
	}
}

func TestEventsStream(t *testing.T) {
	srv := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/t/tok-rot/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	next := func(prefix string) string {
		for lines.Scan() {
			if strings.HasPrefix(lines.Text(), prefix) {
				return lines.Text()
			}
		}
		t.Fatalf("stream ended before %q", prefix)
		return ""
	}
	next("event: robots")
	do(t, srv, "POST", "/t/tok-rot/message", `{"text":"Hallo"}`)
	next("event: log")
	if data := next("data: "); !strings.Contains(data, `"text":"Hallo"`) {
		t.Errorf("log data = %s", data)
	}
}

package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/lab"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

type nopBus struct{}

func (nopBus) Publish(string, []byte, bool, byte) error       { return nil }
func (nopBus) Handle(string, int, func(string, []byte)) error { return nil }

// testLab is a server with two teams ("tok-rot" owns mbot-1, "tok-blau" owns
// mbot-2) and a free robot mbot-3. The lab code is TEST and the admin password pw.
type testLab struct {
	*httptest.Server
	lab *lab.Service
	reg *robots.Registry
	st  *store.Store
}

func newTestServer(t *testing.T) *testLab {
	t.Helper()
	ctx := context.Background()
	cfg := &config.Config{
		Server: config.Server{AdminPassword: "pw", LabCode: "TEST"},
		Robots: []config.Robot{{ID: "mbot-1", Color: "red"}, {ID: "mbot-2", Color: "blue"}, {ID: "mbot-3", Color: "green"}},
		LLM: config.LLM{Default: "fake", MaxParallelCalls: 2, Providers: map[string]config.Provider{
			"fake":  {Kind: config.ProviderFake, Model: config.FakeModel},
			"teuer": {Kind: config.ProviderFake, Model: "teuer-modell", PriceIn: 4, PriceOut: 20},
		}},
	}
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "lab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := robots.New(nopBus{}, []robots.Spec{{ID: "mbot-1", Color: "red"}, {ID: "mbot-2", Color: "blue"}, {ID: "mbot-3", Color: "green"}}, log)
	hub := live.NewHub()
	svc := &lab.Service{Store: st, Robots: reg, Hub: hub, Config: cfg, Log: log}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tm := range []store.Team{{ID: "rot", Name: "Team Rot", Token: "tok-rot", Color: "red"}, {ID: "blau", Name: "Team Blau", Token: "tok-blau", Color: "blue"}} {
		if _, err := st.CreateTeam(ctx, tm); err != nil {
			t.Fatal(err)
		}
		activateTestTask(t, st, tm.ID)
	}
	if err := svc.AssignRobot(ctx, "mbot-1", "rot"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignRobot(ctx, "mbot-2", "blau"); err != nil {
		t.Fatal(err)
	}
	models := llm.New(cfg.LLM, log)
	budgets := config.Budgets{MaxStepsPerWakeup: 3, MaxRunMinutes: 5, MaxHistoryMessages: 40, MaxAgentMessagesPerMinute: 5, MaxConversationMessagesPerMinute: 3}
	agents := agent.NewManager(agent.Deps{Store: st, Robots: reg, Hub: hub, LLM: models, Tools: tools.NewRegistry(reg, st, hub, budgets), Budgets: budgets, Log: log,
		TeamProvider: svc.TeamProvider, Conversations: svc.ConversationEnabled})
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go agents.Run(runCtx)
	s := &Server{Config: cfg, Robots: reg, Store: st, Hub: hub, Lab: svc, Agents: agents, Log: log}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &testLab{Server: srv, lab: svc, reg: reg, st: st}
}

func do(t *testing.T, srv *testLab, method, path, body string) (int, map[string]any) {
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
	if robots := sess["robots"].([]any); len(robots) != 1 || robots[0].(map[string]any)["alias"] != "rot" {
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
	next("event: totals")
	if data := next("event: models"); data == "" {
		t.Fatal("no models event")
	}
	do(t, srv, "POST", "/t/tok-rot/message", `{"text":"Hallo"}`)
	for {
		if data := next("data: "); strings.Contains(data, `"text":"Hallo"`) {
			break
		}
	}
}

func TestRunStartMessageStop(t *testing.T) {
	srv := newTestServer(t)
	code, body := do(t, srv, "POST", "/t/tok-rot/start", "")
	if code != http.StatusConflict || !strings.Contains(body["error"].(string), "Prompt") {
		t.Fatalf("start without prompt: %d %v", code, body)
	}
	do(t, srv, "POST", "/t/tok-rot/prompt", `{"text":"Erkunde die Arena."}`)
	if code, body = do(t, srv, "POST", "/t/tok-rot/start", ""); code != http.StatusAccepted || body["running"] != true {
		t.Fatalf("start: %d %v", code, body)
	}
	if code, _ = do(t, srv, "POST", "/t/tok-rot/start", ""); code != http.StatusConflict {
		t.Errorf("second start: %d", code)
	}
	_, sess := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	if sess["agent_available"] != true || sess["agent"].(map[string]any)["running"] != true {
		t.Errorf("session agent = %v", sess["agent"])
	}
	do(t, srv, "POST", "/t/tok-rot/stop", "")
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, sess = do(t, srv, "GET", "/t/tok-rot/api/session", "")
		if sess["agent"].(map[string]any)["running"] == false {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// conversationLog returns the team's conversation entries.
func conversationLog(srv *testLab, team string) []live.LogEntry {
	backlog, _, cancel := srv.lab.Hub.Subscribe(team, 0)
	cancel()
	var out []live.LogEntry
	for _, e := range backlog {
		if e.Channel == live.ChannelConversation {
			out = append(out, e)
		}
	}
	return out
}

// Without a run, a team message goes to the conversation and gets an answer
// from the test model; it is rate limited, and the control room can switch
// conversations off.
func TestMessageWithoutRunStartsConversation(t *testing.T) {
	srv := newTestServer(t)
	code, e := do(t, srv, "POST", "/t/tok-rot/message", `{"text":"Was kannst du?"}`)
	if code != http.StatusAccepted || e["channel"] != live.ChannelConversation {
		t.Fatalf("message: %d %v", code, e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var answer string
		for _, e := range conversationLog(srv, "rot") {
			if e.Source == live.SourceAgent && e.Kind == "text" {
				answer = e.Text
			}
		}
		if answer != "" {
			if !strings.Contains(answer, "kein Lauf") {
				t.Errorf("answer = %q", answer)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no answer in the conversation")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, e := range conversationLog(srv, "rot") {
		if e.Kind == "info" && strings.Contains(e.Text, "läuft gerade nicht") {
			t.Error("message was dropped")
		}
	}
	_, sess := do(t, srv, "GET", "/t/tok-rot/api/session", "")
	ag := sess["agent"].(map[string]any)
	if ag["conversation"] != true || ag["running"] != false || ag["conversation_id"] == nil {
		t.Errorf("session agent = %v", ag)
	}
	if total := sess["total"].(map[string]any); total["runs"] != 0.0 || total["conversation"].(map[string]any)["count"] != 1.0 {
		t.Errorf("session total = %v", total)
	}

	// Three messages per minute; the first one counts.
	for i := 0; i < 2; i++ {
		if code, _ := do(t, srv, "POST", "/t/tok-rot/message", `{"text":"Und?"}`); code != http.StatusAccepted {
			t.Fatalf("message %d: %d", i+2, code)
		}
	}
	if code, body := do(t, srv, "POST", "/t/tok-rot/message", `{"text":"Und?"}`); code != http.StatusTooManyRequests || !strings.Contains(body["error"].(string), "pro Minute") {
		t.Errorf("fourth message: %d %v", code, body)
	}
	if code, _ := do(t, srv, "POST", "/t/tok-rot/conversation/new", ""); code != http.StatusNoContent {
		t.Errorf("new conversation: %d", code)
	}

	c := adminClient(t, srv)
	if code := adminDo(t, c, srv, "PUT", "/admin/api/conversation", `{"on":false}`); code != http.StatusNoContent {
		t.Fatalf("switch off: %d", code)
	}
	if code, body := do(t, srv, "POST", "/t/tok-blau/message", `{"text":"Hallo?"}`); code != http.StatusConflict || !strings.Contains(body["error"].(string), "Leitstand") {
		t.Errorf("switched off: %d %v", code, body)
	}
	if _, sess := do(t, srv, "GET", "/t/tok-blau/api/session", ""); sess["agent"].(map[string]any)["conversation"] != false {
		t.Error("session still offers the conversation")
	}
	resp, err := c.Get(srv.URL + "/admin/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Conversation bool `json:"conversation"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&state)
	resp.Body.Close()
	if state.Conversation {
		t.Error("admin state still shows conversations on")
	}
}

// testTask is the task every test team works in.
const testTask = "a1"

// activateTestTask assigns, accepts and activates testTask for team.
func activateTestTask(t *testing.T, st *store.Store, team string) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.Task(ctx, testTask); errors.Is(err, store.ErrNotFound) {
		if _, err := st.CreateTask(ctx, store.Task{ID: testTask, Title: "Test", Body: "Aufgabe", State: store.TaskOpen}); err != nil {
			t.Fatal(err)
		}
	}
	tt := store.TeamTask{Team: team, Task: testTask}
	for _, step := range []func(context.Context, store.TeamTask) error{st.AssignTask, st.AcceptTask, st.ActivateTask} {
		if err := step(ctx, tt); err != nil {
			t.Fatal(err)
		}
	}
}

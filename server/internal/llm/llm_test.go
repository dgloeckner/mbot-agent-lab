package llm

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestOpenAIWireFormat(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("path %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"abc123def","function":{"name":"drive","arguments":"{\"cm\":20}"}}]}}],
			"usage":{"prompt_tokens":120,"completion_tokens":15}}`)
	}))
	defer srv.Close()

	c := New(config.LLM{Default: "m", Providers: map[string]config.Provider{
		"m": {Kind: config.ProviderOpenAI, BaseURL: srv.URL + "/v1/", APIKey: "key", Model: "small", PriceIn: 0.1, PriceOut: 0.3},
	}}, quiet)
	resp, call, err := c.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleSystem, Content: "sys"}, {Role: RoleUser, Content: "los"}},
		Tools:    []Tool{{Name: "drive", Description: "fahren", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["model"] != "small" || got["tool_choice"] != "auto" {
		t.Errorf("request = %v", got)
	}
	tools := got["tools"].([]any)
	if tools[0].(map[string]any)["type"] != "function" {
		t.Errorf("tools = %v", tools)
	}
	tc := resp.Message.ToolCalls
	if len(tc) != 1 || tc[0].Function.Name != "drive" || tc[0].Type != "function" || resp.Message.Content != "" {
		t.Errorf("message = %+v", resp.Message)
	}
	// 120 * 0.1 + 15 * 0.3 = 16.5 micro-EUR
	if call.CostMicroEUR != 17 || call.Usage.PromptTokens != 120 || call.Model != "small" {
		t.Errorf("call = %+v", call)
	}
}

type flaky struct {
	fails  int32
	calls  atomic.Int32
	status int
}

func (f *flaky) Chat(context.Context, Request) (Response, error) {
	if f.calls.Add(1) <= f.fails {
		return Response{}, &StatusError{Status: f.status}
	}
	return Response{Message: Message{Role: RoleAssistant, Content: "ok"}}, nil
}

func newClient(def, fallback string) *Client {
	c := New(config.LLM{Default: def, Fallback: fallback, MaxParallelCalls: 2}, quiet)
	c.Backoff = []time.Duration{time.Millisecond, time.Millisecond}
	return c
}

func TestRetriesTransientErrors(t *testing.T) {
	c := newClient("a", "")
	p := &flaky{fails: 2, status: 503}
	c.Use("a", "m", p)
	if _, _, err := c.Chat(context.Background(), Request{}); err != nil || p.calls.Load() != 3 {
		t.Fatalf("err %v after %d calls", err, p.calls.Load())
	}
}

func TestFallbackAfterPermanentError(t *testing.T) {
	c := newClient("a", "b")
	bad := &flaky{fails: 99, status: 401}
	good := &flaky{}
	c.Use("a", "m", bad)
	c.Use("b", "m2", good)
	_, call, err := c.Chat(context.Background(), Request{})
	if err != nil || call.Provider != "b" || bad.calls.Load() != 1 {
		t.Fatalf("call %+v err %v, bad called %d times", call, err, bad.calls.Load())
	}
}

func TestChatWithPicksProvider(t *testing.T) {
	c := newClient("a", "b")
	a, b, x := &flaky{}, &flaky{}, &flaky{fails: 99, status: 500}
	c.Use("a", "ma", a)
	c.Use("b", "mb", b)
	c.Use("x", "mx", x)

	if _, call, err := c.ChatWith(context.Background(), "b", Request{}); err != nil || call.Provider != "b" || call.Model != "mb" {
		t.Fatalf("chosen provider: call %+v err %v", call, err)
	}
	// An unknown provider means the default one.
	if _, call, _ := c.ChatWith(context.Background(), "gone", Request{}); call.Provider != "a" || c.Model("gone") != "ma" {
		t.Fatalf("unknown provider: call %+v", call)
	}
	// A failing chosen provider still falls back.
	if _, call, err := c.ChatWith(context.Background(), "x", Request{}); err != nil || call.Provider != "b" || x.calls.Load() != 3 {
		t.Fatalf("fallback: call %+v err %v, x called %d times", call, err, x.calls.Load())
	}
	if c.Resolve("x") != "x" || c.Resolve("") != "a" || c.Model("x") != "mx" {
		t.Error("Resolve/Model")
	}
}

func TestFakeExploresAndFinishes(t *testing.T) {
	tools := []Tool{
		{Name: "read_sensors", Parameters: json.RawMessage(`{"type":"object","properties":{"robot":{"type":"string","enum":["Scout","Träger"]}}}`)},
		{Name: "drive_until", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "turn", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "finish", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	f := Fake{Delay: time.Millisecond}
	msgs := []Message{{Role: RoleSystem}, {Role: RoleUser, Content: "Los geht's"}}
	var names []string
	for i := 0; i < 20; i++ {
		resp, err := f.Chat(context.Background(), Request{Messages: msgs, Tools: tools})
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, resp.Message)
		if len(resp.Message.ToolCalls) == 0 {
			break
		}
		tc := resp.Message.ToolCalls[0]
		names = append(names, tc.Function.Name)
		if tc.Function.Name == "read_sensors" && tc.Function.Arguments != `{"robot":"Scout"}` {
			t.Errorf("robot param not filled: %s", tc.Function.Arguments)
		}
		if tc.Function.Name == "finish" {
			break
		}
		msgs = append(msgs, Message{Role: RoleTool, Name: tc.Function.Name, ToolCallID: tc.ID, Content: `{"ok":true,"reason":"wall"}`})
	}
	if names[0] != "read_sensors" || names[len(names)-1] != "finish" {
		t.Errorf("sequence = %v", names)
	}
}

// In a conversation (no finish tool) the fake answers in text, reads the
// sensors or writes a note, and never moves a robot even when told to.
func TestFakeConversationNeverMoves(t *testing.T) {
	tools := []Tool{
		{Name: "read_sensors", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "remember", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	f := Fake{Delay: time.Millisecond}
	ask := func(q string) Message {
		line, _ := json.Marshal(map[string]any{"von": "Team", "vor_s": 0, "text": q})
		resp, err := f.Chat(context.Background(), Request{Messages: []Message{{Role: RoleSystem}, {Role: RoleUser, Content: "Posteingang (älteste zuerst):\n" + string(line)}}, Tools: tools})
		if err != nil {
			t.Fatal(err)
		}
		return resp.Message
	}
	for q, want := range map[string]string{
		"Fahr sofort 20 cm!":               "",
		"Stopp! Halt an!":                  "",
		"Wie weit ist die Wand weg?":       "read_sensors",
		"Merk dir: Kanten sind 20 cm lang": "remember",
	} {
		m := ask(q)
		got := ""
		if len(m.ToolCalls) > 0 {
			got = m.ToolCalls[0].Function.Name
		}
		if got != want {
			t.Errorf("%q: tool %q, want %q", q, got, want)
		}
		if want == "" && !strings.Contains(m.Content, "kein Lauf") {
			t.Errorf("%q: answer %q", q, m.Content)
		}
	}
	resp, _ := f.Chat(context.Background(), Request{Tools: tools, Messages: []Message{
		{Role: RoleSystem}, {Role: RoleUser, Content: "x"}, ask("Abstand?"),
		{Role: RoleTool, Name: "read_sensors", Content: `{"ok":true,"distance_cm":87,"battery":98}`},
	}})
	if !strings.Contains(resp.Message.Content, "87 cm") || len(resp.Message.ToolCalls) > 0 {
		t.Errorf("after read_sensors: %+v", resp.Message)
	}
}

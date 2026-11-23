package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
)

// fakeClaude serves /v1/messages with scripted answers and records requests.
type fakeClaude struct {
	t        *testing.T
	answers  []string
	requests []map[string]any
	headers  []http.Header
	n        atomic.Int32
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The SDK sends the key as a bearer token or as X-Api-Key, depending on its version.
	if r.URL.Path != "/v1/messages" || (r.Header.Get("Authorization") != "Bearer key" && r.Header.Get("X-Api-Key") != "key") {
		f.t.Errorf("path %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.requests = append(f.requests, body)
	f.headers = append(f.headers, r.Header.Clone())
	i := int(f.n.Add(1)) - 1
	answer := f.answers[min(i, len(f.answers)-1)]
	if strings.HasPrefix(answer, "HTTP ") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, answer)
}

const claudeToolAnswer = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
 "content":[{"type":"thinking","thinking":"","signature":"sig-1"},
            {"type":"text","text":"Ich fahre los."},
            {"type":"tool_use","id":"toolu_1","name":"drive","input":{"cm":20}}],
 "stop_reason":"tool_use","stop_details":null,
 "usage":{"input_tokens":100,"output_tokens":40,"cache_read_input_tokens":800,"cache_creation_input_tokens":100}}`

const claudeTextAnswer = `{"id":"msg_2","type":"message","role":"assistant","model":"claude-opus-5-5",
 "content":[{"type":"text","text":"Fertig."}],"stop_reason":"end_turn","stop_details":null,
 "usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`

const claudeRefusal = `{"id":"msg_3","type":"message","role":"assistant","model":"claude-opus-5-5",
 "content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"no"},
 "usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`

func claudeClient(t *testing.T, srv *httptest.Server, effort string) *Client {
	c := New(config.LLM{Default: "claude", Providers: map[string]config.Provider{
		"claude": {Kind: config.ProviderAnthropic, BaseURL: srv.URL, APIKey: "key", Model: "claude-opus-5-5",
			Effort: effort, PriceIn: 4, PriceOut: 20},
	}}, quiet)
	c.Backoff = []time.Duration{time.Millisecond, time.Millisecond}
	return c
}

var driveTool = Tool{Name: "drive", Description: "Fährt geradeaus.", Parameters: json.RawMessage(
	`{"type":"object","properties":{"cm":{"type":"number","minimum":-300}},"required":["cm"],"additionalProperties":false}`)}

func TestClaudeRequestAndResponse(t *testing.T) {
	f := &fakeClaude{t: t, answers: []string{claudeToolAnswer}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := claudeClient(t, srv, "low")

	resp, call, err := c.Chat(context.Background(), Request{
		Messages: []Message{{Role: RoleSystem, Content: "Regeln"}, {Role: RoleUser, Content: "los"}},
		Tools:    []Tool{driveTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := f.requests[0]
	if req["model"] != "claude-opus-5-5" || req["max_tokens"] != float64(anthropicMaxTokens) {
		t.Errorf("request = %v", req)
	}
	if sys := req["system"].([]any)[0].(map[string]any); sys["text"] != "Regeln" {
		t.Errorf("system = %v", req["system"])
	}
	thinking := req["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" || thinking["block_binding"].(map[string]any)["prefix_mismatch_behavior"] != "drop_block" {
		t.Errorf("thinking = %v", thinking)
	}
	if req["output_config"].(map[string]any)["effort"] != "low" || req["fallbacks"] != "default" || req["cache_control"] == nil {
		t.Errorf("request = %v", req)
	}
	betas := f.headers[0].Get("Anthropic-Beta")
	if !strings.Contains(betas, "thinking-binding-controls-2026-08-01") || !strings.Contains(betas, "server-side-fallback-2026-07-01") {
		t.Errorf("betas = %q", betas)
	}
	tool := req["tools"].([]any)[0].(map[string]any)
	schema := tool["input_schema"].(map[string]any)
	if tool["name"] != "drive" || schema["type"] != "object" || schema["additionalProperties"] != false ||
		schema["required"].([]any)[0] != "cm" || schema["properties"].(map[string]any)["cm"] == nil {
		t.Errorf("tool = %v", tool)
	}

	m := resp.Message
	if m.Content != "Ich fahre los." || len(m.ToolCalls) != 1 || m.ToolCalls[0].ID != "toolu_1" ||
		m.ToolCalls[0].Function.Name != "drive" || m.ToolCalls[0].Function.Arguments != `{"cm":20}` {
		t.Errorf("message = %+v", m)
	}
	// 100 plain + 0.1 * 800 read + 1.25 * 100 written = 305 input units at 4, 40 * 20 output.
	if call.Usage.PromptTokens != 1000 || call.Usage.CompletionTokens != 40 || call.CostMicroEUR != 305*4+40*20 {
		t.Errorf("call = %+v", call)
	}
}

func TestClaudeReplaysThinkingAndGroupsToolResults(t *testing.T) {
	f := &fakeClaude{t: t, answers: []string{claudeToolAnswer, claudeTextAnswer}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := claudeClient(t, srv, "")

	history := []Message{{Role: RoleSystem, Content: "Regeln"}, {Role: RoleUser, Content: "los"}}
	resp, _, err := c.Chat(context.Background(), Request{Messages: history, Tools: []Tool{driveTool}})
	if err != nil {
		t.Fatal(err)
	}
	history = append(history, resp.Message,
		Message{Role: RoleTool, ToolCallID: "toolu_1", Name: "drive", Content: `{"ok":true}`},
		Message{Role: RoleUser, Content: `{"von":"Team","text":"weiter"}`},
	)
	if _, _, err := c.Chat(context.Background(), Request{Messages: history, Tools: []Tool{driveTool}}); err != nil {
		t.Fatal(err)
	}
	req := f.requests[1]
	if _, ok := req["output_config"]; ok {
		t.Errorf("effort sent although not configured: %v", req["output_config"])
	}
	msgs := req["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages = %v", msgs)
	}
	assistant := msgs[1].(map[string]any)["content"].([]any)
	first := assistant[0].(map[string]any)
	if first["type"] != "thinking" || first["signature"] != "sig-1" {
		t.Errorf("thinking block not replayed unchanged: %v", assistant)
	}
	user := msgs[2].(map[string]any)
	blocks := user["content"].([]any)
	if user["role"] != "user" || len(blocks) != 2 || blocks[0].(map[string]any)["type"] != "tool_result" ||
		blocks[0].(map[string]any)["tool_use_id"] != "toolu_1" || blocks[1].(map[string]any)["type"] != "text" {
		t.Errorf("tool result turn = %v", user)
	}
}

func TestClaudeReadsHistoryOfOtherProviders(t *testing.T) {
	_, msgs, err := toAnthropicMessages([]Message{
		{Role: RoleSystem, Content: "s"},
		{Role: RoleUser, Content: "los"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call1", Type: "function", Function: FunctionCall{Name: "drive", Arguments: `{"cm":5}`}}}},
		{Role: RoleTool, ToolCallID: "call1", Content: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(msgs)
	if len(msgs) != 3 || !strings.Contains(string(raw), `"input":{"cm":5}`) || !strings.Contains(string(raw), `"id":"call1"`) ||
		!strings.Contains(string(raw), `"tool_use_id":"call1"`) {
		t.Errorf("messages = %s", raw)
	}
}

func TestClaudeRefusalFallsBackWithoutRetry(t *testing.T) {
	f := &fakeClaude{t: t, answers: []string{claudeRefusal}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := claudeClient(t, srv, "")
	c.fallback = "backup"
	c.Use("backup", "testmodell", Fake{Delay: time.Millisecond})

	_, call, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "los"}}})
	if err != nil {
		t.Fatal(err)
	}
	if f.n.Load() != 1 || call.Provider != "backup" {
		t.Errorf("calls to Claude = %d, provider = %s", f.n.Load(), call.Provider)
	}
	if retryable(fmt.Errorf("%w (cyber)", ErrRefused)) {
		t.Error("a refusal must not be retried")
	}
}

func TestClaudeRetriesRateLimits(t *testing.T) {
	f := &fakeClaude{t: t, answers: []string{"HTTP 429", claudeTextAnswer}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := claudeClient(t, srv, "")
	resp, _, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "los"}}})
	if err != nil || resp.Message.Content != "Fertig." || f.n.Load() != 2 {
		t.Errorf("resp = %+v err = %v calls = %d", resp.Message, err, f.n.Load())
	}
}

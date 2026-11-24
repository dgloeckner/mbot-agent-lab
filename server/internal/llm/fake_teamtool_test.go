package llm_test

import (
	"context"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
)

// The scripted model recognizes team tools by the sentence the agent runtime
// appends, calls the first one at the start of a run and finishes after it.
func TestFakeUsesATeamTool(t *testing.T) {
	tools := []llm.Tool{
		{Name: "read_sensors", Parameters: []byte(`{}`)},
		{Name: "drive_until", Parameters: []byte(`{}`)},
		{Name: "finish", Parameters: []byte(`{}`)},
		{Name: "labyrinth", Description: "Fährt durchs Labyrinth. " + agent.TeamToolNote, Parameters: []byte(`{}`)},
	}
	f := llm.Fake{Delay: 1}
	start := []llm.Message{{Role: llm.RoleSystem, Content: "x"}, {Role: llm.RoleUser, Content: `{"von":"System","art":"start"}`}}
	resp, err := f.Chat(context.Background(), llm.Request{Messages: start, Tools: tools})
	if err != nil || len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "labyrinth" {
		t.Fatalf("first answer = %+v, %v", resp.Message, err)
	}
	after := append(start, resp.Message, llm.Message{Role: llm.RoleTool, Name: "labyrinth", ToolCallID: resp.Message.ToolCalls[0].ID,
		Content: `{"ok":true,"end":"done","steps":5,"value":"blue"}`})
	resp, _ = f.Chat(context.Background(), llm.Request{Messages: after, Tools: tools})
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "finish" {
		t.Errorf("second answer = %+v", resp.Message)
	}
}

// With only the team's tools offered, it tries drive_until once anyway, then
// takes the team tool.
func TestFakeTriesAnUnofferedToolOnce(t *testing.T) {
	tools := []llm.Tool{
		{Name: "read_sensors", Parameters: []byte(`{}`)},
		{Name: "finish", Parameters: []byte(`{}`)},
		{Name: "labyrinth", Description: "Fährt durchs Labyrinth. " + agent.TeamToolNote, Parameters: []byte(`{}`)},
	}
	f := llm.Fake{Delay: 1}
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "x"}, {Role: llm.RoleUser, Content: `{"von":"System","art":"start"}`}}
	resp, _ := f.Chat(context.Background(), llm.Request{Messages: msgs, Tools: tools})
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "drive_until" {
		t.Fatalf("first answer = %+v", resp.Message)
	}
	msgs = append(msgs, resp.Message, llm.Message{Role: llm.RoleTool, Name: "drive_until", ToolCallID: resp.Message.ToolCalls[0].ID,
		Content: `{"ok":false,"error":"Das Werkzeug \"drive_until\" gibt es nicht."}`})
	resp, _ = f.Chat(context.Background(), llm.Request{Messages: msgs, Tools: tools})
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "labyrinth" {
		t.Errorf("second answer = %+v", resp.Message)
	}
}

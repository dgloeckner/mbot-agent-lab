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

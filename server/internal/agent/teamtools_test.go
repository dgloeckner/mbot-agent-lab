package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

const squareTree = `{"name":"quadrat","description":"Fährt ein Quadrat.","body":[{"id":"r","type":"repeat","times":2,"body":[{"id":"d","type":"call","tool":"drive","args":{"cm":10}},{"id":"t","type":"call","tool":"turn","args":{"degrees":90}}]}]}`

func saveTool(t *testing.T, st *store.Store, team, id, name, desc, tree string, valid bool) {
	t.Helper()
	problems := `[]`
	if !valid {
		problems = `[{"text":"kaputt"}]`
	}
	create := true
	if _, err := st.GetProgram(context.Background(), pairOf(team), id); err == nil {
		create = false
	}
	if _, err := st.SaveProgram(context.Background(), pairOf(team), store.Program{ID: id, Name: name, Description: desc,
		Blockly: json.RawMessage(`{}`), Tree: json.RawMessage(tree), Problems: json.RawMessage(problems)}, create); err != nil {
		t.Fatal(err)
	}
}

func toolsByName(req llm.Request) map[string]llm.Tool {
	out := map[string]llm.Tool{}
	for _, tl := range req.Tools {
		out[tl.Name] = tl
	}
	return out
}

func TestDeployedTeamToolsAreAgentTools(t *testing.T) {
	f := newFixture(t, config.Budgets{MaxStepsPerWakeup: 4})
	saveTool(t, f.st, "rot", "p1", "quadrat", "Fährt ein Quadrat.", squareTree, true)
	saveTool(t, f.st, "rot", "p2", "kreis", "Fährt im Kreis.", squareTree, true)
	saveTool(t, f.st, "rot", "p3", "kaputt", "Geht nicht.", squareTree, false)
	saveTool(t, f.st, "rot", "p4", "stumm", "", squareTree, true)
	f.deploy(t, "rot", "Nutze `quadrat`.")
	versions, _ := f.st.ListPrompts(context.Background(), pairOf("rot"))
	if len(versions[0].Tools) != 4 {
		t.Errorf("the version holds %+v", versions[0].Tools)
	}
	// Changed after the deploy: the agent still runs the old form (two rounds).
	saveTool(t, f.st, "rot", "p1", "quadrat", "Fährt ein Quadrat.", strings.Replace(squareTree, `"times":2`, `"times":4`, 1), true)

	f.model.next = func(n int, req llm.Request) llm.Message {
		if n == 1 {
			return toolCall("c1", "quadrat", `{}`)
		}
		return toolCall("c2", "finish", `{"report":"fertig"}`)
	}
	if err := f.m.Start(context.Background(), "rot"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the run to finish", func() bool { return !f.m.Status("rot").Running })

	reqs := f.model.calls()
	tools := toolsByName(reqs[0])
	if _, ok := tools["quadrat"]; !ok {
		t.Fatal("quadrat is not a tool")
	}
	if _, ok := tools["kreis"]; !ok {
		t.Error("kreis is not a tool")
	}
	for _, left := range []string{"kaputt", "stumm"} {
		if _, ok := tools[left]; ok {
			t.Errorf("%s must be left out", left)
		}
	}
	if q := tools["quadrat"]; !strings.HasSuffix(q.Description, TeamToolNote) || strings.Contains(string(q.Parameters), "robot") {
		t.Errorf("quadrat = %+v", q)
	}
	// The model gets the summary only.
	var result map[string]any
	for _, m := range reqs[1].Messages {
		if m.Role == llm.RoleTool && m.Name == "quadrat" {
			_ = json.Unmarshal([]byte(m.Content), &result)
		}
	}
	if result["end"] != "done" || result["steps"] != 4.0 || result["last"] == nil {
		t.Errorf("result for the model = %v", result)
	}
	// Steps are stored under the run, named after the team tool; they are no model steps.
	steps, _ := f.st.RunSteps(context.Background(), f.m.Status("rot").RunID)
	_ = steps
	var n int
	_ = f.st.DB.QueryRow(`SELECT count(*) FROM tool_calls WHERE program = 'quadrat'`).Scan(&n)
	if n != 4 {
		t.Errorf("stored steps = %d, want 4 (the deployed form)", n)
	}
	if len(reqs) != 2 {
		t.Errorf("model calls = %d", len(reqs))
	}
}

func TestTeamToolWithTwoRobotsAndAnAbort(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	_ = f.m.Robots.Assign("mbot-2", robots.Owner{TeamID: "rot", TeamName: "Rot", Alias: "Träger"})
	tree := `{"name":"spur","description":"Fährt und dreht.","body":[{"id":"d","type":"call","tool":"drive","args":{"cm":10}},{"id":"t","type":"call","tool":"drive","args":{"cm":10}}]}`
	saveTool(t, f.st, "rot", "p1", "spur", "Fährt und dreht.", tree, true)
	f.deploy(t, "rot", "Los.")
	tt, _ := f.m.versionTools(context.Background(), pairOf("rot"))
	defs := f.m.teamToolDefs("rot", tt)
	if len(defs) != 1 || !strings.Contains(string(defs[0].Parameters), `"robot"`) || !strings.Contains(string(defs[0].Parameters), "Träger") {
		t.Fatalf("defs = %+v", defs)
	}
	// The robot is offline: the first step fails hard. The result names the end
	// and the block and holds only that step's result.
	f.bus.deliver("lab/mbot-1/status", "offline")
	a := f.m.actor("rot", true)
	raw, steps := a.runTeamTool(context.Background(), a.env(context.Background(), &run{task: pairOf("rot")}), "spur", tt["spur"], `{"robot":"Scout"}`)
	var res map[string]any
	_ = json.Unmarshal(raw, &res)
	last, _ := res["last"].(map[string]any)
	if res["ok"] != false || res["end"] != "aborted" || res["at"] != "fahre 10 cm" || last["reason"] != "offline" || len(steps) != 1 {
		t.Errorf("result = %v, steps %d", res, len(steps))
	}
	if _, ok := res["floor"]; ok {
		t.Error("the summary holds a floor trace")
	}
}

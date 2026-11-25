package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/programs"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Team tools as agent tools: every valid team tool with a description in the
// deployed version is a tool of its own. It runs inside the agent's tool
// call; the agent waits and gets only the summary, the team sees the steps.

// TeamToolNote is appended to the description of every team tool.
const TeamToolNote = "Dies ist ein Werkzeug, das das Team selbst gebaut hat. Es läuft ohne Rückfragen durch und meldet am Ende, ob es fertig wurde oder wo es abbrach."

// TeamOnlyTools are the built-in tools the agent keeps in a task with the
// tool set team_tools: they move no robot. Everything that moves a robot
// comes from the team's own tools.
var TeamOnlyTools = map[string]bool{"read_sensors": true, "say": true, "finish": true, "tell_team": true, "remember": true, "forget": true}

// TeamOnlyToolNames returns TeamOnlyTools in catalog order.
func TeamOnlyToolNames() []string {
	var out []string
	for _, info := range tools.Catalog {
		if TeamOnlyTools[info.Name] {
			out = append(out, info.Name)
		}
	}
	return out
}

// toolSet returns the task's tool set (store.AgentToolsAll or AgentToolsTeam).
func (m *Manager) toolSet(ctx context.Context, task string) string {
	t, err := m.Store.Task(ctx, task)
	if err != nil || t.AgentTools == "" {
		return store.AgentToolsAll
	}
	return t.AgentTools
}

// allowedDefs filters the built-in tools for a task's tool set.
func allowedDefs(defs []llm.Tool, set string) []llm.Tool {
	if set != store.AgentToolsTeam {
		return defs
	}
	out := defs[:0:0]
	for _, d := range defs {
		if TeamOnlyTools[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// unknownTool answers a call of a tool the agent does not have, like the
// registry answers an unknown tool, so the agent can correct itself.
func unknownTool(name string, defs []llm.Tool) json.RawMessage {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return mustJSON(map[string]any{"ok": false, "error": fmt.Sprintf("Das Werkzeug %q gibt es nicht. Verfügbar: %s.", name, strings.Join(names, ", "))})
}

// teamTool is a team tool of a version, ready to run.
type teamTool struct {
	id   string
	prog *programs.Program
}

// versionTools loads the team tools the agent has in the newest version of
// a pair: valid ones with a description. Without a version there are none.
func (m *Manager) versionTools(ctx context.Context, tt store.TeamTask) (map[string]teamTool, error) {
	p, err := m.Store.GetPrompt(ctx, tt, 0)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil // no version, no team tools
	}
	if err != nil {
		return nil, err
	}
	list, err := m.Store.VersionPrograms(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]teamTool{}
	for _, row := range list {
		if !row.Valid() || strings.TrimSpace(row.Description) == "" {
			continue
		}
		prog, err := programs.Parse(row.Tree)
		if err != nil {
			continue
		}
		out[row.Name] = teamTool{id: row.ID, prog: prog}
	}
	return out, nil
}

// teamToolDefs are the model-facing definitions of team tools: name,
// description with the fixed sentence, no parameters but robot with several
// robots. Sorted by name.
func (m *Manager) teamToolDefs(teamID string, tt map[string]teamTool) []llm.Tool {
	names := make([]string, 0, len(tt))
	for n := range tt {
		names = append(names, n)
	}
	sort.Strings(names)
	team := m.Robots.TeamRobots(teamID)
	props := map[string]any{}
	schema := map[string]any{"type": "object", "properties": props}
	if len(team) > 1 {
		var aliases []string
		for _, rb := range team {
			aliases = append(aliases, rb.Alias)
		}
		props["robot"] = map[string]any{"type": "string", "enum": aliases, "description": "Welcher Roboter."}
		schema["required"] = []string{"robot"}
	}
	raw, _ := json.Marshal(schema)
	out := make([]llm.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, llm.Tool{Name: n, Description: strings.TrimSpace(tt[n].prog.Description) + " " + TeamToolNote, Parameters: raw})
	}
	return out
}

// teamToolRobot resolves the robot a team tool call runs on.
func (m *Manager) teamToolRobot(teamID, rawArgs string) (robots.Robot, error) {
	team := m.Robots.TeamRobots(teamID)
	var args struct {
		Robot string `json:"robot"`
	}
	_ = json.Unmarshal([]byte(rawArgs), &args)
	switch {
	case len(team) == 0:
		return robots.Robot{}, errors.New("Eurem Team ist gerade kein Roboter zugeteilt.")
	case len(team) == 1:
		return team[0], nil
	}
	for _, rb := range team {
		if strings.EqualFold(rb.Alias, args.Robot) {
			return rb, nil
		}
	}
	return robots.Robot{}, fmt.Errorf("Gib mit robot an, welcher Roboter gemeint ist: %s.", aliasList(team))
}

// teamToolScope is the scope of a team tool call: its robot.
func (m *Manager) teamToolScope(teamID, rawArgs string) tools.Scope {
	rb, err := m.teamToolRobot(teamID, rawArgs)
	if err != nil {
		return tools.Scope{}
	}
	return tools.Scope{Robots: []string{rb.ID}, Aliases: []string{rb.Alias}}
}

// teamToolStep is one step of a team tool run inside an agent call, stored
// with the call.
type teamToolStep struct {
	tool         string
	args, result json.RawMessage
	d            time.Duration
	robot        store.RobotRef
}

// runTeamTool runs a team tool inside an agent tool call. The result for the
// model is the summary only: {ok, end, steps, runtime_s, at, last}, a
// returned value and, if it did not end well, the reason in words.
func (a *actor) runTeamTool(ctx context.Context, env *tools.Env, name string, t teamTool, rawArgs string) (json.RawMessage, []teamToolStep) {
	rb, err := a.m.teamToolRobot(a.team, rawArgs)
	if err != nil {
		return mustJSON(map[string]any{"ok": false, "error": err.Error()}), nil
	}
	var mu sync.Mutex
	var steps []teamToolStep
	ref := store.RobotRef{ID: rb.ID, Alias: rb.Alias}
	c := &programCaller{m: a.m, team: a.team, robot: rb, name: name, env: env, lastAt: time.Now(), source: live.SourceAgent,
		onStep: func(tool string, args, result json.RawMessage, d time.Duration) {
			mu.Lock()
			steps = append(steps, teamToolStep{tool: tool, args: args, result: result, d: d, robot: ref})
			mu.Unlock()
		}}
	a.m.Log.Info("team tool called by the agent", "team", a.team, "program", name, "robot", rb.Alias, "run", env.RunID)
	res := programs.Run(ctx, t.prog, c, programs.Options{Describe: programs.Describer(a.m.Tools)})
	a.m.Log.Info("team tool ended", "team", a.team, "program", name, "end", res.End, "steps", res.Steps, "runtime_s", res.RuntimeS,
		"at", res.At, "tool", res.Tool, "reason", res.Reason, "run", env.RunID)
	out := map[string]any{"ok": res.OK, "end": res.End, "steps": res.Steps, "runtime_s": res.RuntimeS, "robot": rb.Alias}
	if res.At != "" {
		out["at"] = res.At
	}
	if len(res.Last) > 0 {
		out["last"] = res.Last
	}
	if res.Value != nil {
		out["value"] = res.Value
	}
	if res.Error != "" {
		out["error"] = res.Error
	}
	return mustJSON(out), steps
}

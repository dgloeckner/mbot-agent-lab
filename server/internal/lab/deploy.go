package lab

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// One deploy is one version of the prompt and all team tools of the pair
// (store.SavePrompt freezes them). The agent gets the valid team tools with
// a description; "zurück" restores a whole version.

// VersionTool is a team tool of a version and whether the agent has it.
type VersionTool struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Agent bool   `json:"agent"`
	// Why says why the agent does not have it: "Fehler" or "ohne Beschreibung".
	Why string `json:"why,omitempty"`
}

// Deploy states of a team tool in the working state, against the newest version.
const (
	DeployDeployed = "deployed" // as in the newest version
	DeployChanged  = "changed"  // changed since the newest version
	DeployNew      = "new"      // not in the newest version
)

func versionTool(p store.Program) VersionTool {
	t := VersionTool{ID: p.ID, Name: p.Name, Agent: true}
	switch {
	case !p.Valid():
		t.Agent, t.Why = false, "Fehler"
	case strings.TrimSpace(p.Description) == "":
		t.Agent, t.Why = false, "ohne Beschreibung"
	}
	return t
}

// VersionTools lists the team tools frozen with a version.
func (s *Service) VersionTools(ctx context.Context, promptID int64) ([]VersionTool, error) {
	list, err := s.Store.VersionPrograms(ctx, promptID)
	if err != nil {
		return nil, err
	}
	out := make([]VersionTool, 0, len(list))
	for _, p := range list {
		out = append(out, versionTool(p))
	}
	return out, nil
}

var codeName = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*)`")

// toolWords is what the deploy warning needs from the tool registry.
type toolWords interface {
	Known(name string) bool
	Words() map[string]bool
}

// DeployWarning names what the prompt writes in code style like a tool call
// the agent cannot make: a name that is no team tool of the version, no
// built-in tool, no robot of the team and no word of the tools (argument
// names, result fields and values such as reason, wall or color); and in a
// task with the tool set team_tools, a built-in tool the agent does not have
// there. It is a warning, not a block: the deploy happens anyway.
func (s *Service) DeployWarning(teamID, text, toolSet string, tools []VersionTool) string {
	known := map[string]bool{"alle": true, "all": true}
	for _, t := range tools {
		known[t.Name] = true
	}
	for _, rb := range s.Robots.TeamRobots(teamID) {
		known[strings.ToLower(rb.Alias)] = true
	}
	reg, _ := s.Tools.(toolWords)
	var words map[string]bool
	if reg != nil {
		words = reg.Words()
	}
	var unknown, missing []string
	seen := map[string]bool{}
	for _, m := range codeName.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if seen[name] || known[name] || known[strings.ToLower(name)] {
			continue
		}
		seen[name] = true
		switch {
		case reg != nil && reg.Known(name):
			if toolSet == store.AgentToolsTeam && !agent.TeamOnlyTools[name] {
				missing = append(missing, "`"+name+"`")
			}
		case words[name]:
		default:
			unknown = append(unknown, "`"+name+"`")
		}
	}
	var out []string
	switch len(unknown) {
	case 0:
	case 1:
		out = append(out, fmt.Sprintf("Im Prompt steht %s, aber ein Werkzeug mit diesem Namen hat euer Agent nicht.", unknown[0]))
	default:
		out = append(out, fmt.Sprintf("Im Prompt stehen %s, aber Werkzeuge mit diesen Namen hat euer Agent nicht.", strings.Join(unknown, ", ")))
	}
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf("%s hat euer Agent in dieser Aufgabe nicht.", joinAnd(missing)))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, " ") + " Deployt ist trotzdem."
}

// joinAnd joins names German style: "a", "a und b", "a, b und c".
func joinAnd(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " und " + names[len(names)-1]
}

// RestoreVersion replaces the working state of the active, open task with a
// version: its team tools (also ones deleted since) here, its prompt text
// for the page's draft. The history keeps everything; the next deploy is a
// new version.
func (s *Service) RestoreVersion(ctx context.Context, teamID string, version int) (store.Prompt, error) {
	tt, err := s.WritableTask(ctx, teamID)
	if err != nil {
		return store.Prompt{}, err
	}
	p, err := s.Store.RestoreVersion(ctx, tt, version)
	if err != nil {
		return p, err
	}
	s.Log.Info("version restored", "team", teamID, "task", tt.Task, "version", version)
	s.Hub.SignalRef(teamID, live.SignalPrograms, "")
	return p, nil
}

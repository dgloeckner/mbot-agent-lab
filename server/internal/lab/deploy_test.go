package lab

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// starter returns the starter prompt and the tool set of a task file
// (docs/lab/aufgaben): the text under "## Startprompt" or "## Starter prompt".
func starter(t *testing.T, path string) (text, toolSet string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	toolSet = store.AgentToolsAll
	if m := regexp.MustCompile(`(?m)^agent_tools:\s*(\S+)`).FindStringSubmatch(src); m != nil {
		toolSet = m[1]
	}
	loc := regexp.MustCompile(`(?mi)^##\s+(startprompt|starter prompt)\s*$`).FindStringIndex(src)
	if loc == nil {
		t.Fatalf("%s: no starter prompt", path)
	}
	return src[loc[1]:], toolSet
}

func TestDeployWarningStarterPrompts(t *testing.T) {
	s, _ := taskService(t)
	files, _ := filepath.Glob("../../../docs/lab/aufgaben/*/*.md")
	if len(files) < 10 {
		t.Fatalf("task files = %v", files)
	}
	// The team tools the "Werkzeuge" series has teams build; a starter prompt may name them.
	var built []VersionTool
	for _, n := range []string{"quadrat", "bis_zur_wand", "labyrinth", "ein_feld", "rechts", "links"} {
		built = append(built, VersionTool{Name: n, Agent: true})
	}
	for _, f := range files {
		text, set := starter(t, f)
		if w := s.DeployWarning("team", text, store.AgentToolsAll, built); w != "" {
			t.Errorf("%s in an all task: %s", f, w)
		}
		if w := s.DeployWarning("team", text, set, built); w != "" {
			t.Errorf("%s with its own tool set %s: %s", f, set, w)
		}
	}
}

func TestDeployWarning(t *testing.T) {
	s, _ := taskService(t)
	own := []VersionTool{{Name: "quadrat", Agent: true}}
	for _, c := range []struct {
		name, text, set string
		want            []string // parts of the warning; nil: no warning
	}{
		{"result words", "Schau dir `reason` an: bei `wall` drehen, bei `color` und `blue` anhalten, `driven_cm` melden.", store.AgentToolsAll, nil},
		{"argument names", "Gib `max_cm` und `stop_below_cm` an.", store.AgentToolsAll, nil},
		{"team tool and built-in", "Nutze `quadrat`, dann `drive_until` und `finish`.", store.AgentToolsAll, nil},
		{"unknown name", "Nutze `zickzack`.", store.AgentToolsAll, []string{"Im Prompt steht `zickzack`", "Deployt ist trotzdem."}},
		{"two unknown", "Nutze `zickzack` und `kreis`.", store.AgentToolsAll, []string{"`zickzack`, `kreis`", "Werkzeuge mit diesen Namen"}},
		{"team tools task keeps finish", "Nutze `quadrat`, `read_sensors` und `finish`.", store.AgentToolsTeam, nil},
		{"team tools task: built-in outside the list", "Nutze `drive_until` und `turn`, dann `finish`.", store.AgentToolsTeam,
			[]string{"`drive_until` und `turn` hat euer Agent in dieser Aufgabe nicht.", "Deployt ist trotzdem."}},
		{"both kinds", "Nutze `drive` und `zickzack`.", store.AgentToolsTeam, []string{"Im Prompt steht `zickzack`", "`drive` hat euer Agent in dieser Aufgabe nicht."}},
	} {
		w := s.DeployWarning("team", c.text, c.set, own)
		if c.want == nil {
			if w != "" {
				t.Errorf("%s: unexpected warning %q", c.name, w)
			}
			continue
		}
		for _, part := range c.want {
			if !strings.Contains(w, part) {
				t.Errorf("%s: warning %q lacks %q", c.name, w, part)
			}
		}
		if strings.Count(w, "Deployt ist trotzdem.") != 1 {
			t.Errorf("%s: %q", c.name, w)
		}
	}
}

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/programs"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

func TestHardEnds(t *testing.T) {
	cases := []struct {
		reason string
		normal bool
		ctx    error
		want   bool
	}{
		{"offline", false, nil, true},
		{"timeout", false, nil, true}, // the robot did not answer
		{"timeout", true, nil, false}, // wait_for without the event
		{"reassigned", false, nil, true},
		{"stopped", false, nil, true},
		{"obstacle", false, nil, false}, // unexpected, the error rule decides
		{"done", true, context.Canceled, true},
	}
	for _, c := range cases {
		if got := hardEnd(c.reason, c.normal, c.ctx); got != c.want {
			t.Errorf("hardEnd(%q, %v, %v) = %v", c.reason, c.normal, c.ctx, got)
		}
	}
}

// loader serves one parsed team tool.
func loader(t *testing.T, body string, problems ...programs.Problem) ProgramLoader {
	p, err := programs.Parse([]byte(`{"name":"test_tool","body":` + body + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context, string, string) (store.TeamTask, *programs.Program, []programs.Problem, error) {
		return pairOf("rot"), p, problems, nil
	}
}

func TestOneTeamToolPerTeamAndHardAborts(t *testing.T) {
	f := newFixture(t, config.Budgets{})
	ctx := context.Background()
	f.m.LoadProgram = loader(t, `[{"id":"d","type":"call","tool":"drive","args":{"cm":50}},{"id":"t","type":"call","tool":"turn","args":{"degrees":90}}]`)
	f.bus.mu.Lock()
	f.bus.gate = true
	f.bus.mu.Unlock()
	run, err := f.m.RunProgram(ctx, "rot", "p1", "", false)
	if err != nil || run == "" {
		t.Fatalf("run = %q, %v", run, err)
	}
	waitFor(t, "the drive to be held", func() bool { return len(f.bus.holding()) == 1 })
	if _, err := f.m.RunProgram(ctx, "rot", "p1", "", false); err == nil {
		t.Error("a second team tool started")
	}
	f.deploy(t, "rot", "Los.")
	if err := f.m.Start(ctx, "rot"); !errors.Is(err, ErrManualRunning) {
		t.Errorf("start during a team tool: %v", err)
	}
	// The robot goes offline: the held drive ends offline, a hard abort.
	f.bus.deliver("lab/mbot-1/status", "offline")
	f.m.Stop("rot", "Team") // cancels the held call at once
	waitFor(t, "the team tool to end", func() bool { p, _ := f.m.RunningProgram("rot"); return p == "" })
	f.bus.release()

	// Invalid tools do not run.
	f.m.LoadProgram = loader(t, `[]`, programs.Problem{Text: "leer"})
	var se *StartError
	if _, err := f.m.RunProgram(ctx, "rot", "p1", "", false); !errors.As(err, &se) {
		t.Errorf("invalid tool: %v", err)
	}
}

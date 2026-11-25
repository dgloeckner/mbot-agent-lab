package lab

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/agent"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Scoring a challenge round in the control room ("Läufe"): every run of a
// task with four measures (success as the lab lead confirms it, time to the
// goal, model calls as cost, interventions) and a ranking of the teams.

// ErrUnknownRun is returned for a run that does not exist (or is no run).
var ErrUnknownRun = errors.New("unknown run")

// Intervention kinds the lab lead enters.
const InterventionMoved = "moved" // the robot was moved by hand

var scoresGen atomic.Int64

// ScoresVersion changes whenever a confirmation or an intervention is
// recorded; the control room's stream reports it so open views reload.
func (s *Service) ScoresVersion() int64 { return scoresGen.Load() }

// RunView is a run with the tools it ran with.
type RunView struct {
	store.RunScore
	// Tools: the built-in tools the agent had (the task's tool set); TeamTools:
	// the team tools of the run's version the agent had.
	Tools     []string `json:"tools"`
	TeamTools []string `json:"team_tools"`
}

// TeamScore is one team in a task's ranking: its runs and its best run.
type TeamScore struct {
	TeamID    string    `json:"team_id"`
	Name      string    `json:"name"`
	Runs      []RunView `json:"runs"`
	Successes int       `json:"successes"`
	// Best is the run the ranking uses: a success with the fewest model
	// calls, then the shortest time; without a success the run with the
	// fewest calls.
	Best *RunView `json:"best,omitempty"`
	Rank int      `json:"rank"`
}

// better reports whether run a beats run b: success, then fewer model calls, then less time.
func better(a, b *RunView) bool {
	sa, sb := a.Success != nil && *a.Success, b.Success != nil && *b.Success
	if sa != sb {
		return sa
	}
	if a.Calls != b.Calls {
		return a.Calls < b.Calls
	}
	return timeOf(a) < timeOf(b)
}

func timeOf(r *RunView) float64 {
	if r == nil || r.ToGoalS == nil {
		return 1e18
	}
	return *r.ToGoalS
}

// Rank orders the teams of a task: successes first, then cost (model calls
// of the best run), then time to the goal. Teams equal on all three share a
// rank. Pure: it only sorts and numbers.
func Rank(teams []TeamScore) {
	for i := range teams {
		teams[i].Best = nil
		teams[i].Successes = 0
		for j := range teams[i].Runs {
			r := &teams[i].Runs[j]
			if r.Success != nil && *r.Success {
				teams[i].Successes++
			}
			if teams[i].Best == nil || better(r, teams[i].Best) {
				teams[i].Best = r
			}
		}
	}
	key := func(t TeamScore) (int, int, float64) {
		calls := 1 << 30
		if t.Best != nil {
			calls = t.Best.Calls
		}
		return -t.Successes, calls, timeOf(t.Best)
	}
	sort.SliceStable(teams, func(i, j int) bool {
		a1, a2, a3 := key(teams[i])
		b1, b2, b3 := key(teams[j])
		if a1 != b1 {
			return a1 < b1
		}
		if a2 != b2 {
			return a2 < b2
		}
		return a3 < b3
	})
	for i := range teams {
		teams[i].Rank = i + 1
		if i > 0 {
			p1, p2, p3 := key(teams[i-1])
			c1, c2, c3 := key(teams[i])
			if p1 == c1 && p2 == c2 && p3 == c3 {
				teams[i].Rank = teams[i-1].Rank
			}
		}
	}
}

// TaskScores lists every team with runs in a task, with its runs, and ranks them.
func (s *Service) TaskScores(ctx context.Context, taskID string) ([]TeamScore, error) {
	task, err := s.Task(ctx, taskID)
	if err != nil {
		return nil, err
	}
	runs, err := s.Store.TaskRuns(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var builtins []string
	for _, info := range tools.Catalog {
		if info.Ready && (task.AgentTools != store.AgentToolsTeam || agent.TeamOnlyTools[info.Name]) {
			builtins = append(builtins, info.Name)
		}
	}
	byTeam := map[string]*TeamScore{}
	var order []string
	for _, r := range runs {
		t := byTeam[r.TeamID]
		if t == nil {
			team, err := s.Store.Team(ctx, r.TeamID)
			if err != nil {
				team.Name = r.TeamID
			}
			t = &TeamScore{TeamID: r.TeamID, Name: team.Name}
			byTeam[r.TeamID] = t
			order = append(order, r.TeamID)
		}
		v := RunView{RunScore: r, Tools: builtins, TeamTools: []string{}}
		if r.PromptID != 0 {
			vt, err := s.VersionTools(ctx, r.PromptID)
			if err != nil {
				return nil, err
			}
			for _, x := range vt {
				if x.Agent {
					v.TeamTools = append(v.TeamTools, x.Name)
				}
			}
		}
		t.Runs = append(t.Runs, v)
	}
	out := make([]TeamScore, 0, len(order))
	for _, id := range order {
		out = append(out, *byTeam[id])
	}
	Rank(out)
	return out, nil
}

// ConfirmRun records the lab lead's verdict on a run (run.confirmed). A run
// confirmed twice keeps the last answer.
func (s *Service) ConfirmRun(ctx context.Context, runID int64, success bool) error {
	tt, prompt, err := s.Store.RunScope(ctx, runID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrUnknownRun
	} else if err != nil {
		return err
	}
	if err := s.Store.AddEvent(ctx, store.Event{Kind: store.EventRunConfirmed, Actor: store.ActorAdmin, Team: tt.Team, Task: tt.Task,
		Prompt: prompt, Run: runID, Data: map[string]any{"success": success}}); err != nil {
		return err
	}
	scoresGen.Add(1)
	s.Log.Info("run confirmed", "team", tt.Team, "task", tt.Task, "run", runID, "success", success)
	return nil
}

// AddIntervention records an intervention the lab lead saw (run.intervention).
func (s *Service) AddIntervention(ctx context.Context, runID int64, kind string) error {
	if strings.TrimSpace(kind) == "" {
		kind = InterventionMoved
	}
	if kind != InterventionMoved {
		return userErr("Diesen Eingriff kenne ich nicht: %s.", kind)
	}
	tt, prompt, err := s.Store.RunScope(ctx, runID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrUnknownRun
	} else if err != nil {
		return err
	}
	if err := s.Store.AddEvent(ctx, store.Event{Kind: store.EventRunIntervention, Actor: store.ActorAdmin, Team: tt.Team, Task: tt.Task,
		Prompt: prompt, Run: runID, Data: map[string]any{"kind": kind}}); err != nil {
		return err
	}
	scoresGen.Add(1)
	s.Log.Info("run intervention", "team", tt.Team, "task", tt.Task, "run", runID, "kind", kind)
	return nil
}

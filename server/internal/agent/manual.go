package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Refusals around by-hand work, in German. There is no driver and no pause:
// by-hand calls work only while no run is active, and a run cannot start
// while one is running.
var (
	ErrRunActive     = &StartError{"Erst anhalten: Euer Agent fährt gerade."}
	ErrManualRunning = &StartError{"Erst warten: Ihr steuert gerade selbst einen Roboter. Stoppt oder wartet, bis er fertig ist."}
)

// gate keeps by-hand calls and run starts of one team apart. Two by-hand
// calls of the same team may overlap (two PCs): the robot answers the second
// motion command with busy.
type gate struct {
	mu       sync.Mutex
	starting bool
	manual   map[int64]context.CancelFunc // running by-hand calls
	next     int64
	seq      sync.Mutex // orders protocol rows of the team's manual run
}

func (m *Manager) gate(teamID string) *gate {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.gates[teamID]
	if g == nil {
		g = &gate{manual: map[int64]context.CancelFunc{}}
		m.gates[teamID] = g
	}
	return g
}

// beginStart marks a run start; it fails while a by-hand call runs.
func (m *Manager) beginStart(teamID string) error {
	g := m.gate(teamID)
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.manual) > 0 {
		return ErrManualRunning
	}
	g.starting = true
	return nil
}

func (m *Manager) endStart(teamID string) {
	g := m.gate(teamID)
	g.mu.Lock()
	g.starting = false
	g.mu.Unlock()
}

// ManualBusy reports whether a by-hand call of the team is running.
func (m *Manager) ManualBusy(teamID string) bool {
	g := m.gate(teamID)
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.manual) > 0
}

// cancelManual cancels every running by-hand call of a team.
func (m *Manager) cancelManual(teamID string) {
	g := m.gate(teamID)
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, cancel := range g.manual {
		cancel()
	}
}

// Manual runs one built-in tool by hand for a team, without a language model,
// and answers its result as the agent would get it. It needs an active, open
// task and no active run (StartError otherwise). The call is stored under the
// manual row of the pair and shown in the live log with source team.
func (m *Manager) Manual(ctx context.Context, teamID, name, rawArgs string) (json.RawMessage, error) {
	tt, err := m.activeTask(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if !m.Tools.Manual(name) {
		// Unknown and agent-only tools: the registry answers why, nothing is stored.
		return m.Tools.CallManual(ctx, &tools.Env{TeamID: teamID, TaskID: tt.Task}, name, rawArgs), nil
	}

	g := m.gate(teamID)
	g.mu.Lock()
	if g.starting || m.Status(teamID).Running {
		g.mu.Unlock()
		m.Log.Info("tool by hand refused", "team", teamID, "task", tt.Task, "tool", name, "why", "run active")
		return nil, ErrRunActive
	}
	m.mu.Lock()
	callCtx, cancel := context.WithCancel(m.ctx)
	m.mu.Unlock()
	g.next++
	key := g.next
	g.manual[key] = cancel
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.manual, key)
		g.mu.Unlock()
		cancel()
	}()

	runID, err := m.Store.ManualRun(ctx, tt)
	if err != nil {
		return nil, err
	}
	team, _ := m.Store.Team(ctx, teamID)
	env := &tools.Env{TeamID: teamID, TaskID: tt.Task, TeamName: team.Name, RunID: runID}
	scope := m.Tools.Scope(env, name, rawArgs)
	start := time.Now()
	result := m.Tools.CallManual(callCtx, env, name, rawArgs)
	d := time.Since(start)

	args := json.RawMessage(validJSON(rawArgs))
	if len(rawArgs) == 0 {
		args = json.RawMessage(`{}`)
	}
	m.Hub.Publish(teamID, live.LogEntry{Source: live.SourceTeam, Kind: "tool", Data: mustJSON(map[string]any{
		"name": name, "args": args, "result": result, "ms": d.Milliseconds(),
	})})
	g.seq.Lock()
	seq, err := m.Store.NextSeq(context.WithoutCancel(ctx), runID)
	if err == nil {
		err = m.Store.AddToolCall(context.WithoutCancel(ctx), runID, seq, name, args, result, d, scope.Robot())
	}
	g.seq.Unlock()
	if err != nil {
		m.Log.Error("store tool call by hand", "team", teamID, "err", err)
	}
	var res struct {
		Reason string `json:"reason"`
		Robot  string `json:"robot"`
	}
	_ = json.Unmarshal(result, &res)
	m.Log.Info("tool by hand", "team", teamID, "task", tt.Task, "robot", res.Robot, "tool", name, "reason", res.Reason, "ms", d.Milliseconds())
	return result, nil
}

// endManual ends the open manual rows of a team, when a run starts or the
// team switches tasks.
func (m *Manager) endManual(teamID, reason string) {
	if err := m.Store.EndManualRuns(context.Background(), teamID, reason); err != nil {
		m.Log.Error("end manual runs", "team", teamID, "err", err)
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// endManualOfTask ends the open manual rows of a task the control room closed.
func (m *Manager) endManualOfTask(taskID string) {
	if err := m.Store.EndManualRunsOfTask(context.Background(), taskID, "task_closed"); err != nil {
		m.Log.Error("end manual runs", "task", taskID, "err", err)
	}
}

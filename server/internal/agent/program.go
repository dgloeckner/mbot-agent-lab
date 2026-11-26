package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/programs"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Running a team tool ("Ausführen"): the server walks the tree on one of the
// team's robots, by hand, like the console. The same rule holds: only while
// no run is active, and a run cannot start while a team tool runs. One team
// tool per team at a time. Stop cancels it at once.

// programRun is a running team tool of a team.
type programRun struct {
	id      string
	program string // team tool id
	name    string
	task    string
	step    chan struct{} // single step: one value lets the next call run
}

// ProgramLoader loads a team tool of the team's active, open task: its pair,
// its tree and its problems (lab.Service.LoadProgram).
type ProgramLoader func(ctx context.Context, teamID, id string) (store.TeamTask, *programs.Program, []programs.Problem, error)

// freshStateTimeout bounds the wait for a robot state newer than the last result.
const freshStateTimeout = 2 * time.Second

// RunProgram starts a team tool on a robot (its name; "" with one robot) and
// answers the run id. With step the run halts before every call until
// ContinueProgram. Refusals are StartErrors (409) or the loader's errors.
func (m *Manager) RunProgram(ctx context.Context, teamID, programID, robot string, step bool) (string, error) {
	if m.LoadProgram == nil {
		return "", errors.New("no program loader")
	}
	tt, prog, problems, err := m.LoadProgram(ctx, teamID, programID)
	if err != nil {
		return "", err
	}
	if len(problems) > 0 {
		return "", &StartError{fmt.Sprintf("Das Werkzeug „%s“ hat noch Fehler. Behebt sie zuerst.", prog.Name)}
	}
	team := m.Robots.TeamRobots(teamID)
	var target robots.Robot
	switch {
	case len(team) == 0:
		return "", &StartError{"Eurem Team ist gerade kein Roboter zugeteilt."}
	case len(team) == 1 && robot == "":
		target = team[0]
	default:
		for _, rb := range team {
			if strings.EqualFold(rb.Alias, robot) {
				target = rb
			}
		}
		if target.ID == "" {
			return "", &StartError{fmt.Sprintf("Wählt, welcher Roboter das Werkzeug ausführt: %s.", aliasList(team))}
		}
	}

	g := m.gate(teamID)
	g.mu.Lock()
	switch {
	case g.starting || m.Status(teamID).Running:
		g.mu.Unlock()
		return "", ErrRunActive
	case g.prog != nil:
		name := g.prog.name
		g.mu.Unlock()
		return "", &StartError{fmt.Sprintf("Es läuft schon ein Werkzeug von euch: %s. Stoppt es oder wartet, bis es fertig ist.", name)}
	}
	m.mu.Lock()
	runCtx, cancel := context.WithCancel(m.ctx)
	m.mu.Unlock()
	g.next++
	key := g.next
	pr := &programRun{id: fmt.Sprintf("r%d-%s", key, programID), program: programID, name: prog.Name, task: tt.Task}
	if step {
		pr.step = make(chan struct{}, 1)
	}
	g.manual[key] = cancel
	g.prog = pr
	g.mu.Unlock()

	manualRun, err := m.Store.ManualRun(ctx, tt)
	if err != nil {
		g.mu.Lock()
		delete(g.manual, key)
		g.prog = nil
		g.mu.Unlock()
		cancel()
		return "", err
	}
	teamRow, _ := m.Store.Team(ctx, teamID)
	c := &programCaller{m: m, g: g, team: teamID, run: manualRun, robot: target, name: prog.Name, lastAt: time.Now(),
		env: &tools.Env{TeamID: teamID, TaskID: tt.Task, TeamName: teamRow.Name, RunID: manualRun}}

	m.Hub.Publish(teamID, live.LogEntry{Source: live.SourceTeam, Kind: "program_start", Robot: target.Alias,
		Data: mustJSON(map[string]any{"run": pr.id, "program": programID, "name": prog.Name, "robot": target.Alias, "step": step})})
	c.message("program_start", map[string]any{"run": pr.id, "program": programID, "robot": target.Alias, "step": step})
	m.signal(teamID, map[string]any{"run": pr.id, "program": programID, "name": prog.Name, "phase": "start", "robot": target.Alias, "step": step})
	m.Log.Info("team tool started", "team", teamID, "task", tt.Task, "program", prog.Name, "robot", target.Alias, "step", step, "run", pr.id)

	go func() {
		defer func() {
			g.mu.Lock()
			delete(g.manual, key)
			g.prog = nil
			g.mu.Unlock()
			cancel()
		}()
		opt := programs.Options{Describe: programs.Describer(m.Tools), Observe: func(e programs.Event) {
			m.signal(teamID, map[string]any{"run": pr.id, "program": programID, "phase": e.Phase, "step": e.Step, "block": e.Block,
				"kind": e.Kind, "tool": e.Tool, "args": e.Args, "round": e.Round, "result": e.Result, "ms": e.Ms})
		}}
		if pr.step != nil {
			opt.Step = pr.step
		}
		res := programs.Run(runCtx, prog, c, opt)
		m.Hub.Publish(teamID, live.LogEntry{Source: live.SourceTeam, Kind: "program_end", Robot: target.Alias,
			Data: mustJSON(map[string]any{"run": pr.id, "program": programID, "name": prog.Name, "result": res})})
		c.message("program_end", map[string]any{"run": pr.id, "program": programID, "result": res})
		m.signal(teamID, map[string]any{"run": pr.id, "program": programID, "phase": "end", "result": res})
		m.Log.Info("team tool ended", "team", teamID, "task", tt.Task, "program", prog.Name, "end", res.End, "steps", res.Steps,
			"runtime_s", res.RuntimeS, "at", res.At, "tool", res.Tool, "reason", res.Reason, "run", pr.id)
	}()
	return pr.id, nil
}

// ContinueProgram lets a single-step run take its next call.
func (m *Manager) ContinueProgram(teamID, runID string) error {
	g := m.gate(teamID)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.prog == nil || g.prog.id != runID || g.prog.step == nil {
		return &StartError{"Dieses Werkzeug läuft nicht mehr im Einzelschritt."}
	}
	select {
	case g.prog.step <- struct{}{}:
	default: // already told to go on
	}
	return nil
}

// RunningProgram returns the id of the team's running team tool and its run, if any.
func (m *Manager) RunningProgram(teamID string) (program, run string) {
	g := m.gate(teamID)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.prog == nil {
		return "", ""
	}
	return g.prog.program, g.prog.id
}

// cancelProgramsOfTask stops running team tools in a task (closed or left).
func (m *Manager) cancelProgramsOfTask(task string) {
	m.mu.Lock()
	gates := make([]*gate, 0, len(m.gates))
	for _, g := range m.gates {
		gates = append(gates, g)
	}
	m.mu.Unlock()
	for _, g := range gates {
		g.mu.Lock()
		if g.prog != nil && g.prog.task == task {
			for _, cancel := range g.manual {
				cancel()
			}
		}
		g.mu.Unlock()
	}
}

func (m *Manager) signal(teamID string, data map[string]any) {
	m.Hub.SignalData(teamID, live.SignalProgram, mustJSON(data))
}

func aliasList(rs []robots.Robot) string {
	names := make([]string, len(rs))
	for i, rb := range rs {
		names[i] = rb.Alias
	}
	return strings.Join(names, ", ")
}

// programCaller runs the steps of a team tool through the registry, like the
// console, and stores them under the manual row of the pair.
type programCaller struct {
	m      *Manager
	g      *gate
	team   string
	run    int64
	robot  robots.Robot
	name   string
	env    *tools.Env
	lastAt time.Time // when the last result arrived: a sensor condition needs a newer state
	// source of the log lines: the team (by hand) or its agent (a tool call).
	source string
	// onStep stores a step; nil stores it under the manual row.
	onStep func(tool string, args, result json.RawMessage, d time.Duration)
}

// hardEnd reports whether a step's end always aborts the team tool: the
// robot is offline, did not answer, was reassigned, or the run was stopped.
// A timeout that is the tool's normal end (wait_for) is not hard.
func hardEnd(reason string, normal bool, ctxErr error) bool {
	switch {
	case ctxErr != nil:
		return true
	case reason == "offline", reason == "reassigned", reason == "stopped":
		return true
	case reason == "timeout":
		return !normal
	}
	return false
}

func (c *programCaller) Call(ctx context.Context, tool string, args map[string]any) programs.Outcome {
	a := make(map[string]any, len(args)+1)
	for k, v := range args {
		a[k] = v
	}
	a["robot"] = c.robot.Alias
	raw, _ := json.Marshal(a)
	start := time.Now()
	result := c.m.Tools.CallManual(ctx, c.env, tool, string(raw))
	d := time.Since(start)
	c.lastAt = time.Now()

	stored, _ := json.Marshal(args)
	source := c.source
	if source == "" {
		source = live.SourceTeam
	}
	c.m.Hub.Publish(c.team, live.LogEntry{Source: source, Kind: "tool", Data: mustJSON(map[string]any{
		"name": tool, "args": json.RawMessage(stored), "result": result, "ms": d.Milliseconds(), "program": c.name,
		"robot": c.robot.Alias, // with several robots the page names it on every step
	})})
	if c.onStep != nil {
		c.onStep(tool, stored, result, d)
	} else {
		c.storeManual(tool, stored, result, d)
	}

	var res struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(result, &res)
	normal := c.m.Tools.NormalEnd(tool, result)
	hard := hardEnd(res.Reason, normal, ctx.Err())
	c.m.Log.Info("team tool step", "team", c.team, "program", c.name, "tool", tool, "reason", res.Reason, "normal", normal, "ms", d.Milliseconds())
	return programs.Outcome{Result: result, Normal: normal, Quiet: c.m.Tools.Quiet(tool), Hard: hard}
}

func (c *programCaller) storeManual(tool string, stored, result json.RawMessage, d time.Duration) {
	c.g.seq.Lock()
	seq, err := c.m.Store.NextSeq(context.Background(), c.run)
	if err == nil {
		err = c.m.Store.AddProgramToolCall(context.Background(), c.run, seq, c.name, tool, stored, result, d, store.RobotRef{ID: c.robot.ID, Alias: c.robot.Alias})
	}
	c.g.seq.Unlock()
	if err != nil {
		c.m.Log.Error("store team tool step", "team", c.team, "err", err)
	}
}

// Sense reads the robot's state once it is newer than the last result.
func (c *programCaller) Sense(ctx context.Context) (map[string]any, error) {
	rb, err := c.m.Robots.FreshState(ctx, c.robot.ID, c.lastAt, freshStateTimeout)
	if errors.Is(err, robots.ErrNoFreshState) {
		return nil, errors.New("keine frische Messung vom Roboter")
	}
	if err != nil {
		return nil, err
	}
	s := rb.State
	return map[string]any{"distance_cm": float64(s.DistanceCM), "floor": s.Floor, "color": s.Color, "line": s.Line}, nil
}

// message stores a start or end mark of the team tool under the manual row.
func (c *programCaller) message(kind string, data map[string]any) {
	c.g.seq.Lock()
	defer c.g.seq.Unlock()
	seq, err := c.m.Store.NextSeq(context.Background(), c.run)
	if err == nil {
		_, err = c.m.Store.AddMessage(context.Background(), c.run, seq, live.SourceTeam, kind, c.name, mustJSON(data), store.RobotRef{ID: c.robot.ID, Alias: c.robot.Alias})
	}
	if err != nil {
		c.m.Log.Error("store team tool mark", "team", c.team, "err", err)
	}
}

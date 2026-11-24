package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

type ctrlKind int

const (
	ctrlStart ctrlKind = iota
	ctrlStop
	ctrlRestart
	ctrlRemove
	ctrlTaskClosed
	ctrlTaskSwitched
	ctrlNewConversation
)

type control struct {
	kind   ctrlKind
	reason string // for stop: why the run ends
	task   string // for task closed: the task
	by     string // who stopped it, for the log
	reply  chan error
}

// run is the state of one run, owned by the actor goroutine.
type run struct {
	id            int64
	task          store.TeamTask // the task the run works in, fixed for the run
	promptID      int64          // the agent version the run started with
	promptVersion int
	provider      string // model provider of the whole run
	started       time.Time
	deadline      time.Time
	history       []llm.Message
	seq           int
	lastMsgID     int64
	tokensIn      int
	tokensOut     int
	finished      atomic.Bool // set by the finish tool, which may run beside others
	report        string
	limitReached  string // time_limit or token_limit when a budget ended the run
}

type actor struct {
	m     *Manager
	team  string
	inbox chan Message
	talk  chan Message // team messages while no run is active
	ctrl  chan control
	conv  *conversation // open conversation; only the actor goroutine touches it

	mu     sync.Mutex
	status Status
	cancel context.CancelFunc // cancels the current model call or tool
}

func newActor(m *Manager, team string) *actor {
	return &actor{m: m, team: team, inbox: make(chan Message, 256), talk: make(chan Message, 8), ctrl: make(chan control, 16), status: Status{State: StateWaiting}}
}

func (a *actor) getStatus() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *actor) updateStatus(f func(*Status)) {
	a.mu.Lock()
	f(&a.status)
	state := a.status.State
	a.mu.Unlock()
	a.m.Robots.SetAgentState(a.team, state)
}

func (a *actor) setState(state string) { a.updateStatus(func(s *Status) { s.State = state }) }

// interrupt cancels whatever the actor is doing right now.
func (a *actor) interrupt() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *actor) log(source, kind, text string, data any) {
	var raw json.RawMessage
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	a.m.Hub.Publish(a.team, live.LogEntry{Source: source, Kind: kind, Text: text, Data: raw})
}

func (a *actor) system(text string) { a.log(live.SourceSystem, "info", text, nil) }

func (a *actor) loop(ctx context.Context) {
	var cur *run
	var pending []Message
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	end := func(reason, by string) {
		if cur != nil {
			a.endRun(cur, reason, by)
			cur, pending = nil, nil
			timer.Stop()
		}
	}
	start := func() error {
		if a.conv != nil {
			a.endConversation("run_start")
			a.talkInfo("Ein Lauf hat begonnen, das Gespräch ist zu Ende. Im Lauf kennt euer Agent davon nur seine Notizen.")
		}
		r, err := a.startRun(ctx)
		if err != nil {
			return err
		}
		cur = r
		timer.Reset(time.Until(r.deadline))
		pending = []Message{{From: SourceSystem, Kind: "start", Text: "Der Lauf beginnt. Lies den Prompt deines Teams und leg los.", At: time.Now(), Wake: true}}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			end("server_stop", "")
			a.endConversation("server_stop")
			return
		case c := <-a.ctrl:
			var err error
			switch c.kind {
			case ctrlStart:
				if cur == nil {
					err = start()
				}
			case ctrlStop:
				end(c.reason, c.by)
			case ctrlRestart:
				end("restart", c.by)
				err = start()
			case ctrlTaskClosed:
				if cur != nil && cur.task.Task == c.task {
					end("task_closed", "Leitstand")
				}
				if a.conv != nil && a.conv.task.Task == c.task {
					a.endConversation("task_closed")
					a.talkInfo("Die Lab-Leitung hat die Aufgabe geschlossen. Das Gespräch ist zu Ende.")
				}
			case ctrlTaskSwitched:
				if a.conv != nil && a.conv.task.Task != c.task {
					a.endConversation("task_switched")
				}
			case ctrlRemove:
				end("team_deleted", "")
				a.endConversation("team_deleted")
				return
			case ctrlNewConversation:
				if a.conv != nil {
					a.endConversation("new")
					a.talkInfo("Neues Gespräch: Euer Agent kennt jetzt nur noch seine Notizen und den letzten Lauf.")
				}
			}
			if c.reply != nil {
				c.reply <- err
			}
		case msg := <-a.inbox:
			if cur != nil {
				pending = append(pending, msg)
				if msg.From == SourceRobot && msg.RobotID != "" {
					a.m.record(store.Event{Kind: store.EventRobotEvent, Actor: store.ActorRobot, Team: a.team, Task: cur.task.Task,
						Robot: msg.RobotID, Prompt: cur.promptID, Run: cur.id, Data: map[string]any{"kind": msg.Kind, "data": msg.Data}})
				}
			}
		case msg := <-a.talk:
			msgs := append([]Message{msg}, a.drainTalk()...)
			if cur != nil {
				// A run began after the team wrote: the run gets the message.
				for _, m := range msgs {
					a.log(live.SourceTeam, "chat", m.Text, nil)
				}
				a.talkInfo("Gerade läuft ein Lauf. Eure Nachricht ist an den Lauf gegangen.")
				pending = append(pending, msgs...)
			} else {
				a.converse(ctx, msgs)
			}
		case <-timer.C:
			end("time_limit", "")
		}
		if cur != nil && wakes(pending) {
			pending = a.process(ctx, cur, pending)
			if cur.finished.Load() || cur.limitReached != "" {
				reason := "finish"
				if cur.limitReached != "" {
					reason = cur.limitReached
				}
				end(reason, "")
			}
		}
	}
}

func wakes(ms []Message) bool {
	for _, m := range ms {
		if m.Wake {
			return true
		}
	}
	return false
}

func (a *actor) startRun(ctx context.Context) (*run, error) {
	task, err := a.m.activeTask(ctx, a.team)
	if err != nil {
		return nil, err
	}
	p, err := a.m.Store.GetPrompt(ctx, task, 0)
	if err != nil {
		return nil, &StartError{"Deployt zuerst einen Prompt, dann kann euer Agent loslegen."}
	}
	provider := ""
	if a.m.TeamProvider != nil {
		provider = a.m.TeamProvider(ctx, a.team)
	}
	provider = a.m.LLM.Resolve(provider)
	model := a.m.LLM.Model(provider)
	a.m.endManual(a.team, "run_start") // by-hand work before the run stays in its own row
	id, err := a.m.Store.CreateRun(ctx, task, p.ID, model)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	length := time.Duration(a.m.Budgets.MaxRunMinutes) * time.Minute
	if a.m.runLength > 0 {
		length = a.m.runLength
	}
	r := &run{id: id, task: task, promptID: p.ID, promptVersion: p.Version, provider: provider, started: now, deadline: now.Add(length)}
	a.updateStatus(func(s *Status) {
		*s = Status{State: StateWaiting, Running: true, RunID: id, Task: task.Task, Provider: provider, Model: model, StartedAt: now}
	})
	// Each run starts a new way: retrace_path must not drive back through
	// earlier runs. Robots that are offline keep their log until they get
	// a new team or a later run starts.
	for _, rb := range a.m.Robots.TeamRobots(a.team) {
		go func() {
			if _, err := a.m.Robots.Send(context.Background(), rb.ID, "clear_log", nil, 2*time.Second); err != nil {
				a.m.Log.Warn("clear trip log at run start", "robot", rb.ID, "err", err)
			}
		}()
	}
	a.log(live.SourceSystem, "run_start", "", map[string]any{"run": id, "version": p.Version, "model": model})
	a.m.record(store.Event{Kind: store.EventRunStarted, Actor: store.ActorTeam, Team: a.team, Task: task.Task, Prompt: p.ID, Run: id,
		Data: map[string]any{"model": model, "provider": provider}})
	a.m.Log.Info("run started", "team", a.team, "task", task.Task, "run", id, "prompt", p.Version, "model", model)
	return r, nil
}

func (a *actor) endRun(r *run, reason, by string) {
	ctx := context.Background()
	if err := a.m.Store.EndRun(ctx, r.id, reason, r.report); err != nil {
		a.m.Log.Error("end run", "run", r.id, "err", err)
	}
	scope := store.Event{Team: a.team, Task: r.task.Task, Prompt: r.promptID, Run: r.id}
	if reason == "finish" {
		e := scope
		e.Kind, e.Actor, e.Data = store.EventRunFinished, store.ActorAgent, map[string]string{"report": r.report}
		a.m.record(e)
	}
	e := scope
	e.Kind, e.Actor = store.EventRunEnded, endActor(reason, by)
	e.Data = map[string]any{"reason": reason, "tokens_in": r.tokensIn, "tokens_out": r.tokensOut, "cost_micro_eur": a.getStatus().CostMicroEUR}
	a.m.record(e)
	if reason == "stop" || reason == "restart" || reason == "team_deleted" || reason == "server_stop" || reason == "time_limit" || reason == "task_closed" {
		// Leave no robot driving on a stopped run. Tools were cancelled already.
		for _, rb := range a.m.Robots.TeamRobots(a.team) {
			go func() { _, _ = a.m.Robots.Send(ctx, rb.ID, "stop", nil, 2*time.Second) }()
		}
	}
	a.updateStatus(func(s *Status) { s.State, s.Running, s.Task = StateWaiting, false, "" })
	a.log(live.SourceSystem, "run_end", "", map[string]any{"run": r.id, "reason": reason, "by": by, "report": r.report})
	a.m.Log.Info("run ended", "team", a.team, "run", r.id, "reason", reason)
}

// process handles one wake-up: model calls and tool calls until the model is
// done, a limit is reached or the run is stopped. It returns the messages
// that arrived but were not used (none, normally).
func (a *actor) process(parent context.Context, r *run, pending []Message) []Message {
	// The deadline cuts a model call or running tools at the run's time limit.
	ctx, cancel := context.WithDeadline(parent, r.deadline)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
		a.setState(StateWaiting)
	}()

	a.addInbox(ctx, r, pending)
	env := a.env(ctx, r)
	env.MsgID = r.lastMsgID
	for step := 0; ; step++ {
		if step >= a.m.Budgets.MaxStepsPerWakeup {
			a.system(fmt.Sprintf("Euer Agent hat %d Schritte am Stück gemacht und pausiert. Eine neue Nachricht weckt ihn wieder.", step))
			return nil
		}
		if time.Now().After(r.deadline) {
			r.limitReached = "time_limit"
			return nil
		}
		a.setState(StateThinking)
		messages, err := a.buildContext(ctx, r)
		if err != nil {
			a.m.Log.Error("build context", "team", a.team, "err", err)
			a.system("Der Agent konnte seinen Kontext nicht laden.")
			return nil
		}
		env.Agents = names(a.m.runningAgents(ctx, a.team))
		resp, call, err := a.m.LLM.ChatWith(ctx, r.provider, llm.Request{Messages: messages, Tools: a.m.Tools.Definitions(env)})
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				r.limitReached = "time_limit"
				return nil
			}
			if !errors.Is(err, context.Canceled) {
				a.m.Log.Error("model call failed", "team", a.team, "err", err)
				a.system("Das Sprachmodell antwortet gerade nicht. Schickt eurem Agenten gleich noch einmal eine Nachricht.")
			}
			return nil
		}
		a.account(r, call)

		msg := resp.Message
		msg.Role = llm.RoleAssistant
		r.history = append(r.history, msg)
		if msg.Content != "" {
			a.store(ctx, r, "agent", "text", msg.Content, nil, store.RobotRef{})
			a.log(live.SourceAgent, "text", msg.Content, nil)
		}
		if limit := a.m.Budgets.MaxTokensPerRun; limit > 0 && r.tokensIn+r.tokensOut >= limit {
			a.closeToolCalls(r, msg.ToolCalls, "Der Lauf ist wegen des Token-Limits beendet.")
			r.limitReached = "token_limit"
			return nil
		}
		if len(msg.ToolCalls) == 0 {
			return nil
		}

		a.setState(StateActing)
		a.runTools(ctx, r, env, msg.ToolCalls)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			r.limitReached = "time_limit"
			return nil
		}
		if r.finished.Load() || ctx.Err() != nil {
			return nil
		}
		// Messages that arrived during the tools go into the next model call.
		if fresh := a.drainInbox(); len(fresh) > 0 {
			a.addInbox(ctx, r, fresh)
			env.MsgID = r.lastMsgID
		}
	}
}

func (a *actor) drainTalk() []Message {
	var out []Message
	for {
		select {
		case m := <-a.talk:
			out = append(out, m)
		default:
			return out
		}
	}
}

// runTools runs the tool calls of one model response. Calls for different
// robots run at the same time, so several robots can move at once. A call
// waits for every earlier call that acts on one of its robots; a call that
// acts on no robot (finish, wait_for, say, ...) waits for all earlier calls
// and holds up all later ones. Each call is logged when it ends; the history
// and the store get the results in call order, as the model sent them.
func (a *actor) runTools(ctx context.Context, r *run, env *tools.Env, calls []llm.ToolCall) {
	type outcome struct {
		ran    bool
		result json.RawMessage
		d      time.Duration
	}
	out := make([]outcome, len(calls))
	done := make([]chan struct{}, len(calls))
	scopes := make([]tools.Scope, len(calls))
	var wg sync.WaitGroup
	for i, tc := range calls {
		scopes[i] = a.m.Tools.Scope(env, tc.Function.Name, tc.Function.Arguments)
		done[i] = make(chan struct{})
		var after []chan struct{}
		for j := range i {
			if scopes[i].Overlaps(scopes[j]) {
				after = append(after, done[j])
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done[i])
			for _, ch := range after {
				<-ch // a cancelled call ends quickly, so this needs no select
			}
			if ctx.Err() != nil || r.finished.Load() {
				return
			}
			start := time.Now()
			result := a.m.Tools.Call(ctx, env, tc.Function.Name, tc.Function.Arguments)
			d := time.Since(start)
			out[i] = outcome{ran: true, result: result, d: d}
			a.log(live.SourceAgent, "tool", "", map[string]any{
				"name": tc.Function.Name, "args": json.RawMessage(validJSON(tc.Function.Arguments)), "result": result, "ms": d.Milliseconds(),
			})
		}()
	}
	wg.Wait()

	for i, tc := range calls {
		if !out[i].ran {
			a.closeToolCalls(r, calls[i:i+1], "Nicht ausgeführt: der Lauf ist beendet.")
			continue
		}
		r.history = append(r.history, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Name: tc.Function.Name, Content: string(out[i].result)})
		r.seq++
		if err := a.m.Store.AddToolCall(context.Background(), r.id, r.seq, tc.Function.Name, json.RawMessage(validJSON(tc.Function.Arguments)), out[i].result, out[i].d, scopes[i].Robot()); err != nil {
			a.m.Log.Error("store tool call", "err", err)
		}
	}
}

func (a *actor) drainInbox() []Message {
	var out []Message
	for {
		select {
		case m := <-a.inbox:
			out = append(out, m)
		default:
			return out
		}
	}
}

// closeToolCalls answers tool calls that will not run, so the history stays valid.
func (a *actor) closeToolCalls(r *run, calls []llm.ToolCall, why string) {
	for _, tc := range calls {
		res, _ := json.Marshal(map[string]any{"ok": false, "error": why})
		r.history = append(r.history, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Name: tc.Function.Name, Content: string(res)})
	}
}

func (a *actor) account(r *run, call llm.Call) {
	r.tokensIn += call.Usage.PromptTokens
	r.tokensOut += call.Usage.CompletionTokens
	a.updateStatus(func(s *Status) {
		s.TokensIn += call.Usage.PromptTokens
		s.TokensOut += call.Usage.CompletionTokens
		s.CostMicroEUR += call.CostMicroEUR
		s.Calls++
	})
	if err := a.m.Store.AddLLMCall(context.Background(), r.id, call.Provider, call.Model, call.Usage.PromptTokens, call.Usage.CompletionTokens, call.Duration, call.CostMicroEUR); err != nil {
		a.m.Log.Error("store llm call", "err", err)
	}
}

func (a *actor) store(ctx context.Context, r *run, source, kind, text string, data json.RawMessage, robot store.RobotRef) int64 {
	r.seq++
	id, err := a.m.Store.AddMessage(context.WithoutCancel(ctx), r.id, r.seq, source, kind, text, data, robot)
	if err != nil {
		a.m.Log.Error("store message", "err", err)
	}
	return id
}

func (a *actor) env(ctx context.Context, r *run) *tools.Env {
	team, _ := a.m.Store.Team(ctx, a.team)
	env := &tools.Env{TeamID: a.team, TaskID: r.task.Task, TeamName: team.Name, RunID: r.id}
	env.Finish = func(report string) {
		r.report = report
		r.finished.Store(true)
	}
	env.Say = func(to, text string) []string {
		return a.m.say(a.team, team.Name, to, text)
	}
	return env
}

func names(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	return out
}

func validJSON(s string) string {
	if json.Valid([]byte(s)) {
		return s
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// endActor says who ended a run, for the event log.
func endActor(reason, by string) string {
	switch reason {
	case "finish":
		return store.ActorAgent
	case "restart":
		return store.ActorTeam
	case "stop":
		if by == "Team" {
			return store.ActorTeam
		}
		return store.ActorAdmin
	case "task_closed", "team_deleted":
		return store.ActorAdmin
	}
	return store.ActorSystem
}

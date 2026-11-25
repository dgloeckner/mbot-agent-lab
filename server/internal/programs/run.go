package programs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Ends of a run.
const (
	EndDone    = "done"    // the program ran to its end or to "gib zurück"
	EndAborted = "aborted" // an unexpected end nobody handled, or a hard error
	EndLimit   = "limit"   // steps, time or rounds of "wiederhole bis" used up
	EndStopped = "stopped" // stopped by the team or the control room
)

// Run limits (start values; docs/programs.md).
const (
	MaxSteps  = 200
	MaxTime   = 120 * time.Second
	MaxRounds = 50
)

// Outcome is the answer of the caller for one tool call.
type Outcome struct {
	Result json.RawMessage
	// Normal: the registry counts the end as normal (a reason of the tool's
	// ends, or ok without a reason).
	Normal bool
	// Quiet: a display or sound tool; it does not replace the last result.
	Quiet bool
	// Hard: always an abort, whatever follows: robot offline or not
	// answering, reassigned, stopped, run budget used up.
	Hard bool
}

// Caller runs tools and reads sensors for the interpreter. It binds the
// robot, checks ownership and logs; the interpreter knows no tool names.
type Caller interface {
	Call(ctx context.Context, tool string, args map[string]any) Outcome
	// Sense returns a reading of the robot's state taken after the last
	// tool result: distance_cm, floor, color, line.
	Sense(ctx context.Context) (map[string]any, error)
}

// Event phases for the observer.
const (
	PhaseBefore = "before" // a step is about to run
	PhaseWait   = "wait"   // single step: waiting before a call until told to go on
	PhaseAfter  = "after"  // a step ran; Result is set
)

// Event is one moment of a run, for the team page's "Ablauf".
type Event struct {
	Phase  string          `json:"phase"`
	Step   int             `json:"step"`
	Block  string          `json:"block"`
	Kind   string          `json:"kind"` // call | sense
	Tool   string          `json:"tool,omitempty"`
	Args   map[string]any  `json:"args,omitempty"`
	Round  int             `json:"round,omitempty"` // round of the innermost loop, from 1
	Result json.RawMessage `json:"result,omitempty"`
	Ms     int64           `json:"ms,omitempty"`
}

// Options of a run. The zero value uses the limits above.
type Options struct {
	MaxSteps  int
	MaxTime   time.Duration
	MaxRounds int
	// Step makes the run halt before every call until a value arrives.
	Step <-chan struct{}
	// Describe names a block in the words of the editor for "at". Nil
	// uses Describe.
	Describe func(*Node) string
	// Observe hears every event. It must not block for long.
	Observe func(Event)
}

// Result is how a run ended. The agent gets it as the tool result.
type Result struct {
	OK       bool            `json:"ok"`
	End      string          `json:"end"`
	Steps    int             `json:"steps"`
	RuntimeS float64         `json:"runtime_s"`
	At       string          `json:"at,omitempty"`    // the block it ended at, in the editor's words
	Block    string          `json:"block,omitempty"` // that block's id
	Tool     string          `json:"tool,omitempty"`  // for an abort: the tool that ended unexpectedly
	Reason   string          `json:"reason,omitempty"`
	Error    string          `json:"error,omitempty"` // German sentence for an end that is not done
	Last     json.RawMessage `json:"last,omitempty"`  // the last tool's result only
	Value    any             `json:"value,omitempty"` // what "gib zurück" reported
}

var (
	errTimeLimit = errors.New("time limit")
	// errEnd unwinds the walk once the run has its end.
	errEnd = errors.New("end")
)

type runner struct {
	ctx    context.Context
	caller Caller
	opt    Options

	steps int
	start time.Time
	last  map[string]any // last result that counts (not quiet)
	raw   json.RawMessage
	round []int // rounds of the enclosing loops

	// pending is an unexpected end that the next thing evaluated must handle.
	pending *unexpected
	res     Result
}

type unexpected struct {
	node   *Node
	tool   string
	reason string
}

// Run walks a valid program. It returns when the program ends; the caller
// stops it by cancelling ctx (end stopped).
func Run(ctx context.Context, p *Program, caller Caller, opt Options) Result {
	if opt.MaxSteps == 0 {
		opt.MaxSteps = MaxSteps
	}
	if opt.MaxTime == 0 {
		opt.MaxTime = MaxTime
	}
	if opt.MaxRounds == 0 {
		opt.MaxRounds = MaxRounds
	}
	if opt.Describe == nil {
		opt.Describe = Describe
	}
	runCtx, cancel := context.WithTimeoutCause(ctx, opt.MaxTime, errTimeLimit)
	defer cancel()
	r := &runner{ctx: runCtx, caller: caller, opt: opt, start: time.Now()}
	err := r.block(p.Body)
	if err == nil {
		if r.pending != nil {
			r.abortPending()
		} else {
			r.res.End = EndDone
		}
	}
	r.res.OK = r.res.End == EndDone
	r.res.Steps = r.steps
	r.res.RuntimeS = math.Round(time.Since(r.start).Seconds()*10) / 10
	if len(r.raw) > 0 {
		r.res.Last = r.raw
	}
	return r.res
}

func (r *runner) at(n *Node) {
	if n != nil {
		r.res.At, r.res.Block = r.opt.Describe(n), n.ID
	}
}

func (r *runner) end(end string, n *Node, msg string) error {
	r.res.End = end
	r.at(n)
	r.res.Error = msg
	return errEnd
}

// ctxEnd turns a finished context into the run's end.
func (r *runner) ctxEnd(n *Node) error {
	switch {
	case errors.Is(context.Cause(r.ctx), errTimeLimit):
		return r.end(EndLimit, n, fmt.Sprintf("Die Zeit ist um: höchstens %d Sekunden.", int(r.opt.MaxTime.Seconds())))
	case errors.Is(r.ctx.Err(), context.DeadlineExceeded):
		return r.end(EndAborted, n, "Die Zeit des Laufs ist um.")
	}
	return r.end(EndStopped, n, "Angehalten.")
}

func (r *runner) abortPending() error {
	u := r.pending
	r.res.Tool, r.res.Reason = u.tool, u.reason
	return r.end(EndAborted, u.node, fmt.Sprintf("%s endete unerwartet mit %s, und kein „wenn“ danach kümmert sich darum.", u.tool, u.reason))
}

// handles reports whether an if handles the pending unexpected end: it tests
// the last result and names that reason, or it has an else.
func (r *runner) handles(n *Node) bool {
	if n.Type != TypeIf || n.Cond == nil || n.Cond.Source != SourceResult {
		return false
	}
	if n.HasElse() {
		return true
	}
	return n.Cond.Field == "reason" && fmt.Sprint(n.Cond.Value) == r.pending.reason
}

func (r *runner) block(nodes []*Node) error {
	for _, n := range nodes {
		if r.pending != nil {
			if !r.handles(n) {
				return r.abortPending()
			}
			r.pending = nil
		}
		if err := r.node(n); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) node(n *Node) error {
	if r.ctx.Err() != nil {
		return r.ctxEnd(n)
	}
	switch n.Type {
	case TypeCall:
		return r.call(n)
	case TypeRepeat:
		for i := 1; i <= n.Times; i++ {
			if err := r.loop(i, n.Body); err != nil {
				return err
			}
		}
	case TypeRepeatUntil:
		for i := 1; ; i++ {
			if i > r.opt.MaxRounds {
				return r.end(EndLimit, n, fmt.Sprintf("Wiederhole bis: nach %d Runden war die Bedingung noch nicht erfüllt.", r.opt.MaxRounds))
			}
			if err := r.loop(i, n.Body); err != nil {
				return err
			}
			if r.pending != nil {
				return r.abortPending() // a loop condition never handles an unexpected end
			}
			ok, err := r.cond(n.Cond)
			if err != nil {
				return err
			}
			if ok {
				break
			}
		}
	case TypeIf:
		ok, err := r.cond(n.Cond)
		if err != nil {
			return err
		}
		if ok {
			return r.block(n.Body)
		}
		return r.block(n.Else)
	case TypeReturn:
		r.res.End = EndDone
		r.at(n)
		if n.Return.Field != "" {
			r.res.Value = r.last[n.Return.Field]
		} else {
			r.res.Value = n.Return.Text
		}
		return errEnd
	}
	return nil
}

func (r *runner) loop(round int, body []*Node) error {
	r.round = append(r.round, round)
	defer func() { r.round = r.round[:len(r.round)-1] }()
	return r.block(body)
}

func (r *runner) curRound() int {
	if len(r.round) == 0 {
		return 0
	}
	return r.round[len(r.round)-1]
}

// step counts one step (a call or a sensor reading) against the limit.
func (r *runner) step(n *Node) error {
	if r.steps >= r.opt.MaxSteps {
		return r.end(EndLimit, n, fmt.Sprintf("Zu viele Schritte: höchstens %d.", r.opt.MaxSteps))
	}
	r.steps++
	return nil
}

func (r *runner) observe(e Event) {
	if r.opt.Observe != nil {
		r.opt.Observe(e)
	}
}

func (r *runner) call(n *Node) error {
	if err := r.step(n); err != nil {
		return err
	}
	ev := Event{Step: r.steps, Block: n.ID, Kind: "call", Tool: n.Tool, Args: n.Args, Round: r.curRound()}
	if r.opt.Step != nil {
		ev.Phase = PhaseWait
		r.observe(ev)
		select {
		case <-r.opt.Step:
		case <-r.ctx.Done():
			return r.ctxEnd(n)
		}
	}
	ev.Phase = PhaseBefore
	r.observe(ev)
	start := time.Now()
	args := n.Args
	if args == nil {
		args = map[string]any{}
	}
	out := r.caller.Call(r.ctx, n.Tool, args)
	ev.Phase, ev.Result, ev.Ms = PhaseAfter, out.Result, time.Since(start).Milliseconds()
	r.observe(ev)
	if r.ctx.Err() != nil {
		return r.ctxEnd(n)
	}
	var res map[string]any
	_ = json.Unmarshal(out.Result, &res)
	reason, _ := res["reason"].(string)
	if !out.Quiet {
		r.last, r.raw = res, out.Result
	}
	switch {
	case out.Hard:
		r.res.Tool, r.res.Reason = n.Tool, reason
		msg := fmt.Sprintf("%s ging nicht: %s.", n.Tool, reason)
		if e, ok := res["error"].(string); ok && e != "" {
			msg = fmt.Sprintf("%s ging nicht: %s", n.Tool, e)
		}
		if reason == EndStopped {
			return r.end(EndStopped, n, "Angehalten.")
		}
		return r.end(EndAborted, n, msg)
	case !out.Normal:
		if reason == "" {
			reason = "error"
		}
		r.pending = &unexpected{node: n, tool: n.Tool, reason: reason}
	}
	return nil
}

// cond evaluates a condition. A sensor condition reads once and counts as a step.
func (r *runner) cond(c *Cond) (bool, error) {
	var val any
	switch c.Source {
	case SourceResult:
		val = r.last[c.Field]
	case SourceSensor:
		n := &Node{ID: c.ID, Type: "sense"}
		if err := r.step(n); err != nil {
			return false, err
		}
		ev := Event{Phase: PhaseBefore, Step: r.steps, Block: c.ID, Kind: "sense", Round: r.curRound()}
		r.observe(ev)
		start := time.Now()
		state, err := r.caller.Sense(r.ctx)
		if r.ctx.Err() != nil {
			return false, r.ctxEnd(n)
		}
		if err != nil {
			return false, r.end(EndAborted, n, "Die Sensoren antworten nicht: "+err.Error())
		}
		ev.Phase, ev.Ms = PhaseAfter, time.Since(start).Milliseconds()
		ev.Result, _ = json.Marshal(state)
		r.observe(ev)
		val = state[c.Field]
	}
	ok := compare(val, c.Op, c.Value)
	if c.Not {
		ok = !ok
	}
	return ok, nil
}

// compare applies one operator. A missing value never matches; lt and gt
// compare numbers; contains looks for letters in a floor pattern or trace.
func compare(have any, op string, want any) bool {
	if have == nil {
		return false
	}
	switch op {
	case OpEq, OpNe:
		eq := false
		if a, ok := toNumber(have); ok {
			if b, ok := toNumber(want); ok {
				eq = a == b
			}
		} else {
			eq = fmt.Sprint(have) == fmt.Sprint(want)
		}
		return eq == (op == OpEq)
	case OpLt, OpGt:
		a, ok1 := toNumber(have)
		b, ok2 := toNumber(want)
		if !ok1 || !ok2 {
			return false
		}
		if op == OpLt {
			return a < b
		}
		return a > b
	case OpContains:
		w := fmt.Sprint(want)
		switch h := have.(type) {
		case string:
			return strings.Contains(h, w)
		case []any: // a floor trace: [[pos, pattern], …]
			for _, e := range h {
				if pair, ok := e.([]any); ok && len(pair) == 2 && strings.Contains(fmt.Sprint(pair[1]), w) {
					return true
				}
			}
		}
	}
	return false
}

func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.Replace(n, ",", ".", 1), 64)
		return f, err == nil
	}
	return 0, false
}

// Package tools holds the tool registry (name, JSON schema, handler) and the
// tool implementations. Robot tools become MQTT commands via the robots
// registry; the others run in the server. See docs/tools.md.
//
// Descriptions for the model are German, like the team prompts they sit next to.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/llm"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// AllRobots is the robot name that addresses every robot of the team (stop only).
const AllRobots = "alle"

// Env is what a tool call needs to know about the calling agent.
type Env struct {
	TeamID   string
	TeamName string
	RunID    int64
	MsgID    int64 // newest inbox message: provenance for notes

	// Agents lists the names of other agents that can receive say.
	Agents []string
	// Say delivers a message to another agent ("alle" for everyone) and
	// returns the names it reached. Set by the agent runtime.
	Say func(to, text string) []string
	// Finish ends the run with a report. Set by the agent runtime.
	Finish func(report string)
}

// robot parameter modes.
const (
	robotNone = iota // tool does not act on a robot
	robotOne         // acts on exactly one robot
	robotAll         // acts on one robot or on "alle"
)

type tool struct {
	name     string
	desc     string
	props    func(env *Env) map[string]any
	required []string
	robot    int
	run      func(ctx context.Context, c *call) any
}

// call carries one invocation.
type call struct {
	r      *Registry
	env    *Env
	args   map[string]any
	robots []robots.Robot // the targets, resolved from the robot parameter
	team   []robots.Robot // all robots of the team
}

// Registry is safe for concurrent use.
type Registry struct {
	Robots  *robots.Registry
	Store   *store.Store
	Hub     *live.Hub
	Budgets config.Budgets

	tools map[string]*tool
	order []string

	mu   sync.Mutex
	says map[string][]time.Time // team -> recent say calls
}

// NewRegistry creates the registry with all ready tools.
func NewRegistry(rb *robots.Registry, st *store.Store, hub *live.Hub, budgets config.Budgets) *Registry {
	r := &Registry{Robots: rb, Store: st, Hub: hub, Budgets: budgets, tools: map[string]*tool{}, says: map[string][]time.Time{}}
	for _, t := range builtins() {
		r.tools[t.name] = t
		r.order = append(r.order, t.name)
	}
	return r
}

// Names returns the tool names in catalog order.
func (r *Registry) Names() []string { return append([]string(nil), r.order...) }

// Definitions builds the tool list for one agent. With several robots, robot
// tools get a required "robot" parameter listing the team's robot names.
func (r *Registry) Definitions(env *Env) []llm.Tool {
	team := r.Robots.TeamRobots(env.TeamID)
	var names []string
	for _, rb := range team {
		names = append(names, rb.Alias)
	}
	out := make([]llm.Tool, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		props := map[string]any{}
		if t.props != nil {
			props = t.props(env)
		}
		required := append([]string(nil), t.required...)
		if t.robot != robotNone && len(team) > 1 {
			enum := names
			desc := "Welcher Roboter."
			if t.robot == robotAll {
				enum = append(append([]string(nil), names...), AllRobots)
				desc = "Welcher Roboter, oder \"alle\"."
			}
			props["robot"] = map[string]any{"type": "string", "enum": enum, "description": desc}
			required = append(required, "robot")
		}
		schema := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}
		raw, _ := json.Marshal(schema)
		out = append(out, llm.Tool{Name: name, Description: t.desc, Parameters: raw})
	}
	return out
}

// Call runs a tool and returns its JSON result. Mistakes by the model (unknown
// tool, bad arguments, missing robot) come back as results with "ok": false,
// so the agent can correct itself; they are not Go errors.
func (r *Registry) Call(ctx context.Context, env *Env, name string, rawArgs string) json.RawMessage {
	res := r.call(ctx, env, name, rawArgs)
	b, err := json.Marshal(res)
	if err != nil {
		b, _ = json.Marshal(fail("Ergebnis konnte nicht gelesen werden."))
	}
	return b
}

func (r *Registry) call(ctx context.Context, env *Env, name, rawArgs string) any {
	t, ok := r.tools[name]
	if !ok {
		return fail("Das Werkzeug %q gibt es nicht. Verfügbar: %s.", name, strings.Join(r.order, ", "))
	}
	args := map[string]any{}
	if strings.TrimSpace(rawArgs) != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return fail("Die Argumente sind kein gültiges JSON-Objekt.")
		}
	}
	for _, req := range t.required {
		if _, ok := args[req]; !ok {
			return fail("Es fehlt das Argument %q.", req)
		}
	}
	c := &call{r: r, env: env, args: args, team: r.Robots.TeamRobots(env.TeamID)}
	if t.robot != robotNone {
		targets, err := c.resolveRobots(t.robot == robotAll)
		if err != nil {
			return fail("%s", err.Error())
		}
		c.robots = targets
	}
	return t.run(ctx, c)
}

func (c *call) resolveRobots(allowAll bool) ([]robots.Robot, error) {
	switch {
	case len(c.team) == 0:
		return nil, errors.New("Eurem Team ist gerade kein Roboter zugeteilt.")
	case len(c.team) == 1:
		return c.team, nil
	}
	want, _ := c.args["robot"].(string)
	if want == "" {
		return nil, fmt.Errorf("Gib mit robot an, welcher Roboter gemeint ist: %s.", aliases(c.team))
	}
	if allowAll && strings.EqualFold(want, AllRobots) {
		return c.team, nil
	}
	for _, rb := range c.team {
		if strings.EqualFold(rb.Alias, want) {
			return []robots.Robot{rb}, nil
		}
	}
	return nil, fmt.Errorf("Einen Roboter %q hat euer Team nicht. Es gibt: %s.", want, aliases(c.team))
}

func aliases(rs []robots.Robot) string {
	var names []string
	for _, rb := range rs {
		names = append(names, rb.Alias)
	}
	return strings.Join(names, ", ")
}

// --- helpers for results and arguments --------------------------------------

func fail(format string, args ...any) map[string]any {
	return map[string]any{"ok": false, "error": fmt.Sprintf(format, args...)}
}

func (c *call) num(key string, def float64) float64 {
	if v, ok := c.args[key].(float64); ok {
		return v
	}
	return def
}

func (c *call) str(key string) string {
	v, _ := c.args[key].(string)
	return strings.TrimSpace(v)
}

func clampRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// command sends a skill to one robot and turns the result into a tool result.
func (c *call) command(ctx context.Context, rb robots.Robot, name string, args map[string]any, timeout time.Duration) map[string]any {
	res, err := c.r.Robots.Send(ctx, rb.ID, name, args, timeout)
	out := map[string]any{"robot": rb.Alias}
	switch {
	case errors.Is(err, robots.ErrOffline):
		out["ok"], out["reason"], out["error"] = false, "offline", rb.Alias+" ist nicht verbunden."
		return out
	case errors.Is(err, robots.ErrTimeout):
		out["ok"], out["reason"], out["error"] = false, "timeout", rb.Alias+" hat nicht rechtzeitig geantwortet."
		return out
	case err != nil:
		out["ok"], out["error"] = false, err.Error()
		return out
	}
	out["ok"], out["reason"] = res.OK, string(res.Reason)
	if res.Error != "" {
		out["error"] = res.Error
	}
	if len(res.Data) > 0 {
		var data map[string]any
		if json.Unmarshal(res.Data, &data) == nil {
			for k, v := range data {
				out[k] = v
			}
		}
	}
	return out
}

func robotState(rb robots.Robot) map[string]any {
	s := rb.State
	return map[string]any{
		"robot": rb.Alias, "online": rb.Online && rb.Accepted, "distance_cm": s.DistanceCM, "floor": s.Floor, "color": s.Color,
		"line": s.Line, "yaw": s.Yaw, "battery": s.Battery, "tilted": s.Tilted, "skill": s.Skill,
	}
}

// allowSay enforces max_agent_messages_per_minute.
func (r *Registry) allowSay(team string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	recent := r.says[team][:0]
	for _, t := range r.says[team] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= r.Budgets.MaxAgentMessagesPerMinute {
		r.says[team] = recent
		return false
	}
	r.says[team] = append(recent, now)
	return true
}

// pick copies the given arguments, if present, into a command's args.
func (c *call) pick(keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := c.args[k]; ok {
			out[k] = v
		}
	}
	return out
}

// motionTimeout is the robot's limit per motion command plus two seconds.
const motionTimeout = 22 * time.Second

var floorColors = []string{"white", "black", "red", "yellow", "green", "blue", "any"}

// floorHelp explains the floor trace once, in the drive tool; the others refer to it.
const floorHelp = "floor ist die Bodenspur: eine Liste aus [Position, Muster]. Die Position zählt ab Beginn der Bewegung, in cm beim Fahren, in Grad beim Drehen. Das Muster sind vier Buchstaben für die vier Bodensonden von links nach rechts: w weiß, k schwarz, r rot, y gelb, g grün, b blau, ? unklar. Ein Eintrag gilt bis zum nächsten. Höchstens 20 Einträge; steht floor_truncated true dabei, fahre kürzere Strecken."

var ledColors = []string{"red", "green", "blue", "yellow", "purple", "cyan", "white", "orange", "off"}
var robotEvents = []string{"obstacle_near", "obstacle_clear", "color_changed", "tilted", "button_a", "button_b", "clap", "battery_low"}

const maxNotes = 20

func static(props map[string]any) func(*Env) map[string]any {
	return func(*Env) map[string]any {
		out := make(map[string]any, len(props))
		for k, v := range props {
			out[k] = v
		}
		return out
	}
}

func builtins() []*tool {
	return []*tool{
		{
			name:     "drive",
			desc:     "Fährt geradeaus um cm Zentimeter, negative Werte fahren rückwärts. Hält von selbst bei einem Hindernis an. Ergebnis: driven_cm, reason (done, obstacle, stopped, tilted, timeout), floor und min_distance_cm. " + floorHelp,
			props:    static(map[string]any{"cm": map[string]any{"type": "number", "minimum": -300, "maximum": 300}, "speed": map[string]any{"type": "number", "minimum": 5, "maximum": 80, "description": "Umdrehungen pro Minute, Standard 40."}}),
			required: []string{"cm"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				args := map[string]any{"cm": c.num("cm", 0)}
				if v, ok := c.args["speed"]; ok {
					args["speed"] = v
				}
				return c.command(ctx, c.robots[0], "drive", args, motionTimeout)
			},
		},
		{
			name:     "turn",
			desc:     "Dreht auf der Stelle um degrees Grad. Positiv dreht nach rechts, negativ nach links. Ergebnis: turned_deg, reason und floor (wie bei drive, Position in Grad).",
			props:    static(map[string]any{"degrees": map[string]any{"type": "number", "minimum": -720, "maximum": 720}}),
			required: []string{"degrees"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "turn", map[string]any{"deg": c.num("degrees", 0)}, motionTimeout)
			},
		},
		{
			name: "drive_until",
			desc: "Fährt geradeaus, bis eine Wand näher als stop_below_cm ist (reason wall), die Bodenfarbe stop_on_color erscheint (reason color) oder max_cm gefahren sind (reason max_cm). Die Farbe zählt, sobald eine der vier Bodensonden sie sieht; any ist jede Farbe außer Weiß. Der Roboter prüft das selbst mit geglätteten Messwerten. Ergebnis: driven_cm, reason, distance_cm, color, floor (wie bei drive) und min_distance_cm.",
			props: static(map[string]any{
				"max_cm":        map[string]any{"type": "number", "minimum": 1, "maximum": 300},
				"stop_below_cm": map[string]any{"type": "number", "minimum": 3, "maximum": 100},
				"stop_on_color": map[string]any{"type": "string", "enum": floorColors},
				"speed":         map[string]any{"type": "number", "minimum": 5, "maximum": 80},
			}),
			required: []string{"max_cm"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				args := map[string]any{"max_cm": c.num("max_cm", 0)}
				for _, k := range []string{"stop_below_cm", "stop_on_color", "speed"} {
					if v, ok := c.args[k]; ok {
						args[k] = v
					}
				}
				return c.command(ctx, c.robots[0], "drive_until", args, motionTimeout)
			},
		},
		{
			name: "turn_until_clear",
			desc: "Dreht auf der Stelle in direction (left oder right), mindestens min_deg, und weiter, bis vorne mindestens min_cm frei sind (reason clear), höchstens max_deg (reason max_deg). Der Roboter entscheidet das selbst mit geglätteten Messwerten. Ergebnis: turned_deg (positiv ist rechts), distance_cm und floor (wie bei drive, Position in Grad).",
			props: static(map[string]any{
				"direction": map[string]any{"type": "string", "enum": []string{"left", "right"}},
				"min_cm":    map[string]any{"type": "number", "minimum": 10, "maximum": 200, "description": "Freier Weg nach vorn, Standard 40."},
				"min_deg":   map[string]any{"type": "number", "minimum": 0, "maximum": 180, "description": "Mindestens so weit drehen, Standard 20."},
				"max_deg":   map[string]any{"type": "number", "minimum": 10, "maximum": 720, "description": "Höchstens so weit drehen, Standard 360."},
			}),
			required: []string{"direction"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "turn_until_clear", c.pick("direction", "min_cm", "min_deg", "max_deg"), motionTimeout)
			},
		},
		{
			name: "scan",
			desc: "Schaut sich um: dreht von from_deg bis to_deg (relativ zur aktuellen Richtung, positiv ist rechts), misst dabei in Schritten von step_deg und dreht sich danach in die freieste Richtung (face best) oder zurück (face start). Ergebnis: profile als Liste [Winkel, cm], best_deg, best_cm, turned_deg.",
			props: static(map[string]any{
				"from_deg": map[string]any{"type": "number", "minimum": -180, "maximum": 180, "description": "Standard -90."},
				"to_deg":   map[string]any{"type": "number", "minimum": -180, "maximum": 180, "description": "Standard 90."},
				"step_deg": map[string]any{"type": "number", "minimum": 10, "maximum": 45, "description": "Standard 15."},
				"face":     map[string]any{"type": "string", "enum": []string{"best", "start"}},
			}),
			robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "scan", c.pick("from_deg", "to_deg", "step_deg", "face"), motionTimeout)
			},
		},
		{
			name: "follow_line_until",
			desc: "Folgt einer Linie auf dem Boden und lenkt dabei selbst. Endet an der Farbe stop_on_color (reason color), an einer Kreuzung (reason crossing, abschaltbar mit stop_at_crossing false), wenn die Linie aufhört (reason line_lost) oder nach max_cm (reason max_cm). Ergebnis: driven_cm, reason, color und floor (wie bei drive).",
			props: static(map[string]any{
				"max_cm":           map[string]any{"type": "number", "minimum": 1, "maximum": 300},
				"stop_on_color":    map[string]any{"type": "string", "enum": floorColors},
				"stop_at_crossing": map[string]any{"type": "boolean", "description": "Standard true."},
				"speed":            map[string]any{"type": "number", "minimum": 10, "maximum": 60, "description": "Standard 30."},
			}),
			required: []string{"max_cm"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "follow_line_until", c.pick("max_cm", "stop_on_color", "stop_at_crossing", "speed"), motionTimeout)
			},
		},
		{
			name:  "retrace_path",
			desc:  "Fährt den ganzen gespeicherten Weg zurück bis zu seinem Anfang, nicht nur das letzte Stück: dreht sich um und fährt die gespeicherten Strecken und Drehungen in umgekehrter Reihenfolge ab, vorwärts, damit er Hindernisse sieht. Der Weg zählt seit dem letzten retrace_path oder seit der Roboter euch gehört. Der Roboter fährt dabei gerade Strecken und folgt keiner Linie. Am Ende steht er am Anfang und schaut in die Richtung, aus der er gekommen ist. Ergebnis: segments (abgefahrene Wegstücke), distance_cm und reason: done, empty (kein Weg gespeichert), obstacle, stopped, tilted oder timeout (nach 20 Sekunden). Bei einem Abbruch bleibt der Rest gespeichert, ein neuer Aufruf fährt von dort weiter.",
			robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "retrace_path", nil, motionTimeout)
			},
		},
		{
			name: "stop", desc: "Hält sofort an. Mit robot \"alle\" halten alle eure Roboter an.",
			robot: robotAll,
			run: func(ctx context.Context, c *call) any {
				var results []any
				ok := true
				for _, rb := range c.robots {
					res := c.command(ctx, rb, "stop", nil, 3*time.Second)
					ok = ok && res["ok"] == true
					results = append(results, res)
				}
				if len(results) == 1 {
					return results[0]
				}
				return map[string]any{"ok": ok, "robots": results}
			},
		},
		{
			name: "read_sensors", desc: "Liest den aktuellen Zustand: distance_cm (Abstand nach vorn), floor (Muster unter den vier Bodensonden von links nach rechts, Buchstaben wie bei drive), color (Boden), line, yaw (Richtung in Grad), battery, tilted, skill.",
			robot: robotOne,
			run: func(_ context.Context, c *call) any {
				st := robotState(c.robots[0])
				st["ok"] = true
				return st
			},
		},
		{
			name: "list_robots", desc: "Listet alle Roboter eures Teams mit Name, Verbindung, Akku und Zustand.",
			run: func(_ context.Context, c *call) any {
				var list []any
				for _, rb := range c.team {
					list = append(list, robotState(rb))
				}
				return map[string]any{"ok": true, "robots": list}
			},
		},
		{
			name: "wait_for",
			desc: "Wartet, bis einer eurer Roboter das Ereignis event meldet, höchstens timeout_s Sekunden. Ergebnis: event, robot, data oder reason timeout.",
			props: static(map[string]any{
				"event":     map[string]any{"type": "string", "enum": robotEvents},
				"timeout_s": map[string]any{"type": "number", "minimum": 1, "maximum": 60},
			}),
			required: []string{"event", "timeout_s"},
			run: func(ctx context.Context, c *call) any {
				kind := c.str("event")
				timeout := time.Duration(max(1, min(60, c.num("timeout_s", 10)))) * time.Second
				ch := make(chan robots.Event, 16)
				c.r.Robots.Subscribe(ch)
				defer c.r.Robots.Unsubscribe(ch)
				timer := time.NewTimer(timeout)
				defer timer.Stop()
				for {
					select {
					case ev := <-ch:
						if ev.Event.Kind != kind {
							continue
						}
						for _, rb := range c.team {
							if rb.ID == ev.RobotID {
								var data any
								_ = json.Unmarshal(ev.Event.Data, &data)
								return map[string]any{"ok": true, "event": kind, "robot": rb.Alias, "data": data}
							}
						}
					case <-timer.C:
						return map[string]any{"ok": false, "reason": "timeout"}
					case <-ctx.Done():
						return map[string]any{"ok": false, "reason": "stopped"}
					}
				}
			},
		},
		{
			name:     "set_leds",
			desc:     "Schaltet die Lichter des Roboters in einer Farbe.",
			props:    static(map[string]any{"color": map[string]any{"type": "string", "enum": ledColors}}),
			required: []string{"color"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "set_leds", map[string]any{"color": c.str("color")}, 3*time.Second)
			},
		},
		{
			name:     "show_text",
			desc:     "Zeigt einen kurzen Text (höchstens 60 Zeichen) auf dem Display des Roboters.",
			props:    static(map[string]any{"text": map[string]any{"type": "string", "maxLength": 60}}),
			required: []string{"text"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "show_text", map[string]any{"text": clampRunes(c.str("text"), 60)}, 3*time.Second)
			},
		},
		{
			name:     "play_sound",
			desc:     "Spielt einen kurzen Ton.",
			props:    static(map[string]any{"name": map[string]any{"type": "string", "enum": []string{"hello", "yeah", "wow", "laugh", "sad", "surprised", "beeps"}}}),
			required: []string{"name"}, robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "play_sound", map[string]any{"name": c.str("name")}, 3*time.Second)
			},
		},
		{
			name: "identify", desc: "Lässt den Roboter einige Sekunden blinken und piepen, damit Menschen ihn finden.",
			robot: robotOne,
			run: func(ctx context.Context, c *call) any {
				return c.command(ctx, c.robots[0], "identify", nil, 3*time.Second)
			},
		},
		{
			name: "say",
			desc: "Sagt einem anderen Agenten (oder \"alle\") etwas. Der Text erscheint auch auf dem Display eures Roboters. Höchstens 60 Zeichen.",
			props: func(env *Env) map[string]any {
				to := append(append([]string(nil), env.Agents...), AllRobots)
				sort.Strings(to[:len(to)-1])
				return map[string]any{
					"to":      map[string]any{"type": "string", "enum": to, "description": "Name des anderen Teams oder \"alle\"."},
					"text":    map[string]any{"type": "string", "maxLength": 60},
					"emotion": map[string]any{"type": "string", "enum": []string{"freundlich", "aufgeregt", "traurig", "wütend"}},
				}
			},
			required: []string{"to", "text"},
			run: func(ctx context.Context, c *call) any {
				if c.env.Say == nil {
					return fail("Reden ist gerade nicht möglich.")
				}
				if !c.r.allowSay(c.env.TeamID) {
					return fail("Zu viele Nachrichten: höchstens %d pro Minute. Warte etwas.", c.r.Budgets.MaxAgentMessagesPerMinute)
				}
				text := clampRunes(c.str("text"), 60)
				for _, rb := range c.team {
					_ = c.command(ctx, rb, "show_text", map[string]any{"text": text}, 2*time.Second)
					break // the first robot is the speaker
				}
				delivered := c.env.Say(c.str("to"), text)
				return map[string]any{"ok": len(delivered) > 0, "delivered": delivered}
			},
		},
		{
			name:     "tell_team",
			desc:     "Schreibt eurem Team eine Nachricht ins Protokoll.",
			props:    static(map[string]any{"text": map[string]any{"type": "string", "maxLength": 300}}),
			required: []string{"text"},
			run: func(_ context.Context, c *call) any {
				c.r.Hub.Publish(c.env.TeamID, live.LogEntry{Source: live.SourceAgent, Kind: "tell_team", Text: clampRunes(c.str("text"), 300)})
				return map[string]any{"ok": true}
			},
		},
		{
			name:     "remember",
			desc:     "Legt eine Notiz an. Notizen siehst du in jeder Runde, auch in späteren Läufen. Höchstens 200 Zeichen.",
			props:    static(map[string]any{"text": map[string]any{"type": "string", "maxLength": 200}}),
			required: []string{"text"},
			run: func(ctx context.Context, c *call) any {
				existing, err := c.r.Store.ListNotes(ctx, c.env.TeamID)
				if err != nil {
					return fail("Notizen sind gerade nicht erreichbar.")
				}
				if len(existing) >= maxNotes {
					return fail("Es gibt schon %d Notizen. Lösche zuerst eine mit forget.", maxNotes)
				}
				n, err := c.r.Store.AddNoteFrom(ctx, c.env.TeamID, clampRunes(c.str("text"), 200), "agent", c.env.RunID, c.env.MsgID)
				if err != nil {
					return fail("Die Notiz konnte nicht gespeichert werden.")
				}
				return map[string]any{"ok": true, "note_id": n.ID}
			},
		},
		{
			name:     "forget",
			desc:     "Löscht eine Notiz anhand ihrer Nummer.",
			props:    static(map[string]any{"note_id": map[string]any{"type": "integer"}}),
			required: []string{"note_id"},
			run: func(ctx context.Context, c *call) any {
				if err := c.r.Store.DeleteNote(ctx, c.env.TeamID, int64(c.num("note_id", 0))); err != nil {
					return fail("Diese Notiz gibt es nicht.")
				}
				return map[string]any{"ok": true}
			},
		},
		{
			name:     "finish",
			desc:     "Beendet den Lauf. report ist dein ehrlicher Bericht an das Team: was du getan hast und ob die Aufgabe gelöst ist.",
			props:    static(map[string]any{"report": map[string]any{"type": "string", "maxLength": 1000}}),
			required: []string{"report"},
			run: func(_ context.Context, c *call) any {
				if c.env.Finish != nil {
					c.env.Finish(clampRunes(c.str("report"), 1000))
				}
				return map[string]any{"ok": true}
			},
		},
	}
}

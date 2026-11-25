package programs

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// Limits of a team tool (start values; docs/programs.md).
const (
	MaxBlocks      = 60  // blocks per team tool, conditions included
	MaxDepth       = 4   // repeat, repeat_until and if inside each other
	MaxRepeat      = 20  // wiederhole n mal: n from 1 to 20
	MaxDescription = 200 // characters
	MaxReturnText  = 200 // characters of a fixed "gib zurück" text
	MaxPerTask     = 12  // team tools per (team, task)
)

var nameRule = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)

// Fields a condition may compare. Sensor fields come from the robot's state.
var (
	resultFields = map[string]bool{
		"reason": true, "driven_cm": true, "turned_deg": true, "distance_cm": true, "min_distance_cm": true,
		"color": true, "floor": true, "best_deg": true, "best_cm": true, "event": true, "ok": true,
	}
	sensorFields = map[string]bool{"distance_cm": true, "floor": true, "color": true, "line": true}
	ops          = map[string]bool{OpEq: true, OpNe: true, OpLt: true, OpGt: true, OpContains: true}
)

// Catalog is what validation needs from the tool registry.
type Catalog interface {
	// Known: a built-in tool (or a planned one) of this name exists.
	Known(name string) bool
	// Manual: a person or a team tool may run it.
	Manual(name string) bool
	// CheckArgs checks fixed block arguments against the tool's schema.
	CheckArgs(name string, args map[string]any) error
}

// Problem is one reason why a team tool cannot run, at the block it is
// about (empty for the name and the description). Text is German.
type Problem struct {
	Block string `json:"block,omitempty"`
	Field string `json:"field,omitempty"` // FieldName or FieldDescription: the input the problem is about
	Text  string `json:"text"`
}

// Fields of a team tool besides its blocks that a problem can be about.
const (
	FieldName        = "name"
	FieldDescription = "description"
)

// Validate checks name, description and tree. An invalid program can be
// stored but not run.
func Validate(p *Program, cat Catalog) []Problem {
	var out []Problem
	add := func(block, format string, args ...any) {
		out = append(out, Problem{Block: block, Text: fmt.Sprintf(format, args...)})
	}
	out = append(out, ValidateName(p.Name, cat)...)
	if n := utf8.RuneCountInString(p.Description); n > MaxDescription {
		out = append(out, Problem{Field: FieldDescription, Text: fmt.Sprintf("Die Beschreibung hat %d Zeichen, erlaubt sind %d.", n, MaxDescription)})
	}
	if len(p.Body) == 0 {
		add("", "Das Werkzeug ist leer. Zieht Blöcke hinein.")
		return out
	}

	blocks := 0
	seen := map[string]bool{}
	id := func(block string) {
		if block == "" {
			add("", "Ein Block hat keine Kennung.")
		} else if seen[block] {
			add(block, "Die Kennung %q gibt es zweimal.", block)
		}
		seen[block] = true
	}
	var check func(nodes []*Node, depth int)
	check = func(nodes []*Node, depth int) {
		for _, n := range nodes {
			if n == nil {
				add("", "Ein Block ist leer.")
				continue
			}
			blocks++
			id(n.ID)
			container := n.Type == TypeRepeat || n.Type == TypeRepeatUntil || n.Type == TypeIf
			if container && depth >= MaxDepth {
				add(n.ID, "Zu tief verschachtelt: höchstens %d Blöcke ineinander.", MaxDepth)
			}
			switch n.Type {
			case TypeCall:
				switch {
				case !cat.Known(n.Tool):
					add(n.ID, "Das Werkzeug %q gibt es nicht.", n.Tool)
				case !cat.Manual(n.Tool):
					add(n.ID, "%s kann nur der Agent benutzen, nicht ein eigenes Werkzeug.", n.Tool)
				default:
					args := n.Args
					if args == nil {
						args = map[string]any{}
					}
					if err := cat.CheckArgs(n.Tool, args); err != nil {
						add(n.ID, "%s", err.Error())
					}
				}
			case TypeRepeat:
				if n.Times < 1 || n.Times > MaxRepeat {
					add(n.ID, "Wiederhole geht von 1 bis %d mal, nicht %d mal.", MaxRepeat, n.Times)
				}
				check(n.Body, depth+1)
			case TypeRepeatUntil:
				blocks += checkCond(n.ID, n.Cond, add, id)
				check(n.Body, depth+1)
			case TypeIf:
				blocks += checkCond(n.ID, n.Cond, add, id)
				check(n.Body, depth+1)
				check(n.Else, depth+1)
			case TypeReturn:
				switch {
				case n.Return == nil || (n.Return.Field == "") == (n.Return.Text == ""):
					add(n.ID, "Gib zurück braucht entweder ein Feld des letzten Ergebnisses oder einen Text.")
				case n.Return.Field != "" && !resultFields[n.Return.Field]:
					add(n.ID, "Das Feld %q gibt es im Ergebnis nicht.", n.Return.Field)
				case utf8.RuneCountInString(n.Return.Text) > MaxReturnText:
					add(n.ID, "Der Text ist zu lang: höchstens %d Zeichen.", MaxReturnText)
				}
			default:
				add(n.ID, "Diesen Block kenne ich nicht: %q.", n.Type)
			}
		}
	}
	check(p.Body, 0)
	if blocks > MaxBlocks {
		add("", "Das Werkzeug hat %d Blöcke, erlaubt sind %d.", blocks, MaxBlocks)
	}
	return out
}

// ValidateName checks the name of a team tool: 3 to 30 lower-case letters,
// digits or underscores, and not the name of a built-in tool.
func ValidateName(name string, cat Catalog) []Problem {
	switch {
	case !nameRule.MatchString(name):
		return []Problem{{Field: FieldName, Text: "Der Name braucht 3 bis 30 Zeichen: kleine Buchstaben ohne Umlaute, Ziffern und _."}}
	case cat.Known(name):
		return []Problem{{Field: FieldName, Text: fmt.Sprintf("%s ist schon ein eingebautes Werkzeug. Wählt einen anderen Namen.", name)}}
	}
	return nil
}

// checkCond checks the condition of an if or a loop and returns how many
// blocks it counts (one).
func checkCond(owner string, c *Cond, add func(string, string, ...any), id func(string)) int {
	if c == nil {
		add(owner, "Hier fehlt die Bedingung.")
		return 0
	}
	id(c.ID)
	at := c.ID
	switch c.Source {
	case SourceResult:
		if !resultFields[c.Field] {
			add(at, "Das Feld %q gibt es im Ergebnis nicht.", c.Field)
		}
	case SourceSensor:
		if !sensorFields[c.Field] {
			add(at, "Den Sensorwert %q gibt es nicht.", c.Field)
		}
	default:
		add(at, "Eine Bedingung prüft das letzte Ergebnis oder einen Sensor, nicht %q.", c.Source)
	}
	switch {
	case !ops[c.Op]:
		add(at, "Den Vergleich %q gibt es nicht.", c.Op)
	case c.Op == OpContains && c.Field != "floor":
		add(at, "„enthält“ geht nur beim Boden.")
	case (c.Op == OpLt || c.Op == OpGt) && !isNumber(c.Value):
		add(at, "Größer und kleiner brauchen eine Zahl.")
	}
	if c.Value == nil {
		add(at, "Der Bedingung fehlt ein Wert.")
	}
	return 1
}

func isNumber(v any) bool {
	_, ok := toNumber(v)
	return ok
}

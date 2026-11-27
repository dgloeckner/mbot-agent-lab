package programs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/tools"
)

// Blocks: the browser stores the Blockly state of a team tool, the server
// turns it into the tree and runs only the tree. Tool blocks come from the
// registry and the block text table (tools.ToolBlocks); control and
// condition blocks are fixed here. Block texts are German; names in the
// block JSON stay English. docs/programs.md, "Blocks".

// BlockCatalog is what the blocks need from the tool registry.
type BlockCatalog interface {
	Catalog
	ToolBlocks() []tools.ToolBlock
}

// Block types that are not tools.
const (
	BlockStart        = "program_start"
	BlockRepeat       = "repeat_times"
	BlockRepeatUntil  = "repeat_until"
	BlockIf           = "if"
	BlockIfElse       = "if_else"
	BlockReturnField  = "return_field"
	BlockReturnText   = "return_text"
	BlockNot          = "cond_not"
	toolPrefix        = "tool_"
	conditionCheck    = "Condition"
	fieldDirection    = "dir"
	noneValue         = "none"
	categoryRepeat    = "Wiederholen"
	categoryDecide    = "Entscheiden"
	colorStart        = "#5c6675"
	colorRepeatBlocks = "#d9480f"
	colorDecideBlocks = "#0b7285"
)

var groupColors = map[string]string{
	tools.GroupDrive: "#2b6cee", tools.GroupSense: "#1f9d55", tools.GroupDisplay: "#8a4fff",
}

// condBlock is a fixed condition block: what it compares and how its
// fields map onto a Cond.
type condBlock struct {
	typ, message string
	source       string
	field        string // fixed field, or "" when the block has a FIELD dropdown
	fieldOptions [][2]string
	ops          [][2]string // OP dropdown; nil: always contains
	value        map[string]any
}

var (
	isOps      = [][2]string{{"ist", OpEq}, {"ist nicht", OpNe}}
	compareOps = [][2]string{{"<", OpLt}, {">", OpGt}, {"=", OpEq}, {"≠", OpNe}}
	floorColor = [][2]string{{"weiß", "white"}, {"schwarz", "black"}, {"rot", "red"}, {"gelb", "yellow"}, {"grün", "green"}, {"blau", "blue"}}
	letters    = [][2]string{{"schwarz", "k"}, {"rot", "r"}, {"gelb", "y"}, {"grün", "g"}, {"blau", "b"}, {"weiß", "w"}}
	reasons    = [][2]string{
		{"Wand", "wall"}, {"Farbe", "color"}, {"Hindernis", "obstacle"}, {"steckt fest", "blocked"}, {"Strecke erreicht", "max_cm"}, {"fertig", "done"},
		{"frei", "clear"}, {"kein freier Weg", "max_deg"}, {"Kreuzung", "crossing"}, {"Linie verloren", "line_lost"},
		{"kein Weg gespeichert", "empty"}, {"Zeit abgelaufen", "timeout"}, {"umgekippt", "tilted"},
	}
	returnFields = [][2]string{
		{"den Grund", "reason"}, {"die Bodenfarbe", "color"}, {"die gefahrenen cm", "driven_cm"}, {"die gedrehten Grad", "turned_deg"},
		{"den Abstand", "distance_cm"}, {"die Bodenspur", "floor"}, {"die freieste Richtung", "best_deg"},
	}
)

var condBlocks = []condBlock{
	{typ: "cond_reason", message: "Grund %1 %2", source: SourceResult, field: "reason", ops: isOps,
		value: map[string]any{"type": "field_dropdown", "name": "VALUE", "options": reasons}},
	{typ: "cond_result_number", message: "Ergebnis: %1 %2 %3", source: SourceResult, ops: compareOps,
		fieldOptions: [][2]string{{"gefahrene cm", "driven_cm"}, {"gedrehte Grad", "turned_deg"}, {"Abstand cm", "distance_cm"}, {"kleinster Abstand cm", "min_distance_cm"}, {"freieste Richtung", "best_deg"}},
		value:        map[string]any{"type": "field_number", "name": "VALUE", "value": 20}},
	{typ: "cond_result_color", message: "Bodenfarbe im Ergebnis %1 %2", source: SourceResult, field: "color", ops: isOps,
		value: map[string]any{"type": "field_dropdown", "name": "VALUE", "options": floorColor}},
	{typ: "cond_result_floor", message: "Bodenspur im Ergebnis enthält %1", source: SourceResult, field: "floor",
		value: map[string]any{"type": "field_dropdown", "name": "VALUE", "options": letters}},
	{typ: "cond_sensor_distance", message: "Abstand nach vorn jetzt %1 %2 cm", source: SourceSensor, field: "distance_cm", ops: compareOps,
		value: map[string]any{"type": "field_number", "name": "VALUE", "value": 20, "min": 0, "max": 400}},
	{typ: "cond_sensor_color", message: "Bodenfarbe jetzt %1 %2", source: SourceSensor, field: "color", ops: isOps,
		value: map[string]any{"type": "field_dropdown", "name": "VALUE", "options": floorColor}},
	{typ: "cond_sensor_floor", message: "Boden jetzt enthält %1", source: SourceSensor, field: "floor",
		value: map[string]any{"type": "field_dropdown", "name": "VALUE", "options": letters}},
	{typ: "cond_sensor_line", message: "Linie jetzt %1 %2", source: SourceSensor, field: "line", ops: isOps,
		value: map[string]any{"type": "field_dropdown", "name": "VALUE", "options": [][2]string{{"keine", "none"}, {"links", "left"}, {"in der Mitte", "center"}, {"rechts", "right"}, {"überall", "all"}}}},
}

// Definitions is what GET blocks answers: Blockly's JSON block definitions
// and the toolbox.
type Definitions struct {
	Blocks  []map[string]any `json:"definitions"`
	Toolbox map[string]any   `json:"toolbox"`
}

func options(pairs [][2]string) [][]string {
	out := make([][]string, len(pairs))
	for i, p := range pairs {
		out[i] = []string{p[0], p[1]}
	}
	return out
}

// toolField is the Blockly field of one tool block field.
func toolField(tb tools.ToolBlock, i int) map[string]any {
	f, s := tb.Fields[i], tb.Schema[i]
	if f.Direction {
		return map[string]any{"type": "field_dropdown", "name": fieldDirection, "options": [][]string{{"rechts", "right"}, {"links", "left"}}}
	}
	signed := false
	for _, g := range tb.Fields {
		signed = signed || (g.Direction && g.Arg == f.Arg)
	}
	switch {
	case len(s.Enum) > 0:
		var opts [][]string
		if f.NoneLabel != "" {
			opts = append(opts, []string{f.NoneLabel, noneValue})
		}
		for _, v := range s.Enum {
			label := v
			if l, ok := f.Labels[v]; ok {
				label = l
			}
			opts = append(opts, []string{label, v})
		}
		return map[string]any{"type": "field_dropdown", "name": f.Arg, "options": opts}
	case s.Type == "number" || s.Type == "integer":
		field := map[string]any{"type": "field_number", "name": f.Arg, "value": f.Default, "precision": 1}
		if s.Min != nil {
			field["min"] = *s.Min
		}
		if s.Max != nil {
			field["max"] = *s.Max
		}
		if signed {
			field["min"] = 0
		}
		return field
	case s.Type == "boolean":
		return map[string]any{"type": "field_checkbox", "name": f.Arg, "checked": f.Default == true}
	}
	field := map[string]any{"type": "field_input", "name": f.Arg, "text": f.Default}
	if s.MaxLen > 0 {
		field["maxLength"] = s.MaxLen
	}
	return field
}

func summary(tool string) string {
	for _, info := range tools.Catalog {
		if info.Name == tool {
			return info.Summary
		}
	}
	return ""
}

// Blocks builds the block definitions and the toolbox from the registry.
// A block has no robot field: the robot is chosen when the program runs.
func Blocks(bc BlockCatalog) Definitions {
	var defs []map[string]any
	cats := map[string][]map[string]any{}
	var order []string
	for _, tb := range bc.ToolBlocks() {
		args := make([]map[string]any, len(tb.Fields))
		for i := range tb.Fields {
			args[i] = toolField(tb, i)
		}
		def := map[string]any{
			"type": toolPrefix + tb.Tool, "message0": tb.Text, "previousStatement": nil, "nextStatement": nil,
			"colour": groupColors[tb.Group], "tooltip": tb.Tool + ": " + summary(tb.Tool),
		}
		if len(args) > 0 {
			def["args0"] = args
		}
		defs = append(defs, def)
		if _, ok := cats[tb.Group]; !ok {
			order = append(order, tb.Group)
		}
		cats[tb.Group] = append(cats[tb.Group], map[string]any{"kind": "block", "type": toolPrefix + tb.Tool})
	}

	stmt := func(name string) map[string]any { return map[string]any{"type": "input_statement", "name": name} }
	cond := func(name string) map[string]any {
		return map[string]any{"type": "input_value", "name": name, "check": conditionCheck}
	}
	defs = append(defs,
		map[string]any{"type": BlockStart, "message0": "Wenn das Werkzeug startet", "nextStatement": nil, "colour": colorStart,
			"tooltip": "Hier beginnt euer Werkzeug. Hängt die Blöcke darunter."},
		map[string]any{"type": BlockRepeat, "message0": "wiederhole %1 mal", "args0": []any{map[string]any{"type": "field_number", "name": "TIMES", "value": 4, "min": 1, "max": MaxRepeat, "precision": 1}},
			"message1": "%1", "args1": []any{stmt("DO")}, "previousStatement": nil, "nextStatement": nil, "colour": colorRepeatBlocks,
			"tooltip": fmt.Sprintf("Macht die Blöcke darin n-mal, 1 bis %d.", MaxRepeat)},
		map[string]any{"type": BlockRepeatUntil, "message0": "wiederhole %1", "args0": []any{stmt("DO")}, "message1": "bis %1", "args1": []any{cond("UNTIL")},
			"previousStatement": nil, "nextStatement": nil, "colour": colorRepeatBlocks,
			"tooltip": fmt.Sprintf("Macht die Blöcke darin und prüft danach die Bedingung, höchstens %d Runden.", MaxRounds)},
		map[string]any{"type": BlockIf, "message0": "wenn %1", "args0": []any{cond("COND")}, "message1": "dann %1", "args1": []any{stmt("DO")},
			"previousStatement": nil, "nextStatement": nil, "colour": colorDecideBlocks, "tooltip": "Macht die Blöcke darin nur, wenn die Bedingung stimmt."},
		map[string]any{"type": BlockIfElse, "message0": "wenn %1", "args0": []any{cond("COND")}, "message1": "dann %1", "args1": []any{stmt("DO")},
			"message2": "sonst %1", "args2": []any{stmt("ELSE")}, "previousStatement": nil, "nextStatement": nil, "colour": colorDecideBlocks,
			"tooltip": "Stimmt die Bedingung, macht es die Blöcke bei „dann“, sonst die bei „sonst“."},
		map[string]any{"type": BlockReturnField, "message0": "gib zurück: %1 des letzten Ergebnisses",
			"args0":             []any{map[string]any{"type": "field_dropdown", "name": "FIELD", "options": options(returnFields)}},
			"previousStatement": nil, "colour": colorDecideBlocks, "tooltip": "Beendet das Werkzeug und meldet diesen Wert."},
		map[string]any{"type": BlockReturnText, "message0": "gib zurück: %1", "args0": []any{map[string]any{"type": "field_input", "name": "TEXT", "text": "fertig", "maxLength": MaxReturnText}},
			"previousStatement": nil, "colour": colorDecideBlocks, "tooltip": "Beendet das Werkzeug und meldet diesen Text."},
	)
	decide := []map[string]any{{"kind": "block", "type": BlockIf}, {"kind": "block", "type": BlockIfElse}}
	for _, cb := range condBlocks {
		var args []any
		if cb.field == "" {
			args = append(args, map[string]any{"type": "field_dropdown", "name": "FIELD", "options": options(cb.fieldOptions)})
		}
		if cb.ops != nil {
			args = append(args, map[string]any{"type": "field_dropdown", "name": "OP", "options": options(cb.ops)})
		}
		v := map[string]any{}
		for k, x := range cb.value {
			v[k] = x
		}
		if o, ok := v["options"].([][2]string); ok {
			v["options"] = options(o)
		}
		args = append(args, v)
		tip := "Prüft das Ergebnis des letzten Werkzeugs."
		if cb.source == SourceSensor {
			tip = "Liest die Sensoren jetzt und prüft den Wert. Zählt als Schritt."
		}
		defs = append(defs, map[string]any{"type": cb.typ, "message0": cb.message, "args0": args, "output": conditionCheck, "colour": colorDecideBlocks, "tooltip": tip})
		decide = append(decide, map[string]any{"kind": "block", "type": cb.typ})
	}
	defs = append(defs, map[string]any{"type": BlockNot, "message0": "nicht %1", "args0": []any{cond("COND")}, "output": conditionCheck,
		"colour": colorDecideBlocks, "tooltip": "Dreht die Bedingung um."})
	decide = append(decide, map[string]any{"kind": "block", "type": BlockNot},
		map[string]any{"kind": "block", "type": BlockReturnField}, map[string]any{"kind": "block", "type": BlockReturnText})

	var contents []any
	for _, g := range order {
		contents = append(contents, map[string]any{"kind": "category", "name": g, "colour": groupColors[g], "contents": cats[g]})
	}
	contents = append(contents,
		map[string]any{"kind": "category", "name": categoryRepeat, "colour": colorRepeatBlocks, "contents": []any{
			map[string]any{"kind": "block", "type": BlockRepeat}, map[string]any{"kind": "block", "type": BlockRepeatUntil}}},
		map[string]any{"kind": "category", "name": categoryDecide, "colour": colorDecideBlocks, "contents": decide},
	)
	return Definitions{Blocks: defs, Toolbox: map[string]any{"kind": "categoryToolbox", "contents": contents}}
}

// --- Blockly state to tree ---------------------------------------------------

// bBlock is one block of Blockly's JSON serialization.
type bBlock struct {
	Type   string                     `json:"type"`
	ID     string                     `json:"id"`
	Fields map[string]json.RawMessage `json:"fields"`
	Inputs map[string]struct {
		Block *bBlock `json:"block"`
	} `json:"inputs"`
	Next *struct {
		Block *bBlock `json:"block"`
	} `json:"next"`
}

type bState struct {
	Blocks struct {
		Blocks []*bBlock `json:"blocks"`
	} `json:"blocks"`
}

type converter struct {
	bc       BlockCatalog
	blocks   map[string]tools.ToolBlock
	problems []Problem
}

func (c *converter) add(block, format string, args ...any) {
	c.problems = append(c.problems, Problem{Block: block, Text: fmt.Sprintf(format, args...)})
}

// FromBlockly turns a Blockly workspace state into the body of a program.
// It refuses unknown block types, agent-only tools as blocks, loose blocks
// beside the program and more than one start, each at its block.
func FromBlockly(state json.RawMessage, bc BlockCatalog) ([]*Node, []Problem) {
	c := &converter{bc: bc, blocks: map[string]tools.ToolBlock{}}
	for _, tb := range bc.ToolBlocks() {
		c.blocks[toolPrefix+tb.Tool] = tb
	}
	var st bState
	if len(state) > 0 && json.Unmarshal(state, &st) != nil {
		c.add("", "Die Blöcke konnten nicht gelesen werden.")
		return nil, c.problems
	}
	var start *bBlock
	for _, b := range st.Blocks.Blocks {
		switch {
		case b.Type == BlockStart && start == nil:
			start = b
		case b.Type == BlockStart:
			c.add(b.ID, "Es gibt mehr als einen Start. Ein Werkzeug hat genau einen Block „Wenn das Werkzeug startet“.")
		default:
			c.add(b.ID, "Dieser Block hängt an nichts. Hängt ihn unter „Wenn das Werkzeug startet“ oder löscht ihn.")
		}
	}
	if start == nil {
		if len(st.Blocks.Blocks) == 0 {
			return nil, c.problems
		}
		c.add("", "Es fehlt der Block „Wenn das Werkzeug startet“.")
		return nil, c.problems
	}
	var body []*Node
	if start.Next != nil {
		body = c.chain(start.Next.Block)
	}
	return body, c.problems
}

func (c *converter) chain(b *bBlock) []*Node {
	out := []*Node{}
	for ; b != nil; b = next(b) {
		if n := c.statement(b); n != nil {
			out = append(out, n)
		}
	}
	return out
}

func next(b *bBlock) *bBlock {
	if b.Next == nil {
		return nil
	}
	return b.Next.Block
}

func (b *bBlock) input(name string) *bBlock {
	in, ok := b.Inputs[name]
	if !ok {
		return nil
	}
	return in.Block
}

func (b *bBlock) str(name string) string {
	var s string
	_ = json.Unmarshal(b.Fields[name], &s)
	return s
}

func (b *bBlock) num(name string) (float64, bool) {
	var f float64
	err := json.Unmarshal(b.Fields[name], &f)
	return f, err == nil && b.Fields[name] != nil
}

func (c *converter) statement(b *bBlock) *Node {
	n := &Node{ID: b.ID}
	switch b.Type {
	case BlockRepeat:
		times, _ := b.num("TIMES")
		n.Type, n.Times = TypeRepeat, int(times)
		n.Body = c.chain(b.input("DO"))
	case BlockRepeatUntil:
		n.Type = TypeRepeatUntil
		n.Body = c.chain(b.input("DO"))
		n.Cond = c.cond(b.input("UNTIL"), false)
	case BlockIf, BlockIfElse:
		n.Type = TypeIf
		n.Cond = c.cond(b.input("COND"), false)
		n.Body = c.chain(b.input("DO"))
		if b.Type == BlockIfElse {
			n.Else = c.chain(b.input("ELSE"))
		}
	case BlockReturnField:
		n.Type, n.Return = TypeReturn, &Value{Field: b.str("FIELD")}
	case BlockReturnText:
		n.Type, n.Return = TypeReturn, &Value{Text: b.str("TEXT")}
	default:
		tb, ok := c.blocks[b.Type]
		if !ok {
			tool := strings.TrimPrefix(b.Type, toolPrefix)
			if strings.HasPrefix(b.Type, toolPrefix) && c.bc.Known(tool) && !c.bc.Manual(tool) {
				c.add(b.ID, "%s kann nur der Agent benutzen, nicht ein eigenes Werkzeug.", tool)
			} else {
				c.add(b.ID, "Diesen Block kenne ich nicht: %q.", b.Type)
			}
			return nil
		}
		n.Type, n.Tool, n.Args = TypeCall, tb.Tool, c.args(b, tb)
	}
	return n
}

// args reads a tool block's fields into the tool's arguments.
func (c *converter) args(b *bBlock, tb tools.ToolBlock) map[string]any {
	args := map[string]any{}
	left := false
	for i, f := range tb.Fields {
		switch {
		case f.Direction:
			left = b.str(fieldDirection) == "left"
		case len(tb.Schema[i].Enum) > 0 || tb.Schema[i].Type == "string":
			v := b.str(f.Arg)
			if _, ok := b.Fields[f.Arg]; !ok {
				v, _ = f.Default.(string)
			}
			if v != "" && v != noneValue {
				args[f.Arg] = v
			}
		case tb.Schema[i].Type == "boolean":
			var v bool
			if json.Unmarshal(b.Fields[f.Arg], &v) == nil {
				args[f.Arg] = v
			}
		default:
			if v, ok := b.num(f.Arg); ok {
				args[f.Arg] = v
			} else if f.Default != nil {
				args[f.Arg] = f.Default
			}
		}
	}
	for _, f := range tb.Fields {
		if f.Direction && left {
			if v, ok := args[f.Arg].(float64); ok && v > 0 {
				args[f.Arg] = -v
			}
		}
	}
	return args
}

// cond reads a condition block; "nicht" toggles the negation.
func (c *converter) cond(b *bBlock, not bool) *Cond {
	if b == nil {
		return nil
	}
	if b.Type == BlockNot {
		return c.cond(b.input("COND"), !not)
	}
	for _, cb := range condBlocks {
		if cb.typ != b.Type {
			continue
		}
		cond := &Cond{ID: b.ID, Source: cb.source, Field: cb.field, Op: OpContains, Not: not}
		if cb.field == "" {
			cond.Field = b.str("FIELD")
		}
		if cb.ops != nil {
			cond.Op = b.str("OP")
		}
		if cb.value["type"] == "field_number" {
			v, _ := b.num("VALUE")
			cond.Value = v
		} else {
			cond.Value = b.str("VALUE")
		}
		return cond
	}
	c.add(b.ID, "Diesen Block kenne ich nicht: %q.", b.Type)
	return nil
}

// Convert turns a Blockly state into a program and validates it.
func Convert(name, description string, state json.RawMessage, bc BlockCatalog) (*Program, []Problem) {
	body, problems := FromBlockly(state, bc)
	p := &Program{Name: name, Description: description, Body: body}
	if len(problems) > 0 {
		// Report the structure first; the values are checked once it holds.
		return p, append(problems, ValidateName(name, bc)...)
	}
	return p, Validate(p, bc)
}

// Describer names blocks in the words of the editor: a tool block as its
// German sentence with its values ("fahre 20 cm").
func Describer(bc BlockCatalog) func(*Node) string {
	texts := map[string]tools.ToolBlock{}
	for _, tb := range bc.ToolBlocks() {
		texts[tb.Tool] = tb
	}
	return func(n *Node) string {
		tb, ok := texts[n.Tool]
		if n.Type != TypeCall || !ok {
			return Describe(n)
		}
		s := tb.Text
		for i, f := range tb.Fields {
			v := n.Args[f.Arg]
			var word string
			switch {
			case f.Direction:
				word = "rechts"
				if x, ok := v.(float64); ok && x < 0 {
					word = "links"
				}
			case v == nil:
				word = f.NoneLabel
			default:
				if x, ok := v.(float64); ok && x < 0 && hasDirection(tb, f.Arg) {
					v = -x
				}
				word = fmt.Sprint(v)
				if l, ok := f.Labels[word]; ok {
					word = l
				}
			}
			s = strings.Replace(s, fmt.Sprintf("%%%d", i+1), word, 1)
		}
		return strings.ReplaceAll(s, "\n", " ")
	}
}

func hasDirection(tb tools.ToolBlock, arg string) bool {
	for _, f := range tb.Fields {
		if f.Direction && f.Arg == arg {
			return true
		}
	}
	return false
}

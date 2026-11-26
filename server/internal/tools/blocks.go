package tools

import "fmt"

// Block texts: one line per by-hand tool, the German sentence a team reads
// in the block editor with its fields (%1, %2, … in order), defaults and
// units. A new by-hand tool needs one line here and then appears in the
// editor. Ranges and options come from the tool's schema; names in the
// block JSON stay English. docs/tools.md, "Block texts".

// BlockField is one field of a tool block.
type BlockField struct {
	// Arg is the tool argument the field sets.
	Arg string
	// Default is the value a new block starts with. For an optional enum, ""
	// means "not given" and is shown with NoneLabel.
	Default any
	// NoneLabel offers leaving an optional enum out, shown with this text.
	NoneLabel string
	// Labels are German names for enum values; values without one show as is.
	Labels map[string]string
	// Direction makes the field a "rechts"/"links" choice that sets the sign
	// of Arg (positive is right); the number field of Arg then takes only
	// the amount.
	Direction bool
}

// BlockText is the block of one by-hand tool.
type BlockText struct {
	Tool   string
	Group  string // toolbox category
	Text   string // German, with %1, %2, … for the fields; "\n" starts a new row in the block
	Fields []BlockField
}

// Toolbox categories of tool blocks.
const (
	GroupDrive   = "Fahren"
	GroupSense   = "Sensoren"
	GroupDisplay = "Licht und Anzeige"
)

// ColorLabels are the German names of floor and light colors.
var ColorLabels = map[string]string{
	"white": "weiß", "black": "schwarz", "red": "rot", "yellow": "gelb", "green": "grün", "blue": "blau", "any": "irgendeine Farbe",
	"purple": "lila", "cyan": "türkis", "orange": "orange", "off": "aus",
}

var blockTexts = []BlockText{
	{Tool: "drive", Group: GroupDrive, Text: "fahre %1 cm", Fields: []BlockField{{Arg: "cm", Default: 20.0}}},
	{Tool: "turn", Group: GroupDrive, Text: "drehe %1 Grad nach %2", Fields: []BlockField{{Arg: "degrees", Default: 90.0}, {Arg: "degrees", Direction: true}}},
	{Tool: "drive_arc", Group: GroupDrive, Text: "fahre eine Kurve nach %1\nmit Radius %2 cm, %3 Grad weit",
		Fields: []BlockField{{Arg: "degrees", Direction: true}, {Arg: "radius_cm", Default: 30.0}, {Arg: "degrees", Default: 90.0}}},
	{Tool: "drive_until", Group: GroupDrive, Text: "fahre geradeaus, höchstens %1 cm,\nbis eine Wand näher als %2 cm ist\noder der Boden %3 ist",
		Fields: []BlockField{{Arg: "max_cm", Default: 100.0}, {Arg: "stop_below_cm", Default: 15.0}, {Arg: "stop_on_color", Default: "", NoneLabel: "egal", Labels: ColorLabels}}},
	{Tool: "turn_until_clear", Group: GroupDrive, Text: "drehe nach %1, bis vorne %2 cm frei sind",
		Fields: []BlockField{{Arg: "direction", Default: "right", Labels: map[string]string{"left": "links", "right": "rechts"}}, {Arg: "min_cm", Default: 40.0}}},
	{Tool: "follow_line_until", Group: GroupDrive, Text: "folge der Linie bis %1,\nhöchstens %2 cm",
		Fields: []BlockField{{Arg: "stop_on_color", Default: "", NoneLabel: "zur Kreuzung", Labels: ColorLabels}, {Arg: "max_cm", Default: 100.0}}},
	{Tool: "retrace_path", Group: GroupDrive, Text: "fahre den Weg zurück zum Anfang"},
	{Tool: "scan", Group: GroupSense, Text: "schau dich um von %1 bis %2 Grad\nund schau dann %3",
		Fields: []BlockField{{Arg: "from_deg", Default: -90.0}, {Arg: "to_deg", Default: 90.0},
			{Arg: "face", Default: "best", Labels: map[string]string{"best": "in die freieste Richtung", "start": "wieder nach vorn"}}}},
	{Tool: "read_sensors", Group: GroupSense, Text: "lies die Sensoren"},
	{Tool: "wait_for", Group: GroupSense, Text: "warte auf %1, höchstens %2 Sekunden",
		Fields: []BlockField{{Arg: "event", Default: "button_a", Labels: map[string]string{
			"obstacle_near": "ein Hindernis", "obstacle_clear": "freien Weg", "color_changed": "eine neue Bodenfarbe", "tilted": "Umkippen",
			"button_a": "Knopf A", "button_b": "Knopf B", "clap": "Klatschen", "battery_low": "schwachen Akku"}}, {Arg: "timeout_s", Default: 10.0}}},
	{Tool: "set_leds", Group: GroupDisplay, Text: "Licht %1", Fields: []BlockField{{Arg: "color", Default: "green", Labels: ColorLabels}}},
	{Tool: "show_text", Group: GroupDisplay, Text: "zeige %1", Fields: []BlockField{{Arg: "text", Default: "Hallo"}}},
	{Tool: "play_sound", Group: GroupDisplay, Text: "spiele den Ton %1", Fields: []BlockField{{Arg: "name", Default: "hello"}}},
	{Tool: "identify", Group: GroupDisplay, Text: "blinke und piepe"},
}

// FieldSchema is the part of a tool argument's schema a block field needs.
type FieldSchema struct {
	Type     string   // number, integer, string, boolean
	Enum     []string // options
	Min, Max *float64
	MaxLen   int
	Required bool
}

// ToolBlock is a block text with the schema of each field's argument.
type ToolBlock struct {
	BlockText
	Schema []FieldSchema // one per field, in order
}

// ToolBlocks returns the blocks of all by-hand tools in table order.
func (r *Registry) ToolBlocks() []ToolBlock {
	out := make([]ToolBlock, 0, len(blockTexts))
	for _, bt := range blockTexts {
		t, ok := r.tools[bt.Tool]
		if !ok || !t.manual {
			continue
		}
		props := map[string]any{}
		if t.props != nil {
			props = t.props(&Env{})
		}
		tb := ToolBlock{BlockText: bt}
		for _, f := range bt.Fields {
			spec, _ := props[f.Arg].(map[string]any)
			fs := FieldSchema{Enum: stringsOf(spec["enum"])}
			fs.Type, _ = spec["type"].(string)
			if v, ok := spec["minimum"]; ok {
				n := toFloat(v)
				fs.Min = &n
			}
			if v, ok := spec["maximum"]; ok {
				n := toFloat(v)
				fs.Max = &n
			}
			if v, ok := spec["maxLength"]; ok {
				fs.MaxLen = int(toFloat(v))
			}
			for _, req := range t.required {
				fs.Required = fs.Required || req == f.Arg
			}
			tb.Schema = append(tb.Schema, fs)
		}
		out = append(out, tb)
	}
	return out
}

// checkBlockTexts reports by-hand tools without a block and blocks of tools
// that are not by hand; the tests call it.
func (r *Registry) checkBlockTexts() []string {
	var problems []string
	have := map[string]bool{}
	for _, bt := range blockTexts {
		have[bt.Tool] = true
		if !r.Manual(bt.Tool) {
			problems = append(problems, fmt.Sprintf("%s has a block but is not by hand", bt.Tool))
		}
	}
	for _, name := range r.ManualNames() {
		if !have[name] {
			problems = append(problems, fmt.Sprintf("%s is by hand but has no block text", name))
		}
	}
	return problems
}

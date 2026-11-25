package tools

import "encoding/json"

// Info describes a tool for people: the team page lists it and offers it as
// autocompletion in the prompt editor. Summaries are German (student-facing).
// The model-facing descriptions and JSON schemas live in registry.go.
type Info struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Params  string `json:"params"`
	Summary string `json:"summary"`
	Ready   bool   `json:"ready"` // false: planned, not offered to agents yet

	// Manual: a person can run it in the console; Schema are its arguments,
	// the same JSON schema the model sees. Set per team by CatalogFor.
	Manual bool            `json:"manual"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

// Catalog mirrors docs/tools.md.
var Catalog = []Info{
	{Name: "drive", Group: "Bewegen", Params: "cm, speed?", Summary: "Fährt eine Strecke geradeaus, negativ heißt rückwärts.", Ready: true},
	{Name: "turn", Group: "Bewegen", Params: "degrees", Summary: "Dreht auf der Stelle, positiv ist rechts.", Ready: true},
	{Name: "drive_arc", Group: "Bewegen", Params: "radius_cm, degrees, stop_on_color?, speed?", Summary: "Fährt eine Kurve oder einen Kreis, positiv ist rechts, auf Wunsch bis zu einer Farbe.", Ready: true},
	{Name: "drive_until", Group: "Bewegen", Params: "max_cm, stop_below_cm?, stop_on_color?", Summary: "Fährt, bis eine Wand nah ist, eine Farbe kommt oder die Strecke erreicht ist.", Ready: true},
	{Name: "turn_until_clear", Group: "Bewegen", Params: "direction, min_cm?, max_deg?", Summary: "Dreht sich, bis vorne genug Platz ist.", Ready: true},
	{Name: "follow_line_until", Group: "Bewegen", Params: "max_cm, stop_on_color?, stop_at_crossing?", Summary: "Folgt einer Linie bis zu einer Farbe, einer Kreuzung oder ihrem Ende.", Ready: true},
	{Name: "retrace_path", Group: "Bewegen", Params: "", Summary: "Fährt den bisherigen Weg zurück zum Anfang.", Ready: true},
	{Name: "stop", Group: "Bewegen", Params: "", Summary: "Hält sofort an.", Ready: true},
	{Name: "read_sensors", Group: "Wahrnehmen", Params: "", Summary: "Liest Abstand, Bodenfarbe, Linie, Richtung und Akku.", Ready: true},
	{Name: "scan", Group: "Wahrnehmen", Params: "from_deg?, to_deg?, step_deg?, face?", Summary: "Dreht sich und misst rundum, dann schaut er in die freieste Richtung.", Ready: true},
	{Name: "wait_for", Group: "Wahrnehmen", Params: "event, timeout_s", Summary: "Wartet auf ein Ereignis, etwa einen Knopfdruck.", Ready: true},
	{Name: "list_robots", Group: "Wahrnehmen", Params: "", Summary: "Zeigt eure Roboter mit Status und Akku.", Ready: true},
	{Name: "set_leds", Group: "Ausdrücken", Params: "color", Summary: "Schaltet die Lichter in einer Farbe.", Ready: true},
	{Name: "show_text", Group: "Ausdrücken", Params: "text", Summary: "Zeigt bis zu 60 Zeichen auf dem Display.", Ready: true},
	{Name: "play_sound", Group: "Ausdrücken", Params: "name", Summary: "Spielt einen Ton.", Ready: true},
	{Name: "play_melody", Group: "Ausdrücken", Params: "notes", Summary: "Spielt eine Melodie aus Tonhöhe und Dauer.", Ready: false},
	{Name: "gesture", Group: "Ausdrücken", Params: "kind: nod, shake, spin", Summary: "Nicken, Kopfschütteln oder Drehen.", Ready: false},
	{Name: "identify", Group: "Ausdrücken", Params: "", Summary: "Blinkt und piept, damit ihr euren Roboter findet.", Ready: true},
	{Name: "say", Group: "Reden", Params: "to, text, emotion?", Summary: "Sagt etwas zu einem anderen Agenten oder zu allen.", Ready: true},
	{Name: "tell_team", Group: "Reden", Params: "text", Summary: "Schreibt euch eine Nachricht.", Ready: true},
	{Name: "remember", Group: "Merken", Params: "text", Summary: "Legt eine Notiz an, die auch im nächsten Lauf bleibt.", Ready: true},
	{Name: "forget", Group: "Merken", Params: "note_id", Summary: "Löscht eine Notiz.", Ready: true},
	{Name: "finish", Group: "Abschluss", Params: "report", Summary: "Beendet den Lauf mit einem Bericht.", Ready: true},
}

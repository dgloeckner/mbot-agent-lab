package tools

// Info describes a tool for people: the team page lists it and offers it as
// autocompletion in the prompt editor. Summaries are German (student-facing).
// The model-facing descriptions and JSON schemas live in registry.go.
type Info struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Params  string `json:"params"`
	Summary string `json:"summary"`
	Ready   bool   `json:"ready"` // false: planned, not offered to agents yet
}

// Catalog mirrors docs/tools.md.
var Catalog = []Info{
	{"drive", "Bewegen", "cm, speed?", "Fährt eine Strecke geradeaus, negativ heißt rückwärts.", true},
	{"turn", "Bewegen", "degrees", "Dreht auf der Stelle, positiv ist rechts.", true},
	{"drive_until", "Bewegen", "max_cm, stop_below_cm?, stop_on_color?", "Fährt, bis eine Wand nah ist, eine Farbe kommt oder die Strecke erreicht ist.", true},
	{"follow_line_until", "Bewegen", "stop_on_color?, max_cm", "Folgt einer Linie bis zur Abbruchbedingung.", false},
	{"retrace_path", "Bewegen", "", "Fährt den bisherigen Weg rückwärts ab.", false},
	{"stop", "Bewegen", "", "Hält sofort an.", true},
	{"read_sensors", "Wahrnehmen", "", "Liest Abstand, Bodenfarbe, Linie, Richtung und Akku.", true},
	{"wait_for", "Wahrnehmen", "event, timeout_s", "Wartet auf ein Ereignis, etwa einen Knopfdruck.", true},
	{"list_robots", "Wahrnehmen", "", "Zeigt eure Roboter mit Status und Akku.", true},
	{"set_leds", "Ausdrücken", "color", "Schaltet die Lichter in einer Farbe.", true},
	{"show_text", "Ausdrücken", "text", "Zeigt bis zu 60 Zeichen auf dem Display.", true},
	{"play_sound", "Ausdrücken", "name", "Spielt einen Ton.", true},
	{"play_melody", "Ausdrücken", "notes", "Spielt eine Melodie aus Tonhöhe und Dauer.", false},
	{"gesture", "Ausdrücken", "kind: nod, shake, spin", "Nicken, Kopfschütteln oder Drehen.", false},
	{"identify", "Ausdrücken", "", "Blinkt und piept, damit ihr euren Roboter findet.", true},
	{"say", "Reden", "to, text, emotion?", "Sagt etwas zu einem anderen Agenten oder zu allen.", true},
	{"tell_team", "Reden", "text", "Schreibt euch eine Nachricht.", true},
	{"remember", "Merken", "text", "Legt eine Notiz an, die auch im nächsten Lauf bleibt.", true},
	{"forget", "Merken", "note_id", "Löscht eine Notiz.", true},
	{"finish", "Abschluss", "report", "Beendet den Lauf mit einem Bericht.", true},
}

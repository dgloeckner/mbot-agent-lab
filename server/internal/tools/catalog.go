package tools

// Info describes a tool for people: the team page lists it and offers it as
// autocompletion in the prompt editor. Summaries are German (student-facing).
// The model-facing JSON schemas will live in the registry once it exists.
type Info struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Params  string `json:"params"`
	Summary string `json:"summary"`
}

// Catalog mirrors docs/tools.md.
var Catalog = []Info{
	{"drive", "Bewegen", "cm, speed?", "Fährt eine Strecke geradeaus, negativ heißt rückwärts."},
	{"turn", "Bewegen", "degrees", "Dreht auf der Stelle, positiv ist rechts."},
	{"drive_until", "Bewegen", "max_cm, stop_below_cm?, stop_on_color?", "Fährt, bis eine Wand nah ist, eine Farbe kommt oder die Strecke erreicht ist."},
	{"follow_line_until", "Bewegen", "stop_on_color?, max_cm", "Folgt einer Linie bis zur Abbruchbedingung."},
	{"retrace_path", "Bewegen", "", "Fährt den bisherigen Weg rückwärts ab."},
	{"stop", "Bewegen", "", "Hält sofort an."},
	{"read_sensors", "Wahrnehmen", "", "Liest Abstand, Bodenfarbe, Linie, Richtung und Akku."},
	{"wait_for", "Wahrnehmen", "event, timeout_s", "Wartet auf ein Ereignis, etwa einen Knopfdruck."},
	{"list_robots", "Wahrnehmen", "", "Zeigt alle Roboter mit Status und Farbe."},
	{"set_leds", "Ausdrücken", "color oder pattern", "Schaltet die Lichter."},
	{"show_text", "Ausdrücken", "text", "Zeigt bis zu 60 Zeichen auf dem Display."},
	{"play_sound", "Ausdrücken", "name", "Spielt einen Ton."},
	{"play_melody", "Ausdrücken", "notes", "Spielt eine Melodie aus Tonhöhe und Dauer."},
	{"gesture", "Ausdrücken", "kind: nod, shake, spin", "Nicken, Kopfschütteln oder Drehen."},
	{"identify", "Ausdrücken", "", "Blinkt und piept, damit ihr euren Roboter findet."},
	{"say", "Reden", "to, text, emotion?", "Sagt etwas zu einem anderen Agenten oder zu allen."},
	{"tell_team", "Reden", "text", "Schreibt euch eine Nachricht."},
	{"remember", "Merken", "text", "Legt eine Notiz an, die auch im nächsten Lauf bleibt."},
	{"forget", "Merken", "note_id", "Löscht eine Notiz."},
	{"finish", "Abschluss", "report", "Beendet den Lauf mit einem Bericht."},
}

# Leitfaden: Eigene Werkzeuge und die Labyrinth-Challenge

Für die Lab-Leitung. Die Teams lernen in fünf Stufen die Grundideen des Programmierens
(Anweisung, Schleife, Bedingung) mit denselben Robotern und auf derselben Seite, auf der sie
später ihren Agenten steuern. Was ein Team aus Blöcken baut, heißt **Werkzeug**; das Bauen heißt
**programmieren**. Die Aufgaben liegen in `aufgaben/de/werkzeuge-*.md` (und gleichnamig in
`aufgaben/en/`), Import wie in `README.md` beschrieben.

## Der rote Faden

| Stufe | Aufgabe | Wer entscheidet | Begriff |
| --- | --- | --- | --- |
| 1. Konsole | `werkzeuge-1-konsole.md` | **die Person** ist der Agent: sie führt ein Werkzeug aus, liest das Ergebnis, entscheidet den nächsten Schritt | Anweisung, Reihenfolge, Ergebnis |
| 2. Wiederholen | `werkzeuge-2-wiederholen.md` | die Person **schreibt** die Wiederholung als Blöcke auf | Schleife mit Zähler und mit Abbruchbedingung |
| 3. Entscheiden | `werkzeuge-3-entscheiden.md` | die Person **schreibt** die Entscheidung als Blöcke auf | Bedingung, Sonderfall |
| 4. Agent | `werkzeuge-4-agent.md` | ein **Sprachmodell** entscheidet, welches Werkzeug wann läuft | Werkzeug, Beschreibung, Prompt |
| 5. Tool-Design | `werkzeuge-5-challenge.md` | die Frage ist, **was ins Werkzeug gehört** und was der Agent entscheidet | groß und starr oder klein und beweglich |

Die Stufen bauen aufeinander auf; Stufe 3 braucht das gebaute Labyrinth (siehe unten). Wählt ein
Team beim Annehmen „Prompt aus Aufgabe … übernehmen“, kommen auch die eigenen Werkzeuge dieser
Aufgabe mit; so arbeitet es in Stufe 4 und 5 mit den Werkzeugen aus Stufe 3 weiter.

## Rollen im Team

Einen Roboter kann immer nur eine Person steuern. Die Seite sperrt das nicht: Ein Team darf sie
auf mehreren Rechnern öffnen, und Mitlesen ist nützlich. Statt einer Sperre gibt es Rollen:

| Rolle | Wo | Macht |
| --- | --- | --- |
| Steuern | am Rechner | führt Werkzeuge aus, baut und speichert die eigenen Werkzeuge, deployt, startet Läufe. Die einzige Person, die schreibt. |
| Beobachten | am Feld | schaut auf den Roboter, stellt ihn zurück, sagt laut, was wirklich passiert ist |
| Mitlesen | zweiter Rechner oder neben dem ersten | verfolgt Protokoll und Aufgabentext, vergleicht Ergebnis und Wirklichkeit |

- Mit jeder Aufgabe tauscht das Team die Rollen, sodass jede und jeder einmal steuert.
- **„Stopp“ darf jede und jeder drücken**, an jedem Rechner. Das ist die eine Aktion, die keine
  Absprache braucht.
- Zu zweit sind Beobachten und Mitlesen eine Rolle.
- Was die Seite schon tut: Der Roboter nimmt immer nur einen Fahrbefehl an, Werkzeuge von Hand
  gehen nicht, solange der Agent fährt, und jeder Aufruf von Hand steht auf allen Rechnern des
  Teams als „Ihr“ im Protokoll.
- Worauf ihr achtet: ein Team, in dem zwei Personen abwechselnd klicken. Der Leitstand zeigt pro
  Team, wie viele Browser offen sind. Dann die Rollen noch einmal ansprechen.
- Die Rollen stehen auch im Aufgabentext von Stufe 1, in den Worten der Teams, weil ein Team dort
  zum ersten Mal selbst steuert.

## Versionen

Ein **Deploy** speichert den Prompt und alle eigenen Werkzeuge des Teams zusammen als eine
Version. Der Agent bekommt die Werkzeuge der neuesten Version, die fehlerfrei sind und eine
Beschreibung haben; eine Änderung danach gilt erst ab dem nächsten Deploy. Im Reiter „Werkzeuge“
steht bei jedem eigenen Werkzeug, ob der Agent es hat (und wenn nicht, warum).

Unter „Verlauf“ steht jede Version. „Diese Version wiederherstellen“ holt Prompt **und** Werkzeuge
zurück und fragt vorher, wenn dabei noch nicht Deploytes verloren ginge. Für jede Version zeigt
der Verlauf, welche Werkzeuge neu, geändert oder entfernt sind; ein Klick auf ein Werkzeug zeigt
alte und neue Blöcke nebeneinander, mit Markierungen für geänderte Werte und neue, entfernte und
verschobene Blöcke. Mit „Vergleichen mit“ lassen sich zwei beliebige Versionen vergleichen.

## Optionale Demo: Linie folgen

Vor Stufe 1 oder 2, etwa zehn Minuten, auf dem Linienfeld (`feld/`), am Beamer:

1. In der Konsole Schritt für Schritt: `read_sensors` zeigt, wo die Linie unter dem Roboter
   liegt (`line`: links, Mitte, rechts). Dann ein paar Zentimeter `drive`, ein paar Grad `turn`,
   wieder `read_sensors` … Das Publikum sagt an, was als Nächstes kommt. Nach einer Minute ist
   klar: Das ist mühsam, und jede Entscheidung braucht eine Messung.
2. Dann das eingebaute Werkzeug `follow_line_until`: ein Aufruf, und der Roboter folgt der Linie
   bis zur Kreuzung. Die schnelle Regelschleife läuft auf dem Roboter.

Botschaft: Ein gutes Werkzeug nimmt dem, der entscheidet, die immer gleichen Schritte ab.

## Drei Schnitte von Werkzeugen

Für Stufe 5 gibt es drei Schnitte; Beispiele liegen als fertige Werkzeuge in
`docs/examples/programs/` (zum Vorführen in einem Demo-Team speichern):

| Schnitt | Beispiel | Der Agent … | Gut, wenn … | Schlecht, wenn … |
| --- | --- | --- | --- | --- |
| groß | `labyrinth`: fährt bis zur nächsten Wand, biegt rechts ab, bis der Boden blau ist | ruft ein Werkzeug auf und wartet | das Labyrinth bekannt ist: ein Modellaufruf, schnell | etwas anders ist als gedacht: das Werkzeug fährt stur weiter oder bricht ab |
| mittel | `wand_ausweichen`: schaut fünfmal nach vorn, biegt an einer Wand links ab, sonst 20 cm weiter | entscheidet nach ein paar Schritten | Teilstrecken immer gleich gehen | die Sonderfälle zwischen den Teilstrecken liegen |
| klein | `ein_feld`, `rechts`, `links`: ein Feld fahren (meldet die Bodenfarbe), eine Vierteldrehung | entscheidet nach jedem Schritt | die Umgebung sich ändert | jeder Schritt einen Modellaufruf kostet |

Zwei Faustregeln für die Teams:

1. **Ins Werkzeug gehört, was immer gleich geht; der Agent entscheidet, was von der Lage abhängt.**
   Bis zur Wand fahren ist immer gleich; wohin danach, hängt vom Labyrinth ab.
2. **Ein Werkzeug endet dort, wo eine Entscheidung nötig wird, und meldet, was man für sie
   braucht** (mit „gib zurück“ oder im letzten Ergebnis: Grund, Bodenfarbe, Abstand). Ein
   Werkzeug, das nichts meldet, lässt den Agenten raten.

## Die Challenge

Aufgabe `werkzeuge-5-challenge.md` (mit `agent_tools: team_tools`: Der Agent hat nur die
deployten eigenen Werkzeuge des Teams und `read_sensors`, `say`, `tell_team`, `remember`,
`forget`, `finish`).

| Runde | Labyrinth | Dauer (Richtwert) |
| --- | --- | --- |
| 1 | das bekannte Labyrinth (wie `labyrinth_rechts`) | 2 Läufe pro Team |
| 2 | gestört (wie `labyrinth_gestoert`); die Teams erfahren vorher nicht, was anders ist | 2 Läufe pro Team |
| Umbau | Werkzeuge und Prompt neu schneiden, deployen | 15 Minuten |
| 3 | noch einmal gestört | 2 Läufe pro Team |

**Störungen** (eine oder mehrere, für alle Teams gleich): eine Kiste im Gang, die einen Umweg
erzwingt; das blaue Feld in einen Seitengang verlegt; eine zusätzliche Wand, sodass „immer rechts
abbiegen“ im Kreis führt. Der Simulator hat beides vorbereitet: `labyrinth_rechts` wird mit
„an jeder Wand rechts“ gelöst, `labyrinth_gestoert` nicht mehr, aber mit „der rechten Wand Feld
für Feld folgen“.

**Werten** im Leitstand: Aufgabe öffnen, „Läufe“. Pro Lauf stehen dort Version, Ende, Zeit bis
zum Ziel, Modellaufrufe, Eingriffe und die Werkzeuge, die der Agent hatte.

1. Jeden Lauf mit „Lauf starten“ beim Team starten (oder das Team startet selbst).
2. Nach dem Lauf „✓ geschafft“ oder „✗ nicht“: Ob der Roboter auf dem blauen Feld steht,
   entscheidet ihr. Der Agent meldet mit `finish` nur, dass er fertig zu sein glaubt.
3. Fasst jemand den Roboter während des Laufs an: „+ von Hand bewegt“. Stopps des Teams und
   Werkzeuge, die das Team vor dem Lauf selbst ausgeführt hat, zählt der Server selbst.
4. Die Rangliste: zuerst geschaffte Läufe, dann wenigste Modellaufrufe, dann kürzeste Zeit;
   gewertet wird der beste Lauf jedes Teams.

## Nachbesprechung

Jedes Team, etwa drei Minuten, am Beamer:

1. **Werkzeugliste** zeigen (Reiter „Werkzeuge“, „Eigene Werkzeuge“): Welche Werkzeuge hat der
   Agent, was steht in den Beschreibungen?
2. **Was hat sich zwischen Runde 2 und Runde 3 geändert?** Im „Verlauf“ die Version aus Runde 2
   wählen, „Vergleichen mit“ die Version aus Runde 3, dann die Werkzeuge anklicken: alte und neue
   Blöcke nebeneinander.
3. **Wo habt ihr die Grenze zwischen Werkzeug und Agent gezogen, und warum?**

Gute Fragen an alle: Welches Team hatte die wenigsten Modellaufrufe in Runde 1, welches in Runde 3?
Wer hat in Runde 2 gewonnen, obwohl er in Runde 1 langsam war?

## Das Labyrinth bauen

Vorlage sind die Welten des Simulators (Punkte als (x, y) in cm, Ursprung in der Ecke unten
links, x nach rechts, y nach oben):

- `sim/worlds/labyrinth_rechts.json`: 200 × 200 cm, Gänge 50 cm breit. Start im linken Gang bei
  (25, 20), Blick nach oben. Innenwände von (50, 0) nach (50, 150), von (150, 0) nach (150, 150)
  und von (50, 150) nach (150, 150). Der Weg: hoch, oben nach rechts, im rechten Gang hinunter
  zum blauen Feld (50 × 40 cm, von (150, 0) bis (200, 40)).
- `sim/worlds/labyrinth_gestoert.json`: dieselbe Außenwand und derselbe Start. Innenwände von
  (50, 0) nach (50, 150), von (150, 0) nach (150, 100), von (100, 150) nach (150, 150) und von
  (100, 0) nach (100, 100); eine Kiste 30 × 30 cm von (110, 160) bis (140, 190) im oberen Gang;
  das blaue Feld liegt am Ende des Seitengangs in der Mitte, von (100, 0) bis (150, 40).

Regeln für den Aufbau:

- **Wände höher als der Ultraschallsensor**, damit er sie sicher sieht: Startwert 20 cm (Karton,
  Bretter). Glatte, gerade Flächen; der Sensor sieht schräge oder weiche Flächen schlecht.
- **Gänge** 50 cm breit wie im Simulator. Schmaler als etwa 30 cm wird es eng (Startwert, im
  Probelauf mit echten Robotern messen).
- **Wenige Kurven.** Ohne Linie zum Ausrichten sammelt jede Drehung einen kleinen Fehler; nach
  etwa acht bis zehn Drehungen fährt der Roboter schief (Schätzung, im Probelauf messen).
- **Blaues Feld** aus mattem blauem Klebeband oder Karton, vollflächig.
- Vorher mit dem Simulator ausprobieren: `npx nx run sim:run --robots=2
  --world=sim/worlds/labyrinth_rechts.json` (und `labyrinth_gestoert.json`).

## Zeitplan für einen Lab-Tag

| Zeit | Was |
| --- | --- |
| 9:00 | Begrüßung, Teams gründen (`/join`), Roboter zuteilen |
| 9:15 | Demo Linie folgen (optional) |
| 9:30 | Stufe 1: Selbst steuern |
| 10:15 | Stufe 2: Wiederholen |
| 11:00 | Pause |
| 11:15 | Stufe 3: Entscheiden |
| 12:00 | Mittagspause; Labyrinth fertig aufbauen, Störung vorbereiten |
| 12:45 | Stufe 4: Agent |
| 13:45 | Stufe 5: Challenge – Runde 1, Runde 2, Umbau, Runde 3 |
| 15:00 | Nachbesprechung |
| 15:30 | Ende; Aufgaben schließen |

<!--
Original field handoff (German), kept as written, like docs/handoff.md. Decisions the user
took on 2026-11-22 that change it are listed right below; they win over the text.
-->

> **Approved deviations (2026-11-22).** Where the text below says otherwise, these apply:
>
> 1. `turn`: positive is **right**, as in the main handoff. Agents count right turns as +90,
>    which matches compass headings (north 0, east 90).
> 2. All turns (`turn`, `turn_until_clear`, `scan`, `find_line`) are measured with the wheel
>    encoders. The gyro is used for tilt detection only.
> 3. Results keep `driven_cm` (not `drove_cm`). The stop reason `crossing` is renamed `junction`.
> 4. `follow_line_until` steers on the gray values of L1 and R1, not on `get_offset_track`.
> 5. Floor colors: own RGB classification per probe against references, three equal readings in
>    a row; the built-in color names are the fallback without calibration.
> 6. Calibration values reach the robot in a new MQTT message `config` (server to robot, after
>    `hello`), stored per robot by the server; the control room gets the calibration functions.
>
> Status: G1 to G3 are implemented (band world and `sim/grid.py`, four probes, color
> classification and `calibrate_colors`, floor trace). See `docs/protocol.md` for the format.

# Handoff: Koordinatensystem im mBot2 Agent Lab

Nov 22, 2026 · @Daniel

## Zweck und Einordnung

Dieses Dokument ergänzt den Handoff „mBot2 Agent Lab“. Es beschreibt ein Bodenfeld aus Linien und Farbstreifen, die Sensorik dafür und die generischen Fahr-Tools, mit denen ein Agent sich auf diesem Feld zurechtfindet.

**Zielbild**

- Das Gitter ist nur Umgebung. Roboter, Tools, Server und Leitstand wissen nichts davon: keine Knoten, kein x und y, keine Karte, keine Wegplanung.
- Der Roboter kennt seine Position nicht. Er fährt, dreht und meldet, was die vier Sonden am Boden gesehen haben.
- Das Wissen liegt beim Agenten. Er leitet Aufbau und Position aus den Rückgaben der Tools ab und merkt sie sich selbst, im Prompt und in Notizen.
- Wie viel der Agent vorab weiß, legt das Team im Prompt fest. Das Feld zu erkunden und zu verstehen kann selbst die Aufgabe sein.

**Was robust funktionieren muss**

1. Die Linie wiederfinden, von jeder Stelle des Feldes aus.
2. Eine Strecke fahren und zurückmelden, was am Boden lag.
3. An Abzweigen über die Encoder kontrolliert drehen.

**Bewusst nicht enthalten**

- Positionsverfolgung auf dem Roboter, Gitter-Konfiguration im Server, Wegplanung, Kartenansicht im Leitstand.
- Tools mit Gitterbegriffen wie `localize`, `go_to`, `get_position` oder `scan_markers`.
- Gyro. Er ist zu ungenau, Drehungen laufen über die Encoder.
- Kamera und Bildauswertung.

**Verhältnis zum Haupt-Handoff**

- Architektur, MQTT, Agent-Runtime und die exklusive Zuordnung der Roboter gelten unverändert.
- Der Tool-Katalog bekommt ein neues Tool, `find_line`, und eine einheitliche Rückgabe `floor` für alle Fahr-Tools.
- Die Sensor-APIs, die dort noch als ungeprüft stehen, sind hier geprüft.

## Entscheidungen

Diese Punkte sind entschieden. Maße und Schwellen sind Startwerte, bis sie am Roboter gemessen sind.

| Thema | Entscheidung | Grund |
| --- | --- | --- |
| Wissen über das Feld | Nur beim Agenten (Prompt, Notizen) und in den Unterlagen | Zielbild; Erkunden wird zur Aufgabe |
| Roboter, Server, Leitstand | Frei von Gitterbegriffen, nur generische Fähigkeiten | Derselbe Code passt zu jedem Linienfeld |
| Rückgabe der Fahr-Tools | Bodenspur `floor`: was die vier Sonden entlang des Wegs sahen | Ersetzt ein eigenes Scan-Tool |
| Linie verloren | `find_line` als eigenes Tool | Der Agent entscheidet, wann gesucht wird |
| Abzweig | Generischer Begriff: eine äußere Sonde sieht Schwarz | Gilt für jede Linienbahn, nicht nur für das Gitter |
| Drehen | Über die Encoder, auf Wunsch mit Einrasten auf die Linie | Gyro ist zu ungenau |
| Linien | Schwarzes Isolierband, 19 mm, auf weißem, mattem Grund | Sicher erkennbar, einfach zu kleben |
| Marken | Farbige Querstreifen unter der Linie, gelesen von L2 und R2 | Das Linienfolgen mit L1 und R1 bleibt ungestört |
| Farben | Rot, Gelb, Grün, Hellblau | Vier Farben reichen für 4 × 4 Knoten |
| Bedeutung der Farben | Steht nur im Feldwissen; Tools melden Farben, nie Zahlen | x und y sind den Tools unbekannt |
| Simulator | Kennt die Welt nur als Zeichnung aus Bändern | Einzige Stelle im Repo mit Feldgeometrie |

## Aufbau des Gitternetzes

Das Feld ist ein Gitter aus schwarzen Linien mit farbigen Querstreifen. Dieser Abschnitt ist die Bauanleitung. Im Code von Roboter, Server und Leitstand kommt nichts davon vor.

**Maße**

| Teil | Maß |
| --- | --- |
| Grundfläche | 120 × 120 cm, weiß und matt |
| Knoten | 4 × 4, Rastermaß 30 cm |
| Rand um das Gitter | 15 cm |
| Linien | schwarzes Isolierband, 19 mm breit |
| Außenrahmen | geschlossen, keine Stummel nach außen |
| Querstreifen | farbiges Isolierband, 19 mm breit, 10 cm lang, mittig quer zur Linie |
| Lage der Streifen | Mitte bei 8 cm und bei 12 cm ab Knotenmitte, auf jedem Arm |

**Farbcode**

| Farbe | Ziffer |
| --- | --- |
| Rot | 0 |
| Gelb | 1 |
| Grün | 2 |
| Hellblau | 3 |

**Streifen an jedem Arm eines Knotens**

- Der innere Streifen, 8 cm ab Knotenmitte, trägt die Farbe für x.
- Der äußere Streifen, 12 cm ab Knotenmitte, trägt die Farbe für y.
- Zwischen zwei Knoten A und B liegen deshalb vier Streifen: bei 8 cm x von A, bei 12 cm y von A, bei 18 cm y von B, bei 22 cm x von B.

Maßstabsgetreuer Bauplan des ganzen Feldes: [`docs/lab/feld/bauplan.svg`](lab/feld/bauplan.svg).

Im Beispiel ist A der Knoten (0,2) und B der Knoten (1,2). Von A nach B liegen Rot, Grün, Grün, Gelb am Weg, von B nach A dieselben Farben rückwärts. Die äußeren Sonden L2 und R2 sehen die Streifenfarbe, die inneren Sonden L1 und R1 bleiben auf Schwarz.

**Feldplan**

```
y=3   o---o---o---o
      |   |   |   |
y=2   o---o---o---o
      |   |   |   |
y=1   o---o---o---o
      |   |   |   |
y=0   o---o---o---o
     x=0 x=1 x=2 x=3
```

Der Ursprung liegt unten links. x wächst nach Osten, y nach Norden. Das ist eine Vereinbarung auf dem Papier, kein Wert im Code.

**Material**

| Material | Menge |
| --- | --- |
| Weiße, matte Unterlage | 120 × 120 cm, etwa Platte, Papierbahn oder Banner |
| Isolierband schwarz | 7,2 m für 24 Streckenstücke |
| Isolierband rot | 2,0 m |
| Isolierband gelb | 2,8 m |
| Isolierband grün | 2,8 m |
| Isolierband hellblau | 2,0 m |

Das sind 96 Streifen zu je 10 cm. Dazu kommt etwas Band für den Referenzstreifen.

**Herstellung**

1. Die 16 Knoten anzeichnen, Raster 30 cm, Rand 15 cm.
2. Auf jedem Arm die Streifenmitten bei 8 cm und 12 cm markieren.
3. Die Farbstreifen quer zur späteren Linie kleben.
4. Die schwarzen Linien durchgehend über die Streifen kleben.
5. Den Referenzstreifen am Rand kleben: Weiß, Schwarz, Rot, Gelb, Grün, Blau.

**Kontrolle nach dem Kleben**

- Jedes Streckenstück hat genau vier Streifen.
- Auf waagrechten Strecken sind die beiden mittleren Streifen gleichfarbig, auf senkrechten die beiden äußeren.
- Alle Arme eines Knotens zeigen dieselbe Farbfolge.

## Feldwissen: Leseregeln

Dieser Abschnitt ist Wissen für Menschen und Agenten. Nichts davon steht im Code von Roboter, Server oder Leitstand. Ein Team gibt es seinem Agenten ganz, teilweise oder gar nicht mit.

**Zwei Regeln**

1. Die ersten zwei Farben nach einem Abzweig gehören zu dem Knoten, den man verlässt: erst x, dann y.
2. Die letzten zwei Farben vor einem Abzweig gehören zu dem Knoten, den man erreicht: erst y, dann x.

**Folgen daraus**

- Die Regeln gelten in jeder Fahrtrichtung.
- Nach einem ganzen Streckenstück kennt der Agent zwei Knoten: woher er kommt und wo er steht.
- Nach `find_line` steht der Roboter mitten auf einer Strecke. Sieht er bis zum nächsten Abzweig noch zwei Farben, kennt er diesen Knoten. Sieht er weniger, fährt er ein Stück weiter.

**Richtung aus zwei Knoten**

| Änderung | Blickrichtung |
| --- | --- |
| x + 1 | Osten |
| x − 1 | Westen |
| y + 1 | Norden |
| y − 1 | Süden |

Danach zählt der Agent seine Drehungen selbst mit. Der Roboter meldet keine Himmelsrichtung.

**Abzweige deuten**

| Was `branches` meldet | Bedeutung |
| --- | --- |
| links, geradeaus und rechts | Knoten im Inneren |
| eine Richtung fehlt | Knoten am Rand |
| nur links oder nur rechts | Eckknoten |

**Beispiel**

Der Agent ruft `follow_line_until` auf und bekommt diese Bodenspur. Die Positionen gelten für eine Sensorleiste, die 5 cm vor der Achse sitzt.

```json
[[0,"wkkw"],[2.1,"rkkr"],[4.0,"wkkw"],[6.1,"gkkg"],[8.0,"wkkw"],
 [12.1,"gkkg"],[14.0,"wkkw"],[16.1,"ykky"],[18.0,"wkkw"],
 [24.1,"kkkk"],[26.0,"wkkw"]]
```

- Die äußeren Buchstaben zeigen die Farbfolge Rot, Grün, Grün, Gelb.
- Erste zwei: x = 0, y = 2. Der Roboter kam von (0,2).
- Letzte zwei: y = 2, x = 1. Er steht jetzt auf (1,2).
- x ist um 1 gewachsen, er blickt nach Osten.
- `kkkk` ist die Querlinie des Abzweigs.

**Was der Agent selbst führen muss**

- Position und Blickrichtung, am besten als Notiz mit `remember` nach jedem Abzweig.
- Eine Karte, wenn die Aufgabe sie braucht.
- Den Abgleich: Passen die gelesenen Farben nicht zur Erwartung, ist die eigene Annahme falsch. Dann neu bestimmen, statt weiterzurechnen.

## Sensorik

Der Quad-RGB-Sensor hat vier einzeln abfragbare Sonden. Die beiden mittleren folgen der Linie, die beiden äußeren lesen die Farbstreifen und erkennen Querlinien.

**Geprüfte Angaben des Herstellers**, gelesen am 22. November 2026 in der [API-Doku der mBuild-Module](https://education.makeblock.com/?p=63819) und im [Support-Artikel zum Sensor](https://support.makeblock.com/hc/en-us/articles/24279693845527-Quad-RGB-Sensor)

| Eigenschaft | Angabe |
| --- | --- |
| Sonden | L2, L1, R1, R2; L1 und R1 liegen in der Mitte |
| Kanalnamen in der API | `"L2"`, `"L1"`, `"R1"`, `"R2"` oder die Zahlen 4, 3, 2, 1 |
| Messabstand | 5 bis 15 mm über dem Untergrund |
| Benannte Farben | Rot, Gelb, Grün, Cyan, Blau, Lila, Weiß, Schwarz |
| Fülllicht | sichtbares Licht in Grün, Rot oder Blau; Farberkennung funktioniert unabhängig von der Lichtfarbe |
| Farbmodi | Standard und „Color enhanced“ auf HSV-Basis |
| Kalibrierung | auf weißem Papier ab A6, Sensor 12 bis 13 mm darüber, normales Raumlicht; setzt Firmware 011 voraus |
| Linienlernen | Doppeldruck auf die Sensortaste, dann über Linie und Hintergrund schwenken, etwa 2,5 Sekunden; Werte bleiben gespeichert |
| Nicht angegeben | Abstand der Sonden, empfohlene Linienbreite, Messrate |

**API je Sonde**

| Funktion | Ergebnis |
| --- | --- |
| `quad_rgb_sensor.get_red(ch)`, `get_green(ch)`, `get_blue(ch)` | je 0 bis 255 |
| `quad_rgb_sensor.get_gray(ch)` | 0 bis 100, Reflexion mit Fülllicht minus ohne |
| `quad_rgb_sensor.get_color_sta(ch)` | Farbname als Text |
| `quad_rgb_sensor.is_color(color, ch)` | wahr oder falsch |
| `quad_rgb_sensor.is_line(ch)`, `is_background(ch)` | wahr oder falsch, nach dem Linienlernen |
| `quad_rgb_sensor.get_line_sta()` | 0 bis 15, ein Bit je Sonde; 1 bedeutet Hintergrund |
| `quad_rgb_sensor.get_offset_track()` | −100 bis +100, Abweichung von der Linie |

Alle Funktionen nehmen zusätzlich `index = 1`. Die genaue Signatur von `is_color` ist in der Doku uneinheitlich und am Gerät zu prüfen.

**Hinweise des Herstellers, die das Design bestimmen**

- Farbige Abschnitte in der Linie und farbige Flächen neben der Linie können die Abweichungsmessung verfälschen.
- Bei farbigem Umfeld empfiehlt der Hersteller, für die Linie nur L1 und R1 zu verwenden.
- Eine Kreuzung lässt sich von einer Einmündung erst nach dem Überfahren unterscheiden.

**Eigene Farbklassifikation**

- Grundlage sind die RGB-Werte je Sonde, nicht die eingebauten Farbnamen.
- Für jede Sonde werden Referenzwerte für Weiß, Schwarz und die vier Bandfarben gespeichert.
- Klassifiziert wird nach dem nächstgelegenen Referenzwert. Verglichen wird der Farbton getrennt von der Helligkeit, damit Schatten weniger stören.
- Ist der Abstand zu allen Referenzen zu groß, lautet das Ergebnis `?`.
- Ein Wert gilt erst nach drei gleichen Messungen in Folge.
- Die Referenzwerte je Roboter speichert der Server und sendet sie beim Verbinden. Ob der CyberPi selbst dauerhaft speichern kann, ist offen.

**Linienfolgen**

- Regelung aus den Grauwerten von L1 und R1. Die eingebaute Abweichungsmessung wird nur verwendet, wenn ein Test zeigt, dass die Farbstreifen sie nicht stören.
- Ob das Linienlernen per Tastendruck nötig bleibt, wenn nur Rohwerte gelesen werden, ist zu prüfen.

## Grundfähigkeiten auf dem Roboter

Vier Skills laufen im nicht blockierenden Loop des Roboterskripts, als Zustandsautomaten wie im Haupt-Handoff. Keiner kennt das Gitter. Jeder lässt sich mit `stop` abbrechen.

Zwei Maße des Roboters kommen immer wieder vor: `o` ist der Abstand von der Sensorleiste zur Radachse, `D` der Abstand zwischen L2 und R2.

### 1. Bodenspur aufzeichnen

Die Spur läuft bei jeder Bewegung mit und ist die Rückgabe `floor`.

- In jedem Durchlauf werden die vier Sonden klassifiziert: `w` weiß, `k` schwarz, `r` rot, `y` gelb, `g` grün, `b` blau, `?` unklar.
- Das Muster sind vier Buchstaben in der Reihenfolge L2, L1, R1, R2.
- Ein neuer Eintrag entsteht, wenn das Muster wechselt und drei Messungen lang hält.
- Die Position kommt aus dem Mittel beider Encoder, in cm seit Beginn der Bewegung. Beim Drehen ist es der Winkel in Grad.
- Die Spur hat höchstens 20 Einträge. Danach wird `floor_truncated` gesetzt und die Bewegung läuft weiter.
- Der Skill deutet nichts. Er weiß nicht, was ein Streifen bedeutet.

### 2. Linie finden

Ziel: Aus beliebiger Lage so auf einer Linie stehen, dass L1 und R1 Schwarz sehen.

1. Geradeaus fahren, bis eine Sonde Schwarz sieht, höchstens `max_cm`.
2. Merken, auf welcher Seite die Linie zuerst getroffen wurde.
3. Um `o` weiterfahren, damit die Radachse über der Linie steht.
4. Auf der Stelle zur Trefferseite drehen, höchstens 180°, bis L1 und R1 Schwarz sehen.
5. Kurz nachregeln und anhalten.

- Farbe zählt nicht als Linie, nur Schwarz.
- Ohne Treffer endet der Skill mit `not_found`. Der Agent entscheidet, ob er dreht und es noch einmal versucht.
- Trifft der Roboter einen Abzweig, richtet er sich auf einem der Arme aus. Welcher es ist, ist ihm gleich.

### 3. Linie folgen

- Geregelt wird nur mit L1 und R1, aus den Grauwerten.
- L2 und R2 lesen mit und steuern nicht.
- Ein Abzweig liegt vor, wenn L2 oder R2 Schwarz sieht, bestätigt durch zwei Messungen.
- Am Abzweig fährt der Roboter um `o` weiter und hält mit der Radachse über der Kreuzung. Von dort kann er sauber drehen.
- `branches` entsteht so: links und rechts aus dem, was L2 und R2 beim Überfahren sahen, geradeaus daraus, ob L1 und R1 nach dem Vorrücken noch Schwarz sehen.
- Sehen alle vier Sonden über 2 cm nur Weiß, pendelt der Roboter einmal um ±20°. Findet er nichts, endet der Skill mit `line_lost`.
- Ein Hindernis vor dem Abstandssensor beendet den Skill mit `obstacle`.

### 4. Drehen über die Encoder

- Gedreht wird auf der Stelle, die Räder laufen gegenläufig.
- Der Weg je Rad ist Spurweite × π × Grad / 360. Für 90° ist das Spurweite × π / 4.
- Ein Korrekturfaktor je Roboter kommt aus der Kalibrierung.
- Mit `snap_to_line` sucht der Roboter nach der Drehung in ±15°, bis L1 und R1 Schwarz sehen. Die Rückgabe sagt, ob das gelang.
- Ohne `snap_to_line` bleibt es bei der reinen Encoder-Drehung.
- Auch beim Drehen läuft die Bodenspur mit. Über einem Abzweig zeigt sie, bei welchen Winkeln die Leiste Linien überstrichen hat.

### Zusammenspiel

- Eine typische Folge ist `find_line`, dann `follow_line_until` bis zum Abzweig, dann `turn` mit `snap_to_line`, dann wieder `follow_line_until`.
- Die Reflexe aus dem Haupt-Handoff gelten unverändert: Nothalt, Grenzen, Heartbeat-Wächter.
- Jeder Skill endet mit einer Rückgabe, auch bei Abbruch. Die bis dahin gelesene Spur geht nie verloren.

## Tools für den Agenten

Alle Tools sind generisch. Keines nimmt oder liefert Koordinaten, Knoten oder Himmelsrichtungen. Neu ist nur `find_line`. Die anderen stehen schon im Haupt-Handoff und bekommen die Rückgabe `floor`.

| Tool | Parameter | Rückgabe | Stand |
| --- | --- | --- | --- |
| `find_line` | `max_cm?` (45) | `reason`, `drove_cm`, `turned_deg`, `floor` | neu |
| `follow_line_until` | `max_cm`, `stop_on?` | `reason`, `drove_cm`, `branches?`, `floor` | erweitert |
| `drive` | `cm`, `speed?` | `reason`, `drove_cm`, `floor` | erweitert |
| `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `reason`, `drove_cm`, `floor` | erweitert |
| `turn` | `degrees`, `snap_to_line?` | `reason`, `turned_deg`, `snapped`, `floor` | erweitert |
| `read_sensors` | keine | wie bisher, dazu `floor` als ein Muster | erweitert |
| `stop` | keine | Stand der abgebrochenen Bewegung | unverändert |

- `stop_on` ist eine Liste aus `junction`, `color` und `line_end`. Ohne Angabe gilt `junction`.
- `stop_on_color` nimmt `black`, `red`, `yellow`, `green`, `blue` oder `any`.
- `turn` dreht bei positiven Werten nach links. Das Vorzeichen muss zum Haupt-Handoff passen.

**Beschreibungen für das Sprachmodell**

| Tool | Beschreibung |
| --- | --- |
| `find_line` | Sucht die nächste schwarze Linie. Fährt bis zu `max_cm` geradeaus, bis der Bodensensor Schwarz sieht, und richtet den Roboter auf der Linie aus. Nutze es, wenn du nicht auf einer Linie stehst oder sie verloren hast. Bei `not_found`: drehen und noch einmal versuchen. |
| `follow_line_until` | Folgt der schwarzen Linie bis zu einem Stoppgrund: Abzweig, Farbe, Linienende oder `max_cm`. An einem Abzweig hält der Roboter mit der Drehachse über der Kreuzung. `branches` sagt, in welche Richtungen Linien weitergehen. `floor` zeigt, was unterwegs am Boden lag. |
| `drive` | Fährt `cm` geradeaus, negativ rückwärts, ohne einer Linie zu folgen. `floor` zeigt, was der Bodensensor unterwegs sah. |
| `drive_until` | Fährt geradeaus, bis ein Hindernis näher als `stop_below_cm` ist, der Bodensensor die Farbe `stop_on_color` sieht oder `max_cm` erreicht sind. |
| `turn` | Dreht auf der Stelle um `degrees`, positiv nach links. Mit `snap_to_line` richtet sich der Roboter am Ende auf eine Linie aus, wenn eine in der Nähe liegt. Deine Blickrichtung musst du selbst mitzählen. |
| `read_sensors` | Liest alle Sensoren im Stand. `floor` ist das Muster unter den vier Sonden, von links nach rechts. |
| `stop` | Bricht die laufende Bewegung sofort ab. |

**Die Bodenspur `floor`**

- Eine Liste aus Paaren `[Position, Muster]`.
- Die Position zählt ab Beginn der Bewegung und gilt für die Sensorleiste: in cm beim Fahren, in Grad bei `turn`.
- Das Muster hat vier Buchstaben für die Sonden L2, L1, R1, R2, also von links nach rechts.
- Buchstaben: `w` weiß, `k` schwarz, `r` rot, `y` gelb, `g` grün, `b` blau, `?` unklar.
- Ein Eintrag gilt, bis der nächste beginnt.
- Höchstens 20 Einträge. Ist die Spur länger, steht `floor_truncated: true` in der Rückgabe. Der Agent fährt dann in kürzeren Schritten.

```json
{"reason":"junction","drove_cm":30.0,
 "branches":{"left":true,"ahead":true,"right":true},
 "floor":[[0,"wkkw"],[2.1,"rkkr"],[4.0,"wkkw"],[6.1,"gkkg"],[8.0,"wkkw"],
          [12.1,"gkkg"],[14.0,"wkkw"],[16.1,"ykky"],[18.0,"wkkw"],
          [24.1,"kkkk"],[26.0,"wkkw"]]}
```

**Werte für `reason`**

| Wert | Bedeutung | Bei welchem Tool |
| --- | --- | --- |
| `done` | Strecke oder Winkel erreicht | `drive`, `turn` |
| `found` | Linie gefunden und ausgerichtet | `find_line` |
| `not_found` | keine Linie innerhalb von `max_cm` | `find_line` |
| `junction` | Abzweig erreicht, Achse steht darüber | `follow_line_until` |
| `color` | gesuchte Farbe unter dem Sensor | `follow_line_until`, `drive_until` |
| `line_end` | Linie endet ohne Abzweig | `follow_line_until` |
| `line_lost` | Linie verloren, Pendeln half nicht | `follow_line_until` |
| `max_cm` | Höchststrecke erreicht | `follow_line_until`, `drive_until`, `find_line` |
| `obstacle` | Hindernis voraus | alle Fahr-Tools |
| `stopped` | durch `stop` oder Nothalt beendet | alle |

**Regeln**

- Ein Aufruf ist eine Strecke oder eine Drehung. Lange Ketten plant der Agent, nicht das Tool.
- Farben werden gemeldet, nie gedeutet.
- Jede Bewegung liefert die bis dahin gelesene Spur, auch bei Abbruch.
- Nur der Besitzer eines Roboters darf ihn fahren, wie im Haupt-Handoff.

**Nicht für Agenten, nur im Leitstand**

| Funktion | Zweck |
| --- | --- |
| `calibrate_colors` | Farbreferenzen je Sonde auf dem Referenzstreifen aufnehmen |
| `calibrate_turn` | Korrekturfaktor für Drehungen bestimmen |
| `calibrate_drive` | Korrekturfaktor für Strecken bestimmen |

Das ist Wartung am Roboter. Auch diese Funktionen kennen das Gitter nicht.

## Protokoll und Konfiguration

Das MQTT-Protokoll des Haupt-Handoffs bleibt, wie es ist. Dazu kommen ein Befehl, zwei Argumente und ein Feld in der Rückgabe.

**Änderungen auf `cmd` und `result`**

| Befehl | Neu |
| --- | --- |
| `find_line` | neuer Befehl mit `max_cm` |
| `follow_line_until` | Argument `stop_on`, Rückgabe `branches` |
| `turn` | Argument `snap_to_line`, Rückgabe `snapped` |
| alle Fahrbefehle | Rückgabe `floor`, bei Bedarf `floor_truncated` |

**Beispiel**

```json
// cmd
{"id":"c-41","name":"follow_line_until","args":{"max_cm":40,"stop_on":["junction"]}}

// result
{"id":"c-41","ok":true,"reason":"junction",
 "data":{"drove_cm":30.0,
         "branches":{"left":true,"ahead":true,"right":true},
         "floor":[[0,"wkkw"],[2.1,"rkkr"],[4.0,"wkkw"],[6.1,"gkkg"],[8.0,"wkkw"],
                  [12.1,"gkkg"],[14.0,"wkkw"],[16.1,"ykky"],[18.0,"wkkw"],
                  [24.1,"kkkk"],[26.0,"wkkw"]]}}
```

- Ohne Leerzeichen sind das rund 330 Byte. Die Grenze von 512 Byte aus dem Haupt-Handoff hält, solange die Spur bei 20 Einträgen endet.
- Positionen haben eine Nachkommastelle.

**Erweiterung von `state`**

```json
{"skill":"follow_line_until","distance_cm":34,"battery":81,
 "floor":"wkkw","seq":205,"t_ms":88400}
```

- `floor` ist das aktuelle Muster unter den vier Sonden.
- Es gibt kein Feld für Position oder Richtung.

**Ereignisse**

Es kommen keine neuen Ereignisse dazu. Alles, was der Agent über den Boden erfährt, steht in den Rückgaben der Tools.

**Konfiguration**

| Wert | Ort | Zweck |
| --- | --- | --- |
| `o`, `D`, Spurweite, Raddurchmesser | je Roboter | Vorrücken am Abzweig, Drehwinkel, Strecke |
| Korrekturfaktoren für Drehung und Strecke | je Roboter | Ergebnis der Kalibrierung |
| Farbreferenzen je Sonde | je Roboter | Klassifikation |
| Geschwindigkeiten, Bestätigungen, Suchwinkel | Roboterskript | Verhalten der Skills |

Der Server speichert die Werte je Roboter und sendet sie beim Verbinden, wie im Haupt-Handoff. Es sind Eigenschaften des Roboters, nicht des Feldes.

**Was der Server nicht tut**

- Er hat keine Konfiguration für das Feld: kein Rastermaß, keine Knotenzahl, kein Farbcode.
- Er führt keine Position und keine Blickrichtung.
- Er plant keine Wege.
- Er deutet keine Farben.
- Er reicht Tool-Aufrufe durch, prüft Besitz und Budgets und schreibt das Protokoll.

**Leitstand**

- Er zeigt Aufrufe und Rückgaben wie bisher.
- Die Spur `floor` darf er als Farbband zeichnen, Buchstabe für Buchstabe. Das ist reine Darstellung und braucht kein Wissen über das Feld.
- Er zeigt keine Karte und keine Position. Wo ein Roboter steht, sieht man im Raum.
- Das Roboter-Display zeigt, was der Agent mit `show_text` schreibt. Will ein Team dort die vermutete Position sehen, lässt es den Agenten sie anzeigen.

**Mehrere Roboter auf dem Feld**

- Vor Zusammenstößen schützt der Nothalt per Ultraschall. Ein blockierter Roboter meldet `obstacle`.
- Wer ausweicht, klären die Agenten untereinander mit `say`. Der Server schlichtet nicht.

## Verwendung

Der Agent arbeitet in kleinen Schritten: Tool aufrufen, Spur lesen, Schluss ziehen, notieren. Was er über das Feld weiß, steht in seinem Prompt oder in seinen Notizen.

**Wissensstufen**

| Stufe | Was im Prompt steht | Passende Aufgaben |
| --- | --- | --- |
| 0 | nichts über den Boden | erkunden und beschreiben |
| 1 | Aufbau: Gitter aus Linien, Abzweige, Rastermaß | im Kreis fahren, den Rand finden, die Größe bestimmen |
| 2 | zusätzlich Farbcode und Leseregeln | Position bestimmen, Ziele anfahren |
| 3 | zusätzlich ein Auftrag mit Koordinaten | Logistik, Treffpunkte, Wettlauf |

Die Stufe wählt das Team oder die Lehrkraft über den Prompt. Im Server gibt es dafür keine Einstellung.

**Erkundungsaufgaben**

1. Finde eine Linie und beschreibe, was der Sensor sieht.
2. Folge der Linie bis zum nächsten Abzweig. Was lag unterwegs?
3. Teste, wie man im Kreis fährt. Woran erkennst du, dass du wieder am Start bist?
4. Teste, wie man an den Rand kommt. Woran erkennst du den Rand?
5. Wie groß ist das Feld? Wie viele Abzweige hat es in jeder Richtung?
6. Was bedeuten die Farben? Stelle eine Vermutung auf und prüfe sie.
7. Wo stehst du gerade, und wohin blickst du?
8. Fahre zu (2,3).
9. Zeichnet zu zweit eine Karte. Tauscht eure Funde über `say` aus.
10. Die Lehrkraft setzt den Roboter um. Merkt der Agent es, und findet er sich wieder?

**Typische Abläufe**

| Situation | Folge der Tool-Aufrufe |
| --- | --- |
| Start an beliebiger Stelle | `find_line` → `follow_line_until` → Spur lesen |
| Position bestimmen | bis zum Abzweig folgen, letzte zwei Farben lesen, ein Streckenstück weiter, Richtung ableiten |
| Abbiegen | am Abzweig `turn(90, snap_to_line)` oder `turn(-90, snap_to_line)`, dann `follow_line_until` |
| Umkehren | `turn(180, snap_to_line)` |
| Rückgabe `line_lost` oder `not_found` | `find_line`, bei Bedarf vorher `turn(90)` |
| Rückgabe `obstacle` | warten oder über `say` mit dem anderen Agenten klären |
| Farben passen nicht zur Erwartung | eigene Position verwerfen und neu bestimmen |

**Prompt-Baustein, Beispiel für Stufe 2**

```
# Feldwissen
Der Boden ist ein Gitter aus schwarzen Linien mit 4 x 4 Kreuzungen im Abstand von 30 cm.
Farbstreifen sind Ziffern: rot 0, gelb 1, grün 2, blau 3.
Nach einer Kreuzung kommen zwei Streifen: erst x, dann y der Kreuzung, die du verlässt.
Vor einer Kreuzung kommen zwei Streifen: erst y, dann x der Kreuzung, die du erreichst.
(0,0) ist unten links, x wächst nach Osten, y nach Norden.

# Arbeitsweise
Du kennst deine Position nicht. Kein Tool sagt sie dir.
Lies nach jeder Fahrt das Feld floor. Die äußeren Buchstaben zeigen die Streifenfarbe.
Notiere nach jeder Kreuzung Position und Blickrichtung mit remember.
Zähle jede Drehung mit: links +90, rechts -90.
Passt eine Farbe nicht zu deiner Erwartung, bestimme deine Position neu.
```

Teams schreiben ihren eigenen Baustein. Wie gut er ist, entscheidet über den Erfolg. Das ist der Lerninhalt.

**Was damit im Lab möglich wird**

| Szenario | Idee |
| --- | --- |
| Entdecker | Agenten ohne Feldwissen erkunden; gewertet wird die beste Beschreibung des Feldes |
| Navigator und Fahrer | ein Agent hat das Feldwissen, der andere den Roboter; sie reden über `say` |
| Treffpunkt ohne Chef | Agenten einigen sich auf eine Kreuzung und fahren hin |
| Lagerlogistik | Aufträge der Form „hole bei (0,3), liefere nach (3,1)“ |
| Kartierung zu zweit | zwei Agenten teilen sich das Feld und führen ihre Notizen zusammen |
| Versetzter Roboter | wer merkt am schnellsten, dass seine Position nicht mehr stimmt? |
| Modell-Rennen | dieselbe Aufgabe mit verschiedenen Sprachmodellen; gewertet werden Zeit und Tool-Aufrufe |

**Stolpersteine für das Sprachmodell**

- Buchhaltung: Position und Blickrichtung über viele Schritte zu führen, geht ohne Notizen schief.
- Leserichtung: Die Regeln für „nach dem Abzweig“ und „vor dem Abzweig“ werden leicht vertauscht.
- Unklare Muster: Ein `?` in der Spur ist ein Grund, langsamer oder noch einmal zu fahren, nicht zu raten.
- Zu große Schritte: Wer `max_cm` hoch setzt, bekommt eine abgeschnittene Spur.

## Simulator

Alles muss im Simulator laufen, bevor es an die Hardware geht. Er führt das echte Roboterskript aus. Er ist die einzige Stelle im Repo, die die Geometrie des Feldes kennt, und auch er nur als Zeichnung.

**Welt**

- Eine Weltdatei beschreibt den Boden als Liste von Bändern: Anfang, Ende, Breite, Farbe, Ebene. Farbstreifen liegen unten, schwarze Linien darüber.
- Die Datei kennt keine Knoten und keine Koordinaten des Gitters, nur Zentimeter auf der Unterlage.
- Ein kleines Hilfsskript erzeugt die Weltdatei für das Gitter aus Rastermaß, Knotenzahl, Streifenabständen und Farbcode. Dasselbe Skript gibt den Bauplan als SVG aus.
- Andere Felder sind andere Weltdateien: ein Oval, ein Labyrinth, ein Gitter mit Lücken.
- Der Server lädt die Weltdatei nie.

**Roboter**

- Pose aus Position und Winkel, Differentialantrieb mit Spurweite und Raddurchmesser.
- Vier Sondenpositionen relativ zur Pose, bestimmt durch `o`, `D` und den Abstand zwischen L1 und R1.
- Encoder-Winkel je Rad aus der simulierten Bewegung.
- Die wahre Pose kennt nur der Simulator. Das Roboterskript bekommt sie nicht.

**Sensorattrappe**

- `quad_rgb_sensor.get_red`, `get_green`, `get_blue`, `get_gray`, `get_color_sta`, `is_line`, `get_line_sta` je Sonde, gespeist aus der Welt.
- Jede Fläche hat einen RGB-Sollwert. An Kanten wird anteilig gemischt, weil eine Sonde eine Fläche sieht und keinen Punkt.

**Störungen, einzeln zuschaltbar**

| Störung | Zweck |
| --- | --- |
| Rauschen auf den RGB-Werten | prüft Klassifikation und Dreifach-Bestätigung |
| Farbstich und Helligkeit je Roboter | prüft die Kalibrierung gegen Referenzwerte |
| Schlupf und ungleiche Räder | prüft Encoder-Drehung und Einrasten |
| falsche Spurweite | prüft `calibrate_turn` |
| fehlender oder falscher Streifen | prüft, ob der Agent den Widerspruch bemerkt |
| Roboter wird versetzt | prüft `find_line` und das Neubestimmen durch den Agenten |
| schwankende Schleifenzeit | prüft die Spur bei wenigen Messungen je Streifen |

**Ansicht**

- Eine einfache Draufsicht genügt: Boden, Roboter, Sonden, wahre Lage.
- Sie ist ein Werkzeug für Entwicklung und Tests und gehört nicht zum Leitstand.
- Die Zufallsquelle ist über einen Startwert festlegbar, damit Tests wiederholbar sind.

## Parameter und Kalibrierung

Alle Zahlen sind Startwerte. Kein Wert ist am Roboter erprobt. Die Geometrie stammt aus Überlegung, die Robotermaße sind zu messen.

**Feld**

Diese Werte braucht nur, wer das Feld baut, und das Hilfsskript für die Weltdatei. Roboter, Server und Leitstand kennen sie nicht.

| Parameter | Startwert | Herkunft |
| --- | --- | --- |
| Rastermaß | 30 cm | Entwurf |
| Linienbreite | 1,9 cm | Breite des Isolierbands |
| Mitte x-Streifen ab Knotenmitte | 8 cm | Entwurf |
| Mitte y-Streifen ab Knotenmitte | 12 cm | Entwurf |
| Streifenbreite in Fahrtrichtung | 1,9 cm | Breite des Isolierbands |
| Streifenlänge quer zur Linie | 10 cm | Entwurf, muss L2 und R2 sicher abdecken |
| Rand um das Gitter | 15 cm | Entwurf |

**Roboter, zu messen**

| Parameter | Bedeutung | Bedingung |
| --- | --- | --- |
| `o` | Sensorleiste bis Radachse | kleiner als 7 cm, sonst Streifen weiter nach außen setzen |
| `D` | Abstand L2 bis R2 | deutlich kleiner als die Streifenlänge, größer als die Linienbreite |
| Abstand L1 bis R1 | Lage der mittleren Sonden | beide müssen auf der Linie Platz haben oder sie knapp einrahmen |
| Raddurchmesser | Umrechnung Encoder-Winkel in Strecke | je Roboter |
| Spurweite | Umrechnung Radweg in Drehwinkel | je Roboter, über `calibrate_turn` verfeinert |
| Schleifenzeit | Messungen pro Zentimeter | Ziel höchstens 30 ms |

**Fahren und Lesen**

| Parameter | Startwert |
| --- | --- |
| Geschwindigkeit beim Linienfolgen | 10 cm/s |
| Geschwindigkeit bei `drive` ohne Linie | bis 20 cm/s; die Spur wird dann gröber |
| Suchfahrt in `find_line` | 10 cm/s, höchstens 45 cm |
| Drehgeschwindigkeit, grob und fein | 90°/s und 30°/s |
| Einrasten nach der Drehung | ±15° |
| Pendeln bei Linienverlust | ±20° nach 2 cm ohne Linie |
| Bestätigung eines Musters | 3 gleiche Messungen in Folge |
| Bestätigung eines Abzweigs | 2 Messungen |
| Länge der Bodenspur | höchstens 20 Einträge |

**Abhängigkeiten zwischen den Werten**

- Der innere Streifen beginnt bei 8 cm minus halber Bandbreite, also bei 7,05 cm. Nach dem Halt über einem Abzweig steht die Sensorleiste bei `o`. Deshalb muss `o` kleiner als 7 cm sein. Dann überstreicht die Leiste beim Drehen auch keine Streifen.
- Zwischen den äußeren Streifen zweier Nachbarknoten liegen 30 − 12 − 12 = 6 cm. Bei kleinerem Raster rücken sie zusammen und werden schwer trennbar.
- Bei 10 cm/s und 30 ms Schleifenzeit fallen rund 6 Messungen auf einen Streifen. Drei davon braucht die Bestätigung.
- Die Suchstrecke von `find_line` deckt die Diagonale einer Gitterzelle ab: 30 cm × Wurzel 2, aufgerundet. Auf einem anderen Feld wählt der Agent `max_cm` selbst.

**Kalibrierabläufe**

1. **Sensor:** Herstellerkalibrierung auf Weiß, einmal je Roboter und Raum.
2. **Farben:** `calibrate_colors` auf dem Referenzstreifen. Der Roboter fährt über Weiß, Schwarz, Rot, Gelb, Grün und Blau und meldet die RGB-Mittelwerte je Sonde.
3. **Drehung:** `calibrate_turn` auf einer Linienkreuzung. Viermal 90° drehen, aus den Korrekturwinkeln die wirksame Spurweite berechnen.
4. **Strecke:** `calibrate_drive` zwischen zwei Querlinien mit bekanntem Abstand, den die Lehrkraft eingibt. Das korrigiert den Raddurchmesser.

Der Server speichert alle Kalibrierwerte je Roboter und sendet sie beim Verbinden.

**Referenzstreifen**

- Am Feldrand, außerhalb des Rahmens: sechs Felder nebeneinander, je 5 cm lang und 10 cm breit, in der Reihenfolge Weiß, Schwarz, Rot, Gelb, Grün, Blau.
- Eine schwarze Führungslinie ist nicht nötig. Der Roboter wird gerade davor aufgestellt und fährt eine feste Strecke.

## Tests und Abnahme

Die drei Kernfähigkeiten haben messbare Ziele. Im Simulator müssen sie mit zugeschalteten Störungen erreicht werden, an der Hardware auf dem echten Feld.

| Fähigkeit | Versuch | Ziel im Simulator | Ziel an der Hardware |
| --- | --- | --- | --- |
| Linie wiederfinden | 100 zufällige Posen im Rahmen, an der Hardware 20 | 100 von 100 | mindestens 19 von 20 |
| Bodenspur lesen | 500 Streifen beim Linienfolgen, an der Hardware 200 | kein Fehler ohne Störung, höchstens 1 % mit Rauschen | höchstens 2 Fehllesungen |
| Abzweig erkennen | alle 16 Kreuzungen aus allen Richtungen | alle erkannt, `branches` korrekt | alle erkannt, `branches` korrekt |
| Drehen am Abzweig | 40 Drehungen, gemischt 90° und 180°, mit `snap_to_line` | alle enden auf der Linie | mindestens 38 ohne Einrasten über 10°, alle auf der Linie |
| Rundfahrt | 30 Streckenstücke mit 10 Drehungen | kein `line_lost` | höchstens ein `line_lost`, danach mit `find_line` erholt |
| Alle Roboter | jede Prüfung mit allen fünf Geräten | entfällt | gleiche Ziele nach Kalibrierung je Gerät |

**Reichen die Rückgaben zum Orientieren?**

- Ein Referenz-Auswerter im Testcode wendet die Leseregeln auf die Bodenspuren an. Er startet auf jedem Streckenstück in beiden Richtungen und muss Position und Richtung nach höchstens zwei Abzweigen richtig bestimmen.
- Er beweist, dass ein Agent aus den Rückgaben allein seine Position ableiten kann.
- Er lebt nur im Testverzeichnis des Simulators. Er wird nie Teil von Roboterskript, Server oder Leitstand.

**Gitterfreiheit**

- Ein Test durchsucht Roboterskript, Server und Leitstand nach Gitterbegriffen: Knoten, Koordinaten, Rastermaß, Farbcode. Er darf nichts finden.
- Dieselben Skills müssen auf einer zweiten Weltdatei laufen, etwa einem Oval mit einem Abzweig. Das zeigt, dass nichts auf das Gitter zugeschnitten ist.

**Regeln, die eigene Tests brauchen**

- `stop` wirkt in jedem Skill innerhalb von 100 ms.
- Jede Bewegung liefert ihre Spur, auch nach `stop`, `obstacle` und `line_lost`.
- Eine Spur mit mehr als 20 Einträgen wird abgeschnitten und als abgeschnitten gemeldet.
- Ein Muster, das kürzer als drei Messungen anliegt, erscheint nicht in der Spur.
- An einem Abzweig ohne Linie geradeaus meldet `branches` für `ahead` den Wert `false`.
- `turn` mit `snap_to_line` meldet `snapped: false`, wenn im Suchwinkel keine Linie liegt, und dreht dann auf den Encoder-Winkel zurück.

**Tests mit Sprachmodell**

- Ein Agent mit Modell-Double löst im Simulator mit dem Prompt-Baustein der Stufe 2: Start an zufälliger Stelle, fahre zu (3,3).
- Die Erkundungsaufgaben 1 bis 8 laufen mit dem echten Modell je fünfmal. Gemessen werden Erfolgsquote, Tool-Aufrufe und Tokens.
- Scheitert das Modell regelmäßig, werden zuerst Tool-Beschreibungen und Spurformat verbessert. Wissen über das Gitter wandert nicht in die Tools.

## Arbeitsschritte für Claude Code

Die Schritte bauen auf den Meilensteinen M3 und M4 des Haupt-Handoffs auf: Simulator und Bewegungs-Skills müssen stehen. Lege dieses Dokument als `docs/field.md` ins Repo.

| Nr. | Inhalt | Abnahme |
| --- | --- | --- |
| G0 | Messungen an echter Hardware, von Daniel von Hand: `o`, `D`, Abstand L1 bis R1, Schleifenzeit, RGB-Werte aller Bandfarben, Hellblau gegen Schwarz | Werte stehen in der Roboter-Konfiguration, offene Punkte sind abgehakt |
| G1 | Weltdatei aus Bändern, Hilfsskript für das Gitter mit SVG-Bauplan, Sensorattrappe mit vier Sonden | Der Bauplan zeigt das Feld maßstabsgetreu; eine Sonde liefert an bekannten Punkten die erwartete Farbe |
| G2 | Farbklassifikation mit Referenzwerten, `calibrate_colors` | Klassifikation besteht den Rauschtest |
| G3 | Bodenspur aufzeichnen, `floor` in `drive`, `drive_until` und `read_sensors` | Spurtest im Simulator besteht |
| G4 | Linie folgen mit L1 und R1, Abzweig erkennen, über dem Abzweig anhalten, `branches` | Abzweigtest im Simulator besteht |
| G5 | Drehen über Encoder mit `snap_to_line`, `calibrate_turn`, `calibrate_drive` | Drehtest besteht auch mit falscher Spurweite nach der Kalibrierung |
| G6 | `find_line` | Test mit 100 zufälligen Posen besteht |
| G7 | Protokoll und Tool-Registry im Server erweitern, Tool-Beschreibungen, Farbband für `floor` im Leitstand | Agent mit Modell-Double fährt von zufälliger Stelle zu (3,3); Test auf Gitterfreiheit besteht |
| G8 | Referenz-Auswerter im Testcode, zweite Weltdatei, Erkundungsaufgaben mit echtem Modell | Auswerter bestimmt jede Position richtig; Messwerte je Aufgabe liegen vor |
| G9 | Erprobung auf dem echten Feld, Parameter nachziehen | Hardware-Ziele aus dem Abschnitt Tests sind erreicht |

**Hinweise**

- G0 kann sofort beginnen und läuft parallel. Bis die Messwerte vorliegen, arbeitet der Simulator mit plausiblen Annahmen, die in einer Datei stehen und leicht zu ändern sind.
- Feldgeometrie steht nur in der Weltdatei und im Hilfsskript dafür. Roboterskript, Server und Leitstand lesen sie nie.
- Kein Tool, kein Skill und kein Protokollfeld heißt nach Knoten, Koordinaten oder Gitter. Im Zweifel ist der generische Name der richtige.
- Die Tool-Beschreibungen sind Teil des Produkts. Das Sprachmodell lernt die Tools nur aus ihnen. Änderungen daran werden mit den Erkundungsaufgaben geprüft.
- Die Leseregeln gehören in die Unterlagen für Teams und in Beispiel-Prompts, nicht in den Code. Lege sie mit dem Bauplan unter `docs/` ab.
- Für die Hardware-Messungen in G0 schreibe kurze Testskripte, die Werte auf dem Display und per MQTT ausgeben, mit einer Anleitung, was zu messen ist.
- Passe `docs/protocol.md` und `docs/tools.md` an und trage die geprüften Sensor-APIs dort ein, wo der Haupt-Handoff sie noch als ungeprüft führt.

## Offene Punkte und Referenzen

Nichts in diesem Dokument ist am Roboter erprobt. Die folgenden Fragen entscheiden, ob Maße oder Verfahren geändert werden müssen.

**Am Roboter und auf dem Feld zu klären**

- [ ] Wie groß sind `o`, `D` und der Abstand L1 bis R1? Passen sie zu 8 und 12 cm Streifenabstand und 10 cm Streifenlänge?
- [ ] Passt 19 mm Band als Linie zu den mittleren Sonden, oder muss die Linie breiter sein?
- [ ] Unterscheidet der Sensor Hellblau sicher von Schwarz und Gelb sicher von Weiß?
- [ ] Spiegelt Isolierband das Fülllicht so stark, dass die Farbwerte kippen? Alternative wäre mattes Papier oder Gewebeband.
- [ ] Stört die Bandkante, wo Schwarz über Farbe liegt, als kleine Stufe den Sensorabstand oder die Fahrt?
- [ ] Welche Schleifenzeit erreicht das Skript, wenn es vier Sonden mit je drei Farbwerten liest? Reichen die Messungen pro Streifen bei 10 cm/s?
- [ ] Wie genau dreht der Roboter über Encoder auf dem Karton, und wie stark streut das zwischen den fünf Geräten?
- [ ] Lassen sich die Encoder-Winkel während der Fahrt ohne spürbare Verzögerung lesen?
- [ ] Bleibt das Linienlernen per Tastendruck nötig, wenn nur Rohwerte gelesen werden?
- [ ] Welcher Farbmodus ist besser, Standard oder „Color enhanced“?
- [ ] Kann der CyberPi Kalibrierwerte dauerhaft speichern?

**Fachlich offen**

- [ ] Reichen 4 × 4 Knoten für fünf Roboter, oder soll gleich mit Lila und Cyan auf 6 × 6 geplant werden?
- [ ] Kommt ein günstiges Sprachmodell mit dem Spurformat und der Buchhaltung über Position und Richtung zurecht?
- [ ] Braucht `floor` eine Kurzfassung, die nur die Farbfolge der äußeren Sonden nennt?
- [ ] Passt das Vorzeichen von `turn`, positiv nach links, zum Haupt-Handoff?
- [ ] Soll der Haupt-Handoff um `find_line`, `floor` und die geprüften Sensor-APIs ergänzt werden?

**Referenzen**

| Thema | Quelle |
| --- | --- |
| API der mBuild-Module, Abschnitt Quad RGB Sensor | [Makeblock Help Center](https://education.makeblock.com/?p=63819) |
| Sensor: Messabstand, Kalibrierung, Linienlernen, Szenarien | [Makeblock Support](https://support.makeblock.com/hc/en-us/articles/24279693845527-Quad-RGB-Sensor) |
| Sensor: Kanalnamen, eigene Farben mit Toleranz, Grenzen | [Makeblock Support](https://support.makeblock.com/hc/en-us/articles/12545566572695-About-Quad-RGB-Sensor) |
| Fahrbefehle des mBot2-Shields | [mBot2 Python-Handbuch, PDF](https://d4iqe7beda780.cloudfront.net/resources/static/main/pdf/mb10132_python_instruction_manual.pdf) |

Die drei Makeblock-Seiten wurden am 22. November 2026 gelesen. Das Python-Handbuch stammt aus einem Suchtreffer und ist für die Encoder-Funktionen noch zu prüfen.

# Linienfeld

Ein Gitter aus schwarzen Linien mit farbigen Querstreifen. Die Roboter wissen nichts davon: Sie
fahren, drehen und melden, was ihre vier Bodensonden gesehen haben. Was die Farben bedeuten,
steht nur hier und in den Prompts der Teams.

Bauplan im Maßstab 1:1: [`bauplan.svg`](bauplan.svg). Er wird mit der Simulator-Welt aus
`sim/grid.py` erzeugt; Änderungen am Feld bitte dort machen.

## Bauen

| Teil | Maß |
| --- | --- |
| Grundfläche | 120 × 120 cm, weiß und matt |
| Kreuzungen | 4 × 4, Abstand 30 cm, Rand 15 cm |
| Linien | schwarzes Isolierband, 19 mm, Außenrahmen geschlossen |
| Querstreifen | farbiges Isolierband, 19 mm breit, 10 cm lang, mittig quer unter der Linie |
| Lage der Streifen | Mitte bei 8 cm und 12 cm ab Kreuzungsmitte, auf jedem Arm |
| Referenzstreifen | eigener Karton neben dem Feld, sechs Felder je 5 × 10 cm: Weiß, Schwarz, Rot, Gelb, Grün, Blau |

Material: 7,2 m schwarzes Band, je 2,0 m Rot und Blau, je 2,8 m Gelb und Grün (96 Streifen).

1. Die 16 Kreuzungen anzeichnen.
2. Auf jedem Arm die Streifenmitten bei 8 und 12 cm markieren.
3. Die Farbstreifen quer zur späteren Linie kleben.
4. Die schwarzen Linien durchgehend über die Streifen kleben.
5. Den Referenzstreifen auf einen eigenen Karton kleben. Auf der Unterlage ist dafür kein Platz:
   Die Streifen am Außenrahmen ragen 5 cm in den Rand.

Kontrolle: Jedes Stück zwischen zwei Kreuzungen hat genau vier Streifen. Alle Arme einer
Kreuzung zeigen dieselbe Farbfolge.

## Farbcode und Leseregeln

Dieses Wissen bekommt ein Agent nur, wenn das Team es in den Prompt schreibt.

| Farbe | Ziffer | Buchstabe in der Bodenspur |
| --- | --- | --- |
| Rot | 0 | `r` |
| Gelb | 1 | `y` |
| Grün | 2 | `g` |
| Blau | 3 | `b` |

- Der innere Streifen (8 cm) trägt x der Kreuzung, der äußere (12 cm) trägt y.
- Die ersten zwei Farben nach einer Kreuzung gehören zur Kreuzung, die man verlässt: erst x, dann y.
- Die letzten zwei Farben vor einer Kreuzung gehören zur Kreuzung, die man erreicht: erst y, dann x.
- (0,0) liegt unten links, x wächst nach Osten, y nach Norden.

Beispiel: Ein Roboter fährt von der Kreuzung (0,2) nach Osten. Die Bodenspur:

```json
[[0,"wkkw"],[2.1,"rkkr"],[4.0,"wkkw"],[6.1,"gkkg"],[8.0,"wkkw"],
 [12.1,"gkkg"],[14.0,"wkkw"],[16.1,"ykky"],[18.0,"wkkw"],[24.1,"kkkk"],[26.0,"wkkw"]]
```

Die äußeren Buchstaben ergeben Rot, Grün, Grün, Gelb. Erste zwei: x = 0, y = 2, er kam von (0,2).
Letzte zwei: y = 2, x = 1, er steht jetzt auf (1,2). `kkkk` ist die Querlinie der Kreuzung.

Am Rand fehlt eine Seite der Querlinie: Dort meldet die Spur z. B. `kkkw` statt `kkkk`.

## Wissensstufen

| Stufe | Was im Prompt steht | Aufgaben |
| --- | --- | --- |
| 0 | nichts über den Boden | erkunden und beschreiben |
| 1 | Aufbau: Gitter aus Linien, Kreuzungen, Abstand | den Rand finden, die Größe bestimmen |
| 2 | dazu Farbcode und Leseregeln | Position bestimmen, Ziele anfahren |
| 3 | dazu ein Auftrag mit Koordinaten | Logistik, Treffpunkte, Wettlauf |

Ein Beispiel für Stufe 2 steht in [`../prompts/de/linienfeld.md`](../prompts/de/linienfeld.md).

Hinweis: Linie folgen, Kreuzungen erkennen und `find_line` kommen erst mit den nächsten Schritten
(G4 bis G6). Bis dahin fährt der Agent mit `drive` und `turn` von Kreuzung zu Kreuzung.

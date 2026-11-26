---
title: Linienfeld – fahr zur Kreuzung (2,3)
---

## Aufgabe

Auf dem Boden liegt ein Gitter aus schwarzen Linien mit 4 × 4 Kreuzungen, 30 cm voneinander
entfernt. Zwischen den Kreuzungen kleben Farbstreifen. Sie sind ein Code:

- rot = 0, gelb = 1, grün = 2, blau = 3
- Nach einer Kreuzung kommen zwei Streifen: erst x, dann y der Kreuzung, die man verlässt.
- Vor einer Kreuzung kommen zwei Streifen: erst y, dann x der Kreuzung, die man erreicht.
- (0,0) ist unten links. x wächst nach Osten, y nach Norden.

Euer Roboter startet an einer Kreuzung, die er nicht kennt. **Bringt ihn zur Kreuzung (2,3).**
Dort meldet er sich mit `finish`.

Kein Werkzeug sagt dem Agenten, wo er ist. Er sieht nur, welche Farben unter dem Roboter
vorbeigezogen sind. Wie er daraus seine Position berechnet, muss in eurem Prompt stehen.

## Startprompt

# Feldwissen

Der Boden ist ein Gitter aus schwarzen Linien mit 4 x 4 Kreuzungen im Abstand von 30 cm.
Farbstreifen sind Ziffern: rot 0, gelb 1, grün 2, blau 3.
Nach einer Kreuzung kommen zwei Streifen: erst x, dann y der Kreuzung, die du verlässt.
Vor einer Kreuzung kommen zwei Streifen: erst y, dann x der Kreuzung, die du erreichst.
(0,0) ist unten links, x wächst nach Osten, y nach Norden.

# Arbeitsweise

Du kennst deine Position nicht. Kein Tool sagt sie dir.
Lies nach jeder Fahrt das Feld `floor`. Die äußeren Buchstaben zeigen die Streifenfarbe.
Notiere nach jeder Kreuzung Position und Blickrichtung mit `remember`.
Führe die Blickrichtung als Kompasswinkel: Norden 0, Osten 90, Süden 180, Westen 270.
Eine Drehung nach rechts addiert, nach links zieht ab.
Passt eine Farbe nicht zu deiner Erwartung, bestimme deine Position neu.

# Auftrag

Fahre zur Kreuzung (2,3) und melde dich dort mit `finish`.

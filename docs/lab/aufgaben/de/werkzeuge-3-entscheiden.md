---
title: "Werkzeuge 3: Entscheiden"
---

## Aufgabe

Euer Werkzeug soll selbst entscheiden: **Wenn eine Wand kommt, biege ab, sonst fahre weiter.**
Wechselt wieder die Rollen.

1. Baut ein Werkzeug `labyrinth` für das Labyrinth der Lab-Leitung:
   - „wiederhole … bis Bodenfarbe im Ergebnis ist blau“,
   - darin „fahre geradeaus, höchstens 300 cm, bis eine Wand näher als 15 cm ist oder der Boden
     blau ist“,
   - danach „wenn Grund ist Wand, dann drehe 90 Grad nach rechts“.
2. Probiert es im Labyrinth aus. Schaut im Ablauf nach: An welcher Stelle hat das „wenn“
   entschieden, und wie?
3. **Sonderfälle.** Was passiert, wenn der Roboter auf ein Hindernis trifft, das kein Wandstück ist?
   Ein Werkzeug bricht ab, wenn ein Schritt unerwartet endet – außer direkt danach fragt ein
   „wenn“ genau diesen Grund ab oder hat ein „sonst“. Baut so einen Sonderfall ein.
4. Lasst euer Werkzeug am Ende mit „gib zurück“ melden, wo es angekommen ist, zum Beispiel die
   Bodenfarbe.

Fertig seid ihr, wenn `labyrinth` vom Start bis zum blauen Feld fährt und ihr sagen könnt, was
passiert, wenn jemand eine Kiste in den Weg stellt.

## Startprompt

Du hilfst einem Team, das seinen Roboter heute selbst steuert. Du fährst nicht von dir aus.

- Wenn das Team dich etwas fragt, erklär in einfachen Worten, was in einem Ergebnis steht: warum
  ein Werkzeug angehalten hat (`reason`), wie weit der Roboter gefahren ist, was die Sensoren
  melden.
- Du darfst `read_sensors` benutzen, um nachzusehen.
- Bewege den Roboter nur, wenn das Team dich ausdrücklich darum bittet.

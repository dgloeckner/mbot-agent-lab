---
title: "Werkzeuge 2: Wiederholen"
---

## Aufgabe

Jetzt baut ihr euer erstes **eigenes Werkzeug** aus Blöcken. Wechselt die Rollen: Wer eben
beobachtet hat, steuert jetzt.

Links unter „Prompt“ findet ihr **„Werkzeug bauen“**. Klickt auf „Neues Werkzeug“. Jedes Werkzeug
beginnt mit dem Block „Wenn das Werkzeug startet“; alles, was darunter hängt, läuft der Reihe
nach ab.

1. **Fahrt ein Quadrat.** Nennt das Werkzeug `quadrat`. Nehmt aus „Wiederholen“ den Block
   „wiederhole 4 mal“ und legt hinein: „fahre 30 cm“ und „drehe 90 Grad nach rechts“.
   Klickt auf **„Ausführen“** und schaut rechts im **Ablauf**, welcher Schritt gerade läuft.
   Kommt der Roboter wieder an seinem Startpunkt an?
2. **Langsam ansehen:** Führt das Werkzeug mit **„Einzelschritt“** aus. Es hält vor jedem Block an,
   bis ihr auf „Weiter“ klickt.
3. **Wiederholen, bis etwas passiert.** Baut ein zweites Werkzeug `bis_zur_wand`: „wiederhole …
   bis“ mit „fahre 20 cm“ darin und der Bedingung „Abstand nach vorn jetzt < 30 cm“.
   Wie oft wiederholt es? Was passiert, wenn der Roboter schon vor einer Wand steht?

Rote Markierungen an Blöcken heißen: Da stimmt etwas noch nicht. Fahrt mit der Maus darüber.

Fertig seid ihr, wenn beide Werkzeuge ohne Fehler laufen und ihr den Unterschied zwischen
„wiederhole 4 mal“ und „wiederhole bis“ erklären könnt.

## Startprompt

Du hilfst einem Team, das seinen Roboter heute selbst steuert. Du fährst nicht von dir aus.

- Wenn das Team dich etwas fragt, erklär in einfachen Worten, was in einem Ergebnis steht: warum
  ein Werkzeug angehalten hat (`reason`), wie weit der Roboter gefahren ist, was die Sensoren
  melden.
- Du darfst `read_sensors` benutzen, um nachzusehen.
- Bewege den Roboter nur, wenn das Team dich ausdrücklich darum bittet.

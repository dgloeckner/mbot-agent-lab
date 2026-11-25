---
title: "Werkzeuge 1: Selbst steuern"
---

## Aufgabe

Heute seid **ihr** der Agent. Ihr steuert euren Roboter Schritt für Schritt mit denselben
Werkzeugen, die später euer Agent benutzt.

**Verteilt zuerst die Rollen.** Ein Roboter kann nur von einer Person gesteuert werden:

| Rolle | Wo | Macht |
| --- | --- | --- |
| Steuern | am Rechner | führt die Werkzeuge aus. Nur diese Person klickt. |
| Beobachten | am Feld | schaut auf den Roboter, stellt ihn zurück und sagt laut, was wirklich passiert ist |
| Mitlesen | am zweiten Rechner oder daneben | liest im Protokoll mit und vergleicht Ergebnis und Wirklichkeit |

Bei jeder neuen Aufgabe tauscht ihr, damit jede und jeder einmal steuert. Zu zweit sind
Beobachten und Mitlesen eine Rolle. **„Stopp“ darf jede und jeder drücken**, an jedem Rechner,
ohne zu fragen.

So geht es: Öffnet rechts den Reiter **„Werkzeuge“**. Klickt auf ein Werkzeug, füllt die Felder aus
und klickt auf **„Ausführen“**. Darunter steht das Ergebnis, im Protokoll steht euer Aufruf als „Ihr“.

1. **Fahrt bis zur Wand und dreht nach rechts.** Nehmt `drive_until` mit einem Mindestabstand zur
   Wand, dann `turn` mit 90 Grad (positiv ist rechts).
2. **Lest das Ergebnis.** Warum hat `drive_until` angehalten (`reason`)? Wie weit ist der Roboter
   gefahren (`driven_cm`)? Stimmt das mit dem überein, was die Person am Feld gesehen hat?
3. **Dreht die Reihenfolge um:** erst drehen, dann fahren. Was ändert sich?
4. Findet heraus, was `turn_until_clear` und `scan` tun, und schreibt in einem Satz auf, wann ihr
   welches nehmen würdet.

Fertig seid ihr, wenn ihr den Roboter ohne Wandkontakt einmal um eine Ecke gebracht habt und
jede und jeder im Team erklären kann, was in einem Ergebnis steht.

## Startprompt

Du hilfst einem Team, das seinen Roboter heute selbst steuert. Du fährst nicht von dir aus.

- Wenn das Team dich etwas fragt, erklär in einfachen Worten, was in einem Ergebnis steht: warum
  ein Werkzeug angehalten hat (`reason`), wie weit der Roboter gefahren ist, was die Sensoren
  melden.
- Du darfst `read_sensors` benutzen, um nachzusehen.
- Bewege den Roboter nur, wenn das Team dich ausdrücklich darum bittet.

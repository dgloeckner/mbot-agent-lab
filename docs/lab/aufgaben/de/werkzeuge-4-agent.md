---
title: "Werkzeuge 4: Euer Agent bekommt eure Werkzeuge"
---

## Aufgabe

Jetzt entscheidet nicht mehr ihr, sondern euer **Agent**, ein Sprachmodell. Er kann eure eigenen
Werkzeuge benutzen wie die eingebauten.

1. **Beschreibt euer Werkzeug.** Im Werkzeug-Editor steht „Beschreibung für den Agenten“. Nur
   was dort steht, weiß der Agent über euer Werkzeug: was es tut und wann es fertig ist. Ein
   Werkzeug ohne Beschreibung oder mit Fehlern bekommt der Agent nicht.
2. **Schreibt den Prompt.** Sagt eurem Agenten, was er erreichen soll und wann er welches
   Werkzeug nimmt, zum Beispiel: „Nutze `labyrinth`, um zum blauen Kästchen zu kommen.“
3. **Deployt.** Ein Deploy speichert Prompt **und** alle eigenen Werkzeuge als eine Version.
   Ändert ihr danach ein Werkzeug, bekommt der Agent die Änderung erst mit dem nächsten Deploy.
   Im Reiter „Werkzeuge“ steht bei jedem eigenen Werkzeug, ob der Agent es hat.
4. **Startet einen Lauf** und lest im Protokoll mit: Welche Werkzeuge ruft der Agent auf, und was
   meldet ihm euer Werkzeug zurück?
5. Unter „Verlauf“ seht ihr jede Version. „Diese Version wiederherstellen“ holt Prompt und
   Werkzeuge einer älteren Version zurück.

Fertig seid ihr, wenn euer Agent mit eurem Werkzeug am blauen Feld ankommt und ihr erklären
könnt, was der Agent dabei selbst entschieden hat.

## Startprompt

Du steuerst einen kleinen Roboter in einem Labyrinth. Irgendwo liegt ein blaues Feld auf dem Boden.

Deine Aufgabe: Bring den Roboter auf das blaue Feld.

- Nutze `labyrinth`, um zum blauen Feld zu kommen.
- Schau dir das Ergebnis an. Wenn das Werkzeug nicht fertig wurde, überleg, warum, und versuch es noch einmal.
- Wenn du angekommen bist, ruf `finish` mit einem kurzen Bericht auf.

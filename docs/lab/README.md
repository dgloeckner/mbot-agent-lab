# Lab-Material

Material für die Lab-Leitung. Das Lab-Material ist auf Deutsch; Aufgaben gibt es auf Deutsch und
Englisch mit gleichem Dateinamen.

- `aufgaben/de/`, `aufgaben/en/`: fertige Aufgaben zum Importieren in den Leitstand.
- `feld/`: Bauplan und Aufbau des Linienfelds.

## Wie ein Lab-Tag mit Aufgaben läuft

1. **Vorbereiten (vor dem Lab-Tag).** Im Leitstand (`/admin`) unter „Aufgaben“ auf „Neue Aufgabe“
   klicken. Im Dialog „Aufgabendatei importieren“: eine Datei aus `aufgaben/de/` hochladen oder
   ihren Inhalt einfügen. Titel, Aufgabentext und Startprompt lassen sich danach noch ändern.
   In der „Vorschau“ seht ihr die Aufgabe so, wie die Teams sie sehen.
2. **Veröffentlichen.** Ab dann ist der Text fest: Alle Teams und die Auswertung beziehen sich auf
   denselben Text. Wer etwas ändern will, dupliziert die Aufgabe und veröffentlicht die Kopie.
3. **Teams gründen.** Die Teams öffnen `/join`, geben den Lab-Code ein und sehen dann „Wartet auf
   eine Aufgabe von der Lab-Leitung“. Roboter könnt ihr ihnen schon zuteilen.
4. **Zuweisen.** Eine offene Aufgabe auf eine Teamkarte ziehen oder über „Zuweisen …“ an ein Team
   oder an alle Teams geben. Teams, die später beitreten, bekommen nichts automatisch. Solange ein
   Team nicht angenommen hat, könnt ihr die Aufgabe zurückziehen.
5. **Annehmen.** Das Team liest die Aufgabe und wählt, womit sein Prompt beginnt: mit dem
   Startprompt der Aufgabe, leer oder mit dem Prompt aus einer anderen Aufgabe. Erst dann kann es
   deployen, Läufe starten und mit seinem Agenten reden.
6. **Arbeiten.** Prompt-Versionen, Notizen, Läufe, Gespräch und Protokoll gehören immer zur aktiven
   Aufgabe. Über den Aufgaben-Wähler oben auf der Teamseite wechselt das Team zwischen seinen
   Aufgaben; alle Rechner des Teams wechseln mit. Während eines Laufs geht das nicht.
7. **Werten.** Im Leitstand bei der Aufgabe auf „Läufe“ klicken. Für jeden Lauf eines Agenten
   steht dort: Version, wie er endete, Zeit bis zum Ziel, Modellaufrufe, Eingriffe und welche
   Werkzeuge der Agent hatte. Ob ein Lauf geschafft ist, entscheidet ihr, nicht der Agent: Er
   meldet nur „fertig“. Nach jedem Lauf „✓ geschafft“ oder „✗ nicht“ klicken (ein zweiter Klick
   überschreibt). Wenn jemand den Roboter während des Laufs anfasst oder zurücksetzt:
   „+ von Hand bewegt“. Stopps des Teams und Werkzeuge, die das Team vor dem Lauf von Hand
   aufgerufen hat, zählt der Server selbst. Die Rangliste: zuerst geschaffte Läufe, dann wenigste
   Modellaufrufe, dann kürzeste Zeit; gewertet wird der beste Lauf jedes Teams. Mit
   „Lauf starten“ startet ihr den Lauf eines Teams selbst, z. B. für den Wertungslauf.
8. **Schließen.** Wenn die Zeit um ist: Aufgabe öffnen, „Aufgabe schließen“. Laufende Läufe enden
   sofort, danach kann niemand mehr darin deployen oder starten. Alles bleibt lesbar.

## Wichtig für die Teams

**Der Agent sieht den Aufgabentext nie.** Die Teams lernen genau das: aus einer Aufgabe einen Prompt
machen. Was der Agent wissen muss (Ziel, Regeln, Feldwissen), muss im Prompt stehen. Der Startprompt
einer Aufgabe ist bewusst unvollständig; was fehlt, finden die Teams beim Testen heraus.

## Eine Aufgabendatei schreiben

```markdown
---
title: Finde das blaue Feld
---

## Aufgabe

Text für die Teams, in Markdown. Für Jugendliche geschrieben, nicht für einen Agenten:
Was ist das Ziel, woran merkt man, dass es geschafft ist, worauf muss man achten?

## Startprompt

Ein erster Prompt, den die Teams übernehmen können. Freiwillig.
```

Für die Challenge (Stufe 5) kommt in die Kopfzeilen `agent_tools: team_tools`: Dann hat der
Agent in dieser Aufgabe nur die deployten eigenen Werkzeuge des Teams und `read_sensors`, `say`,
`tell_team`, `remember`, `forget` und `finish`. Ohne die Zeile (oder mit `agent_tools: all`) hat er
alle Werkzeuge; ein anderer Wert wird beim Import abgelehnt. Im Leitstand ist das die Checkbox
„Agent darf nur eigene Werkzeuge des Teams nutzen“; nach dem Veröffentlichen ist sie fest.

Auf Englisch heißen die Abschnitte `## Task` und `## Starter prompt`. Jede Datei gibt es in
`aufgaben/de/` und `aufgaben/en/` mit gleichem Namen; ein Test prüft, dass sich alle importieren
lassen.

## Vorhandene Aufgaben

| Datei | Titel | Welt |
| --- | --- | --- |
| `erkunden.md` | Finde das blaue Feld | Arena oder Labyrinth, ein Roboter |
| `zwei-roboter.md` | Zu zweit suchen | Arena oder Labyrinth, zwei Roboter pro Team |
| `linienfeld.md` | Linienfeld – fahr zur Kreuzung (2,3) | Linienfeld (`feld/`), ein Roboter |
| `verhandeln.md` | Die Engstelle | mehrere Teams; Übung zu Prompt-Injection |

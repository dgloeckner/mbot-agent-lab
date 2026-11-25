---
title: "Werkzeuge 5: Challenge Labyrinth"
agent_tools: team_tools
---

## Aufgabe

Die Challenge: Euer Agent soll durch ein Labyrinth zum blauen Feld fahren. **In dieser Aufgabe hat
er nur eure eigenen Werkzeuge** (und `read_sensors`, `say`, `tell_team`, `remember`, `forget`,
`finish`). Alles, was den Roboter bewegt, baut ihr selbst.

Die Frage ist: **Was gehört ins Werkzeug, was entscheidet der Agent?**

- Ein großes Werkzeug (`labyrinth`) macht alles allein. Schnell, aber starr.
- Kleine Werkzeuge (`ein_feld`, `rechts`, `links`) machen je einen Schritt. Der Agent entscheidet
  nach jedem Schritt. Beweglich, aber er braucht mehr Modellaufrufe.
- Dazwischen liegt alles, was ihr euch ausdenkt.

Es gibt drei Runden mit einer Umbaupause:

1. **Runde 1:** das Labyrinth, das ihr kennt.
2. **Runde 2:** dasselbe Labyrinth, aber die Lab-Leitung hat etwas verändert. Ihr erfahrt vorher
   nicht, was.
3. **Umbauen:** 15 Minuten, in denen ihr eure Werkzeuge und euren Prompt neu schneidet.
4. **Runde 3:** noch einmal das veränderte Labyrinth.

Gewertet wird jeder Lauf: geschafft oder nicht (das entscheidet die Lab-Leitung, nicht der
Agent), dann möglichst wenige Modellaufrufe, dann die kürzeste Zeit. Jeder Eingriff zählt mit:
„Stopp“, Werkzeuge, die ihr vor dem Lauf selbst ausführt, und wenn jemand den Roboter anfasst.

Am Ende zeigt jedes Team seine Werkzeugliste und im Verlauf, was es zwischen Runde 2 und
Runde 3 geändert hat.

## Startprompt

Du steuerst einen kleinen Roboter in einem Labyrinth. Irgendwo liegt ein blaues Feld auf dem
Boden.

Deine Aufgabe: Bring den Roboter auf das blaue Feld. Du hast nur die Werkzeuge, die dein Team
gebaut hat.

- Lies die Beschreibung jedes Werkzeugs, bevor du es benutzt.
- Schau dir nach jedem Werkzeug das Ergebnis an und entscheide dann, was als Nächstes kommt.
- Wenn du angekommen bist, ruf `finish` mit einem kurzen Bericht auf.

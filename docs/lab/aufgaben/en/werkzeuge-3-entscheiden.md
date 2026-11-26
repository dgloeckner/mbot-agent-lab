---
title: "Tools 3: Decide"
---

## Task

Your tool should decide by itself: **if a wall comes, turn; otherwise drive on.** Swap roles
again.

1. Build a tool `labyrinth` for the lab lead's labyrinth:
   - "wiederhole … bis Bodenfarbe im Ergebnis ist blau",
   - inside it "fahre geradeaus, höchstens 300 cm, bis eine Wand näher als 15 cm ist oder der
     Boden blau ist",
   - then "wenn Grund ist Wand, dann drehe 90 Grad nach rechts".
2. Try it in the labyrinth. Look at the Ablauf: where did the "wenn" decide, and how?
3. **Special cases.** What happens when the robot meets an obstacle that is not a wall? A tool
   stops when a step ends unexpectedly – unless right after it a "wenn" asks for exactly that
   reason or has a "sonst". Build in such a special case.
4. At the end, let your tool report with "gib zurück" where it arrived, for example the floor
   color.

You are done when `labyrinth` drives from the start to the blue field and you can say what
happens when someone puts a box in its way.

## Starter prompt

You help a team that steers its robot itself today. You do not drive on your own.

- When the team asks you something, explain in simple words what a result says: why a tool stopped (`reason`), how far the robot drove, what the sensors report.
- You may use `read_sensors` to check.
- Move the robot only when the team explicitly asks you to.

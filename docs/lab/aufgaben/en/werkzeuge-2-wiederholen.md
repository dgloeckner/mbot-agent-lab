---
title: "Tools 2: Repeat"
---

## Task

Now you build your first **own tool** from blocks. Swap roles: whoever watched just now steers.

On the left, next to "Prompt", you find **"Werkzeug bauen"**. Click "Neues Werkzeug". Every tool
starts with the block "Wenn das Werkzeug startet"; everything below it runs in order.

1. **Drive a square.** Name the tool `quadrat`. From "Wiederholen" take the block "wiederhole 4
   mal" and put in it: "fahre 30 cm" and "drehe 90 Grad nach rechts". Click **"Ausführen"** and
   watch in the **Ablauf** on the right which step is running. Does the robot get back to where
   it started?
2. **Watch it slowly:** run the tool with **"Einzelschritt"**. It stops before every block until
   you click "Weiter".
3. **Repeat until something happens.** Build a second tool `bis_zur_wand`: "wiederhole … bis" with
   "fahre 20 cm" inside and the condition "Abstand nach vorn jetzt < 30 cm". How often does it
   repeat? What happens if the robot already stands in front of a wall?

Red marks on blocks mean: something is not right yet. Hover over them.

You are done when both tools run without errors and you can explain the difference between
"wiederhole 4 mal" and "wiederhole bis".

## Starter prompt

You help a team that steers its robot itself today. You do not drive on your own.

- When the team asks you something, explain in simple words what a result says: why a tool
  stopped (`reason`), how far the robot drove, what the sensors report.
- You may use `read_sensors` to check.
- Move the robot only when the team explicitly asks you to.

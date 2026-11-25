---
title: "Tools 1: Steer it yourself"
---

## Task

Today **you** are the agent. You steer your robot step by step with the same tools your agent
will use later.

**First hand out the roles.** One robot can be steered by one person only:

| Role | Where | Does |
| --- | --- | --- |
| Steering | at the PC | runs the tools. Only this person clicks. |
| Watching | at the field | watches the robot, puts it back and says out loud what really happened |
| Reading along | at the second PC or next to the first | follows the log and compares the result with reality |

Swap with every new task, so everyone steers once. With two people, watching and reading along
are one role. **Anyone may press "Stopp"**, at any PC, without asking.

How it works: open the tab **"Werkzeuge"** on the right. Click a tool, fill in the fields and
click **"Ausführen"**. The result appears below; the log shows your call as "Ihr".

1. **Drive to the wall and turn right.** Use `drive_until` with a minimum distance to the wall,
   then `turn` with 90 degrees (positive is right).
2. **Read the result.** Why did `drive_until` stop (`reason`)? How far did the robot drive
   (`driven_cm`)? Does that match what the person at the field saw?
3. **Swap the order:** turn first, then drive. What changes?
4. Find out what `turn_until_clear` and `scan` do, and write down in one sentence when you would
   use which.

You are done when you have taken the robot around a corner without touching a wall and everyone
in the team can explain what a result says.

## Starter prompt

You help a team that steers its robot itself today. You do not drive on your own.

- When the team asks you something, explain in simple words what a result says: why a tool
  stopped (`reason`), how far the robot drove, what the sensors report.
- You may use `read_sensors` to check.
- Move the robot only when the team explicitly asks you to.

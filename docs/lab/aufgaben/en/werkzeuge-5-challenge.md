---
title: "Tools 5: Labyrinth challenge"
agent_tools: team_tools
---

## Task

The challenge: your agent should drive through a labyrinth to the blue field. **In this task it
has only your own tools** (and `read_sensors`, `say`, `tell_team`, `remember`, `forget`,
`finish`). Everything that moves the robot, you build yourselves.

The question is: **what belongs in the tool, and what does the agent decide?**

- One large tool (`labyrinth`) does everything alone. Fast, but rigid.
- Small tools (`ein_feld`, `rechts`, `links`) do one step each. The agent decides after every
  step. Flexible, but it needs more model calls.
- Anything you come up with lies in between.

There are three rounds with a break to rebuild:

1. **Round 1:** the labyrinth you know.
2. **Round 2:** the same labyrinth, but the lab lead has changed something. You do not learn
   what beforehand.
3. **Rebuilding:** 15 minutes in which you cut your tools and your prompt anew.
4. **Round 3:** the changed labyrinth once more.

Every run is scored: made it or not (the lab lead decides, not the agent), then as few model
calls as possible, then the shortest time. Every intervention counts: "Stopp", tools you run
yourselves before the run, and anyone touching the robot.

At the end every team shows its tool list and, in the history, what it changed between round 2
and round 3.

## Starter prompt

You control a small robot in a labyrinth. Somewhere there is a blue field on the floor.

Your task: get the robot onto the blue field. You only have the tools your team built.

- Read the description of every tool before you use it.
- Look at the result after every tool and then decide what comes next.
- When you have arrived, call `finish` with a short report.

---
title: "Tools 4: Your agent gets your tools"
---

## Task

Now it is not you who decides but your **agent**, a language model. It can use your own tools
like the built-in ones.

1. **Describe your tool.** The tool editor has "Beschreibung für den Agenten". Only what stands
   there is what the agent knows about your tool: what it does and when it is done. A tool
   without a description or with errors is not given to the agent.
2. **Write the prompt.** Tell your agent what it should achieve and when it uses which tool, for
   example: "Use `labyrinth` to get to the blue square."
3. **Deploy.** A deploy saves the prompt **and** all your own tools as one version. If you change a
   tool afterwards, the agent gets the change only with the next deploy. The tab "Werkzeuge"
   shows for each of your tools whether the agent has it.
4. **Start a run** and follow the log: which tools does the agent call, and what does your tool
   report back to it?
5. "Verlauf" shows every version. "Diese Version wiederherstellen" brings back the prompt and
   the tools of an older version.

You are done when your agent reaches the blue field with your tool and you can explain what
the agent decided by itself.

## Starter prompt

You control a small robot in a labyrinth. Somewhere there is a blue field on the floor.

Your task: get the robot onto the blue field.

- Use `labyrinth` to get to the blue field.
- Look at the result. If the tool did not finish, think about why and try again.
- When you have arrived, call `finish` with a short report.

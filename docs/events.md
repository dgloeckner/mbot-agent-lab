# Event log

The event log (`events` table) holds the coarse facts that can count for a result: who joined,
which task was assigned and accepted, which agent version was deployed, which runs started and
how they ended, what the robots reported. Achievements and the results page read it. The run
protocol (`messages`, `tool_calls`, `llm_calls`) stays the detailed record per run.

## Rules

- **One writer:** `store.AddEvent`. Callers pass the scope they know; the store never guesses.
  `lab.Service.Record` wraps it for the server: it writes after the fact and only logs a failed
  write, so writing an event never blocks a robot or a model call.
- **Scope columns are real columns:** `team_id`, `task_id`, `robot_id`, `prompt_id` (agent
  version), `run_id`. "Per (team, task, robot, agent version)" is a plain `GROUP BY`; the index
  `events_scope` covers `(task_id, team_id, robot_id, prompt_id)`.
- **The database checks the scope:** `(team_id, task_id)` must be an assigned pair (composite
  foreign key to `team_tasks`); SQLite skips that check when one of the two is NULL, so team-only
  and task-only events work. Robot, prompt and run must exist.
- **`actor`** says who caused the event: `admin` (control room), `team`, `agent`, `robot`,
  `system` (the server on its own).
- **`data`** is small JSON with the fields below. Kinds and fields only grow; a reader ignores
  fields it does not know.

## Kinds

| Kind | Actor | Scope | `data` | Written by |
| --- | --- | --- | --- | --- |
| `team.joined` | team | team | `name` | `lab.Join` |
| `task.published` | admin | task | `title` | `lab.PublishTask` |
| `task.closed` | admin | task | — | `lab.CloseTask` |
| `task.assigned` | admin | team, task | — | `lab.AssignTask` (only teams that got the task now) |
| `task.withdrawn` | admin | team, task | — | `lab.WithdrawTask` |
| `task.accepted` | team | team, task | `prompt` (`starter`, `empty`, `copy`) | `lab.AcceptTask` |
| `task.activated` | team | team, task | — | `lab.AcceptTask`, `lab.ActivateTask` |
| `prompt.deployed` | team | team, task, prompt | `version`, `chars`, `restart`, `tools` (names of the team tools in the version); on accept also `source` (`starter`, `copy`) and `from_task` | team route `POST prompt`, `lab.AcceptTask` |
| `run.started` | team | team, task, prompt, run | `model`, `provider` | agent actor, run start |
| `run.ended` | team, agent, admin, system | team, task, prompt, run | `reason`, `tokens_in`, `tokens_out`, `cost_micro_eur` | agent actor, every run end; actor `agent` for `finish`, `team` for a stop or restart by the team, `admin` for the control room (stop all, `task_closed`, team deleted), `system` for limits and server stops |
| `run.finished` | agent | team, task, prompt, run | `report` (the agent's claim, not a result) | agent actor, before `run.ended` when the agent called `finish` |
| `robot.assigned` | admin | team, robot, task if active | `alias` | `lab.AssignRobot` |
| `robot.renamed` | team | team, robot, task if active | `alias`, `old` | `lab.RenameRobot` |
| `robot.released` | admin | team, robot, task if active | `alias` | `lab.AssignRobot` (moved or freed), `lab.DeleteTeam` |
| `robot.event` | robot | team, task, robot, prompt, run | `kind` (`obstacle_near`, `tilted`, `color_changed`, …), `data` | agent actor, every robot message that reaches a run |
| `note.added` | team, agent | team, task (and run for the agent) | `note_id`, `author` | team route `POST notes`, tool `remember` |
| `note.deleted` | team, agent | team, task (and run for the agent) | `note_id`, `author` | team route `DELETE notes/{id}`, tool `forget` |

## Example query

Runs per (team, task, robot, agent version):

```sql
SELECT team_id, task_id, robot_id, prompt_id, count(*)
FROM events WHERE kind = 'run.started'
GROUP BY task_id, team_id, robot_id, prompt_id;
```

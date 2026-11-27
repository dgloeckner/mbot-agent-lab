# Architecture

Current state of the mBot2 Agent Lab. This document describes the system as it is; a PR that
changes behaviour updates it in the same PR. Why things are the way they are: `decisions.md`.
MQTT: `protocol.md`. Tools: `tools.md`. Team tools: `programs.md`. Event log: `events.md`. The line field: `field.md`. Setup, tasks and deployment: `development.md`.

## Purpose

A lab in which teams of teenagers (around 15) instruct AI agents that drive real Makeblock mBot2
robots. Each team writes and deploys the prompt of its agent. The agent runs as a loop inside the
server, controls the team's robots through a small set of tools and reacts to messages from the
robots, from its team and from other agents.

- **Hardware:** Makeblock mBot2 with CyberPi (ESP32, MicroPython), ultrasonic sensor, quad RGB
  sensor, encoder motors, display, speaker and LEDs.
- **Users:** teams at Windows PCs in a corporate network. They need a browser only: no account
  with a model provider, no local software.
- **Learning goals:** instructing precisely, recognising failures and limits of agents, designing
  tools, understanding attacks such as prompt injection, treating tokens as a cost.
- **Out of scope for now:** camera, mapping and path planning; speech in and out (the vendor's
  cloud speech recognition is not used); user management beyond team links and an admin password.

## Components

```
 Lab PCs (browser) ──HTTPS/SSE──┐
                                ▼
                    ┌──────────────────────────┐        OpenAI-compatible API
                    │ labserver (one Go binary)│ ─────▶ (Mistral, Ollama, …) or Claude
                    │  web · agents · tools    │
                    │  robots · store (SQLite) │
                    │  embedded MQTT broker    │
                    └──────────▲───────────────┘
                               │ MQTT (TLS), QoS 0, compact JSON
                 ┌─────────────┴─────────────┐
           mBot2 + CyberPi             simulated robots
           (MicroPython)               (same script, fake hardware)
```

| Path | Content |
| --- | --- |
| `server/` | Go module, one binary `labserver`: broker, robot registry, agents, model access, store, web; the USB uploader `robotupload` |
| `web/` | team page and control room (Preact + signals, CodeMirror 6, Vite), embedded into the binary |
| `robot/` | MicroPython sources (`robot/src`), bundler, config example |
| `sim/` | simulator: fakes for the vendor modules, 2D world, worlds, tests, end-to-end test |
| `docs/` | these docs, protocol examples, lab material (`docs/lab/`, German) |
| `configs/`, `deploy/` | example lab configuration; Azure VM, systemd unit, local development config |

Deployment target: the server on an Azure VM, robots in a guest Wi-Fi, lab PCs in the corporate
network. Local development runs everything on one machine with simulated robots.

## Robot script

One MicroPython script per robot in upload mode (`robot/dist/main.py`). It starts after power-on,
connects to the broker and never blocks.

- **Bundle:** `robot/tools/bundle.py` joins the modules of `robot/src` in dependency order
  (`MODULES`) into one `main.py`, strips comments and docstrings and inlines `config.py`. All
  modules share one namespace after bundling. Only `hw.py` touches vendor modules (`cyberpi`,
  `mbot2`, `mbuild`, `event`); the MQTT client is bundled (`mqtt.py`) because the firmware has no
  `umqtt`.
- **Start:** the script's top level only defines names and ends with `hw_on_start(main)`, which
  registers `main` as the firmware's start handler (`event.start`, as in the code mBlock generates)
  and returns. Without the `event` module `main` runs right away. `main` starts the robot loop in
  its own thread with a 32 KB stack (the program's thread has 6 KB, too little for the quad RGB
  sensor library); without `_thread` the loop runs in the handler itself. The handler then sleeps
  as long as its robot runs instead of returning. Every start bumps a counter kept in
  `sys.modules`: after a new upload into the same interpreter, the older robot thread and its
  handler see the newer value and stop, so only one robot uses the MQTT client ID.
- **Upload:** `npx nx run robot:upload --config=…` bundles and runs `server/cmd/robotupload`
  (macOS, USB serial 115200 8N1; mBlock still works). The protocol lives in
  `server/internal/cyberpi`, ported from MIT-licensed community tools that reverse-engineered
  mBlock: F3/F4 frames (`F3, (F3+len_lo+len_hi)&FF, len_lo, len_hi, body, sum(body)&FF, F4`);
  ping and two scripts (import `config`, turn the REPL off) twice before the file and once after;
  a file header (size, XOR-of-words checksum, path `/flash/_xx_main.py`) and 80-byte blocks after
  padding the file with newlines to `len % 80 == 4`, the last 4 bytes in a block of their own.
  Every frame waits for its ack (3 s, 3 attempts); bytes outside frames are program output.
  Closing and reopening the port toggles DTR and restarts the CyberPi, whose firmware then moves
  the staged file to `main.py`.
- **Main loop** (about 40 Hz): fetch commands without blocking, advance the active skill one step
  (every skill is a state machine), check reflexes, send events on state changes, send the state
  on change and otherwise two to five times per second, watch the server heartbeat.
- **Nothing blocks:** the vendor's fixed-distance drive calls are not used; motors start with a
  speed and the loop stops them from encoder readings. Sounds play without blocking.
- **Filtered sensors:** `sense.py` turns raw readings into the filtered snapshot every decision
  uses (median distance, floor pattern after three equal readings, tilt with hysteresis, stalled
  wheels and a lifted nose after a minimum duration).
- **Reflexes** on the robot, independent of the server: emergency stop before obstacles, stop when
  held up by something the sensor does not see (`blocked`: stalled wheels or a lifted nose), limits
  per command, watchdog (3 s without heartbeat: stop, show "getrennt"), stop and reconnect with
  backoff on Wi-Fi or broker loss, stop and event when tilted or lifted.
- **Trip log:** finished motion segments, the basis of `retrace_path`; cleared on owner change,
  on `clear_log` and on restart.
- **Display and LEDs:** robot name, team, connection state, last text; LEDs in the team color,
  pulsing while the agent thinks.

Skills, events, the trip log and the floor trace are specified in `protocol.md`.

## Server

`server/cmd/labserver` wires the packages in `server/internal/`:

| Package | Role |
| --- | --- |
| `config` | loads the YAML configuration; secrets only from environment variables (`*_env`); validated at startup |
| `protocol` | MQTT message types, tested against `docs/examples/protocol/` |
| `broker` | embedded MQTT broker (mochi-mqtt), TCP and TLS listeners, per-robot login and ACL |
| `robots` | registry of all robots: state, owner, heartbeat, matching results to commands, reassignment |
| `store` | SQLite (pure Go driver `modernc.org/sqlite`), migrations in `store/migrations` |
| `lab` | joining, teams, robot assignment, robot names, lab code, released models: the rules; web handlers stay thin |
| `live` | per-team log hub for SSE, with a backlog for reconnecting browsers |
| `agent` | one actor per team: inbox, wake rules, context, budgets, runs and conversations |
| `tools` | tool registry: model-facing German descriptions, JSON schemas, handlers |
| `programs` | team tools: tree format, validation and interpreter; knows no tool names, asks the registry |
| `llm` | OpenAI-compatible client and Claude (official Anthropic Go SDK), retries, fallback, cost; `Fake` scripted test model |
| `memory` | history trimming and notes formatting for the context |
| `web` | HTTP handlers, SSE, admin session, embedded pages |
| `auth`, `mcp` | stubs |

Libraries: `github.com/mochi-mqtt/server/v2`, `modernc.org/sqlite`, `gopkg.in/yaml.v3`,
`golang.org/x/crypto/acme/autocert`, `github.com/anthropics/anthropic-sdk-go/v2`; otherwise the
standard library.

### Configuration

`configs/lab.example.yaml` documents every key: `server` (HTTP and MQTT addresses, TLS mode
`off`/`autocert`, admin password and optional fixed lab code by variable name, database path),
`llm` (providers with `kind` OpenAI-compatible, `anthropic` or `fake`, `default`, `fallback`,
prices, parallel calls), `budgets` and `robots` (ID, password variable, sticker color). Teams and
assignments are not in the configuration; they live in the database.

### Robot ownership

- The owner is stored per robot (`assignments`, primary key `robot_id`), so a robot can never
  have two owners. A team can have any number of robots.
- A free robot stands still and shows "frei".
- Reassigning is one step: stop, clear trip log and state, change owner, show the new owner. A
  running command ends with `reassigned`.
- A newly assigned robot is named after its sticker color (`rot`, `blau`, …). Teams rename their
  robots; `all` and `alle` are reserved. A team cannot give two robots the same name. A robot
  the lab lead takes away and later gives back to the same team gets the name that team gave it
  (the alias of its last `robot.released` event for that team, if still free); another team
  starts from the sticker color.

### Model access

- One `Provider` interface for a chat call with tools. Mistral, other OpenAI-compatible
  providers and Ollama differ only in configuration; Claude (`kind: anthropic`) goes through the
  official SDK and keeps its own form of an assistant turn in `Message.Native` (thinking blocks
  are replayed unchanged).
- A semaphore limits parallel calls (`max_parallel_calls`).
- Every call is stored with tokens, duration and estimated cost (prompt-cache reads and writes
  priced separately).
- Provider errors: bounded retries with backoff, then the configured fallback provider, then a
  notice to the team.
- Each team picks its model among the providers the control room released (`default` is always
  released). A choice applies at the team's next run.

### Data model (SQLite)

Timestamps are RFC 3339 strings in UTC. The schema lives in `server/internal/store/migrations`
(numbered files, applied in order, tracked in `PRAGMA user_version`). A database from before the
tasks reset (no `team_tasks` table) is refused at startup with "delete the file and start again";
its data is not migrated.

```
tasks ──< team_tasks >── teams
              │
              ├──< prompts      (agent versions, numbered per (team, task)) ──< prompt_programs
              ├──< programs     (team tools: working state)
              ├──< notes
              └──< runs ──< messages, tool_calls (robot), llm_calls
active_tasks: one row per team, points at an assigned team_task
robots: every robot the lab knows, so runs can reference it
```

**The key of everything a team produces is the pair (team, task)**, `(team_id, task_id)`.
`prompts`, `notes`, `programs` and `runs` carry both columns with a composite foreign key to `team_tasks`, so
queries per pair need no join and the database refuses rows for a pair that was never assigned.
In Go the pair is one type, `store.TeamTask{Team, Task}`; every team-scoped store function takes
it (`SavePrompt(ctx, tt, text)`, `ListNotes(ctx, tt)`, `LastRun(ctx, tt)`, …). Finer scopes:
(team, task, robot) through `robot_id` on messages and tool calls, and (team, task, robot, agent
version) through `runs.prompt_id`. The agent version is the prompt version; the run records the
model next to it.

| Table | Content |
| --- | --- |
| `teams` | ID, name (unique among teams not deleted, case-insensitive), secret token, color, chosen model provider, `deleted_at` |
| `tasks` | ID, title, body (Markdown for the team), starter prompt, state (`draft`, `open`, `closed`), tool set of the agent (`agent_tools`: `all` or `team_tools`), created, published and closed time |
| `team_tasks` | a task assigned to a team: assigned time, accepted time, withdrawn time |
| `active_tasks` | the task a team works in (one row per team, must be an assigned pair) |
| `robots` | device ID (stable across renames), first seen; every configured robot is registered at startup |
| `assignments` | robot → owner (`team:<id>`), alias (unique per owner) |
| `prompts` | (team, task), version (from 1 per pair), text, time |
| `prompt_programs` | the team tools frozen with a version: prompt, team tool id, name, description, Blockly state, tree, problems |
| `runs` | (team, task), kind (`run`, `conversation` or `manual`), prompt (agent version), model, start, end, end reason, tokens, cost, report |
| `messages` | run, sequence, source, kind, text, data, robot ID and alias at that moment, time |
| `tool_calls` | run, sequence, name, arguments, result, duration, robot ID and alias at that moment, the team tool it belongs to |
| `llm_calls` | run, provider, model, tokens in and out, duration, cost |
| `notes` | (team, task), text, origin run and message, author (`agent` or `team`), time |
| `programs` | team tools of a (team, task): stable id, name (unique per pair), description, Blockly state, derived tree and problems, update time |
| `settings` | lab-wide settings: lab code, released models, conversations on or off |
| `events` | the event log: kind, actor, scope (team, task, robot, prompt, run) as real columns, small JSON data; see `events.md` |

**Event log:** the coarse facts that can count for a result (joins, task assignments, deploys,
runs, robot reports, notes) at the finest scope known, written through `store.AddEvent` only.
Kinds, scope and data fields: [`events.md`](events.md).

**Deleting a team** is a soft delete: `deleted_at` is set, its robots are freed, the team
disappears from the control room and its link stops working; its tasks, prompts, notes and runs
stay, so results and history stay consistent. Its name is free again.

### Tasks

The control room prepares tasks and assigns them to teams; the rules live in `internal/lab`
(`tasks.go`).

- **Lifecycle:** `draft` → `open` (published) → `closed`. Only drafts can be edited or deleted.
  Publishing needs a title and a text and fixes title, text, starter prompt and the agent's tool
  set: all teams and the results refer to the same task.
- **Tool set** (`agent_tools`): `all`, or `team_tools` for the challenge: the agent gets only the
  team's deployed own tools and a few that move no robot (`tools.md`, "Tool set of a task"). The
  task editor has the checkbox "Agent darf nur eigene Werkzeuge des Teams nutzen"; task files set
  it with `agent_tools` in the front matter. The team page names the rule in "Aufgabe" and marks
  the built-in tools the agent does not have in "Werkzeuge". "Duplicate" creates a new draft from a task in any state.
- **Assigning:** only open tasks, to listed teams or to all current teams (`"all"`); teams that
  join later get nothing automatically. Assigning twice changes nothing. An assignment the team
  has not accepted can be withdrawn (`team_tasks.withdrawn_at`); assigning again makes it new.
- **Closing:** no more deploys, runs or conversations in the task, for every team. Running runs
  end with reason `task_closed` and open conversations in it end (`lab.Service.OnTaskClosed` →
  `agent.Manager.TaskClosed`). Prompts, notes and runs stay readable; a team whose active task
  closes keeps it selected.
- **IDs:** `"a"` + 6 random characters, like team IDs.
- **Board:** the control room state lists all tasks (without text) and, per team, its assigned
  tasks with their status: `assigned`, `accepted` or `active`.
- **Accepting** (team, `lab/teamtasks.go`): the team accepts an assigned, open task and it
  becomes active. The chosen start prompt becomes version 1 of the new pair: the task's starter
  prompt (`starter`; no version if it has none), nothing (`empty`), or the newest prompt of
  another task the team accepted (`copy`); `copy` also copies that task's team tools.
- **The active task** (`active_tasks`) is owned by the server, so all PCs of a team follow a
  switch. A team switches between accepted tasks; switching and accepting are refused while a
  run is active, and an open conversation ends (`agent.Manager.TaskActivated`).
- **No task, nothing works:** a team that joins has no task. Without an active task the team
  routes that act on one answer 409 "Eurem Team ist noch keine Aufgabe zugewiesen."; robots can
  still be assigned. In a closed task reads work and writes answer 409.

### HTTP routes

Pages and joining:

| Route | Purpose |
| --- | --- |
| `GET /`, `GET /join`, `POST /api/join` | start page, join page, join with lab code and team name (rate limited) |
| `POST /api/join-team` | another PC of a team: a join code gives the team link (rate limited) |
| `GET /assets/…` | embedded team page and control room build |
| `GET /healthz` | health check |

Team routes (`/t/{token}/…`, the token is the secret part of the team link). Prompts, notes,
runs, conversations and the log act on the team's active task: without one they answer 409, in
a closed task writes answer 409 and reads work. Stop always works. An unknown token answers 404
with JSON, except a browser opening the team page itself (`Accept: text/html`): it gets the page
with 404, which explains the wrong link, links to `/join` and lists the teams this browser joined.

| Route | Purpose |
| --- | --- |
| `GET /t/{token}` | team page |
| `GET api/session` | team, active task (with text), task list, robots, models, prompt, versions, notes, state; works without a task |
| `GET tasks` | assigned tasks with title, state, status, assigned and accepted time, and the active one |
| `GET tasks/{id}` | one assigned task with its text (the "Aufgabe" tab) |
| `POST tasks/{id}/accept` | accept and activate: `{prompt: "starter" \| "empty" \| "copy", from_task?}` |
| `POST tasks/{id}/activate` | switch to another accepted task; 409 while a run is active |
| `GET prompts`, `GET prompts/{version}`, `POST prompt` | versions (each with the team tools that are new, changed or removed against the version before, the prompt's changed characters and its runs by end); deploy, optionally with restart: answers the version's team tools and a warning, or `unchanged` without a new version |
| `POST prompts/{version}/restore` | restore a version: team tools here, prompt text for the draft; SSE `programs` |
| `GET prompts/{version}/tools?against=now\|{n}` | team tools that differ between the version and the working state (default) or version n: `{left, right, tools}`, the older state on the left, version 0 is the working state; reads work in a closed task |
| `GET prompts/{version}/tools/{id}?against=now\|{n}` | one team tool in both states (`{left, right}` with `version` and the stored tool or null) and the changes between them (`docs/programs.md`, "Comparing versions"); 404 for an unknown version or a tool in neither |
| `POST start`, `POST stop` | start or stop a run; stop also cancels a running by-hand call; start answers 409 while one runs |
| `POST tools/{name}` | run one by-hand tool (body: its arguments, `robot` with several robots) and answer its result as the agent would get it; 409 while a run is active |
| `GET programs`, `GET programs/{id}` | team tools of the active task (name, description, valid, problems); one with its Blockly state |
| `POST programs`, `PUT programs/{id}`, `DELETE programs/{id}` | create, replace (last write wins) or delete a team tool: `{name, description, blockly}`; the server derives tree and problems, stores invalid tools too and answers the problems per block; at most 12 per task |
| `POST programs/{id}/run`, `POST program-runs/{id}/continue` | run a team tool on a robot `{robot?, step?}`; let a single-step run take its next call (`programs.md`, "Running a team tool") |
| `GET blocks` | Blockly block definitions and toolbox of the tool editor, built from the registry (`programs.md`, "Blocks") |
| `POST message`, `POST conversation/new` | message to the own agent; start a new conversation |
| `GET events` | SSE: `ping` (keep-alive with its interval), `log`, `robots`, `agent`, `totals`, `models`, `task` (active task changed), `tasks` (assigned, withdrawn, accepted or closed), `programs` (a team tool was saved or deleted; `id`), `program` (a step of a running team tool); `Last-Event-ID` or `?after=` catches up |
| `GET/POST notes`, `PUT/DELETE notes/{id}` | notes |
| `PUT name`, `PUT model` | rename the team, choose the model |
| `POST join-code` | a short-lived join code for another PC of the team |
| `PUT robots/{robot}/alias`, `POST robots/{robot}/identify` | rename a robot, make it blink and beep |

Control room (`/admin`, admin password, session cookie):

| Route | Purpose |
| --- | --- |
| `GET /admin`, `POST /admin/api/login`, `/logout` | page and session |
| `GET /admin/api/state`, `GET /admin/api/events` | board payload and its SSE stream |
| `POST /admin/api/assign` | assign or free a robot |
| `POST /admin/api/lab-code` | replace the lab code |
| `PUT /admin/api/teams/{id}`, `PUT …/color`, `DELETE …` | rename, recolor, delete a team |
| `POST /admin/api/stop-all` | emergency stop for all robots |
| `PUT /admin/api/models`, `PUT /admin/api/conversation` | released models; conversations on or off |
| `GET/POST /admin/api/tasks` | list tasks; create a draft `{title, body, starter, agent_tools}` |
| `GET/PUT/DELETE /admin/api/tasks/{id}` | read a task with its text; edit or delete a draft |
| `POST /admin/api/tasks/{id}/publish`, `/close`, `/duplicate` | publish a draft, close an open task, copy any task into a new draft |
| `POST /admin/api/tasks/{id}/assign` | assign `{teams: ["t…", …]}` or `{teams: "all"}`; answers the teams that got it now |
| `DELETE /admin/api/tasks/{id}/assign/{team}` | withdraw an assignment the team has not accepted |
| `GET /admin/api/tasks/{id}/runs` | "Läufe": per team its runs with the four measures and tools, best run and rank |
| `POST /admin/api/runs/{id}/confirm` | score a run `{success}`; the last answer counts |
| `POST /admin/api/runs/{id}/intervention` | note an intervention `{kind: "moved"}` |
| `POST /admin/api/teams/{id}/start` | start a team's run from the control room (same rules as the team's start) |

## Agent runtime

Every team has exactly one agent: a goroutine with an inbox (`agent.Manager` holds one actor per
team). It calls the model only when a message wakes it. Everything that happens is a message.

- **Message:** source (`team`, `robot`, `agent:<team name>`, `system`), kind, text, data, time.
- **States:** `waiting`, `thinking`, `acting`. In `waiting` the model is not called. The state
  goes to the team page and, in the heartbeat, to the robots (LEDs pulse while thinking).
- **Wake rules:** team messages, other agents, the start of a run and the robot events `tilted`,
  `button_a`, `button_b`, `clap` and `battery_low` wake the agent. Routine readings (obstacles, floor colors) wait in the inbox for the next wake-up.
- **Loop:** collect all pending messages, build the context, call the model, run the requested
  tools and append the results, insert messages that arrived meanwhile, repeat until the model
  requests no tool or an end condition holds, then back to `waiting`.
- **Interruption:** normal messages wait for the next model call. A stop by the team or the
  control room cancels the running model call and tool calls through the `context` and stops the
  robots.

### Working in a task

A run or conversation starts in the team's active task and keeps it to the end (switching is
refused during a run). Prompt and notes come from that (team, task); runs and conversations are
stored with it; `remember` and `forget` act on its notes; a conversation's "last run" is the last
run of the same task. Without an active task, or in a closed task, a start is refused with a
German message. Closing the task ends a running run (reason `task_closed`) and an open
conversation; switching tasks ends an open conversation. **The task text is never in the
agent's context:** turning the task into a prompt is what the teams learn.

Tool calls of robot tools and robot messages record the robot's device ID and its name at that
moment (`robot_id`, `robot_alias`). The actor writes the events `run.started`, `run.ended`,
`run.finished` and `robot.event` (every robot message that reaches a run); the tool registry
writes `note.added` and `note.deleted` for `remember` and `forget`.

### Context per model call

- **System:** fixed platform rules (German; extra rules for several robots), the team prompt of
  the run's task in its newest version, the notes of that task. Not the task text.
- **Tools:** the built-in tools and the team tools of the newest version (valid, described).
- **History:** the last messages of the run verbatim (`max_history_messages`); older ones are
  left out. Summaries of long runs are not implemented.
- **Inbox:** one JSON line per new message with sender, age, kind, text and data. The sender field
  (`Team`, `Roboter <Name>`, `Agent <Name>`, `System`) is set by the server, so another agent
  cannot pose as the team.

| Source | Meaning for the agent |
| --- | --- |
| Team | instructions it follows |
| Robot | facts about the world |
| Other agents | claims to be checked |
| System | platform rules, e.g. budget or end of a run |

### Memory layers

| Layer | Written by | Seen by the model | Purpose |
| --- | --- | --- | --- |
| Working context | runtime | yes, sliding window | the current run |
| Notes | the agent via `remember`, the team on its page | yes, every round, size-limited | learning across the runs of one task |
| Protocol | runtime (`messages`, `tool_calls`, `llm_calls`) | no | comparing report and protocol ("Lügendetektor"), evaluation |

Each note stores its origin: the run and the message after which it was written.

### Deploying prompts and team tools

Every deploy that changes something creates a new version; old versions stay. A version is the
prompt text and every team tool of the pair, valid or not (`prompt_programs`); a deploy in which
only a tool changed creates a new version with the same text. A deploy with the same text and the
same team tools as the newest version creates nothing (`store.SavePrompt`): it answers that
version with `unchanged` (200, no log line), and the page says "Nichts geändert seit Version N";
"Deploy and restart" still begins a new run with it. A plain deploy takes effect at the next model
call, never in the middle of a tool call. "Deploy and restart" begins a new run. Each run stores
its version.

- The version list (`GET prompts`) gives per version the team tools that changed, the characters
  of the prompt that changed (`delta`, counted like the page's snapshots) and its runs by how
  they ended (`runs`); the history shows that as one line ("12 Zeichen geändert · 2 Läufe:
  1 × fertig, 1 × gestoppt"). "Deployen" is the primary button only while the draft or a team
  tool differs from the newest version.
- A restore writes a log line for every PC of the team ("Version 3 wiederhergestellt, Werkzeug
  quadrat entfernt.").

- The agent gets the team tools of the newest version that are valid and have a description
  (`tools.md`, "Team tools as agent tools").
- The deploy answers the version with its team tools (whether the agent has each, and why not) and
  a warning (`lab.DeployWarning`) for names in code style that look like a tool call the agent
  cannot make: no team tool of the version, no built-in tool, no robot of the team and no word of
  the tools (argument names, result fields and values such as `reason`, `wall`, `color`, collected
  by `tools.Registry.Words`). In a task with the tool set `team_tools` a built-in tool outside the
  kept list is named too ("`drive_until` hat euer Agent in dieser Aufgabe nicht."). The deploy's
  log entry carries the warning (`data.warning`); the page shows it as a warning that stays until
  it is closed or the next deploy.
- **Zurück:** `POST prompts/{version}/restore` replaces the working state with a version: the team
  tools of the pair (also ones deleted since, with their ids) and, on the page, the prompt draft.
  Nothing is deleted from the history; the next deploy is a new version.

### Budgets and end conditions

Budgets (config `budgets`): steps per wake-up, tokens per run, minutes per run, messages to other
agents per minute, messages kept in the context, team messages to a stopped agent per minute,
tokens per conversation. A reached budget gives the agent a system message and the team a notice.

A run ends when the agent calls `finish(report)`, at the step, token or time limit, or on a stop
by the team or the control room.

### Conversations

While no run is active, a team message wakes the agent in conversation mode. A conversation is a
`runs` row with `kind = 'conversation'`; its messages, tool calls and model calls are stored like
a run's and its tokens count into the team totals. The context has conversation rules, every tool
as text, the team, prompt and notes, and the numbered protocol of the last run. Only
`read_sensors` and `remember` can be called. It ends when a run starts, with "Neues Gespräch" or
at its token budget. The control room can switch conversations off for the lab.

### By hand

Stage 1 of the learning path: the team is the agent. `POST tools/{name}` runs one tool the
registry flags as by hand (`docs/tools.md`) on one of the team's robots, without a language
model (`agent.Manager.Manual`, through `tools.Registry.CallManual`). Unknown and agent-only tools
answer `ok: false` like any mistake; arguments are checked against the tool's schema.

- **No driver, no pause:** by-hand calls work only while no run is active (409 "Erst anhalten:
  Euer Agent fährt gerade."), and a run cannot start while a by-hand call runs. An open
  conversation does not block: the agent moves no robot there. Two PCs of a team are not
  locked against each other; the robot answers a second motion command with `busy`.
- **Stop** cancels a running by-hand call (its result ends with `stopped`) and stops the robots.
- **Storage:** by-hand calls are stored like the agent's tool calls, under a `runs` row of kind
  `manual` of the (team, task): no model, no tokens, no cost. It ends when a run starts, the team
  switches tasks or the task is closed; the next by-hand call opens a new one.
- **Log:** one live log entry per call with source `team` and kind `tool`, in the shape of the
  agent's tool entries; the server writes one `INFO` line `tool by hand` (team, task, robot,
  tool, reason), and `tool by hand refused` during a run.

### Talking between agents

`say(to, text)` puts a message into the recipient's inbox and wakes its agent; the text also
shows on the speaker's display. Agent messages are rate limited per minute.

### Live log

`live.Hub` keeps a backlog of log entries per team and fans them out to the team's SSE
connections. It knows each team's active task (`SetTask`, loaded at startup and set on every
accept or switch) and stamps it on every entry (`task_id`). A reconnecting page gets only the
backlog of the active task, and a stream never forwards entries of another task. When the active
task changes, every page of the team gets the SSE event `task` and reloads prompt, notes, log and
conversation; `tasks` tells it to reload the task list.

A dead connection often gives the browser no error (a Wi-Fi that drops packets, a frozen server):
the stream stays open and nothing arrives. The team stream therefore sends a named keep-alive
`ping` (`{"every_s":10}`) at the start and every 10 s; an SSE comment would not reach the page.
The page notes the time of every event. After three intervals of silence, or when the browser
reports `offline`, the band shows "Verbindung unterbrochen", robot cards show "Stand vor N s"
with greyed values instead of "online", and the page opens a new stream with `?after=` the last
log entry (the same as `Last-Event-ID`). "Stopp" without a connection, or one that gets no answer
within 5 s, says that the robots were **not** stopped and what to do.

## Web UI

Built with Vite into `server/internal/web/dist/app` and embedded; `dist/README.md` keeps the
embed compiling without a build. No CDN assets. Rarely used views are lazy-loaded
(`web/src/lazy.tsx`).

- **Team page** (`/t/{token}`): prompt editor (CodeMirror, Markdown, tool name completion),
  preview, history with diffs against deployed versions and local snapshots, live log over SSE,
  conversation tab, notes, tools and console, robots with rename and "Finden", usage, model choice.
  - **Prompt editor and keys:** the editor does not take the focus on load; Tab indents, Escape
    and then Tab leaves it (said to screen readers via `aria-description`). Toasts sit above the
    editor's status line.
  - **Protokoll:** steps of a team tool carry their robot (`data.robot`); with several robots
    the line names it (`drive(robot: blau, cm: 20)`). Lab events (task assigned, withdrawn or
    closed, robot given or taken) also show as a toast on every tab. Scrolled up, the log keeps
    its place and offers "Neue Einträge ↓". "Stopp" always answers with a toast. Which
    conversation answers were seen is kept per task in the UI preferences, so a reload does not
    mark them new again.
  - **Layout:** prompt and side pane side by side with a splitter (a share of the width, 58 %
    by default, held between 420 px and 400 px minimums; a double click or Home resets it); the
    side pane's tabs get closer when it is narrow (container query) and scroll inside their row
    rather than widen the page. Below 960 px the panes stack. No horizontal page scroll from
    960 px up (`web/e2e/layout.spec.ts`).
  - **Console** (tab "Werkzeuge"): every tool of the catalog; a click on a by-hand tool opens it
    in place (lazy-loaded `ToolConsole`): a form built from the tool's JSON schema in the session
    (`web/src/console.ts`; fields in the order of the signature shown in the row, optional ones
    marked "optional"; a missing required value is caught in the browser with the server's
    sentence and not sent), "Ausführen" (`POST tools/{name}`) and the result below, scrolled into
    view when it arrives (the previous one dims while a call runs): one sentence
    in the protocol's words, the key fields, the floor trace as a band (four rows, one per probe)
    and the JSON on demand. "In den Prompt einfügen" stays as a second action. Agent-only tools
    carry "nur Agent" and open the same way, with only "In den Prompt einfügen" inside. The
    insert goes to the cursor only if the team placed it in this page view, otherwise on a new
    line at the end, with the missing spaces (`web/src/promptinsert.ts`); the prompt comes into
    view, the name flashes and a toast says where it went. With several robots a robot picker sits
    above the list.
    While a run is active the forms are disabled ("Erst anhalten"); a closed task is read-only.
    By-hand calls appear in the protocol as "Ihr"; without a run the protocol offers "Selbst
    steuern", which opens the tab.
  - **Versions:** "Eigene Werkzeuge" shows per tool whether the agent has it ("beim Agenten",
    "geändert" since the last deploy, "noch nicht deployt", or "nicht beim Agenten" with the
    reason). The prompt editor completes the names of team tools. "Deployen" also sits in the
    tool editor; when the prompt draft differs from the deployed version its button reads
    "Deployen + Prompt" and the answer says "mit geändertem Prompt". The history shows per version the team tools that are new, changed or removed;
    "Diese Version wiederherstellen" restores prompt and team tools and first asks, naming what
    is not deployed yet and would be lost.
  - **Tool comparison** (history, `web/src/components/ToolDiffView.tsx`, lazy-loaded with
    Blockly): for the selected version a row of chips, "Prompt" and one per team tool that
    differs ("neu", "geändert", "entfernt"), against "Euer Stand jetzt" or, with "Vergleichen
    mit", another version (the prompt chip then compares the two version texts). A tool chip
    shows two read-only Blockly workspaces, the older state left, each loaded from its stored
    state and as high as its blocks (scrolling sideways), side by side when the pane is at least
    620 px wide and one above the other below. Marks per block id from the server's changes: a
    changed value outlines its field on both sides, a new block has a solid outline (right), a
    removed one a dashed outline (left), a moved one a dotted outline (both); the legend gives
    each its sign (→ + − ↕), so no mark relies on color. Name and description changes stand as
    text above. A new tool shows only on the right ("Gab es in Version n noch nicht."), a
    removed one only on the left. Pure helpers in `web/src/tooldiff.ts`.
  - **Team tools** (tab "Werkzeuge", group "Eigene Werkzeuge" above the built-in tools): name,
    description, "bereit" or "hat Fehler", "Neues Werkzeug", per tool "Bearbeiten" and
    "Ausführen" with the end of the last run below. "Bearbeiten" opens the tool editor in the left
    pane: its head switches between "Prompt" and "Werkzeug bauen", the side pane stays. The editor
    (`web/src/components/ToolEditor.tsx`, Blockly, lazy-loaded) has a picker of the team's tools,
    "Ausführen", "Einzelschritt" ("Weiter"), "Stopp" and "Groß" (hides the side pane for a while)
    in its head, name and description in one row with "Ablauf zeigen" and "Löschen", the block
    categories and the workspace, and the "Ablauf" beside it while the tool runs or ran or the
    team opens it. Details: `programs.md`, "Editor".
  Browser storage (`web/src/storage.ts`) holds only the draft, draft snapshots and UI preferences.
  - **Tasks:** the team band shows the active task with a picker: accepted tasks to switch to
    (disabled during a run, "Erst anhalten") and new tasks with a badge. Without an active task
    the page shows "Wartet auf eine Aufgabe von der Lab-Leitung" (or the new tasks to accept)
    and the team's robots; editor, run and conversation are hidden. The accept dialog
    (lazy-loaded) asks first, in a part that does not scroll, for the start prompt: the task's
    starter prompt, empty, or the prompt of another accepted task, as a row of options with a
    preview of the chosen one (fixed height). Below, the task text scrolls, under the page's own
    hint that the agent does not see it (task files carry no such sentence).
    The side pane has a tab "Aufgabe" with the text. A closed task shows a banner and is
    read-only (no deploy, start, messages or note changes).
  - **Scoping:** every panel shows only the active task: log, conversation, notes, prompt
    versions, diffs and snapshots. On the SSE event `task` (a switch on any PC of the team) the
    page reloads the session, empties log and conversation and reopens the stream, which replays
    the new task's backlog; entries of another task are ignored (`web/src/taskview.ts`).
    Drafts and snapshots are stored per (team, task) (`mbot-lab:v2:<team>:<task>:…`); keys from
    before tasks are dropped on load, preferences stay.
- **Join page** (`/join`): lab code and team name; or "Zu einem Team dazukommen" with a join
  code. A join link `/join?code=<lab code>` fills in the lab code (`web/src/join/link.ts`).
  The browser remembers joined teams. Join codes (`lab.JoinCode`): six characters without
  look-alikes, valid ten minutes, kept in memory; `POST /t/{token}/join-code` shows one (the same
  one while it has more than three minutes left), on the welcome page and behind the band's link
  button ("Weiteren Rechner dazuholen", which also copies the link).
- **Control room** (`/admin`): lab code for the projector with a QR code of the join link (built
  in the browser from the address the control room was opened with; a click shows it large,
  `web/src/admin/JoinQR.tsx`), all robots live, every team with its
  robots and open browsers, robot assignment by drag and drop or from a list, team rename and
  delete, released models, conversations switch, usage per team, stop all.
  - **Task panel** ("Aufgaben"): tasks grouped by state (Entwürfe, Offen, Geschlossen),
    "Neue Aufgabe". Clicking a task opens a dialog (`web/src/admin/TaskEditor.tsx`, lazy-loaded
    with CodeMirror and the Markdown renderer): a draft is edited there (title, task text,
    starter prompt, both Markdown) with a preview as the team will see it, saved, published or
    deleted; a published task is shown read-only with "Duplizieren" and "Aufgabe schließen"
    (with confirmation: running runs end).
    A draft can be filled from a task file (`docs/lab/aufgaben/{de,en}/*.md`: front matter with
    `title` and optional `agent_tools`, sections `## Aufgabe`/`## Task` and `## Startprompt`/`## Starter prompt`) by upload
    or paste; the browser parses it (`web/src/admin/taskfile.ts`), the server never reads files.
  - **Assigning:** drag an open task onto a team card, or pick a team or "An alle Teams" in the
    task's "Zuweisen …" menu (keyboard and screen readers, like robot assignment). Each team
    card lists its tasks with status (zugewiesen, angenommen, aktiv) and "Zurückziehen" while the
    team has not accepted. Pure helpers in `web/src/admin/tasks.ts`.
  - **Runs** ("Läufe" on a published task, `web/src/admin/RunsView.tsx`, lazy-loaded): the
    ranking and, per team, every agent run with version, end, success, time to the goal, model
    calls, interventions and the tools it had (team tools and built-in ones). Buttons per run:
    "geschafft", "nicht" (`run.confirmed`), "+ von Hand bewegt" (`run.intervention`); per team
    "Lauf starten". The view reloads when the board's `scores` counter or an agent changes.
    Measures and ranking are computed by the server (`store.TaskRuns`, `lab.Rank`,
    `docs/events.md`); `web/src/admin/runs.ts` only words them.

## Simulator

`sim/` runs the real bundled robot script under CPython with fakes for `cyberpi`, `mbot2`,
`mbuild` and `event` (`sim/fakes/`) and a 2D world (`sim/world.py`) with optional seeded sensor noise. MQTT is
real. Like the firmware, the simulator loads the script first and then runs its start handler
(`event.fire_start`); CPython refuses a 32 KB thread stack, so there the robot loop runs in the
handler. The floor is a drawing of tape bands seen by four probes. Assumed robot geometry in
`sim/mbot2.json`. Worlds: `arena`, `labyrinth`, `labyrinth_rechts` (up, right, down to blue:
solvable by turning right at every wall), `labyrinth_gestoert` (the same with a box in the top
corridor, a detour through the middle and the blue field in a side corridor: turning right at every
wall no longer arrives, following the right-hand wall cell by cell does), `linie`, `gitter`
(generated by `sim/grid.py` together with `docs/lab/feld/bauplan.svg`). A second robot in the way
is a second simulated robot, not world geometry. `--delay-ms` puts a TCP proxy (`sim/netdelay.py`)
between each robot and the broker that delays every packet in both directions, like the guest
Wi-Fi, so nothing works in the simulator that would come too late on a real robot. `sim/tests` run
the real bundle in-process; `sim/e2e` starts `labserver` with the scripted model and simulated
robots and drives the lab over HTTP. `sim/e2e/test_walkthrough.py` is the challenge walk-through with
150 ms delay: the rigid team tool `labyrinth` arrives in `labyrinth_rechts` and not in
`labyrinth_gestoert`; the scripted agent with the small cut (`ein_feld`, `rechts`, `links`,
`read_sensors`) arrives in both.

# Architecture

Current state of the mBot2 Agent Lab. This document describes the system as it is; a PR that
changes behaviour updates it in the same PR. Why things are the way they are: `decisions.md`.
MQTT: `protocol.md`. Tools: `tools.md`. Team tools: `programs.md`. Event log: `events.md`. The line field: `field.md`.

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
| `server/` | Go module, one binary `labserver`: broker, robot registry, agents, model access, store, web |
| `web/` | team page and control room (Preact + signals, CodeMirror 6, Vite), embedded into the binary |
| `robot/` | MicroPython sources (`robot/src`), bundler, config example |
| `sim/` | simulator: fakes for the vendor modules, 2D world, worlds, tests, end-to-end test |
| `docs/` | these docs, protocol examples, lab material (`docs/lab/`, German) |
| `configs/`, `deploy/` | example lab configuration; Azure VM, systemd unit, local development config |

Deployment target: the server on an Azure VM, robots in a guest Wi-Fi, lab PCs in the corporate
network. Local development runs everything on one machine with simulated robots.

## Robot script

One MicroPython script per robot in upload mode (`robot/dist/main.py`, uploaded with mBlock). It
starts after power-on, connects to the broker and never blocks.

- **Bundle:** `robot/tools/bundle.py` joins the modules of `robot/src` in dependency order
  (`MODULES`) into one `main.py`, strips comments and docstrings and inlines `config.py`. All
  modules share one namespace after bundling. Only `hw.py` touches vendor modules (`cyberpi`,
  `mbot2`, `mbuild`); the MQTT client is bundled (`mqtt.py`) because the firmware has no `umqtt`.
- **Main loop** (about 40 Hz): fetch commands without blocking, advance the active skill one step
  (every skill is a state machine), check reflexes, send events on state changes, send the state
  on change and otherwise two to five times per second, watch the server heartbeat.
- **Nothing blocks:** the vendor's fixed-distance drive calls are not used; motors start with a
  speed and the loop stops them from encoder readings. Sounds play without blocking.
- **Filtered sensors:** `sense.py` turns raw readings into the filtered snapshot every decision
  uses (median distance, floor pattern after three equal readings, tilt with hysteresis).
- **Reflexes** on the robot, independent of the server: emergency stop before obstacles, limits
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
  robots; `all` and `alle` are reserved. A team cannot give two robots the same name.

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
| `tasks` | ID, title, body (Markdown for the team), starter prompt, state (`draft`, `open`, `closed`), created, published and closed time |
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
  Publishing needs a title and a text and fixes title, text and starter prompt: all teams and
  the results refer to the same text. "Duplicate" creates a new draft from a task in any state.
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
| `GET /assets/…` | embedded team page and control room build |
| `GET /healthz` | health check |

Team routes (`/t/{token}/…`, the token is the secret part of the team link). Prompts, notes,
runs, conversations and the log act on the team's active task: without one they answer 409, in
a closed task writes answer 409 and reads work. Stop always works.

| Route | Purpose |
| --- | --- |
| `GET /t/{token}` | team page |
| `GET api/session` | team, active task (with text), task list, robots, models, prompt, versions, notes, state; works without a task |
| `GET tasks` | assigned tasks with title, state, status, assigned and accepted time, and the active one |
| `GET tasks/{id}` | one assigned task with its text (the "Aufgabe" tab) |
| `POST tasks/{id}/accept` | accept and activate: `{prompt: "starter" \| "empty" \| "copy", from_task?}` |
| `POST tasks/{id}/activate` | switch to another accepted task; 409 while a run is active |
| `GET prompts`, `GET prompts/{version}`, `POST prompt` | versions (each with the team tools that are new, changed or removed against the version before); deploy, optionally with restart: answers the version's team tools and a warning |
| `POST prompts/{version}/restore` | restore a version: team tools here, prompt text for the draft; SSE `programs` |
| `POST start`, `POST stop` | start or stop a run; stop also cancels a running by-hand call; start answers 409 while one runs |
| `POST tools/{name}` | run one by-hand tool (body: its arguments, `robot` with several robots) and answer its result as the agent would get it; 409 while a run is active |
| `GET programs`, `GET programs/{id}` | team tools of the active task (name, description, valid, problems); one with its Blockly state |
| `POST programs`, `PUT programs/{id}`, `DELETE programs/{id}` | create, replace (last write wins) or delete a team tool: `{name, description, blockly}`; the server derives tree and problems, stores invalid tools too and answers the problems per block; at most 12 per task |
| `POST programs/{id}/run`, `POST program-runs/{id}/continue` | run a team tool on a robot `{robot?, step?}`; let a single-step run take its next call (`programs.md`, "Running a team tool") |
| `GET blocks` | Blockly block definitions and toolbox of the tool editor, built from the registry (`programs.md`, "Blocks") |
| `POST message`, `POST conversation/new` | message to the own agent; start a new conversation |
| `GET events` | SSE: `log`, `robots`, `agent`, `totals`, `models`, `task` (active task changed), `tasks` (assigned, withdrawn, accepted or closed), `programs` (a team tool was saved or deleted; `id`), `program` (a step of a running team tool); `Last-Event-ID` catches up |
| `GET/POST notes`, `PUT/DELETE notes/{id}` | notes |
| `PUT name`, `PUT model` | rename the team, choose the model |
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
| `GET/POST /admin/api/tasks` | list tasks; create a draft `{title, body, starter}` |
| `GET/PUT/DELETE /admin/api/tasks/{id}` | read a task with its text; edit or delete a draft |
| `POST /admin/api/tasks/{id}/publish`, `/close`, `/duplicate` | publish a draft, close an open task, copy any task into a new draft |
| `POST /admin/api/tasks/{id}/assign` | assign `{teams: ["t…", …]}` or `{teams: "all"}`; answers the teams that got it now |
| `DELETE /admin/api/tasks/{id}/assign/{team}` | withdraw an assignment the team has not accepted |

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

Every deploy creates a new version; old versions stay. A version is the prompt text and every team
tool of the pair, valid or not (`prompt_programs`); a deploy in which only a tool changed creates a
new version with the same text. A plain deploy takes effect at the next model call, never in the
middle of a tool call. "Deploy and restart" begins a new run. Each run stores its version.

- The agent gets the team tools of the newest version that are valid and have a description
  (`tools.md`, "Team tools as agent tools").
- The deploy answers the version with its team tools (whether the agent has each, and why not) and
  a warning for names in code style the agent has no tool for.
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

## Web UI

Built with Vite into `server/internal/web/dist/app` and embedded; `dist/README.md` keeps the
embed compiling without a build. No CDN assets. Rarely used views are lazy-loaded
(`web/src/lazy.tsx`).

- **Team page** (`/t/{token}`): prompt editor (CodeMirror, Markdown, tool name completion),
  preview, history with diffs against deployed versions and local snapshots, live log over SSE,
  conversation tab, notes, tools and console, robots with rename and "Finden", usage, model choice.
  - **Console** (tab "Werkzeuge"): every tool of the catalog; a click on a by-hand tool opens it
    in place (lazy-loaded `ToolConsole`): a form built from the tool's JSON schema in the session
    (`web/src/console.ts`), "Ausführen" (`POST tools/{name}`) and the result below: one sentence
    in the protocol's words, the key fields, the floor trace as a band (four rows, one per probe)
    and the JSON on demand. "In den Prompt einfügen" stays as a second action. Agent-only tools
    carry "nur Agent" and do not open. With several robots a robot picker sits above the list.
    While a run is active the forms are disabled ("Erst anhalten"); a closed task is read-only.
    By-hand calls appear in the protocol as "Ihr"; without a run the protocol offers "Selbst
    steuern", which opens the tab.
  - **Versions:** "Eigene Werkzeuge" shows per tool whether the agent has it ("beim Agenten",
    "geändert" since the last deploy, "noch nicht deployt", or "nicht beim Agenten" with the
    reason). The prompt editor completes the names of team tools. "Deployen" also sits in the
    tool editor. The history shows per version the team tools that are new, changed or removed;
    "Diese Version wiederherstellen" restores prompt and team tools and first asks, naming what
    is not deployed yet and would be lost.
  - **Team tools** (tab "Werkzeuge", group "Eigene Werkzeuge" above the built-in tools): name,
    description, "bereit" or "hat Fehler", "Neues Werkzeug", per tool "Bearbeiten" and
    "Ausführen" with the end of the last run below. "Bearbeiten" opens the tool editor in the left
    pane: its head switches between "Prompt" and "Werkzeug bauen", the side pane stays. The editor
    (`web/src/components/ToolEditor.tsx`, Blockly, lazy-loaded) has a picker of the team's tools,
    "Ausführen", "Einzelschritt" ("Weiter") and "Stopp" in its head, name and description, the
    block categories and the workspace, and the "Ablauf" beside it. Details: `programs.md`,
    "Editor".
  Browser storage (`web/src/storage.ts`) holds only the draft, draft snapshots and UI preferences.
  - **Tasks:** the team band shows the active task with a picker: accepted tasks to switch to
    (disabled during a run, "Erst anhalten") and new tasks with a badge. Without an active task
    the page shows "Wartet auf eine Aufgabe von der Lab-Leitung" (or the new tasks to accept)
    and the team's robots; editor, run and conversation are hidden. The accept dialog
    (lazy-loaded) shows the task text, says that the agent does not see it, and asks for the
    start prompt: the task's starter prompt, empty, or the prompt of another accepted task.
    The side pane has a tab "Aufgabe" with the text. A closed task shows a banner and is
    read-only (no deploy, start, messages or note changes).
  - **Scoping:** every panel shows only the active task: log, conversation, notes, prompt
    versions, diffs and snapshots. On the SSE event `task` (a switch on any PC of the team) the
    page reloads the session, empties log and conversation and reopens the stream, which replays
    the new task's backlog; entries of another task are ignored (`web/src/taskview.ts`).
    Drafts and snapshots are stored per (team, task) (`mbot-lab:v2:<team>:<task>:…`); keys from
    before tasks are dropped on load, preferences stay.
- **Join page** (`/join`): lab code and team name; the browser remembers joined teams.
- **Control room** (`/admin`): lab code for the projector, all robots live, every team with its
  robots and open browsers, robot assignment by drag and drop or from a list, team rename and
  delete, released models, conversations switch, usage per team, stop all.
  - **Task panel** ("Aufgaben"): tasks grouped by state (Entwürfe, Offen, Geschlossen),
    "Neue Aufgabe". Clicking a task opens a dialog (`web/src/admin/TaskEditor.tsx`, lazy-loaded
    with CodeMirror and the Markdown renderer): a draft is edited there (title, task text,
    starter prompt, both Markdown) with a preview as the team will see it, saved, published or
    deleted; a published task is shown read-only with "Duplizieren" and "Aufgabe schließen"
    (with confirmation: running runs end).
    A draft can be filled from a task file (`docs/lab/aufgaben/{de,en}/*.md`: front matter with
    `title`, sections `## Aufgabe`/`## Task` and `## Startprompt`/`## Starter prompt`) by upload
    or paste; the browser parses it (`web/src/admin/taskfile.ts`), the server never reads files.
  - **Assigning:** drag an open task onto a team card, or pick a team or "An alle Teams" in the
    task's "Zuweisen …" menu (keyboard and screen readers, like robot assignment). Each team
    card lists its tasks with status (zugewiesen, angenommen, aktiv) and "Zurückziehen" while the
    team has not accepted. Pure helpers in `web/src/admin/tasks.ts`.

## Simulator

`sim/` runs the real bundled robot script under CPython with fakes for `cyberpi`, `mbot2` and
`mbuild` (`sim/fakes/`) and a 2D world (`sim/world.py`) with optional seeded sensor noise. MQTT is
real. The floor is a drawing of tape bands seen by four probes. Assumed robot geometry in
`sim/mbot2.json`. Worlds: `arena`, `labyrinth`, `labyrinth_rechts` (up, right, down to blue: solvable by turning right at every wall), `linie`, `gitter` (generated by `sim/grid.py`
together with `docs/lab/feld/bauplan.svg`). `sim/tests` run the real bundle in-process; `sim/e2e`
starts `labserver` with the scripted model and one simulated robot and drives the lab over HTTP.

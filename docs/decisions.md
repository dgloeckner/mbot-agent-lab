# Decisions

Binding decisions for the mBot2 Agent Lab, each with a one-line why. Deviations are proposed
first and only implemented once approved; an approved deviation replaces the entry it changes
and goes here, not into `CLAUDE.md`. How the system works today: `architecture.md`.

## Platform

| Topic | Decision | Why |
| --- | --- | --- |
| Repository | `mbot-agent-lab`, one monorepo | everything in one place |
| Server | Go, one binary `labserver` | embeddable broker, simple distribution |
| Robot code | MicroPython in upload mode, uploaded over USB with `robot:upload` (Go CLI `robotupload`, macOS); mBlock still works | approved deviation: no clicking through mBlock per robot; the CyberPi's upload protocol is known from MIT-licensed community tools |
| Robot bundle | one `main.py` built from `robot/src` | unclear whether mBlock uploads several files |
| Simulator | Python, runs the real bundled script against fakes of the vendor modules | tests the robot code itself, not a copy |
| Transport robot–server | MQTT, QoS 0, compact JSON | last will, retained state, easy to watch |
| Broker | mochi-mqtt, embedded in the server | no second process |
| Storage | SQLite with a pure Go driver | stays one binary |
| UI | web pages from the Go server, browser only | no software on the lab PCs |
| Live data in the UI | Server-Sent Events | pass corporate proxies reliably |
| Web assets | built with Vite and embedded, no CDN | lab PCs sit behind a corporate proxy |
| Deployment | server on an Azure VM, robots in a guest Wi-Fi, PCs in the corporate network | the corporate network stays untouched |
| Language | UI and lab material in German; code, comments and repo docs in English; task files in both | audience and common practice |
| Tasks | run through Nx (`nx:run-commands`), no Makefile | approved deviation: one cached task runner for Go, Python and web |

## Robots

| Topic | Decision | Why |
| --- | --- | --- |
| Reflexes | emergency stop, limits and watchdog run on the robot | the model needs seconds per decision |
| Nothing blocks | no fixed-distance vendor drive calls, no blocking sounds | a stop must arrive while a motor runs |
| Filtered sensors | every decision uses the filtered snapshot from `sense.py`, never a single raw reading | sensors flicker |
| Ownership | a robot always belongs to exactly one owner, stored per robot | no locking logic needed |
| Robots per team | the control room assigns 1..N robots per team; no profiles | approved deviation: flexible lab setups without YAML |
| Robot names | teams name their robots (default: German sticker color); `all`/`alle` reserved; a robot given back to a team gets the name that team gave it | approved deviation: agents use names, devices can be swapped; a team's prompt and notes keep working when its robot comes back |
| Heartbeat | carries `team`, `owner`, `name`, `color`, `agent` | approved deviation: owner changes reach the robot even when single QoS 0 messages get lost |
| `turn` sign | positive is right | matches compass headings (north 0, east 90) |
| Turn measurement | all turns via wheel encoders; gyro only for tilt | approved field deviation (`field.md`) |
| Line following | on the gray values of L1 and R1 | approved field deviation (`field.md`) |
| Floor colors | own RGB classification per probe against references; vendor color names as fallback | approved field deviation (`field.md`) |
| Calibration | values reach the robot in an MQTT `config` message, stored per robot | approved field deviation (`field.md`) |
| Result fields | `driven_cm` stays; `follow_line_until` stops with `crossing` at a crossing, as robot, registry and `protocol.md` report it (the rename to `junction` in `field.md` is not done); team tools and blocks use the reasons the registry reports and rename nothing | one name in robot, server, docs and the teams' tools |
| Grid freedom | robot script, server and web UI know nothing about the line grid; tools report floor letters and never interpret them | reading the field is what teams teach their agents |

## Agents and models

| Topic | Decision | Why |
| --- | --- | --- |
| Agents | one loop per team in the server, an actor with an inbox | events can wake the agent |
| Message sources | robot, own team, other agents, system; one envelope, trust by source | the prompt injection exercise needs unforgeable senders |
| Prompts | deployable by the team, versioned | comparison and rollback |
| Memory | working context, notes, protocol | learning across runs, material for the "Lügendetektor" |
| Tools | few powerful tools with built-in stop conditions, structured results with `reason` | the model decides at the corners, the robot drives in between |
| Language model | external and cheap over an OpenAI-compatible API, Mistral Small as default, Ollama as fallback | runs in parallel for all teams, no GPU machine |
| Claude | second provider (`kind: anthropic`) on the official Anthropic Go SDK | approved deviation, an exception to "no SDK for the language model"; Mistral stays the default |
| Model client | otherwise a lean HTTP client, no SDK | keeps providers interchangeable |
| MCP | optional second door onto the same tool registry | for external clients such as Open WebUI |

## Teams and lab

| Topic | Decision | Why |
| --- | --- | --- |
| Teams | join on `/join` with a lab code and get a secret link; not configured in YAML; another PC of the team joins with the link or with a join code (six characters, ten minutes, kept in memory) that only a PC with the link can show | approved deviation: teams form on the lab day; lab PCs share no clipboard, and a short-lived code does not give the link away for good |
| Secrets | never in YAML or the repo; config names environment variables (`*_env`); `robot/config.py` is git-ignored; the checked-in `server/.env` holds only the development placeholders for the local server, real values go into the git-ignored `server/.env.local` | no leaked keys; approved deviation: one env file pair instead of values in `server/project.json` |

## Data

| Topic | Decision | Why |
| --- | --- | --- |
| Scope | the pair (team, task) is the key of everything a team produces: prompt versions, notes, runs, conversations; finer scopes add robot and agent version | results per task and team need no guessing |
| Agent version | the prompt version (`prompts.id`), which also freezes the pair's team tools; a model change is not a new version, the run records the model | results can still be split by model |
| Schema changes for tasks | full reset, no data migration; an old database is refused with a clear message | tables in their final form, no `ALTER` chain |
| Deleting teams | soft delete (`deleted_at`) | results and history stay consistent |
| Published tasks | fixed: only drafts can be edited; to change one, duplicate it | all teams and the results page refer to the same text |
| Tasks are shared | one task row, assigned to teams through `team_tasks`; never copied per team | one text, comparable results |
| No task, nothing works | a team without an accepted task cannot deploy, start a run or talk with its agent; robots can still be assigned | everything a team produces belongs to a task |
| Active task | owned by the server (`active_tasks`), not the browser; switching is refused during a run | all PCs of a team follow a switch; a run keeps one task |
| Task text | for the team, never in the agent's context | turning the task into a prompt is what the teams learn |
| Event log | one table with the scope as real columns, one writer (`store.AddEvent`), written after the fact | results read one place at the finest scope; logging never blocks a robot or a model call |

## Team tools

Teams build their own tools from blocks in the browser, run them by hand and hand them to their
agent with the prompt. How it works: `architecture.md` and `programs.md`; how a lab day uses
them: `lab/werkzeuge.md`.

| Topic | Decision | Why |
| --- | --- | --- |
| Words | the page says "Werkzeuge" for built-in tools and the team's own ("eigenes Werkzeug"); "program" is the code word for the tree behind a team tool (`programs` package, `programs.md`); building one is "programmieren" | one place for tools on the page; one clear name in the code |
| One tool in the browser | blocks next to console and prompt, on the team page; no mBlock, no installation | lab PCs need a browser only |
| A block is a finished robot tool | team tools run in the server as a tree and never become MicroPython; the robot script does not change for them | the fast control loop stays on the robot |
| Language | sequence, `wiederhole n mal`, `wiederhole bis`, `wenn … sonst`, `gib zurück`; a condition is one comparison, optionally negated; no `und`/`oder` | enough for the learning path, small enough to read |
| A team tool is a tool | it has a name and a description and is called like a built-in tool; no catch-all `run_program(name)`, no separate release for the agent | teams learn what makes a good tool |
| Scope | team tools belong to a (team, task) pair like prompts; accepting a task with `copy` copies them too | results per task and team need no guessing |
| Versions | one deploy freezes the prompt and all team tools of the pair under one version; a deploy with the same text and team tools as the newest version creates none ("Nichts geändert seit Version N"); "zurück" restores both; the agent version stays `prompts.id` and covers the team tools | a run points at one version; comparison and rollback stay whole; identical versions only make the history longer |
| History | what changed in a team tool between two versions is shown as the old and the new blocks side by side, with marks | teams explain their changes in the debrief |
| No driver, no pause | by-hand calls and "Ausführen" work only while no run is active; a team tool the agent calls runs inside that call; the robot answers a second motion command with `busy`; stop always works | one rule instead of a driver role and a pause state |
| One writer per team | saving a team tool is "last write wins": no version check, no conflict dialog, no lock; other open PCs of the team reload what changed; the lab material recommends roles (`lab/werkzeuge.md`) | a team coordinates among its people; several PCs stay useful for reading along |
| By hand is a property of the tool | the registry flags which built-in tools a person or a team tool may use; a new tool is agent-only until flagged; console, blocks and interpreter read the registry and keep no tool list | one place decides, nothing drifts |
| Console | the "Werkzeuge" tab is the console; by-hand calls appear in the protocol as "Ihr" | no new tab; the team sees its own actions next to the agent's |
| The browser never runs anything | it shows and sends states; the server checks every rule | rules hold for every PC and every client |
| Error rule | a tool that ends normally (its reason is a normal end in the registry) lets the team tool go on; an unexpected end is handled only by an `if` on the result that names that reason or has an `else`; a loop condition never handles one; otherwise the team tool ends `aborted` | a failed `turn` must not disappear inside `wiederhole bis` |
| Last result | display and sound tools (`set_leds`, `show_text`, `play_sound`, `identify`) do not replace the "last result" | a light between a drive and its `wenn` must not change what the `wenn` tests |
| `gib zurück` | the return block is part of the first version: a team shapes what its tool reports | the agent reads what the team chose to tell it |
| Agent tool set | "the agent may only use the team's own tools" is an option of the task (`agent_tools`: `all` or `team_tools`), fixed when the task is published, not a lab-wide switch; with `team_tools` the agent keeps `read_sensors`, `say`, `finish`, `tell_team`, `remember` and `forget`, which move no robot | all teams in a challenge play by the same rule; what moves a robot is the team's |
| Sensor conditions | a sensor condition waits for a robot `state` newer than the last result before it compares | a reading from before the last motion is never used |
| Storage of by-hand work | by-hand calls and runs of team tools are stored under a `runs` row of kind `manual` of the pair; no tokens, no cost | the protocol of a task stays in one place |
| Tests in the browser | browser end-to-end tests (Playwright, `web:e2e`) run in CI next to `sim:e2e` | pages are checked the way teams use them |
| Unmerged tools stack | the branches `claude/tools-0-memory` … `claude/tools-6-leds` (`ask_human`, `led_effect`, moods for `say`) are not a prerequisite; a tool that lands later needs only its by-hand flag, its normal ends and one line of block text | blocks and console come from the registry |
| Unchanged | a robot belongs to exactly one team; emergency stop, limits and reflexes hold for team tools as for the agent; robot, server and control room know nothing about a grid; lab PCs need a browser only and load nothing from other hosts; with several robots a robot tool takes `robot` | the platform rules stay as they are |

## Documentation

| Topic | Decision | Why |
| --- | --- | --- |
| Long-term docs | `architecture.md`, `decisions.md`, `protocol.md`, `tools.md`, `events.md`, `programs.md` describe the current state; a PR that changes behaviour updates them | docs stay true without reading history |
| Protocol | `protocol.md` is the source of truth for MQTT; changing a message changes `docs/examples/protocol/*` too | server, robot and simulator test against the same examples |
| Field | `field.md` is the original field handoff (German), kept as written with the approved deviations on top | record of the field design |

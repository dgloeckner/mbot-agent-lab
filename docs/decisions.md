# Decisions

Binding decisions for the mBot2 Agent Lab, each with a one-line why. Deviations are proposed
first and only implemented once approved; an approved deviation replaces the entry it changes
and goes here, not into `CLAUDE.md`. How the system works today: `architecture.md`.

## Platform

| Topic | Decision | Why |
| --- | --- | --- |
| Repository | `mbot-agent-lab`, one monorepo | everything in one place |
| Server | Go, one binary `labserver` | embeddable broker, simple distribution |
| Robot code | MicroPython in upload mode, uploaded with mBlock | the only option on the CyberPi |
| Robot bundle | one `main.py` built from `robot/src` | unclear whether mBlock uploads several files |
| Simulator | Python, runs the real bundled script against fakes of the vendor modules | tests the robot code itself, not a copy |
| Transport robot–server | MQTT, QoS 0, compact JSON | last will, retained state, easy to watch |
| Broker | mochi-mqtt, embedded in the server | no second process |
| Storage | SQLite with a pure Go driver | stays one binary |
| UI | web pages from the Go server, browser only | no software on the lab PCs |
| Live data in the UI | Server-Sent Events | pass corporate proxies reliably |
| Web assets | built with Vite and embedded, no CDN | lab PCs sit behind a corporate proxy |
| Deployment | server on an Azure VM, robots in a guest Wi-Fi, PCs in the corporate network | the corporate network stays untouched |
| Language | UI and lab material in German; code, comments and repo docs in English; example prompts in both | audience and common practice |
| Tasks | run through Nx (`nx:run-commands`), no Makefile | approved deviation: one cached task runner for Go, Python and web |

## Robots

| Topic | Decision | Why |
| --- | --- | --- |
| Reflexes | emergency stop, limits and watchdog run on the robot | the model needs seconds per decision |
| Nothing blocks | no fixed-distance vendor drive calls, no blocking sounds | a stop must arrive while a motor runs |
| Filtered sensors | every decision uses the filtered snapshot from `sense.py`, never a single raw reading | sensors flicker |
| Ownership | a robot always belongs to exactly one owner, stored per robot | no locking logic needed |
| Robots per team | the control room assigns 1..N robots per team; no profiles | approved deviation: flexible lab setups without YAML |
| Robot names | teams name their robots (default: German sticker color); `all`/`alle` reserved | approved deviation: agents use names, devices can be swapped |
| Heartbeat | carries `team`, `owner`, `name`, `color`, `agent` | approved deviation: owner changes reach the robot even when single QoS 0 messages get lost |
| `turn` sign | positive is right | matches compass headings (north 0, east 90) |
| Turn measurement | all turns via wheel encoders; gyro only for tilt | approved field deviation (`field.md`) |
| Line following | on the gray values of L1 and R1 | approved field deviation (`field.md`) |
| Floor colors | own RGB classification per probe against references; vendor color names as fallback | approved field deviation (`field.md`) |
| Calibration | values reach the robot in an MQTT `config` message, stored per robot | approved field deviation (`field.md`) |
| Result fields | `driven_cm` stays; the stop reason `crossing` becomes `junction` | approved field deviation (`field.md`) |
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
| Teams | join on `/join` with a lab code and get a secret link; not configured in YAML | approved deviation: teams form on the lab day |
| Secrets | never in YAML or the repo; config names environment variables (`*_env`); `robot/config.py` is git-ignored | no leaked keys |

## Data

| Topic | Decision | Why |
| --- | --- | --- |
| Scope | the pair (team, task) is the key of everything a team produces: prompt versions, notes, runs, conversations; finer scopes add robot and agent version | results per task and team need no guessing |
| Agent version | the prompt version (`prompts.id`); a model change is not a new version, the run records the model | results can still be split by model |
| Schema changes for tasks | full reset, no data migration; an old database is refused with a clear message | tables in their final form, no `ALTER` chain |
| Deleting teams | soft delete (`deleted_at`) | results and history stay consistent |
| Published tasks | fixed: only drafts can be edited; to change one, duplicate it | all teams and the results page refer to the same text |
| Tasks are shared | one task row, assigned to teams through `team_tasks`; never copied per team | one text, comparable results |
| Event log | one table with the scope as real columns, one writer (`store.AddEvent`), written after the fact | results read one place at the finest scope; logging never blocks a robot or a model call |

## Documentation

| Topic | Decision | Why |
| --- | --- | --- |
| Long-term docs | `architecture.md`, `decisions.md`, `protocol.md`, `tools.md`, `events.md` describe the current state; a PR that changes behaviour updates them | docs stay true without reading history |
| Protocol | `protocol.md` is the source of truth for MQTT; changing a message changes `docs/examples/protocol/*` too | server, robot and simulator test against the same examples |
| Field | `field.md` is the original field handoff (German), kept as written with the approved deviations on top | record of the field design |

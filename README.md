# mBot2 Agent Lab

A lab in which teams of 15-year-olds instruct AI agents that drive real robots.

Each team writes and deploys the prompt of its agent. The agent runs as a loop inside the server,
controls the team's Makeblock mBot2 through a small set of tools and reacts to messages from the
robot, from its team and from other agents. Students only need a browser.

Learning goals: instructing precisely, recognising failures and limits of agents, designing
tools, understanding attacks such as prompt injection, and treating tokens as a cost.

> Repository, code and technical docs are in English. Lab material and the student-facing UI are
> in German; example prompts exist in both languages.

## Architecture

```
 Lab PCs (browser) ──HTTPS/SSE──┐
                                ▼
                    ┌──────────────────────────┐        OpenAI-compatible API
                    │ labserver (one Go binary)│ ─────▶ (Mistral, Ollama, …)
                    │  web · agents · tools    │
                    │  robots · store (SQLite) │
                    │  embedded MQTT broker    │
                    └──────────▲───────────────┘
                               │ MQTT (TLS), QoS 0, compact JSON
                 ┌─────────────┴─────────────┐
           mBot2 + CyberPi             simulated robots
           (MicroPython)               (same script, fake hardware)
```

- **Robot** (`robot/`): one MicroPython script per robot, uploaded via mBlock. Non-blocking main
  loop with skills as state machines, local reflexes (emergency stop, watchdog, tilt) and events.
- **Server** (`server/`): broker, robot registry, agent runtime, model access, storage and web UI.
- **Team page** (`web/`): Preact and CodeMirror 6, built with Vite and embedded into the server
  binary. No CDN, so it works behind corporate proxies.
- **Simulator** (`sim/`): runs the real bundled robot script under CPython with fakes for the
  vendor modules and a 2D world. Talks real MQTT to the server.
- **Protocol** (`docs/protocol.md`): the single source of truth, with shared example messages
  that server, robot and simulator are all tested against.

## Quick start (no hardware needed)

Requirements: Go (see `server/go.mod`), Python 3.10+, Node.js 20.19+ (build tooling and team page).

```bash
npm install
```

Start the server and the team page dev server (hot reload):

```bash
npm run dev
```

In a second terminal:

```bash
npx nx run sim:run --robots=2 --world=sim/worlds/labyrinth.json
```

- Team page for "Team Rot": http://localhost:5173/t/rot-dev (Vite, hot reload) or
  http://127.0.0.1:8080/t/rot-dev (embedded build, as on the VM).
- Robot overview: http://127.0.0.1:8080

```bash
npx nx run-many -t lint test
```

## Tasks

Tasks run through [Nx](https://nx.dev). The workspace has four projects: `server`, `web`, `robot`
and `sim`. Server builds depend on `web:build`, so the binary always embeds the current team page. Results of `lint`, `test` and the builds are cached, and `npx nx affected -t test` only
tests what changed compared to `main`.

| Command | What it does |
| --- | --- |
| `npx nx run server:dev` | server with `deploy/local/lab.yaml` and dev secrets (HTTP 127.0.0.1:8080, MQTT :1883) |
| `npx nx run sim:run [--robots=2] [--world=…]` | simulated robots against the local server |
| `npm run dev` | `server:dev` and `web:dev` together |
| `npx nx run web:dev` | team page with hot reload on :5173, API calls go to the Go server |
| `npx nx run-many -t lint test` | gofmt, go vet, TypeScript, Go tests, web tests, robot and simulator tests |
| `npx nx run robot:bundle [--config=robot/config.py]` | build `robot/dist/main.py` |
| `npx nx run server:build` / `server:build-linux` | build the server into `bin/` |
| `HOST=user@vm npx nx run server:deploy` | build for linux/amd64, copy to the VM, restart |
| `npx nx graph` | show projects and task dependencies |

## Real robots

1. Copy `robot/config_example.py` to `robot/config.py` and fill in robot ID, Wi-Fi (2.4 GHz) and
   broker credentials. `config.py` is git-ignored.
2. Bundle the script with the config inlined:

   ```bash
   npx nx run robot:bundle --config=robot/config.py
   ```

3. Upload `robot/dist/main.py` to the CyberPi with mBlock in upload mode.

All vendor API calls live in `robot/src/hw.py` and are marked `VERIFY` until checked on hardware.

## Repository layout

| Path | Content |
| --- | --- |
| `docs/` | handoff, protocol, tool catalog, example messages |
| `docs/lab/` | lab material (German), example prompts (German and English) |
| `robot/` | robot script sources, bundler, config example |
| `sim/` | simulator: fakes, world, worlds, tests |
| `server/` | Go server (`cmd/labserver`, `internal/*`) |
| `web/` | team page (Preact, CodeMirror, Vite) |
| `configs/` | example lab configuration |
| `deploy/` | Azure VM, systemd unit, local development config |

## Team page

Each team opens its secret link `/t/<token>`. The page has the team's color, the same color the
robot's LEDs show, and the color band pulses in the robot's rhythm while the agent is thinking.

- **Prompt editor:** Markdown source with highlighting, search, undo, list continuation and
  completion of tool names. Shortcuts: `Ctrl/⌘ + Enter` deploys, `Ctrl/⌘ + S` keeps a snapshot.
- **Preview** of the rendered Markdown and **history** with a side-by-side diff against any
  deployed version or local snapshot, plus restore.
- **Live log** over Server-Sent Events: robot events, deploys, messages, system notices. A
  reconnecting browser catches up via `Last-Event-ID`.
- **Notes** the agent sees every round, and the **tool catalog** with one-click insertion.
- **Browser storage** (`localStorage`, per team): the unsaved draft, up to 80 draft snapshots and
  UI preferences (theme, split, font size). Deployed versions live on the server. If another PC of
  the team deploys while this draft has changes, the page offers to take over the new version.

## Status

Initial scaffold. Working: configuration with validation, protocol types, embedded broker with
per-robot auth and ACL, robot registry with heartbeat and command/result matching, SQLite schema,
status page, team page with prompt versions, notes, stop and live log, robot script with `drive`,
`turn`, `drive_until`, `stop`, reflexes and events, bundler, simulator. Not yet: agent runtime, LLM
client, tool registry, control room, chat wall, MCP.

## License

Apache 2.0, see [LICENSE](LICENSE).

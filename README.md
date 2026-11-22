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
- **Simulator** (`sim/`): runs the real bundled robot script under CPython with fakes for the
  vendor modules and a 2D world. Talks real MQTT to the server.
- **Protocol** (`docs/protocol.md`): the single source of truth, with shared example messages
  that server, robot and simulator are all tested against.

## Quick start (no hardware needed)

Requirements: Go (see `server/go.mod`), Python 3.10+, Node.js 20+ (only for the Nx task runner).

```bash
npm install
```

```bash
npx nx run server:dev
```

In a second terminal:

```bash
npx nx run sim:run --robots=2 --world=sim/worlds/labyrinth.json
```

Open http://127.0.0.1:8080: both simulated robots show up as online.

```bash
npx nx run-many -t lint test
```

## Tasks

Tasks run through [Nx](https://nx.dev). The workspace has three projects: `server`, `robot` and
`sim`. Results of `lint`, `test` and the builds are cached, and `npx nx affected -t test` only
tests what changed compared to `main`.

| Command | What it does |
| --- | --- |
| `npx nx run server:dev` | server with `deploy/local/lab.yaml` and dev secrets (HTTP 127.0.0.1:8080, MQTT :1883) |
| `npx nx run sim:run [--robots=2] [--world=…]` | simulated robots against the local server |
| `npx nx run-many -t lint test` | gofmt and go vet, Go tests, robot and simulator tests |
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
| `configs/` | example lab configuration |
| `deploy/` | Azure VM, systemd unit, local development config |

## Status

Initial scaffold. Working: configuration with validation, protocol types, embedded broker with
per-robot auth and ACL, robot registry with heartbeat and command/result matching, SQLite schema,
status page, robot script with `drive`, `turn`, `drive_until`, `stop`, reflexes and events,
bundler, simulator. Not yet: agent runtime, LLM client, tools, team page, control room, SSE, MCP.

## License

Apache 2.0, see [LICENSE](LICENSE).

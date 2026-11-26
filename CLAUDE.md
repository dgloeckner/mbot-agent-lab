# CLAUDE.md

mBot2 Agent Lab: teams of teenagers prompt AI agents that drive real Makeblock mBot2 robots.
How the system works: `docs/architecture.md`. Binding decisions and approved deviations:
`docs/decisions.md`. The line field with colored stripes: `docs/field.md` (field handoff, German,
with approved deviations on top).

## Ground rules

- Decisions in `docs/decisions.md` are binding. Propose deviations first; do not implement them
  silently. An approved deviation goes into `docs/decisions.md`, not into this file.
- Long-term docs (`docs/architecture.md`, `docs/decisions.md`, `docs/protocol.md`,
  `docs/tools.md`, `docs/events.md`, `docs/programs.md`) describe the current state, in English,
  without history. A PR that changes behaviour updates them in the same PR.
- Grid freedom: robot script, server and web UI know nothing about the line grid (no junction
  coordinates, pitch or color code). Tools report floor letters and never interpret them. Field
  geometry lives only in `sim/grid.py`, the world files and `sim/tests`; reading rules live in
  `docs/lab/` and the task files.
- Language: repo docs, code and comments in English. Lab material (`docs/lab/`) and student-facing
  UI text (web templates, robot display) in German. Task files (task text and starter prompt)
  in German and English, same file name under `docs/lab/aufgaben/de/` and `docs/lab/aufgaben/en/`;
  a web test parses all of them.
- `docs/protocol.md` is the source of truth for MQTT. Changing a message means changing
  `docs/examples/protocol/*` too; Go and Python tests read those files.
- Secrets never go into YAML or the repo: config files only name environment variables
  (`*_env`). `robot/config.py` is git-ignored.

## Layout

- `server/`: Go module `github.com/dgloeckner/mbot-agent-lab/server`, one binary `labserver`
  (`cmd/labserver`) plus the setup CLI `cmd/labctl` (macOS; upload protocol in `internal/cyberpi`).
  Packages in `internal/`: config, protocol, broker (mochi-mqtt, auth/ACL), robots (registry,
  heartbeat, cmd/result, reassignment), store (SQLite via modernc, migrations in
  `store/migrations`), lab (joining, teams, assignments, robot names, lab code: the rules live
  here, web handlers stay thin), live (per-team log hub for SSE), web.
  agent (one actor per team: inbox, wake rules, context, budgets, runs, by-hand calls), programs
  (team tools: tree, validation, interpreter; knows no tool names), tools (registry with
  model-facing German descriptions and JSON schemas), llm (OpenAI-compatible client, Claude via
  the official Anthropic Go SDK, retries, fallback, cost; `Fake` scripted test model), memory
  (history trimming, notes formatting). Messages stay in the OpenAI format; a provider keeps its
  own form of an assistant turn in `Message.Native` (Claude's thinking blocks must be replayed
  unchanged).
  Stubs: auth, mcp.
- `robot/src/`: MicroPython modules, bundled in dependency order by `robot/tools/bundle.py` into
  one `main.py`. All modules share one namespace after bundling: no duplicate top-level names,
  import local names with `from module import name`. Only `hw.py` touches vendor modules.
  Only `sense.py` turns raw readings into the filtered snapshot every decision uses; never decide
  on a single raw reading (sensors flicker). New modules go into `MODULES` in `bundle.py`.
  The bundler strips comments and docstrings; the size budget is tight, so keep code lean.
- `web/`: team page (Preact + signals, CodeMirror 6, Vite). `vite build` writes to
  `server/internal/web/dist/app`, which the server embeds; `dist/README.md` keeps the embed
  compiling without a build. No CDN assets: lab PCs sit behind a corporate proxy. Keep the
  initial bundle lean: rarely used views (preview, history) are lazy-loaded via `web/src/lazy.tsx`,
  and use `markdownLanguage` instead of `markdown()` (which drags in HTML/CSS/JS parsers).
  Browser storage is only for drafts, draft snapshots and UI prefs (`web/src/storage.ts`).
- `sim/`: fakes for `cyberpi`, `mbot2`, `mbuild` in `sim/fakes/` (MQTT is real: the robot bundles
  its own client, `robot/src/mqtt.py`, because the CyberPi firmware has no `umqtt`); 2D world in
  `sim/world.py` with optional sensor noise (seeded `random.Random`); the floor is a drawing of
  tape bands seen by four probes. Assumed robot geometry in `sim/mbot2.json` (replace with
  measurements). `sim/grid.py` generates `sim/worlds/gitter.json` and `docs/lab/feld/bauplan.svg`;
  never edit those by hand. Tests in `sim/tests/` run the real bundle in-process. Robot behaviour
  that depends on sensors is tested with noise on.

## Robot script constraints

- Nothing may block while a motor runs: no fixed-distance vendor drive calls, no blocking sounds.
- Keep MicroPython-compatible: no f-strings, dataclasses, typing; `ticks_ms`/`ticks_diff` from
  `compat`. Keep messages under 512 bytes and the bundle under the size budget in the tests.

## Commands

Tasks run through Nx (`nx:run-commands` in `server/`, `robot/`, `sim/` `project.json`; no Make,
the user chose Nx over Make). Run `npm install` once.

- `npx nx run-many -t lint test`: gofmt + go vet, Go tests, Python tests. Cached.
- `npm run dev`: `server:dev` + `web:dev`. Control room http://localhost:5173/admin (password
  `dev`), join http://localhost:5173/join (lab code `DEV1`).
- `npx nx run server:dev`: server with `deploy/local/lab.yaml` (HTTP 127.0.0.1:8080, MQTT :1883,
  dev secrets set in `server/project.json`). Real API keys (`ANTHROPIC_API_KEY`, ...) live in
  git-ignored `server/.env.local`, which Nx loads; never read or print its values.
- `npx nx run sim:run --robots=2 --world=sim/worlds/labyrinth.json`: simulated robots; `--delay-ms=150`
  delays every packet like the guest Wi-Fi. Challenge worlds: `labyrinth_rechts`, `labyrinth_gestoert`.
- `npx nx run sim:e2e`: end-to-end smoke test (`sim/e2e/`): builds and starts `labserver` with the
  scripted model, one seeded sim robot, drives join/assign/prompt/start over HTTP. Runs in CI.
- `npx nx run web:e2e`: browser end-to-end tests (`web/e2e/`, Playwright, Chromium): the same lab as
  `sim:e2e` (built `labserver`, scripted model, simulated robots), driven through the team page.
  Screenshots go to the git-ignored `web/e2e/screenshots/`. Runs in CI.
- `npx nx run robot:bundle [--config=robot/config.py]`: build `robot/dist/main.py`.
- `npx nx run robot:flash --robot=mbot-1 [--server=https://…] [--port=/dev/cu.…]`: get the
  robot's setup from the server's admin API, bundle, upload over USB, wait until online (macOS;
  battery on, mBlock closed). `robot:upload --file=…` uploads a file as it is; `robot:usb` lists USB
  devices and serial ports. Not cached, not in CI.
- `npx nx run sim:grid`: regenerate the line field world and build plan after changing `sim/grid.py`.
- `go test -race ./...` in `server/` before touching agent, robots or tools: they are concurrent.
- New inputs a task depends on (e.g. shared example files) must be listed in its `inputs`, or the
  Nx cache returns stale results.

# CLAUDE.md

mBot2 Agent Lab: teams of teenagers prompt AI agents that drive real Makeblock mBot2 robots.
Full background and all decisions: `docs/handoff.md` (original handoff, German).

## Ground rules

- Decisions in `docs/handoff.md` ("Entscheidungen im Überblick") are binding. Propose deviations
  first; do not implement them silently.
- Language: repo docs, code and comments in English. Lab material (`docs/lab/`) and student-facing
  UI text (web templates, robot display) in German. Example prompts in German and English, same
  file name under `docs/lab/prompts/de/` and `docs/lab/prompts/en/`.
- `docs/protocol.md` is the source of truth for MQTT. Changing a message means changing
  `docs/examples/protocol/*` too; Go and Python tests read those files.
- Secrets never go into YAML or the repo: config files only name environment variables
  (`*_env`). `robot/config.py` is git-ignored.

## Layout

- `server/`: Go module `github.com/dgloeckner/mbot-agent-lab/server`, one binary `labserver`.
  Packages in `internal/`: config, protocol, broker (mochi-mqtt, auth/ACL), robots (registry,
  heartbeat, cmd/result), store (SQLite via modernc, migrations in `store/migrations`), web.
  Stubs with doc comments: tools, agent, llm, memory, auth, mcp.
- `robot/src/`: MicroPython modules, bundled in dependency order by `robot/tools/bundle.py` into
  one `main.py`. All modules share one namespace after bundling: no duplicate top-level names,
  import local names with `from module import name`. Only `hw.py` touches vendor modules.
- `sim/`: fakes for `cyberpi`, `mbot2`, `mbuild`, `umqtt.simple` in `sim/fakes/`; 2D world in
  `sim/world.py`; tests in `sim/tests/` run the real bundle in-process.

## Robot script constraints

- Nothing may block while a motor runs: no fixed-distance vendor drive calls, no blocking sounds.
- Keep MicroPython-compatible: no f-strings, dataclasses, typing; `ticks_ms`/`ticks_diff` from
  `compat`. Keep messages under 512 bytes and the bundle under the size budget in the tests.

## Commands

Tasks run through Nx (`nx:run-commands` in `server/`, `robot/`, `sim/` `project.json`; no Make,
the user chose Nx over Make). Run `npm install` once.

- `npx nx run-many -t lint test`: gofmt + go vet, Go tests, Python tests. Cached.
- `npx nx run server:dev`: server with `deploy/local/lab.yaml` (HTTP 127.0.0.1:8080, MQTT :1883,
  dev secrets set in `server/project.json`).
- `npx nx run sim:run --robots=2 --world=sim/worlds/labyrinth.json`: simulated robots.
- `npx nx run robot:bundle [--config=robot/config.py]`: build `robot/dist/main.py`.
- New inputs a task depends on (e.g. shared example files) must be listed in its `inputs`, or the
  Nx cache returns stale results.

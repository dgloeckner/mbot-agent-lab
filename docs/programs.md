# Team tools (programs)

A team builds its own tools ("eigene Werkzeuge") from blocks. Behind each team tool is a
**program**: a tree of blocks that the server validates and walks. A program never becomes
MicroPython: every call is a finished robot tool of the registry (`docs/tools.md`), and the fast
control loop stays on the robot. Why: `decisions.md`, "Team tools".

Implementation: `server/internal/programs` (tree, validation, interpreter). The package knows no
tool names: whether a tool may run by hand, its schema, its normal ends and whether it is a
display or sound tool come from the tool registry.

## Tree

A program has a `name`, a `description` (what the agent reads) and a `body`, a list of nodes.
Every node carries the `id` of its block in the editor.

| Node | Fields | Meaning |
| --- | --- | --- |
| `call` | `tool`, `args` | one by-hand tool with fixed arguments; the robot is chosen when the program runs |
| `repeat` | `times` (1 to 20), `body` | "wiederhole n mal" |
| `repeat_until` | `cond`, `body` | "wiederhole bis": the body first, then the condition; at most 50 rounds |
| `if` | `cond`, `body`, `else?` | "wenn … sonst"; without `else` nothing happens when the condition is false |
| `return` | `return`: `{field}` or `{text}` | "gib zurück": ends the program with a field of the last result or a fixed text |

A **condition** (`cond`) is one comparison, optionally negated (`not`), with its own block `id`:

| Field | Values |
| --- | --- |
| `source` | `result`: a field of the last result; `sensor`: a fresh reading of the robot |
| `field` | result: `reason`, `driven_cm`, `turned_deg`, `distance_cm`, `min_distance_cm`, `color`, `floor`, `best_deg`, `best_cm`, `event`, `ok`; sensor: `distance_cm`, `floor`, `color`, `line` |
| `op` | `eq`, `ne`, `lt`, `gt` (numbers), `contains` (`floor` only: a letter in the pattern or anywhere in the trace) |
| `value` | text or number |

There is no `and` and no `or`. A missing field never matches.

Examples, read by the tests: `docs/examples/programs/` (`quadrat`, `labyrinth`, `wand`). Each
file holds the Blockly state of the tool (`blockly`) and the tree the server derives from it
(`program`); see "Blocks".

```json
{"id": "l_loop", "type": "repeat_until",
 "cond": {"id": "l_until", "source": "result", "field": "reason", "op": "eq", "value": "color"},
 "body": [
  {"id": "l_drive", "type": "call", "tool": "drive_until", "args": {"max_cm": 300, "stop_below_cm": 15, "stop_on_color": "blue"}},
  {"id": "l_if", "type": "if",
   "cond": {"id": "l_wall", "source": "result", "field": "reason", "op": "eq", "value": "wall"},
   "body": [{"id": "l_turn", "type": "call", "tool": "turn", "args": {"degrees": 90}}]}
 ]}
```

## Validation

`programs.Validate` checks name, description and tree and answers a list of problems, each with
the block id (empty for name and description) and a German text. An invalid program can be
stored but not run.

- **Tools:** only tools the registry flags as by hand. An agent-only tool, an unknown tool or a
  `robot` argument in a block is refused. Arguments are checked against the tool's schema:
  required ones, type, enum, range and length.
- **Limits:**

  | Limit | Value |
  | --- | --- |
  | Blocks per team tool (conditions count) | 60 |
  | Nesting depth (`repeat`, `repeat_until`, `if` inside each other) | 4 |
  | `wiederhole n mal` | n from 1 to 20 |
  | Name | lower-case letters, digits, underscore; 3 to 30 characters; not the name of a built-in or planned tool |
  | Description | up to 200 characters |
  | Text of `gib zurück` | up to 200 characters |

- Block ids are unique; an empty program is refused.

## Interpreter

`programs.Run(ctx, program, caller, options)` walks a valid program. The **caller** runs a tool
(and reports whether its end is normal, whether it is a display or sound tool, and whether it is
a hard error) and reads the sensors; the **observer** (`Options.Observe`) hears every step.

- **Last result:** the result of the last tool that ran, except display and sound tools
  (`set_leds`, `show_text`, `play_sound`, `identify`): a light between a drive and its "wenn"
  does not change what the "wenn" tests.
- **Error rule:** a tool that ends normally (its reason is one of the tool's normal ends, or it
  is ok without a reason) lets the program go on. An unexpected end (for example `drive` with
  `obstacle`) is handled only if the next thing evaluated is an `if` on the last result that
  names this reason or has an `else`. A loop condition never handles one, and neither does an `if`
  on a sensor. Otherwise the program ends `aborted` and names tool, reason and block.
- **Hard errors** always abort, whatever follows: robot offline or not answering, `reassigned`,
  stopped, the run's time used up.
- **Steps:** every call is a step; a sensor condition reads once and counts as a step too.
- **Single step** (`Options.Step`): the run halts before every call (event `wait`) until it is
  told to go on.
- **Limits of a run:** 200 steps, 120 s, 50 rounds of `wiederhole bis`.

### Ends and result

| End | When |
| --- | --- |
| `done` | the program ran to its end or to `gib zurück` |
| `aborted` | an unexpected end nobody handled, or a hard error |
| `limit` | 200 steps, 120 s or 50 rounds used up |
| `stopped` | stopped by the team or the control room |

The result is `{ok, end, steps, runtime_s, at, last}`: `ok` is true for `done`; `at` names the
block the program ended at in the words of the editor; `last` is the last tool's result only
(earlier steps stay out). An abort adds `tool`, `reason` and `error` (a German sentence);
`gib zurück` adds `value`. `block` is the id of the block in `at`.

### Events

The observer gets `{phase, step, block, kind, tool, args, round, result, ms}` for every step:
`before` and `after` each call (`kind: call`) and sensor reading (`kind: sense`), and `wait` in
single step. `round` is the round of the innermost loop, from 1.

## Working state

A team tool belongs to a (team, task) pair like prompts and notes (table `programs`): a stable
id, its name (unique per pair), description, the Blockly state the team edits, and the tree and
problems the server derived from it. At most 12 per pair. Accepting a task with the prompt
choice `copy` copies the team tools of the other task too, with new ids.

- **Team routes** (`docs/architecture.md`): list, read, create, replace, delete. The browser sends
  `{name, description, blockly}` and never a tree; the server converts and validates (see
  "Blocks"), stores the tool even when it is invalid and answers the problems per block.
- **Last write wins.** A team has one writer and coordinates among its people (`decisions.md`):
  no version check, no conflict answer, no lock. Every save or delete sends the SSE event
  `programs` with the tool's id, so other open PCs of the team reload it.
- Without an active task the routes answer 409; in a closed task reads work and writes answer 409.
- The server logs `team tool saved` (team, task, id, name, valid, problems) and `team tool deleted`.

## Running a team tool

"Ausführen": `POST /t/{token}/programs/{id}/run` with `{robot?, step?}` starts a run of the
tool's tree on one of the team's robots and answers its run id (`agent.Manager.RunProgram`).

- **Rules** (409 with a German message): not while an agent run is active, not for an invalid
  tool, not while another team tool of the team runs; `POST start` answers 409 while a team tool
  runs. With several robots `robot` is required. By-hand calls stay possible beside it; the robot
  answers a second motion command with `busy`.
- **Caller:** every step goes through `tools.Registry.CallManual`, so ownership, argument checks
  and logging are the same as in the console. Display and sound tools are quiet; `offline`,
  `reassigned`, `stopped` and a `timeout` that is not the tool's normal end are hard errors.
- **Sensor conditions** wait (at most 2 s) for a robot `state` that arrived after the last result
  (`robots.Registry.FreshState`), so a reading from before the last motion is never used.
- **Stop:** `POST stop` cancels the run at once; the running command ends `stopped` and the robot
  gets `stop`. Switching or closing the task stops a running team tool too.
- **Single step:** with `step: true` the run halts before every call (event `wait`);
  `POST /t/{token}/program-runs/{id}/continue` ("Weiter") lets it take the next call.
- **SSE `program`:** `{run, program, phase, step, block, kind, tool, args, round, result, ms}` for
  `start`, `wait`, `before`, `after` of every step, and `{run, program, phase: "end", result}` at
  the end. The team page marks the running block and builds the "Ablauf" from these. Not kept in
  the backlog.
- **Log and storage:** start and end as "Ihr" lines (`program_start`, `program_end`) and every
  step as a tool line with the tool's name (`data.program`). They are stored under the `manual`
  row of the pair: start and end as messages, each step as a tool call that names the team tool
  (`tool_calls.program`). `INFO` lines `team tool started`, `team tool step`, `team tool ended`.

## Editor

The team page builds team tools in the left pane ("Werkzeug bauen"); the side pane stays, so a
team can try a built-in tool on the right while it builds on the left.

- **Blockly** is a lazy-loaded part of the build (`web/src/components/ToolEditor.tsx`), German
  locale, with its cursors and icons served by the server under `/assets/blockly-media/` (a Vite
  plugin copies them from the package): nothing is fetched from another host. The block
  definitions and the toolbox come from `GET blocks`; the browser keeps no tool list.
- **Saving** is automatic 1.5 s after the last change (blocks, name or description), last write
  wins. A `programs` event for the open tool from another PC reloads it, unless a change of this
  PC is still waiting to be saved.
- **Problems:** a block the server refuses gets a red outline and a warning icon with the reason;
  problems of name or description stand below the workspace.
- **Running:** "Ausführen" and "Einzelschritt" save first and start the run; the `program` events
  mark the running block (`highlightBlock`) and fill the "Ablauf": every step with its result in
  words, the rounds of a loop as separator lines, the end with where and why it stopped. In single
  step "Weiter" replaces "Ausführen". "Stopp" is the team's stop.
- While an agent run is active "Ausführen" and "Einzelschritt" are disabled ("Erst anhalten"); a
  closed task is read-only. The browser never runs anything: it shows and sends states.
- Pure helpers with tests: `web/src/toolrun.ts` (events to "Ablauf", the saver, problems per block).

## Blocks

Two representations, one source: the browser stores the **Blockly state** of a team tool, the
server turns it into the tree (`programs.Convert`, with the validation above) and runs only the
tree. The browser never sends a tree.

- **Blockly:** version 13.4.0, pinned in `package.json`. License Apache-2.0. The project belongs to
  the Raspberry Pi Foundation (repository `RaspberryPiFoundation/blockly`, previously Google). It is
  part of the web build; nothing is loaded from another host.
- **Definitions:** `GET /t/{token}/blocks` answers Blockly's JSON block definitions and the
  toolbox (`programs.Blocks`). Tool blocks are built from the registry (by-hand tools, their
  schemas) and the block text table in `server/internal/tools/blocks.go` (`docs/tools.md`, "Block
  texts"); control and condition blocks are fixed in `server/internal/programs/blocks.go`. Block
  texts are German; block and field names in JSON stay English. A block has no robot field: the
  robot is chosen when the program runs.
- **Toolbox:** Fahren, Sensoren, Licht und Anzeige (tool blocks by group), Wiederholen
  (`wiederhole n mal`, `wiederhole … bis`), Entscheiden (`wenn`, `wenn … sonst`, the conditions,
  `nicht`, `gib zurück`).

| Block | Type | Becomes |
| --- | --- | --- |
| Wenn das Werkzeug startet | `program_start` | the start; the program is the chain below it |
| a tool's sentence, e.g. "fahre %1 cm" | `tool_<name>` | `call`; "rechts"/"links" set the sign of `degrees`; "egal", "zur Kreuzung" leave an argument out |
| wiederhole %1 mal | `repeat_times` | `repeat` |
| wiederhole … bis %1 | `repeat_until` | `repeat_until` |
| wenn … dann | `if` | `if` without `else` |
| wenn … dann … sonst | `if_else` | `if` with `else` |
| gib zurück: … des letzten Ergebnisses / gib zurück: Text | `return_field`, `return_text` | `return` |
| Grund ist / ist nicht … | `cond_reason` | result `reason` `eq`/`ne` |
| Ergebnis: Feld < > = ≠ Zahl | `cond_result_number` | result number comparison |
| Bodenfarbe im Ergebnis ist … | `cond_result_color` | result `color` |
| Bodenspur im Ergebnis enthält … | `cond_result_floor` | result `floor` `contains` |
| Abstand nach vorn jetzt … cm | `cond_sensor_distance` | sensor `distance_cm` |
| Bodenfarbe jetzt ist … | `cond_sensor_color` | sensor `color` |
| Boden jetzt enthält … | `cond_sensor_floor` | sensor `floor` `contains` |
| Linie jetzt ist … | `cond_sensor_line` | sensor `line` |
| nicht … | `cond_not` | toggles `not` of the condition inside |

**Conversion** refuses, each at its block: an unknown block type, an agent-only tool as a block,
blocks that hang on nothing beside the program, and more than one start. Values out of range are
refused by the validation, also at their block. Block ids become node ids, so problems and the
running block are marked in the editor.

**Shared examples:** `docs/examples/programs/blocks.json` pins the served definitions (a Go test
compares it; `UPDATE_GOLDEN=1 go test ./internal/programs` rewrites it). A `vitest` test defines
those blocks in a headless Blockly workspace, loads each example state and checks that Blockly
saves exactly the stored state; the Go tests convert the same states into the stored trees. So
the Go tests convert real Blockly output.

**Words in results:** `at` names a tool block by its sentence with its values ("fahre 30 cm",
"drehe 90 Grad nach links"; `programs.Describer`).

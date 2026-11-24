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

Examples, read by the tests: `docs/examples/programs/` (`quadrat`, `labyrinth`,
`wand_ausweichen`). Each file holds `{"program": …}`.

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

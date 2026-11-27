# Tool catalog

Agents get a few powerful tools with built-in stop conditions instead of a one-to-one mapping of
the vendor API. The model decides at the corners; the robot drives on its own in between.

Implementation: `server/internal/tools` (registry with name, German description for the model,
JSON schema and handler). The agent calls them directly; the optional MCP endpoint will expose the
same registry to external clients.

Goal-oriented tools keep real-time decisions on the robot: the agent states the goal ("turn until
the way is clear", "follow the line to blue"), the robot closes the loop with filtered sensors in
its 40 Hz main loop and reports a structured result. The ultrasonic sensor only looks forward, so
the goals are about driving, turning and scanning, not about following walls on the side.

Not offered to agents yet, because the robot does not implement them: `play_melody`,
`gesture`. The team page lists them as "kommt bald". `set_leds`
takes a color only, no pattern.

| Group | Tool | Parameters | Result | By hand | Normal ends |
| --- | --- | --- | --- | --- | --- |
| Move | `drive` | `cm`, `speed?` | `driven_cm`, `reason` (`done`, `obstacle`, `blocked`), `floor`, `min_distance_cm` | yes | `done` |
| Move | `turn` | `degrees`, positive is right | `turned_deg`, `reason` (`done`, `blocked`), `distance_cm`, `floor` | yes | `done` |
| Move | `drive_arc` | `radius_cm`, `degrees` (positive is right), `stop_on_color?`, `speed?` | `turned_deg`, `driven_cm`, `reason` (`done`, `color`, `obstacle`, `blocked`), `distance_cm`, `color`, `floor`, `min_distance_cm` | yes | `done`, `color` |
| Move | `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `driven_cm`, `reason` (`wall`, `color`, `obstacle`, `max_cm`, `blocked`), `distance_cm`, `color`, `floor`, `min_distance_cm` | yes | `wall`, `color`, `obstacle`, `max_cm` |
| Move | `turn_until_clear` | `direction`, `min_cm?`, `min_deg?`, `max_deg?` | `turned_deg`, `reason` (`clear`, `max_deg`), `distance_cm`, `floor` | yes | `clear`, `max_deg` |
| Move | `follow_line_until` | `max_cm`, `stop_on_color?`, `stop_at_crossing?` | `driven_cm`, `reason` (`color`, `crossing`, `line_lost`, `max_cm`, `obstacle`, `blocked`), `color`, `floor` | yes | `color`, `crossing`, `max_cm` |
| Move | `retrace_path` | none | `segments`, `driven_cm`, `reason` (`done`, `empty`, `obstacle`, `blocked`, `stopped`, `timeout`), `distance_cm` | yes | `done`, `empty` |
| Move | `stop` | none | `ok` | no | `done` |
| Sense | `read_sensors` | none | `distance_cm`, `floor` (current pattern), `color`, `line`, `yaw`, `battery`, `tilted` | yes | `done` |
| Sense | `scan` | `from_deg?`, `to_deg?`, `step_deg?`, `face?` | `profile` `[[deg, cm], …]`, `best_deg`, `best_cm`, `turned_deg` | yes | `done` |
| Sense | `wait_for` | `event`, `timeout_s` | `event`, `data` or `reason:"timeout"` | yes | the event arrived, `timeout` |
| Sense | `list_robots` | none | name, online, battery, color per robot | no | `done` |
| Express | `set_leds` | `color` or `pattern` | `ok` | yes | `done` |
| Express | `show_text` | `text`, at most 60 characters | `ok` | yes | `done` |
| Express | `play_sound` | `name` | `ok` | yes | `done` |
| Express | `play_melody` | `notes` as a list of pitch and duration | `ok` | — | — |
| Express | `gesture` | `kind`: `nod`, `shake`, `spin` | `ok` | — | — |
| Express | `identify` | none | `ok` | yes | `done` |
| Talk | `say` | `to`, `text` up to 60 characters, `emotion?` | `delivered` | no | `done` |
| Talk | `tell_team` | `text` | `ok` | no | `done` |
| Remember | `remember` | `text` | `note_id` | no | `done` |
| Remember | `forget` | `note_id` | `ok` | no | `done` |
| Finish | `finish` | `report` | ends the run | no | `done` |

## Rules

- **By hand and normal ends:** the registry flags which tools a person (the console) or a team
  tool may run without an agent ("by hand"), and which stop reasons count as a normal end. A
  result without a reason ends normally when it is ok; every other reason is unexpected (for
  example `drive` with `obstacle`, `follow_line_until` with `line_lost`, any motion with `blocked`,
  any tool with `offline`).
  `say`, `finish`, `remember` and `forget` need the agent runtime; `tell_team`, `list_robots` and
  `stop` are pointless by hand (the team sits there, the robots are on the page, stop is a
  button). **A new tool is agent-only until it is flagged.** Console, blocks and interpreter read
  both from the registry and keep no tool list of their own. `Registry.CallManual` refuses an
  agent-only tool with `ok: false` and a German message. The team page gets, per tool, whether it
  is by hand and its JSON schema (`GET api/session`, `tools[]`), the same schema the model sees.
  A team runs a by-hand tool with `POST /t/{token}/tools/{name}` while no run is active
  (`architecture.md`, "By hand").
- **Arguments are checked** against the tool's schema before a call: type, enum and range of
  every argument given; unknown keys are ignored, long texts are clamped. A mistake comes back as
  `ok: false` with a German message naming the argument.
- **Structured results with a reason:** every motion tool returns `reason`. The agent's ability
  to decide depends on it.
- **Floor trace instead of a scan tool:** every motion tool except `scan` returns `floor`, what
  the four floor probes saw on the way (format in `docs/protocol.md`). Tools report colors as
  letters and never interpret them; what a stripe means lives in the team's prompt and notes. No
  tool, skill or protocol field is named after a grid, junction or coordinate (`docs/field.md`).
- **Robot parameter only when needed:** if the team has exactly one robot, tools have no `robot`
  parameter. With several robots (the control room can give a team any number), `robot` is an
  enum of the team's robot names, and `stop` additionally accepts `"alle"`.
- **Names instead of device IDs:** the agent knows the names the team gave its robots (`Scout`,
  `Träger`; by default the sticker color, e.g. `rot`), never `mbot-2`. Devices can be swapped
  without changing prompts as long as the new robot gets the same name. `all` and `alle` are
  reserved and cannot be robot names.
- **Obstacles:** every forward motion stops by itself before an obstacle the ultrasonic sensor
  sees (reason `obstacle`). The sensor looks forward only: `drive` backwards is blind, and the
  tool description tells the agent so. Details and limits in `docs/protocol.md`, "Rules".
- **Blocked:** an obstacle the sensor does not see (too low, nearer than 5 cm, behind or beside
  the robot) the robot only feels: the motors need clearly more power than their speed asks for,
  or the nose lifts while it drives forward. The motion then ends within a second with
  `ok: false` and reason `blocked`, for every motion tool, also `turn` and `drive` backwards. The
  result says what told the robot in `blocked_by`: `motor_power` (it pushes against something
  that does not give way, the wheels slip) or `nose_up` (it climbs onto something). The
  description of `drive` tells the agent what both mean, that `driven_cm` is too large then, and
  what to do (back off the way it came, turn away, take another way); it also explains `tilted`.
  The other motion tools refer to it. It is never a normal end: a team tool goes on only if an
  `if` on the reason handles it ("Grund ist steckt fest"). The team page words the cause with the
  reason ("steckt fest (Motoren brauchen zu viel Kraft)"). Details in `docs/protocol.md`, "Rules".
- **`retrace_path` turns around:** it drives the way back forward, also a stretch the team drove
  backwards, so the obstacle reflex stays on, and ends at the start facing the way it came. The way counts from the start of the
  run: the server clears the trip log of the team's robots when a run starts. After an abort the
  rest stays in the trip log and the next call continues (details in `docs/protocol.md`, "Trip log").
- **`drive_arc` curves:** it drives forward on a circle of `radius_cm` (10 to 200 cm, to the
  center) until the heading has changed by `degrees`, measured like `turn`; 360 is a full circle.
  The sign follows `turn`: positive curves right. The obstacle reflex looks along the tangent.
  `stop_on_color` ends it early, like `drive_until`. `floor` counts cm along the way, so stripe
  widths read the same as when driving straight. In the trip log an arc is its chord (half turn, straight drive, half
  turn), so `retrace_path` drives back on a straight line.
- **`say` is a bundle:** text on the speaker's display, light, sound, optional gesture, and the
  message lands in the recipient's inbox. `to` is an agent name or `"all"`.
- **`finish` delivers the report** that the team later compares with the log.
- **Several robots at once:** the tool calls of one model answer that act on different robots
  run at the same time, so the agent moves several robots by calling a motion tool for each in
  one answer. Calls for the same robot keep their order, and so do calls that act on every robot
  (`stop` with `"alle"`). Calls without a robot (`wait_for`, `say`, `tell_team`, `list_robots`,
  `remember`, `forget`, `finish`) wait for all earlier calls of the answer and hold up the later
  ones. Each call shows up in the team's log when it ends; the model gets all results together,
  in its call order, once the whole answer is done. The platform rules tell a team with several
  robots so. A stop by the team, the time limit or deleting the team cancels every running
  call and stops all robots of the team.
- **Later:** motion tools that return at once and report the end as an event, so the agent can
  react to one robot while another still moves.
- **Deliberately no tool for:** Wi-Fi, vendor cloud services, reboot, setting the name, raw motor
  power.

## Team tools as agent tools

A deploy freezes the prompt and every team tool of the (team, task) as one version
(`programs.md`, "Versions"). The agent gets each team tool of the newest version that is valid
and has a description as a tool of its own:

- **Name and description** are the team's. The server appends one fixed sentence: "Dies ist ein
  Werkzeug, das das Team selbst gebaut hat. Es läuft ohne Rückfragen durch und meldet am Ende,
  ob es fertig wurde oder wo es abbrach." No parameters, except `robot` (required enum) when the
  team has several robots.
- **Call:** the team tool runs inside the agent's tool call; the agent waits, messages collect in
  its inbox meanwhile. Its steps count against the run's time limit, not as model steps or model
  calls. A tool changed after the deploy applies from the next deploy.
- **Result for the agent:** `{ok, end, steps, runtime_s, at, last, robot}`, plus `value` from
  `gib zurück` and `error` (a German sentence) when it did not end `done`. `last` is the last
  built-in tool's result only; earlier steps, and their floor traces, stay out of the agent's
  context.
- **Protocol:** the team sees every step in the log (source agent, `data.program`); the steps are
  stored as tool calls of the run that name the team tool (`tool_calls.program`), before the
  call's own row.
- **Conversation mode** lists the team tools as text, like the other tools.
- **Deploy warning:** if the prompt names something in code style (`` `name` ``) that is neither a
  team tool of the version, nor a built-in tool, nor one of the team's robots, the deploy answers
  a warning and still deploys.

## Tool set of a task

A task says which tools the agent gets (`agent_tools`, set on the draft, fixed when published):

- `all` (default): every built-in tool and the team tools of the deployed version.
- `team_tools`: the team's deployed own tools plus `read_sensors`, `say`, `finish`, `tell_team`,
  `remember` and `forget`. These six move no robot, so the comparison of how teams cut their tools
  (stage 5) still measures the team tools: everything that moves a robot is the team's.

A call of a tool that is not on the list answers like any unknown tool ("Das Werkzeug … gibt es
nicht. Verfügbar: …"), so the agent can correct itself; the server logs `tool not offered`.
Conversation mode lists the same tools. The console and "Ausführen" stay available outside runs;
in the challenge the lab lead counts them as interventions.

## Block texts

Every by-hand tool has one line in `server/internal/tools/blocks.go`: the German sentence of its
block in the team tool editor, with its fields in order, their defaults and units. Ranges and
options come from the tool's schema. A line break (`\n`, shown as ↵ below) starts a new row in
the block, so long blocks fit a laptop-sized canvas; step descriptions join the rows with a space. A new by-hand tool needs one line there and then appears in
the editor (`docs/programs.md`, "Blocks"); a test fails for a by-hand tool without one.

| Tool | Group | Block text | Fields (default) |
| --- | --- | --- | --- |
| `drive` | Fahren | fahre %1 cm | `cm` (20) |
| `turn` | Fahren | drehe %1 Grad nach %2 | `degrees` (90), rechts/links sets the sign |
| `drive_arc` | Fahren | fahre eine Kurve nach %1 ↵ mit Radius %2 cm, %3 Grad weit | rechts/links, `radius_cm` (30), `degrees` (90) |
| `drive_until` | Fahren | fahre geradeaus, höchstens %1 cm, ↵ bis eine Wand näher als %2 cm ist ↵ oder der Boden %3 ist | `max_cm` (100), `stop_below_cm` (15), `stop_on_color` (egal) |
| `turn_until_clear` | Fahren | drehe nach %1, bis vorne %2 cm frei sind | `direction` (rechts), `min_cm` (40) |
| `follow_line_until` | Fahren | folge der Linie bis %1, ↵ höchstens %2 cm | `stop_on_color` (zur Kreuzung), `max_cm` (100) |
| `retrace_path` | Fahren | fahre den Weg zurück zum Anfang | — |
| `scan` | Sensoren | schau dich um von %1 bis %2 Grad ↵ und schau dann %3 | `from_deg` (-90), `to_deg` (90), `face` (in die freieste Richtung) |
| `read_sensors` | Sensoren | lies die Sensoren | — |
| `wait_for` | Sensoren | warte auf %1, höchstens %2 Sekunden | `event` (Knopf A), `timeout_s` (10) |
| `set_leds` | Licht und Anzeige | Licht %1 | `color` (grün) |
| `show_text` | Licht und Anzeige | zeige %1 | `text` (Hallo) |
| `play_sound` | Licht und Anzeige | spiele den Ton %1 | `name` (hello) |
| `identify` | Licht und Anzeige | blinke und piepe | — |

## Mapping to the protocol

Motion and expression tools become a `cmd` with the same name (exceptions: `turn` and `drive_arc`
map `degrees` to `deg`). The server answers `read_sensors` and `list_robots` from the last received `state`
without asking the robot. `wait_for`, `say`, `tell_team`, `remember`, `forget` and `finish` run
only in the server.

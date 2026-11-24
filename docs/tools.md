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
| Move | `drive` | `cm`, `speed?` | `driven_cm`, `reason`, `floor`, `min_distance_cm` | yes | `done` |
| Move | `turn` | `degrees`, positive is right | `turned_deg`, `reason`, `distance_cm`, `floor` | yes | `done` |
| Move | `drive_arc` | `radius_cm`, `degrees` (positive is right), `stop_on_color?`, `speed?` | `turned_deg`, `driven_cm`, `reason` (`done`, `color`), `distance_cm`, `color`, `floor`, `min_distance_cm` | yes | `done`, `color` |
| Move | `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `driven_cm`, `reason`, `distance_cm`, `color`, `floor`, `min_distance_cm` | yes | `wall`, `color`, `obstacle`, `max_cm` |
| Move | `turn_until_clear` | `direction`, `min_cm?`, `min_deg?`, `max_deg?` | `turned_deg`, `reason` (`clear`, `max_deg`), `distance_cm`, `floor` | yes | `clear`, `max_deg` |
| Move | `follow_line_until` | `max_cm`, `stop_on_color?`, `stop_at_crossing?` | `driven_cm`, `reason` (`color`, `crossing`, `line_lost`, `max_cm`), `color`, `floor` | yes | `color`, `crossing`, `max_cm` |
| Move | `retrace_path` | none | `segments`, `driven_cm`, `reason` (`done`, `empty`, `obstacle`, `stopped`, `timeout`), `distance_cm` | yes | `done`, `empty` |
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
  example `drive` with `obstacle`, `follow_line_until` with `line_lost`, any tool with `offline`).
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

## Mapping to the protocol

Motion and expression tools become a `cmd` with the same name (exceptions: `turn` and `drive_arc`
map `degrees` to `deg`). The server answers `read_sensors` and `list_robots` from the last received `state`
without asking the robot. `wait_for`, `say`, `tell_team`, `remember`, `forget` and `finish` run
only in the server.

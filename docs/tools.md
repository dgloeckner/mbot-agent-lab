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

| Group | Tool | Parameters | Result |
| --- | --- | --- | --- |
| Move | `drive` | `cm`, `speed?` | `driven_cm`, `reason`, `floor`, `min_distance_cm` |
| Move | `turn` | `degrees`, positive is right | `turned_deg`, `reason`, `distance_cm`, `floor` |
| Move | `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `driven_cm`, `reason`, `distance_cm`, `color`, `floor`, `min_distance_cm` |
| Move | `turn_until_clear` | `direction`, `min_cm?`, `min_deg?`, `max_deg?` | `turned_deg`, `reason` (`clear`, `max_deg`), `distance_cm`, `floor` |
| Move | `follow_line_until` | `max_cm`, `stop_on_color?`, `stop_at_crossing?` | `driven_cm`, `reason` (`color`, `crossing`, `line_lost`, `max_cm`), `color`, `floor` |
| Move | `retrace_path` | none | `segments`, `reason` (`done`, `empty`, `obstacle`, `stopped`, `timeout`), `distance_cm` |
| Move | `stop` | none | `ok` |
| Sense | `read_sensors` | none | `distance_cm`, `floor` (current pattern), `color`, `line`, `yaw`, `battery`, `tilted` |
| Sense | `scan` | `from_deg?`, `to_deg?`, `step_deg?`, `face?` | `profile` `[[deg, cm], …]`, `best_deg`, `best_cm`, `turned_deg` |
| Sense | `wait_for` | `event`, `timeout_s` | `event`, `data` or `reason:"timeout"` |
| Sense | `list_robots` | none | name, online, battery, color per robot |
| Express | `set_leds` | `color` or `pattern` | `ok` |
| Express | `show_text` | `text`, at most 60 characters | `ok` |
| Express | `play_sound` | `name` | `ok` |
| Express | `play_melody` | `notes` as a list of pitch and duration | `ok` |
| Express | `gesture` | `kind`: `nod`, `shake`, `spin` | `ok` |
| Express | `identify` | none | `ok` |
| Talk | `say` | `to`, `text` up to 60 characters, `emotion?` | `delivered` |
| Talk | `tell_team` | `text` | `ok` |
| Remember | `remember` | `text` | `note_id` |
| Remember | `forget` | `note_id` | `ok` |
| Finish | `finish` | `report` | ends the run |

## Rules

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
- **`retrace_path` turns around:** it drives the way back forward, so the obstacle reflex
  stays on, and ends at the start facing the way it came. After an abort the rest stays in the
  trip log and the next call continues (details in `docs/protocol.md`, "Trip log").
- **`say` is a bundle:** text on the speaker's display, light, sound, optional gesture, and the
  message lands in the recipient's inbox. `to` is an agent name or `"all"`.
- **`finish` delivers the report** that the team later compares with the log.
- **Later:** `run_together(steps)` for simultaneous choreographies of several robots.
- **Deliberately no tool for:** Wi-Fi, vendor cloud services, reboot, setting the name, raw motor
  power.

## Mapping to the protocol

Motion and expression tools become a `cmd` with the same name (exception: `turn` maps `degrees`
to `deg`). The server answers `read_sensors` and `list_robots` from the last received `state`
without asking the robot. `wait_for`, `say`, `tell_team`, `remember`, `forget` and `finish` run
only in the server.

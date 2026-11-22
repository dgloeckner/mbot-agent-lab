# Tool catalog

Agents get a few powerful tools with built-in stop conditions instead of a one-to-one mapping of
the vendor API. The model decides at the corners; the robot drives on its own in between.

Implementation: `server/internal/tools` (registry with name, JSON schema and handler). The agent
calls them directly; the optional MCP endpoint exposes the same registry to external clients.

| Group | Tool | Parameters | Result |
| --- | --- | --- | --- |
| Move | `drive` | `cm`, `speed?` | `driven_cm`, `reason` |
| Move | `turn` | `degrees`, positive is right | `turned_deg`, `reason` |
| Move | `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `driven_cm`, `reason`, `distance_cm`, `color` |
| Move | `follow_line_until` | `stop_on_color?`, `max_cm` | `driven_cm`, `reason`, `color` |
| Move | `retrace_path` | none | `segments`, `reason` |
| Move | `stop` | none | `ok` |
| Sense | `read_sensors` | none | `distance_cm`, `color`, `line`, `yaw`, `battery`, `tilted` |
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
- **Robot parameter only when needed:** if the owner has exactly one robot, tools have no `robot`
  parameter. With several robots, `robot` is an enum of the allowed aliases, and `stop`
  additionally accepts `"all"`.
- **Aliases instead of device IDs:** the agent knows `ich`, `links`, `rechts`, never `mbot-2`.
  Devices can be swapped without changing prompts.
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

# MQTT protocol

Authoritative description of the protocol between robots and server. The server
(`server/internal/protocol`), the robot script (`robot/src`) and the simulator (`sim/`) are all
tested against the same example messages in [`examples/protocol/`](examples/protocol/). Whoever
changes this document changes the examples too.

Protocol version: **1**

## Topics

Every robot has its own prefix `lab/<robot_id>/`, e.g. `lab/mbot-1/`.

| Topic | Direction | Retained | Content | Example |
| --- | --- | --- | --- | --- |
| `cmd` | server → robot | no | command with ID, name and arguments | [cmd.json](examples/protocol/cmd.json) |
| `result` | robot → server | no | result of a command | [result.json](examples/protocol/result.json) |
| `event` | robot → server | no | event on a state change | [event.json](examples/protocol/event.json) |
| `state` | robot → server | yes | current state | [state.json](examples/protocol/state.json) |
| `status` | robot → server | yes, as last will | `online` or `offline`, plain text | [status.txt](examples/protocol/status.txt) |
| `hello` | robot → server | no | protocol version, script version, MAC address, free memory on connect | [hello.json](examples/protocol/hello.json) |
| `hb` | server → robot | no | heartbeat, once per second | [hb.json](examples/protocol/hb.json) |

## Messages

### `cmd`

```json
{"id":"c-8f3a","name":"drive_until","args":{"max_cm":200,"stop_below_cm":12,"stop_on_color":"blue"}}
```

- `id`: assigned by the server, unique per robot, also across server restarts (the server adds a
  random prefix per start).
- `name`: skill name, see the table below.
- `args`: object, may be omitted.

### `result`

```json
{"id":"c-8f3a","ok":true,"reason":"wall","data":{"driven_cm":42,"distance_cm":11,"color":"white"}}
```

- `ok`: `true` if the skill ended regularly (including by reaching a stop condition).
- `reason`: one of the values below.
- `data`: measurements at the end of the skill, optional.
- `error`: optional text for `reason:"error"`.

### `event`

```json
{"kind":"obstacle_near","data":{"distance_cm":8},"seq":117,"t_ms":123456}
```

Values for `kind`: `obstacle_near`, `obstacle_clear`, `color_changed`, `tilted`, `button_a`,
`button_b`, `clap`, `battery_low`, `skill_done`, `skill_aborted`.

`clap` (`data.loudness`, 0..100) is a short peak well above the background level. The robot
does not listen while its motors run or it beeps for `identify`, until 0.5 s after that, nor
for 2 s after `play_sound`: the microphone sits next to motors and speaker.

### `state`

```json
{"skill":"idle","distance_cm":34,"floor":"wkkw","color":"white","line":"center","yaw":-2,"battery":81,"tilted":false,"seq":118,"t_ms":123460}
```

`floor` is the current floor pattern (see [Floor trace](#floor-trace)). `color` is the floor color
both outer probes agree on, `line` where black is under the probes (`none`, `left`, `center`,
`right`, `all`).

Sent immediately on change, otherwise two to five times per second.

### `hello`

```json
{"v":1,"script":"0.1.0","mac":"aa:bb:cc:dd:ee:ff","mem_free":61440}
```

- `mem_free`: free heap in bytes right after connecting, after a garbage collection; `-1` where
  unknown (simulator). It tells how much room the script has left (`docs/measurements.md`).

### `hb`

```json
{"team":"t4kq9ma","owner":"Rote Raketen","name":"Scout","color":"red","agent":"thinking"}
```

- `team`: ID of the owning team, empty when the robot is free. When it changes, the robot stops,
  clears its trip log and shows the new owner. Renaming the team or the robot does not change it,
  so a run is not interrupted.
- `owner`: team name, `name`: what the team calls this robot. Both are shown on the display.
- `color`: team color for the LEDs. `agent`: `waiting`, `thinking` or `acting`; the LEDs pulse while
  the agent thinks.

> **Addition to the handoff:** the handoff does not define the heartbeat payload. It carries owner
> and agent state so that changes reach the robot even when single QoS 0 messages get lost.
> A free robot shows "frei" and stands still.

## Skills (`cmd.name`)

| Name | Arguments | `data` in the result |
| --- | --- | --- |
| `drive` | `cm`, `speed?` | `driven_cm`, `distance_cm`, `color`, trace |
| `turn` | `deg`, positive is right | `turned_deg`, `distance_cm`, trace |
| `drive_arc` | `radius_cm` (10..200, to the center), `deg` (heading change, positive is right), `stop_on_color?`, `speed?` (40, of the center) | `turned_deg`, `driven_cm`, `distance_cm`, `color`, trace (position in cm along the way) |
| `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?`, `speed?` | `driven_cm`, `distance_cm`, `color`, trace |
| `turn_until_clear` | `direction` (`left`/`right`), `min_cm?` (40), `min_deg?` (20), `max_deg?` (360) | `turned_deg`, `distance_cm`, trace |
| `scan` | `from_deg?` (-90), `to_deg?` (90), `step_deg?` (15), `face?` (`best`/`start`) | `profile` (`[[deg, cm], …]`, `cm` null if no reading), `best_deg`, `best_cm`, `turned_deg`, `distance_cm` |
| `follow_line_until` | `max_cm`, `stop_on_color?`, `stop_at_crossing?` (true), `speed?` (30) | `driven_cm`, `color`, `distance_cm`, trace |
| `calibrate_colors` | `cm?` (38) | `refs`: per probe six `[r, g, b]` for white, black, red, yellow, green, blue; `driven_cm` |
| `retrace_path` | none | `segments`, `driven_cm` (straight drives only, not turns), `distance_cm` (see [Trip log](#trip-log)) |
| `stop` | none | none |
| `set_leds` | `color` or `pattern` | none |
| `show_text` | `text` | none |
| `play_sound` | `name` | none |
| `play_melody` | `notes`: list of `[frequency_hz, duration_ms]` | none |
| `gesture` | `kind`: `nod`, `shake`, `spin` | none |
| `identify` | none | none |
| `clear_log` | none | none |

## Rules

- **QoS 0 everywhere.** Reliability comes from ID, result and a server-side timeout. The timeout
  is the skill's maximum duration plus 2 seconds. For now every motion skill is capped at
  20 seconds on the robot (`SK_MAX_MS` in `robot/src/skills.py`), so the server waits 22 seconds.
- **Duplicate commands:** the robot remembers the last 16 IDs and ignores repeats. It forgets them
  on every new connection.
- **One motion skill at a time:** a new motion command during a drive ends with `ok:false` and
  `reason:"busy"`. `stop` is always accepted and idempotent.
- **Values for `reason`:** `done`, `wall`, `color`, `max_cm`, `timeout`, `stopped`, `obstacle`,
  `busy`, `tilted`, `reassigned`, `error`, and for the goal skills `clear` and `max_deg`
  (`turn_until_clear`), `crossing` and `line_lost` (`follow_line_until`), `empty`
  (`retrace_path` with an empty trip log).
- **Trace:** every motion skill except `scan` and `retrace_path` reports `floor` (and `floor_truncated` when cut
  off) and `min_distance_cm`, also when it ends with `stopped`, `obstacle` or another abort.
- **`stop_on_color`:** `white`, `black`, `red`, `yellow`, `green`, `blue` or `any` (anything but
  white). It triggers when one of the four probes sees the color. A color the probes already see
  at the start counts only after they have left it (a robot in the red start field can drive back
  to red). Same for `drive_until`, `drive_arc` and `follow_line_until`.
- **Filtered sensors:** every decision on the robot uses filtered readings (`robot/src/sense.py`):
  distance as the median of the last 7 valid readings (dropouts below 2 cm ignored; 10 dropouts
  in a row mean the sensor is blind, and the distance reads 0), the floor
  pattern only after 3 equal readings in a row, tilt with hysteresis (on above 45°, off below 30°)
  and a minimum duration. Stop conditions must hold for several loop iterations. `state` and
  `event` carry the same filtered values. The emergency stop distance grows with speed
  (8 cm + 0.05 cm per RPM).
- **Obstacles:** the ultrasonic sensor looks forward in a cone of about 30°. Every forward motion
  (`drive`, `drive_until`, `drive_arc`, `follow_line_until`, the drives of `retrace_path`) ends
  with `obstacle` once something is nearer than the emergency stop distance, and does not start
  when it already is; the 2 cm above the 6 cm of the handoff keep the front corners off a wall met
  at a slant down to about 30° (`sim/tests/test_safety.py`). The robot cannot see behind or
  beside itself: `drive` with negative `cm` is blind, which the agent has to know; turns on the
  spot do not check; a wall grazed at a flatter angle can be touched. `retrace_path` decides its
  moves itself and therefore never drives backwards.
- **`calibrate_colors`** is maintenance for the control room, not an agent tool. The robot stands
  right before the reference strip and drives across it; it finds the black field itself, so the
  start position needs no precision. The references apply at once and are lost on restart until
  the server resends them (planned: a `config` message, field handoff G7).

## Trip log

The robot logs every finished or aborted motion skill as a segment, `drive` with signed cm or
`turn` with signed degrees (positive right). The log is the way from where it began to the
current pose. `drive_arc` goes in as its chord: a turn by half the heading change, a drive of the
chord length (from `driven_cm` and `turned_deg`), the other half turn; zero entries are left out. It is cleared by `clear_log`, on owner change and on restart. The server sends
`clear_log` to a team's robots when the team's agent starts a run, so `retrace_path` never drives
back through earlier runs; a robot that is offline then keeps its log.

`retrace_path` works the log off from the end:

- A turn at the end of the log is folded into the robot's extra rotation; nothing moves yet.
- Before a drive the robot turns until it faces back (extra rotation 180°; 0° for a drive that
  went backwards), then drives the logged distance forward. The forward-looking obstacle reflex stays active the whole way.
  So the logged turns come out negated and the robot ends at the start, facing the way it came.
- Drives go straight and turns go through the same turn skill as `turn`. Lines are not
  followed; turns at the end of a log with no drive before them do not move the robot.
- The retrace's own moves are not logged. `segments` counts the log entries worked off,
  `driven_cm` the cm this call drove (turns not counted).
- `done` (or `empty` when there was nothing to do) leaves the log empty: the robot's pose is the
  new beginning. An abort (`obstacle`, `stopped`, `tilted`, `timeout`, …) leaves the way still to
  go in the log plus a turn for the extra rotation, so the log again describes the way from the
  beginning to here. A new `retrace_path` continues from there, also after detours.
- The whole retrace counts as one command: it ends with `timeout` after 20 seconds like every
  motion skill, and a new call drives on.

## Floor trace

The quad RGB sensor has four probes, from left to right `L2`, `L1`, `R1`, `R2`. The robot
classifies each into a letter: `w` white, `k` black, `r` red, `y` yellow, `g` green, `b` blue,
`?` unclear. The four letters are the floor pattern, e.g. `rkkr`: a red stripe under a black line.

- With references from `calibrate_colors` the robot classifies RGB values per probe: nearest
  reference by hue and brightness, `?` if none is near enough. A neutral gray between white and
  black (the edge of a line under the probe's spot) counts as the nearer of the two. Without
  references it uses the sensor's built-in color names.
- A pattern counts after 3 equal readings in a row; shorter flicker never shows up.
- At a stripe edge the probes rarely change together. An entry that the next one replaces within
  0.5 cm (4° when turning) is such a transition and left out of `floor`.
- `floor` in a result is a list of `[position, pattern]`. An entry starts where its pattern
  began. Position counts from the start of the motion: cm when driving (on an arc: along the way
  of the robot's center), degrees (positive right) when turning on the spot, one decimal. An entry lasts until the next begins.
- At most 20 entries; then `floor_truncated: true` and the motion goes on.
- The robot never interprets patterns. What a stripe means is up to the agent and its prompt
  (see `docs/field.md`).

```json
{"id":"c-41","ok":true,"reason":"done","data":{"driven_cm":30.0,"distance_cm":300,"color":"white",
 "floor":[[0,"wkkw"],[2.1,"rkkr"],[4.0,"wkkw"],[6.1,"gkkg"],[8.0,"wkkw"],[12.1,"gkkg"],
          [14.0,"wkkw"],[16.1,"ykky"],[18.0,"wkkw"],[24.1,"kkkk"],[26.0,"wkkw"]],"min_distance_cm":300}}
```

## Connection rules

- **Time:** the robot has no clock. `t_ms` counts milliseconds since boot; the server stamps the
  arrival time.
- **Size:** compact JSON, every message below 512 bytes.
- **Keepalive:** 30 seconds. The last will sets `status` to `offline`.
- **Watchdog:** 3 seconds without `hb` make the robot stop its motors and show "getrennt".
- **Access:** the user name is the robot ID, plus one password per robot. A robot may only read
  and write below its own prefix.
- **Version:** `hello.v` is the protocol version. For an unknown version the server does not put
  the robot into service and shows this in the control room.

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
| `hello` | robot → server | no | protocol version, script version, MAC address on connect | [hello.json](examples/protocol/hello.json) |
| `hb` | server → robot | no | heartbeat, once per second | [hb.json](examples/protocol/hb.json) |

## Messages

### `cmd`

```json
{"id":"c-8f3a","name":"drive_until","args":{"max_cm":200,"stop_below_cm":12,"stop_on_color":"blue"}}
```

- `id`: assigned by the server, unique per robot.
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

### `state`

```json
{"skill":"idle","distance_cm":34,"color":"white","line":"none","yaw":-2,"battery":81,"tilted":false,"seq":118,"t_ms":123460}
```

Sent immediately on change, otherwise two to five times per second.

### `hello`

```json
{"v":1,"script":"0.1.0","mac":"aa:bb:cc:dd:ee:ff"}
```

### `hb`

```json
{"owner":"Team Rot","color":"red","agent":"thinking"}
```

> **Addition to the handoff, to be confirmed:** the handoff does not define the heartbeat
> payload. Here it carries the owner's display name, the team color and the agent state
> (`waiting`, `thinking`, `acting`). Because it arrives every second, a change reaches the robot
> even if single QoS 0 messages get lost. `owner` is empty when the robot is free; it then shows
> "frei" and stands still.

## Skills (`cmd.name`)

| Name | Arguments | `data` in the result |
| --- | --- | --- |
| `drive` | `cm`, `speed?` | `driven_cm` |
| `turn` | `deg`, positive is right | `turned_deg` |
| `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `driven_cm`, `distance_cm`, `color` |
| `follow_line_until` | `stop_on_color?`, `max_cm` | `driven_cm`, `color` |
| `retrace_path` | none | `segments` |
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
- **Duplicate commands:** the robot remembers the last 16 IDs and ignores repeats.
- **One motion skill at a time:** a new motion command during a drive ends with `ok:false` and
  `reason:"busy"`. `stop` is always accepted and idempotent.
- **Values for `reason`:** `done`, `wall`, `color`, `max_cm`, `timeout`, `stopped`, `obstacle`,
  `busy`, `tilted`, `reassigned`, `error`.
- **Time:** the robot has no clock. `t_ms` counts milliseconds since boot; the server stamps the
  arrival time.
- **Size:** compact JSON, every message below 512 bytes.
- **Keepalive:** 30 seconds. The last will sets `status` to `offline`.
- **Watchdog:** 3 seconds without `hb` make the robot stop its motors and show "getrennt".
- **Access:** the user name is the robot ID, plus one password per robot. A robot may only read
  and write below its own prefix.
- **Version:** `hello.v` is the protocol version. For an unknown version the server does not put
  the robot into service and shows this in the control room.

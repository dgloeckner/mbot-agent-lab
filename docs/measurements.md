# Measurements on the robot

Values the code needs but no documentation gives. Each section says how to measure, what the
result decides, and has a table to fill in. Until a value is filled in, the code uses a start value
marked VERIFY.

## Measurement script

`robot/tools/measure.py` takes most of the measurements below in one go. Upload it alone instead
of `main.py` (upload mode, or `cd server && go run ./cmd/robotupload ../robot/tools/measure.py`),
then follow the display. Three seconds after the start the robot wiggles 2 cm forward and back,
twice (`M wiggle ...`: do the wheels turn at all?). Then every station waits for button A (B skips
it), waits two seconds and measures; a line `M health ...` after each station says whether the
mBot2 shield still answers. Results are lines starting with `M ` on the serial log; keep the whole
log. Afterwards upload the normal bundle again.

Without anyone at the robot: list the stations in `STATIONS` and `AUTO` at the top of the script
(a station in `AUTO` starts three seconds after its prompt, without a button), and let the
uploader read the console: `robotupload --monitor 100s measure.py` prints the robot's lines for
that long after the upload. mBlock must not hold the serial port.

The shield's battery must be charged and its power switch on. On USB power alone the CyberPi
runs and `get_battery` still reads 50, but motors, encoders, ultrasonic and the quad RGB sensor
read 0 and the wheels stand.

The robot drives in `wheels`, `slow`, `drive`, `dist`, `turn`, `block`, `power` and `ram`, with no
obstacle reflex.

| Station | What to do | What it tells |
| --- | --- | --- |
| `info` | nothing | time per vendor call and per loop iteration's sensor reads; free memory |
| `rest` | robot stands on the floor | pitch level and noise, yaw drift, ultrasonic and loudness at rest |
| `nose` | lift the front 2 cm, then the left side | `PITCH_NOSE_UP_SIGN`; pitch per cm of lift |
| `sonic` | open space; wall at 20 cm and 5 cm before the sensor; wall at 50 cm turned 30° away; the low obstacle touching | what open space reads (`SN_DIST_BLIND`), accuracy, dropouts, whether the low obstacle is invisible |
| `wheels` | 10 cm free | whether the wheels turn at all; if not, the driving stations are skipped (shield without power: everything behind it reads 0) |
| `slow` | 15 cm free | whether the wheels turn at 2, 3, 5 and 8 RPM: `SK_MIN_SPEED` |
| `drive` | 60 cm free | speeding up and run-on at 20, 40 and 80 RPM forwards, backwards and on the spot: `SK_RUNON_S`, the simulator's `motor_lag_s`; pitch while driving free: `SN_NOSE_ON_DEG`, `SN_NOSE_MS` |
| `dist` | mark the wheel position, 1.2 m free, measure the cm driven | `CM_PER_WHEEL_DEG` (wheel diameter); how straight it drives |
| `turn` | mark where the nose points, note the error after a full turn right and left | `TRACK_CM`; encoder turn against the gyro |
| `block` | put the robot 5 cm before an obstacle, up to nine runs (40, 80, 20 RPM in turn) | wheel speed and pitch while it pushes from a standing start |
| `power` | 50 cm free behind, 20 cm to each side | motor power in free driving at 10 to 100 RPM, forwards, backwards, on the spot and in arcs: `SN_PWR_BASE`, `SN_PWR_PER_RPM`, `SN_PWR_OVER`; time per power reading |
| `ram` | facing a wall or obstacle the ultrasonic sensor sees, up to 1.5 m away | closes in to 30 cm by itself, drives into it at each speed in `RAM_SPEEDS`, pushes for 2 s and backs off: wheel speed, pitch, motor power and acceleration over time (`M ramtrace`): `SN_PWR_OVER`, `SN_HELD_MS`, `SN_NOSE_ON_DEG` |
| `floor` | sensor over white, black, each tape color, then the line under the left edge, the right edge and the middle | RGB per probe and color (field handoff G0), sign and range of `get_offset_track` |
| `sound` | quiet; three claps; motors running with the wheels in the air | `EV_CLAP_LEVEL`, `EV_CLAP_JUMP`, the hush while the motors run |

With a ruler, not with the script (`sim/mbot2.json`, `docs/field.md` "Roboter, zu messen"): sensor
bar to wheel axle, L2 to R2, L1 to R1, ultrasonic sensor to wheel axle, wheel diameter, track
width, and the widest point of the body.

### Results of the run of 2026-11-27

One robot (firmware 44.01.011), shield battery at 90 to 100 %, smooth floor. `floor`, `sound` and
the quarter turns of `turn` have not run. Nothing was measured with a ruler, so the geometry
constants (`CM_PER_WHEEL_DEG`, `TRACK_CM`) stay as they are.

| What | Measured |
| --- | --- |
| Time per call: both wheel encoders | 55 ms |
| Time per call: RGB of one probe (3 calls) | 110 ms |
| Time per call: `get_battery` | 33 ms |
| Time per call: `EM_get_power`, one motor | 32 ms |
| Time per call: `EM_get_speed`, one motor | 25 ms |
| Time per call: `get_color_sta` 2.0, ultrasonic 1.1, `get_offset_track` 1.1, yaw, pitch or roll 0.25, button 0.2, loudness 0.16 | ms |
| Sensor reads of one loop iteration, built-in color names | about 45 ms, 33 of it the battery |
| Sensor reads of one loop iteration, calibrated RGB | about 475 ms |
| Pitch and roll at rest | whole degrees, no flicker |
| Ultrasonic at rest | standard deviation 0.18 cm, no dropouts; open space reads 188 to 300 |
| Ultrasonic, wall 5 cm before the sensor | 300: too near reads as free |
| Ultrasonic, wall at 50 cm turned 30° away | 38.5 |
| Slowest speed at which the wheels turn | 2 RPM, starting after about 300 ms |
| Commanded wheel travel reached after a start | within 100 ms at 20, 40 and 80 RPM |
| Run-on after a stop | about 5° of wheel at 40 RPM, 20° at 80 RPM (`SK_RUNON_S` 0.02 to 0.04 s); standstill after about 100 ms |
| 1440° of wheel straight ahead | 1437° and 1439°; the gyro turned 8° on the way |
| Full turn right by the encoders (359.4°) | gyro 363° |
| Full turn left by the encoders (359.4°) | gyro 372° |
| Free memory at the start of the measuring script | 1 270 896 bytes |

An upload only restarts the interpreter ("fast reboot"): modules in `sys.modules` and threads of
the script that ran before survive.

## Memory: how big may the robot script get?

The bundle test (`sim/tests/test_world_and_bundle.py`) caps `robot/dist/main.py` at a byte
budget. The first budget, 40000 bytes, was a guess. Two limits are real:

- **Compiling:** mBlock uploads the source and the CyberPi compiles it at start. A script that is
  too big fails with `MemoryError` before the robot connects.
- **Running:** the compiled code stays in RAM. What is left (`mem_free`) must hold MQTT buffers,
  JSON messages, sensor snapshots and the floor trace, and the heap fragments over time.

### Steps

1. **Room without our script.** Upload `robot/tools/hwcheck.py` alone (upload mode, Python). The
   first lines show `mem_free, mem_alloc`, `largest block` and `flash statvfs /`;
   `micropython.mem_info()` goes to the serial log only.
2. **Room with our script.** Build and upload the normal bundle:

   ```bash
   npx nx run robot:bundle --config=robot/config.py
   ```

   After connecting, the robot reports `mem_free` in `hello`. The control room shows it as the
   tooltip of the robot chip ("Skript 0.1.0, freier Speicher … KB"); the server log has it in the
   line `robot hello`.
3. **Growth.** Build bundles with extra code that is never called and upload each in turn:

   ```bash
   npx nx run robot:bundle --config=robot/config.py --pad=20000
   ```

   Go on with 40000, then halve the step towards the failing size. **Careful:** a bundle that is
   too big (120 KB did) makes the CyberPi hang at start, and mBlock can then no longer upload
   (see "Recovery" below). Do not go beyond the limit found below. For every size note the bundle size the bundler prints
   and `mem_free` from the control room. The padding never runs: a padded robot drives normally.
4. **Under load.** With the normal bundle, let an agent drive for a few minutes (line following,
   several tools), then restart only the connection (Wi-Fi off and on, or restart the server):
   the new `hello` shows `mem_free` after a longer run. Lower than in step 2 means memory leaks or
   fragments.

### Results

| Measurement | Value | Date |
| --- | --- | --- |
| MicroPython version (`python`) | 1.11.0, Makeblock build 44.01.008 of 2022-09-21 | 2026-11-23 |
| `mem_free` / `mem_alloc` without script | 1 287 824 / 249 008 B (heap 1.5 MB, so PSRAM) | 2026-11-23 |
| `largest block` without script | 1 229 824 B | 2026-11-23 |
| Flash free (`statvfs`) | 514 of 762 blocks of 4 KB: 2.1 of 3.1 MB | 2026-11-23 |
| `mem_free` with bundle, pad 0 (bundle bytes) | about 1215 KB (control room, rounded), bundle 40 134 B: the script takes about 43 KB of heap | 2026-11-23 |
| `mem_free` with pad 40000 (bundle bytes) | about 1166 KB, bundle 80 282 B (3483 lines): starts and connects, about 1.2 B of heap per source byte. The first try showed no connection for 30 s; the cause is unknown, the second try connected | 2026-11-23 |
| Largest bundle that still starts (bytes) | 80 282 B (pad 40000) | 2026-11-23 |
| Smallest bundle that fails, and the error | 120 182 B (pad 80000): uploads, then the CyberPi hangs at start without any output, and mBlock connects but can no longer upload | 2026-11-23 |
| `mem_free` after a few minutes of driving | | |

Notes on the hwcheck run of 2026-11-23:

- `micropython.mem_info()` ran after the `largest block` probe and still counted its 1.2 MB block
  (the GC is conservative and found a stale pointer), so its "used" and "free" are not the room
  without script. hwcheck now prints it before the probe.
- `mem_info` also showed `stack: 2400 out of 6144`: the program's main thread has 6 KB of stack.
  That is why the quad RGB calls fail with "maximum recursion depth exceeded" ten frames down and
  why the robot runs in its own thread with 32 KB (`robot/src/main.py`). The thread stack is not
  part of the heap above.

### Start: entry or size?

The 120 KB bundle that hung was started by a top level that never returned (`main()` called
directly, then an endless sleep loop). Other uploaders report that top-level loops can hang the
CyberPi at boot and block later uploads. The script now starts through the firmware's start event
(`hw_on_start`, see `docs/architecture.md`). This measurement tells whether the hang came from
the entry or from the size. Until it is filled in, the budget stays at 60 000 bytes.

1. **New entry, normal bundle.** Build and upload the normal bundle. The robot connects (robot
   chip in the control room).
2. **Upload over the running script.** With the robot connected, upload the same bundle again.
   mBlock uploads, the robot connects again, and the server log shows one robot, not two fighting
   over the client ID.
3. **Power cycle.** Switch off and on: the robot starts and connects without mBlock.
4. **Size.** Upload bundles padded to about 80, 100 and 120 KB, in this order:

   ```bash
   npx nx run robot:bundle --config=robot/config.py --pad=40000
   ```

   ```bash
   npx nx run robot:bundle --config=robot/config.py --pad=60000
   ```

   ```bash
   npx nx run robot:bundle --config=robot/config.py --pad=80000
   ```

   For each: note the bundle size the bundler prints, whether the CyberPi starts and connects,
   and whether mBlock can then still upload (upload the next bundle, or the normal one after the
   last). Stop at the first size that hangs and follow "Recovery" below.

| Bundle (pad, bytes) | Starts and connects | mBlock uploads afterwards | Date |
| --- | --- | --- | --- |
| Normal, pad 0 | | | |
| Normal again over the running script | | | |
| Normal after power cycle | | | |
| pad 40000 | | | |
| pad 60000 | | | |
| pad 80000 | | | |

If 120 KB starts and mBlock can upload afterwards, the hang came from the entry and the budget can
be raised as in "What follows". If it still hangs, the limit is the size and the budget stays.

### Recovery after a bundle that is too big

1. Switch off, wait 2 to 3 minutes after switching on again, then upload the normal bundle.
2. In mBlock switch from upload mode to live mode and back, then upload.
3. Last resort: update the CyberPi firmware in mBlock. It removes `main.py`, but the firmware
   version changes: run `robot/tools/hwcheck.py` again and compare.

### What follows

- RAM per source byte: (`mem_free` at pad 0 − `mem_free` at pad 20000) / 20000.
- New budget: the largest bundle that starts, minus a safety margin, and no bigger than what still
  leaves enough `mem_free` for running. Start values for both until we know better: 20 % of the
  largest bundle and 30 KB free heap.
- The new budget goes into `test_bundle_size_budget` with a note on the measurement date.

### Decision (2026-11-23)

RAM is not the limit: 80 KB of source leave about 1166 KB free. The limit is the size the CyberPi
still starts, somewhere between 80 and 120 KB. The budget is 60 000 bytes: 25 % below the largest
bundle known to start, and about 20 KB more than the bundle had then (40 134 B).

## Blocked: motor power and a lifted nose

A motion that is held up by something the ultrasonic sensor does not see ends with `blocked`
(`docs/protocol.md`, "Rules"). The thresholds behind it come from one robot, measured on
2026-11-27 with `measure.py` (stations `nose`, `slow`, `drive`, `block`, `power`, `ram`): 9 runs
against a wall from 5 cm, 5 against a wall with a run-up of 30 cm, 4 against a light, low obstacle
that the robot pushed.

### What a held robot shows

- **The wheels never stand.** In all 18 runs the speed control kept both wheels at the commanded
  speed, at 10, 20, 40 and 80 RPM: they slip on the floor or push the obstacle. The encoders
  cannot tell a held robot from a free one, and `driven_cm` counts the slipping.
- **The motors need more power.** `mbot2.EM_get_power` reads what the speed control gives a motor,
  in percent. Free driving needs 3.2 + 0.41 × RPM; against a wall it is at least 6 more, from the
  moment of contact (tables below).
- **The nose lifts only when the robot climbs.** That happened at the wall at 80 RPM, at 40 RPM
  with a run-up, and once on the low obstacle. Pushing leaves the pitch flat.
- **A light obstacle that gives way is not a held robot.** Pushing it at 40 RPM cost 2 percent
  more; nothing reports that, and nothing should.

Motor power in free driving, mean of the steady part (left / right motor):

| Commanded | Formula | Forwards | Backwards |
| --- | --- | --- | --- |
| 10 RPM | 7.3 | 6.9 / 7.8 | 6.5 / 7.9 |
| 20 RPM | 11.4 | 11.1 / 12.4 | 10.4 / 12.1 |
| 40 RPM | 19.6 | 19.5 / 20.5 | 18.8 / 21.2 |
| 60 RPM | 27.8 | 27.9 / 28.3 | 26.7 / 29.9 |
| 80 RPM | 36.0 | 35.9 / 36.5 | 34.6 / 38.2 |
| 100 RPM | 44.2 | 44.2 / 44.7 | 42.6 / 46.5 |

Turning on the spot at 20 and 40 RPM and arcs (40/20, 8/2 RPM) fit the same formula per wheel.
Single readings in steady free driving lie within 1 of the mean; the largest reading above the
formula was 4.4 (one motor, turning at 40 RPM), the largest mean of both motors 3.0. In the first
200 ms after a start a motor needs up to 11 more.

Motor power and pitch while held (pitch against the level at rest):

| Run | Motor power, left / right | Above the formula, mean of both | Pitch |
| --- | --- | --- | --- |
| Wall, 10 RPM | 14 to 17 / 15 to 22 | 6 to 11 | flat |
| Wall, 20 RPM | 20 to 22 / 23 to 27 | 9 to 13 | flat |
| Wall, 40 RPM, run-up | 31 to 35 / 30 to 41 | 11 to 17 | flat for 0.9 s, then 21 to 31° and it stays up |
| Wall, 40 RPM, from 5 cm | not recorded | | 0 to −3°, never climbs |
| Wall, 80 RPM, run-up | 43 to 52 | 7 to 16 | 27° within 150 ms |
| Wall, 80 RPM, from 5 cm | not recorded | | 24 to 37° after about 1.5 s |
| Low obstacle pushed, 20 RPM | 19 to 24 (left only) | 8 to 12 | flat |
| Low obstacle pushed, 40 RPM | 21 to 23 (left only) | 2 | flat |
| Low obstacle climbed, 40 RPM | 27 to 32 (left only) | 8 to 12 | 28 to 34° and it stays up |

Pitch in free driving: +4° when starting at 40 RPM and +9° at 80 RPM, for less than 200 ms; −19°
when braking from 80 RPM; +14° when braking from 80 RPM backwards (the nose check runs in
forward motions only). Lifting the front by 2 cm reads +8°.

### Constants

| Constant (`sense.py`) | Value | Why |
| --- | --- | --- |
| `SN_PWR_BASE`, `SN_PWR_PER_RPM` | 3.2, 0.41 | free driving, table above |
| `SN_PWR_OVER`, `SN_PWR_OVER_PART` | 4, 0.1 | held when the mean of both motors exceeds the formula by 4 plus a tenth of it: 4.7 at 10 RPM (held: 6 or more, free: 3 or less), 6 at 40 RPM (held: 11 or more), 8.4 at 100 RPM. The share leaves room for a battery that needs more power for the same speed |
| `SN_HELD_MS` | 400 ms | twice as long as the extra power of speeding up lasts |
| `SN_NOSE_ON_DEG`, `SN_NOSE_OFF_DEG` | 8°, 4° | a climbing robot reaches 21° and more; free forward driving stays below 8° except for the first 200 ms at 80 RPM |
| `SN_NOSE_MS` | 300 ms | longer than that start lasts |
| `SN_LEVEL_SMOOTH` | 32 readings | not measured; the level at rest did not move |
| `PITCH_NOSE_UP_SIGN` (`hw.py`) | 1 | front lifted 2 cm reads +8° |

A power reading takes 32 ms, so the robot reads one motor per loop iteration, in turns, and only
while the motors run.

### Still to measure

1. **Other robots.** The `power` station on at least two more: does 3.2 + 0.41 × RPM hold within 3?
   The simulator assumes motors within 10 %.
2. **A low battery.** The same station with the shield battery nearly empty: how much more power
   does the same speed need? `SN_PWR_OVER_PART` assumes at most about 15 %.
3. **The field's surface.** Free power and the power at a wall on the field itself: tape and foil
   may roll and slip differently from the floor measured here.
4. **Backwards into a wall** and a **wall met at a slant**, where one wheel carries most of the
   load. The check judges the mean of both motors; backwards is assumed to be like forwards.
5. **False alarms.** A full lap of the field with drives, turns, arcs, scans and line following
   with the real robot script: no result and no `skill_aborted` event with `blocked`.

## Display labels

A screen of the robot script is a list of labels (`robot/src/ui.py`), drawn by `hw_screen` in
`robot/src/hw.py` with one `cyberpi.display.show_label` call per label. Makeblock's Python API
page documents `show_label(message, size, x, y)` and `display.clear()` ("Clears CyberPi's
screen") and nothing about several labels. Seen on the robot on 2026-11-27: with these four
arguments only the last label of a screen stays visible. Programs written by others for the
CyberPi pass a label number as fifth argument (`index=0`, `index=1`, ...; also positional) to
show several texts at once, and `hw_screen` does the same. None of the following is documented,
so the code assumes it until it is measured:

- `show_label` accepts `index=`, and labels with different numbers are visible together.
- There are at least 8 labels (numbers 0 to 7). The largest screen, the agent's `show_text`, has
  8: two in the status bar and six lines of text. `sim/tests/test_display.py` fails if a screen
  needs more.
- `clear()` may or may not remove labels with a number above 0. `hw_screen` does not rely on it:
  it draws an empty text into every label the screen before used and the new one does not.

### Steps

1. Upload `robot/tools/hwcheck.py` alone (see [`development.md`](development.md), "Real robots").
   After the turns it shows three screens for 10 s each; the comment above `display_check` says
   what to look for on each.
2. Upload the normal bundle, assign the robot to a team and let the agent call `show_text` with a
   text of several lines, for example "Schoen, dass du da bist! Team Gallier gruesst dich :)".
   The status bar and all four lines must be visible. Then call `show_text` with a short text:
   nothing of the long one may be left.

### Results

| Measurement | Value | Date |
| --- | --- | --- |
| hwcheck screen 1: all five rows visible at once, or `HW ERR display labels` | | |
| hwcheck screen 2: only "Zeile A" and "Zeile B", or which rows of screen 1 are left | | |
| hwcheck screen 3: which of L0 to L11 are visible | | |
| Robot script: status bar and every line of a long `show_text` visible | | |
| Robot script: short `show_text` after a long one leaves nothing behind | | |

### What follows

- `HW ERR display labels: TypeError`: the firmware does not take `index=`. Try the positional
  form (`show_label(text, size, x, y, i)`) in `hwcheck.py` and `hw_screen`.
- Fewer than L0 to L7 visible on screen 3: the screens in `ui.py` must get by with that many
  labels (fewer lines for `show_text`, or the status bar as one label); set `LABELS` in
  `sim/fakes/cyberpi.py` to the number found, and the test names the screens that are too big.
- Screen 2 shows only its two rows: `clear()` removes every label, and `hw_screen` can stop
  emptying unused labels (`hw_label_count`). Rows of screen 1 left over: keep it.
- Then replace the `VERIFY` notes at `hw_screen` and the assumptions in `sim/fakes/cyberpi.py`
  with what was measured.

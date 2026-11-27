# Measurements on the robot

Values the code needs but no documentation gives. Each section says how to measure, what the
result decides, and has a table to fill in. Until a value is filled in, the code uses a start value
marked VERIFY.

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

## Blocked: stalled wheels and a lifted nose

A motion that is held up by something the ultrasonic sensor does not see ends with `blocked`
(`docs/protocol.md`, "Rules"). Every threshold behind it is a start value chosen without a robot;
the simulator only shows that the logic works, not that the numbers fit. Measure them before
relying on `blocked`, and above all check that free driving never reports it.

### Steps

1. **Pitch sign and level.** With `robot/tools/hwcheck.py` (line `pitch_roll`) or the control
   room: note the pitch standing on the floor, then with the front lifted about 2 cm. If the
   lifted pitch is lower than the level one, set `PITCH_NOSE_UP_SIGN = -1` in `robot/src/hw.py`.
   The level value itself does not matter (the robot measures against its pitch at rest), only
   that it is steady.
2. **Pitch while driving free.** Log the pitch during `drive` at 20, 40 and 80 RPM and during
   `follow_line_until`: the highest rise above the level and how long it lasts, especially while
   speeding up and stopping. `SN_NOSE_ON_DEG` must lie clearly above it, or `SN_NOSE_STABLE`
   (readings at 25 ms) must outlast it.
3. **Pitch on the obstacle.** Drive against the obstacle of the stuck run and against the lowest
   one the field may hold. Note the pitch the robot reaches and whether the wheels stand or slip.
   `SN_NOSE_ON_DEG` must lie below that pitch; `SN_NOSE_OFF_DEG` about half of it.
4. **Wheels held.** Hold a wheel (or drive against a heavy box) at 20, 40 and 80 RPM and read the
   encoder angles: does a held wheel stand, creep or jump? `SN_STALL_RATIO` must lie above what a
   held wheel still does and well below what a free wheel does on a weak battery.
5. **Speeding up.** From the encoder log of a free `drive` at 80 RPM: the time until the wheels
   reach a quarter of the commanded travel. `SN_STALL_MS` must be clearly longer.
6. **Slowest speed.** The lowest RPM at which a wheel still turns steadily on the floor
   (`drive` with `speed` 5, `drive_arc` with radius 10 and speed 5, whose inner wheel is told
   2 RPM). `SN_STALL_MIN_RPM` must not lie below it, or slow motions end with `blocked`.
7. **Loop time.** The stall check reads both encoders once more per loop iteration while the
   motors run. Check that the loop stays within its 20 to 50 ms.
8. **False alarms.** A full lap of the field with drives, turns, arcs, scans and line following,
   on a full and on a nearly empty battery: no result and no `skill_aborted` event with `blocked`.

### Results

| Constant | Start value | Measured | Date |
| --- | --- | --- | --- |
| `PITCH_NOSE_UP_SIGN` (`hw.py`): sign that makes a lifted nose positive | 1 | | |
| Pitch at rest on the floor (degrees) | not used | | |
| Highest pitch rise while driving free, and its duration | unknown | | |
| Pitch reached on the obstacle; wheels stand or slip | unknown | | |
| `SN_NOSE_ON_DEG` (`sense.py`): rise above the level that counts as nose up | 8° | | |
| `SN_NOSE_OFF_DEG`: nose down again below this | 4° | | |
| `SN_NOSE_STABLE`: readings in a row before the state flips | 8 (about 0.2 s) | | |
| `SN_LEVEL_SMOOTH`: readings over which the level at rest follows the pitch | 32 (about 0.8 s) | | |
| `SN_STALL_RATIO`: share of the commanded travel below which a wheel counts as held | 0.25 | | |
| `SN_STALL_MS`: how long a wheel must stay behind | 400 ms | | |
| `SN_STALL_MIN_RPM`: a wheel commanded slower is not judged | 5 RPM | | |
| Loop time with the stall check, motors running | unknown | | |

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

## Calls and sensors: what the new tools may use

The handoff "Neue Tools im mBot2 Agent Lab" names vendor calls whose behaviour the documentation
leaves open. `robot/tools/measure.py` measures them. Upload it alone (wheels in the air), follow
the display (clap, cover the light, lift, shake, move the joystick) and paste the serial log.
`MS DIR` lines list what each vendor module really offers.

A call counts as blocking when it takes longer than 20 ms to return. A blocking call is not used
in the robot script; the tool that needs it is not built.

| Measurement | Decides | Value | Date |
| --- | --- | --- | --- |
| `audio.play_tone(440, 0.5)` returns after | `beep` | | |
| `audio.play_drum(…)` returns after | rhythm via `beep` | | |
| `audio.record()` / `stop_record()` return after | `record_sound` | | |
| `audio.play_record()` returns after | `play_recording` | | |
| `led.on` all / one LED, `led.show`, `led.set_bri` (ms each) | `led_effect` update rate | | |
| `ultrasonic2.set_bri`, `led_show` (ms, signature that works) | `eyes` | | |
| Eye LED order (which LED is 1 … 8) | `eyes` patterns | | |
| `display.show_label`, `display.clear` (ms) | `show`, `ask_human` | | |
| `get_bri` with LEDs off / white / red / covered | `light_up`/`light_down`, suppression during effects | | |
| `get_loudness` quiet / motors / clapping | `clap` thresholds | | |
| `get_acc` quiet / motors / bump; `acc_z` when lifted | `bumped`, `lifted` | | |
| `is_shake`, `get_shakeval` while shaking | `shake` | | |
| `is_freefall` exists, value when lifted | `lifted` | | |
| Joystick key names for `controller.is_press` | `joystick` | | |
| `get_firmware_version`, `get_name`, `set_name`, `restart` exist | `hello`, `set_name`, `restart` | | |

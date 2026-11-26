# CyberPi firmware: custom firmware, script size and module loading

Research notes on whether replacing or working around the stock CyberPi firmware would let the
robot script grow or run faster. Nothing here is decided: a change to how the robot script gets
onto the CyberPi is a deviation from `docs/decisions.md` ("MicroPython in upload mode, uploaded
with mBlock") and needs approval first.

## Summary

- Custom firmware (Arduino/PlatformIO) exists, but it loses the mBot2 shield and the mBuild
  modules: motors, encoders, quad RGB sensor and ultrasonic. Not an option for us.
- The stock firmware's real limit for us is the size of one `main.py` (between 80 and 120 KB),
  not RAM or flash. The bundle is 43 KB today against a 60 KB budget, room for about ten more
  motion skills.
- Splitting the script into modules on the flash, precompiling them to `.mpy`, and loading them
  over Wi-Fi could remove that limit and the per-robot USB upload. None of this is tested on the
  device yet.
- None of these options makes the control loop, sensor reads or MQTT faster.

## Hardware and stock firmware

| Item | Value | Source |
| --- | --- | --- |
| Module | ESP32-WROVER-B, 8 MB flash, PSRAM | community Arduino projects |
| MicroPython | 1.11.0, Makeblock build 44.01.008 of 2022-09-21 | `docs/measurements.md` |
| Free heap without script | about 1.2 MB (PSRAM) | `docs/measurements.md` |
| Free flash | about 2.1 of 3.1 MB | `docs/measurements.md` |
| Main thread stack | 6 KB (the robot runs in its own 32 KB thread) | `docs/measurements.md` |

The motors, encoders and mBuild sensors are not wired to the ESP32 directly. They sit behind the
mBot2 shield, which has its own microcontroller, and the CyberPi talks to it through Makeblock's
MicroPython modules (`mbot2`, `mbuild`). The protocol between CyberPi and shield is not
documented, and no reverse-engineering work on it was found.

## Custom firmware

People do flash the CyberPi with their own firmware, using the ESP32 Arduino core:

- [PerfecXX/mBot2](https://github.com/PerfecXX/mBot2): MicroPython and Arduino (PlatformIO)
  examples. The PlatformIO template uses `board = esp32dev` with PSRAM flags and a bundled
  display library. The repo also keeps backups of stock firmware images (`firmware/`).
- [oshsen4262-art/CyberPi-Arduino-Projects](https://github.com/oshsen4262-art/CyberPi-Arduino-Projects):
  emulators, games and tools using the display, speaker and buttons.

What custom firmware costs:

- **No shield and no mBuild.** PerfecXX's support table lists the mBot2 shield, the pocket
  shield and all mBuild modules as MicroPython only. Driving, encoders, line sensor and
  ultrasonic would need the shield protocol rebuilt from scratch.
- CyberPiOS, upload from mBlock and Bluetooth controller support are gone until the stock
  firmware is restored (firmware update in mBlock).
- Files stored on the CyberPi are lost.

Conclusion: custom firmware would only pay off if C code had to replace MicroPython in the
control loop, and even then only after the shield protocol is reverse-engineered.

## The script size limit

From `docs/measurements.md`: an 80 KB `main.py` starts, a 120 KB one makes the CyberPi hang at
start without output, after which mBlock can no longer upload until recovery. RAM is not the
cause: 80 KB of source still leave about 1.17 MB of heap. Whether the hang comes from compiling
one large module or from how the firmware handles a large `main.py` is unknown.

The bundle is 43 KB today (`robot/tools/bundle.py`, comments stripped), and
`test_bundle_size_budget` caps it at 60 KB. A motion skill takes 1.3 to 1.9 KB of the bundle
(`drive_arc` 1.4 KB, `retrace_path` 1.9 KB), so the 17 KB left hold roughly ten more of them.

## Options within the stock firmware

### Modules on the flash

A script on the CyberPi can write files to the flash with `open(path, "wb")`. PerfecXX's
"CyberPi File Manager" (`tools/01-CyberPi File Manager`) runs a small TCP server on the CyberPi
that receives files over Wi-Fi this way. If the CyberPi imports modules from the flash, the robot
script could be split into several files, each compiled on import on its own. That would turn
the limit on one file into a limit per module.

### Precompiled `.mpy`

`.mpy` files skip compiling on the device. MicroPython 1.11 loads `.mpy` version 4 only, made by
`mpy-cross` 1.11 ([version table](https://docs.micropython.org/en/v1.13/reference/mpyfiles.html)).
[`mpy-cross` 1.11 on PyPI](https://pypi.org/project/mpy-cross/1.11) has no wheel for Apple
Silicon (x86_64 macOS, Linux and Windows only); otherwise it has to be built from the MicroPython
`v1.11` tag. The upstream ESP32 port of 1.11 enables `.mpy` loading
(`MICROPY_PERSISTENT_CODE_LOAD`); whether Makeblock's build keeps it, and which feature flags the
`.mpy` header must carry, is unknown.

`.mpy` speeds up loading only. It is the same bytecode, so the running script is not faster.

### Loading over Wi-Fi

A small fixed `main.py`, uploaded once with mBlock, could fetch the modules from the lab server
and import them. Every robot would then get a new script without a USB cable per robot. This is
the deviation from `docs/decisions.md` mentioned at the top.

## What each option changes

| Bottleneck | Custom firmware | Modules on flash / `.mpy` | Loading over Wi-Fi |
| --- | --- | --- | --- |
| Size of the robot script | yes | yes, if imports from flash work | yes, with modules |
| Start-up time | yes | `.mpy` only | no |
| Upload per robot over USB | no | no | yes |
| Control loop speed (about 40 Hz) | possible, with C | no | no |
| Sensor and motor latency (shield) | only with the shield protocol | no | no |
| MQTT and Wi-Fi latency | no | no | no |

## Open questions

1. **Imports from the flash.** Write generated modules to the flash from a test script (200 B,
   40 KB, then four of 30 KB, 120 KB in all), import each, and record the working directory,
   `sys.path`, import time and heap per module. If the four 30 KB modules load, the limit is per
   file. If the CyberPi hangs, compiling itself is the limit and `.mpy` is the next test.
2. **`.mpy` on the device.** Build `mpy-cross` 1.11, compile a small module with and without
   `-mcache-lookup-bc`, write both to the flash and import them.
3. **Where the loop time goes.** There are no measurements of time per loop iteration split into
   sensor reads, `sense.py`, MQTT and skill logic. Without them it is unknown whether any of the
   options above addresses a real bottleneck in driving.

#!/usr/bin/env python3
"""Run ONE simulated robot: the real bundled robot script with fake vendor modules.

Normally started by sim/run.py; can also be used directly for debugging.
"""

import argparse
import pathlib
import random
import runpy
import sys
import threading
import time
import types

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE / "fakes"))
sys.path.insert(0, str(HERE))

import simstate  # noqa: E402  (fakes/simstate.py)
from world import World  # noqa: E402


def make_config(a):
    cfg = types.ModuleType("config")
    cfg.ROBOT_ID = a.id
    cfg.WIFI_SSID = "sim"
    cfg.WIFI_PASSWORD = "sim"
    cfg.BROKER_HOST = a.host
    cfg.BROKER_PORT = a.port
    cfg.BROKER_TLS = a.tls
    cfg.BROKER_PASSWORD = a.password
    return cfg


# Mirrors COLOR_RGB in robot/src/hw.py, to name LED colors in the trace.
LED_COLORS = {
    "red": (255, 0, 0), "green": (0, 255, 0), "blue": (0, 0, 255),
    "yellow": (255, 200, 0), "purple": (160, 0, 255), "cyan": (0, 255, 255),
    "white": (255, 255, 255), "orange": (255, 100, 0),
}
BEEP_HZ = 880  # hw_beep in robot/src/hw.py


def led_name(rgb):
    """Name of the closest LED color, ignoring brightness."""
    top = max(rgb)
    if top == 0:
        return "off"
    norm = [c * 255.0 / top for c in rgb]
    return min(LED_COLORS, key=lambda n: sum((a - b) ** 2 for a, b in zip(norm, LED_COLORS[n])))


def drain(items):
    """Take what a robot thread appended so far, leaving later appends."""
    n = len(items)
    out = items[:n]
    del items[:n]
    return out


def runs(names):
    """Collapse repeats: ["off", "off", "white"] -> ["off", "white"] and counts."""
    out = []
    for n in names:
        if out and out[-1][0] == n:
            out[-1][1] += 1
        else:
            out.append([n, 1])
    return out


def trace_line(world):
    """Pose, motors, LEDs and sounds since the last line, and the display."""
    s = world.snapshot()
    leds = [n for n, _ in runs(led_name(c) for c in drain(simstate.led_trail))] or [led_name(simstate.leds)]
    line = "[%s] x=%s y=%s heading=%s motors=%s/%s leds=%s" % (
        world.robot_id, s["x"], s["y"], s["heading"], s["left_rpm"], s["right_rpm"], "/".join(leds))
    sounds = ["beep" if x == BEEP_HZ else ("%sHz" % x if isinstance(x, (int, float)) else str(x))
              for x in drain(simstate.sounds)]
    if sounds:
        line += " sound=" + ",".join(n if k == 1 else "%sx%d" % (n, k) for n, k in runs(sounds))
    return line + " | display: " + " / ".join(simstate.display)


def trace(world, period):
    while True:
        time.sleep(period)
        print(trace_line(world), flush=True)


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--id", required=True)
    p.add_argument("--password", required=True)
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--port", type=int, default=1883)
    p.add_argument("--tls", action="store_true")
    p.add_argument("--world", default=str(HERE / "worlds" / "arena.json"))
    p.add_argument("--start", type=int, default=0, help="index into the world's start poses")
    p.add_argument("--bundle", required=True, help="bundled main.py built without --config")
    p.add_argument("--trace", type=float, default=0, help="print pose, LEDs and sounds every N seconds")
    p.add_argument("--no-noise", action="store_true", help="ideal sensors instead of realistic noise")
    p.add_argument("--seed", type=int, default=None, help="seed for the sensor noise (default: random)")
    a = p.parse_args(argv)

    noise = None if a.no_noise else random.Random(a.seed)
    simstate.world = World.load(a.world, a.id, a.start, noise=noise)
    sys.modules["config"] = make_config(a)
    if a.trace > 0:
        threading.Thread(target=trace, args=(simstate.world, a.trace), daemon=True).start()
    runpy.run_path(a.bundle, run_name="__main__")


if __name__ == "__main__":
    main()

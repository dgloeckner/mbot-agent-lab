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


def trace(world, period):
    while True:
        time.sleep(period)
        s = world.snapshot()
        print("[%s] x=%s y=%s heading=%s motors=%s/%s | display: %s" % (
            world.robot_id, s["x"], s["y"], s["heading"], s["left_rpm"], s["right_rpm"],
            " / ".join(simstate.display)), flush=True)


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
    p.add_argument("--trace", type=float, default=0, help="print the pose every N seconds")
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

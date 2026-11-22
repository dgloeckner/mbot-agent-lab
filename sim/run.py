#!/usr/bin/env python3
"""Start n simulated robots against a lab server.

Each robot runs in its own process with its own copy of the world (robots do
not see each other yet). The robot script is bundled fresh from robot/src.

Passwords come from MBOT<n>_PASSWORD, defaulting to "dev" (see deploy/local).

Example:
    python3 sim/run.py --robots 2 --world sim/worlds/labyrinth.json --trace 1
"""

import argparse
import os
import pathlib
import signal
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent
sys.path.insert(0, str(ROOT / "robot" / "tools"))

import bundle  # noqa: E402


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--robots", type=int, default=2)
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--port", type=int, default=1883)
    p.add_argument("--world", default=str(HERE / "worlds" / "arena.json"))
    p.add_argument("--trace", type=float, default=0)
    p.add_argument("--no-noise", action="store_true", help="ideal sensors (default: realistic noise)")
    a = p.parse_args(argv)

    build = HERE / ".build" / "main.py"
    build.parent.mkdir(exist_ok=True)
    build.write_text(bundle.bundle(), encoding="utf-8")

    procs = []
    for i in range(1, a.robots + 1):
        robot_id = "mbot-%d" % i
        cmd = [sys.executable, str(HERE / "robot_proc.py"),
               "--id", robot_id,
               "--password", os.environ.get("MBOT%d_PASSWORD" % i, "dev"),
               "--host", a.host, "--port", str(a.port),
               "--world", a.world, "--start", str(i - 1),
               "--bundle", str(build), "--trace", str(a.trace)] + (["--no-noise"] if a.no_noise else [])
        procs.append(subprocess.Popen(cmd))
        print("sim: started %s" % robot_id, file=sys.stderr)

    def stop(*_):
        for pr in procs:
            pr.terminate()

    signal.signal(signal.SIGTERM, stop)
    try:
        for pr in procs:
            pr.wait()
    except KeyboardInterrupt:
        stop()
        for pr in procs:
            pr.wait()


if __name__ == "__main__":
    main()

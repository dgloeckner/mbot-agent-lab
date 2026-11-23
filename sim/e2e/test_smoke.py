"""End-to-end smoke test: real labserver + one simulated robot + scripted agent.

Starts the built `labserver` binary with the scripted fake model (no API key,
no network), starts one robot through sim/run.py, then drives the HTTP side
like a lab: the control room logs in, a team joins, gets the robot, deploys a
prompt, starts its agent and sends it a message. It checks that the robot's
heartbeat reaches the registry, the run starts, a robot command comes back
with a result and the simulated robot moved.

Run with `npx nx run sim:e2e` (builds the server first). The binary comes from
$LABSERVER or bin/labserver.
"""

import json
import os
import pathlib
import re
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from http.cookiejar import CookieJar

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent.parent
BINARY = pathlib.Path(os.environ.get("LABSERVER", ROOT / "bin" / "labserver"))
BASE_CONFIG = ROOT / "deploy" / "local" / "lab.yaml"
WORLD = ROOT / "sim" / "worlds" / "arena.json"
ROBOT = "mbot-1"
SEED = 1234
TOTAL_TIMEOUT = 60  # seconds for the whole chain
POSE = re.compile(r"^\[(\S+)\] x=(-?[\d.]+) y=(-?[\d.]+) heading=(-?\d+)")

# Skills that send a cmd to the robot and wait for its result.
ROBOT_TOOLS = {"drive_until", "turn", "turn_until_clear", "drive", "stop"}


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def test_config(http_port, mqtt_port, db_path):
    """deploy/local/lab.yaml with free ports and a temp database."""
    text = BASE_CONFIG.read_text(encoding="utf-8")
    subs = [
        (r'http_addr: "[^"]*"', 'http_addr: "127.0.0.1:%d"' % http_port),
        (r'mqtt_plain_addr: "[^"]*"', 'mqtt_plain_addr: "127.0.0.1:%d"' % mqtt_port),
        (r"db_path: \S+", "db_path: %s" % json.dumps(str(db_path))),
    ]
    for pattern, repl in subs:
        text, n = re.subn(pattern, repl, text, count=1)
        if n != 1:
            raise AssertionError("%s: no match for %r" % (BASE_CONFIG, pattern))
    if not re.search(r"^\s*default: test\s*$", text, re.M):
        raise AssertionError("%s must use the scripted model (default: test)" % BASE_CONFIG)
    return text


class SmokeTest(unittest.TestCase):
    def setUp(self):
        if not BINARY.exists():
            self.fail("%s missing: run `npx nx run server:build` (or set LABSERVER)" % BINARY)
        self.deadline = time.monotonic() + TOTAL_TIMEOUT
        self.tmp = pathlib.Path(tempfile.mkdtemp(prefix="lab-e2e-"))
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.procs = []
        self.addCleanup(self.stop_all)

        http_port, mqtt_port = free_port(), free_port()
        self.base = "http://127.0.0.1:%d" % http_port
        cfg = self.tmp / "lab.yaml"
        cfg_text = test_config(http_port, mqtt_port, self.tmp / "lab.db")
        cfg.write_text(cfg_text, encoding="utf-8")

        # Dev secrets, as in the server:dev target. Never in the YAML.
        self.admin_password = secrets.token_hex(8)
        self.lab_code = "E2E1"
        robot_password = secrets.token_hex(8)
        env = dict(os.environ, LAB_ADMIN_PASSWORD=self.admin_password, LAB_CODE=self.lab_code,
                   MBOT1_PASSWORD=robot_password, MBOT2_PASSWORD=robot_password,
                   MBOT3_PASSWORD=robot_password)
        # Keys of providers other than the default are optional: drop them so
        # the server starts without any, and real keys never reach the test.
        for var in re.findall(r"api_key_env:\s*(\w+)", cfg_text):
            env.pop(var, None)

        self.server_log = self.tmp / "server.log"
        self.sim_log = self.tmp / "sim.log"
        self.spawn([str(BINARY), "-config", str(cfg), "-debug"], env, self.server_log)
        self.wait_for(self.healthy, "server /healthz")
        self.spawn([sys.executable, str(ROOT / "sim" / "run.py"), "--robots", "1",
                    "--port", str(mqtt_port), "--world", str(WORLD),
                    "--trace", "0.2", "--seed", str(SEED)], env, self.sim_log)

        self.admin = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(CookieJar()))
        self.events = []
        self.events_lock = threading.Lock()

    # --- processes and logs ------------------------------------------------

    def spawn(self, cmd, env, log_path):
        log = open(log_path, "wb")
        self.addCleanup(log.close)
        self.procs.append(subprocess.Popen(cmd, env=env, stdout=log, stderr=subprocess.STDOUT,
                                           cwd=str(self.tmp), start_new_session=True))

    def stop_all(self):
        for p in reversed(self.procs):
            if p.poll() is None:
                os.killpg(p.pid, signal.SIGTERM)
        for p in reversed(self.procs):
            try:
                p.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(p.pid, signal.SIGKILL)
                p.wait()

    def logs(self):
        out = []
        for name, path in (("server", self.server_log), ("sim", self.sim_log)):
            lines = path.read_text(encoding="utf-8", errors="replace").splitlines() if path.exists() else []
            if name == "sim":  # the pose trace would drown everything else; keep its tail
                trace = [ln for ln in lines if POSE.match(ln)]
                lines = [ln for ln in lines if not POSE.match(ln)] + trace[-5:]
            out.append("----- %s log (last 60 of %d lines) -----" % (name, len(lines)))
            out.extend(lines[-60:])
        with self.events_lock:
            out.append("----- team log (%d entries) -----" % len(self.events))
            out.extend(json.dumps(e, ensure_ascii=False) for e in self.events[-30:])
        return "\n".join(out)

    def wait_for(self, probe, what):
        """Poll probe until it returns something truthy; fail with logs at the deadline."""
        while True:
            for p in self.procs:
                if p.poll() is not None:
                    self.fail("process %s exited with %s while waiting for %s\n%s" % (p.args[0], p.returncode, what, self.logs()))
            got = probe()
            if got:
                return got
            if time.monotonic() > self.deadline:
                self.fail("timeout after %ds waiting for %s\n%s" % (TOTAL_TIMEOUT, what, self.logs()))
            time.sleep(0.1)

    # --- HTTP ----------------------------------------------------------------

    def request(self, method, path, body=None, opener=None, expect=(200,)):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"})
        try:
            with (opener or self.admin).open(req, timeout=10) as resp:
                status, raw = resp.status, resp.read()
        except urllib.error.HTTPError as e:
            status, raw = e.code, e.read()
        if status not in expect:
            self.fail("%s %s: HTTP %d %s\n%s" % (method, path, status, raw.decode(errors="replace"), self.logs()))
        return json.loads(raw) if raw else None

    def healthy(self):
        try:
            with urllib.request.urlopen(self.base + "/healthz", timeout=1) as resp:
                return resp.status == 200
        except OSError:
            return False

    def follow_team_log(self, token):
        """Collect the team's SSE log entries in the background."""
        resp = urllib.request.urlopen(self.base + "/t/%s/events" % token, timeout=30)
        closing = threading.Event()

        def close():
            closing.set()
            resp.close()

        self.addCleanup(close)

        def read():
            event = None
            try:
                for raw in resp:
                    line = raw.decode().rstrip("\n")
                    if line.startswith("event: "):
                        event = line[7:]
                    elif line.startswith("data: ") and event == "log":
                        with self.events_lock:
                            self.events.append(json.loads(line[6:]))
            except Exception as e:  # closing the response under the reader raises various errors
                if not closing.is_set():
                    with self.events_lock:
                        self.events.append({"kind": "e2e_reader_error", "text": repr(e)})

        threading.Thread(target=read, daemon=True).start()

    def log_entries(self, match):
        with self.events_lock:
            return [e for e in self.events if match(e)]

    # --- robot pose from the sim trace ---------------------------------------

    def poses(self):
        if not self.sim_log.exists():
            return []
        out = []
        for line in self.sim_log.read_text(encoding="utf-8", errors="replace").splitlines():
            m = POSE.match(line)
            if m and m.group(1) == ROBOT:
                out.append((float(m.group(2)), float(m.group(3)), int(m.group(4))))
        return out

    # --- the chain -------------------------------------------------------------

    def test_team_agent_drives_simulated_robot(self):
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))

        def robot_online():
            st = self.request("GET", "/admin/api/state")
            for rb in st["robots"]:
                if rb["id"] == ROBOT and rb["online"] and rb["accepted"] and rb["state"]["seq"] > 0:
                    return rb
            return None

        rb = self.wait_for(robot_online, "heartbeat of %s in the registry" % ROBOT)
        self.assertTrue(rb["script"], "hello without script version")
        start_pose = self.wait_for(lambda: self.poses()[:1], "sim pose trace")[0]

        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Testteam"},
                            opener=urllib.request.build_opener(), expect=(201,))
        token = team["token"]
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(token)

        def team_robot():
            s = self.request("GET", "/t/%s/api/session" % token)
            return s["agent_available"] and any(r["id"] == ROBOT and r["online"] for r in s["robots"])

        self.wait_for(team_robot, "robot assigned to the team")

        t = "/t/%s" % token
        self.request("POST", t + "/prompt", {"text": "Erkunde die Arena und fahre bis zur Wand."}, expect=(201,))
        status = self.request("POST", t + "/start", expect=(202,))
        self.assertTrue(status["running"], status)
        self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "run_start"), "run_start in the team log")
        self.request("POST", t + "/message", {"text": "Fahrt los!"}, expect=(202,))

        def robot_result():
            for e in self.log_entries(lambda e: e.get("kind") == "tool"):
                d = e.get("data") or {}
                res = d.get("result") or {}
                if d.get("name") in ROBOT_TOOLS and "ok" in res and res.get("reason") not in ("offline", "timeout"):
                    return d
            return None

        call = self.wait_for(robot_result, "a robot cmd answered with a result")
        self.assertNotIn("error", call["result"], call)

        def moved():
            for x, y, heading in self.poses():
                if abs(x - start_pose[0]) > 2 or abs(y - start_pose[1]) > 2 or abs(heading - start_pose[2]) > 5:
                    return (x, y, heading)
            return None

        end_pose = self.wait_for(moved, "the simulated robot to move")
        print("e2e: %s moved %s -> %s after %s" % (ROBOT, start_pose, end_pose, call["name"]), file=sys.stderr)

        # The scripted model keeps calling the model; the run stays healthy.
        st = self.request("GET", "/admin/api/state")
        agent = next(x for x in st["teams"] if x["id"] == team["id"])["agent"]
        self.assertTrue(agent["running"], agent)
        self.assertGreater(agent["calls"], 0, agent)


if __name__ == "__main__":
    unittest.main()

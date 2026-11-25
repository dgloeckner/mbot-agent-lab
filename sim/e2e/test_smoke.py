"""End-to-end smoke test: real labserver + simulated robots + scripted agent.

Starts the built `labserver` binary with the scripted fake model (no API key,
no network), starts one robot through sim/run.py, then drives the HTTP side
like a lab: the control room logs in, a team joins, gets the robot, deploys a
prompt, starts its agent and sends it a message. It checks that the robot's
heartbeat reaches the registry, the run starts, a robot command comes back
with a result and the simulated robot moved. After a stop, a message gets an
answer in the conversation without starting a run. A second test gives one
team two robots and checks that they move at the same time.

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
ROBOT2 = "mbot-2"
SEED = 1234
TOTAL_TIMEOUT = 60  # seconds for the whole chain
POSE = re.compile(r"^\[(\S+)\] x=(-?[\d.]+) y=(-?[\d.]+) heading=(-?\d+)")

# Skills that send a cmd to the robot and wait for its result.
ROBOT_TOOLS = {"drive_until", "turn", "turn_until_clear", "drive", "stop"}
MOTION_TOOLS = ROBOT_TOOLS - {"stop"}


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


class LabCase(unittest.TestCase):
    """A lab: labserver, simulated robots in WORLD and HTTP helpers. No tests."""

    WORLD = WORLD
    ROBOTS = 2
    DELAY_MS = 0  # network delay of the simulated robots (sim/run.py --delay-ms)
    CONFIG = ()   # extra (pattern, replacement) edits of the lab configuration

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
        for pattern, repl in self.CONFIG:
            cfg_text, n = re.subn(pattern, repl, cfg_text, count=1)
            if n != 1:
                raise AssertionError("lab.yaml: no match for %r" % pattern)
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
        self.spawn([sys.executable, str(ROOT / "sim" / "run.py"), "--robots", str(self.ROBOTS),
                    "--port", str(mqtt_port), "--world", str(self.WORLD),
                    "--trace", "0.2", "--seed", str(SEED), "--delay-ms", str(self.DELAY_MS)], env, self.sim_log)

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
                        entry = json.loads(line[6:])
                        entry["e2e_received"] = time.monotonic()
                        with self.events_lock:
                            self.events.append(entry)
            except Exception as e:  # closing the response under the reader raises various errors
                if not closing.is_set():
                    with self.events_lock:
                        self.events.append({"kind": "e2e_reader_error", "text": repr(e)})

        threading.Thread(target=read, daemon=True).start()

    def log_entries(self, match):
        with self.events_lock:
            return [e for e in self.events if match(e)]

    # --- robot pose from the sim trace ---------------------------------------

    def poses(self, robot=ROBOT):
        if not self.sim_log.exists():
            return []
        out = []
        for line in self.sim_log.read_text(encoding="utf-8", errors="replace").splitlines():
            m = POSE.match(line)
            if m and m.group(1) == robot:
                out.append((float(m.group(2)), float(m.group(3)), int(m.group(4))))
        return out

    # --- the chain -------------------------------------------------------------

    def open_task(self, team, title):
        """The control room prepares a task and assigns it to the team."""
        task = self.request("POST", "/admin/api/tasks", {"title": title, "body": "Erkundet die Arena."}, expect=(201,))
        self.request("POST", "/admin/api/tasks/%s/publish" % task["id"])
        self.request("POST", "/admin/api/tasks/%s/assign" % task["id"], {"teams": [team["id"]]})
        return task

    def team_request(self, team, method, path, body=None, expect=(200,)):
        return self.request(method, "/t/%s%s" % (team["token"], path), body, opener=urllib.request.build_opener(), expect=expect)

    def give_task(self, team, title):
        """The control room prepares a task and assigns it; the team accepts it."""
        task = self.open_task(team, title)
        accepted = self.team_request(team, "POST", "/tasks/%s/accept" % task["id"], {"prompt": "empty"})
        self.assertEqual(accepted["status"], "active", accepted)
        return task

    def robot_online(self, robot):
        def probe():
            st = self.request("GET", "/admin/api/state")
            for rb in st["robots"]:
                if rb["id"] == robot and rb["online"] and rb["accepted"] and rb["state"]["seq"] > 0:
                    return rb
            return None
        return self.wait_for(probe, "heartbeat of %s in the registry" % robot)

    def run_team_tool(self, team, example, name):
        """Saves an example team tool, runs it and waits for its end in the log."""
        ex = json.loads((ROOT / "docs" / "examples" / "programs" / (example + ".json")).read_text(encoding="utf-8"))
        saved = self.team_request(team, "POST", "/programs", {"name": name, "description": ex["program"]["description"],
                                                               "blockly": ex["blockly"]}, expect=(201,))
        self.assertTrue(saved["valid"], saved)
        run = self.team_request(team, "POST", "/programs/%s/run" % saved["id"], {}, expect=(202,))["run"]
        end = self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "program_end" and (e.get("data") or {}).get("run") == run),
                            "the end of team tool %s" % name)[0]
        for line in self.server_log.read_text(encoding="utf-8").splitlines():
            if "team tool" in line:
                print("e2e: server: " + line, file=sys.stderr)
        return end["data"]["result"]


class SmokeTest(LabCase):
    def test_team_agent_drives_simulated_robot(self):
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        rb = self.robot_online(ROBOT)
        self.assertTrue(rb["script"], "hello without script version")
        start_pose = self.wait_for(lambda: self.poses()[:1], "sim pose trace")[0]

        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Testteam"},
                            opener=urllib.request.build_opener(), expect=(201,))
        token = team["token"]
        self.give_task(team, "Arena")
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

        # Without a run, a team message goes to the conversation: the agent
        # answers there, may read the sensors, and moves nothing.
        self.request("POST", t + "/stop", expect=(200,))
        self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "run_end"), "run_end in the team log")
        sent = self.request("POST", t + "/message", {"text": "Wie weit ist die Wand weg?"}, expect=(202,))
        self.assertEqual(sent.get("channel"), "conversation", sent)

        def conversation(e):
            return e.get("channel") == "conversation"

        answer = self.wait_for(lambda: self.log_entries(lambda e: conversation(e) and e.get("source") == "agent" and e.get("kind") == "text"),
                               "the agent's answer in the conversation")
        tools = [(e.get("data") or {}).get("name") for e in self.log_entries(lambda e: conversation(e) and e.get("kind") == "tool")]
        self.assertTrue(set(tools) <= {"read_sensors", "remember"}, tools)
        self.assertFalse(self.log_entries(lambda e: e.get("kind") == "run_start" and e.get("data", {}).get("run") != status.get("run_id")),
                         "the conversation started a run")
        print("e2e: conversation answer %r after %s" % (answer[0].get("text"), tools), file=sys.stderr)

    def test_drive_by_hand(self):
        """Stage 1: the team is the agent. It drives its robot by hand with
        one tool; the call shows up in the log. During a run it is refused."""
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        self.robot_online(ROBOT)
        start_pose = self.wait_for(lambda: self.poses()[:1], "sim pose trace")[0]
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Handbetrieb"},
                            opener=urllib.request.build_opener(), expect=(201,))
        self.give_task(team, "Konsole")
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(team["token"])
        self.wait_for(lambda: any(r["online"] for r in self.team_request(team, "GET", "/api/session")["robots"]), "robot assigned")

        res = self.team_request(team, "POST", "/tools/drive", {"cm": 20})
        self.assertTrue(res["ok"], res)
        self.assertEqual(res["reason"], "done", res)
        self.assertGreater(res["driven_cm"], 15, res)
        self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "tool" and e.get("source") == "team"), "the by-hand call in the log")
        end = self.poses()[-1]
        moved = ((end[0] - start_pose[0]) ** 2 + (end[1] - start_pose[1]) ** 2) ** 0.5
        self.assertGreater(moved, 15, (start_pose, end))
        print("e2e: by hand drive %s -> %s (%.1f cm)" % (start_pose, end, moved), file=sys.stderr)

        self.team_request(team, "POST", "/prompt", {"text": "Warte."}, expect=(201,))
        self.team_request(team, "POST", "/start", expect=(202,))
        refused = self.team_request(team, "POST", "/tools/drive", {"cm": 20}, expect=(409,))
        self.assertEqual(refused["error"], "Erst anhalten: Euer Agent fährt gerade.")
        self.team_request(team, "POST", "/stop")
        for line in self.server_log.read_text(encoding="utf-8").splitlines():
            if "tool by hand" in line:
                print("e2e: server: " + line, file=sys.stderr)

    def test_save_team_tool(self):
        """A team saves the quadrat example over the API and reads it back as valid."""
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Baumeister"},
                            opener=urllib.request.build_opener(), expect=(201,))
        self.give_task(team, "Eigene Werkzeuge")
        example = json.loads((ROOT / "docs" / "examples" / "programs" / "quadrat.json").read_text(encoding="utf-8"))
        saved = self.team_request(team, "POST", "/programs", {"name": "quadrat", "description": "Fährt ein Quadrat.",
                                                               "blockly": example["blockly"]}, expect=(201,))
        self.assertTrue(saved["valid"], saved)
        back = self.team_request(team, "GET", "/programs/%s" % saved["id"])
        self.assertEqual(back["blockly"], example["blockly"])
        listed = self.team_request(team, "GET", "/programs")
        self.assertEqual([(p["name"], p["valid"]) for p in listed], [("quadrat", True)])
        for line in self.server_log.read_text(encoding="utf-8").splitlines():
            if "team tool saved" in line:
                print("e2e: server: " + line, file=sys.stderr)

    def test_quadrat_brings_the_robot_back(self):
        """Stage 2: a loop. The quadrat tool drives a square and ends near its start."""
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        self.robot_online(ROBOT)
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Quadrat"},
                            opener=urllib.request.build_opener(), expect=(201,))
        self.give_task(team, "Wiederholen")
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(team["token"])
        self.wait_for(lambda: any(r["online"] for r in self.team_request(team, "GET", "/api/session")["robots"]), "robot assigned")
        time.sleep(1.5)  # the robot learns its owner from the next heartbeat
        start = self.poses()[-1]
        result = self.run_team_tool(team, "quadrat", "quadrat")
        self.assertEqual(result["end"], "done", result)
        self.assertEqual(result["steps"], 8, result)
        end = self.poses()[-1]
        off = ((end[0] - start[0]) ** 2 + (end[1] - start[1]) ** 2) ** 0.5
        print("e2e: quadrat %s -> %s, %.1f cm off" % (start, end, off), file=sys.stderr)
        self.assertLess(off, 10, (start, end))

    def test_team_tools_only_task(self):
        """Challenge rule: the agent has only the team's own tools. Its call of
        a built-in motion tool comes back as unknown; its team tool runs."""
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        self.robot_online(ROBOT)
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Nur Eigene"},
                            opener=urllib.request.build_opener(), expect=(201,))
        task = self.request("POST", "/admin/api/tasks", {"title": "Challenge", "body": "Nur eigene Werkzeuge.", "agent_tools": "team_tools"}, expect=(201,))
        self.request("POST", "/admin/api/tasks/%s/publish" % task["id"])
        self.request("POST", "/admin/api/tasks/%s/assign" % task["id"], {"teams": [team["id"]]})
        self.team_request(team, "POST", "/tasks/%s/accept" % task["id"], {"prompt": "empty"})
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(team["token"])
        self.wait_for(lambda: any(r["online"] for r in self.team_request(team, "GET", "/api/session")["robots"]), "robot assigned")
        time.sleep(1.5)
        ex = json.loads((ROOT / "docs" / "examples" / "programs" / "quadrat.json").read_text(encoding="utf-8"))
        self.team_request(team, "POST", "/programs", {"name": "quadrat", "description": "Fährt ein Quadrat.", "blockly": ex["blockly"]}, expect=(201,))
        self.team_request(team, "POST", "/prompt", {"text": "Fahre mit `quadrat` ein Quadrat."}, expect=(201,))
        self.team_request(team, "POST", "/start", expect=(202,))
        self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "run_end"), "the end of the run")
        calls = {(e.get("data") or {}).get("name"): (e.get("data") or {}).get("result") for e in
                 self.log_entries(lambda e: e.get("kind") == "tool" and e.get("source") == "agent" and not (e.get("data") or {}).get("program"))}
        print("e2e: restricted task: drive_until -> %s; quadrat -> %s" % (calls.get("drive_until"), calls.get("quadrat")), file=sys.stderr)
        self.assertIn("gibt es nicht", calls["drive_until"]["error"])
        self.assertEqual(calls["quadrat"]["end"], "done")

        # The control room scores the finished run: the lab lead confirms it
        # and notes a hand on the robot; the run shows the four measures.
        scored = self.request("GET", "/admin/api/tasks/%s/runs" % task["id"])["teams"]
        self.assertEqual([t["name"] for t in scored], ["Nur Eigene"], scored)
        run = scored[0]["runs"][0]
        self.assertIsNone(run["success"], run)
        self.request("POST", "/admin/api/runs/%d/confirm" % run["run"], {"success": True}, expect=(204,))
        self.request("POST", "/admin/api/runs/%d/intervention" % run["run"], {"kind": "moved"}, expect=(204,))
        scored = self.request("GET", "/admin/api/tasks/%s/runs" % task["id"])["teams"]
        run = scored[0]["runs"][0]
        print("e2e: scored run: success=%s to_goal_s=%s calls=%s interventions=%s team_tools=%s end=%s" % (
            run["success"], run.get("to_goal_s"), run["calls"], run["interventions"], run["team_tools"], run.get("end_reason")), file=sys.stderr)
        self.assertTrue(run["success"], run)
        self.assertGreater(run["calls"], 0, run)
        self.assertEqual(run["interventions"]["moved"], 1, run)
        self.assertEqual(run["team_tools"], ["quadrat"], run)
        if run.get("end_reason") == "finish":
            self.assertIn("to_goal_s", run)
        self.assertEqual((scored[0]["successes"], scored[0]["rank"]), (1, 1), scored)

    def test_two_tasks_and_a_switch(self):
        """A team works in two tasks and switches between them: prompt, notes
        and runs always belong to the active task. Starting before accepting a
        task is refused."""
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Wechsler"},
                            opener=urllib.request.build_opener(), expect=(201,))
        a = self.open_task(team, "Aufgabe A")
        b = self.open_task(team, "Aufgabe B")
        refused = self.team_request(team, "POST", "/start", expect=(409,))
        self.assertIn("Aufgabe", refused["error"])

        for task, text in ((a, "Prompt A"), (b, "Prompt B")):
            self.team_request(team, "POST", "/tasks/%s/accept" % task["id"], {"prompt": "empty"})
            self.team_request(team, "POST", "/prompt", {"text": text}, expect=(201,))
            self.team_request(team, "POST", "/notes", {"text": "Notiz " + text[-1]}, expect=(201,))

        self.team_request(team, "POST", "/tasks/%s/activate" % a["id"])
        s = self.team_request(team, "GET", "/api/session")
        self.assertEqual((s["task"]["id"], s["prompt"]["text"], [n["text"] for n in s["notes"]]), (a["id"], "Prompt A", ["Notiz A"]))
        self.follow_team_log(team["token"])
        status = self.team_request(team, "POST", "/start", expect=(202,))
        start = self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "run_start"), "run_start in task A")[0]
        self.assertEqual(start.get("task_id"), a["id"], start)
        self.assertEqual(status.get("task_id"), a["id"], status)
        self.team_request(team, "POST", "/tasks/%s/activate" % b["id"], expect=(409,))  # not during a run
        self.team_request(team, "POST", "/stop")
        self.wait_for(lambda: not self.team_request(team, "GET", "/api/session")["agent"]["running"], "the run to stop")

        self.team_request(team, "POST", "/tasks/%s/activate" % b["id"])
        s = self.team_request(team, "GET", "/api/session")
        self.assertEqual((s["task"]["id"], s["prompt"]["text"], [n["text"] for n in s["notes"]]), (b["id"], "Prompt B", ["Notiz B"]))
        print("e2e: run in %s, switched to %s" % (a["title"], b["title"]), file=sys.stderr)

    def test_two_robots_move_at_once(self):
        """The scripted model moves every robot of the team in one answer; the
        server runs those calls at the same time."""
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        for robot in (ROBOT, ROBOT2):
            self.robot_online(robot)
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Duo"},
                            opener=urllib.request.build_opener(), expect=(201,))
        token = team["token"]
        self.give_task(team, "Zu zweit")
        for robot in (ROBOT, ROBOT2):
            self.request("POST", "/admin/api/assign", {"robot": robot, "team": team["id"]}, expect=(204,))
        self.follow_team_log(token)
        self.wait_for(lambda: len([r for r in self.request("GET", "/t/%s/api/session" % token)["robots"] if r["online"]]) == 2,
                      "both robots assigned to the team")

        t = "/t/%s" % token
        self.request("POST", t + "/prompt", {"text": "Erkundet die Arena, beide gleichzeitig."}, expect=(201,))
        self.request("POST", t + "/start", expect=(202,))

        def overlap():
            # A tool entry is logged when its call ends; data.ms is how long it ran.
            spans = []
            for e in self.log_entries(lambda e: e.get("kind") == "tool"):
                d = e.get("data") or {}
                res = d.get("result") or {}
                if d.get("name") in MOTION_TOOLS and res.get("ok") and d.get("ms", 0) > 200:
                    end = e["e2e_received"]
                    spans.append((res.get("robot"), end - d["ms"] / 1000.0, end, d["name"]))
            for a in spans:
                for b in spans:
                    # Sequential calls would touch at most; demand a clear overlap.
                    if a[0] != b[0] and min(a[2], b[2]) - max(a[1], b[1]) > 0.2:
                        return (a, b)
            return None

        a, b = self.wait_for(overlap, "motion calls of two robots that overlap in time")
        print("e2e: %s %s and %s %s moved at the same time" % (a[0], a[3], b[0], b[3]), file=sys.stderr)
        for robot in (ROBOT, ROBOT2):
            poses = self.poses(robot)
            self.assertTrue(poses, "no pose trace for %s" % robot)
            start = poses[0]
            self.assertTrue(any(abs(x - start[0]) > 2 or abs(y - start[1]) > 2 or abs(h - start[2]) > 5 for x, y, h in poses),
                            "%s did not move\n%s" % (robot, self.logs()))


if __name__ == "__main__":
    unittest.main()

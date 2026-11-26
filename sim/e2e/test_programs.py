"""End-to-end: team tools on a simulated robot in a labyrinth.

The labyrinth example ("turn right at every wall") runs in
sim/worlds/labyrinth_rechts.json, where turning right at every wall leads
to the blue field. Run with `npx nx run sim:e2e`.
"""

import json
import sys
import time
import unittest
import urllib.request

from test_smoke import ROOT, LabCase

LABYRINTH = ROOT / "sim" / "worlds" / "labyrinth_rechts.json"
ROBOT = "mbot-1"


class LabyrinthTest(LabCase):
    WORLD = LABYRINTH
    ROBOTS = 1

    def test_labyrinth_tool_reaches_the_blue_field(self):
        self.deadline = time.monotonic() + 120  # the drive itself takes about 40 s
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        self.robot_online(ROBOT)
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Labyrinth"},
                            opener=urllib.request.build_opener(), expect=(201,))
        self.give_task(team, "Labyrinth")
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(team["token"])
        self.wait_for(lambda: any(r["online"] for r in self.team_request(team, "GET", "/api/session")["robots"]), "robot assigned")
        time.sleep(1.5)  # the robot learns its owner from the next heartbeat
        result = self.run_team_tool(team, "labyrinth", "labyrinth")
        x, y, heading = self.poses()[-1]
        print("e2e: labyrinth end %s at x=%.1f y=%.1f heading=%d after %d steps, %.1f s" % (
            result["end"], x, y, heading, result["steps"], result["runtime_s"]), file=sys.stderr)
        self.assertEqual(result["end"], "done", result)
        self.assertEqual(result.get("value"), "blue", result)
        self.assertTrue(150 <= x <= 200 and 0 <= y <= 50, (x, y))

    def test_agent_calls_the_labyrinth_tool(self):
        """Stage 4: the team deploys labyrinth with its prompt; the agent calls it as a tool."""
        self.deadline = time.monotonic() + 120
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        self.robot_online(ROBOT)
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": "Agentur"},
                            opener=urllib.request.build_opener(), expect=(201,))
        self.give_task(team, "Labyrinth mit Agent")
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(team["token"])
        self.wait_for(lambda: any(r["online"] for r in self.team_request(team, "GET", "/api/session")["robots"]), "robot assigned")
        time.sleep(1.5)
        ex = json.loads((ROOT / "docs" / "examples" / "programs" / "labyrinth.json").read_text(encoding="utf-8"))
        self.team_request(team, "POST", "/programs", {"name": "labyrinth", "description": ex["program"]["description"],
                                                      "blockly": ex["blockly"]}, expect=(201,))
        deployed = self.team_request(team, "POST", "/prompt", {"text": "Nutze `labyrinth`, um zum blauen Feld zu kommen."}, expect=(201,))
        self.assertEqual([(t["name"], t["agent"]) for t in deployed["tools"]], [("labyrinth", True)])
        self.assertFalse(deployed.get("warning"), deployed)
        self.team_request(team, "POST", "/start", expect=(202,))
        end = self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "run_end"), "the end of the run")[0]
        data = end["data"]
        x, y, heading = self.poses()[-1]
        print("e2e: agent run ended %s: %s; robot at x=%.1f y=%.1f" % (data["reason"], data.get("report"), x, y), file=sys.stderr)
        for line in self.server_log.read_text(encoding="utf-8").splitlines():
            if "team tool" in line or "run started" in line or "run ended" in line:
                print("e2e: server: " + line, file=sys.stderr)
        self.assertEqual(data["reason"], "finish", data)
        self.assertIn("fertig", data.get("report", ""))  # the scripted model reports the end in German
        self.assertTrue(150 <= x <= 200 and 0 <= y <= 50, (x, y))
        steps = self.log_entries(lambda e: e.get("kind") == "tool" and (e.get("data") or {}).get("program") == "labyrinth")
        self.assertGreaterEqual(len(steps), 3, steps)


if __name__ == "__main__":
    unittest.main()

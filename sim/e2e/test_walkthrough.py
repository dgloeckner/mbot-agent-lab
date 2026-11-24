"""The challenge walk-through in the simulator, with a network delay.

- The rigid team tool `labyrinth` ("turn right at every wall") reaches the
  blue field in the simple labyrinth and does not in the disturbed one.
- An agent (the scripted model) with the small cut of tools (`ein_feld`,
  `rechts`, `links`) and `read_sensors` follows the right-hand wall cell by
  cell and reaches the goal in both.

The small cut needs many tool calls in one wake-up; this test raises
max_steps_per_wakeup for it (see the PR and docs/lab for the default).
"""

import json
import sys
import time
import unittest
import urllib.request

from test_smoke import ROOT, LabCase

DELAY_MS = 150
ROBOT = "mbot-1"
SMALL_CUT = ("ein_feld", "rechts", "links")


class Walkthrough(LabCase):
    ROBOTS = 1
    DELAY_MS = DELAY_MS
    CONFIG = ((r"max_steps_per_wakeup: \d+", "max_steps_per_wakeup: 200"),)
    GOAL = None  # (x0, x1, y0, y1) of the blue field

    def team_with_robot(self, name):
        self.deadline = time.monotonic() + 240
        self.request("POST", "/admin/api/login", {"password": self.admin_password}, expect=(204,))
        self.robot_online(ROBOT)
        team = self.request("POST", "/api/join", {"code": self.lab_code, "name": name},
                            opener=urllib.request.build_opener(), expect=(201,))
        self.give_task(team, "Labyrinth")
        self.request("POST", "/admin/api/assign", {"robot": ROBOT, "team": team["id"]}, expect=(204,))
        self.follow_team_log(team["token"])
        self.wait_for(lambda: any(r["online"] for r in self.team_request(team, "GET", "/api/session")["robots"]), "robot assigned")
        time.sleep(1.5)  # the robot learns its owner from the next heartbeat
        return team

    def save(self, team, example):
        ex = json.loads((ROOT / "docs" / "examples" / "programs" / (example + ".json")).read_text(encoding="utf-8"))
        saved = self.team_request(team, "POST", "/programs", {"name": ex["program"]["name"], "description": ex["program"]["description"],
                                                               "blockly": ex["blockly"]}, expect=(201,))
        self.assertTrue(saved["valid"], saved)
        return saved

    def at_goal(self):
        x, y, _ = self.poses()[-1]
        x0, x1, y0, y1 = self.GOAL
        return x0 <= x <= x1 and y0 <= y <= y1, (x, y)

    def agent_with_small_cut(self):
        team = self.team_with_robot("Kleiner Schnitt")
        for name in SMALL_CUT:
            self.save(team, name)
        dep = self.team_request(team, "POST", "/prompt", {"text": "Folge der rechten Wand Feld für Feld: `rechts`, `read_sensors`, "
                                                                     "`ein_feld` oder `links`, bis der Boden blau ist."}, expect=(201,))
        self.assertEqual(sorted(t["name"] for t in dep["tools"] if t["agent"]), sorted(SMALL_CUT))
        start = time.monotonic()
        self.team_request(team, "POST", "/start", expect=(202,))
        end = self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "run_end"), "the end of the agent run")[0]["data"]
        calls = self.request("GET", "/admin/api/state")
        tools = self.log_entries(lambda e: e.get("kind") == "tool" and e.get("source") == "agent" and not (e.get("data") or {}).get("program"))
        ok, pos = self.at_goal()
        print("e2e: %s agent (small cut): %s after %d tool calls, %.0f s; robot at x=%.1f y=%.1f; %s" % (
            self.WORLD.stem, end["reason"], len(tools), time.monotonic() - start, pos[0], pos[1], end.get("report")), file=sys.stderr)
        self.assertEqual(end["reason"], "finish", end)
        self.assertTrue(ok, pos)

    def rigid_tool(self):
        team = self.team_with_robot("Starr")
        saved = self.save(team, "labyrinth")
        run = self.team_request(team, "POST", "/programs/%s/run" % saved["id"], {}, expect=(202,))["run"]
        end = self.wait_for(lambda: self.log_entries(lambda e: e.get("kind") == "program_end" and (e.get("data") or {}).get("run") == run),
                            "the end of the labyrinth tool")[0]["data"]["result"]
        ok, pos = self.at_goal()
        print("e2e: %s rigid tool: end %s after %d steps, %.1f s, %s; robot at x=%.1f y=%.1f" % (
            self.WORLD.stem, end["end"], end["steps"], end["runtime_s"], end.get("error", ""), pos[0], pos[1]), file=sys.stderr)
        return end, ok


class SimpleLabyrinth(Walkthrough):
    WORLD = ROOT / "sim" / "worlds" / "labyrinth_rechts.json"
    GOAL = (150, 200, 0, 50)

    def test_rigid_tool_arrives(self):
        end, ok = self.rigid_tool()
        self.assertEqual(end["end"], "done", end)
        self.assertTrue(ok)

    def test_agent_with_small_tools_arrives(self):
        self.agent_with_small_cut()


class DisturbedLabyrinth(Walkthrough):
    WORLD = ROOT / "sim" / "worlds" / "labyrinth_gestoert.json"
    GOAL = (100, 150, 0, 50)

    def test_rigid_tool_does_not_arrive(self):
        end, ok = self.rigid_tool()
        self.assertNotEqual(end["end"], "done", end)
        self.assertFalse(ok)

    def test_agent_with_small_tools_arrives(self):
        self.agent_with_small_cut()


if __name__ == "__main__":
    unittest.main()

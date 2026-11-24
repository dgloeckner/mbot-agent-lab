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


if __name__ == "__main__":
    unittest.main()

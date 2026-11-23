"""retrace_path: the robot drives its trip log back, under realistic noise."""

import math
import unittest

from helpers import SkillRun, simstate


def box(w, h):
    return [[0, 0, w, 0], [w, 0, w, h], [w, h, 0, h], [0, h, 0, 0]]


def pose():
    w = simstate.world
    w.update()
    return w.x, w.y, w.heading


def heading_diff(a, b):
    return abs((a - b + 180) % 360 - 180)


# Back at the start within 6 cm and facing the way it came within 10 degrees
# after about 120 cm and a right angle with noisy sensors.
POS_TOLERANCE_CM = 6
HEADING_TOLERANCE_DEG = 10


class RetraceTest(unittest.TestCase):
    def setUp(self):
        self.run_ = SkillRun({"walls": box(300, 300), "start": {"x": 100, "y": 50, "heading": 0}}, seed=5)

    def trip(self):
        r = self.run_
        for name, args in (("drive", {"cm": 60}), ("turn", {"deg": 90}), ("drive", {"cm": 50})):
            self.assertEqual(r.run(name, args)["reason"], "done")
        self.assertEqual([k for k, _ in r.robot.trip_log], ["drive", "turn", "drive"])

    def test_retrace_returns_to_the_start(self):
        start = pose()
        self.trip()
        res = self.run_.run("retrace_path", {})
        self.assertTrue(res["ok"], res)
        self.assertEqual(res["reason"], "done", res)
        self.assertEqual(res["data"]["segments"], 3, res)
        x, y, heading = pose()
        self.assertLess(math.hypot(x - start[0], y - start[1]), POS_TOLERANCE_CM, (x, y))
        self.assertLess(heading_diff(heading, start[2] + 180), HEADING_TOLERANCE_DEG, heading)
        self.assertEqual(self.run_.robot.trip_log, [], "the retrace itself must not be logged")

    def test_empty_log(self):
        res = self.run_.run("retrace_path", {})
        self.assertEqual((res["ok"], res["reason"], res["data"]["segments"]), (True, "empty", 0), res)
        self.assertEqual(self.run_.robot.trip_log, [])

    def test_a_later_move_is_logged_from_scratch(self):
        self.trip()
        self.run_.run("retrace_path", {})
        self.run_.run("drive", {"cm": 20})
        self.assertEqual(len(self.run_.robot.trip_log), 1)
        self.assertAlmostEqual(self.run_.robot.trip_log[0][1], 20, delta=2)

    def test_obstacle_aborts_and_keeps_the_rest(self):
        start = pose()
        self.trip()
        # Someone puts a box across the first leg of the way back, 25 cm from the start.
        simstate.world.walls.append((60, 75, 140, 75))
        r = self.run_
        res = r.run("retrace_path", {})
        self.assertFalse(res["ok"], res)
        self.assertEqual(res["reason"], "obstacle", res)
        log = r.robot.trip_log
        # Left: the first leg's rest, and the turn the robot has on top of it.
        self.assertEqual([k for k, _ in log], ["drive", "turn"], log)
        x, y, _ = pose()
        self.assertLess(y, 100, "it got onto the first leg")
        self.assertAlmostEqual(log[0][1], y - start[1], delta=4)
        self.assertGreater(abs(log[1][1]), 170, log)

        # The box is gone: a new retrace_path continues home.
        simstate.world.walls.pop()
        r.settle()  # the distance median still remembers the box
        res = r.run("retrace_path", {})
        self.assertEqual(res["reason"], "done", res)
        x, y, _ = pose()
        self.assertLess(math.hypot(x - start[0], y - start[1]), POS_TOLERANCE_CM, (x, y))
        self.assertEqual(r.robot.trip_log, [])

    def test_stop_aborts_and_keeps_the_rest(self):
        self.trip()
        r = self.run_
        r.hb()
        r.client.deliver("lab/mbot-1/cmd", {"id": "rt", "name": "retrace_path", "args": {}})
        for _ in range(60):  # into the turn
            r.settle(1)
        r.client.deliver("lab/mbot-1/cmd", {"id": "st", "name": "stop", "args": {}})
        r.settle(2)
        res = [m for m in r.client.messages("result") if m["id"] == "rt"][0]
        self.assertEqual(res["reason"], "stopped", res)
        log = r.robot.trip_log
        self.assertEqual([k for k, _ in log][:3], ["drive", "turn", "drive"], log)
        self.assertLessEqual(len(log), 4, log)


if __name__ == "__main__":
    unittest.main()

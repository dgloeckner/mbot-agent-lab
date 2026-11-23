"""drive_arc: curves and circles under realistic noise.

The sim world's heading 0 points along +y and positive headings turn right,
so a right curve from heading 0 bends toward +x.
"""

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


# A full circle of radius 20 cm (about 126 cm of way) ends within 5 cm of its
# start and within 8 degrees of its start heading, with noisy sensors. The
# robot stops up to SK_TURN_TOLERANCE_DEG early, like turn.
POS_TOLERANCE_CM = 5
HEADING_TOLERANCE_DEG = 8


class ArcTest(unittest.TestCase):
    def setUp(self):
        self.run_ = SkillRun({"walls": box(300, 300), "start": {"x": 150, "y": 100, "heading": 0}}, seed=11)

    def assertPose(self, want_x, want_y, want_heading):
        x, y, heading = pose()
        self.assertLess(math.hypot(x - want_x, y - want_y), POS_TOLERANCE_CM, (x, y, heading))
        self.assertLess(heading_diff(heading, want_heading), HEADING_TOLERANCE_DEG, (x, y, heading))

    def test_full_circle_ends_at_the_start(self):
        res = self.run_.run("drive_arc", {"radius_cm": 20, "deg": 360, "speed": 60})
        self.assertEqual((res["ok"], res["reason"]), (True, "done"), res)
        self.assertAlmostEqual(res["data"]["turned_deg"], 360, delta=6)
        self.assertAlmostEqual(res["data"]["driven_cm"], 2 * math.pi * 20, delta=5)
        self.assertPose(150, 100, 0)

    def test_full_circle_left_ends_at_the_start(self):
        res = self.run_.run("drive_arc", {"radius_cm": 20, "deg": -360, "speed": 60})
        self.assertEqual((res["ok"], res["reason"]), (True, "done"), res)
        self.assertAlmostEqual(res["data"]["turned_deg"], -360, delta=6)
        self.assertPose(150, 100, 0)

    def test_half_circle_right_lands_one_diameter_to_the_right(self):
        res = self.run_.run("drive_arc", {"radius_cm": 20, "deg": 180, "speed": 60})
        self.assertEqual(res["reason"], "done", res)
        self.assertPose(190, 100, 180)

    def test_quarter_left_with_a_large_radius(self):
        res = self.run_.run("drive_arc", {"radius_cm": 40, "deg": -90})
        self.assertEqual(res["reason"], "done", res)
        self.assertPose(110, 140, 270)

    def test_trip_log_holds_the_chord_and_retrace_returns(self):
        r = self.run_
        r.run("drive_arc", {"radius_cm": 30, "deg": 90, "speed": 60})
        log = r.robot.trip_log
        self.assertEqual([k for k, _ in log], ["turn", "drive", "turn"], log)
        self.assertAlmostEqual(log[0][1], 45, delta=4)
        self.assertAlmostEqual(log[1][1], 2 * 30 * math.sin(math.radians(45)), delta=3)
        res = r.run("retrace_path", {})
        self.assertEqual(res["reason"], "done", res)
        # Back at the start, facing back along the chord it came on (45 degrees).
        self.assertPose(150, 100, 225)

    def test_floor_trace_counts_cm_along_the_way(self):
        spec = {"walls": box(300, 300), "fields": [{"rect": [160, 90, 60, 60], "color": "blue"}],
                "start": {"x": 150, "y": 100, "heading": 0}}
        res = SkillRun(spec, seed=4).run("drive_arc", {"radius_cm": 20, "deg": 360, "speed": 60})
        floor = res["data"]["floor"]
        self.assertTrue(any("b" in p for _, p in floor), floor)
        self.assertTrue(all(0 <= pos <= res["data"]["driven_cm"] + 1 for pos, _ in floor), floor)
        self.assertGreater(floor[-1][0], 40, floor)  # cm: degrees would run up to 360

    def test_stripes_keep_their_width_on_a_wide_arc(self):
        # Three 3 cm stripes across the way of a radius 150 arc: 3 cm are only
        # about 1 degree there, so a trace in degrees lost them as edge transitions.
        stripes = [{"rect": [140, 110 + 6 * i, 20, 3], "color": c} for i, c in enumerate(("red", "green", "blue"))]
        spec = {"walls": box(300, 300), "fields": stripes, "start": {"x": 150, "y": 100, "heading": 0}}
        res = SkillRun(spec, seed=8).run("drive_arc", {"radius_cm": 150, "deg": 15})
        floor = res["data"]["floor"]
        widths = {p[1]: nxt[0] - pos for (pos, p), nxt in zip(floor, floor[1:]) if p[1] in "rgb"}
        self.assertEqual(sorted(widths), ["b", "g", "r"], floor)
        for w in widths.values():
            self.assertAlmostEqual(w, 3, delta=1, msg=floor)

    def test_stops_at_a_color_blob(self):
        # A blue blob a quarter circle along the way (radius 30 to the right).
        spec = {"walls": box(300, 300), "fields": [{"rect": [172, 122, 16, 16], "color": "blue"}],
                "start": {"x": 150, "y": 100, "heading": 0}}
        res = SkillRun(spec, seed=6).run("drive_arc", {"radius_cm": 30, "deg": 360, "stop_on_color": "blue"})
        self.assertEqual((res["ok"], res["reason"]), (True, "color"), res)
        self.assertTrue(40 < res["data"]["turned_deg"] < 110, res)
        self.assertIn("b", res["data"]["floor"][-1][1], res)

    def test_circle_from_the_start_field_back_to_its_color(self):
        # The robot stands in a red field; red counts only once it has left it.
        spec = {"walls": box(300, 300), "fields": [{"rect": [135, 85, 30, 30], "color": "red"}],
                "start": {"x": 150, "y": 100, "heading": 0}}
        res = SkillRun(spec, seed=3).run("drive_arc", {"radius_cm": 40, "deg": 360, "stop_on_color": "red"})
        self.assertEqual((res["ok"], res["reason"]), (True, "color"), res)
        self.assertTrue(270 < res["data"]["turned_deg"] < 360, res)
        x, y, _ = pose()
        self.assertLess(math.hypot(x - 150, y - 100), 30, (x, y))

    def test_unknown_stop_color_is_an_error(self):
        res = self.run_.run("drive_arc", {"radius_cm": 30, "deg": 90, "stop_on_color": "pink"})
        self.assertEqual((res["ok"], res["reason"]), (False, "error"), res)

    def test_stops_before_a_wall(self):
        run = SkillRun({"walls": box(300, 300) + [[100, 140, 300, 140]],
                        "start": {"x": 150, "y": 100, "heading": 0}}, seed=2)
        res = run.run("drive_arc", {"radius_cm": 80, "deg": 90})
        self.assertEqual((res["ok"], res["reason"]), (False, "obstacle"), res)
        self.assertGreater(res["data"]["min_distance_cm"], 0)

    def test_needs_a_radius(self):
        res = self.run_.run("drive_arc", {"deg": 90})
        self.assertEqual((res["ok"], res["reason"]), (False, "error"), res)


if __name__ == "__main__":
    unittest.main()

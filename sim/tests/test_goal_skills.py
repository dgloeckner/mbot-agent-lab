"""Filtered sensors and goal-oriented skills, tested under realistic noise.

The seeds are fixed so a failure can be reproduced. Each test would fail if
the robot decided on single raw readings: a ghost echo ends drive_until early,
a gyro spike aborts a drive, color jitter at edges fakes extra colors.
"""

import unittest

from helpers import SkillRun, load_robot


def box(w, h):
    return [[0, 0, w, 0], [w, 0, w, h], [w, h, 0, h], [0, h, 0, 0]]


class FilterTest(unittest.TestCase):
    def setUp(self):
        self.ns = load_robot({"walls": box(100, 100), "start": {"x": 50, "y": 50, "heading": 0}})

    def test_distance_median_ignores_dropouts_and_ghosts(self):
        sense = self.ns["Sense"]()
        readings = [80, 81, 0, 79, 4, 80, 0, 82, 81]  # 0 = dropout, 4 = ghost echo
        out = [sense.distance(r) for r in readings]
        self.assertTrue(all(v >= 79 for v in out), out)

    def test_floor_pattern_needs_three_equal_readings(self):
        steady = self.ns["Steady"](3)
        steady.update("wkkw", 0)
        flicker = ["rkkr", "wkkw", "rkkr", "rkkr", "wkkw", "?kkr"]  # an edge: never 3 in a row
        self.assertFalse(any(steady.update(p, i) for i, p in enumerate(flicker, 1)))
        self.assertEqual(steady.value, "wkkw")
        changed = [steady.update("rkkr", pos) for pos in (7, 8, 9)]
        self.assertEqual(changed, [False, False, True])
        self.assertEqual((steady.value, steady.tag), ("rkkr", 7), "the change begins at the first reading")

    def test_trace_drops_edge_transitions(self):
        trace = self.ns["Trace"]({"floor": "wkkw", "distance_cm": 300})
        # One outer probe reaches each stripe edge 2 mm before the other.
        for pos, pattern in [(2.0, "gkkw"), (2.2, "gkkg"), (3.9, "gkkw"), (4.1, "wkkw"),
                             (6.0, "wkkr"), (6.2, "wkkw"), (9.0, "rkkr")]:
            trace.add(pos, pattern)
        self.assertEqual(trace.entries, [[0, "wkkw"], [2.2, "gkkg"], [4.1, "wkkw"], [9.0, "rkkr"]])

    def test_tilt_hysteresis_and_duration(self):
        sense = self.ns["Sense"]()
        self.assertFalse(sense.tilt(60, 0))  # one spike is not a tip-over
        self.assertFalse(sense.tilt(0, 0))
        for _ in range(4):
            tilted = sense.tilt(50, 0)
        self.assertTrue(tilted)
        for _ in range(4):
            tilted = sense.tilt(40, 0)  # below 45 but above 30: still tilted
        self.assertTrue(tilted)
        for _ in range(4):
            tilted = sense.tilt(10, 0)
        self.assertFalse(tilted)


class NoisySkillTest(unittest.TestCase):
    def test_drive_until_wall_is_not_fooled_by_ghost_echoes(self):
        run = SkillRun({"walls": box(200, 200), "start": {"x": 100, "y": 50, "heading": 0}})
        r = run.run("drive_until", {"max_cm": 200, "stop_below_cm": 20, "speed": 80})
        self.assertEqual(r["reason"], "wall", r)
        self.assertGreater(r["data"]["driven_cm"], 100, r)
        self.assertLess(r["data"]["distance_cm"], 25, r)

    def test_gyro_spikes_do_not_abort_a_drive(self):
        run = SkillRun({"walls": box(200, 200), "start": {"x": 100, "y": 30, "heading": 0}}, seed=7)
        r = run.run("drive", {"cm": 60})
        self.assertEqual(r["reason"], "done", r)

    def test_drive_reports_the_floor_trace(self):
        spec = {
            "walls": box(200, 200),
            "fields": [{"rect": [0, 80, 200, 12], "color": "red"}],
            "lines": [{"points": [[0, 60], [200, 60]], "width": 2}],
            "start": {"x": 100, "y": 30, "heading": 0},
        }
        r = SkillRun(spec, seed=3).run("drive", {"cm": 90, "speed": 40})
        d = r["data"]
        self.assertEqual(r["reason"], "done", r)
        patterns = [p for _, p in d["floor"]]
        self.assertEqual(patterns, ["wwww", "kkkk", "wwww", "rrrr", "wwww"], d["floor"])
        # The sensor bar sits 5 cm ahead of the axle: line at 60 - 1 - 30 - 5 = 24 cm.
        self.assertAlmostEqual(d["floor"][1][0], 24, delta=1)
        self.assertAlmostEqual(d["floor"][3][0], 45, delta=1)
        self.assertIsNotNone(d["min_distance_cm"])
        self.assertNotIn("floor_truncated", d)

    def test_turn_until_clear_finds_the_open_side(self):
        # Facing a wall 13 cm away in the top of a box; the open space is to the right.
        run = SkillRun({"walls": box(100, 100), "start": {"x": 50, "y": 80, "heading": 0}})
        r = run.run("turn_until_clear", {"direction": "right", "min_cm": 40})
        self.assertEqual(r["reason"], "clear", r)
        self.assertGreaterEqual(r["data"]["distance_cm"], 40, r)
        self.assertTrue(30 <= r["data"]["turned_deg"] <= 120, r)

    def test_turn_until_clear_gives_up_after_max_deg(self):
        run = SkillRun({"walls": box(30, 30), "start": {"x": 15, "y": 15, "heading": 0}})
        r = run.run("turn_until_clear", {"direction": "left", "min_cm": 80, "max_deg": 120})
        self.assertEqual(r["reason"], "max_deg", r)
        self.assertLessEqual(r["data"]["turned_deg"], -115, r)

    def test_scan_faces_the_most_open_direction(self):
        # A wall close on the left and ahead, open space far to the right.
        walls = [[30, 0, 30, 200], [30, 200, 250, 200], [250, 200, 250, 0], [250, 0, 30, 0]]
        run = SkillRun({"walls": walls, "start": {"x": 50, "y": 150, "heading": 0}})
        r = run.run("scan", {"from_deg": -90, "to_deg": 90, "step_deg": 15})
        d = r["data"]
        self.assertEqual(r["reason"], "done", r)
        self.assertEqual(len(d["profile"]), 13, d)
        self.assertGreaterEqual(d["best_deg"], 60, d)
        self.assertGreater(d["best_cm"], 100, d)
        self.assertLess(abs(d["turned_deg"] - d["best_deg"]), 10, d)
        left = [cm for deg, cm in d["profile"] if deg <= -75 and cm is not None]
        self.assertTrue(left and max(left) < 40, d["profile"])

    def test_follow_line_until_color(self):
        spec = {
            "walls": box(200, 200),
            "lines": [{"points": [[40, 25], [40, 90], [70, 130], [110, 140]], "width": 2}],
            "fields": [{"rect": [100, 128, 30, 25], "color": "blue"}],
            "start": {"x": 40, "y": 22, "heading": 0},
        }
        r = SkillRun(spec, seed=5).run("follow_line_until", {"max_cm": 250, "stop_on_color": "blue", "speed": 40})
        self.assertEqual(r["reason"], "color", r)
        self.assertTrue(100 < r["data"]["driven_cm"] < 170, r)

    def test_drive_until_color_ignores_the_color_it_starts_on(self):
        spec = {
            "walls": box(200, 200),
            "fields": [{"rect": [20, 10, 40, 30], "color": "blue"}, {"rect": [20, 90, 40, 20], "color": "blue"}],
            "start": {"x": 40, "y": 20, "heading": 0},
        }
        r = SkillRun(spec, seed=7).run("drive_until", {"max_cm": 150, "stop_on_color": "blue"})
        self.assertEqual(r["reason"], "color", r)
        self.assertTrue(55 < r["data"]["driven_cm"] < 75, r)

    def test_follow_line_reports_line_end(self):
        spec = {
            "walls": box(200, 200),
            "lines": [{"points": [[40, 25], [40, 80]], "width": 2}],
            "start": {"x": 40, "y": 22, "heading": 0},
        }
        r = SkillRun(spec, seed=6).run("follow_line_until", {"max_cm": 200})
        self.assertEqual(r["reason"], "line_lost", r)
        self.assertTrue(45 < r["data"]["driven_cm"] < 75, r)

    def test_unfinished_skill_says_so(self):
        r = SkillRun({"walls": box(100, 100), "start": {"x": 50, "y": 50, "heading": 0}}, seed=None).run("gesture", {"kind": "nod"})
        self.assertEqual(r["reason"], "error")


if __name__ == "__main__":
    unittest.main()

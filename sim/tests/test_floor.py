"""The line field: band world, four probes, color calibration and the floor trace.

Field geometry and the color code appear only here, in sim/grid.py and in the
world file; the robot script just reports letters. These tests check that the
reports carry enough to read the field (handoff G1 to G3).
"""

import json
import time
import unittest

from helpers import SIM, SkillRun, World  # sets up the import path
import grid  # noqa: E402
import simstate  # noqa: E402

WORLD = json.loads((SIM / "worlds" / "gitter.json").read_text())
LETTER = {"white": "w", "black": "k", "red": "r", "yellow": "y", "green": "g", "blue": "b"}
HEADING = {(1, 0): 90, (-1, 0): 270, (0, 1): 0, (0, -1): 180}


def spec(start):
    out = dict(WORLD, start=start)
    out.pop("starts", None)
    return out


def expected_segment(a, b):
    """Floor trace from junction a to junction b, axle starting on a.

    With the sensor bar 5 cm ahead of the axle: stripe at d cm from a begins
    under the sensor at d - 0.95 - 5 cm.
    """
    (ax, ay), (bx, by) = a, b
    stripes = [(8, grid.CODE[ax]), (12, grid.CODE[ay]), (18, grid.CODE[by]), (22, grid.CODE[bx])]
    out = [[0, "wkkw"]]
    for d, color in stripes:
        c = LETTER[color]
        out += [[d - 0.95 - 5, c + "kk" + c], [d + 0.95 - 5, "wkkw"]]
    # The crossing line at b reaches the outer probes only where it goes on:
    # at the frame, the side outside the field stays white.
    dx, dy = bx - ax, by - ay
    inside = lambda x, y: 0 <= x < grid.NODES and 0 <= y < grid.NODES
    left = "k" if inside(bx - dy, by + dx) else "w"
    right = "k" if inside(bx + dy, by - dx) else "w"
    ahead = "wkkw" if inside(bx + dx, by + dy) else "wwww"  # past a corner the line ends
    out += [[30 - 0.95 - 5, left + "kk" + right], [30 + 0.95 - 5, ahead]]
    return out


class FieldTest(unittest.TestCase):
    def test_generated_files_are_current(self):
        grid.main(["--check"])

    def test_probes_see_the_drawing(self):
        clock = lambda: 0.0
        w = World(spec({"x": 45, "y": 75, "heading": 90}), clock=clock)  # junction (1,2), facing east
        names = lambda: "".join(LETTER.get(w.probe_name(p), "?") for p in ("L2", "L1", "R1", "R2"))
        self.assertEqual(names(), "wkkw")          # sensor 5 cm ahead, on the line
        w.x = 45 + 8 - 5                           # sensor over the x stripe of (1,2): yellow
        self.assertEqual(names(), "ykky", "the line lies over the stripe")
        w.x = 45 + 12 - 5                          # y stripe: green
        self.assertEqual(names(), "gkkg")
        w.x = 75 - 5                               # the crossing line at junction (2,2)
        self.assertEqual(names(), "kkkk")
        self.assertEqual(w.probe_rgb(3), w.probe_rgb("L1"), "probes by number: 4 = L2 .. 1 = R2")

    def test_spot_mixes_at_edges(self):
        w = World(spec({"x": 45, "y": 75, "heading": 90}), clock=lambda: 0.0)
        w.x = 45 + 8 - 0.95 - 5  # stripe edge right under the sensor bar
        r, g, b = w.probe_rgb("L2")  # half yellow, half white
        self.assertTrue(60 < b < 220, (r, g, b))

    def test_line_sta_bits_mean_background(self):
        SkillRun(spec({"x": 45, "y": 75, "heading": 90}), seed=None)
        import mbuild
        self.assertEqual(mbuild.quad_rgb_sensor.get_line_sta(1), 0b1001)  # L2 and R2 on white


class CalibrationTest(unittest.TestCase):
    def calibrated(self, seed):
        run = SkillRun(spec(WORLD["reference_start"]), seed=seed)
        r = run.run("calibrate_colors", {})
        self.assertTrue(r["ok"], r)
        self.assertEqual(sorted(r["data"]["refs"]), ["L1", "L2", "R1", "R2"])
        self.assertEqual(len(r["data"]["refs"]["L2"]), 6)
        self.assertLess(len(json.dumps(r, separators=(",", ":"))), 512)
        return run

    def test_classification_with_color_cast_and_noise(self):
        # Seeds give each robot a different color cast; the references absorb it.
        for seed in (11, 12, 13):
            run = self.calibrated(seed)
            sense = run.robot.sense
            wrong, unclear, total = 0, 0, 0
            for k, color in enumerate(grid.REF_COLORS):
                x = grid.REF_X_CM + k * grid.REF_FIELD_CM + 2.5 - 5
                run.place(x, grid.REF_Y_CM, 90)
                for _ in range(100):
                    for letter in sense.raw_pattern():
                        total += 1
                        if letter == "?":
                            unclear += 1
                        elif letter != LETTER[color]:
                            wrong += 1
            self.assertEqual(wrong, 0, "seed %d" % seed)
            self.assertLess(unclear / total, 0.01, "seed %d" % seed)

    def test_calibration_fails_without_strip(self):
        run = SkillRun({"floor": "white", "start": {"x": 0, "y": 0, "heading": 0}}, seed=None)
        r = run.run("calibrate_colors", {})
        self.assertFalse(r["ok"], r)


class TraceTest(unittest.TestCase):
    def drive_segment(self, run, a, b):
        x, y = grid.node_xy(*a)
        run.place(x, y, HEADING[(b[0] - a[0], b[1] - a[1])])
        return run.run("drive", {"cm": 30, "speed": 40})

    def assertTrace(self, floor, want, delta=1.2):
        # Positions are good to about a centimetre: a pattern counts from its
        # first steady reading, and with noise the probes reach an edge a few
        # readings apart.
        self.assertEqual([p for _, p in floor], [p for _, p in want], floor)
        for (got, _), (exp, _) in zip(floor, want):
            self.assertAlmostEqual(got, exp, delta=delta, msg=floor)

    def test_segment_trace_matches_the_reading_rules(self):
        # The handoff's example: from (0,2) to (1,2) the stripes read red, green, green, yellow.
        run = SkillRun(spec({"x": 15, "y": 75, "heading": 90}), seed=None)
        r = self.drive_segment(run, (0, 2), (1, 2))
        self.assertEqual(r["reason"], "done", r)
        self.assertTrace(r["data"]["floor"], expected_segment((0, 2), (1, 2)))
        self.assertEqual([p for _, p in r["data"]["floor"]][1::2][:4], ["rkkr", "gkkg", "gkkg", "ykky"])

    def test_noisy_calibrated_traces_on_rows_and_columns(self):
        segments = [((0, 0), (1, 0)), ((1, 0), (2, 0)), ((3, 3), (2, 3)), ((2, 1), (2, 2)), ((0, 3), (0, 2)), ((3, 1), (3, 0))]
        for seed in (21, 22):
            run = SkillRun(spec(WORLD["reference_start"]), seed=seed)
            self.assertTrue(run.run("calibrate_colors", {})["ok"])
            for a, b in segments:
                r = self.drive_segment(run, a, b)
                self.assertEqual(r["reason"], "done", r)
                self.assertTrace(r["data"]["floor"], expected_segment(a, b))

    def test_uncalibrated_robot_uses_built_in_names(self):
        run = SkillRun(spec({"x": 15, "y": 15, "heading": 0}), seed=4)
        r = self.drive_segment(run, (2, 1), (2, 2))
        self.assertTrace(r["data"]["floor"], expected_segment((2, 1), (2, 2)))

    def test_long_drive_truncates_the_trace(self):
        run = SkillRun(spec({"x": 15, "y": 45, "heading": 90}), seed=None)
        r = run.run("drive", {"cm": 85, "speed": 60})
        d = r["data"]
        self.assertEqual(len(d["floor"]), 20)
        self.assertTrue(d["floor_truncated"])
        self.assertEqual(r["reason"], "done", "the motion goes on after the trace is full")
        self.assertLess(len(json.dumps(r, separators=(",", ":"))), 512)

    def test_stop_keeps_the_trace(self):
        run = SkillRun(spec({"x": 15, "y": 45, "heading": 90}), seed=None)
        run.n += 1
        run.hb()
        run.client.deliver("lab/mbot-1/cmd", {"id": "go", "name": "drive", "args": {"cm": 60}})
        for _ in range(200):
            run.robot.iterate()
            if simstate.world.x > 15 + 12:
                break
            time.sleep(0.01)
        r = run.run("stop", {})
        self.assertTrue(r["ok"])
        aborted = [m for m in run.client.messages("result") if m["id"] == "go"][0]
        self.assertEqual(aborted["reason"], "stopped")
        self.assertGreaterEqual(len(aborted["data"]["floor"]), 3, aborted)

    def test_turn_trace_counts_degrees(self):
        run = SkillRun(spec({"x": 45, "y": 45, "heading": 0}), seed=None)  # axle on junction (1,1)
        r = run.run("turn", {"deg": 90})
        floor = r["data"]["floor"]
        patterns = [p for _, p in floor]
        self.assertEqual(floor[0], [0, "wkkw"])
        self.assertIn("wwww", patterns, "between the arms the bar is off the lines")
        self.assertEqual(patterns[-1], "wkkw", "facing along the east arm")
        self.assertTrue(all(0 <= pos <= 92 for pos, _ in floor), floor)

    def test_state_reports_the_pattern(self):
        run = SkillRun(spec({"x": 45, "y": 75, "heading": 90}), seed=None)
        deadline = time.monotonic() + 2
        while not run.client.messages("state") and time.monotonic() < deadline:
            run.settle(1)
        run.settle(10)
        state = run.client.messages("state")[-1]
        self.assertEqual(state["floor"], "wkkw")
        self.assertEqual(state["line"], "center")


if __name__ == "__main__":
    unittest.main()

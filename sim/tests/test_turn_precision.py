"""Turns and drives land on target although the motors run on after a stop.

The simulated wheels follow a speed command with a lag (motor_lag_s in
sim/mbot2.json, varied per robot with noise), like the real ones: a turn that
stops only when it reaches the target overshoots by 10 degrees and more. The
skills measure with the wheel encoders, slow down before the target and stop
early by the expected run-on (SK_RUNON_S, which the noise makes up to 30 %
wrong). On a stand the body does not rotate, and a turn
still ends by encoder count.
"""

import math
import unittest

from helpers import SkillRun, simstate, World

# turn(90) and turn(180) must land this close, measured in the world.
TURN_TOLERANCE_DEG = 3
DRIVE_TOLERANCE_CM = 0.5
# What the robot reports may differ from what the world did by this much.
REPORT_TOLERANCE_DEG = 1.5

OPEN = {"walls": [[-200, -200, 200, -200], [200, -200, 200, 200], [200, 200, -200, 200], [-200, 200, -200, -200]],
        "start": {"x": 0, "y": 0, "heading": 0}}


def heading():
    w = simstate.world
    w.update()
    return w.heading


def turned_since(before):
    """Signed rotation in -180..180 since the heading before."""
    return (heading() - before + 180) % 360 - 180


class WorldMotorTest(unittest.TestCase):
    def test_wheels_run_on_after_a_stop(self):
        clock = [0.0]
        w = World(OPEN, clock=lambda: clock[0])
        w.set_motors(30, -30)
        clock[0] = 1.0
        w.set_motors(0, 0)
        before = w.wheel_deg()[0]
        clock[0] = 2.0
        run_on = w.wheel_deg()[0] - before
        # 30 RPM is 180 wheel degrees per second; the lag lets it run on ~18 degrees.
        self.assertAlmostEqual(run_on, 180 * w.motor_lag, delta=3)
        clock[0] = 3.0
        self.assertEqual(w.wheel_deg()[0], before + run_on, "a stopped wheel stands still in the end")

    def test_on_a_stand_the_wheels_turn_but_not_the_body(self):
        clock = [0.0]
        w = World(dict(OPEN, stand=True), clock=lambda: clock[0])
        w.set_motors(30, -30)
        clock[0] = 1.0
        self.assertGreater(w.wheel_deg()[0], 100)
        self.assertEqual((w.x, w.y, w.heading), (0, 0, 0))


class TurnPrecisionTest(unittest.TestCase):
    def check_turn(self, run, deg):
        before = heading()
        res = run.run("turn", {"deg": deg})
        self.assertEqual((res["ok"], res["reason"]), (True, "done"), res)
        err = (heading() - before - deg + 180) % 360 - 180
        self.assertLess(abs(err), TURN_TOLERANCE_DEG, "turn(%s) is off by %.1f" % (deg, err))
        self.assertLess(abs(res["data"]["turned_deg"] - (deg + err)), REPORT_TOLERANCE_DEG, (res, err))

    def test_turns_land_on_target_with_noise(self):
        for seed in (1, 2, 3):
            run = SkillRun(OPEN, seed=seed)
            for deg in (90, 180, -90, -180, 45):
                with self.subTest(seed=seed, deg=deg):
                    self.check_turn(run, deg)

    def test_drive_lands_on_target_and_reports_the_run_on(self):
        run = SkillRun(OPEN, seed=4)
        for cm, speed in ((20, 40), (30, 80), (-15, 40)):
            w = simstate.world
            w.update()
            x, y = w.x, w.y
            res = run.run("drive", {"cm": cm, "speed": speed})
            w.update()
            actual = math.hypot(w.x - x, w.y - y)
            self.assertLess(abs(actual - abs(cm)), DRIVE_TOLERANCE_CM, (cm, speed, actual))
            self.assertLess(abs(res["data"]["driven_cm"] - actual), 0.3, (res, actual))

    def test_four_right_angles_face_the_start(self):
        run = SkillRun(OPEN, seed=6)
        before = heading()
        for _ in range(4):
            run.run("turn", {"deg": 90})
        # A run-on that differs from SK_RUNON_S leaves the same small error in every turn.
        self.assertLess(abs(turned_since(before)), 8)

    def test_turn_until_clear_stops_at_max_deg(self):
        run = SkillRun({"walls": [[-30, 15, 30, 15]], "start": {"x": 0, "y": 0, "heading": 0}}, seed=7)
        # A wall all around the front: never clear within 60 degrees.
        before = heading()
        res = run.run("turn_until_clear", {"direction": "left", "min_cm": 250, "max_deg": 60})
        self.assertEqual(res["reason"], "max_deg", res)
        self.assertLess(abs(turned_since(before) + 60), TURN_TOLERANCE_DEG)

    def test_scan_faces_the_start_again(self):
        run = SkillRun(OPEN, seed=8)
        before = heading()
        res = run.run("scan", {"from_deg": -60, "to_deg": 60, "face": "start"})
        self.assertEqual(res["reason"], "done", res)
        self.assertLess(abs(turned_since(before)), TURN_TOLERANCE_DEG)


class StandTest(unittest.TestCase):
    """On a stand the gyro sees no rotation; the turn ends by encoder count."""

    def test_turn_on_a_stand_ends_by_encoders(self):
        run = SkillRun(dict(OPEN, stand=True), seed=9)
        for deg in (90, 180):
            res = run.run("turn", {"deg": deg}, timeout=8)
            self.assertEqual((res["ok"], res["reason"]), (True, "done"), res)
            self.assertLess(abs(res["data"]["turned_deg"] - deg), TURN_TOLERANCE_DEG, res)
        self.assertEqual(heading(), 0, "the body stays put")


if __name__ == "__main__":
    unittest.main()

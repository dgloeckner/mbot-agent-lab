"""A motion that is physically held up ends with "blocked", under realistic noise.

A low obstacle lies below the ultrasonic cone: the sensor reads open space
right up to it. The robot only feels it, in one of two ways (sim/world.py,
"low_obstacles"): the wheels stand ("stall"), or they slip while the nose
lifts ("nose_up_deg"). Wheels that slip without lifting the nose stay
unnoticed: nothing on the robot can tell.

Normal motions never report "blocked": every speed, speeding up, slowing down
before the target, turning back and forth in a scan, steering along a line.
"""

import json
import time
import unittest

from helpers import SIM, SkillRun, load_robot, simstate


def box(w, h):
    return [[0, 0, w, 0], [w, 0, w, h], [w, h, 0, h], [0, h, 0, 0]]


def world(**low):
    """A 300 x 300 cm box, the robot at (150, 80) facing a low obstacle along y = 100.

    The body (radius 8 cm) touches it after 12 cm. Without arguments the box is empty.
    """
    spec = {"walls": box(300, 300), "start": {"x": 150, "y": 80, "heading": 0}}
    if low:
        spec["low_obstacles"] = [dict({"from": [100, 100], "to": [200, 100]}, **low)]
    return spec


def load(name):
    return json.loads((SIM / "worlds" / (name + ".json")).read_text())


# From the first push against the obstacle to the result, including the wait
# for the wheels to stand: "well under a second".
MAX_BLOCKED_S = 0.9
SEEDS = (1, 2, 3)


class BlockedTest(unittest.TestCase):
    def assertBlocked(self, run, name, args):
        simstate.world.low_hit_at = None
        res = run.run(name, args)
        took = time.monotonic() - (simstate.world.low_hit_at or 0)
        self.assertEqual((res["ok"], res["reason"]), (False, "blocked"), res)
        self.assertLess(took, MAX_BLOCKED_S, res)
        return res

    def test_stalled_wheels_end_a_drive(self):
        for seed in SEEDS:
            for speed in (15, 40, 80):
                with self.subTest(seed=seed, speed=speed):
                    run = SkillRun(world(wheels="stall"), seed=seed)
                    res = self.assertBlocked(run, "drive", {"cm": 100, "speed": speed})
                    self.assertAlmostEqual(res["data"]["driven_cm"], 12, delta=2)
                    self.assertGreater(res["data"]["min_distance_cm"], 150, "the sensor never saw it")

    def test_lifted_nose_ends_a_drive_with_slipping_wheels(self):
        for seed in SEEDS:
            for speed in (15, 40, 80):
                with self.subTest(seed=seed, speed=speed):
                    run = SkillRun(world(wheels="slip", nose_up_deg=12), seed=seed)
                    self.assertBlocked(run, "drive", {"cm": 100, "speed": speed})

    def test_every_forward_skill_is_covered(self):
        for low in ({"wheels": "stall"}, {"wheels": "slip", "nose_up_deg": 12}):
            for name, args in (("drive_until", {"max_cm": 100, "stop_below_cm": 10}),
                               ("drive_arc", {"radius_cm": 100, "deg": 60})):
                with self.subTest(name=name, **low):
                    self.assertBlocked(SkillRun(world(**low), seed=4), name, args)

    def test_line_following_is_covered(self):
        for low in ({"wheels": "stall"}, {"wheels": "slip", "nose_up_deg": 12}):
            with self.subTest(**low):
                spec = world(**low)
                spec["lines"] = [{"points": [[150, 20], [150, 280]], "width": 2}]
                res = self.assertBlocked(SkillRun(spec, seed=5), "follow_line_until", {"max_cm": 100})
                if low["wheels"] == "stall":
                    self.assertAlmostEqual(res["data"]["driven_cm"], 12, delta=2)

    def test_backing_into_it_ends_with_stalled_wheels(self):
        # Backing up is blind for the ultrasonic sensor, not for the wheels.
        run = SkillRun(world(wheels="stall"), seed=6)
        run.place(150, 120, 0)
        res = self.assertBlocked(run, "drive", {"cm": -100})
        self.assertAlmostEqual(res["data"]["driven_cm"], 12, delta=2)

    def test_robot_frees_itself_afterwards(self):
        # What the tool description tells the agent: back up, turn away, drive on.
        for low in ({"wheels": "stall"}, {"wheels": "slip", "nose_up_deg": 12}):
            with self.subTest(**low):
                run = SkillRun(world(**low), seed=7)
                self.assertBlocked(run, "drive", {"cm": 100})
                self.assertEqual(run.run("drive", {"cm": -10})["reason"], "done")
                self.assertEqual(run.run("turn", {"deg": 90})["reason"], "done")
                self.assertEqual(run.run("drive", {"cm": 30})["reason"], "done")

    def test_retrace_keeps_the_rest_of_the_way(self):
        run = SkillRun(world(), seed=8)
        self.assertEqual(run.run("drive", {"cm": 60})["reason"], "done")
        # Someone puts a threshold across the way back.
        simstate.world.low = [((100, 100, 200, 100), True, 0.0)]
        res = run.run("retrace_path", {})
        self.assertEqual((res["ok"], res["reason"]), (False, "blocked"), res)
        self.assertEqual([kind for kind, _ in run.robot.trip_log], ["drive", "turn"], run.robot.trip_log)
        self.assertAlmostEqual(res["data"]["driven_cm"], 32, delta=3)
        self.assertAlmostEqual(run.robot.trip_log[0][1] + res["data"]["driven_cm"], 60, delta=3)

    def test_blocked_reaches_the_event_stream(self):
        run = SkillRun(world(wheels="stall"), seed=9)
        self.assertBlocked(run, "drive", {"cm": 100})
        aborted = [e for e in run.client.messages("event") if e["kind"] == "skill_aborted"]
        self.assertEqual(aborted[-1]["data"], {"skill": "drive", "reason": "blocked"})


class NotBlockedTest(unittest.TestCase):
    """Free motions under noise: any reason but "blocked"."""

    def check(self, run, name, args, want=None):
        res = run.run(name, args)
        self.assertNotEqual(res["reason"], "blocked", (name, args, res))
        if want:
            self.assertEqual(res["reason"], want, (name, args, res))
        return res

    def test_drives_at_every_speed(self):
        for seed in SEEDS[:2]:
            run = SkillRun(world(), seed=seed)
            for cm, speed in ((10, 5), (3, 40), (40, 40), (-60, 80), (80, 80), (-20, 20), (-2, 40)):
                with self.subTest(seed=seed, cm=cm, speed=speed):
                    self.check(run, "drive", {"cm": cm, "speed": speed}, "done")

    def test_turns_scans_and_arcs(self):
        for seed in SEEDS[:2]:
            run = SkillRun({"walls": box(300, 300), "start": {"x": 150, "y": 150, "heading": 0}}, seed=seed)
            for name, args, want in (
                    ("turn", {"deg": 90}, "done"), ("turn", {"deg": -180}, "done"), ("turn", {"deg": 3}, "done"),
                    ("scan", {"face": "start"}, "done"),
                    ("turn_until_clear", {"direction": "left", "min_cm": 50}, "clear"),
                    ("drive_arc", {"radius_cm": 10, "deg": 45, "speed": 5}, "done"),
                    ("drive_arc", {"radius_cm": 20, "deg": -180, "speed": 80}, "done"),
                    ("drive_arc", {"radius_cm": 200, "deg": 10}, "done")):
                with self.subTest(seed=seed, name=name, **args):
                    self.check(run, name, args, want)

    def test_walls_the_sensor_sees_stay_obstacles(self):
        run = SkillRun(world(), seed=3)
        self.assertEqual(run.run("drive", {"cm": 300, "speed": 80})["reason"], "obstacle")
        self.assertEqual(run.run("drive_until", {"max_cm": 100, "stop_below_cm": 30})["reason"], "wall")

    def test_line_following_on_the_existing_worlds(self):
        arena = load("arena")
        arena["start"] = {"x": 45, "y": 100, "heading": 90}
        self.check(SkillRun(arena, seed=2), "follow_line_until", {"max_cm": 60, "speed": 60}, "max_cm")
        for name in ("linie", "gitter"):
            with self.subTest(world=name):
                spec = load(name)
                spec["start"] = spec["starts"][0]
                run = SkillRun(spec, seed=3)
                for cm, speed in ((12, 10), (30, 30), (40, 60)):
                    self.check(run, "follow_line_until", {"max_cm": cm, "speed": speed, "stop_at_crossing": False})

    def test_labyrinth_walk(self):
        # Drive to the wall, turn to the open side: what an agent does in the labyrinths.
        spec = load("labyrinth_gestoert")
        spec["start"] = spec["starts"][0]
        run = SkillRun(spec, seed=1)
        for _ in range(3):
            self.check(run, "drive_until", {"max_cm": 150, "stop_below_cm": 15, "speed": 60})
            self.check(run, "turn_until_clear", {"direction": "right", "min_cm": 30})

    def test_robot_on_a_stand_is_not_stalled(self):
        run = SkillRun({"stand": True, "start": {"x": 0, "y": 0, "heading": 0}}, seed=2)
        self.check(run, "drive", {"cm": 30}, "done")
        self.check(run, "turn", {"deg": 90}, "done")


class FilterTest(unittest.TestCase):
    """The filters in sense.py, fed by hand."""

    def setUp(self):
        self.ns = load_robot(world(), noise=None)
        self.sense = self.ns["Sense"]()
        self.now = 1000
        self.pos = [0.0, 0.0]

    def nose(self, pitches, moving=True):
        return [self.sense.nose(p, moving) for p in pitches][-1]

    def test_nose_needs_duration_and_has_hysteresis(self):
        n = self.ns["SN_NOSE_STABLE"]
        self.assertFalse(self.nose([0] * 5, moving=False))
        self.assertFalse(self.nose([55, 0, 55, 0]), "bumps are single readings")
        self.assertFalse(self.nose([12] * (n - 1) + [0]), "shorter than the minimum duration")
        self.assertTrue(self.nose([12] * n))
        self.assertTrue(self.nose([6] * n), "between off and on: still up")
        self.assertFalse(self.nose([2] * n))

    def test_nose_counts_against_the_pitch_at_rest(self):
        n = self.ns["SN_NOSE_STABLE"]
        # A CyberPi mounted at a slant, or a sloped floor: 15 degrees is level here.
        self.assertFalse(self.nose([15] * 3 * n, moving=False))
        self.assertFalse(self.nose([15] * 3 * n))
        self.assertTrue(self.nose([27] * n))
        # At rest the state is off and the level follows: carried around, put down.
        self.assertFalse(self.nose([27], moving=False))
        self.nose([-20] * 400, moving=False)
        self.nose([0] * 400, moving=False)
        self.assertFalse(self.nose([1] * 3 * n))

    def test_nose_ignores_a_dipping_nose(self):
        self.assertFalse(self.nose([-30] * 40))

    def stall(self, cmd, steps):
        """Feed (left, right) wheel degrees per 25 ms step; the motors are told cmd."""
        hw = self.ns
        hw["hw_cmd"][0], hw["hw_cmd"][1] = cmd
        hw["hw_wheels_deg"] = lambda: tuple(self.pos)
        out = []
        for left, right in steps:
            self.now += 25
            self.pos[0] += left
            self.pos[1] += right
            out.append(self.sense.stall(self.now))
        return out

    def test_stall_needs_the_whole_window(self):
        full = 40 * 6 * 0.025  # degrees per step at 40 RPM
        self.assertFalse(any(self.stall((40, 40), [(full, full)] * 40)))
        # Speeding up: nothing for 300 ms, then the wheels turn.
        self.assertFalse(any(self.stall((40, 40), [(0, 0)] * 12 + [(full, full)] * 20)))
        # Held: flagged after SN_STALL_MS, not before.
        held = self.stall((40, 40), [(0, 0)] * 20)
        self.assertEqual(held.index(True), self.ns["SN_STALL_MS"] // 25 - 1, held)
        # One wheel held is enough, a crawling wheel is held too.
        self.stall((0, 0), [(0, 0)])
        self.assertTrue(self.stall((40, 40), [(full, 0)] * 20)[-1])
        self.stall((0, 0), [(0, 0)])
        self.assertTrue(self.stall((40, 40), [(full / 10, full / 10)] * 20)[-1])

    def test_stall_starts_anew_after_every_stop(self):
        self.assertFalse(any(self.stall((40, 40), [(0, 0)] * 14)))
        # A stop and a new start between two looks, as when commands follow at once.
        self.ns["hw_motors_stop"]()
        self.assertFalse(any(self.stall((40, 40), [(0, 0)] * 14)))

    def test_stall_does_not_judge_a_creeping_wheel(self):
        slow = self.ns["SN_STALL_MIN_RPM"] - 1
        self.assertFalse(any(self.stall((40, slow), [(6, 0)] * 40)))


if __name__ == "__main__":
    unittest.main()

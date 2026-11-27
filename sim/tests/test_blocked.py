"""A motion that is physically held up ends with "blocked", under realistic noise.

What the ultrasonic sensor does not see, the robot only feels: a low obstacle
below the sound cone, a wall behind it. The wheels do not stand then, they slip
at full speed, as measured on the real robot; what shows is the power the
motors need (sim/world.py, motor_power), and the pitch when the robot climbs
("nose_up_deg").

Normal motions never report "blocked": every speed, speeding up, slowing down
before the target, turning back and forth in a scan, steering along a line,
with motors that need a little more or less than the measured ones.
"""

import json
import time
import unittest

from helpers import SIM, SkillRun, load_robot, simstate
import world as world_mod


def box(w, h):
    return [[0, 0, w, 0], [w, 0, w, h], [w, h, 0, h], [0, h, 0, 0]]


def world(low=None):
    """A 300 x 300 cm box, the robot at (150, 80) facing a low obstacle along y = 100.

    The body (radius 8 cm) touches it after 12 cm. low holds the obstacle's
    properties ({} for none); without it the box is empty.
    """
    spec = {"walls": box(300, 300), "start": {"x": 150, "y": 80, "heading": 0}}
    if low is not None:
        spec["low_obstacles"] = [dict({"from": [100, 100], "to": [200, 100]}, **low)]
    return spec


def load(name):
    return json.loads((SIM / "worlds" / (name + ".json")).read_text())


# From the first push to the result: the motors need too much for 0.4 s, then the
# wheels come to rest. With room for a busy machine.
MAX_BLOCKED_S = 1.0
SEEDS = (1, 2, 3)
PUSH = {}                    # pushes against it: the motors need more
CLIMB = {"nose_up_deg": 20}  # and climbs it: 21 degrees and more on the real robot


def slip_cm(speed):
    """Way the slipping wheels count at most until the motion ends."""
    return speed * 6 * world_mod.CM_PER_WHEEL_DEG * MAX_BLOCKED_S


class BlockedTest(unittest.TestCase):
    def assertBlocked(self, run, name, args):
        simstate.world.held_at = None
        res = run.run(name, args)
        took = time.monotonic() - (simstate.world.held_at or 0)
        self.assertEqual((res["ok"], res["reason"]), (False, "blocked"), res)
        self.assertLess(took, MAX_BLOCKED_S, res)
        self.assertIn(res["data"]["blocked_by"], ("motor_power", "nose_up"), res)
        return res

    def no_extra_power(self):
        """Only the nose tells: pushing costs the motors nothing extra."""
        old = world_mod.POWER_HELD, world_mod.POWER_HELD_PER_RPM
        world_mod.POWER_HELD = world_mod.POWER_HELD_PER_RPM = 0

        def restore():
            world_mod.POWER_HELD, world_mod.POWER_HELD_PER_RPM = old
        self.addCleanup(restore)

    def test_pushing_against_a_low_obstacle_ends_a_drive(self):
        for seed in SEEDS:
            for speed in (10, 40, 80):
                with self.subTest(seed=seed, speed=speed):
                    run = SkillRun(world(PUSH), seed=seed)
                    res = self.assertBlocked(run, "drive", {"cm": 100, "speed": speed})
                    self.assertEqual(res["data"]["blocked_by"], "motor_power")
                    # The wheels slip until the motion ends, and driven_cm counts that.
                    self.assertGreater(res["data"]["driven_cm"], 10)
                    self.assertLess(res["data"]["driven_cm"], 14 + slip_cm(speed))
                    self.assertGreater(res["data"]["min_distance_cm"], 150, "the sensor never saw it")

    def test_backing_into_a_wall_ends_a_drive(self):
        # Backing up is blind for the ultrasonic sensor, not for the motors.
        for seed in SEEDS:
            for speed in (10, 40, 80):
                with self.subTest(seed=seed, speed=speed):
                    run = SkillRun(world(), seed=seed)
                    run.place(150, 20, 0)
                    self.assertBlocked(run, "drive", {"cm": -100, "speed": speed})
                    self.assertAlmostEqual(simstate.world.y, 8, delta=0.5)

    def test_weak_and_strong_motors_feel_it_too(self):
        for gain in (0.9, 1.1):
            for speed in (10, 80):
                with self.subTest(gain=gain, speed=speed):
                    run = SkillRun(world(PUSH), seed=4)
                    simstate.world.power_gain = gain
                    self.assertBlocked(run, "drive", {"cm": 100, "speed": speed})

    def test_climbing_ends_a_drive_by_the_nose_alone(self):
        self.no_extra_power()
        for seed in SEEDS:
            for speed in (15, 40, 80):
                with self.subTest(seed=seed, speed=speed):
                    run = SkillRun(world(CLIMB), seed=seed)
                    res = self.assertBlocked(run, "drive", {"cm": 100, "speed": speed})
                    self.assertEqual(res["data"]["blocked_by"], "nose_up")

    def test_every_forward_skill_is_covered(self):
        for low in (PUSH, CLIMB):
            for name, args in (("drive_until", {"max_cm": 100, "stop_below_cm": 10}),
                               ("drive_arc", {"radius_cm": 100, "deg": 60})):
                with self.subTest(name=name, **low):
                    self.assertBlocked(SkillRun(world(low), seed=4), name, args)

    def test_line_following_is_covered(self):
        for low in (PUSH, CLIMB):
            with self.subTest(**low):
                spec = world(low)
                spec["lines"] = [{"points": [[150, 20], [150, 280]], "width": 2}]
                self.assertBlocked(SkillRun(spec, seed=5), "follow_line_until", {"max_cm": 100})

    def test_backing_into_a_low_obstacle(self):
        run = SkillRun(world(PUSH), seed=6)
        run.place(150, 120, 0)
        self.assertBlocked(run, "drive", {"cm": -100})

    def test_robot_frees_itself_afterwards(self):
        # What the tool description tells the agent: back up, turn away, drive on.
        for low in (PUSH, CLIMB):
            with self.subTest(**low):
                run = SkillRun(world(low), seed=7)
                self.assertBlocked(run, "drive", {"cm": 100})
                self.assertEqual(run.run("drive", {"cm": -10})["reason"], "done")
                self.assertEqual(run.run("turn", {"deg": 90})["reason"], "done")
                self.assertEqual(run.run("drive", {"cm": 30})["reason"], "done")

    def test_retrace_keeps_the_rest_of_the_way(self):
        run = SkillRun(world(), seed=8)
        self.assertEqual(run.run("drive", {"cm": 60})["reason"], "done")
        # Someone puts a threshold across the way back.
        simstate.world.low = [((100, 100, 200, 100), 0.0)]
        res = run.run("retrace_path", {})
        self.assertEqual((res["ok"], res["reason"]), (False, "blocked"), res)
        self.assertEqual([kind for kind, _ in run.robot.trip_log], ["drive", "turn"], run.robot.trip_log)
        # 32 cm to the threshold, plus what the slipping wheels counted.
        self.assertGreater(res["data"]["driven_cm"], 30)
        self.assertLess(res["data"]["driven_cm"], 34 + slip_cm(40))
        self.assertAlmostEqual(run.robot.trip_log[0][1] + res["data"]["driven_cm"], 60, delta=3)

    def test_blocked_reaches_the_event_stream(self):
        run = SkillRun(world(PUSH), seed=9)
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

    def test_weak_and_strong_motors(self):
        for gain in (0.9, 1.1):
            run = SkillRun(world(), seed=5)
            simstate.world.power_gain = gain
            for name, args in (("drive", {"cm": 60, "speed": 80}), ("drive", {"cm": -40, "speed": 80}),
                               ("drive", {"cm": 10, "speed": 5}), ("turn", {"deg": 180})):
                with self.subTest(gain=gain, name=name, **args):
                    self.check(run, name, args, "done")

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
        res = run.run("drive", {"cm": 300, "speed": 80})
        self.assertEqual(res["reason"], "obstacle")
        self.assertNotIn("blocked_by", res["data"])
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

    def test_robot_on_a_stand_is_not_held(self):
        run = SkillRun({"stand": True, "start": {"x": 0, "y": 0, "heading": 0}}, seed=2)
        self.check(run, "drive", {"cm": 30}, "done")
        self.check(run, "turn", {"deg": 90}, "done")


class FilterTest(unittest.TestCase):
    """The filters in sense.py, fed by hand, one reading every STEP_MS."""

    STEP_MS = 25

    def setUp(self):
        self.ns = load_robot(world(), noise=None)
        self.sense = self.ns["Sense"]()
        self.now = 1000
        self.power = [0.0, 0.0]
        self.ns["hw_power"] = lambda i: self.power[i]

    def steps(self, ms):
        return ms // self.STEP_MS

    def nose(self, pitches, moving=True):
        out = False
        for p in pitches:
            self.now += self.STEP_MS
            out = self.sense.nose(p, moving, self.now)
        return out

    def test_nose_needs_duration_and_has_hysteresis(self):
        n = self.steps(self.ns["SN_NOSE_MS"]) + 2
        self.assertFalse(self.nose([0] * 5, moving=False))
        self.assertFalse(self.nose([55, 0, 55, 0] * 10 + [0, 0]), "bumps are single readings")
        self.assertFalse(self.nose([12] * (n - 2) + [0, 0]), "shorter than the minimum duration")
        self.assertTrue(self.nose([12] * n))
        self.assertTrue(self.nose([12, -40] * n), "a bump does not end it")
        self.assertTrue(self.nose([6] * n), "between off and on: still up")
        self.assertFalse(self.nose([2] * n))

    def test_nose_counts_time_not_readings(self):
        # The real loop is four times slower than the simulated one: same duration.
        self.nose([0] * 3, moving=False)
        self.STEP_MS = 100
        self.assertFalse(self.nose([12] * 4))
        self.assertTrue(self.nose([12]))

    def test_nose_counts_against_the_pitch_at_rest(self):
        n = self.steps(self.ns["SN_NOSE_MS"]) + 2
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

    def free(self, rpm):
        return self.ns["SN_PWR_BASE"] + self.ns["SN_PWR_PER_RPM"] * abs(rpm)

    def held(self, cmd, extra, ms):
        """Motors told cmd need extra = (left, right) percent more than free driving, for ms."""
        hw = self.ns
        hw["hw_cmd"][0], hw["hw_cmd"][1] = cmd
        out = []
        for _ in range(self.steps(ms)):
            self.now += self.STEP_MS
            for i in (0, 1):
                self.power[i] = (self.free(cmd[i]) + extra[i]) * (1 if cmd[i] >= 0 else -1)
            out.append(self.sense.held(self.now))
        return out

    def stop(self):
        self.ns["hw_motors_stop"]()
        self.sense.held(self.now)

    def test_held_needs_the_whole_duration(self):
        ms = self.ns["SN_HELD_MS"]
        self.assertFalse(any(self.held((40, 40), (0, 0), 1000)))
        # Speeding up: more for 200 ms, then what the speed asks for.
        self.stop()
        self.assertFalse(any(self.held((80, 80), (11, 11), 200) + self.held((80, 80), (0, 0), 600)))
        # Held: flagged after SN_HELD_MS, not before.
        self.stop()
        held = self.held((40, 40), (10, 10), 1000)
        self.assertIn(held.index(True), range(self.steps(ms), self.steps(ms) + 3), held)
        # A break in between starts the time anew.
        self.stop()
        runs = self.held((40, 40), (10, 10), 300) + self.held((40, 40), (0, 0), 100) + self.held((40, 40), (10, 10), 300)
        self.assertFalse(any(runs))

    def test_held_at_every_speed_and_backwards(self):
        # The least the motors needed more at a wall on the real robot: 6 percent at 10 RPM.
        for cmd, extra in (((10, 10), (6, 6)), ((20, 20), (7, 7)), ((40, 40), (11, 11)), ((80, 80), (9, 9)),
                           ((-40, -40), (11, 11)), ((40, -40), (11, 11)), ((40, 40), (0, 22))):
            with self.subTest(cmd=cmd, extra=extra):
                self.stop()
                self.assertTrue(self.held(cmd, extra, 800)[-1])

    def test_held_ignores_what_free_driving_shows(self):
        # Largest deviations measured in free driving, a pushed light obstacle,
        # and a low battery that needs 15 % more for the same speed.
        for cmd, extra in (((40, 40), (2, 2)), ((-100, -100), (-1, 4)), ((40, -40), (2, 4.5)), ((8, 2), (1, 3.5)),
                           ((80, 80), (5.4, 5.4)), ((100, 100), (6.6, 6.6))):
            with self.subTest(cmd=cmd, extra=extra):
                self.stop()
                self.assertFalse(any(self.held(cmd, extra, 2000)))

    def test_held_starts_anew_after_every_stop(self):
        self.assertFalse(any(self.held((40, 40), (10, 10), 350)))
        # A stop and a new start between two looks, as when commands follow at once.
        self.ns["hw_motors_stop"]()
        self.assertFalse(any(self.held((40, 40), (10, 10), 350)))

    def test_held_reads_one_motor_per_look(self):
        calls = []
        self.ns["hw_power"] = lambda i: calls.append(i) or 20
        self.ns["hw_cmd"][0] = self.ns["hw_cmd"][1] = 40
        for _ in range(4):
            self.now += self.STEP_MS
            self.sense.held(self.now)
        self.assertEqual(calls, [1, 0, 1, 0])
        self.ns["hw_cmd"][0] = self.ns["hw_cmd"][1] = 0
        self.sense.held(self.now)
        self.assertEqual(len(calls), 4, "no reading while the motors are off")


if __name__ == "__main__":
    unittest.main()

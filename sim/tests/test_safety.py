"""Driving skills keep the robot off obstacles, under realistic noise.

The ultrasonic sensor looks forward in a cone (sim/mbot2.json). Forward
motions stop before what it sees, also a wall met at a slant down to 30
degrees. Backing up is blind; that is the agent's job (the tool says so).
retrace_path decides on its own, so it never drives backwards.
"""

import unittest

from helpers import SkillRun, simstate


def box(w, h):
    return [[0, 0, w, 0], [w, 0, w, h], [w, h, 0, h], [0, h, 0, 0]]


class SafetyTest(unittest.TestCase):
    def test_slanted_walls_are_not_hit(self):
        # Wall along y = 0; heading 90 + a meets it at angle a.
        for speed in (20, 80):
            for angle in (90, 45, 30):
                with self.subTest(speed=speed, angle=angle):
                    run = SkillRun({"walls": box(400, 300), "start": {"x": 60, "y": 60, "heading": 0}}, seed=1)
                    run.place(60, 60, 90 + angle)
                    simstate.world.bumped = False
                    res = run.run("drive", {"cm": 300, "speed": speed})
                    self.assertEqual(res["reason"], "obstacle", res)
                    self.assertFalse(simstate.world.bumped, res)

    def test_blind_sensor_counts_as_obstacle(self):
        run = SkillRun({"walls": box(300, 300), "start": {"x": 150, "y": 150, "heading": 0}}, seed=None)
        simstate.world.distance_cm = lambda: 0.0  # every echo fails
        run.settle(12)
        res = run.run("drive", {"cm": 50})
        self.assertEqual((res["ok"], res["reason"]), (False, "obstacle"), res)
        self.assertEqual(run.run("turn", {"deg": 90})["reason"], "done")

    def test_retrace_never_backs_up(self):
        run = SkillRun({"walls": box(300, 300), "start": {"x": 150, "y": 100, "heading": 0}}, seed=4)
        for name, args in (("drive", {"cm": 40}), ("turn", {"deg": 90}), ("drive", {"cm": -25})):
            self.assertEqual(run.run(name, args)["reason"], "done")
        world = simstate.world
        backwards = []
        set_motors = world.set_motors

        def record(left, right):
            if left < 0 and right < 0:
                backwards.append((left, right))
            set_motors(left, right)

        world.set_motors = record
        res = run.run("retrace_path", {})
        self.assertEqual((res["ok"], res["reason"], res["data"]["segments"]), (True, "done", 3), res)
        self.assertAlmostEqual(res["data"]["driven_cm"], 65, delta=4)
        self.assertEqual(backwards, [])
        world.update()
        self.assertLess(abs(world.x - 150) + abs(world.y - 100), 8, (world.x, world.y))


if __name__ == "__main__":
    unittest.main()

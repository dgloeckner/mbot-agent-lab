"""led_effect on the robot: frames computed in the loop, priorities, end."""

import time
import unittest

from helpers import load_robot, MemoryClient
import simstate  # noqa: E402  (helpers puts the fakes on sys.path)

ARENA = {
    "walls": [[0, 0, 300, 0], [300, 0, 300, 300], [300, 300, 0, 300], [0, 300, 0, 0]],
    "start": {"x": 150, "y": 150, "heading": 0},
}
OFF = (0, 0, 0)


class LedTest(unittest.TestCase):
    def setUp(self):
        self.ns = load_robot(ARENA)
        self.robot = self.ns["Robot"]()
        self.robot.iterate()
        self.client = MemoryClient.instances[-1]
        self.agent = "waiting"
        self.spin(0.3)

    def spin(self, seconds, sample=None):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Rot", "name": "Scout",
                                                  "color": "red", "agent": self.agent})
            self.robot.iterate()
            if sample is not None:
                sample.append(list(simstate.led_strip))
            time.sleep(0.01)

    def effect(self, **args):
        self.client.deliver("lab/mbot-1/cmd", {"id": "fx%d" % time.monotonic_ns(), "name": "led_effect", "args": args})

    def test_team_color_without_effect(self):
        self.assertEqual(simstate.led_strip, [(255, 0, 0)] * 5)

    def test_breathe_changes_brightness_of_all_five_together(self):
        self.effect(fx="breathe", c=[0, 0, 255], p=600)
        frames = []
        self.spin(0.7, frames)
        blues = {f[0][2] for f in frames[5:]}
        self.assertGreater(max(blues) - min(blues), 150)
        self.assertTrue(all(f.count(f[0]) == 5 for f in frames[5:]))
        self.assertTrue(all(f[0][0] == 0 for f in frames[5:]))

    def test_blink_left_and_level(self):
        self.effect(fx="blink_left", c=[255, 100, 0], p=400)
        frames = []
        self.spin(0.5, frames)
        lit = [f for f in frames if f[0] != OFF]
        dark = [f for f in frames[5:] if f[0] == OFF]
        self.assertTrue(lit and dark)
        self.assertTrue(all(f[1] == f[0] and f[2:] == [OFF] * 3 for f in lit))
        self.effect(fx="level", c=[0, 255, 0], v=60)
        self.spin(0.2)
        self.assertEqual(simstate.led_strip[:3], [(0, 255, 0)] * 3)
        self.assertEqual(simstate.led_strip[3:], [OFF] * 2)

    def test_effect_wins_over_thinking_and_ends_after_its_time(self):
        self.agent = "thinking"
        self.effect(fx="gradient", c=[0, 255, 0], c2=[255, 255, 255], ms=500)
        self.spin(0.3)
        self.assertEqual(simstate.led_strip[0], (0, 255, 0))
        self.assertEqual(simstate.led_strip[4], (255, 255, 255))
        self.agent = "waiting"
        self.spin(0.4)
        self.assertEqual(simstate.led_strip, [(255, 0, 0)] * 5)

    def test_set_leds_ends_an_effect_and_off_stays_dark(self):
        self.effect(fx="rainbow", p=1000)
        self.spin(0.2)
        self.assertGreater(len(set(simstate.led_strip)), 1)
        self.client.deliver("lab/mbot-1/cmd", {"id": "c1", "name": "set_leds", "args": {"color": "blue"}})
        self.spin(0.2)
        self.assertEqual(simstate.led_strip, [(0, 0, 255)] * 5)
        self.effect(fx="off")
        self.spin(0.3)
        self.assertEqual(simstate.led_strip, [OFF] * 5)

    def test_effect_runs_on_while_driving(self):
        self.effect(fx="chase", c=[0, 0, 255], p=500)
        self.client.deliver("lab/mbot-1/cmd", {"id": "d1", "name": "drive", "args": {"cm": 40}})
        frames = []
        self.spin(0.6, frames)
        self.assertIsNotNone(self.robot.skill)
        brightest = {max(range(5), key=lambda i: f[i][2]) for f in frames[10:]}
        self.assertGreaterEqual(len(brightest), 3)  # the bright point moves around

    def test_new_team_clears_the_effect(self):
        self.effect(fx="off")
        self.spin(0.2)
        self.client.deliver("lab/mbot-1/hb", {"team": "t2", "owner": "Blau", "name": "X", "color": "blue", "agent": "waiting"})
        self.robot.iterate()
        self.client.deliver("lab/mbot-1/hb", {"team": "t2", "owner": "Blau", "name": "X", "color": "blue", "agent": "waiting"})
        for _ in range(15):
            self.robot.iterate()
            time.sleep(0.01)
        self.assertEqual(simstate.led_strip, [(0, 0, 255)] * 5)


if __name__ == "__main__":
    unittest.main()

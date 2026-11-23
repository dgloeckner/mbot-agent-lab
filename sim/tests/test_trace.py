"""The sim trace line shows what a person would see and hear on the robot."""

import time
import unittest

from helpers import load_robot, MemoryClient
import robot_proc  # noqa: E402  (sim/robot_proc.py, path set up by helpers)
import simstate  # noqa: E402

ARENA = {
    "walls": [[0, 0, 200, 0], [200, 0, 200, 200], [200, 200, 0, 200], [0, 200, 0, 0]],
    "start": {"x": 100, "y": 50, "heading": 0},
}


class LedNameTest(unittest.TestCase):
    def test_names_ignore_brightness(self):
        self.assertEqual(robot_proc.led_name((0, 0, 0)), "off")
        self.assertEqual(robot_proc.led_name((25, 25, 25)), "white")
        self.assertEqual(robot_proc.led_name((76, 30, 0)), "orange")
        self.assertEqual(robot_proc.led_name((25, 0, 0)), "red")
        self.assertEqual(robot_proc.led_name((255, 200, 0)), "yellow")


class TraceLineTest(unittest.TestCase):
    def setUp(self):
        simstate.led_trail[:] = []
        simstate.sounds[:] = []
        ns = load_robot(ARENA)
        self.robot = ns["Robot"]()
        self.robot.iterate()
        self.client = MemoryClient.instances[-1]
        self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Team Rot", "name": "Scout",
                                              "color": "red", "agent": "waiting"})
        self.spin(0.3)
        robot_proc.trace_line(simstate.world)  # drain the start-up colors

    def spin(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.robot.iterate()
            time.sleep(0.01)

    def test_idle_shows_team_color(self):
        line = robot_proc.trace_line(simstate.world)
        self.assertIn(" leds=red ", line)
        self.assertNotIn("sound=", line)
        self.assertTrue(line.startswith("[mbot-1] x="), line)

    def test_identify_blinks_white_and_beeps(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "i1", "name": "identify", "args": {}})
        self.spin(1.2)
        line = robot_proc.trace_line(simstate.world)
        self.assertRegex(line, r" leds=[a-z/]*white/off")
        self.assertIn(" sound=beep", line)
        # Sounds and blinking are reported once, not again on the next line.
        self.assertNotIn("sound=", robot_proc.trace_line(simstate.world))


if __name__ == "__main__":
    unittest.main()

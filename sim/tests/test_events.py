"""Events from the newer sensors: buttons, joystick, tilt, shake, bump, lift,
clap and light. Run with sensor noise on: quiet readings must stay quiet."""

import random
import time
import unittest

from helpers import load_robot, MemoryClient
import simstate  # noqa: E402  (helpers puts the fakes on sys.path)

ARENA = {
    "walls": [[0, 0, 300, 0], [300, 0, 300, 300], [300, 300, 0, 300], [0, 300, 0, 0]],
    "start": {"x": 150, "y": 150, "heading": 0},
}


class EventTest(unittest.TestCase):
    def setUp(self):
        simstate.light, simstate.loudness, simstate.shake = 40, 2, False
        simstate.acc = {"x": 0.0, "y": 0.0, "z": 9.8}
        simstate.buttons = {"a": False, "b": False}
        self.ns = load_robot(ARENA, noise=random.Random(7))
        self.robot = self.ns["Robot"]()
        self.robot.iterate()
        self.client = MemoryClient.instances[-1]
        self.spin(0.5)

    def spin(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Rot", "name": "Scout",
                                                  "color": "red", "agent": "waiting"})
            self.robot.iterate()
            time.sleep(0.01)

    def kinds(self):
        return [e["kind"] for e in self.client.messages("event")]

    def events(self, kind):
        return [e for e in self.client.messages("event") if e["kind"] == kind]

    def test_quiet_sensors_stay_quiet(self):
        self.spin(2.0)
        newer = {"joystick", "lifted", "bumped", "tilted", "shake", "clap", "light_up", "light_down"}
        self.assertEqual([k for k in self.kinds() if k in newer], [])

    def test_state_carries_the_new_readings(self):
        state = self.client.messages("state")[-1]
        for key in ("light", "loudness", "pitch", "roll"):
            self.assertIn(key, state)
        self.assertAlmostEqual(state["light"], 40, delta=4)

    def test_light_jump_fires_once_each_way(self):
        simstate.light = 75
        self.spin(1.0)
        up = self.events("light_up")
        self.assertEqual(len(up), 1, self.kinds())
        self.assertAlmostEqual(up[0]["data"]["base"], 40, delta=4)
        # Back to the usual level after a short while: the mean has hardly
        # moved, so that is no jump.
        simstate.light = 40
        self.spin(0.6)
        self.assertEqual(self.events("light_down"), [])
        simstate.light = 10  # covered
        self.spin(0.6)
        down = self.events("light_down")
        self.assertEqual(len(down), 1, self.kinds())
        self.assertLess(down[0]["data"]["light"], 15)

    def test_clap_fires_once_and_needs_more_while_driving(self):
        simstate.loudness = 60
        self.spin(0.05)
        simstate.loudness = 2
        simstate.loudness = 60
        self.spin(0.1)
        simstate.loudness = 2
        self.spin(0.3)
        self.assertEqual(len(self.events("clap")), 1, self.kinds())
        self.spin(0.5)
        self.client.deliver("lab/mbot-1/cmd", {"id": "d1", "name": "drive", "args": {"cm": 100}})
        self.spin(0.2)
        simstate.loudness = 30  # loud enough standing, not with the motors running
        self.spin(0.2)
        simstate.loudness = 2
        self.spin(0.3)
        self.assertEqual(len(self.events("clap")), 1, self.kinds())

    def test_bump_and_lift(self):
        simstate.acc = {"x": 9.0, "y": 0.0, "z": 9.8}
        self.spin(0.1)
        simstate.acc = {"x": 0.0, "y": 0.0, "z": 9.8}
        self.spin(0.6)
        simstate.acc = {"x": 0.0, "y": 0.0, "z": 14.0}
        self.spin(0.1)
        simstate.acc = {"x": 0.0, "y": 0.0, "z": 9.8}
        self.spin(0.1)
        self.assertEqual(len(self.events("bumped")), 1, self.kinds())
        self.assertEqual(len(self.events("lifted")), 1, self.kinds())

    def test_shake_and_joystick(self):
        simstate.shake = True
        simstate.buttons["left"] = True
        self.spin(0.2)
        simstate.shake = False
        simstate.buttons["left"] = False
        self.spin(0.1)
        self.assertEqual(len(self.events("shake")), 1, self.kinds())
        self.assertEqual([e["data"]["dir"] for e in self.events("joystick")], ["left"])

    def test_button_presses_are_reported_at_most_every_half_second(self):
        for _ in range(3):
            simstate.buttons["a"] = True
            self.spin(0.05)
            simstate.buttons["a"] = False
            self.spin(0.05)
        self.assertEqual(len(self.events("button_a")), 1, self.kinds())

    def test_tilt_counts_after_half_a_second(self):
        simstate.world.pitch = 60
        self.spin(0.3)
        self.assertEqual(self.events("tilted"), [])
        self.spin(0.5)
        tilted = self.events("tilted")
        self.assertEqual(len(tilted), 1, self.kinds())
        self.assertGreater(tilted[0]["data"]["pitch"], 45)
        simstate.world.pitch = 0


if __name__ == "__main__":
    unittest.main()

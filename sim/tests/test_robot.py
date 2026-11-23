"""Behaviour tests of the real bundled robot script against fakes."""

import time
import unittest

from helpers import load_robot, example, MemoryClient

ARENA = {
    "walls": [[0, 0, 200, 0], [200, 0, 200, 200], [200, 200, 0, 200], [0, 200, 0, 0]],
    "fields": [{"rect": [0, 150, 200, 50], "color": "blue"}],
    "start": {"x": 100, "y": 50, "heading": 0},
}


class RobotTest(unittest.TestCase):
    def setUp(self):
        self.ns = load_robot(ARENA)
        self.robot = self.ns["Robot"]()
        self.robot.iterate()  # connects
        self.client = MemoryClient.instances[-1]
        self.hb()

    def hb(self):
        self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Team Rot", "name": "Scout", "color": "red", "agent": "waiting"})

    def run_until(self, pred, timeout=5.0):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.robot.iterate()
            if pred():
                return
            time.sleep(0.01)
            if int(time.monotonic() * 2) % 2 == 0:
                self.hb()
        self.fail("condition not reached within %.1fs" % timeout)

    def result(self, cmd_id):
        for r in self.client.messages("result"):
            if r["id"] == cmd_id:
                return r
        return None

    def test_hello_status_and_will(self):
        self.assertEqual(self.client.will, ("lab/mbot-1/status", "offline", True))
        self.assertIn(("lab/mbot-1/status", "online", True), self.client.published)
        hello = self.client.messages("hello")[0]
        self.assertEqual(set(hello), set(example("hello.json")))
        self.assertEqual(hello["v"], 1)

    def test_state_matches_protocol_example(self):
        self.run_until(lambda: self.client.messages("state"))
        state = self.client.messages("state")[0]
        self.assertEqual(set(state), set(example("state.json")))

    def test_drive_measures_distance(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-1", "name": "drive", "args": {"cm": 10}})
        self.run_until(lambda: self.result("c-1"))
        r = self.result("c-1")
        self.assertTrue(r["ok"], r)
        self.assertEqual(r["reason"], "done")
        self.assertAlmostEqual(r["data"]["driven_cm"], 10, delta=2)

    def test_drive_until_color(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-2", "name": "drive_until",
                                               "args": {"max_cm": 150, "stop_on_color": "blue", "speed": 80}})
        self.run_until(lambda: self.result("c-2"), timeout=10)
        r = self.result("c-2")
        self.assertEqual(r["reason"], "color", r)
        self.assertEqual(set(r), set(example("result.json")))

    def test_turn_right(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-3", "name": "turn", "args": {"deg": 90}})
        self.run_until(lambda: self.result("c-3"))
        r = self.result("c-3")
        self.assertTrue(r["ok"], r)
        self.assertAlmostEqual(r["data"]["turned_deg"], 90, delta=6)

    def test_busy_stop_and_duplicates(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-4", "name": "drive", "args": {"cm": 100}})
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-5", "name": "turn", "args": {"deg": 90}})
        self.run_until(lambda: self.result("c-5"))
        self.assertEqual(self.result("c-5")["reason"], "busy")
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-6", "name": "stop"})
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-6", "name": "stop"})
        self.run_until(lambda: self.result("c-6"))
        self.assertEqual(self.result("c-4")["reason"], "stopped")
        stops = [r for r in self.client.messages("result") if r["id"] == "c-6"]
        self.assertEqual(len(stops), 1, "duplicate command must be ignored")

    def test_obstacle_reflex(self):
        # Facing the wall 50 cm ahead; drive_until without stop distance hits the reflex.
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-7", "name": "drive_until",
                                               "args": {"max_cm": 200, "speed": 80}})
        import simstate
        simstate.world.heading = 180  # face the wall at y=0
        self.run_until(lambda: self.result("c-7"), timeout=10)
        self.assertEqual(self.result("c-7")["reason"], "obstacle")

    def test_watchdog_stops_motors(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-8", "name": "drive", "args": {"cm": 200, "speed": 5}})
        deadline = time.monotonic() + 5
        while self.result("c-8") is None and time.monotonic() < deadline:
            self.robot.iterate()  # no heartbeats any more
            time.sleep(0.02)
        self.assertEqual(self.result("c-8")["reason"], "stopped")
        self.assertEqual(self.robot.ui.conn, "getrennt")

    def test_set_leds_survives_heartbeats_until_team_changes(self):
        import simstate
        green, red = (0, 255, 0), (255, 0, 0)
        self.run_until(lambda: simstate.leds == red)  # team color from the heartbeat
        self.client.deliver("lab/mbot-1/cmd", {"id": "l1", "name": "set_leds", "args": {"color": "green"}})
        self.run_until(lambda: self.result("l1") is not None)
        end = time.monotonic() + 1.5  # several heartbeats with team color red
        while time.monotonic() < end:
            self.robot.iterate()
            self.hb()
            time.sleep(0.02)
        self.assertEqual(simstate.leds, green)
        # A new team gets its own color again. Only send its heartbeats here:
        # run_until() would interleave heartbeats of the old team.
        end = time.monotonic() + 5
        while simstate.leds != (0, 0, 255):
            self.assertLess(time.monotonic(), end, "LEDs %s, want the new team's blue" % (simstate.leds,))
            self.client.deliver("lab/mbot-1/hb", {"team": "t2", "owner": "Team Blau", "name": "Scout", "color": "blue", "agent": "waiting"})
            self.robot.iterate()
            time.sleep(0.02)

    def test_unknown_command(self):
        self.client.deliver("lab/mbot-1/cmd", {"id": "c-9", "name": "self_destruct"})
        self.run_until(lambda: self.result("c-9"))
        self.assertEqual(self.result("c-9")["reason"], "error")


    def test_rename_keeps_driving_but_reassign_stops(self):
        """Renaming team or robot must not interrupt a running drive; a new owner must."""
        self.client.deliver("lab/mbot-1/cmd", {"id": "r-1", "name": "drive", "args": {"cm": 100, "speed": 5}})
        self.robot.iterate()
        self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Rote Raketen", "name": "Flitzer", "color": "red", "agent": "waiting"})
        self.robot.iterate()
        self.assertIsNone(self.result("r-1"), "rename must not end the drive")
        self.assertEqual(self.robot.ui.name, "Flitzer")
        self.client.deliver("lab/mbot-1/hb", {"team": "t2", "owner": "Blau", "name": "blau", "color": "blue", "agent": "waiting"})
        self.robot.iterate()
        self.assertEqual(self.result("r-1")["reason"], "reassigned")


if __name__ == "__main__":
    unittest.main()

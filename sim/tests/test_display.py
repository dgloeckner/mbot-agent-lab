"""The robot display: one screen per state, every label inside the 128 x 128 pixels."""

import time
import unittest

from helpers import load_robot, simstate

ARENA = {
    "walls": [[0, 0, 200, 0], [200, 0, 200, 200], [200, 200, 0, 200], [0, 200, 0, 0]],
    "start": {"x": 100, "y": 100, "heading": 0},
}


class DisplayTest(unittest.TestCase):
    def setUp(self):
        self.ns = load_robot(ARENA)
        self.robot = self.ns["Robot"]()
        self.robot.iterate()  # connects
        self.client = self.ns["MQTTClient"].instances[-1]

    def hb(self, team="t1", owner="Rote Raketen", name="Scout", agent="waiting"):
        self.client.deliver("lab/mbot-1/hb", {"team": team, "owner": owner, "name": name,
                                              "color": "red", "agent": agent})

    def shows(self, *texts, heartbeat=None, timeout=3.0):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if heartbeat:
                heartbeat()
            self.robot.iterate()
            if all(t in simstate.display for t in texts):
                return
            time.sleep(0.02)
        self.fail("display %r, want %r" % (simstate.display, texts))

    def test_free_robot_shows_its_id(self):
        self.shows("frei", "mbot-1", "wartet auf Team", heartbeat=lambda: self.hb(team="", owner="", name=""))

    def test_assigned_robot_shows_name_in_team_color_and_team(self):
        self.shows("Scout", "Rote Raketen", heartbeat=self.hb)
        name = [label for label in simstate.screen if label[0] == "Scout"][0]
        self.assertEqual(name[1], 32, "short names in the largest size")
        self.assertEqual(name[4], (255, 0, 0), "name in team color")

    def test_thinking_shows_in_the_status_bar(self):
        self.shows("denkt...", heartbeat=lambda: self.hb(agent="thinking"))

    def test_show_text_takes_the_screen_and_a_new_team_clears_it(self):
        self.shows("Scout", heartbeat=self.hb)
        self.client.deliver("lab/mbot-1/cmd", {"id": "t-1", "name": "show_text",
                                               "args": {"text": "Ich bin auf Kreuzung B2 und fahre nach Norden"}})
        self.shows("Ich bin auf", "Kreuzung B2 und", "fahre nach", "Norden", heartbeat=self.hb)
        self.assertNotIn("Rote Raketen", simstate.display)
        self.shows("Blau", heartbeat=lambda: self.hb(team="t2", owner="Blau"))
        self.assertNotIn("Norden", simstate.display)

    def test_lost_connection_says_motors_stopped(self):
        self.shows("Scout", heartbeat=self.hb)
        self.shows("Verbindung weg", "Motoren gestoppt", timeout=5)  # no heartbeats: watchdog

    def test_wrap(self):
        wrap = self.ns["ui_wrap"]
        self.assertEqual(wrap("Ich bin auf Kreuzung B2", 6), ["Ich bin auf", "Kreuzung B2"])
        self.assertEqual(wrap("a" * 20 + " b", 6), ["a" * 16, "aaaa b"])
        self.assertEqual(len(wrap("x " * 60, 6)), 6)

    def test_every_screen_fits(self):
        ui = self.ns["Ui"]("mbot-a1b2c3")
        char_w = self.ns["UI_CHAR_W"]
        long = "Die superschnellen Robo-Raketen aus dem Keller"
        for conn in ("startet", "verbunden", "getrennt"):
            for owner, name in (("", ""), ("Rot", "Scout"), (long, long)):
                for text in ("", long):
                    for agent in ("waiting", "thinking"):
                        ui.set_conn(conn)
                        ui.set_owner(owner, name, "red")
                        ui.set_text(text)
                        ui.set_agent(agent)
                        ui.set_battery(100)
                        for label, size, x, y, _ in ui.screen():
                            where = (conn, owner, text, agent, label)
                            self.assertGreaterEqual(x, 0, where)
                            self.assertLessEqual(x + len(label) * char_w[size], 128, where)
                            self.assertLessEqual(y + size, 128, where)


if __name__ == "__main__":
    unittest.main()

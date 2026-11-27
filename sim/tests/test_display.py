"""The robot display: one screen per state, every label inside the 128 x 128 pixels."""

import time
import unittest

from helpers import load_robot, simstate

import cyberpi  # noqa: E402  the fake in sim/fakes, on the path after helpers

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

    def test_show_text_keeps_every_line_and_the_status_bar_on_screen(self):
        # Seen on the robot on 2026-11-27: of this text only "dich :)" was visible,
        # because every label was drawn into the same slot.
        self.shows("Scout", heartbeat=self.hb)
        self.client.deliver("lab/mbot-1/cmd", {"id": "t-1", "name": "show_text", "args": {
            "text": "Schoen, dass du da bist! Team Gallier gruesst dich :)"}})
        self.shows("dich :)", heartbeat=self.hb)
        self.assertEqual(simstate.display, ["Scout", "100%", "Schoen, dass du", "da bist! Team",
                                            "Gallier gruesst", "dich :)"])

    def test_a_smaller_screen_leaves_nothing_of_a_bigger_one(self):
        # Even if clear() on the real display should keep the labels it does not redraw.
        cyberpi.display.clear_keeps_labels = True
        self.addCleanup(setattr, cyberpi.display, "clear_keeps_labels", False)
        self.shows("Scout", heartbeat=self.hb)
        self.client.deliver("lab/mbot-1/cmd", {"id": "t-1", "name": "show_text",
                                               "args": {"text": "eins zwei drei vier " * 3}})
        self.shows("Scout", "100%", "eins zwei drei", heartbeat=self.hb)
        self.assertEqual(len(simstate.display), 6)
        self.shows("frei", heartbeat=lambda: self.hb(team="", owner="", name=""))
        self.assertEqual(simstate.display, ["online", "100%", "frei", "mbot-1", "wartet auf Team"])

    def test_error_screen_shows_the_whole_message(self):
        self.ns["ui_error"]("OSError(113, 'kein Weg zum Broker')")
        self.assertEqual(simstate.display, ["Fehler", "OSError(113,", "'kein Weg zum", "Broker')"])

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
                        self.assertLessEqual(len(ui.screen()), cyberpi.LABELS, (conn, owner, text, agent))
                        for label, size, x, y, _ in ui.screen():
                            where = (conn, owner, text, agent, label)
                            self.assertGreaterEqual(x, 0, where)
                            self.assertLessEqual(x + len(label) * char_w[size], 128, where)
                            self.assertLessEqual(y + size, 128, where)


if __name__ == "__main__":
    unittest.main()

"""ask_human on the robot: a question on the display, answered with A or B."""

import random
import time
import unittest

from helpers import load_robot, MemoryClient
import simstate  # noqa: E402  (helpers puts the fakes on sys.path)

ARENA = {
    "walls": [[0, 0, 300, 0], [300, 0, 300, 300], [300, 300, 0, 300], [0, 300, 0, 0]],
    "start": {"x": 150, "y": 150, "heading": 0},
}


class AskTest(unittest.TestCase):
    def setUp(self):
        simstate.buttons = {"a": False, "b": False}
        self.ns = load_robot(ARENA, noise=random.Random(3))
        self.robot = self.ns["Robot"]()
        self.robot.iterate()
        self.client = MemoryClient.instances[-1]
        self.spin(0.3)

    def spin(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Rot", "name": "Scout",
                                                  "color": "red", "agent": "waiting"})
            self.robot.iterate()
            time.sleep(0.01)

    def cmd(self, cmd_id, name, **args):
        self.client.deliver("lab/mbot-1/cmd", {"id": cmd_id, "name": name, "args": args})

    def result(self, cmd_id):
        for r in self.client.messages("result"):
            if r["id"] == cmd_id:
                return r
        return None

    def press(self, key):
        simstate.buttons[key] = True
        self.spin(0.1)
        simstate.buttons[key] = False
        self.spin(0.1)

    def test_answer_with_a_shows_the_question_and_no_button_event(self):
        self.cmd("q1", "ask_human", text="Darf ich losfahren?", s=10)
        self.spin(0.3)
        self.assertIn("Darf ich losfahren?", simstate.display)
        self.assertIn("A = ja, B = nein", simstate.display)
        self.assertIsNone(self.result("q1"))
        self.press("a")
        r = self.result("q1")
        self.assertEqual((r["ok"], r["reason"], r["data"]["answer"]), (True, "done", "a"))
        self.assertEqual([e for e in self.client.messages("event") if e["kind"] == "button_a"], [])
        self.spin(0.3)
        self.assertNotIn("A = ja, B = nein", simstate.display)
        # Afterwards the keys are events again.
        self.spin(0.5)
        self.press("b")
        self.assertEqual(len([e for e in self.client.messages("event") if e["kind"] == "button_b"]), 1)

    def test_no_answer_times_out(self):
        self.cmd("q2", "ask_human", text="Hallo?", s=1)
        self.spin(1.3)
        r = self.result("q2")
        self.assertEqual((r["ok"], r["reason"], r["data"]["answer"]), (True, "timeout", "timeout"))

    def test_a_key_held_when_asked_does_not_answer(self):
        simstate.buttons["b"] = True
        self.spin(0.1)
        self.cmd("q3", "ask_human", text="Wirklich?", s=10)
        self.spin(0.3)
        self.assertIsNone(self.result("q3"))
        simstate.buttons["b"] = False
        self.spin(0.1)
        self.press("b")
        self.assertEqual(self.result("q3")["data"]["answer"], "b")

    def test_a_drive_command_ends_the_question(self):
        self.cmd("q4", "ask_human", text="Warten?", s=10)
        self.spin(0.2)
        self.cmd("d1", "drive", cm=5)
        self.spin(0.2)
        r = self.result("q4")
        self.assertEqual((r["ok"], r["reason"], r["data"]["answer"]), (False, "stopped", "stopped"))
        self.assertIsNotNone(self.robot.skill)

    def test_only_one_question_and_none_while_driving(self):
        self.cmd("q5", "ask_human", text="Eins?", s=10)
        self.cmd("q6", "ask_human", text="Zwei?", s=10)
        self.spin(0.2)
        self.assertEqual(self.result("q6")["reason"], "busy")
        self.cmd("s1", "stop")
        self.cmd("d2", "drive", cm=50)
        self.spin(0.2)
        self.cmd("q7", "ask_human", text="Drei?", s=10)
        self.spin(0.2)
        self.assertEqual(self.result("q7")["reason"], "busy")


if __name__ == "__main__":
    unittest.main()

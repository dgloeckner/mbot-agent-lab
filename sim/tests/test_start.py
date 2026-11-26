"""How the bundle starts: through the firmware's start event, and without it."""

import _thread
import sys
import threading
import time
import types
import unittest

from helpers import ENTRY, MemoryClient, load_robot
import event  # fakes/event.py

BOX = {
    "walls": [[0, 0, 100, 0], [100, 0, 100, 100], [100, 100, 0, 100], [0, 100, 0, 0]],
    "start": {"x": 50, "y": 50, "heading": 0},
}


def wait_for(cond, timeout=5):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if cond():
            return True
        time.sleep(0.01)
    return False


def hello(client):
    return any(t.endswith("/hello") for t, _, _ in client.published)


def thread_shim():
    """_thread as on the CyberPi: CPython refuses stacks below about 52 KB."""
    m = types.ModuleType("_thread")
    m.sizes = []
    m.stack_size = m.sizes.append
    m.start_new_thread = _thread.start_new_thread
    return m


class StartTest(unittest.TestCase):
    def setUp(self):
        sys.modules.pop("mbot_gen", None)
        self.saved_event = sys.modules.get("event")
        self.saved_thread = sys.modules.get("_thread")
        self.shim = sys.modules["_thread"] = thread_shim()
        self.threads = []
        self.clients = []
        del event.handlers[:]

    def tearDown(self):
        # A robot thread stops once the counter is no longer its own.
        sys.modules.pop("mbot_gen", None)
        for t in self.threads:
            t.join(3)
        for c in self.clients:
            wait_for(lambda: c.closed, 3)
        sys.modules["event"] = self.saved_event
        sys.modules["_thread"] = self.saved_thread
        del event.handlers[:]

    def upload(self):
        """Load the bundle up to its last line, as a fresh upload would."""
        ns = load_robot(BOX)
        exec(ENTRY, ns)
        return ns

    def started_client(self):
        self.assertTrue(wait_for(lambda: MemoryClient.instances and hello(MemoryClient.instances[-1])),
                        "robot did not connect")
        c = MemoryClient.instances[-1]
        self.clients.append(c)
        return c

    def fire(self):
        threads = event.fire_start()
        self.threads += threads
        return threads

    def test_starts_through_start_event(self):
        ns = self.upload()
        self.assertIs(ns["event"], event)
        # Loading only registers the handler: nothing runs before the firmware fires it.
        self.assertEqual(event.handlers, [ns["main"]])
        time.sleep(0.05)
        self.assertEqual(MemoryClient.instances, [])
        self.assertNotIn("mbot_gen", sys.modules)

        (handler,) = self.fire()
        self.started_client()
        self.assertEqual(sys.modules["mbot_gen"], [1])
        self.assertEqual(self.shim.sizes, [32 * 1024])  # the robot runs in its own thread
        # The handler keeps running while its robot runs.
        time.sleep(0.1)
        self.assertTrue(handler.is_alive())

    def test_second_start_stops_older_robot(self):
        self.upload()
        (first,) = self.fire()
        old = self.started_client()

        self.upload()  # a new upload into the same interpreter
        (second,) = self.fire()
        new = self.started_client()
        self.assertIsNot(old, new)
        self.assertEqual(sys.modules["mbot_gen"], [2])

        self.assertTrue(wait_for(lambda: old.closed, 3), "older robot thread did not stop")
        first.join(3)
        self.assertFalse(first.is_alive(), "older start handler did not return")
        self.assertFalse(new.closed)
        self.assertTrue(second.is_alive())

    def test_second_start_stops_older_robot_without_thread(self):
        # Without _thread the robot runs inside the start handler.
        sys.modules["_thread"] = None
        self.upload()
        (first,) = self.fire()
        old = self.started_client()
        self.upload()
        (second,) = self.fire()
        new = self.started_client()
        first.join(3)
        self.assertFalse(first.is_alive(), "older robot did not stop")
        self.assertTrue(old.closed)
        self.assertFalse(new.closed)
        self.assertTrue(second.is_alive())

    def test_fallback_without_event_module(self):
        sys.modules["event"] = None  # import event fails, as on firmware without it
        ns = load_robot(BOX)
        self.assertIsNone(ns["event"])
        # Without the event module the entry runs main() right away and stays there.
        entry = threading.Thread(target=exec, args=(ENTRY, ns), daemon=True)
        entry.start()
        self.threads.append(entry)
        self.started_client()
        self.assertEqual(event.handlers, [])
        self.assertEqual(self.shim.sizes, [32 * 1024])
        self.assertTrue(entry.is_alive())

        sys.modules["mbot_gen"][0] += 1  # what the next start does
        entry.join(3)
        self.assertFalse(entry.is_alive(), "entry did not return after a newer start")


if __name__ == "__main__":
    unittest.main()

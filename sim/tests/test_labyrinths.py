"""The labyrinth worlds of the challenge and the network delay of the simulator.

labyrinth_rechts: up, right, down to blue; turning right at every wall arrives.
labyrinth_gestoert: the same with a box in the top corridor, a detour and the
blue field in a side corridor. Sensors are tested with noise on.
"""

import random
import socket
import statistics
import threading
import time
import unittest

from helpers import SIM, World
from netdelay import DelayProxy

WORLDS = SIM / "worlds"


def world(name, x, y, heading, seed=7):
    w = World.load(str(WORLDS / (name + ".json")), noise=random.Random(seed))
    w.x, w.y, w.heading = float(x), float(y), float(heading)
    w.yaw_zero = w.heading
    return w


def distance(w, n=15):
    """Median of n readings: the noisy sensor drops out and echoes now and then."""
    return statistics.median(w.distance_cm() for _ in range(n))


class LabyrinthWorldTest(unittest.TestCase):
    def test_ultrasonic_sees_walls_and_the_box(self):
        # Top corridor, looking east from the second cell.
        free = distance(world("labyrinth_rechts", 75, 175, 90))
        boxed = distance(world("labyrinth_gestoert", 75, 175, 90))
        self.assertGreater(free, 80)  # the sound cone meets the side walls before the far end
        self.assertAlmostEqual(boxed, 110 - 75 - 7, delta=4)
        # The detour down the second column is open in the disturbed world only.
        self.assertGreater(distance(world("labyrinth_gestoert", 75, 175, 180)), 60)
        self.assertLess(distance(world("labyrinth_rechts", 75, 175, 180)), 25)
        # A cell wall straight ahead reads below 40 cm, an open cell above.
        self.assertLess(distance(world("labyrinth_gestoert", 25, 25, 90)), 40)
        self.assertGreater(distance(world("labyrinth_gestoert", 25, 25, 0)), 40)

    def test_probes_see_the_blue_field(self):
        for name, x, y in (("labyrinth_rechts", 175, 20), ("labyrinth_gestoert", 125, 20)):
            w = world(name, x, y, 180)
            colors = [w.ground_color() for _ in range(9)]
            self.assertEqual(statistics.mode(colors), "blue", (name, colors))
            self.assertEqual(world(name, x, 75, 180).ground_color(), "white")


class DelayTest(unittest.TestCase):
    def test_delay_applies_to_every_packet_in_both_directions(self):
        echo = socket.socket()
        echo.bind(("127.0.0.1", 0))
        echo.listen(1)

        def serve():
            conn, _ = echo.accept()
            while True:
                data = conn.recv(100)
                if not data:
                    return
                conn.sendall(data)

        threading.Thread(target=serve, daemon=True).start()
        proxy = DelayProxy("127.0.0.1", echo.getsockname()[1], 0.1)
        c = socket.create_connection(("127.0.0.1", proxy.port))
        try:
            for msg in (b"cmd", b"result"):
                start = time.monotonic()
                c.sendall(msg)
                got = b""
                while len(got) < len(msg):
                    got += c.recv(100)
                took = time.monotonic() - start
                self.assertEqual(got, msg)
                self.assertGreaterEqual(took, 0.2)  # 100 ms there, 100 ms back
                self.assertLess(took, 0.6)
        finally:
            c.close()
            proxy.close()
            echo.close()


if __name__ == "__main__":
    unittest.main()

import ast
import unittest

from helpers import bundle, World


class FakeClock:
    def __init__(self):
        self.t = 0.0

    def __call__(self):
        return self.t


BOX = {
    "walls": [[0, 0, 100, 0], [100, 0, 100, 100], [100, 100, 0, 100], [0, 100, 0, 0]],
    "fields": [{"rect": [0, 80, 100, 20], "color": "red"}],
    "lines": [{"points": [[50, 0], [50, 100]], "width": 2}],
    "start": {"x": 50, "y": 20, "heading": 0},
}


class WorldTest(unittest.TestCase):
    def setUp(self):
        self.clock = FakeClock()
        self.w = World(BOX, clock=self.clock)
        self.w.motor_lag = 0  # ideal motors; WorldMotorTest covers the lag

    def test_distance_ahead(self):
        # Sensor sits 7 cm ahead of the centre, wall at y=100.
        self.assertAlmostEqual(self.w.distance_cm(), 100 - 20 - 7, delta=0.2)

    def test_drive_straight(self):
        self.w.set_motors(60, 60)
        self.clock.t = 1.0
        left, right = self.w.wheel_deg()
        self.assertAlmostEqual(left, 360, delta=0.1)
        self.assertAlmostEqual(self.w.y, 20 + 6.5 * 3.14159, delta=0.1)

    def test_turn_right_increases_yaw(self):
        self.w.set_motors(30, -30)
        self.clock.t = 0.5
        self.assertGreater(self.w.yaw(), 0)

    def test_wall_blocks(self):
        self.w.set_motors(200, 200)
        self.clock.t = 10
        self.w.update()
        self.assertLess(self.w.y, 100 - 7)
        self.assertTrue(self.w.bumped)

    def test_ground(self):
        self.assertEqual(self.w.ground_color(), "black")  # on the line
        self.assertEqual(self.w.line_bits(), 0b0110)
        self.w.x = 20
        self.assertEqual(self.w.ground_color(), "white")
        self.w.y = 85
        self.assertEqual(self.w.ground_color(), "red")


class BundleTest(unittest.TestCase):
    def test_bundle_has_no_local_imports(self):
        src = bundle.bundle()
        tree = ast.parse(src)
        local = set(bundle.MODULES)
        for node in tree.body:
            if isinstance(node, ast.ImportFrom):
                self.assertNotIn(node.module, local)
        self.assertTrue(src.rstrip().endswith("\nhw_on_start(main)"))

    def test_bundle_imports_only_firmware_modules(self):
        # Every import must exist on the CyberPi: the firmware had no umqtt, so the
        # robot crashed with ImportError at start. Add a module only after checking
        # it on the robot. json/time are fallbacks for CPython, _thread, gc and event are
        # optional, all behind try/except (event: mBlock's generated code imports it, VERIFY).
        allowed = {"config", "cyberpi", "mbot2", "mbuild", "event", "socket", "ssl", "time", "json", "ujson",
                   "_thread", "sys", "gc"}
        found = set()
        for node in ast.walk(ast.parse(bundle.bundle())):
            if isinstance(node, ast.Import):
                found |= {a.name.split(".")[0] for a in node.names}
            elif isinstance(node, ast.ImportFrom):
                found.add(node.module.split(".")[0])
        self.assertLessEqual(found, allowed)

    def test_bundle_size_budget(self):
        # Not RAM but start-up limits the size: on 2026-11-23 a bundle of 80 KB
        # started, one of 120 KB hung the CyberPi (docs/measurements.md). The
        # budget keeps 25 % below 80 KB. Raise it deliberately, not by accident.
        self.assertLess(len(bundle.bundle().encode()), 60000)


if __name__ == "__main__":
    unittest.main()

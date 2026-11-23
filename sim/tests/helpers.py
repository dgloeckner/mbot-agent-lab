"""Test helpers: load the bundled robot script in-process with fakes."""

import json
import pathlib
import random
import sys
import time
import types

HERE = pathlib.Path(__file__).resolve().parent
SIM = HERE.parent
ROOT = SIM.parent
EXAMPLES = ROOT / "docs" / "examples" / "protocol"

for p in (str(SIM / "fakes"), str(SIM), str(ROOT / "robot" / "tools")):
    if p not in sys.path:
        sys.path.insert(0, p)

import bundle  # noqa: E402
import simstate  # noqa: E402
from world import World  # noqa: E402


class MemoryClient:
    """In-memory stand-in for the robot's MQTTClient (robot/src/mqtt.py)."""

    instances = []

    def __init__(self, client_id, server, **kw):
        self.client_id = client_id
        self.cb = None
        self.published = []   # (topic, payload str, retain)
        self.incoming = []
        self.will = None
        MemoryClient.instances.append(self)

    def set_callback(self, f):
        self.cb = f

    def set_last_will(self, topic, msg, retain=False, qos=0):
        self.will = (topic, msg, retain)

    def connect(self, clean_session=True):
        return 0

    def subscribe(self, topic, qos=0):
        pass

    def publish(self, topic, msg, retain=False, qos=0):
        self.published.append((topic, msg, retain))

    def check_msg(self):
        if self.incoming:
            topic, payload = self.incoming.pop(0)
            self.cb(topic.encode(), payload.encode())

    def ping(self):
        pass

    def disconnect(self):
        pass

    def deliver(self, topic, obj):
        self.incoming.append((topic, json.dumps(obj)))

    def messages(self, suffix):
        return [json.loads(m) for t, m, _ in self.published if t.endswith("/" + suffix) and m.startswith("{")]


def load_robot(world_spec, robot_id="mbot-1", noise=None):
    """Return the bundle namespace with world, config and MemoryClient installed.

    noise is a random.Random for realistic sensor noise, None for ideal sensors.
    """
    simstate.world = World(world_spec, robot_id, noise=noise)
    cfg = types.ModuleType("config")
    cfg.ROBOT_ID = robot_id
    cfg.WIFI_SSID = cfg.WIFI_PASSWORD = "sim"
    cfg.BROKER_HOST, cfg.BROKER_PORT, cfg.BROKER_TLS, cfg.BROKER_PASSWORD = "x", 1883, False, "pw"
    sys.modules["config"] = cfg

    source = bundle.bundle()
    body, last = source.rstrip().rsplit("\n", 1)
    assert last.strip() == "main()", "bundle must end with main()"
    ns = {"__name__": "robot_under_test"}
    exec(compile(body, "main.py", "exec"), ns)
    ns["MQTTClient"] = MemoryClient
    MemoryClient.instances = []
    return ns


def example(name):
    return json.loads((EXAMPLES / name).read_text())


class SkillRun:
    """Runs the real robot script against a world and returns command results."""

    def __init__(self, spec, seed=1):
        self.ns = load_robot(spec, noise=random.Random(seed) if seed is not None else None)
        self.robot = self.ns["Robot"]()
        self.robot.iterate()
        self.client = MemoryClient.instances[-1]
        self.n = 0

    def hb(self):
        self.client.deliver("lab/mbot-1/hb", {"team": "t1", "owner": "Test", "name": "Scout", "color": "blue", "agent": "acting"})

    def settle(self, n=6):
        """Let the filters catch up, e.g. after moving the robot by hand."""
        for _ in range(n):
            self.robot.iterate()
            time.sleep(0.005)

    def place(self, x, y, heading):
        """Put the robot somewhere else, as a teacher would."""
        world = simstate.world
        world.update()
        world.x, world.y, world.heading = float(x), float(y), float(heading)
        self.settle()

    def run(self, name, args, timeout=25):
        self.n += 1
        cid = "g-%d" % self.n
        self.hb()
        self.client.deliver("lab/mbot-1/cmd", {"id": cid, "name": name, "args": args})
        deadline = time.monotonic() + timeout
        last_hb = time.monotonic()
        while time.monotonic() < deadline:
            self.robot.iterate()
            for r in self.client.messages("result"):
                if r["id"] == cid:
                    return r
            if time.monotonic() - last_hb > 0.5:
                self.hb()
                last_hb = time.monotonic()
            time.sleep(0.01)
        raise AssertionError("%s did not finish within %ss" % (name, timeout))

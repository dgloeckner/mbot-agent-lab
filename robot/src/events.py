# Event detection on filtered sensors (sense.py debounces colors and tilt),
# plus hysteresis for obstacles. Detected events are queued and published by
# the main loop.

from compat import ticks_diff

EV_NEAR_CM = 15         # obstacle_near below this distance
EV_CLEAR_CM = 20        # obstacle_clear above this distance (hysteresis)
EV_BATTERY_LOW = 15     # percent
EV_CLAP_LEVEL = 70      # loudness 0..100
EV_CLAP_JUMP = 40       # a clap stands this far above the background level
EV_CLAP_COOLDOWN_MS = 1000
EV_HUSH_MOTION_MS = 500  # deaf to claps while motors run and this long after
EV_HUSH_SOUND_MS = 2000  # deaf to claps this long after the robot plays a sound


class Events:
    def __init__(self):
        self.queue = []
        self.near = False
        self.color = None
        self.buttons = {"a": False, "b": False}
        self.battery_warned = False
        self.tilted = False
        self.last_clap = -EV_CLAP_COOLDOWN_MS
        self.noise = 0
        self.hush_at = 0
        self.hush_ms = 0

    def emit(self, kind, data=None):
        self.queue.append((kind, data))

    def hush(self, now, ms):
        """Ignore claps until ms after now: the robot's own motors and speaker
        sit right next to the microphone."""
        if ms > self.hush_ms - ticks_diff(now, self.hush_at):
            self.hush_at = now
            self.hush_ms = ms

    def update(self, s, now):
        """s is the sensor snapshot dict built by the main loop."""
        d = s["distance_cm"]
        if not self.near and d < EV_NEAR_CM:
            self.near = True
            self.emit("obstacle_near", {"distance_cm": d})
        elif self.near and d > EV_CLEAR_CM:
            self.near = False
            self.emit("obstacle_clear", {"distance_cm": d})

        c = s["color"]
        if c != self.color:
            prev = self.color
            self.color = c
            if prev is not None:
                self.emit("color_changed", {"color": c, "previous": prev})

        for name in ("a", "b"):
            pressed = s["button_" + name]
            if pressed and not self.buttons[name]:
                self.emit("button_" + name)
            self.buttons[name] = pressed

        if s["battery"] < EV_BATTERY_LOW and not self.battery_warned:
            self.battery_warned = True
            self.emit("battery_low", {"battery": s["battery"]})

        if s["tilted"] and not self.tilted:
            self.emit("tilted")
        self.tilted = s["tilted"]

        # A clap is a short peak above the background level, which follows
        # steady noise (talking, music) so that alone never counts as a clap.
        x = s["loudness"]
        if (x >= EV_CLAP_LEVEL and x - self.noise >= EV_CLAP_JUMP
                and ticks_diff(now, self.hush_at) > self.hush_ms
                and ticks_diff(now, self.last_clap) > EV_CLAP_COOLDOWN_MS):
            self.last_clap = now
            self.emit("clap", {"loudness": x})
        self.noise += (x - self.noise) / 8

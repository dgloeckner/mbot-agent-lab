# Event detection on filtered sensors (sense.py debounces colors and tilt),
# plus hysteresis for obstacles. Detected events are queued and published by
# the main loop.

from compat import ticks_diff

EV_NEAR_CM = 15         # obstacle_near below this distance
EV_CLEAR_CM = 20        # obstacle_clear above this distance (hysteresis)
EV_BATTERY_LOW = 15     # percent
EV_CLAP_LEVEL = 70      # loudness 0..100
EV_CLAP_COOLDOWN_MS = 1000


class Events:
    def __init__(self):
        self.queue = []
        self.near = False
        self.color = None
        self.buttons = {"a": False, "b": False}
        self.battery_warned = False
        self.tilted = False
        self.last_clap = -EV_CLAP_COOLDOWN_MS

    def emit(self, kind, data=None):
        self.queue.append((kind, data))

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

        if s["loudness"] >= EV_CLAP_LEVEL and ticks_diff(now, self.last_clap) > EV_CLAP_COOLDOWN_MS:
            self.last_clap = now
            self.emit("clap", {"loudness": s["loudness"]})

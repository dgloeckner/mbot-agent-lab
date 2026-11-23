# Event detection on filtered sensors (sense.py debounces colors and tilt),
# plus hysteresis for obstacles. Detected events are queued and published by
# the main loop. The robot only reports what happened; what it means (lifted:
# someone picked me up) is up to the agent.
#
# All thresholds are start values until measured (robot/tools/measure.py).

from compat import ticks_diff

EV_NEAR_CM = 15         # obstacle_near below this distance
EV_CLEAR_CM = 20        # obstacle_clear above this distance (hysteresis)
EV_BATTERY_LOW = 15     # percent
EV_GAP_MS = 500         # each of the newer events at most once per this
EV_MEAN_MS = 10000      # light and loudness count against their mean over about this long
EV_LIGHT_JUMP = 15      # light_up / light_down: this far from the mean. VERIFY
EV_CLAP_JUMP = 15       # clap: loudness this far above the mean ... VERIFY
EV_CLAP_JUMP_MOVING = 35  # ... while the motors run: they are loud. VERIFY
EV_BUMP = 6.0           # bumped: horizontal acceleration beyond this, m/s^2 ... VERIFY
EV_BUMP_MOVING = 10.0   # ... while the motors run: they shake. VERIFY
EV_LIFT = 3.0           # lifted: vertical acceleration this far above gravity. VERIFY
EV_G = 9.8
EV_TILT_MS = 500        # tilted: only when it lasts this long


def ev_mean(mean, x, dt):
    """Running mean that forgets over about EV_MEAN_MS."""
    if mean is None:
        return x
    k = dt / EV_MEAN_MS
    return mean + (x - mean) * (k if k < 1 else 1)


class Events:
    def __init__(self):
        self.queue = []
        self.near = False
        self.color = None
        self.buttons = {"a": False, "b": False}
        self.joystick = None
        self.shake = False
        self.battery_warned = False
        self.tilt_since = None
        self.tilt_sent = False
        self.sent = {}            # kind -> when it was last sent
        self.t = None
        self.light_mean = None
        self.light_armed = True   # a jump fires once, then waits until light is near the mean
        self.loud_mean = None
        self.clap_armed = True

    def emit(self, kind, data=None):
        self.queue.append((kind, data))

    def emit_gap(self, kind, now, data=None):
        """emit, but not twice within EV_GAP_MS."""
        last = self.sent.get(kind)
        if last is not None and ticks_diff(now, last) < EV_GAP_MS:
            return
        self.sent[kind] = now
        self.emit(kind, data)

    def update(self, s, now, moving=False):
        """s is the sensor snapshot dict built by the main loop."""
        dt = 0 if self.t is None else ticks_diff(now, self.t)
        self.t = now

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
                self.emit_gap("button_" + name, now)
            self.buttons[name] = pressed

        j = s["joystick"]
        if j and j != self.joystick:
            self.emit_gap("joystick", now, {"dir": j})
        self.joystick = j

        if s["battery"] < EV_BATTERY_LOW and not self.battery_warned:
            self.battery_warned = True
            self.emit("battery_low", {"battery": s["battery"]})

        if not s["tilted"]:
            self.tilt_since = None
            self.tilt_sent = False
        elif self.tilt_since is None:
            self.tilt_since = now
        elif not self.tilt_sent and ticks_diff(now, self.tilt_since) >= EV_TILT_MS:
            self.tilt_sent = True
            self.emit_gap("tilted", now, {"pitch": s["pitch"], "roll": s["roll"]})

        if s["shake"] and not self.shake:
            self.emit_gap("shake", now)
        self.shake = s["shake"]

        ax, ay, az = s["acc"]
        side = max(abs(ax), abs(ay))
        if side > (EV_BUMP_MOVING if moving else EV_BUMP):
            self.emit_gap("bumped", now)
        elif abs(az) - EV_G > EV_LIFT:
            self.emit_gap("lifted", now)

        x = s["light"]
        if self.light_mean is not None:
            diff = x - self.light_mean
            if self.light_armed and abs(diff) >= EV_LIGHT_JUMP:
                self.light_armed = False
                self.emit_gap("light_up" if diff > 0 else "light_down", now,
                              {"light": x, "base": int(self.light_mean)})
            elif abs(diff) < EV_LIGHT_JUMP / 2:
                self.light_armed = True
        self.light_mean = ev_mean(self.light_mean, x, dt)

        x = s["loudness"]
        if self.loud_mean is not None:
            diff = x - self.loud_mean
            if self.clap_armed and diff >= (EV_CLAP_JUMP_MOVING if moving else EV_CLAP_JUMP):
                self.clap_armed = False
                self.emit_gap("clap", now, {"loudness": x})
            elif diff < EV_CLAP_JUMP / 2:
                self.clap_armed = True
        self.loud_mean = ev_mean(self.loud_mean, x, dt)

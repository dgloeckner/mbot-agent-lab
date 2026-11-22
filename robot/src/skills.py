# Skills are small state machines. A motion skill is started once and then
# advanced by step() on every loop iteration; it never blocks.
#
# step() returns None while running, or a tuple (ok, reason, data) when done.

from compat import ticks_ms, ticks_diff
from hw import (hw_motors, hw_motors_stop, hw_wheels_reset, hw_wheels_deg,
                hw_yaw, hw_yaw_reset, CM_PER_WHEEL_DEG)

SK_DEFAULT_SPEED = 40      # RPM
SK_MAX_SPEED = 80          # RPM, hard limit
SK_TURN_SPEED = 30         # RPM
SK_MAX_CM = 300            # per command
SK_MAX_DEG = 720           # per command
SK_MAX_MS = 20000          # per command; the server waits this + 2 s
SK_TURN_TOLERANCE_DEG = 2


def _clamp(v, lo, hi):
    return lo if v < lo else hi if v > hi else v


def _num(args, key, default=None):
    v = args.get(key, default)
    if v is None:
        return None
    if not isinstance(v, (int, float)):
        raise ValueError("%s must be a number" % key)
    return v


def _driven_cm():
    left, right = hw_wheels_deg()
    return (left + right) / 2 * CM_PER_WHEEL_DEG


class DriveSkill:
    """drive(cm, speed) and drive_until(max_cm, stop_below_cm, stop_on_color)."""

    name = "drive"
    forward = True

    def __init__(self, args, until=False):
        self.until = until
        if until:
            self.name = "drive_until"
            cm = _num(args, "max_cm")
            self.stop_below = _num(args, "stop_below_cm")
            self.stop_color = args.get("stop_on_color")
        else:
            cm = _num(args, "cm")
            self.stop_below = None
            self.stop_color = None
        if cm is None:
            raise ValueError("distance missing")
        self.target = _clamp(abs(cm), 0, SK_MAX_CM)
        self.direction = -1 if cm < 0 else 1
        self.forward = self.direction > 0
        self.speed = _clamp(abs(_num(args, "speed", SK_DEFAULT_SPEED)), 5, SK_MAX_SPEED)
        self.started = 0
        self.driven = 0

    def start(self):
        hw_wheels_reset()
        self.started = ticks_ms()
        v = self.direction * self.speed
        hw_motors(v, v)

    def segment(self):
        return ("drive", round(self.direction * self.driven, 1))

    def _end(self, ok, reason, s):
        hw_motors_stop()
        data = {"driven_cm": round(self.driven, 1)}
        if self.until:
            data["distance_cm"] = s["distance_cm"]
            data["color"] = s["color"]
        return ok, reason, data

    def step(self, s, now):
        self.driven = abs(_driven_cm())
        if self.driven >= self.target:
            return self._end(True, "max_cm" if self.until else "done", s)
        if self.stop_below is not None and self.forward and s["distance_cm"] < self.stop_below:
            return self._end(True, "wall", s)
        if self.stop_color is not None and s["color"] == self.stop_color:
            return self._end(True, "color", s)
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        return None

    def abort(self, reason, s):
        return self._end(False, reason, s)


class TurnSkill:
    """turn(deg): positive is right, measured with the gyro."""

    name = "turn"
    forward = False

    def __init__(self, args):
        deg = _num(args, "deg")
        if deg is None:
            raise ValueError("deg missing")
        self.target = _clamp(deg, -SK_MAX_DEG, SK_MAX_DEG)
        self.started = 0
        self.turned = 0
        self.last_yaw = 0

    def start(self):
        hw_yaw_reset()
        self.started = ticks_ms()
        self.last_yaw = 0
        v = SK_TURN_SPEED if self.target > 0 else -SK_TURN_SPEED
        hw_motors(v, -v)

    def segment(self):
        return ("turn", round(self.turned))

    def _end(self, ok, reason):
        hw_motors_stop()
        return ok, reason, {"turned_deg": round(self.turned)}

    def step(self, s, now):
        # Accumulate yaw deltas so turns beyond +-180 degrees work across the wrap.
        yaw = hw_yaw()
        delta = yaw - self.last_yaw
        if delta > 180:
            delta -= 360
        elif delta < -180:
            delta += 360
        self.turned += delta
        self.last_yaw = yaw
        if abs(self.turned) >= abs(self.target) - SK_TURN_TOLERANCE_DEG:
            return self._end(True, "done")
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout")
        return None

    def abort(self, reason, s):
        return self._end(False, reason)


MOTION_SKILLS = ("drive", "turn", "drive_until", "follow_line_until", "retrace_path", "gesture")


def make_motion_skill(name, args):
    """Create a motion skill or raise ValueError / NotImplementedError."""
    if name == "drive":
        return DriveSkill(args)
    if name == "drive_until":
        return DriveSkill(args, until=True)
    if name == "turn":
        return TurnSkill(args)
    # TODO: follow_line_until, retrace_path, gesture
    raise NotImplementedError(name)

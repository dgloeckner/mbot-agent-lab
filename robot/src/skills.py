# Skills are small state machines. A motion skill is started once and then
# advanced by step() on every loop iteration; it never blocks.
#
# Goal-oriented skills keep the real-time decisions on the robot: the agent
# says what it wants ("turn until the way is clear", "follow the line to the
# blue field"), the robot closes the loop with filtered sensors at 40 Hz.
#
# start(s) and step(s, now) get the filtered sensor snapshot from sense.py.
# step() returns None while running, or a tuple (ok, reason, data) when done.
#
# Every motion except scan reports its floor trace ("floor"): what the four
# probes saw on the way. The robot reports it and never interprets it.

from compat import ticks_ms, ticks_diff, sleep_ms
from hw import hw_motors, hw_motors_stop, hw_wheels_reset, hw_wheels_deg, CM_PER_WHEEL_DEG, TRACK_CM
from sense import Steady, SN_FLOOR_STEADY, SN_FROM_NAME

SK_DEFAULT_SPEED = 40      # RPM
SK_MAX_SPEED = 80          # RPM, hard limit
SK_TURN_SPEED = 30         # RPM
SK_MIN_SPEED = 8           # RPM; drives and turns slow down to this before the target
SK_DRIVE_KP = 10           # RPM per cm still to go while slowing down (turns: 1 RPM per degree)
SK_SCAN_SPEED = 20         # RPM, slower so every angle gets several readings
SK_LINE_SPEED = 30         # RPM
SK_LINE_MAX_SPEED = 60
SK_LINE_KP = 0.35          # RPM per unit of line offset (-100 .. 100)
SK_LINE_SMOOTH = 0.5       # share of the newest offset reading (1 = no smoothing)
SK_MAX_CM = 300            # per command
SK_MAX_DEG = 720           # per command
SK_MAX_MS = 20000          # per command; the server waits this + 2 s
SK_TURN_TOLERANCE_DEG = 2
SK_RUNON_S = 0.1           # the wheels run on for about this long at full speed after a stop.
                           # VERIFY/calibrate: robot/tools/hwcheck.py prints the run-on
# Body rotation per wheel degree of (left - right): in place each wheel runs on a
# circle with the track width as diameter.
SK_DEG_PER_WHEEL = CM_PER_WHEEL_DEG / TRACK_CM * 57.2958
SK_TURN_RUNON = 12 * SK_RUNON_S * SK_DEG_PER_WHEEL  # degrees per RPM (2 wheels, 6 deg/s per RPM)
SK_ARC_MIN_CM = 10         # arc radius limits; the inner wheel turns forward down to TRACK_CM / 2
SK_ARC_MAX_CM = 200
SK_CONFIRM = 2             # loop iterations a stop condition must hold
SK_CLEAR_CONFIRM = 3       # same for "the way ahead is clear"
SK_LOST_CONFIRM = 8        # same for "the line is gone": long enough to bridge tape gaps
                           # and to let the color vote settle where a line ends in a field
SK_FLOOR_MAX = 20          # floor trace entries per motion; more sets floor_truncated
SK_FLOOR_MIN_CM = 0.5      # a trace entry shorter than this is an edge transition and dropped
SK_FLOOR_MIN_DEG = 4       # the same for turns


def _clamp(v, lo, hi):
    return lo if v < lo else hi if v > hi else v


def _num(args, key, default=None):
    v = args.get(key, default)
    if v is None:
        return None
    if not isinstance(v, (int, float)):
        raise ValueError("%s must be a number" % key)
    return v


def _halt():
    """Stop and wait until the wheels stand still (at most 400 ms), so a
    measurement after it includes the run-on. The motors are off, so this
    short wait may block. The wheels only run on away from the reset."""
    hw_motors_stop()
    last = -9
    for _ in range(16):
        sleep_ms(25)
        left, right = hw_wheels_deg()
        now = abs(left) + abs(right)
        if now - last < 1:
            return
        last = now


def _driven_cm():
    left, right = hw_wheels_deg()
    return (left + right) / 2 * CM_PER_WHEEL_DEG


class Confirm:
    """True once a condition has held for n checks in a row."""

    def __init__(self, n):
        self.n = n
        self.count = 0

    def check(self, cond):
        self.count = self.count + 1 if cond else 0
        return self.count >= self.n


def _color_arg(args, key):
    """A floor color argument: a color name or "any" (anything but white)."""
    color = args.get(key)
    if color is not None and color != "any" and color not in SN_FROM_NAME:
        raise ValueError("unknown color: %s" % color)
    return color


def _sees_color(floor, color):
    """Does one of the probes see the color?"""
    if color == "any":
        for c in floor:
            if c not in "w?":
                return True
        return False
    return SN_FROM_NAME[color] in floor


class Trace:
    """The floor trace of one motion and the closest obstacle on the way.

    An entry [position, pattern] starts where a new floor pattern began, once
    it held SN_FLOOR_STEADY readings. Position counts from the start of the
    motion: cm when driving, degrees (positive right) when turning.

    At the edge of a stripe the probes rarely change together: one sees the
    color a few millimetres before the others. An entry that the next one
    replaces within min_step (cm or degrees) is such a transition and dropped.
    """

    def __init__(self, s, min_step=SK_FLOOR_MIN_CM):
        self.steady = Steady(SN_FLOOR_STEADY)
        self.steady.update(s["floor"], 0)
        self.entries = [[0, s["floor"]]]
        self.min_step = min_step
        self.truncated = False
        self.min_cm = s["distance_cm"]

    def update(self, s, pos):
        if self.steady.update(s["floor_raw"], pos):
            self.add(round(self.steady.tag, 1), self.steady.value)
        if s["distance_cm"] < self.min_cm:
            self.min_cm = s["distance_cm"]

    def add(self, pos, pattern):
        e = self.entries
        if len(e) > 1 and abs(pos - e[-1][0]) < self.min_step:
            e.pop()  # the last entry was an edge transition
            if e[-1][1] == pattern:
                return
        if len(e) < SK_FLOOR_MAX:
            e.append([pos, pattern])
        else:
            self.truncated = True

    def data(self, out):
        out["floor"] = self.entries
        if self.truncated:
            out["floor_truncated"] = True
        out["min_distance_cm"] = self.min_cm
        return out


class Motion:
    """What the motion skills share. Turning skills measure self.deg."""

    forward = False
    deg = 0
    stop_color = None
    armed = False

    def color_hit(self, s):
        """Has the stop color appeared? A color under the robot at the start
        counts only once the probes have left it, so a robot standing in the
        red start field can drive back to red."""
        if self.stop_color is None:
            return False
        if _sees_color(s["floor"], self.stop_color):
            return self.armed
        self.armed = True
        return False

    def segment(self):
        return ("turn", round(self.deg))

    def abort(self, reason, s):
        return self._end(False, reason, s)

    def turned(self):
        """Rotation in degrees since the wheel reset, positive right.

        On the spot and on a curve the body turns by (left - right) wheel
        travel over the track width. The gyro lags and sees nothing on a
        stand; the encoders count what the wheels did. Slip is not seen.
        """
        left, right = hw_wheels_deg()
        self.deg = (left - right) * SK_DEG_PER_WHEEL
        return self.deg

    def toward(self, target, speed):
        """Turn on the spot toward target degrees; True once stopped.

        Slows down over the last degrees and stops early by the run-on
        expected at the speed it turns with, so it comes to rest on target.
        """
        rest = target - self.turned()
        v = _clamp(abs(rest), SK_MIN_SPEED, speed)
        if abs(rest) <= v * SK_TURN_RUNON:
            hw_motors_stop()
            return True
        if rest < 0:
            v = -v
        hw_motors(v, -v)
        return False


class DriveSkill(Motion):
    """drive(cm, speed) and drive_until(max_cm, stop_below_cm, stop_on_color)."""

    name = "drive"
    forward = True

    def __init__(self, args, until=False):
        self.until = until
        if until:
            self.name = "drive_until"
            cm = _num(args, "max_cm")
            self.stop_below = _num(args, "stop_below_cm")
            self.stop_color = _color_arg(args, "stop_on_color")
        else:
            cm = _num(args, "cm")
            self.stop_below = None
        if cm is None:
            raise ValueError("distance missing")
        self.target = _clamp(abs(cm), 0, SK_MAX_CM)
        self.direction = -1 if cm < 0 else 1
        self.forward = self.direction > 0
        self.speed = _clamp(abs(_num(args, "speed", SK_DEFAULT_SPEED)), 5, SK_MAX_SPEED)
        self.driven = 0
        self.wall = Confirm(SK_CONFIRM)

    def start(self, s):
        self.trace = Trace(s)
        hw_wheels_reset()
        self.started = ticks_ms()

    def segment(self):
        return ("drive", round(self.direction * self.driven, 1))

    def _end(self, ok, reason, s):
        _halt()
        self.driven = abs(_driven_cm())
        data = {"driven_cm": round(self.driven, 1), "distance_cm": s["distance_cm"], "color": s["color"]}
        return ok, reason, self.trace.data(data)

    def step(self, s, now):
        self.driven = abs(_driven_cm())
        self.trace.update(s, self.driven)
        rest = self.target - self.driven
        v = _clamp(rest * SK_DRIVE_KP, SK_MIN_SPEED, self.speed)
        if rest <= v * 6 * SK_RUNON_S * CM_PER_WHEEL_DEG:  # the run-on at this speed
            return self._end(True, "max_cm" if self.until else "done", s)
        if self.stop_below is not None and self.forward and self.wall.check(s["distance_cm"] < self.stop_below):
            return self._end(True, "wall", s)
        if self.color_hit(s):
            return self._end(True, "color", s)
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        v *= self.direction
        hw_motors(v, v)
        return None


class TurnSkill(Motion):
    """turn(deg): positive is right, measured with the wheel encoders."""

    name = "turn"
    min_step = SK_FLOOR_MIN_DEG

    def __init__(self, args):
        deg = _num(args, "deg")
        if deg is None:
            raise ValueError("deg missing")
        self.target = _clamp(deg, -SK_MAX_DEG, SK_MAX_DEG)
        self.direction = 1 if deg > 0 else -1

    def start(self, s):
        self.trace = Trace(s, self.min_step)
        hw_wheels_reset()
        self.started = ticks_ms()

    def _end(self, ok, reason, s):
        _halt()
        return ok, reason, self.trace.data({"turned_deg": round(self.turned()), "distance_cm": s["distance_cm"]})

    def there(self):
        return self.toward(self.target, SK_TURN_SPEED)

    def pos(self):
        return self.deg

    def step(self, s, now):
        if self.there():
            return self._end(True, "done", s)
        self.trace.update(s, self.pos())
        if self.color_hit(s):
            return self._end(True, "color", s)
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        return None


class TurnUntilClearSkill(TurnSkill):
    """turn_until_clear(direction, min_cm, min_deg, max_deg).

    Turns at least min_deg (so it looks for a new direction), then keeps
    turning until the way ahead is clear for min_cm, at most max_deg.
    """

    name = "turn_until_clear"

    def __init__(self, args):
        direction = args.get("direction", "right")
        if direction not in ("left", "right"):
            raise ValueError("direction must be left or right")
        self.direction = 1 if direction == "right" else -1
        self.min_cm = _clamp(_num(args, "min_cm", 40), 10, 200)
        self.min_deg = _clamp(_num(args, "min_deg", 20), 0, 180)
        self.max_deg = _clamp(_num(args, "max_deg", 360), 10, SK_MAX_DEG)
        self.clear = Confirm(SK_CLEAR_CONFIRM)

    def step(self, s, now):
        if self.toward(self.direction * self.max_deg, SK_TURN_SPEED):
            return self._end(True, "max_deg", s)
        self.trace.update(s, self.deg)
        if abs(self.deg) >= self.min_deg and self.clear.check(s["distance_cm"] >= self.min_cm):
            return self._end(True, "clear", s)
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        return None


class ScanSkill(Motion):
    """scan(from_deg, to_deg, step_deg, face).

    Turns to from_deg, sweeps slowly to to_deg while measuring, then faces the
    most open direction ("best") or the starting direction ("start").
    Angles are relative to the heading at the start, positive is right.
    """

    name = "scan"

    def __init__(self, args):
        self.from_deg = _clamp(_num(args, "from_deg", -90), -180, 180)
        self.to_deg = _clamp(_num(args, "to_deg", 90), -180, 180)
        if self.to_deg <= self.from_deg:
            raise ValueError("to_deg must be greater than from_deg")
        self.step_deg = _clamp(_num(args, "step_deg", 15), 10, 45)
        self.face = args.get("face", "best")
        if self.face not in ("best", "start"):
            raise ValueError("face must be best or start")
        self.bins = {}
        self.phase = "to_start"
        self.target = self.from_deg
        self.best = None

    def start(self, s):
        hw_wheels_reset()
        self.started = ticks_ms()

    def _profile(self):
        out = []
        n = int((self.to_deg - self.from_deg) // self.step_deg) + 1
        for i in range(n):
            vals = self.bins.get(i)
            cm = int(sorted(vals)[len(vals) // 2]) if vals else None
            out.append([int(self.from_deg + i * self.step_deg), cm])
        return out

    def _end(self, ok, reason, s):
        _halt()
        data = {"turned_deg": round(self.turned()), "distance_cm": s["distance_cm"]}
        if self.best is not None:
            data["profile"] = self._profile()
            data["best_deg"], data["best_cm"] = self.best
        return ok, reason, data

    def step(self, s, now):
        heading = self.turned()
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        if self.phase == "to_start":
            if self.toward(self.from_deg, SK_TURN_SPEED):
                self.phase = "sweep"
            return None
        if self.phase == "sweep":
            raw = s["distance_raw"]
            if raw >= 2:  # valid readings only; the median per angle removes ghosts
                i = int(round((heading - self.from_deg) / self.step_deg))
                if i >= 0:
                    self.bins.setdefault(i, []).append(min(raw, 300))
            if self.toward(self.to_deg, SK_SCAN_SPEED):
                best_deg, best_cm = 0, -1
                for deg, cm in self._profile():
                    if cm is not None and cm > best_cm:
                        best_deg, best_cm = deg, cm
                self.best = (best_deg, best_cm)
                self.target = best_deg if self.face == "best" else 0
                self.phase = "face"
            return None
        if self.toward(self.target, SK_TURN_SPEED):
            return self._end(True, "done", s)
        return None


class FollowLineSkill(DriveSkill):
    """follow_line_until(max_cm, stop_on_color, stop_at_crossing, speed).

    Steers along the line with the quad RGB sensor (P-controller on the line
    offset) until the color, a crossing, the end of the line or max_cm.
    """

    name = "follow_line_until"
    direction = 1

    def __init__(self, args):
        cm = _num(args, "max_cm")
        if cm is None:
            raise ValueError("max_cm missing")
        self.target = _clamp(abs(cm), 1, SK_MAX_CM)
        self.stop_color = _color_arg(args, "stop_on_color")
        self.stop_crossing = bool(args.get("stop_at_crossing", True))
        self.speed = _clamp(abs(_num(args, "speed", SK_LINE_SPEED)), 10, SK_LINE_MAX_SPEED)
        self.lost = Confirm(SK_LOST_CONFIRM)
        self.crossing = Confirm(SK_CONFIRM)
        self.driven = 0
        self.offset = 0

    def step(self, s, now):
        self.driven = abs(_driven_cm())
        self.trace.update(s, self.driven)
        if self.color_hit(s):
            return self._end(True, "color", s)
        if self.driven >= self.target:
            return self._end(True, "max_cm", s)
        if self.stop_crossing and self.driven > 3 and self.crossing.check(s["line"] == "all"):
            return self._end(True, "crossing", s)
        if self.lost.check(s["line"] == "none"):
            return self._end(True, "line_lost", s)
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        self.offset += SK_LINE_SMOOTH * (_clamp(s["line_offset"], -100, 100) - self.offset)
        turn = SK_LINE_KP * self.offset
        hw_motors(self.speed + turn, self.speed - turn)
        return None


def _norm(deg):
    """An angle in -180 .. 180."""
    return (deg + 180) % 360 - 180


def _sin(deg):
    """Sine of an angle in degrees (Bhaskara I, error below 0.002): no math module."""
    d = _norm(deg)
    p = abs(d) * (180 - abs(d))
    return 4 * p / (40500 - p) * (1 if d > 0 else -1)


class RetraceSkill(Motion):
    """retrace_path(): drive the trip log back to where it began.

    The log is the way from its start to here. A turn at its end is folded
    into self.turned (the robot's rotation on top of the logged way). Before a
    drive the robot faces back (turned 180) and drives the same distance
    forward, so the obstacle reflex stays on. Sub-steps are ordinary drive and
    turn skills and never reach the log. The skill works the log off in place:
    done leaves it empty, an abort leaves the way still to go plus a turn for
    self.turned, so a new retrace_path continues from here.
    """

    name = "retrace_path"
    speed = SK_DEFAULT_SPEED

    def __init__(self, log):
        self.log = log
        self.empty = not log
        self.turned = 0
        self.segments = 0
        self.driven = 0
        self.sub = None
        self.started = 0

    def start(self, s):
        self.started = ticks_ms()

    def segment(self):
        return ("drive", 0)  # the log is already up to date

    def _next(self, s):
        """Start the next sub-step; False when the log is worked off."""
        log = self.log
        while log and log[-1][0] == "turn":
            self.turned += log.pop()[1]
            self.segments += 1
        if not log:
            return False
        deg = _norm(180 - self.turned)
        if abs(deg) > SK_TURN_TOLERANCE_DEG:
            self.sub = TurnSkill({"deg": deg})
        else:
            self.sub = DriveSkill({"cm": log[-1][1]})
        self.forward = self.sub.forward
        self.sub.start(s)
        return True

    def _apply(self, s, reason):
        """End the sub-step and take what it did off the log."""
        if reason:
            self.sub.abort(reason, s)
        kind, value = self.sub.segment()
        if kind == "turn":
            self.turned += value
        else:
            self.driven += abs(value)
            rest = round(self.log[-1][1] - value, 1)
            if abs(rest) < 1:
                self.log.pop()
                self.segments += 1
            else:
                self.log[-1] = ("drive", rest)
        self.sub = None
        self.forward = False

    def _end(self, ok, reason, s):
        hw_motors_stop()
        turned = _norm(round(self.turned))
        if self.log and turned:
            self.log.append(("turn", turned))
        return ok, reason, {"segments": self.segments, "driven_cm": round(self.driven, 1),
                            "distance_cm": s["distance_cm"]}

    def step(self, s, now):
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self.abort("timeout", s)
        if self.sub:
            r = self.sub.step(s, now)
            if r is None:
                return None
            self._apply(s, None)
            if not r[0]:
                return self._end(False, r[1], s)
        if self._next(s):
            return None
        return self._end(True, "empty" if self.empty else "done", s)

    def abort(self, reason, s):
        if self.sub:
            self._apply(s, reason)
        return self._end(False, reason, s)


class ArcSkill(TurnSkill):
    """drive_arc(radius_cm, deg, stop_on_color, speed): drive forward along a circle.

    deg is the heading change, positive curves right; it ends when the wheel
    encoders say so, like turn, or at stop_on_color like drive_until. The floor
    trace counts cm along the way, like drive: stripe widths stay readable on
    any radius. radius_cm is to the circle's center, speed the speed of the
    robot's center. The trip log gets the arc as its chord (turn half, drive,
    turn half), so retrace_path drives a straight line back.
    """

    name = "drive_arc"
    forward = True
    min_step = SK_FLOOR_MIN_CM

    def __init__(self, args, log):
        TurnSkill.__init__(self, args)
        r = _num(args, "radius_cm")
        if r is None:
            raise ValueError("radius_cm missing")
        self.r = _clamp(r, SK_ARC_MIN_CM, SK_ARC_MAX_CM)
        self.speed = _clamp(abs(_num(args, "speed", SK_DEFAULT_SPEED)), 5, SK_MAX_SPEED)
        self.stop_color = _color_arg(args, "stop_on_color")
        self.log = log

    def start(self, s):
        TurnSkill.start(self, s)
        k = TRACK_CM / 2 / self.r
        v = min(self.speed, SK_MAX_SPEED / (1 + k))
        self.runon = v * k * SK_TURN_RUNON  # the wheels differ by 2 v k
        k *= self.direction
        hw_motors(v * (1 + k), v * (1 - k))

    def there(self):
        """Stop early by the expected run-on; no slowing down on a curve."""
        if (self.target - self.turned()) * self.direction > self.runon:
            return False
        hw_motors_stop()
        return True

    def pos(self):
        return abs(_driven_cm())

    def segment(self):
        return ("drive", 0)  # _end logs the arc

    def _end(self, ok, reason, s):
        r = TurnSkill._end(self, ok, reason, s)
        r[2]["color"] = s["color"]
        r[2]["driven_cm"] = cm = round(self.pos(), 1)
        h = self.deg
        if abs(h) > 1:
            cm = abs(cm * 114.6 * _sin(h / 2) / h)  # chord = 2 r sin(h / 2), r = cm / h in radians
        h = round(h / 2)
        for e in (("turn", h), ("drive", round(cm, 1)), ("turn", h)):
            if e[1]:
                self.log.append(e)
        return r


MOTION_SKILLS = ("drive", "turn", "drive_until", "turn_until_clear", "scan", "follow_line_until",
                 "retrace_path", "drive_arc", "gesture")


def make_motion_skill(name, args, log=None):
    """Create a motion skill or raise ValueError / NotImplementedError.

    log is the robot's trip log; retrace_path works it off in place.
    """
    if name == "drive":
        return DriveSkill(args)
    if name == "drive_until":
        return DriveSkill(args, until=True)
    if name == "turn":
        return TurnSkill(args)
    if name == "turn_until_clear":
        return TurnUntilClearSkill(args)
    if name == "scan":
        return ScanSkill(args)
    if name == "follow_line_until":
        return FollowLineSkill(args)
    if name == "retrace_path":
        return RetraceSkill(log)
    if name == "drive_arc":
        return ArcSkill(args, log)
    # TODO: gesture
    raise NotImplementedError(name)

# Color calibration: maintenance from the control room, not a tool for agents.
#
# The robot stands right before the reference strip (white, black, red,
# yellow, green, blue, 5 cm each) and drives slowly across it, summing the
# RGB readings of each probe in 0.5 cm bins. Afterwards it finds the black
# field (the darkest 5 cm) and takes the middle of every field as that color's
# reference. So the start position needs no precision, only "just before the
# white field". The references go to sense.py at once and back in the result,
# so the server can store and resend them.

from compat import ticks_ms, ticks_diff
from hw import hw_motors, hw_motors_stop, hw_wheels_reset, PROBES
from sense import SN_LETTERS
from skills import _driven_cm, _num, _clamp, SK_MAX_MS

CB_SPEED = 20           # RPM, slow so each field gets many readings
CB_BIN_CM = 0.5
CB_FIELD_CM = 5         # length of each reference field
CB_MARGIN_CM = 1.5      # ignore this much at both edges of a field
CB_DRIVE_CM = 38        # six fields plus room before and after


class CalibrateColorsSkill:
    name = "calibrate_colors"
    forward = True
    speed = CB_SPEED

    def __init__(self, args, sense):
        self.sense = sense
        self.target = _clamp(_num(args, "cm", CB_DRIVE_CM), 32, 60)
        self.bins = {}
        self.driven = 0
        self.started = 0

    def start(self, s):
        hw_wheels_reset()
        self.started = ticks_ms()
        hw_motors(CB_SPEED, CB_SPEED)

    def segment(self):
        return ("drive", round(self.driven, 1))

    def step(self, s, now):
        self.driven = abs(_driven_cm())
        b = int(self.driven / CB_BIN_CM)
        acc = self.bins.get(b)
        if acc is None:
            acc = [0] * 13
            self.bins[b] = acc
        acc[0] += 1
        i = 1
        for rgb in self.sense.probe_rgbs():
            for v in rgb:
                acc[i] += v
                i += 1
        if self.driven >= self.target:
            hw_motors_stop()
            return self.finish(s)
        if ticks_diff(now, self.started) > SK_MAX_MS:
            return self._end(False, "timeout", s)
        return None

    def _end(self, ok, reason, s, data=None, error=None):
        hw_motors_stop()
        out = {"driven_cm": round(self.driven, 1)}
        if data:
            out.update(data)
        if error:
            out["error"] = error
        return ok, reason, out

    def _brightness(self, b):
        acc = self.bins.get(b)
        if not acc:
            return None
        return sum(acc[1:]) / acc[0]

    def finish(self, s):
        per_field = int(CB_FIELD_CM / CB_BIN_CM)
        last = max(self.bins) if self.bins else 0
        # The black field: the darkest window of one field length.
        dark, dark_at = None, None
        for start in range(0, last - per_field + 2):
            vals = [self._brightness(b) for b in range(start, start + per_field)]
            if None in vals:
                continue
            m = sum(vals) / per_field
            if dark is None or m < dark:
                dark, dark_at = m, start
        if dark_at is None:
            return self._end(False, "error", s, error="reference strip not found")
        refs = {}
        for p in PROBES:
            refs[p] = []
        for field in range(len(SN_LETTERS)):
            first = dark_at + (field - 1) * per_field + int(CB_MARGIN_CM / CB_BIN_CM)
            sums, n = [0] * 12, 0
            for b in range(first, dark_at + field * per_field - int(CB_MARGIN_CM / CB_BIN_CM)):
                acc = self.bins.get(b)
                if not acc:
                    continue
                n += acc[0]
                for i in range(12):
                    sums[i] += acc[i + 1]
            if n == 0:
                return self._end(False, "error", s, error="reference strip not found")
            for k, p in enumerate(PROBES):
                refs[p].append([int(sums[3 * k + j] / n) for j in range(3)])
        white, black = sum(refs["L1"][0]), sum(refs["L1"][1])
        if white < 2 * black:
            return self._end(False, "error", s, error="white and black too similar")
        self.sense.set_refs(refs)
        return self._end(True, "done", s, {"refs": refs})

    def abort(self, reason, s):
        return self._end(False, reason, s)

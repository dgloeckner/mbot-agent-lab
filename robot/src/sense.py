# Sensor filtering. Every decision on the robot (skills, reflexes, events,
# state reports) uses these filtered values, never a single raw reading:
# ultrasonic sensors produce dropouts and ghost echoes, the color probes
# jitter at tape edges, the gyro twitches on a bump, and a motor needs more
# power for a moment whenever it speeds up.
#
# The floor: each of the four probes (L2, L1, R1, R2) is classified into a
# letter, w white, k black, r red, y yellow, g green, b blue, ? unclear. The
# four letters from left to right are the floor pattern, e.g. "rkkr". A new
# pattern counts only after it held for SN_FLOOR_STEADY readings in a row.
# The robot never interprets patterns; it reports them.
#
# Held up: what the ultrasonic sensor does not see shows in two ways. The motors
# need more power than their speed asks for ("held"): the wheels keep turning,
# the speed control sees to that, but they slip or push. Or the robot climbs
# with its nose up ("nose_up"). The wheel encoders show neither.

from compat import ticks_ms, ticks_diff
from hw import (hw_distance_cm, hw_probe_rgb, hw_probe_name, hw_line_offset, hw_yaw,
                hw_tilt, hw_battery, hw_button, hw_loudness, hw_power, hw_cmd, PROBES)

SN_DIST_WINDOW = 7      # median over the last valid distance readings: 4 ghost echoes to fool it
SN_DIST_MIN = 2         # readings below are dropouts (the sensor reports 0 on errors)
SN_DIST_MAX = 300       # the sensor's range; nothing in sight reads as this
SN_DIST_BLIND = 10      # dropouts in a row: blind (too close, unplugged), reads 0. VERIFY: open space reads 300, not 0
SN_FLOOR_STEADY = 3     # equal floor patterns in a row before the pattern changes
SN_TILT_ON_DEG = 45     # tilted when pitch or roll goes beyond this ...
SN_TILT_OFF_DEG = 30    # ... and level again only when both are below this
SN_TILT_STABLE = 4      # readings in a row before the tilt state flips (~100 ms)
# Measured on one robot on 2026-11-27: docs/measurements.md, "Blocked".
SN_NOSE_ON_DEG = 8      # nose up when the pitch rises this far above its level at rest ...
SN_NOSE_OFF_DEG = 4     # ... and down again below this
SN_NOSE_MS = 300        # the nose state flips after this long: speeding up lifts it for 200 ms
SN_LEVEL_SMOOTH = 32    # at rest the level follows the pitch over about this many readings
SN_PWR_BASE = 3.2       # motor power in free driving: this ...
SN_PWR_PER_RPM = 0.41   # ... plus this per RPM, forward, backward and in turns
SN_PWR_OVER = 4         # held when the mean of both motors is this far above it ...
SN_PWR_OVER_PART = 0.1  # ... plus this share of it (a low battery needs more for the same speed) ...
SN_HELD_MS = 400        # ... for this long: speeding up needs more for 200 ms

# Color classification against reference values per probe (calibrate_colors).
SN_LETTERS = "wkrygb"   # order of the reference values
SN_NAMES = {"w": "white", "k": "black", "r": "red", "y": "yellow", "g": "green", "b": "blue"}
SN_FROM_NAME = {"white": "w", "black": "k", "red": "r", "yellow": "y", "green": "g", "blue": "b", "cyan": "b"}
SN_HUE_DAMP = 30        # keeps the hue of dark readings from exploding
SN_HUE_WEIGHT = 4.0     # hue counts more than brightness: shadows change brightness
SN_MAX_DIST = 0.35      # farther than this from every reference: "?" ...
SN_MIX_MAX = 30         # ... unless within this RGB distance of a mix of two references


def sn_median(values):
    ordered = sorted(values)
    return ordered[len(ordered) // 2]


def sn_feature(rgb):
    """Hue and brightness of a reading, separated so shadows disturb less."""
    r, g, b = rgb
    total = r + g + b + 3 * SN_HUE_DAMP
    return ((r + SN_HUE_DAMP) / total * SN_HUE_WEIGHT,
            (g + SN_HUE_DAMP) / total * SN_HUE_WEIGHT,
            (r + g + b) / 765)


def sn_classify(rgb, refs):
    """Letter of the nearest reference, or "?" if none is near enough.

    refs is a list of (letter, feature, rgb). At a tape edge the probe's spot
    sees two surfaces at once; a reading on the mix line between two
    references counts as the one it is closer to.
    """
    feature = sn_feature(rgb)
    best, best_d = "?", SN_MAX_DIST * SN_MAX_DIST
    for letter, ref, _ in refs:
        d = 0
        for i in range(3):
            d += (feature[i] - ref[i]) ** 2
        if d < best_d:
            best, best_d = letter, d
    if best == "?":
        best_d = SN_MIX_MAX * SN_MIX_MAX
        for a in range(len(refs)):
            for b in range(a + 1, len(refs)):
                t, d = sn_mix(rgb, refs[a][2], refs[b][2])
                if d < best_d:
                    best, best_d = refs[a][0] if t < 0.5 else refs[b][0], d
    return best


def sn_mix(x, a, b):
    """Share t of b in the mix of a and b nearest to x, and the squared distance."""
    ab = [b[i] - a[i] for i in range(3)]
    n = ab[0] * ab[0] + ab[1] * ab[1] + ab[2] * ab[2]
    if n == 0:
        return 0, 1e9
    t = ((x[0] - a[0]) * ab[0] + (x[1] - a[1]) * ab[1] + (x[2] - a[2]) * ab[2]) / n
    t = 0 if t < 0 else 1 if t > 1 else t
    d = 0
    for i in range(3):
        d += (x[i] - a[i] - t * ab[i]) ** 2
    return t, d


class Steady:
    """A value that changes only after n equal readings in a row.

    tag remembers something about the first of those readings (e.g. the
    position), so a trace can say where a change began, not where it was
    confirmed.
    """

    def __init__(self, n):
        self.n = n
        self.value = None
        self.tag = None
        self.cand = None
        self.cand_tag = None
        self.count = 0

    def update(self, v, tag=None):
        """Feed one reading; True when the value changed."""
        if v == self.value:
            self.cand = None
            self.count = 0
            return False
        if v != self.cand:
            self.cand, self.cand_tag, self.count = v, tag, 0
        self.count += 1
        if self.value is None or self.count >= self.n:
            self.value, self.tag = v, self.cand_tag
            self.cand, self.count = None, 0
            return True
        return False


class Flip:
    """An on/off state that flips only after n readings in a row ask for it."""

    def __init__(self, n):
        self.n = n
        self.on = False
        self.count = 0

    def update(self, want):
        if want == self.on:
            self.count = 0
        else:
            self.count += 1
            if self.count >= self.n:
                self.on = want
                self.count = 0
        return self.on


class Sense:
    def __init__(self):
        self.dist_window = []
        self.dist_raw = SN_DIST_MAX
        self.dropouts = 0
        self.refs = None          # per probe: [(letter, feature, rgb)], None = built-in names
        self.floor = Steady(SN_FLOOR_STEADY)
        self.color = "white"
        self.tilted = Flip(SN_TILT_STABLE)
        self.nose_on = False
        self.nose_t = None        # since when the pitch asks for the other nose state
        self.pitches = []         # the last three pitch readings
        self.level = None         # pitch at rest
        self.stops = 0            # hw_cmd[2] when the power check last looked
        self.pwr = [0, 0]         # per motor: how far its last reading lay above the allowed power
        self.pwr_i = 0            # the motor read last
        self.pwr_n = 0            # readings since the motors started
        self.held_t = None        # since when the motors need too much

    def set_refs(self, refs):
        """Reference RGB per probe: {"L2": [[r,g,b] x 6 in SN_LETTERS order], ...}, or None."""
        if refs is None:
            self.refs = None
            return
        out = []
        for p in PROBES:
            values = refs[p]
            if len(values) != len(SN_LETTERS):
                raise ValueError("refs need %d colors per probe" % len(SN_LETTERS))
            out.append([(SN_LETTERS[i], sn_feature(values[i]), values[i]) for i in range(len(SN_LETTERS))])
        self.refs = out

    def probe_rgbs(self):
        return [hw_probe_rgb(p) for p in PROBES]

    def raw_pattern(self):
        """Floor pattern from single readings, before the steady filter."""
        if self.refs is None:
            return "".join(SN_FROM_NAME.get(hw_probe_name(p), "?") for p in PROBES)
        return "".join(sn_classify(hw_probe_rgb(p), self.refs[i]) for i, p in enumerate(PROBES))

    def distance(self, raw):
        """Median of the last valid readings; single dropouts do not count.

        Many dropouts in a row (also right after the start) mean the sensor
        sees nothing usable: then the distance reads 0, so forward motions
        stop instead of trusting an old or missing reading.
        """
        self.dist_raw = raw
        if raw >= SN_DIST_MIN:
            self.dropouts = 0
            self.dist_window.append(min(raw, SN_DIST_MAX))
            if len(self.dist_window) > SN_DIST_WINDOW:
                self.dist_window.pop(0)
        else:
            self.dropouts += 1
            if self.dropouts >= SN_DIST_BLIND:
                self.dist_window = []
        if not self.dist_window:
            return SN_DIST_MAX if self.dropouts < SN_DIST_BLIND else 0
        return int(sn_median(self.dist_window))

    def tilt(self, pitch, roll):
        """Tilt state with hysteresis and a minimum duration."""
        t = self.tilted
        lim = SN_TILT_OFF_DEG if t.on else SN_TILT_ON_DEG
        return t.update(abs(pitch) > lim or abs(roll) > lim)

    def nose(self, pitch, moving, now):
        """Nose-up state while the motors run, with hysteresis and a minimum duration.

        The pitch is the median of the last three readings and counts against
        its level at rest, so the CyberPi's mounting
        angle and a sloped floor do not matter. At rest the level follows the
        pitch and the state is off; while the motors run the level holds.
        """
        w = self.pitches
        w.append(pitch)
        if len(w) > 3:
            w.pop(0)
        pitch = sn_median(w)  # a bump is one reading far off
        if self.level is None:
            self.level = pitch
        up = pitch - self.level
        if not moving:
            self.level += up / SN_LEVEL_SMOOTH
            self.nose_on, self.nose_t = False, None
            return False
        if (up > (SN_NOSE_OFF_DEG if self.nose_on else SN_NOSE_ON_DEG)) == self.nose_on:
            self.nose_t = None
        elif self.nose_t is None:
            self.nose_t = now
        elif ticks_diff(now, self.nose_t) >= SN_NOSE_MS:
            self.nose_on, self.nose_t = not self.nose_on, None
        return self.nose_on

    def held(self, now):
        """Do the motors work against something that holds the robot?

        The speed control keeps the wheels turning whatever holds the robot, so
        the encoders show nothing; the power it needs for that does. Free
        driving needs SN_PWR_BASE + SN_PWR_PER_RPM * speed. Reads one motor per
        call, in turns (a reading is slow), judges each reading against the
        speed its motor was told then, and takes the mean of both: held when
        that stays too high for SN_HELD_MS. Every motor stop starts anew.
        """
        cmd = hw_cmd
        if cmd[2] != self.stops or not (cmd[0] or cmd[1]):
            self.stops = cmd[2]
            self.pwr_n = 0
            self.held_t = None
            return False
        i = self.pwr_i = 1 - self.pwr_i
        p = hw_power(i)
        # Against the speed at the time of the reading: the other motor's is one look old.
        free = SN_PWR_BASE + SN_PWR_PER_RPM * abs(cmd[i])
        self.pwr[i] = (p if cmd[i] >= 0 else -p) - free - SN_PWR_OVER_PART * free
        self.pwr_n += 1
        if self.pwr_n < 2 or self.pwr[0] + self.pwr[1] <= 2 * SN_PWR_OVER:
            self.held_t = None
            return False
        if self.held_t is None:
            self.held_t = now
        return ticks_diff(now, self.held_t) >= SN_HELD_MS

    def read(self):
        now = ticks_ms()
        pitch, roll = hw_tilt()
        raw = self.raw_pattern()
        self.floor.update(raw)
        floor = self.floor.value
        # The floor color is what both outer probes agree on; L1 and R1 sit on
        # a line while following it, the outer ones see stripes and fields.
        if floor[0] == floor[3] and floor[0] != "?":
            self.color = SN_NAMES[floor[0]]
        return {
            "distance_cm": self.distance(hw_distance_cm()),
            "distance_raw": self.dist_raw,
            "floor": floor,
            "floor_raw": raw,
            "color": self.color,
            "line": sn_line(floor),
            "line_offset": hw_line_offset(),
            "yaw": int(hw_yaw()),
            "battery": int(hw_battery()),
            "tilted": self.tilt(pitch, roll),
            "nose_up": self.nose(pitch, hw_cmd[0] != 0 or hw_cmd[1] != 0, now),
            "held": self.held(now),
            "button_a": hw_button("a"),
            "button_b": hw_button("b"),
            "loudness": hw_loudness(),
        }


def sn_line(floor):
    """Where black is under the probes: none, left, center, right or all."""
    if "k" not in floor:
        return "none"
    if floor == "kkkk":
        return "all"
    if floor[1] == "k" or floor[2] == "k":
        return "center"
    return "left" if floor[0] == "k" else "right"

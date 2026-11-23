# Sensor filtering. Every decision on the robot (skills, reflexes, events,
# state reports) uses these filtered values, never a single raw reading:
# ultrasonic sensors produce dropouts and ghost echoes, the color probes
# jitter at tape edges, and the gyro twitches on a bump.
#
# The floor: each of the four probes (L2, L1, R1, R2) is classified into a
# letter, w white, k black, r red, y yellow, g green, b blue, ? unclear. The
# four letters from left to right are the floor pattern, e.g. "rkkr". A new
# pattern counts only after it held for SN_FLOOR_STEADY readings in a row.
# The robot never interprets patterns; it reports them.

from hw import (hw_distance_cm, hw_probe_rgb, hw_probe_name, hw_line_offset, hw_yaw,
                hw_tilt, hw_battery, hw_button, hw_loudness, PROBES)

SN_DIST_WINDOW = 7      # median over the last valid distance readings: 4 ghost echoes to fool it
SN_DIST_MIN = 2         # readings below are dropouts (the sensor reports 0 on errors)
SN_DIST_MAX = 300       # the sensor's range; nothing in sight reads as this
SN_DIST_BLIND = 10      # dropouts in a row: blind (too close, unplugged), reads 0. VERIFY: open space reads 300, not 0
SN_FLOOR_STEADY = 3     # equal floor patterns in a row before the pattern changes
SN_TILT_ON_DEG = 45     # tilted when pitch or roll goes beyond this ...
SN_TILT_OFF_DEG = 30    # ... and level again only when both are below this
SN_TILT_STABLE = 4      # readings in a row before the tilt state flips (~100 ms)

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


class Sense:
    def __init__(self):
        self.dist_window = []
        self.dist_raw = SN_DIST_MAX
        self.dropouts = 0
        self.refs = None          # per probe: [(letter, feature, rgb)], None = built-in names
        self.floor = Steady(SN_FLOOR_STEADY)
        self.color = "white"
        self.tilted = False
        self.tilt_count = 0

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
        if self.tilted:
            want = abs(pitch) > SN_TILT_OFF_DEG or abs(roll) > SN_TILT_OFF_DEG
        else:
            want = abs(pitch) > SN_TILT_ON_DEG or abs(roll) > SN_TILT_ON_DEG
        if want == self.tilted:
            self.tilt_count = 0
        else:
            self.tilt_count += 1
            if self.tilt_count >= SN_TILT_STABLE:
                self.tilted = want
                self.tilt_count = 0
        return self.tilted

    def read(self):
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

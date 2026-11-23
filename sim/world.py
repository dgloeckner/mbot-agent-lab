"""2D world for the simulator: walls, the floor drawing and the robot pose.

The floor is a drawing on a white base: rectangular "fields" at the bottom,
then "bands" (strips of tape: from, to, width, color, layer) on top, higher
layers over lower ones. Legacy "lines" (black polylines with round joints) sit
on the top layer. The world knows centimetres only, never what a drawing means.

The quad RGB sensor has four probes (L2, L1, R1, R2). Each sees a small spot,
not a point, so at edges its RGB value mixes both sides.

With noise enabled (spec "noise": true or a random.Random passed in), the
sensors misbehave like real ones: ultrasonic dropouts (0) and ghost echoes,
color and line readings that jitter at edges, and gyro twitches on bumps.
The robot script must cope with that; the tests check it does.

The motors do not follow a speed change at once: the wheel speed follows the
command with a lag (motor_lag_s in mbot2.json; with noise it differs per
robot), so after a stop the wheels run on. With "stand": true the robot sits
on a stand: the wheels and encoders turn, the body does not move.

Units are centimetres and degrees. Heading 0 points along +y, positive headings
turn clockwise (to the right), matching the robot's yaw convention.
Time advances lazily: every sensor read or motor change integrates the motion
since the previous call, using wall-clock time.
"""

import json
import math
import pathlib
import random
import time

# Robot geometry: assumptions until measured on the real robot (sim/mbot2.json).
GEOMETRY = json.loads((pathlib.Path(__file__).resolve().parent / "mbot2.json").read_text())
WHEEL_DIAMETER_CM = GEOMETRY["wheel_diameter_cm"]
CM_PER_WHEEL_DEG = math.pi * WHEEL_DIAMETER_CM / 360
TRACK_CM = GEOMETRY["track_cm"]                    # distance between the wheels
ROBOT_RADIUS_CM = GEOMETRY["robot_radius_cm"]
SENSOR_FRONT_CM = GEOMETRY["ultrasonic_front_cm"]  # ultrasonic sensor ahead of the axle
GROUND_FRONT_CM = GEOMETRY["probe_front_cm"]       # quad RGB sensor bar ahead of the axle ("o")
PROBES = ("L2", "L1", "R1", "R2")
PROBE_LATERAL_CM = tuple(GEOMETRY["probe_lateral_cm"][p] for p in PROBES)
LINE_OFFSETS_CM = PROBE_LATERAL_CM
PROBE_SPOT_CM = GEOMETRY["probe_spot_cm"]
MOTOR_LAG_S = GEOMETRY["motor_lag_s"]  # time constant of the wheel speed
MOTOR_SNAP_RPM = 2       # below this a stopped motor stands still
PROBE_JITTER_CM = 0.2    # the bar is fixed; only the edge blur wobbles
PROBE_SAMPLE_S = 0.004   # one RGB measurement serves the three channel reads
MAX_RANGE_CM = 300

# What each surface reflects under the sensor's fill light (0..255). "blue" is
# the light blue tape of the line field.
SURFACE_RGB = {
    "white": (235, 235, 228), "black": (28, 28, 30), "red": (205, 60, 55),
    "yellow": (228, 192, 48), "green": (70, 160, 72), "blue": (82, 166, 218),
    "cyan": (60, 200, 205), "purple": (140, 80, 190),
}
RGB_NOISE = 5.0          # standard deviation per channel


class World:
    def __init__(self, spec, robot_id="mbot-sim", clock=time.monotonic, noise=None):
        self.robot_id = robot_id
        if noise is None and spec.get("noise"):
            noise = random.Random()
        self.rng = noise
        self.walls = [tuple(w) for w in spec.get("walls", [])]
        self.fields = spec.get("fields", [])
        self.lines = spec.get("lines", [])
        self.floor = spec.get("floor", "white")
        self.bands = _bands(spec)
        self.stand = bool(spec.get("stand"))
        self.probe_cache = {}
        # Color cast and brightness of this robot's sensor, fixed per robot.
        self.tint = (1.0, 1.0, 1.0)
        if self.rng:
            gain = self.rng.uniform(0.85, 1.05)
            self.tint = tuple(gain * self.rng.uniform(0.92, 1.08) for _ in range(3))
        self.motor_lag = MOTOR_LAG_S
        if self.rng:
            self.motor_lag *= self.rng.uniform(0.7, 1.3)
        start = spec.get("start", {"x": 0, "y": 0, "heading": 0})
        self.x, self.y, self.heading = float(start["x"]), float(start["y"]), float(start.get("heading", 0))
        self.pitch = 0.0
        self.roll = 0.0
        self.left_rpm = 0.0      # commanded
        self.right_rpm = 0.0
        self.wheel_rpm = [0.0, 0.0]  # what the wheels actually do
        self.left_deg = 0.0
        self.right_deg = 0.0
        self.yaw_zero = self.heading
        self.bumped = False
        self.clock = clock
        self.t = clock()

    @classmethod
    def load(cls, path, robot_id="mbot-sim", start_index=0, noise=None):
        with open(path, encoding="utf-8") as f:
            spec = json.load(f)
        starts = spec.get("starts")
        if starts:
            spec["start"] = starts[start_index % len(starts)]
        return cls(spec, robot_id, noise=noise)

    # ---- motion -----------------------------------------------------------

    def update(self):
        now = self.clock()
        dt = now - self.t
        self.t = now
        if dt <= 0:
            return
        # Integrate in small steps so collisions are not tunnelled through.
        steps = max(1, int(dt / 0.01))
        h = dt / steps
        for _ in range(steps):
            self._step(h)

    def _step(self, dt):
        k = min(1.0, dt / self.motor_lag) if self.motor_lag > 0 else 1.0
        w = self.wheel_rpm
        for i, cmd in enumerate((self.left_rpm, self.right_rpm)):
            w[i] += (cmd - w[i]) * k
            if cmd == 0 and abs(w[i]) < MOTOR_SNAP_RPM:
                w[i] = 0.0
        dl_deg = w[0] * 6 * dt     # rpm * 360 / 60
        dr_deg = w[1] * 6 * dt
        self.left_deg += dl_deg
        self.right_deg += dr_deg
        if self.stand:
            return
        dl, dr = dl_deg * CM_PER_WHEEL_DEG, dr_deg * CM_PER_WHEEL_DEG
        d = (dl + dr) / 2
        dtheta = math.degrees((dl - dr) / TRACK_CM)
        mid = math.radians(self.heading + dtheta / 2)
        nx, ny = self.x + d * math.sin(mid), self.y + d * math.cos(mid)
        if self._collides(nx, ny):
            # Blocked: wheels slip, the pose only rotates.
            self.bumped = True
            nx, ny = self.x, self.y
        self.x, self.y = nx, ny
        self.heading = (self.heading + dtheta) % 360

    def _collides(self, x, y):
        return any(_point_segment_dist(x, y, w) < ROBOT_RADIUS_CM for w in self.walls)

    def set_motors(self, left_rpm, right_rpm):
        self.update()
        self.left_rpm, self.right_rpm = float(left_rpm), float(right_rpm)

    def reset_wheels(self):
        self.update()
        self.left_deg = self.right_deg = 0.0

    def wheel_deg(self):
        self.update()
        return self.left_deg, self.right_deg

    def yaw(self):
        self.update()
        jitter = self.rng.gauss(0, 0.3) if self.rng else 0
        return _wrap180(self.heading - self.yaw_zero + jitter)

    def tilt_reading(self):
        """Pitch and roll as the gyro reports them: small jitter, rare spikes."""
        pitch, roll = self.pitch, self.roll
        if self.rng:
            pitch += self.rng.gauss(0, 1.5)
            roll += self.rng.gauss(0, 1.5)
            if self.rng.random() < 0.01:  # a bump: one reading far off
                pitch += self.rng.choice((-1, 1)) * 55
        return pitch, roll

    def reset_yaw(self):
        self.update()
        self.yaw_zero = self.heading

    # ---- sensors ------------------------------------------------------------

    def _ahead(self, dist, lateral=0.0):
        r = math.radians(self.heading)
        return (self.x + dist * math.sin(r) + lateral * math.cos(r),
                self.y + dist * math.cos(r) - lateral * math.sin(r))

    def distance_cm(self):
        self.update()
        ox, oy = self._ahead(SENSOR_FRONT_CM)
        r = math.radians(self.heading)
        dx, dy = math.sin(r), math.cos(r)
        best = MAX_RANGE_CM
        for w in self.walls:
            t = _ray_segment(ox, oy, dx, dy, w)
            if t is not None and t < best:
                best = t
        if self.rng:
            p = self.rng.random()
            if p < 0.04:
                return 0.0  # dropout: the real sensor reports 0 on a failed echo
            if p < 0.06:
                return round(self.rng.uniform(3, 15), 1)  # ghost echo
            if best < MAX_RANGE_CM:
                best = max(2.0, best + self.rng.gauss(0, 0.8))
        return round(best, 1)

    def _jitter(self, x, y, amount=1.2):
        """Sensor spot wobble: readings at edges flicker between both sides.

        The legacy single color reading flickers more (1.2 cm) than the probes,
        which sit in a fixed bar and only see the edge blur.
        """
        if self.rng:
            return x + self.rng.uniform(-amount, amount), y + self.rng.uniform(-amount, amount)
        return x, y

    def surface(self, x, y):
        """Color name of the floor drawing at a point."""
        for b in self.bands:
            if _in_band(x, y, b):
                return b["color"]
        for f in reversed(self.fields):
            fx, fy, fw, fh = f["rect"]
            if fx <= x <= fx + fw and fy <= y <= fy + fh:
                return f["color"]
        return self.floor

    def probe_point(self, probe):
        """Centre of a probe's spot in world coordinates."""
        return self._ahead(GROUND_FRONT_CM, PROBE_LATERAL_CM[_probe_index(probe)])

    def _spot_rgb(self, probe):
        """Mean surface RGB over the probe's spot, without sensor effects."""
        x, y = self._jitter(*self.probe_point(probe), amount=PROBE_JITTER_CM)
        r = PROBE_SPOT_CM
        pts = ((x, y), (x + r, y), (x - r, y), (x, y + r), (x, y - r))
        acc = [0.0, 0.0, 0.0]
        for px, py in pts:
            rgb = SURFACE_RGB.get(self.surface(px, py), SURFACE_RGB["white"])
            for i in range(3):
                acc[i] += rgb[i]
        return [v / len(pts) for v in acc]

    def _noisy(self, values):
        if not self.rng:
            return [int(round(v)) for v in values]
        return [int(max(0, min(255, round(v + self.rng.gauss(0, RGB_NOISE))))) for v in values]

    def probe_rgb(self, probe):
        """(r, g, b) 0..255 as the probe reports it: spot mix, color cast, noise.

        The robot reads red, green and blue in three calls; they come from one
        measurement, so a reading is reused for a few milliseconds.
        """
        self.update()
        cached = self.probe_cache.get(probe)
        if cached and self.t - cached[0] < PROBE_SAMPLE_S:
            return cached[1]
        mixed = self._spot_rgb(probe)
        rgb = tuple(self._noisy([min(255, v * t) for v, t in zip(mixed, self.tint)]))
        self.probe_cache[probe] = (self.t, rgb)
        return rgb

    def probe_gray(self, probe):
        """Reflection 0..100 (fill light on minus off), from the spot mix."""
        self.update()
        mixed = self._spot_rgb(probe)
        gray = sum(mixed) / 3 / 255 * 100
        if self.rng:
            gray += self.rng.gauss(0, 1.5)
        return int(max(0, min(100, round(gray))))

    def probe_name(self, probe):
        """The sensor's built-in color name: nearest nominal color, white-balanced."""
        self.update()
        rgb = self._noisy(self._spot_rgb(probe))
        return min(SURFACE_RGB, key=lambda n: sum((a - b) ** 2 for a, b in zip(rgb, SURFACE_RGB[n])))

    def ground_color(self):
        """Legacy single reading: the surface under the sensor bar's centre."""
        self.update()
        return self.surface(*self._jitter(*self._ahead(GROUND_FRONT_CM)))

    def line_bits(self):
        """Probes over black, one bit each: L2 is bit 3, R2 bit 0 (1 = black)."""
        self.update()
        bits = 0
        for i, off in enumerate(LINE_OFFSETS_CM):
            x, y = self._jitter(*self._ahead(GROUND_FRONT_CM, off), amount=PROBE_JITTER_CM)
            if self.surface(x, y) == "black":
                bits |= 1 << (3 - i)
        return bits

    def line_offset(self):
        """Where the line runs under the sensor: -100 far left .. 100 far right, 0 if none."""
        self.update()
        cx, cy = self._ahead(GROUND_FRONT_CM)
        r = math.radians(self.heading)
        rx, ry = math.cos(r), -math.sin(r)  # the robot's right in world coordinates
        best, lateral = 4.0, None
        for b in self.bands:
            if b["color"] != "black":
                continue
            px, py = _closest_point(cx, cy, b["x1"], b["y1"], b["x2"], b["y2"])
            d = math.hypot(px - cx, py - cy)
            if d < best:
                best, lateral = d, (px - cx) * rx + (py - cy) * ry
        if lateral is None:
            return 0
        if self.rng:
            lateral += self.rng.gauss(0, 0.3)
        return int(max(-100, min(100, lateral / 2.5 * 100)))

    def snapshot(self):
        return {"x": round(self.x, 1), "y": round(self.y, 1), "heading": round(self.heading),
                "left_rpm": self.left_rpm, "right_rpm": self.right_rpm}


def _bands(spec):
    """All bands, top layer first. Legacy lines become round-capped black bands."""
    out = []
    for i, b in enumerate(spec.get("bands", [])):
        (x1, y1), (x2, y2) = b["from"], b["to"]
        out.append({"x1": x1, "y1": y1, "x2": x2, "y2": y2, "half": b.get("width", 1.9) / 2,
                    "color": b["color"], "layer": b.get("layer", 0), "round": False, "i": i})
    top = max([b["layer"] for b in out] + [0]) + 1
    for line in spec.get("lines", []):
        pts = line["points"]
        for (x1, y1), (x2, y2) in zip(pts, pts[1:]):
            out.append({"x1": x1, "y1": y1, "x2": x2, "y2": y2, "half": line.get("width", 2) / 2,
                        "color": line.get("color", "black"), "layer": top, "round": True, "i": len(out)})
    # Higher layers first; within a layer, later bands lie on top.
    out.sort(key=lambda b: (-b["layer"], -b["i"]))
    return out


def _in_band(x, y, b):
    """Is the point on the band? Tape has square ends; legacy lines round ones."""
    if b["round"]:
        return _point_segment_dist(x, y, (b["x1"], b["y1"], b["x2"], b["y2"])) <= b["half"]
    dx, dy = b["x2"] - b["x1"], b["y2"] - b["y1"]
    length2 = dx * dx + dy * dy
    if length2 == 0:
        return False
    t = ((x - b["x1"]) * dx + (y - b["y1"]) * dy) / length2
    if t < 0 or t > 1:
        return False
    return abs((x - b["x1"]) * dy - (y - b["y1"]) * dx) / math.sqrt(length2) <= b["half"]


def _probe_index(probe):
    """Probe by name ("L2".."R2") or by the vendor's number (4 = L2 .. 1 = R2)."""
    if isinstance(probe, int):
        return 4 - probe
    return PROBES.index(probe)


def _wrap180(a):
    a = (a + 180) % 360 - 180
    return a


def _closest_point(px, py, x1, y1, x2, y2):
    dx, dy = x2 - x1, y2 - y1
    if dx == dy == 0:
        return x1, y1
    t = max(0.0, min(1.0, ((px - x1) * dx + (py - y1) * dy) / (dx * dx + dy * dy)))
    return x1 + t * dx, y1 + t * dy


def _point_segment_dist(px, py, seg):
    x1, y1, x2, y2 = seg
    dx, dy = x2 - x1, y2 - y1
    if dx == dy == 0:
        return math.hypot(px - x1, py - y1)
    t = max(0.0, min(1.0, ((px - x1) * dx + (py - y1) * dy) / (dx * dx + dy * dy)))
    return math.hypot(px - (x1 + t * dx), py - (y1 + t * dy))


def _ray_segment(ox, oy, dx, dy, seg):
    """Distance along the ray (ox,oy)+t*(dx,dy) to the segment, or None."""
    x1, y1, x2, y2 = seg
    ex, ey = x2 - x1, y2 - y1
    denom = dx * ey - dy * ex
    if abs(denom) < 1e-9:
        return None
    t = ((x1 - ox) * ey - (y1 - oy) * ex) / denom
    u = ((x1 - ox) * dy - (y1 - oy) * dx) / denom
    if t >= 0 and 0 <= u <= 1:
        return t
    return None

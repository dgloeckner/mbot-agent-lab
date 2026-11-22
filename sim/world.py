"""2D world for the simulator: walls, colored fields, lines and the robot pose.

With noise enabled (spec "noise": true or a random.Random passed in), the
sensors misbehave like real ones: ultrasonic dropouts (0) and ghost echoes,
color and line readings that jitter at edges, and gyro twitches on bumps.
The robot script must cope with that; the tests check it does.

Units are centimetres and degrees. Heading 0 points along +y, positive headings
turn clockwise (to the right), matching the robot's yaw convention.
Time advances lazily: every sensor read or motor change integrates the motion
since the previous call, using wall-clock time.
"""

import json
import math
import random
import time

WHEEL_DIAMETER_CM = 6.5
CM_PER_WHEEL_DEG = math.pi * WHEEL_DIAMETER_CM / 360
TRACK_CM = 12.0            # distance between the wheels
ROBOT_RADIUS_CM = 8.0
SENSOR_FRONT_CM = 7.0      # ultrasonic sensor ahead of the centre
GROUND_FRONT_CM = 5.0      # quad RGB sensor ahead of the centre
LINE_OFFSETS_CM = (-1.8, -0.6, 0.6, 1.8)  # L2, L1, R1, R2
MAX_RANGE_CM = 300


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
        start = spec.get("start", {"x": 0, "y": 0, "heading": 0})
        self.x, self.y, self.heading = float(start["x"]), float(start["y"]), float(start.get("heading", 0))
        self.pitch = 0.0
        self.roll = 0.0
        self.left_rpm = 0.0
        self.right_rpm = 0.0
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
        dl_deg = self.left_rpm * 6 * dt     # rpm * 360 / 60
        dr_deg = self.right_rpm * 6 * dt
        dl, dr = dl_deg * CM_PER_WHEEL_DEG, dr_deg * CM_PER_WHEEL_DEG
        d = (dl + dr) / 2
        dtheta = math.degrees((dl - dr) / TRACK_CM)
        mid = math.radians(self.heading + dtheta / 2)
        nx, ny = self.x + d * math.sin(mid), self.y + d * math.cos(mid)
        if self._collides(nx, ny):
            # Blocked: wheels slip, the pose only rotates.
            self.bumped = True
            nx, ny = self.x, self.y
        self.left_deg += dl_deg
        self.right_deg += dr_deg
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

        The color reading flickers more (1.2 cm) than the line sensors, which
        sit in a fixed bar and only see the line's edge blur (0.4 cm).
        """
        if self.rng:
            return x + self.rng.uniform(-amount, amount), y + self.rng.uniform(-amount, amount)
        return x, y

    def ground_color(self):
        self.update()
        x, y = self._jitter(*self._ahead(GROUND_FRONT_CM))
        if any(self._on_line(x, y, line) for line in self.lines):
            return "black"
        for f in reversed(self.fields):
            fx, fy, fw, fh = f["rect"]
            if fx <= x <= fx + fw and fy <= y <= fy + fh:
                return f["color"]
        return self.floor

    def line_bits(self):
        self.update()
        bits = 0
        for i, off in enumerate(LINE_OFFSETS_CM):
            x, y = self._jitter(*self._ahead(GROUND_FRONT_CM, off), amount=0.4)
            if any(self._on_line(x, y, line) for line in self.lines):
                bits |= 1 << (3 - i)
        return bits

    def line_offset(self):
        """Where the line runs under the sensor: -100 far left .. 100 far right, 0 if none."""
        self.update()
        cx, cy = self._ahead(GROUND_FRONT_CM)
        r = math.radians(self.heading)
        rx, ry = math.cos(r), -math.sin(r)  # the robot's right in world coordinates
        best, lateral = 4.0, None
        for line in self.lines:
            pts = line["points"]
            for (x1, y1), (x2, y2) in zip(pts, pts[1:]):
                px, py = _closest_point(cx, cy, x1, y1, x2, y2)
                d = math.hypot(px - cx, py - cy)
                if d < best:
                    best, lateral = d, (px - cx) * rx + (py - cy) * ry
        if lateral is None:
            return 0
        if self.rng:
            lateral += self.rng.gauss(0, 0.3)
        return int(max(-100, min(100, lateral / 2.5 * 100)))

    @staticmethod
    def _on_line(x, y, line):
        pts = line["points"]
        half = line.get("width", 2) / 2
        for (x1, y1), (x2, y2) in zip(pts, pts[1:]):
            if _point_segment_dist(x, y, (x1, y1, x2, y2)) <= half:
                return True
        return False

    def snapshot(self):
        return {"x": round(self.x, 1), "y": round(self.y, 1), "heading": round(self.heading),
                "left_rpm": self.left_rpm, "right_rpm": self.right_rpm}


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

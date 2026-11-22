# Sensor filtering. Every decision on the robot (skills, reflexes, events,
# state reports) uses these filtered values, never a single raw reading:
# ultrasonic sensors produce dropouts and ghost echoes, the color sensor
# jitters at field edges and lines, and the gyro twitches on a bump.

from hw import (hw_distance_cm, hw_ground_color, hw_line_state, hw_line_offset, hw_yaw,
                hw_tilt, hw_battery, hw_button, hw_loudness)

SN_DIST_WINDOW = 5      # median over the last valid distance readings
SN_DIST_MIN = 2         # readings below are dropouts (the sensor reports 0 on errors)
SN_DIST_MAX = 300       # the sensor's range; nothing in sight reads as this
SN_COLOR_WINDOW = 7     # the floor color changes when a new color shows up in
SN_COLOR_NEED = 5       # at least 5 of the last 7 readings (majority vote)
SN_LINE_WINDOW = 3      # same for the line state: 2 of the last 3
SN_LINE_NEED = 2
SN_TILT_ON_DEG = 45     # tilted when pitch or roll goes beyond this ...
SN_TILT_OFF_DEG = 30    # ... and level again only when both are below this
SN_TILT_STABLE = 4      # readings in a row before the tilt state flips (~100 ms)


def sn_median(values):
    ordered = sorted(values)
    return ordered[len(ordered) // 2]


class Vote:
    """A reading that changes only when a new value wins a majority vote.

    Flicker back and forth at an edge never reaches the majority; a real
    change does within a few readings.
    """

    def __init__(self, window, need):
        self.window = window
        self.need = need
        self.recent = []
        self.value = None

    def update(self, v):
        self.recent.append(v)
        if len(self.recent) > self.window:
            self.recent.pop(0)
        if self.value is None:
            self.value = v
        elif v != self.value and self.recent.count(v) >= self.need:
            self.value = v
        return self.value


class Sense:
    def __init__(self):
        self.dist_window = []
        self.dist_raw = SN_DIST_MAX
        self.color = Vote(SN_COLOR_WINDOW, SN_COLOR_NEED)
        self.line = Vote(SN_LINE_WINDOW, SN_LINE_NEED)
        self.tilted = False
        self.tilt_count = 0

    def distance(self, raw):
        """Median of the last valid readings; dropouts do not count."""
        self.dist_raw = raw
        if raw >= SN_DIST_MIN:
            self.dist_window.append(min(raw, SN_DIST_MAX))
            if len(self.dist_window) > SN_DIST_WINDOW:
                self.dist_window.pop(0)
        if not self.dist_window:
            return SN_DIST_MAX
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
        raw_color = hw_ground_color()
        return {
            "distance_cm": self.distance(hw_distance_cm()),
            "distance_raw": self.dist_raw,
            "color": self.color.update(raw_color),
            "color_raw": raw_color,
            "line": self.line.update(hw_line_state()),
            "line_offset": hw_line_offset(),
            "yaw": int(hw_yaw()),
            "battery": int(hw_battery()),
            "tilted": self.tilt(pitch, roll),
            "button_a": hw_button("a"),
            "button_b": hw_button("b"),
            "loudness": hw_loudness(),
        }

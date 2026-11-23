"""Fake of the Makeblock `mbuild` module (ultrasonic sensor 2, quad RGB sensor).

The quad RGB sensor follows the vendor API checked on 2026-11-22: probes by
name ("L2", "L1", "R1", "R2") or number (4, 3, 2, 1), every call also takes
index=1, and get_line_sta sets a bit per probe where it sees BACKGROUND.
"""

import simstate


class _Ultrasonic2:
    def get(self, index=1):
        return simstate.world.distance_cm()


class _QuadRGB:
    def get_red(self, ch, index=1):
        return simstate.world.probe_rgb(ch)[0]

    def get_green(self, ch, index=1):
        return simstate.world.probe_rgb(ch)[1]

    def get_blue(self, ch, index=1):
        return simstate.world.probe_rgb(ch)[2]

    def get_gray(self, ch, index=1):
        return simstate.world.probe_gray(ch)

    def get_color_sta(self, ch, index=1):
        return simstate.world.probe_name(ch)

    def is_color(self, color, ch, index=1):
        return simstate.world.probe_name(ch) == color

    def get_line_sta(self, index=1):
        return ~simstate.world.line_bits() & 0b1111

    def is_line(self, ch, index=1):
        return simstate.world.probe_name(ch) == "black"

    def is_background(self, ch, index=1):
        return not self.is_line(ch, index)

    def get_offset_track(self, index=1):
        return simstate.world.line_offset()


ultrasonic2 = _Ultrasonic2()
quad_rgb_sensor = _QuadRGB()

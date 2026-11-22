"""Fake of the Makeblock `mbuild` module (ultrasonic sensor 2, quad RGB sensor)."""

import simstate


class _Ultrasonic2:
    def get(self, index=1):
        return simstate.world.distance_cm()


class _QuadRGB:
    def get_color_sta(self, ch, index=1):
        return simstate.world.ground_color()

    def get_line_sta(self, ch="all", index=1):
        return simstate.world.line_bits()

    def get_offset_track(self, index=1):
        return simstate.world.line_offset()


ultrasonic2 = _Ultrasonic2()
quad_rgb_sensor = _QuadRGB()

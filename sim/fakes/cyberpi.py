"""Fake of the Makeblock `cyberpi` module, limited to what robot/src/hw.py uses."""

import simstate


class _Wifi:
    def __init__(self):
        self._connected = False

    def connect(self, ssid, password):
        self._connected = True

    def is_connect(self):
        return self._connected


class _Controller:
    def is_press(self, name):
        return simstate.buttons.get(name, False)


def _set_leds(rgb):
    if rgb != simstate.leds:
        simstate.led_trail.append(rgb)
        del simstate.led_trail[:-100]
    simstate.leds = rgb


class _Led:
    def on(self, r, g, b, id="all"):
        _set_leds((r, g, b))

    def off(self, id="all"):
        _set_leds((0, 0, 0))


LABELS = 8  # assumed number of labels: no screen of the robot script may need more


class _Display:
    """Labels live in numbered slots: a label drawn with an index that is already
    on screen replaces the earlier one, and a call without index always writes
    label 0 (seen on the robot: only the last such label stays visible).

    Not known from the real CyberPi: how many labels there are (here LABELS, a
    label with a higher index is not shown) and whether clear() removes all of
    them. clear_keeps_labels models the bad case, where a label stays until its
    index is drawn again.
    """

    def __init__(self):
        self._rgb = (255, 255, 255)
        self._labels = {}
        self.clear_keeps_labels = False

    def clear(self):
        if not self.clear_keeps_labels:
            self._labels = {}
        self._show()

    def set_brush(self, r, g, b):
        self._rgb = (r, g, b)

    def show_label(self, text, size, x, y, index=0):
        if 0 <= index < LABELS:
            self._labels[index] = (str(text), size, x, y, self._rgb)
        self._show()

    def _show(self):
        simstate.screen = [self._labels[i] for i in sorted(self._labels) if self._labels[i][0]]
        simstate.display = [label[0] for label in simstate.screen]


class _Audio:
    def play(self, name):
        simstate.sounds.append(name)

    def play_tone(self, freq, t):
        simstate.sounds.append(freq)


wifi = _Wifi()
controller = _Controller()
led = _Led()
display = _Display()
audio = _Audio()


def get_mac_address():
    return "sim:" + (simstate.world.robot_id if simstate.world else "?")


def get_yaw():
    # Like the real CyberPi: counterclockwise positive (the world uses right = positive).
    return -simstate.world.yaw()


def reset_yaw():
    simstate.world.reset_yaw()


def get_pitch():
    return simstate.world.tilt_reading()[0]


def get_roll():
    return simstate.world.tilt_reading()[1]


def get_battery():
    return simstate.battery


def get_loudness(mode="maximum"):
    return simstate.loudness

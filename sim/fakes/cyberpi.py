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


class _Led:
    def on(self, r, g, b, id="all"):
        simstate.leds = (r, g, b)

    def off(self, id="all"):
        simstate.leds = (0, 0, 0)


class _Console:
    def clear(self):
        simstate.display = []

    def println(self, text):
        simstate.display.append(str(text))


class _Audio:
    def play(self, name):
        simstate.sounds.append(name)

    def play_tone(self, freq, t):
        simstate.sounds.append(freq)


wifi = _Wifi()
controller = _Controller()
led = _Led()
console = _Console()
audio = _Audio()


def get_mac_address():
    return "sim:" + (simstate.world.robot_id if simstate.world else "?")


def get_yaw():
    return simstate.world.yaw()


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

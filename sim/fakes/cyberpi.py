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


class _Console:
    def clear(self):
        simstate.display = []

    def println(self, text):
        simstate.display.append(str(text))


class _Audio:
    # No play(): the built-in sounds block the main loop (docs/tools.md).
    def play_tone(self, freq, t):
        simstate.sounds.append(freq)


wifi = _Wifi()
controller = _Controller()
led = _Led()
console = _Console()
audio = _Audio()


def get_mac_address():
    return "sim:" + (simstate.world.robot_id if simstate.world else "?")


def get_firmware_version():
    return "sim"


def get_name():
    return simstate.device_name


def set_name(name):
    simstate.device_name = name


def restart():
    # The real CyberPi reboots; the robot script then closes its connection.
    simstate.restarts += 1


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

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
    # Five LEDs, ids 1..5. simstate.leds is the middle one, for the trace.
    def on(self, r, g, b, id="all"):
        if id == "all":
            simstate.led_strip = [(r, g, b)] * 5
        else:
            simstate.led_strip = list(simstate.led_strip)
            simstate.led_strip[int(id) - 1] = (r, g, b)
        _set_leds(simstate.led_strip[2])

    def off(self, id="all"):
        self.on(0, 0, 0, id)


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


def _noise(sigma):
    rng = simstate.world.rng if simstate.world else None
    return rng.gauss(0, sigma) if rng else 0


def get_loudness(mode="maximum"):
    return max(0, min(100, simstate.loudness + abs(_noise(1.5))))


def get_bri():
    return max(0, min(100, int(round(simstate.light + _noise(1.0)))))


def get_acc(axis):
    # Running motors shake the robot a little.
    w = simstate.world
    moving = w is not None and (w.left_rpm or w.right_rpm)
    return simstate.acc[axis] + _noise(0.8 if moving else 0.2)


def is_shake():
    return simstate.shake

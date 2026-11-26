# Hardware abstraction: the ONLY module that touches the vendor modules
# cyberpi, mbot2, mbuild and event. The simulator provides fakes with the same names.
#
# Every vendor call is marked VERIFY until it has been checked against the
# Makeblock documentation and on a real robot (names, units, signs, blocking).

import cyberpi
import mbot2
import mbuild

# The firmware's event system, as in the code mBlock generates for upload mode
# (`@event.start`). VERIFY on the robot; without it the script starts directly.
try:
    import event
except Exception:
    event = None

# Wheel geometry: centimetres per wheel degree. VERIFY/calibrate on the floor.
WHEEL_DIAMETER_CM = 6.5
CM_PER_WHEEL_DEG = 3.14159 * WHEEL_DIAMETER_CM / 360
# Track width: wheel center to wheel center, as in sim/mbot2.json. VERIFY: measure.
# Turns are measured with it: calibrate so that turn(360) ends facing the start.
TRACK_CM = 12.0

# Motor direction: the right motor is mounted mirrored. VERIFY sign.
RIGHT_MOTOR_SIGN = -1

COLOR_RGB = {
    "red": (255, 0, 0), "green": (0, 255, 0), "blue": (0, 0, 255),
    "yellow": (255, 200, 0), "purple": (160, 0, 255), "cyan": (0, 255, 255),
    "white": (255, 255, 255), "orange": (255, 100, 0), "off": (0, 0, 0),
    "gray": (120, 120, 120),
}


def hw_on_start(fn):
    """Run fn as the firmware's start handler, or right away without the event module.

    The start handler runs once the whole program has loaded, so the top level
    of the script returns to the firmware instead of looping there.
    """
    if event is None:
        fn()
    else:
        event.start(fn)  # VERIFY: same as decorating fn with @event.start


def hw_wifi_connect(ssid, password):
    cyberpi.wifi.connect(ssid, password)  # VERIFY: returns immediately?


def hw_wifi_ok():
    return bool(cyberpi.wifi.is_connect())  # VERIFY


def hw_mac():
    try:
        return cyberpi.get_mac_address()  # VERIFY
    except Exception:
        return "unknown"


def hw_motors(left, right):
    """Run both motors at the given speed (RPM, forward positive). Non-blocking."""
    mbot2.drive_speed(left, RIGHT_MOTOR_SIGN * right)  # VERIFY argument order and sign


def hw_motors_stop():
    mbot2.EM_stop("all")  # VERIFY


def hw_wheels_reset():
    mbot2.EM_reset_angle("all")  # VERIFY


def hw_wheels_deg():
    """Return (left, right) wheel angles in degrees since reset, forward positive."""
    left = mbot2.EM_get_angle("EM1")  # VERIFY port names and which side is EM1
    right = mbot2.EM_get_angle("EM2")
    return left, RIGHT_MOTOR_SIGN * right


def hw_yaw():
    """Gyro heading in degrees, positive = right (clockwise seen from above).

    Only for the state's yaw field: turns are measured with the wheel encoders,
    which also work on a stand and do not lag.

    The CyberPi counts yaw the other way round: on the robot a right turn by
    90 degrees read -100 (checked 2026-11-23).
    """
    return -cyberpi.get_yaw()


def hw_tilt():
    return cyberpi.get_pitch(), cyberpi.get_roll()  # VERIFY


def hw_distance_cm():
    return mbuild.ultrasonic2.get(1)  # VERIFY: index argument, max value when nothing seen


# Quad RGB sensor: four probes L2, L1, R1, R2 from left to right; L1 and R1 sit
# in the middle. API checked against the vendor docs on 2026-11-22 (probes by
# name or number 4..1, every call takes index=1); VERIFY on the robot.
PROBES = ("L2", "L1", "R1", "R2")


def hw_probe_rgb(ch):
    """(r, g, b) 0..255 of one probe."""
    q = mbuild.quad_rgb_sensor
    return q.get_red(ch, 1), q.get_green(ch, 1), q.get_blue(ch, 1)


def hw_probe_name(ch):
    """The sensor's built-in color name for one probe ("white", "red", ...)."""
    return mbuild.quad_rgb_sensor.get_color_sta(ch, 1)


def hw_line_offset():
    """Line position under the quad RGB sensor, -100 (far left) .. 100 (far right).

    VERIFY: return range and sign of get_offset_track on the real sensor.
    """
    return mbuild.quad_rgb_sensor.get_offset_track(1)


def hw_battery():
    return cyberpi.get_battery()  # VERIFY: percent


def hw_loudness():
    return cyberpi.get_loudness("maximum")  # VERIFY


def hw_button(name):
    return bool(cyberpi.controller.is_press(name))  # VERIFY: "a", "b"


def hw_leds(color, brightness=100):
    r, g, b = COLOR_RGB.get(color, COLOR_RGB["white"])
    k = brightness / 100
    cyberpi.led.on(int(r * k), int(g * k), int(b * k), "all")  # VERIFY


def hw_screen(labels):
    """Clear the display and draw labels (text, size 16/24/32, x, y, color)."""
    cyberpi.display.clear()  # VERIFY
    for text, size, x, y, color in labels:
        cyberpi.display.set_brush(*COLOR_RGB.get(color, COLOR_RGB["white"]))  # VERIFY
        cyberpi.display.show_label(text, size, x, y)  # VERIFY: x, y = top left in pixels


def hw_play(name):
    cyberpi.audio.play(name)  # VERIFY: must not block


def hw_beep():
    cyberpi.audio.play_tone(880, 0.05)  # VERIFY: blocks for 50 ms

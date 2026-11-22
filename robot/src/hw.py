# Hardware abstraction: the ONLY module that touches the vendor modules
# cyberpi, mbot2 and mbuild. The simulator provides fakes with the same names.
#
# Every vendor call is marked VERIFY until it has been checked against the
# Makeblock documentation and on a real robot (names, units, signs, blocking).

import cyberpi
import mbot2
import mbuild

# Wheel geometry: centimetres per wheel degree. VERIFY/calibrate on the floor.
WHEEL_DIAMETER_CM = 6.5
CM_PER_WHEEL_DEG = 3.14159 * WHEEL_DIAMETER_CM / 360

# Motor direction: the right motor is mounted mirrored. VERIFY sign.
RIGHT_MOTOR_SIGN = -1

COLOR_RGB = {
    "red": (255, 0, 0), "green": (0, 255, 0), "blue": (0, 0, 255),
    "yellow": (255, 200, 0), "purple": (160, 0, 255), "cyan": (0, 255, 255),
    "white": (255, 255, 255), "orange": (255, 100, 0), "off": (0, 0, 0),
}


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
    return cyberpi.get_yaw()  # VERIFY: degrees, sign (positive = right?)


def hw_yaw_reset():
    cyberpi.reset_yaw()  # VERIFY


def hw_tilt():
    return cyberpi.get_pitch(), cyberpi.get_roll()  # VERIFY


def hw_distance_cm():
    return mbuild.ultrasonic2.get(1)  # VERIFY: index argument, max value when nothing seen


def hw_ground_color():
    # VERIFY: channel name and return values ("white", "red", ...).
    return mbuild.quad_rgb_sensor.get_color_sta("L1", 1)


def hw_line_state():
    """Return 'none', 'left', 'center', 'right' or 'all' from the line bitmask."""
    bits = mbuild.quad_rgb_sensor.get_line_sta("all", 1)  # VERIFY: bitmask L2 L1 R1 R2
    if bits == 0:
        return "none"
    if bits == 15:
        return "all"
    if bits & 0b0110:
        return "center"
    return "left" if bits & 0b1000 else "right"


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


def hw_show_lines(lines):
    cyberpi.console.clear()  # VERIFY
    for line in lines:
        cyberpi.console.println(line)


def hw_play(name):
    cyberpi.audio.play(name)  # VERIFY: must not block


def hw_beep():
    cyberpi.audio.play_tone(880, 0.05)  # VERIFY: blocks for 50 ms

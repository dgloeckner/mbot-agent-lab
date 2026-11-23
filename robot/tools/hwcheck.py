# Hardware check for CyberPi + mBot2. Upload this file ALONE with mBlock
# (upload mode, Python) instead of main.py. It tries every vendor call the
# robot script uses (robot/src/hw.py) and prints one line per call:
#   HW OK <call> = <value>      or      HW ERR <call>: <exception>
# The lines go to the serial log (mBlock shows it as hex) and, shortened, to
# the display. Wheels in the air: the last block turns each motor briefly.

import time

import cyberpi
import mbot2
import mbuild


def out(line):
    print(line)
    try:
        cyberpi.console.println(line[:36])
    except Exception:
        pass


def check(name, f):
    try:
        out("HW OK %s = %r" % (name, f()))
    except Exception as e:
        out("HW ERR %s: %s %s" % (name, type(e).__name__, e))
    time.sleep(0.3)


def nested(depth, f):
    # Calls f under `depth` extra Python frames, like the robot's main loop does.
    return f() if depth == 0 else nested(depth - 1, f)


cyberpi.console.clear()
out("HW check start")
q = mbuild.quad_rgb_sensor
print("HW DIR quad_rgb_sensor %r" % (dir(q),))
print("HW DIR ultrasonic2 %r" % (dir(mbuild.ultrasonic2),))
print("HW DIR mbot2 %r" % (dir(mbot2),))

# CyberPi built-ins
check("battery", lambda: cyberpi.get_battery())
check("yaw", lambda: cyberpi.get_yaw())
check("pitch_roll", lambda: (cyberpi.get_pitch(), cyberpi.get_roll()))
check("loudness", lambda: cyberpi.get_loudness("maximum"))
check("button_a", lambda: cyberpi.controller.is_press("a"))
check("mac", lambda: cyberpi.get_mac_address())

# Ultrasonic
check("ultrasonic2.get(1)", lambda: mbuild.ultrasonic2.get(1))

# Quad RGB sensor: the robot crashed in get_color_sta with "maximum recursion
# depth exceeded". Top level first, then under extra frames.
check("quad.get_color_sta('L2',1)", lambda: q.get_color_sta("L2", 1))
check("quad.get_color_sta(4,1)", lambda: q.get_color_sta(4, 1))
check("quad.get_color_sta('L1')", lambda: q.get_color_sta("L1"))
check("quad.get_red/green/blue('L1',1)", lambda: (q.get_red("L1", 1), q.get_green("L1", 1), q.get_blue("L1", 1)))
check("quad.get_offset_track(1)", lambda: q.get_offset_track(1))
check("quad.get_color_sta('L2',1) depth 10", lambda: nested(10, lambda: q.get_color_sta("L2", 1)))
check("quad.get_color_sta('L2',1) depth 20", lambda: nested(20, lambda: q.get_color_sta("L2", 1)))
check("quad.get_red('L1',1) depth 20", lambda: nested(20, lambda: q.get_red("L1", 1)))

# Encoders and motors (wheels in the air!)
check("EM_reset_angle", lambda: mbot2.EM_reset_angle("all"))
check("EM angles before", lambda: (mbot2.EM_get_angle("EM1"), mbot2.EM_get_angle("EM2")))
out("HW motors in 3 s")
time.sleep(3)
check("drive_speed(20, 20)", lambda: mbot2.drive_speed(20, 20))
time.sleep(1)
check("EM_stop", lambda: mbot2.EM_stop("all"))
check("EM angles after drive_speed(20,20)", lambda: (mbot2.EM_get_angle("EM1"), mbot2.EM_get_angle("EM2")))

out("HW check done")

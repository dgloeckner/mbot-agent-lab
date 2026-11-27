# Hardware check for CyberPi + mBot2. Upload this file ALONE with mBlock
# (upload mode, Python) instead of main.py. It tries every vendor call the
# robot script uses (robot/src/hw.py) and prints one line per call:
#   HW OK <call> = <value>      or      HW ERR <call>: <exception>
# The lines go to the serial log (mBlock shows it as hex) and, shortened, to
# the display. Wheels in the air: the motor block turns each motor briefly.
# The turn block at the end turns on the spot: on the floor it checks the
# angle against the gyro, on a stand only the encoders and the run-on.

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


def largest_block():
    # Largest single allocation that still works, to 1 KB: the heap may be
    # fragmented, so this can be well below mem_free.
    lo, hi = 0, 16 * 1024 * 1024
    while hi - lo > 1024:
        mid = (lo + hi) // 2
        try:
            b = bytearray(mid)
            del b
            lo = mid
        except MemoryError:
            hi = mid
        gc.collect()
    return lo


cyberpi.console.clear()
out("HW check start")

# Memory without the robot script: the room our main.py can use at most.
# The robot reports mem_free with its script loaded in hello (control room).
import gc
import os
import sys

check("python", lambda: (sys.implementation, sys.version))
check("mem_free, mem_alloc", lambda: (gc.collect(), gc.mem_free(), gc.mem_alloc())[1:])
check("flash statvfs /", lambda: os.statvfs("/"))
try:
    # Before the probe below: the conservative GC may keep its big block alive.
    import micropython
    micropython.mem_info()  # serial log only
except Exception as e:
    out("HW ERR mem_info: %s" % e)
check("largest block", largest_block)
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

# Encoder turns, like the robot's turn skill: turn right on the spot until the
# encoders say `deg`, slowing down over the last degrees, stop, then wait for
# the wheels to stand still. Prints the angle at the stop, the run-on after it
# and the gyro's view. On the floor the gyro shows the real rotation: if it
# reads consistently more than the encoders, TRACK_CM in hw.py is too large
# (new TRACK_CM = TRACK_CM * encoder / gyro). On a stand the gyro stays near 0.
# Keep in step with robot/src/hw.py and skills.py.
TRACK_CM = 12.0
CM_PER_WHEEL_DEG = 3.14159 * 6.5 / 360
DEG_PER_WHEEL_DEG = CM_PER_WHEEL_DEG / TRACK_CM * 57.2958
RUNON_S = 0.1


def encoder_turn():
    left, right = mbot2.EM_get_angle("EM1"), -mbot2.EM_get_angle("EM2")
    return (left - right) * DEG_PER_WHEEL_DEG


def turn_check(deg):
    mbot2.EM_reset_angle("all")
    cyberpi.reset_yaw()
    t = time.time()
    while time.time() - t < 10:
        rest = deg - encoder_turn()
        v = min(30, max(8, rest))
        if rest <= v * 12 * RUNON_S * DEG_PER_WHEEL_DEG:
            break
        mbot2.drive_speed(v, v)  # the right motor is mirrored: both forward turns right
        time.sleep(0.02)
    mbot2.EM_stop("all")
    at_stop = encoder_turn()
    time.sleep(0.5)
    end = encoder_turn()
    out("HW TURN %d: stop %.1f end %.1f run-on %.1f gyro %.1f" % (deg, at_stop, end, end - at_stop, -cyberpi.get_yaw()))


out("HW turns in 3 s")
time.sleep(3)
check("turn 90", lambda: turn_check(90))
time.sleep(1)
check("turn 180", lambda: turn_check(180))

out("HW check done")


# Display labels, like robot/src/hw.py hw_screen and ui.py. Every label gets
# its own number (`index=`); without one, only the last label of a screen stays
# visible (seen on the robot on 2026-11-27). Three screens, 10 s each. Note what
# you see in docs/measurements.md, "Display labels".
#
# Screen 1, five rows. All five must be visible at the same time:
# - the row of 16 digits at size 16 fills the width exactly (8 px per glyph),
#   the 10 letters at size 24 (12 px) and the 8 at size 32 (16 px) too;
# - the digits start in the top left corner (x = 0, y = 0), nothing is cut off;
# - "Grün Weiß" shows its umlauts; red, green, orange and gray are colored.
# If a row is shorter, longer or wraps, fix UI_CHAR_W in robot/src/ui.py.
# "HW ERR display labels: TypeError ..." on the display means the firmware does
# not take `index=`.
#
# Screen 2, after clear(), two rows ("Zeile A", "Zeile B") at the top with the
# numbers 0 and 1. Nothing else may be on the screen. If rows 3 to 5 of screen 1
# ("Scout123" and below) are still there, clear() does not remove labels, and
# hw_screen must go on emptying the labels a smaller screen does not use.
#
# Screen 3, twelve labels "L0" to "L11" in three columns. The numbers that are
# visible are the labels the firmware has; the robot script needs L0 to L7.
ROWS = (("1234567890123456", 16, 0, (255, 255, 255)),
        ("ABCDEFGHIJ", 24, 20, (255, 0, 0)),
        ("Scout123", 32, 48, (0, 255, 0)),
        ("Grün Weiß", 16, 88, (255, 100, 0)),
        ("mbot-1  81%", 16, 110, (120, 120, 120)))


def display_check(name, rows):
    # Like check(), but success goes to the serial log only: a console line
    # on the display would draw over the labels to look at.
    try:
        d = cyberpi.display
        d.clear()
        for i, (text, size, x, y, rgb) in enumerate(rows):
            d.set_brush(*rgb)
            d.show_label(text, size, x, y, index=i)
        print("HW OK %s = %d" % (name, len(rows)))
    except Exception as e:
        out("HW ERR %s: %s %s" % (name, type(e).__name__, e))


WHITE = (255, 255, 255)
out("HW display in 3 s")
time.sleep(3)
display_check("display labels", [(t, size, 0, y, rgb) for t, size, y, rgb in ROWS])
time.sleep(10)
display_check("display fewer labels", [("Zeile A", 16, 0, 0, WHITE), ("Zeile B", 16, 0, 20, WHITE)])
time.sleep(10)
display_check("display 12 labels", [("L%d" % i, 16, 44 * (i % 3), 30 * (i // 3), WHITE) for i in range(12)])
time.sleep(10)

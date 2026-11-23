# Measurements for the new tools (docs/measurements.md, "Calls and sensors").
# Upload this file ALONE with mBlock (upload mode, Python) instead of main.py,
# with the wheels in the air. It prints one line per measurement:
#   MS OK <what> = <value> [<ms> ms]      or      MS ERR <what>: <exception>
# The lines go to the serial log (mBlock shows it as hex) and, shortened, to
# the display. Some steps need a person: the display says what to do.
#
# A call counts as blocking when it takes longer than 20 ms to return.

import time

import cyberpi
import mbot2
import mbuild

US = mbuild.ultrasonic2


def out(line):
    print(line)
    try:
        cyberpi.console.println(line[:36])
    except Exception:
        pass


def timed(name, f):
    """Run f once, print its value and how long it took to return."""
    t = time.ticks_ms()
    try:
        v = f()
        out("MS OK %s = %r [%d ms]" % (name, v, time.ticks_diff(time.ticks_ms(), t)))
        return v
    except Exception as e:
        out("MS ERR %s: %s %s" % (name, type(e).__name__, e))
    return None


def stats(name, f, n=20, gap_ms=50):
    """Read f n times: min, mean, max."""
    vals = []
    for _ in range(n):
        try:
            vals.append(f())
        except Exception as e:
            out("MS ERR %s: %s %s" % (name, type(e).__name__, e))
            return None
        time.sleep_ms(gap_ms)
    out("MS OK %s = min %s mean %.1f max %s" % (name, min(vals), sum(vals) / len(vals), max(vals)))
    return vals


def say(text, wait_s):
    """Ask the person at the robot to do something, then wait."""
    out("MS DO %s (%d s)" % (text, wait_s))
    time.sleep(wait_s)


def dirs():
    for name, obj in (("cyberpi", cyberpi), ("audio", cyberpi.audio), ("led", cyberpi.led),
                      ("display", cyberpi.display), ("controller", cyberpi.controller)):
        try:
            print("MS DIR %s %r" % (name, dir(obj)))
        except Exception as e:
            out("MS ERR dir %s: %s" % (name, e))


cyberpi.console.clear()
out("MS start")
dirs()

# ---- device info (hello, set_name, restart) -------------------------------
timed("get_firmware_version", lambda: cyberpi.get_firmware_version())
name = timed("get_name", lambda: cyberpi.get_name())
if name is not None:
    timed("set_name (same name)", lambda: cyberpi.set_name(name))
timed("has restart", lambda: hasattr(cyberpi, "restart"))
timed("get_battery", lambda: cyberpi.get_battery())

# ---- sounds: do they return at once? (beep, record_sound, play_recording) ---
timed("audio.set_vol(30)", lambda: cyberpi.audio.set_vol(30))
timed("audio.play_tone(440, 0.5)", lambda: cyberpi.audio.play_tone(440, 0.5))
time.sleep(1)
timed("audio.play_drum('bass-drum', 0.25)", lambda: cyberpi.audio.play_drum("bass-drum", 0.25))
time.sleep(1)
say("Gleich Aufnahme: klatschen!", 2)
timed("audio.record()", lambda: cyberpi.audio.record())
time.sleep(2)
timed("audio.stop_record()", lambda: cyberpi.audio.stop_record())
timed("audio.play_record()", lambda: cyberpi.audio.play_record())
time.sleep(3)
timed("audio.stop()", lambda: cyberpi.audio.stop())

# ---- LEDs and display: cost per call (led_effect, eyes, show) ---------------
timed("led.on(255,0,0,'all')", lambda: cyberpi.led.on(255, 0, 0, "all"))
timed("led.on(0,255,0,1)", lambda: cyberpi.led.on(0, 255, 0, 1))
timed("led.show('red orange yellow green cyan')", lambda: cyberpi.led.show("red orange yellow green cyan"))
timed("led.move(1)", lambda: cyberpi.led.move(1))
timed("led.set_bri(30)", lambda: cyberpi.led.set_bri(30))
timed("led.set_bri(100)", lambda: cyberpi.led.set_bri(100))
timed("led.off('all')", lambda: cyberpi.led.off("all"))
timed("display.clear()", lambda: cyberpi.display.clear())
timed("display.show_label('Test', 16, 0, 0)", lambda: cyberpi.display.show_label("Test", 16, 0, 0))
timed("console.println", lambda: cyberpi.console.println("x"))
cyberpi.console.clear()

# Eyes: the eight LEDs of the ultrasonic sensor. First all on, then one at a
# time: note which LED lights up for each number.
timed("ultrasonic2.set_bri(100, 'all', 1)", lambda: US.set_bri(100, "all", 1))
time.sleep(1)
timed("ultrasonic2.led_show([100,0,0,0,0,0,0,100], 1)", lambda: US.led_show([100, 0, 0, 0, 0, 0, 0, 100], 1))
time.sleep(1)
for i in range(1, 9):
    US.set_bri(0, "all", 1)
    timed("eye %d on: which LED? set_bri(100, %d, 1)" % (i, i), lambda: US.set_bri(100, i, 1))
    time.sleep(1.5)
US.set_bri(0, "all", 1)

# ---- sensors (read_sensors, events) -----------------------------------------
timed("get_bri()", lambda: cyberpi.get_bri())
timed("get_loudness('maximum')", lambda: cyberpi.get_loudness("maximum"))
timed("get_acc('x')", lambda: cyberpi.get_acc("x"))
timed("is_shake()", lambda: cyberpi.is_shake())
timed("get_shakeval()", lambda: cyberpi.get_shakeval())
timed("is_freefall()", lambda: cyberpi.is_freefall())
for key in ("a", "b", "up", "down", "left", "right", "middle"):
    timed("controller.is_press('%s')" % key, lambda: cyberpi.controller.is_press(key))

# Light: do our own LEDs disturb the light sensor?
cyberpi.led.off("all")
stats("bri leds off", lambda: cyberpi.get_bri())
cyberpi.led.on(255, 255, 255, "all")
stats("bri leds white", lambda: cyberpi.get_bri())
cyberpi.led.on(255, 0, 0, "all")
stats("bri leds red", lambda: cyberpi.get_bri())
cyberpi.led.off("all")
say("Licht abdecken", 3)
stats("bri covered", lambda: cyberpi.get_bri())
say("Wieder frei", 2)

# Loudness and acceleration: quiet, motors running, clapping, bumping.
stats("loudness quiet", lambda: cyberpi.get_loudness("maximum"))
stats("acc_x quiet", lambda: cyberpi.get_acc("x"))
mbot2.drive_speed(30, -30)
time.sleep(0.5)
stats("loudness motors 30", lambda: cyberpi.get_loudness("maximum"))
stats("acc_x motors 30", lambda: cyberpi.get_acc("x"))
mbot2.EM_stop("all")
say("Gleich klatschen", 2)
stats("loudness clapping", lambda: cyberpi.get_loudness("maximum"), n=60)
say("Gleich vorn antippen", 2)
stats("acc_x bumping", lambda: cyberpi.get_acc("x"), n=60)
say("Gleich anheben", 2)
stats("acc_z lifting", lambda: cyberpi.get_acc("z"), n=60)
timed("is_freefall() while lifted?", lambda: cyberpi.is_freefall())
say("Gleich schuetteln", 2)
stats("shakeval shaking", lambda: cyberpi.get_shakeval(), n=40)
say("Gleich Joystick bewegen", 2)
seen = []
t = time.ticks_ms()
while time.ticks_diff(time.ticks_ms(), t) < 5000:
    for key in ("up", "down", "left", "right", "middle"):
        try:
            if cyberpi.controller.is_press(key) and key not in seen:
                seen.append(key)
        except Exception:
            pass
    time.sleep_ms(20)
out("MS OK joystick seen = %r" % (seen,))

out("MS done")

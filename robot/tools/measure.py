# Measurements on a real CyberPi + mBot2. Upload this file ALONE (upload mode,
# Python, or `cd server && go run ./cmd/robotupload ../robot/tools/measure.py`)
# instead of main.py, then follow the display. What each station is for and
# where its numbers go: docs/measurements.md, "Measurement script".
#
# Switch the mBot2 on and put it on the floor before uploading: three seconds
# after the start it wiggles 2 cm forward and back to see whether the wheels
# turn at all ("M wiggle ..."). After every station a line "M health ..."
# says whether the mBot2 shield still answers.
#
# Every station waits for button A (B skips it). After A the robot waits two
# seconds so the hand is gone, then measures; the LEDs are green while it
# waits and red while it measures. Results are printed as lines starting with
# "M " on the serial log; the display shows the prompts and a few numbers.
# Send the whole log back, also when a station failed ("M ERR ...").
#
# The robot DRIVES in the stations drive, dist, turn and block, without any
# obstacle reflex. Give it the room each prompt asks for.
#
# Keep in step with robot/src/hw.py, skills.py and sense.py.

import sys
import time

import cyberpi
import mbot2
import mbuild

try:
    from time import ticks_ms, ticks_diff, sleep_ms
except ImportError:  # CPython: the simulator's dry run
    _T0 = time.monotonic()

    def ticks_ms():
        return int((time.monotonic() - _T0) * 1000)

    def ticks_diff(a, b):
        return a - b

    def sleep_ms(ms):
        time.sleep(ms / 1000)

try:
    from time import ticks_us
except ImportError:
    def ticks_us():
        return ticks_ms() * 1000

try:
    import event
except Exception:
    event = None

# Stations to run, in this order. Shorten the tuple to repeat only some.
STATIONS = ("info", "rest", "nose", "sonic", "wheels", "slow", "drive", "dist", "turn", "block", "floor", "sound")

# Stations that start by themselves after three seconds instead of waiting for
# button A: for runs watched over the serial log (robotupload --monitor).
AUTO = ()

WAIT_S = 300            # a station nobody starts is skipped after this
GAP_MS = 20             # pause after every vendor call that is repeated
STACK = 32 * 1024       # like robot/src/main.py: the quad RGB calls need the room
TRACK_CM = 12.0
CM_PER_WHEEL_DEG = 3.14159 * 6.5 / 360
DEG_PER_WHEEL_DEG = CM_PER_WHEEL_DEG / TRACK_CM * 57.2958
RUNON_S = 0.1
PROBES = ("L2", "L1", "R1", "R2")

state = {"level": None, "gen": None, "mine": 0, "wheels": None}

# Motors, encoders, ultrasonic and the quad RGB sensor sit behind the mBot2
# shield. On USB power alone the CyberPi runs (display, buttons, gyro), but the
# shield does not: everything behind it then reads 0 and the wheels stand.
SHIELD_HINT = "mBot2 power switch on? battery charged? CyberPi seated, sensor cables plugged?"


def alive():
    """False once a newer program was started in this interpreter."""
    return state["gen"][0] == state["mine"]


def out(line):
    print("M " + line)


def show(line):
    out(line)
    try:
        cyberpi.console.println(line[:60])
    except Exception:
        pass


def health(tag):
    """One line on whether the mBot2 shield answers: printed after every station,
    so the log shows where it stopped, if it does."""
    try:
        d = []
        for _ in range(3):
            d.append(mbuild.ultrasonic2.get(1))
            sleep_ms(60)
        t0 = ticks_us()
        a, b = wheels()
        em = ticks_diff(ticks_us(), t0) / 1000
        out("health %s ultrasonic=%r encoders=%r read in %.1f ms battery=%r" % (tag, d, (a, b), em, cyberpi.get_battery()))
    except Exception as e:
        out("ERR health %s: %s %s" % (tag, type(e).__name__, e))


def wiggle(tag):
    """2 cm forward and back, like hwcheck's motor check. True if the wheels turned."""
    try:
        mbot2.EM_reset_angle("all")
        sleep_ms(200)
        motors(20, 20)
        sleep_ms(500)
        stop()
        sleep_ms(400)
        a, b = wheels()
        motors(-20, -20)
        sleep_ms(500)
        stop()
        sleep_ms(400)
        a2, b2 = wheels()
        out("wiggle %s forward deg=%r,%r then back at=%r,%r (asked about 60)" % (tag, a, b, a2, b2))
        return abs(a) >= 10 and abs(b) >= 10
    except Exception as e:
        stop()
        out("ERR wiggle %s: %s %s" % (tag, type(e).__name__, e))
        return False


def warn(what):
    show("WARN " + what)
    show(SHIELD_HINT)
    led(60, 0, 60)
    sleep_ms(1500)


def led(r, g, b):
    try:
        cyberpi.led.on(r, g, b, "all")
    except Exception:
        pass


def beep():
    try:
        cyberpi.audio.play_tone(880, 0.05)
    except Exception:
        pass


def pressed(name):
    try:
        return bool(cyberpi.controller.is_press(name))
    except Exception:
        return False


def wait(prompt):
    """Show the prompt and wait for a button: True on A, False on B or timeout."""
    if state.get("auto"):
        show(prompt)
        led(60, 40, 0)
        sleep_ms(3000)
        beep()
        led(60, 0, 0)
        return alive()
    show(prompt)
    show("A = go   B = skip")
    led(0, 60, 0)
    while alive() and (pressed("a") or pressed("b")):
        sleep_ms(30)
    t0 = ticks_ms()
    go = False
    while alive() and ticks_diff(ticks_ms(), t0) < WAIT_S * 1000:
        if pressed("a"):
            go = True
            break
        if pressed("b"):
            break
        sleep_ms(30)
    if not go:
        out("skipped")
        return False
    led(60, 40, 0)
    sleep_ms(2000)  # hands off
    beep()
    led(60, 0, 0)
    return True


def motors(left, right):
    mbot2.drive_speed(left, -right)  # the right motor is mounted mirrored


def stop():
    mbot2.EM_stop("all")


def wheels():
    return mbot2.EM_get_angle("EM1"), -mbot2.EM_get_angle("EM2")


def stats(values):
    """min, median, max, mean and standard deviation, one decimal."""
    v = sorted(values)
    n = len(v)
    mean = sum(v) / n
    sd = (sum([(x - mean) ** 2 for x in v]) / n) ** 0.5
    return "min=%.1f med=%.1f max=%.1f mean=%.2f sd=%.2f" % (v[0], v[n // 2], v[-1], mean, sd)


# ---- info: versions, memory, time per vendor call ------------------------------


def timeit(name, f, n=10):
    """Mean time of one call in ms. The calls are spaced: the robot script never
    fires vendor calls back to back either."""
    try:
        total = 0
        for _ in range(n):
            t0 = ticks_us()
            f()
            total += ticks_diff(ticks_us(), t0)
            sleep_ms(GAP_MS)
        return total / n / 1000
    except Exception as e:
        out("ERR time %s: %s %s" % (name, type(e).__name__, e))
        return 0


def station_info():
    try:
        import gc
        gc.collect()
        out("info python=%r mem_free=%d" % (sys.version, gc.mem_free()))
    except Exception as e:
        out("info python=%r (%s)" % (sys.version, e))
    out("info battery=%r" % (cyberpi.get_battery(),))
    q = mbuild.quad_rgb_sensor
    calls = (
        ("ultrasonic", lambda: mbuild.ultrasonic2.get(1)),
        ("rgb_one_probe", lambda: (q.get_red("L1", 1), q.get_green("L1", 1), q.get_blue("L1", 1))),
        ("color_sta_one_probe", lambda: q.get_color_sta("L1", 1)),
        ("offset_track", lambda: q.get_offset_track(1)),
        ("yaw", cyberpi.get_yaw),
        ("pitch", cyberpi.get_pitch),
        ("roll", cyberpi.get_roll),
        ("encoders_both", wheels),
        ("battery", cyberpi.get_battery),
        ("button", lambda: cyberpi.controller.is_press("a")),
        ("loudness", lambda: cyberpi.get_loudness("maximum")),
    )
    ms = {}
    for name, f in calls:
        ms[name] = timeit(name, f)
        out("time %s ms=%.2f" % (name, ms[name]))
    # One loop iteration of the robot script reads these (robot/src/sense.py).
    common = (ms["ultrasonic"] + ms["offset_track"] + ms["yaw"] + ms["pitch"] + ms["roll"]
              + ms["battery"] + 2 * ms["button"] + ms["loudness"])
    out("time loop_reads_rgb ms=%.1f (calibrated: 4 probes RGB)" % (common + 4 * ms["rgb_one_probe"]))
    out("time loop_reads_names ms=%.1f (built-in color names)" % (common + 4 * ms["color_sta_one_probe"]))
    out("time loop_extra_stall ms=%.2f (encoders once more while driving)" % ms["encoders_both"])


# ---- rest: what the sensors read while nothing happens --------------------------


def station_rest():
    if not wait("REST: stand on the floor, hands off"):
        return
    pitch, roll, dist, loud = [], [], [], []
    yaw0 = cyberpi.get_yaw()
    t0 = ticks_ms()
    for _ in range(80):
        pitch.append(cyberpi.get_pitch())
        roll.append(cyberpi.get_roll())
        dist.append(mbuild.ultrasonic2.get(1))
        loud.append(cyberpi.get_loudness("maximum"))
        sleep_ms(15)
    took = ticks_diff(ticks_ms(), t0)
    state["level"] = sum(pitch) / len(pitch)
    out("rest pitch " + stats(pitch))
    out("rest roll " + stats(roll))
    out("rest yaw_drift=%.2f in %d ms" % (cyberpi.get_yaw() - yaw0, took))
    out("rest ultrasonic " + stats(dist))
    if max(dist) < 2:
        warn("ultrasonic reads 0 only")
    out("rest loudness " + stats(loud))
    show("level pitch %.1f" % state["level"])


# ---- nose: which sign of the pitch is nose up ------------------------------------


def station_nose():
    if not wait("NOSE: lift the FRONT 2 cm and hold it"):
        return
    pitch = []
    for _ in range(40):
        pitch.append(cyberpi.get_pitch())
        sleep_ms(GAP_MS)
    mean = sum(pitch) / len(pitch)
    out("nose lifted pitch " + stats(pitch))
    if state["level"] is not None:
        rise = mean - state["level"]
        out("nose rise=%.1f over level %.1f -> PITCH_NOSE_UP_SIGN = %d" % (rise, state["level"], 1 if rise > 0 else -1))
    show("lifted pitch %.1f" % mean)
    if not wait("NOSE: lift the LEFT side 2 cm and hold it"):
        return
    roll = []
    for _ in range(40):
        roll.append(cyberpi.get_roll())
        sleep_ms(GAP_MS)
    out("nose left side lifted roll " + stats(roll))


# ---- sonic: the ultrasonic sensor at known distances ------------------------------


def station_sonic():
    todo = ("open space, 3 m free", "wall 20 cm before the sensor", "wall 5 cm before the sensor",
            "wall 50 cm, turned 30 deg away", "the low obstacle, touching")
    for i, what in enumerate(todo):
        if not wait("SONIC %d: %s" % (i + 1, what)):
            continue
        t0 = ticks_ms()
        d = []
        for _ in range(60):
            d.append(mbuild.ultrasonic2.get(1))
            sleep_ms(GAP_MS)
        took = ticks_diff(ticks_ms(), t0)
        zeros = len([x for x in d if x < 2])
        out("sonic %d (%s) %s below2=%d of 60 in %d ms" % (i + 1, what, stats(d), zeros, took))
        if zeros == 60 and i:
            warn("ultrasonic reads 0 only")
        show("median %.1f" % sorted(d)[30])


# ---- drive: wheels and pitch while driving free -----------------------------------


def run_trace(left, right, drive_ms, after_ms=700):
    """Run the motors at the given speeds, stop, keep sampling.

    Returns rows of (t_ms, left_deg, right_deg, pitch)."""
    mbot2.EM_reset_angle("all")
    sleep_ms(150)
    rows = []
    stopped = False
    t0 = ticks_ms()
    motors(left, right)
    while alive():
        t = ticks_diff(ticks_ms(), t0)
        if not stopped and t >= drive_ms:
            stop()
            stopped = True
        if t >= drive_ms + after_ms:
            break
        a, b = wheels()
        rows.append((t, a, b, cyberpi.get_pitch()))
        sleep_ms(10)
    stop()
    return rows


def streak(rows, cmd, ratio, until):
    """Longest time in ms a wheel stayed below `ratio` of its commanded travel,
    counted like Sense.stall in robot/src/sense.py (every wheel is judged)."""
    best = 0
    p0 = None
    since = last = 0
    exp = [0, 0]
    for t, a, b, _ in rows:
        if t > until:
            break
        pos = (a, b)
        if p0 is None:
            p0, since, last = pos, t, t
            continue
        ok = True
        for i in (0, 1):
            exp[i] += abs(cmd[i]) * (t - last) * 0.006
            if abs(pos[i] - p0[i]) < ratio * exp[i]:
                ok = False
        last = t
        if ok:
            p0, since = pos, t
            exp[0] = exp[1] = 0
        elif t - since > best:
            best = t - since
    return best


def longest(rows, lo, hi, test):
    """Longest run in ms of rows between lo and hi ms whose pitch passes test."""
    best = 0
    start = None
    for t, _, _, p in rows:
        if lo <= t < hi and test(p):
            if start is None:
                start = t
            if t - start > best:
                best = t - start
        else:
            start = None
    return best


def at(rows, t_ms):
    """Wheel angles at the first sample at or after t_ms."""
    for t, a, b, _ in rows:
        if t >= t_ms:
            return a, b
    return rows[-1][1], rows[-1][2]


def report(tag, rows, left, right, drive_ms, trace=True):
    n = len(rows)
    if n < 5:
        out("run %s too few samples: %d" % (tag, n))
        return
    dts = [rows[i][0] - rows[i - 1][0] for i in range(1, n)]
    out("run %s cmd=%d,%d ms=%d n=%d dt_mean=%.1f dt_max=%d" % (tag, left, right, drive_ms, n, sum(dts) / len(dts), max(dts)))
    parts = []
    for t in (100, 200, 300, 400, 600, 1000):
        if t <= drive_ms:
            a, b = at(rows, t)
            parts.append("t%d=%.0f/%.0f/%.0f" % (t, a, b, abs(left) * 6 * t / 1000))
    out("run %s travel left/right/asked deg: %s" % (tag, " ".join(parts)))
    a1, b1 = at(rows, drive_ms - 400)
    a2, b2 = at(rows, drive_ms - 20)
    out("run %s steady deg_per_s=%.0f,%.0f asked=%d,%d" % (tag, (a2 - a1) / 0.38, (b2 - b1) / 0.38, left * 6, right * 6))
    a1, b1 = at(rows, drive_ms)
    a2, b2 = rows[-1][1], rows[-1][2]
    still = drive_ms
    for i in range(n - 1, 0, -1):
        if abs(rows[i][1] - a2) + abs(rows[i][2] - b2) >= 1:
            still = rows[i][0]
            break
    out("run %s runon deg=%.0f,%.0f standstill_ms_after_stop=%d" % (tag, a2 - a1, b2 - b1, max(0, still - drive_ms)))
    cmd = (left, right)
    out("run %s stall_streak_ms ratio25=%d ratio50=%d ratio75=%d (free: must stay far below 400)" % (
        tag, streak(rows, cmd, 0.25, drive_ms), streak(rows, cmd, 0.5, drive_ms), streak(rows, cmd, 0.75, drive_ms)))
    level = state["level"]
    if level is None:
        level = rows[0][3]
    end = drive_ms + 10000
    for name, lo, hi in (("drive", 0, drive_ms), ("after", drive_ms, end)):
        ps = [p - level for t, _, _, p in rows if lo <= t < hi]
        if not ps:
            continue
        out("run %s pitch %s rel_min=%.1f rel_max=%.1f above4=%d above8=%d above12=%d below4=%d below8=%d below12=%d ms" % (
            tag, name, min(ps), max(ps),
            longest(rows, lo, hi, lambda p: p - level > 4), longest(rows, lo, hi, lambda p: p - level > 8),
            longest(rows, lo, hi, lambda p: p - level > 12), longest(rows, lo, hi, lambda p: p - level < -4),
            longest(rows, lo, hi, lambda p: p - level < -8), longest(rows, lo, hi, lambda p: p - level < -12)))
    if not trace:
        return
    # The trace, thinned to about 50 ms: t:left:right:pitch relative to the level.
    line = []
    last = -1000
    for t, a, b, p in rows:
        if t - last >= 50:
            last = t
            line.append("%d:%.0f:%.0f:%.1f" % (t, a, b, p - level))
            if len(line) == 8:
                out("trace %s %s" % (tag, " ".join(line)))
                line = []
    if line:
        out("trace %s %s" % (tag, " ".join(line)))


def station_wheels():
    """Do the wheels turn at all? The driving stations are skipped if not."""
    if not wait("WHEELS: floor, 10 cm free ahead, short test"):
        return
    mbot2.EM_reset_angle("all")
    sleep_ms(150)
    t0 = ticks_ms()
    motors(30, 30)
    sleep_ms(600)
    a, b = wheels()
    t1 = ticks_diff(ticks_ms(), t0)
    stop()
    sleep_ms(400)
    a2, b2 = wheels()
    out("wheels 30 RPM for %d ms: deg=%.0f,%.0f asked=%.0f after_stop=%.0f,%.0f" % (t1, a, b, 30 * 6 * t1 / 1000, a2, b2))
    state["wheels"] = abs(a2) >= 10 and abs(b2) >= 10
    if state["wheels"]:
        show("wheels ok")
        if a2 < 0 or b2 < 0:
            show("WARN a wheel counts backwards: sign in hw.py")
    else:
        warn("wheels did not turn")


def driving(prompt):
    """Start a driving station, unless the wheel test failed."""
    if state["wheels"] is False:
        show("skipped: wheels did not turn")
        return False
    return wait(prompt)


def station_slow():
    if not driving("SLOW: floor, 15 cm free ahead"):
        return
    for v in (2, 3, 5, 8):
        rows = run_trace(v, v, 2000, 300)
        report("slow%d" % v, rows, v, v, 2000, False)
        sleep_ms(300)
    # The inner wheel of the tightest, slowest arc: drive_arc radius 10, speed 5.
    rows = run_trace(8, 2, 2000, 300)
    report("slowarc8_2", rows, 8, 2, 2000, False)


def station_drive():
    if not driving("DRIVE: floor, 60 cm free ahead"):
        return
    for v in (20, 40, 80):
        for sign in (1, -1):
            rows = run_trace(sign * v, sign * v, 1500)
            report("%s%d" % ("fwd" if sign > 0 else "back", v), rows, sign * v, sign * v, 1500)
            sleep_ms(400)
    # Turning on the spot, both ways, as turn and scan do.
    for sign in (1, -1):
        rows = run_trace(sign * 30, -sign * 30, 1000)
        report("spin%s30" % ("R" if sign > 0 else "L"), rows, sign * 30, -sign * 30, 1000)
        sleep_ms(400)


# ---- dist: wheel degrees against centimetres on the floor ------------------------


def station_dist():
    if not driving("DIST: mark the wheel position, 1.2 m free ahead"):
        return
    target = 1440  # four wheel turns, about 82 cm
    mbot2.EM_reset_angle("all")
    cyberpi.reset_yaw()
    sleep_ms(300)
    t0 = ticks_ms()
    while alive() and ticks_diff(ticks_ms(), t0) < 20000:
        a, b = wheels()
        rest = (target - (a + b) / 2) * CM_PER_WHEEL_DEG
        v = min(40, max(8, rest * 10))
        if rest <= v * 6 * RUNON_S * CM_PER_WHEEL_DEG:
            break
        motors(v, v)
        sleep_ms(10)
    stop()
    sleep_ms(800)
    a, b = wheels()
    out("dist wheel_deg left=%.0f right=%.0f mean=%.1f assumed_cm=%.1f yaw_change=%.1f" % (
        a, b, (a + b) / 2, (a + b) / 2 * CM_PER_WHEEL_DEG, cyberpi.get_yaw()))
    out("dist -> measure the driven cm on the floor: CM_PER_WHEEL_DEG = cm / %.1f" % ((a + b) / 2))
    show("measure the cm driven")


# ---- turn: encoder turn against the floor and the gyro ----------------------------


def turn_run(deg):
    mbot2.EM_reset_angle("all")
    cyberpi.reset_yaw()
    sleep_ms(300)
    sign = 1 if deg > 0 else -1
    gyro = 0
    last = cyberpi.get_yaw()
    enc = 0
    stopped = None
    at_stop = 0
    t0 = ticks_ms()
    while alive():
        t = ticks_diff(ticks_ms(), t0)
        a, b = wheels()
        enc = (a - b) * DEG_PER_WHEEL_DEG
        y = cyberpi.get_yaw()
        gyro += (y - last + 180) % 360 - 180
        last = y
        if stopped is None:
            rest = (deg - enc) * sign
            v = min(30, max(8, rest))
            if rest <= v * 12 * RUNON_S * DEG_PER_WHEEL_DEG or t > 25000:
                stop()
                stopped, at_stop = t, enc
            else:
                motors(sign * v, -sign * v)
        elif t - stopped > 800:
            break
        sleep_ms(10)
    stop()
    # The CyberPi counts yaw left positive; reported here right positive like the robot script.
    out("turn %d encoder_at_stop=%.1f encoder_end=%.1f gyro=%.1f ms=%d" % (deg, at_stop, enc, -gyro, stopped or 0))


def station_turn():
    if not driving("TURN: mark where the nose points, 40 cm free around"):
        return
    turn_run(360)
    out("turn -> note how far the nose is from the mark (+ past it, - short of it)")
    if not wait("TURN: note the error, then A turns back left"):
        return
    turn_run(-360)
    out("turn -> note the error again; new TRACK_CM = 12.0 * 360 / (360 + error)")
    if not wait("TURN: A for a quarter turn right and left"):
        return
    turn_run(90)
    sleep_ms(500)
    turn_run(-90)


# ---- block: what the robot feels when it is held up --------------------------------


def station_block():
    speeds = (40, 80, 20)
    i = 0
    while alive() and i < 9:
        v = speeds[i % 3]
        if not driving("BLOCK %d: 5 cm before an obstacle, %d RPM, 2.5 s" % (i + 1, v)):
            break
        rows = run_trace(v, v, 2500)
        report("block%d_%d" % (i + 1, v), rows, v, v, 2500)
        i += 1
    show("BLOCK done after %d runs" % i)


# ---- power: what the motors need when nothing holds the robot ---------------------

POWER_RUNS = ((-10, -10, 1500), (10, 10, 1500), (-20, -20, 1500), (20, 20, 1500), (-40, -40, 1500), (40, 40, 1500),
              (-60, -60, 1200), (60, 60, 1200), (-80, -80, 1200), (80, 80, 1200), (-100, -100, 1000), (100, 100, 1000),
              (20, -20, 1500), (-20, 20, 1500), (40, -40, 1500), (-40, 40, 1500),
              (-40, -20, 1500), (40, 20, 1500), (-20, -40, 1500), (20, 40, 1500), (-8, -2, 1500), (8, 2, 1500))


def power_stats(v):
    if not v:
        return "none"
    return "%.1f/%.1f/%.1f" % (min(v), sum(v) / len(v), max(v))


def station_power():
    """Motor power in free driving: straight both ways, turning on the spot and in
    arcs. The robot first backs up, then returns, so it needs room behind."""
    if not driving("POWER: floor, 50 cm free behind"):
        return
    out("power time_ms EM_get_power=%.1f EM_get_speed=%.1f" % (
        timeit("power", lambda: mbot2.EM_get_power("EM1")), timeit("speed", lambda: mbot2.EM_get_speed("EM1"))))
    for left, right, ms in POWER_RUNS:
        start, run = ([], []), ([], [])
        n = 0
        t0 = ticks_ms()
        motors(left, right)
        while alive():
            t = ticks_diff(ticks_ms(), t0)
            if t >= ms:
                break
            p = (mbot2.EM_get_power("EM1"), -mbot2.EM_get_power("EM2"))
            w = start if t < 400 else run
            w[0].append(p[0])
            w[1].append(p[1])
            n += 1
            sleep_ms(GAP_MS)
        stop()
        out("power cmd=%d,%d n=%d first400ms left=%s right=%s then left=%s right=%s (min/mean/max)" % (
            left, right, n, power_stats(start[0]), power_stats(start[1]), power_stats(run[0]), power_stats(run[1])))
        sleep_ms(500)


# ---- ram: drive into an obstacle from a distance and keep pushing ------------------

RAM_SPEEDS = (40, 20, 80)
RAM_CM = 30


def ram_run(v, ms):
    """Forward at v RPM for ms, sampling wheels, pitch, motor power and acceleration.

    Returns rows of (t_ms, left_deg, right_deg, pitch, power_left, ax, ay, az, power_right)."""
    mbot2.EM_reset_angle("all")
    sleep_ms(150)
    rows = []
    stopped = False
    t0 = ticks_ms()
    motors(v, v)
    while alive():
        t = ticks_diff(ticks_ms(), t0)
        if not stopped and t >= ms:
            stop()
            stopped = True
        if t >= ms + 700:
            break
        a, b = wheels()
        try:
            power = mbot2.EM_get_power("EM1")
            power2 = -mbot2.EM_get_power("EM2")
        except Exception:
            power = power2 = -999
        try:
            acc = (cyberpi.get_acc("x"), cyberpi.get_acc("y"), cyberpi.get_acc("z"))
        except Exception:
            acc = (0, 0, 0)
        rows.append((t, a, b, cyberpi.get_pitch(), power, acc[0], acc[1], acc[2], power2))
        sleep_ms(5)
    stop()
    return rows


def approach():
    """Close in to 30 cm when the ultrasonic sensor sees the obstacle farther away."""
    t0 = ticks_ms()
    near = 0
    moving = False
    while alive() and ticks_diff(ticks_ms(), t0) < 8000:
        d = mbuild.ultrasonic2.get(1)
        near = near + 1 if d <= RAM_CM else 0
        if near >= 2 or d > 150:
            break
        if not moving:
            motors(25, 25)
            moving = True
        sleep_ms(GAP_MS)
    stop()
    sleep_ms(600)
    out("ram approach ultrasonic=%r" % mbuild.ultrasonic2.get(1))


def station_ram():
    """The robot stands about 30 cm before a low obstacle and drives into it at
    three speeds, pushes for two seconds and backs off again by itself."""
    for v in RAM_SPEEDS:
        if not driving("RAM: 30 cm before the obstacle, facing it, %d RPM" % v):
            return
        approach()
        ms = int(45 / (v * 6 * CM_PER_WHEEL_DEG) * 1000) + 2000  # 45 cm of way, then 2 s more
        rows = ram_run(v, ms)
        tag = "ram%d" % v
        report(tag, [r[:4] for r in rows], v, v, ms, False)
        level = state["level"]
        if level is None:
            level = rows[0][3]
        line = []
        last = rows[0]
        for r in rows[1:]:
            dt = (r[0] - last[0]) / 1000
            speed = ((r[1] - last[1]) + (r[2] - last[2])) / 2 / dt if dt > 0 else 0
            line.append("%d:%.0f:%.0f:%r:%.1f:%.1f:%.1f:%r" % (r[0], speed, r[3] - level, r[4], r[5], r[6], r[7], r[8]))
            last = r
            if len(line) == 6:
                out("ramtrace %s %s" % (tag, " ".join(line)))
                line = []
        if line:
            out("ramtrace %s %s" % (tag, " ".join(line)))
        out("ramtrace columns t_ms:wheel_deg_per_s:pitch_rel:power_left:acc_x:acc_y:acc_z:power_right")
        # Back off to where it started, by the encoders; the next run closes in
        # again by the ultrasonic sensor, because slipping wheels count too much.
        sleep_ms(500)
        t0 = ticks_ms()
        motors(-40, -40)
        while alive() and ticks_diff(ticks_ms(), t0) < ms + 3000:
            a, b = wheels()
            if (a + b) / 2 <= 15:
                break
            sleep_ms(10)
        stop()
        sleep_ms(600)
        a, b = wheels()
        out("ram %s backed off: wheels at %r,%r pitch_rel=%.1f ultrasonic=%r" % (tag, a, b, cyberpi.get_pitch() - level, mbuild.ultrasonic2.get(1)))


# ---- floor: what the four probes read on each surface ------------------------------


def station_floor():
    q = mbuild.quad_rgb_sensor
    todo = ("white floor", "black line under all four", "red", "yellow", "green", "blue",
            "black line under L2 only (left edge)", "black line under R2 only (right edge)",
            "black line centered under L1 and R1")
    for i, what in enumerate(todo):
        if not wait("FLOOR %d: %s" % (i + 1, what)):
            continue
        parts = []
        for p in PROBES:
            acc = [0, 0, 0]
            for _ in range(8):
                acc[0] += q.get_red(p, 1)
                acc[1] += q.get_green(p, 1)
                acc[2] += q.get_blue(p, 1)
                sleep_ms(GAP_MS)
            parts.append("%s=%d,%d,%d/%s" % (p, acc[0] // 8, acc[1] // 8, acc[2] // 8, q.get_color_sta(p, 1)))
        offs = []
        for _ in range(8):
            offs.append(q.get_offset_track(1))
            sleep_ms(GAP_MS)
        try:
            line = q.get_line_sta(1)
        except Exception as e:
            line = repr(e)
        out("floor %d (%s) %s offset_track=%d..%d line_sta=%r" % (i + 1, what, " ".join(parts), min(offs), max(offs), line))


# ---- sound: loudness for the clap event --------------------------------------------


def listen(ms):
    v = []
    t0 = ticks_ms()
    while ticks_diff(ticks_ms(), t0) < ms:
        v.append(cyberpi.get_loudness("maximum"))
        sleep_ms(10)
    return v


def station_sound():
    if not wait("SOUND: be quiet for 3 s"):
        return
    out("sound quiet " + stats(listen(3000)))
    if not wait("SOUND: after the beep clap 3 times, 1 m away"):
        return
    v = listen(4000)
    out("sound claps %s above50=%d above70=%d of %d" % (stats(v), len([x for x in v if x >= 50]), len([x for x in v if x >= 70]), len(v)))
    if not wait("SOUND: wheels in the air, motors run 2 s"):
        return
    motors(40, 40)
    v = listen(2000)
    stop()
    out("sound motors40 " + stats(v))


TABLE = {"info": station_info, "rest": station_rest, "nose": station_nose, "sonic": station_sonic,
         "wheels": station_wheels, "slow": station_slow, "drive": station_drive, "dist": station_dist, "turn": station_turn,
         "block": station_block, "power": station_power, "ram": station_ram, "floor": station_floor, "sound": station_sound}


def run_all():
    try:
        cyberpi.console.clear()
    except Exception:
        pass
    show("measure start")
    for name in STATIONS:
        if not alive():
            break
        out("station %s" % name)
        state["auto"] = name in AUTO
        try:
            TABLE[name]()
        except Exception as e:
            stop()
            out("ERR station %s: %s %s" % (name, type(e).__name__, e))
        stop()
        health("after " + name)
    led(0, 0, 60)
    show("measure done")
    state["done"] = True


def claim():
    # A program uploaded over a running robot script shares its interpreter: bump the
    # counter the robot thread watches so it stops (robot/src/main.py), and stop
    # ourselves the same way when the next program starts.
    gen = sys.modules.get("mbot_gen") or [0]
    gen[0] += 1
    sys.modules["mbot_gen"] = gen
    state["gen"], state["mine"] = gen, gen[0]
    sleep_ms(1500)  # let an older robot thread stop its motors and leave


def main():
    health("start handler")
    try:
        import _thread
        _thread.stack_size(STACK)
        _thread.start_new_thread(run_all, ())
    except Exception:
        run_all()
        return
    while alive() and not state.get("done"):
        sleep_ms(500)


# Before anything else: does the shield answer, do the wheels turn? With an empty
# battery the CyberPi still runs on USB power, but everything behind the shield
# reads 0 (seen on 2026-11-27).
claim()
show("measure: wiggles 2 cm in 3 s")
sleep_ms(3000)
health("start")
if not wiggle("start"):
    warn("wheels did not turn")

if event is None:
    main()
else:
    event.start(main)

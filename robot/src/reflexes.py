# Reflexes run on the robot, independent of the server: the model needs seconds
# per decision, a wall needs milliseconds.

RF_STOP_CM = 8          # emergency stop distance while driving forward ...
RF_BRAKE_PER_RPM = 0.05 # ... plus this many cm per RPM for braking and sensor delay
RF_WATCHDOG_MS = 3000   # no heartbeat for this long stops the motors
# 8 cm rather than 6: a wall met at a slant answers from the edge of the sound
# cone, nearer to the robot's front corner than the reading says; the simulator
# keeps the body off walls down to 30 degrees (sim/tests/test_safety.py).
# The sensor looks forward only: backing up is blind, and walls grazed at a
# flatter angle stay unseen.

# What the sensor does not see (too low, too near, behind, beside) the robot only
# feels: the motors need more power than their speed asks for, or it climbs with
# its nose up while driving forward. Both end the motion with "blocked".

# Distances, tilt, nose and motor power come filtered from sense.py (median,
# hysteresis, minimum durations), so a single ghost echo, a bump or the first
# moment of speeding up does not trigger a reflex.


def stop_distance(skill):
    """Emergency stop distance for a forward skill: faster means earlier."""
    speed = getattr(skill, "speed", 40)
    return RF_STOP_CM + int(speed * RF_BRAKE_PER_RPM)


def reflex_abort_reason(skill, s):
    """Return a reason to abort the running motion skill, or None."""
    if skill is None:
        return None
    if s["tilted"]:
        return "tilted"
    if skill.forward and s["distance_cm"] < stop_distance(skill):
        return "obstacle"
    if s["held"] or skill.forward and s["nose_up"]:
        return "blocked"
    return None


def blocked_by(s):
    """What told the robot it is blocked, for the result."""
    return "motor_power" if s["held"] else "nose_up"

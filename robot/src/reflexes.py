# Reflexes run on the robot, independent of the server: the model needs seconds
# per decision, a wall needs milliseconds.

RF_STOP_CM = 6          # emergency stop distance while driving forward ...
RF_BRAKE_PER_RPM = 0.05 # ... plus this many cm per RPM for braking and sensor delay
RF_WATCHDOG_MS = 3000   # no heartbeat for this long stops the motors

# Distances and tilt come filtered from sense.py (median, hysteresis), so a
# single ghost echo or a bump does not trigger a reflex.


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
    return None

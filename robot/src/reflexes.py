# Reflexes run on the robot, independent of the server: the model needs seconds
# per decision, a wall needs milliseconds.

RF_STOP_CM = 6          # emergency stop distance while driving forward
RF_TILT_DEG = 45        # pitch or roll beyond this counts as tipped over / lifted
RF_WATCHDOG_MS = 3000   # no heartbeat for this long stops the motors


def reflex_abort_reason(skill, s):
    """Return a reason to abort the running motion skill, or None."""
    if skill is None:
        return None
    if s["tilted"]:
        return "tilted"
    if skill.forward and s["distance_cm"] < RF_STOP_CM:
        return "obstacle"
    return None


def is_tilted(pitch, roll):
    return abs(pitch) > RF_TILT_DEG or abs(roll) > RF_TILT_DEG

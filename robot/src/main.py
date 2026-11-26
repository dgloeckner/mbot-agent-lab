# Entry point. In the bundle this file comes last.
#
# The firmware starts the program through its event system: the last line hands
# main() to hw_on_start, which registers it as the start handler (@event.start)
# and returns, so the top level of the script ends. Without the event module
# (or in the simulator's fallback test) main() runs right away.
#
# mBlock starts the program in a thread with a small stack: the quad RGB sensor
# library failed with "maximum recursion depth exceeded" ten frames below the
# top level (robot/tools/hwcheck.py). So the robot runs in its own thread with
# a larger stack; without _thread it runs right here.
#
# A new upload starts the program again in the same interpreter but does not
# end our thread: two robots then fought over one MQTT client ID. Every start
# bumps a counter kept in sys.modules; an older robot thread sees it and stops.
#
# The start handler keeps running (sleeping) while its robot thread is the
# current one, like the forever loops mBlock generates inside @event.start.
# Whether the firmware treats a returned start handler as the end of the
# program is unknown, so we do not return while the robot runs.

import sys

from compat import sleep_ms
from hw import hw_motors_stop, hw_on_start
from loop import Robot
from ui import ui_error

MAIN_STACK = 32 * 1024


def robot_current(gen, mine):
    return sys.modules.get("mbot_gen") is gen and gen[0] == mine


def run_robot(gen, mine):
    try:
        Robot().run(lambda: robot_current(gen, mine))
    except Exception as e:
        hw_motors_stop()
        ui_error(repr(e)[:60])
        raise


def main():
    gen = sys.modules.get("mbot_gen") or [0]
    gen[0] += 1
    mine = gen[0]
    sys.modules["mbot_gen"] = gen
    try:
        import _thread
        _thread.stack_size(MAIN_STACK)
        _thread.start_new_thread(run_robot, (gen, mine))
    except Exception:
        run_robot(gen, mine)
        return
    while robot_current(gen, mine):
        sleep_ms(1000)


hw_on_start(main)

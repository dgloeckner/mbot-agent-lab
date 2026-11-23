# Entry point. In the bundle this file comes last.
#
# mBlock starts the program in a thread with a small stack: the quad RGB sensor
# library failed with "maximum recursion depth exceeded" ten frames below the
# top level (robot/tools/hwcheck.py). So the robot runs in its own thread with
# a larger stack; without _thread it runs right here.
#
# A new upload starts the program again in the same interpreter but does not
# end our thread: two robots then fought over one MQTT client ID. Every start
# bumps a counter kept in sys.modules; an older robot thread sees it and stops.

import sys

from compat import sleep_ms
from hw import hw_motors_stop
from loop import Robot
from ui import ui_error

MAIN_STACK = 32 * 1024


def run_robot(gen, mine):
    try:
        Robot().run(lambda: sys.modules.get("mbot_gen") is gen and gen[0] == mine)
    except Exception as e:
        hw_motors_stop()
        ui_error(repr(e)[:60])
        raise


def main():
    gen = sys.modules.get("mbot_gen") or [0]
    gen[0] += 1
    sys.modules["mbot_gen"] = gen
    try:
        import _thread
        _thread.stack_size(MAIN_STACK)
        _thread.start_new_thread(run_robot, (gen, gen[0]))
    except Exception:
        run_robot(gen, gen[0])
        return
    while True:
        sleep_ms(1000)


main()

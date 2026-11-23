# Entry point. In the bundle this file comes last.
#
# mBlock starts the program in a thread with a small stack: the quad RGB sensor
# library failed with "maximum recursion depth exceeded" ten frames below the
# top level (robot/tools/hwcheck.py). So the robot runs in its own thread with
# a larger stack; without _thread it runs right here.

from compat import sleep_ms
from hw import hw_motors_stop, hw_show_lines
from loop import Robot

MAIN_STACK = 32 * 1024


def run_robot():
    try:
        Robot().run()
    except Exception as e:
        hw_motors_stop()
        hw_show_lines(["Fehler", repr(e)[:60]])
        raise


def main():
    try:
        import _thread
        _thread.stack_size(MAIN_STACK)
        _thread.start_new_thread(run_robot, ())
    except Exception:
        run_robot()
        return
    while True:
        sleep_ms(1000)


main()

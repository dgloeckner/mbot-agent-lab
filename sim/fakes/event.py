"""Fake of the CyberPi firmware's `event` module, limited to what robot/src/hw.py uses.

As far as known, the firmware runs the program's top level first and then calls
every `@event.start` handler, each in its own thread. `start` here only registers
the handler; whoever runs the script (sim/robot_proc.py, the tests) calls
`fire_start` once the script has loaded.
"""

import threading

handlers = []


def start(fn):
    handlers.append(fn)
    return fn


def fire_start():
    """Run every registered start handler in its own thread; return the threads."""
    threads = [threading.Thread(target=fn, daemon=True) for fn in handlers]
    del handlers[:]
    for t in threads:
        t.start()
    return threads

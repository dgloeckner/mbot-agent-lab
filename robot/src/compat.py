# Compatibility layer: the same code runs on MicroPython (CyberPi) and CPython (simulator).

try:
    from time import ticks_ms, ticks_diff, sleep_ms
except ImportError:  # CPython
    import time as _compat_time

    _COMPAT_T0 = _compat_time.monotonic()

    def ticks_ms():
        return int((_compat_time.monotonic() - _COMPAT_T0) * 1000)

    def ticks_diff(a, b):
        return a - b

    def sleep_ms(ms):
        _compat_time.sleep(ms / 1000)

try:
    import ujson as json
except ImportError:
    import json

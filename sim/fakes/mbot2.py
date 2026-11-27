"""Fake of the Makeblock `mbot2` module. The right motor is mounted mirrored,
like on the real robot, so its speed and angle have the opposite sign."""

import simstate


def drive_speed(em1, em2):
    simstate.world.set_motors(em1, -em2)


def EM_stop(port="all"):
    simstate.world.set_motors(0, 0)


def EM_reset_angle(port="all"):
    simstate.world.reset_wheels()


def EM_get_angle(port):
    left, right = simstate.world.wheel_deg()
    return left if port == "EM1" else -right


def EM_get_power(port):
    left, right = simstate.world.motor_power()
    return left if port == "EM1" else -right

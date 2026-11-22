"""Shared state between the fake vendor modules and the simulator process."""

world = None          # sim.world.World, set by robot_proc before the robot script runs
display = []          # last lines printed to the console
leds = (0, 0, 0)      # last LED color
sounds = []           # names / tones played
buttons = {"a": False, "b": False}
loudness = 0
battery = 100

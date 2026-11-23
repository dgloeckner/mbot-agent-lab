"""Shared state between the fake vendor modules and the simulator process."""

world = None          # sim.world.World, set by robot_proc before the robot script runs
display = []          # texts of the labels on the display
screen = []           # the labels: (text, size, x, y, rgb)
leds = (0, 0, 0)      # last LED color
led_trail = []        # LED colors since the trace last looked (capped)
sounds = []           # names / tones played
buttons = {"a": False, "b": False}
loudness = 0
battery = 100

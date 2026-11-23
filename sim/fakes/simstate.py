"""Shared state between the fake vendor modules and the simulator process."""

world = None          # sim.world.World, set by robot_proc before the robot script runs
display = []          # last lines printed to the console
leds = (0, 0, 0)      # last LED color
led_trail = []        # LED colors since the trace last looked (capped)
sounds = []           # names / tones played
buttons = {"a": False, "b": False}  # also joystick keys: up, down, left, right, middle
loudness = 0
light = 40            # cyberpi.get_bri, 0..100
acc = {"x": 0.0, "y": 0.0, "z": 9.8}  # m/s^2 on top of the motors' shaking
shake = False
battery = 100
device_name = "mbot"  # cyberpi.get_name / set_name
restarts = 0          # cyberpi.restart calls

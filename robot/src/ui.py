# Display and LEDs. Student-facing text on the robot is German.
# Nothing here may block the main loop: no sounds, no vendor animations.

from compat import ticks_ms, ticks_diff
from hw import hw_led, hw_show_lines, COLOR_RGB
from fx import fx_frame, FX_OFF

UI_DISPLAY_MIN_MS = 200
UI_LED_PERIOD_MS = 100  # plain colors
UI_FX_MS = 40           # an effect while standing ...
UI_FX_MOVING_MS = 75    # ... and while driving: every third loop, the loop has more to do


def ui_solid(color, brightness=100):
    r, g, b = COLOR_RGB.get(color, COLOR_RGB["white"])
    k = brightness / 100
    return [(int(r * k), int(g * k), int(b * k))] * 5


class Ui:
    def __init__(self, robot_id):
        self.robot_id = robot_id
        self.owner = ""
        self.name = ""
        self.conn = "startet"
        self.text = ""
        self.question = None  # ask_human: shown instead of the text, with the keys
        self.color = "white"
        self.led = None  # agent's set_leds color; loop clears it when the team changes
        self.fx = None   # agent's led_effect (the command's args); wins over led and the thinking pulse
        self.fx_t0 = 0
        self.fx_start = None
        self.frame = None  # the five colors last set
        self.agent = "waiting"
        self.identify_until = 0
        self.finding = False  # identify is running: blink and say so on the display
        self.dirty = True
        self.last_display = 0
        self.last_led = 0

    def set_conn(self, conn):
        if conn != self.conn:
            self.conn = conn
            self.dirty = True

    def set_owner(self, owner, name, color):
        if owner != self.owner or name != self.name:
            self.owner = owner
            self.name = name
            self.dirty = True
        self.color = color or "white"

    def set_text(self, text):
        self.text = text[:60]
        self.dirty = True

    def set_question(self, text):
        self.question = text
        self.dirty = True

    def set_color(self, color):
        """set_leds: a plain color ends a running effect."""
        self.led = color
        self.fx = None

    def set_effect(self, fx, now):
        self.fx = fx
        self.fx_t0 = now
        self.fx_start = self.frame or [FX_OFF] * 5

    def show(self, frame):
        """Set the LEDs to frame, touching only what changed: each call costs loop time."""
        old = self.frame
        if frame == old:
            return
        if frame.count(frame[0]) == 5:
            hw_led(frame[0])
        else:
            for i in range(5):
                if old is None or frame[i] != old[i]:
                    hw_led(frame[i], i)
        self.frame = frame

    def set_agent(self, agent):
        self.agent = agent

    def identify(self, ms=3000):
        self.identify_until = ticks_ms() + ms

    def tick(self, now, moving=False):
        finding = ticks_diff(self.identify_until, now) > 0
        if finding != self.finding:
            self.finding = finding
            self.dirty = True
        if self.dirty and ticks_diff(now, self.last_display) >= UI_DISPLAY_MIN_MS:
            self.dirty = False
            self.last_display = now
            lines = [
                "%s (%s)" % (self.name, self.robot_id) if self.name else self.robot_id,
                self.owner or "frei",
                self.conn,
            ]
            if self.question is not None:
                lines += [self.question, "A = ja, B = nein"]
            else:
                lines.append("Hier bin ich!" if finding else self.text)
            hw_show_lines(lines)
        fx = self.fx
        period = UI_LED_PERIOD_MS if fx is None else UI_FX_MOVING_MS if moving else UI_FX_MS
        if ticks_diff(now, self.last_led) < period:
            return
        self.last_led = now
        t = ticks_diff(now, self.fx_t0)
        if fx is not None and fx.get("ms") and t >= fx["ms"]:
            self.fx = fx = None
        # Priority: identify, connection, free robot, the agent's effect, thinking, plain color.
        if finding:
            self.show(ui_solid("white" if (now // 250) % 2 == 0 else "off"))
        elif self.conn != "verbunden":
            self.show(ui_solid("orange", 30))
        elif not self.owner:
            self.show(ui_solid("white", 10))
        elif fx is not None:
            self.show(fx_frame(fx, t, self.fx_start))
        elif self.agent == "thinking":
            # Triangle wave between 10 and 100 % brightness, period 1 s.
            phase = now % 1000
            level = phase if phase < 500 else 1000 - phase
            self.show(ui_solid(self.led or self.color, 10 + level * 90 // 500))
        else:
            self.show(ui_solid(self.led or self.color))

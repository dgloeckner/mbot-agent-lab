# Display and LEDs. Student-facing text on the robot is German.
# Nothing here may block the main loop: no sounds, no vendor animations.

from compat import ticks_ms, ticks_diff
from hw import hw_leds, hw_show_lines

UI_DISPLAY_MIN_MS = 200
UI_LED_PERIOD_MS = 100


class Ui:
    def __init__(self, robot_id):
        self.robot_id = robot_id
        self.owner = ""
        self.name = ""
        self.conn = "startet"
        self.text = ""
        self.color = "white"
        self.led = None  # agent's set_leds color; loop clears it when the team changes
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

    def set_agent(self, agent):
        self.agent = agent

    def identify(self, ms=3000):
        self.identify_until = ticks_ms() + ms

    def tick(self, now):
        finding = ticks_diff(self.identify_until, now) > 0
        if finding != self.finding:
            self.finding = finding
            self.dirty = True
        if self.dirty and ticks_diff(now, self.last_display) >= UI_DISPLAY_MIN_MS:
            self.dirty = False
            self.last_display = now
            hw_show_lines([
                "%s (%s)" % (self.name, self.robot_id) if self.name else self.robot_id,
                self.owner or "frei",
                self.conn,
                "Hier bin ich!" if finding else self.text,
            ])
        if ticks_diff(now, self.last_led) < UI_LED_PERIOD_MS:
            return
        self.last_led = now
        if finding:
            hw_leds("white" if (now // 250) % 2 == 0 else "off")
        elif self.conn != "verbunden":
            hw_leds("orange", 30)
        elif not self.owner:
            hw_leds("white", 10)
        elif self.agent == "thinking":
            # Triangle wave between 10 and 100 % brightness, period 1 s.
            phase = now % 1000
            level = phase if phase < 500 else 1000 - phase
            hw_leds(self.led or self.color, 10 + level * 90 // 500)
        else:
            hw_leds(self.led or self.color)

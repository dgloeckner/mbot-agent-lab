# Display and LEDs. Student-facing text on the robot is German.
#
# One screen per state, each with one big thing: "Verbinde...", the free
# robot's ID, the assigned robot's name in team color, the agent's show_text,
# or "Verbindung weg". A status bar on top: state or name left, battery or
# "denkt..." right. Screens are lists of labels (text, size, x, y, color) on a
# 128 x 128 display; tick() redraws only when the list changes.

from compat import ticks_ms, ticks_diff
from hw import hw_leds, hw_screen, hw_beep

UI_DISPLAY_MIN_MS = 200
UI_LED_PERIOD_MS = 100
UI_W = 128
UI_CHAR_W = {16: 8, 24: 12, 32: 16}  # VERIFY: glyph width per label size
UI_COLS = 16  # characters per line at size 16


def ui_center(text, y, color, size=16):
    text = text[:UI_W // UI_CHAR_W[size]]
    return (text, size, (UI_W - len(text) * UI_CHAR_W[size]) // 2, y, color)


def ui_big(text, y, color):
    """Centered in the largest size the text fits."""
    size = 32 if len(text) <= 8 else 24 if len(text) <= 10 else 16
    return ui_center(text, y, color, size)


def ui_wrap(text, n):
    """At most n lines of UI_COLS characters, broken between words where possible."""
    lines = []
    line = ""
    for word in text.split():
        if line and len(line) + 1 + len(word) <= UI_COLS:
            line += " " + word
            continue
        if line:
            lines.append(line)
        while len(word) > UI_COLS:
            lines.append(word[:UI_COLS])
            word = word[UI_COLS:]
        line = word
    if line:
        lines.append(line)
    return lines[:n]


def ui_lines(text, y, n, color):
    return [(t, 16, 0, y + i * 18, color) for i, t in enumerate(ui_wrap(text, n))]


def ui_error(text):
    hw_screen([ui_center("Fehler", 0, "red")] + ui_lines(text, 22, 6, "white"))


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
        self.battery = -1  # shown value, moves in steps so the screen does not flicker
        self.identify_until = 0
        self.shown = None
        self.last_display = 0
        self.last_led = 0

    def set_conn(self, conn):
        self.conn = conn

    def set_owner(self, owner, name, color):
        self.owner = owner
        self.name = name
        self.color = color or "white"

    def set_text(self, text):
        self.text = text[:60]

    def set_agent(self, agent):
        self.agent = agent

    def set_battery(self, pct):
        if abs(pct - self.battery) >= 5:
            self.battery = pct

    def identify(self, ms=3000):
        self.identify_until = ticks_ms() + ms

    def bar(self, left, color):
        right = "denkt..." if self.owner and self.agent == "thinking" else \
            "%d%%" % self.battery if self.battery >= 0 else ""
        return [(left[:UI_COLS - 1 - len(right)], 16, 0, 0, color),
                (right, 16, UI_W - len(right) * 8, 0, "gray")]

    def screen(self):
        name = self.name or self.robot_id
        if self.conn != "verbunden":
            if not self.owner:
                return self.bar(self.robot_id, "gray") + [ui_center("Verbinde...", 56, "orange")]
            return self.bar(name, self.color) + [
                ui_center("Verbindung weg", 40, "orange"),
                ui_center("Motoren gestoppt", 62, "orange"),
                ui_center(self.owner, 100, "gray")]
        if not self.owner:
            return self.bar("online", "green") + [
                ui_center("frei", 32, "gray"),
                ui_big(self.robot_id, 56, "white"),
                ui_center("wartet auf Team", 104, "gray")]
        if self.text:
            return self.bar(name, self.color) + ui_lines(self.text, 22, 6, "white")
        return self.bar("online", "green") + [ui_big(name, 32, self.color)] + [
            ui_center(t, 76 + i * 18, "white") for i, t in enumerate(ui_wrap(self.owner, 2))] + [
            ui_center(self.robot_id, 112, "gray")]

    def tick(self, now):
        if ticks_diff(now, self.last_display) >= UI_DISPLAY_MIN_MS:
            self.last_display = now
            s = self.screen()
            if s != self.shown:
                self.shown = s
                hw_screen(s)
        if ticks_diff(now, self.last_led) < UI_LED_PERIOD_MS:
            return
        self.last_led = now
        if ticks_diff(self.identify_until, now) > 0:
            on = (now // 250) % 2 == 0
            hw_leds("white" if on else "off")
            if on and (now // 250) % 4 == 0:
                hw_beep()
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

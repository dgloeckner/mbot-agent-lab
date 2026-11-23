# Main loop: commands, skills, reflexes, events, state, heartbeat watchdog.
# Target: 20 to 50 ms per iteration. Nothing in here may block while a motor runs.

from compat import ticks_ms, ticks_diff, sleep_ms, mem_free
from config import ROBOT_ID, WIFI_SSID, WIFI_PASSWORD, BROKER_HOST, BROKER_PORT, BROKER_TLS, BROKER_PASSWORD
from hw import hw_motors_stop, hw_mac, hw_play
from sense import Sense
from ui import Ui
from events import Events, EV_HUSH_MOTION_MS, EV_HUSH_SOUND_MS
from net import Link
from skills import MOTION_SKILLS, make_motion_skill
from calib import CalibrateColorsSkill
from reflexes import RF_WATCHDOG_MS, reflex_abort_reason, stop_distance

SCRIPT_VERSION = "0.1.0"
PROTOCOL_VERSION = 1
LOOP_MS = 25
STATE_PERIOD_MS = 300
STATE_MIN_MS = 100
DEDUP_SIZE = 16
RECONNECT_MIN_MS = 500
RECONNECT_MAX_MS = 10000
MAX_MSGS_PER_LOOP = 5


class Robot:
    def __init__(self):
        self.ui = Ui(ROBOT_ID)
        self.sense = Sense()
        self.events = Events()
        self.link = Link(ROBOT_ID, BROKER_HOST, BROKER_PORT, BROKER_PASSWORD, BROKER_TLS, self.on_message)
        self.inbox = []
        self.skill = None
        self.skill_cmd_id = None
        self.seen_ids = []
        self.trip_log = []
        self.seq = 0
        self.owner = None
        self.last_hb = 0
        self.watchdog_tripped = False
        self.last_state = None
        self.last_state_at = 0
        self.backoff = RECONNECT_MIN_MS
        self.next_connect = 0
        self.s = None

    # ---- messages -------------------------------------------------------

    def on_message(self, suffix, payload):
        self.inbox.append((suffix, payload))

    def reply(self, cmd_id, ok, reason, data=None, error=None):
        msg = {"id": cmd_id, "ok": ok, "reason": reason}
        if data:
            msg["data"] = data
        if error:
            msg["error"] = error[:80]
        self.link.publish("result", msg)

    def handle(self, suffix, p, now):
        if suffix == "hb":
            self.on_heartbeat(p, now)
        elif suffix == "cmd":
            self.on_cmd(p)

    def on_heartbeat(self, p, now):
        self.last_hb = now
        if self.watchdog_tripped:
            self.watchdog_tripped = False
            self.ui.set_conn("verbunden")
        team = p.get("team", "")
        if self.owner is not None and team != self.owner:
            self.end_skill("reassigned")
            hw_motors_stop()
            self.trip_log = []
            self.ui.led = None
            self.ui.text = ""
        self.owner = team
        self.ui.set_owner(p.get("owner", "") if team else "", p.get("name", ""), p.get("color"))
        self.ui.set_agent(p.get("agent", "waiting"))

    def on_cmd(self, cmd):
        cmd_id = cmd.get("id")
        name = cmd.get("name")
        args = cmd.get("args") or {}
        if not cmd_id or not isinstance(args, dict):
            return
        if cmd_id in self.seen_ids:
            return
        self.seen_ids.append(cmd_id)
        if len(self.seen_ids) > DEDUP_SIZE:
            self.seen_ids.pop(0)

        if name == "stop":
            self.end_skill("stopped")
            hw_motors_stop()
            self.reply(cmd_id, True, "done")
        elif name in MOTION_SKILLS or name == "calibrate_colors":
            self.start_motion(cmd_id, name, args)
        elif name == "set_leds":
            self.ui.led = args.get("color") or self.ui.led
            self.reply(cmd_id, True, "done")
        elif name == "show_text":
            self.ui.set_text(str(args.get("text", "")))
            self.reply(cmd_id, True, "done")
        elif name == "play_sound":
            hw_play(str(args.get("name", "hello")))
            self.events.hush(ticks_ms(), EV_HUSH_SOUND_MS)
            self.reply(cmd_id, True, "done")
        elif name == "identify":
            self.ui.identify()
            self.reply(cmd_id, True, "done")
        elif name == "clear_log":
            self.trip_log = []
            self.reply(cmd_id, True, "done")
        else:
            self.reply(cmd_id, False, "error", error="unknown or not implemented: %s" % name)

    def start_motion(self, cmd_id, name, args):
        if self.skill is not None:
            self.reply(cmd_id, False, "busy")
            return
        if self.s["tilted"]:
            self.reply(cmd_id, False, "tilted")
            return
        try:
            if name == "calibrate_colors":
                skill = CalibrateColorsSkill(args, self.sense)
            else:
                skill = make_motion_skill(name, args, self.trip_log)
        except NotImplementedError:
            self.reply(cmd_id, False, "error", error="not implemented: %s" % name)
            return
        except (ValueError, TypeError) as e:
            self.reply(cmd_id, False, "error", error=str(e))
            return
        if skill.forward and self.s["distance_cm"] < stop_distance(skill):
            self.reply(cmd_id, False, "obstacle", {"distance_cm": self.s["distance_cm"]})
            return
        skill.start(self.s)
        self.skill = skill
        self.skill_cmd_id = cmd_id

    def end_skill(self, reason):
        """Abort the running motion skill, if any, with reason."""
        if self.skill is not None:
            self.finish_skill(self.skill.abort(reason, self.s))

    def finish_skill(self, result):
        ok, reason, data = result
        kind, value = self.skill.segment()
        if value:
            self.trip_log.append((kind, value))
        self.reply(self.skill_cmd_id, ok, reason, data)
        self.events.emit("skill_done" if ok else "skill_aborted", {"skill": self.skill.name, "reason": reason})
        self.skill = None
        self.skill_cmd_id = None

    # ---- sensing ----------------------------------------------------------

    def read_sensors(self):
        return self.sense.read()

    def next_seq(self):
        self.seq += 1
        return self.seq

    def publish_state(self, now):
        s = self.s
        key = (self.skill.name if self.skill else "idle", s["distance_cm"] // 3, s["floor"],
               s["color"], s["line"], s["tilted"])
        due = ticks_diff(now, self.last_state_at) >= STATE_PERIOD_MS
        changed = key != self.last_state and ticks_diff(now, self.last_state_at) >= STATE_MIN_MS
        if not (due or changed):
            return
        self.last_state = key
        self.last_state_at = now
        self.link.publish("state", {
            "skill": key[0], "distance_cm": s["distance_cm"], "floor": s["floor"], "color": s["color"],
            "line": s["line"], "yaw": s["yaw"], "battery": s["battery"],
            "tilted": s["tilted"], "seq": self.next_seq(), "t_ms": now,
        }, True)

    # ---- connection ---------------------------------------------------------

    def ensure_connected(self, now):
        if self.link.up:
            return
        self.end_skill("error")
        hw_motors_stop()
        self.ui.set_conn("getrennt")
        if ticks_diff(now, self.next_connect) < 0:
            return
        self.ui.set_conn("verbinde")
        self.ui.tick(now)
        try:
            self.link.connect(WIFI_SSID, WIFI_PASSWORD,
                              {"v": PROTOCOL_VERSION, "script": SCRIPT_VERSION, "mac": hw_mac(),
                               "mem_free": mem_free()})
        except Exception:
            self.link.close()
            self.ui.set_conn("getrennt")
            self.next_connect = ticks_ms() + self.backoff
            self.backoff = min(self.backoff * 2, RECONNECT_MAX_MS)
            return
        self.backoff = RECONNECT_MIN_MS
        # A new connection may be a restarted server that counts command IDs
        # from the start again: forget the old ones so nothing is skipped.
        self.seen_ids = []
        self.last_hb = ticks_ms()  # grace period until the first heartbeat
        self.watchdog_tripped = False
        self.last_state = None
        self.ui.set_conn("verbunden")

    def check_watchdog(self, now):
        if not self.link.up or self.watchdog_tripped:
            return
        if ticks_diff(now, self.last_hb) > RF_WATCHDOG_MS:
            self.watchdog_tripped = True
            self.end_skill("stopped")
            hw_motors_stop()
            self.ui.set_conn("getrennt")

    # ---- main loop ------------------------------------------------------------

    def iterate(self):
        now = ticks_ms()
        self.s = self.read_sensors()
        self.ensure_connected(now)

        # 1. Commands, non-blocking.
        for _ in range(MAX_MSGS_PER_LOOP):
            self.link.poll()
            if not self.inbox:
                break
            while self.inbox:
                suffix, payload = self.inbox.pop(0)
                self.handle(suffix, payload, now)

        # 2. Advance the active skill.
        if self.skill is not None:
            result = self.skill.step(self.s, now)
            if result is not None:
                self.finish_skill(result)

        # 3. Reflexes.
        reason = reflex_abort_reason(self.skill, self.s)
        if reason:
            self.end_skill(reason)
        self.check_watchdog(now)

        # 4. Events.
        if self.skill is not None or ticks_diff(self.ui.identify_until, now) > 0:
            self.events.hush(now, EV_HUSH_MOTION_MS)
        self.events.update(self.s, now)
        while self.events.queue:
            kind, data = self.events.queue.pop(0)
            msg = {"kind": kind, "seq": self.next_seq(), "t_ms": now}
            if data:
                msg["data"] = data
            self.link.publish("event", msg)

        # 5. State.
        self.publish_state(now)
        self.ui.set_battery(self.s["battery"])
        self.ui.tick(now)

    def run(self, alive=None):
        """Loop until alive() turns false, then stop and drop the connection."""
        while alive is None or alive():
            start = ticks_ms()
            self.iterate()
            rest = LOOP_MS - ticks_diff(ticks_ms(), start)
            if rest > 0:
                sleep_ms(rest)
        hw_motors_stop()
        self.link.close()

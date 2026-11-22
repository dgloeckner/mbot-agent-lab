# Wi-Fi and MQTT link. Connecting may block; the main loop stops the motors first.

from compat import json, ticks_ms, ticks_diff, sleep_ms
from hw import hw_wifi_connect, hw_wifi_ok
from umqtt.simple import MQTTClient

NET_KEEPALIVE_S = 30
NET_PING_MS = 10000
NET_WIFI_TIMEOUT_MS = 15000


class Link:
    def __init__(self, robot_id, host, port, password, tls, on_message):
        self.robot_id = robot_id
        self.prefix = "lab/" + robot_id + "/"
        self.host = host
        self.port = port
        self.password = password
        self.tls = tls
        self.on_message = on_message
        self.client = None
        self.up = False
        self.last_ping = 0

    def connect(self, wifi_ssid, wifi_password, hello):
        """Connect Wi-Fi (if needed) and the broker. Raises on failure."""
        if not hw_wifi_ok():
            hw_wifi_connect(wifi_ssid, wifi_password)
            start = ticks_ms()
            while not hw_wifi_ok():
                if ticks_diff(ticks_ms(), start) > NET_WIFI_TIMEOUT_MS:
                    raise OSError("wifi timeout")
                sleep_ms(100)
        c = MQTTClient(self.robot_id, self.host, port=self.port, user=self.robot_id,
                       password=self.password, keepalive=NET_KEEPALIVE_S, ssl=self.tls)
        c.set_callback(self._dispatch)
        c.set_last_will(self.prefix + "status", "offline", retain=True)
        c.connect()
        c.subscribe(self.prefix + "cmd")
        c.subscribe(self.prefix + "hb")
        self.client = c
        self.up = True
        self.last_ping = ticks_ms()
        self.publish_raw("status", "online", True)
        self.publish("hello", hello)

    def _dispatch(self, topic, msg):
        if isinstance(topic, bytes):
            topic = topic.decode()
        try:
            payload = json.loads(msg)
        except ValueError:
            return
        self.on_message(topic[len(self.prefix):], payload)

    def poll(self):
        """Handle at most one incoming message without blocking."""
        if not self.up:
            return
        try:
            self.client.check_msg()
            now = ticks_ms()
            if ticks_diff(now, self.last_ping) > NET_PING_MS:
                self.last_ping = now
                self.client.ping()
        except Exception:
            self.close()

    def publish(self, suffix, obj, retain=False):
        self.publish_raw(suffix, json.dumps(obj), retain)

    def publish_raw(self, suffix, text, retain=False):
        if not self.up:
            return
        try:
            self.client.publish(self.prefix + suffix, text, retain)
        except Exception:
            self.close()

    def close(self):
        self.up = False
        if self.client is not None:
            try:
                self.client.disconnect()
            except Exception:
                pass
        self.client = None

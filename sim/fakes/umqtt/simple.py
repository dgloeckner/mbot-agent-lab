"""Minimal MQTT 3.1.1 client with the API of MicroPython's umqtt.simple.

The simulator uses it so that the robot script talks real MQTT over a real
socket to the server, exactly like on the CyberPi. QoS 0 only.
"""

import select
import socket
import ssl as _ssl
import struct


class MQTTException(Exception):
    pass


def _encode_str(s):
    if isinstance(s, str):
        s = s.encode()
    return struct.pack("!H", len(s)) + s


def _encode_len(n):
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        out.append(b | (0x80 if n else 0))
        if not n:
            return bytes(out)


class MQTTClient:
    def __init__(self, client_id, server, port=0, user=None, password=None,
                 keepalive=0, ssl=False, ssl_params=None):
        self.client_id = client_id
        self.server = server
        self.port = port or (8883 if ssl else 1883)
        self.user = user
        self.pswd = password
        self.keepalive = keepalive
        self.ssl = ssl
        self.ssl_params = ssl_params or {}
        self.sock = None
        self.cb = None
        self.pid = 0
        self.lw_topic = None
        self.lw_msg = None
        self.lw_retain = False

    def set_callback(self, f):
        self.cb = f

    def set_last_will(self, topic, msg, retain=False, qos=0):
        self.lw_topic, self.lw_msg, self.lw_retain = topic, msg, retain

    def _send(self, ptype, body):
        self.sock.sendall(bytes([ptype]) + _encode_len(len(body)) + body)

    def _recv_exact(self, n):
        buf = b""
        while len(buf) < n:
            chunk = self.sock.recv(n - len(buf))
            if not chunk:
                raise OSError("connection closed")
            buf += chunk
        return buf

    def _read_packet(self):
        first = self._recv_exact(1)[0]
        n, shift = 0, 0
        while True:
            b = self._recv_exact(1)[0]
            n |= (b & 0x7F) << shift
            shift += 7
            if not b & 0x80:
                break
        return first, self._recv_exact(n) if n else b""

    def connect(self, clean_session=True):
        sock = socket.create_connection((self.server, self.port), timeout=10)
        if self.ssl:
            ctx = _ssl.create_default_context()
            sock = ctx.wrap_socket(sock, server_hostname=self.server)
        self.sock = sock
        flags = 0x02 if clean_session else 0
        payload = _encode_str(self.client_id)
        if self.lw_topic:
            flags |= 0x04 | (0x20 if self.lw_retain else 0)
            payload += _encode_str(self.lw_topic) + _encode_str(self.lw_msg)
        if self.user is not None:
            flags |= 0x80
            payload += _encode_str(self.user)
            if self.pswd is not None:
                flags |= 0x40
                payload += _encode_str(self.pswd)
        body = _encode_str("MQTT") + bytes([4, flags]) + struct.pack("!H", self.keepalive) + payload
        self._send(0x10, body)
        ptype, resp = self._read_packet()
        if ptype != 0x20 or len(resp) != 2:
            raise MQTTException("unexpected CONNACK")
        if resp[1] != 0:
            raise MQTTException(resp[1])
        return resp[0] & 1

    def disconnect(self):
        try:
            self._send(0xE0, b"")
        finally:
            self.sock.close()

    def ping(self):
        self._send(0xC0, b"")

    def publish(self, topic, msg, retain=False, qos=0):
        if isinstance(msg, str):
            msg = msg.encode()
        self._send(0x30 | (1 if retain else 0), _encode_str(topic) + msg)

    def subscribe(self, topic, qos=0):
        self.pid = (self.pid % 0xFFFF) + 1
        self._send(0x82, struct.pack("!H", self.pid) + _encode_str(topic) + bytes([qos]))
        while True:
            if self.wait_msg() == 0x90:
                return

    def wait_msg(self):
        """Block for one packet. Calls the callback for PUBLISH; returns the packet type."""
        first, body = self._read_packet()
        ptype = first & 0xF0
        if ptype == 0x30:
            tlen = struct.unpack("!H", body[:2])[0]
            topic = body[2:2 + tlen]
            pos = 2 + tlen
            if first & 0x06:  # QoS > 0: skip the packet id
                pos += 2
            if self.cb:
                self.cb(topic, body[pos:])
        return ptype

    def check_msg(self):
        """Handle one pending packet if there is one, without blocking."""
        readable, _, _ = select.select([self.sock], [], [], 0)
        if readable or (self.ssl and self.sock.pending()):
            return self.wait_msg()
        return None

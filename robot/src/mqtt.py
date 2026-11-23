# Minimal MQTT 3.1.1 client, QoS 0, with the API of umqtt.simple. The CyberPi
# firmware ships no umqtt, so the robot brings its own. Runs on MicroPython and
# CPython (the simulator uses it against the real broker).

import socket


def _mq_str(s):
    if isinstance(s, str):
        s = s.encode()
    return bytes([len(s) >> 8, len(s) & 255]) + s


class MQTTClient:
    def __init__(self, client_id, server, port=1883, user=None, password=None, keepalive=0, ssl=False):
        self.client_id = client_id
        self.server = server
        self.port = port
        self.user = user
        self.pswd = password
        self.keepalive = keepalive
        self.ssl = ssl
        self.sock = None
        self.cb = None
        self.pid = 0
        self.lw = None

    def set_callback(self, f):
        self.cb = f

    def set_last_will(self, topic, msg, retain=False, qos=0):
        self.lw = (topic, msg, retain)

    def _w(self, ptype, body):
        n = len(body)
        head = bytearray([ptype])
        while True:
            head.append((n & 127) | (128 if n > 127 else 0))
            n >>= 7
            if not n:
                break
        # MicroPython sockets and TLS streams write(); a plain CPython socket sendall()s.
        (getattr(self.sock, "write", None) or self.sock.sendall)(bytes(head) + body)

    def _rd(self, n):
        return (getattr(self.sock, "read", None) or self.sock.recv)(n)

    def _r(self, n):
        buf = b""
        while len(buf) < n:
            b = self._rd(n - len(buf))
            if not b:
                raise OSError("mqtt closed")
            buf += b
        return buf

    def _packet(self, first):
        n, shift = 0, 0
        while True:
            b = self._r(1)[0]
            n |= (b & 127) << shift
            shift += 7
            if b < 128:
                break
        body = self._r(n) if n else b""
        if first & 0xF0 == 0x30 and self.cb:
            t = (body[0] << 8) | body[1]
            self.cb(body[2:2 + t], body[2 + t + (2 if first & 6 else 0):])
        return first & 0xF0

    def connect(self, clean_session=True):
        addr = socket.getaddrinfo(self.server, self.port)[0][-1]
        s = socket.socket()
        s.settimeout(10)
        s.connect(addr)
        if self.ssl:
            import ssl
            if hasattr(ssl, "create_default_context"):  # CPython
                s = ssl.create_default_context().wrap_socket(s, server_hostname=self.server)
            else:  # MicroPython. VERIFY: certificate checking on the CyberPi
                s = ssl.wrap_socket(s, server_hostname=self.server)
        self.sock = s
        flags = 2 if clean_session else 0
        pl = _mq_str(self.client_id)
        if self.lw:
            flags |= 4 | (32 if self.lw[2] else 0)
            pl += _mq_str(self.lw[0]) + _mq_str(self.lw[1])
        if self.user is not None:
            flags |= 128
            pl += _mq_str(self.user)
            if self.pswd is not None:
                flags |= 64
                pl += _mq_str(self.pswd)
        self._w(0x10, _mq_str("MQTT") + bytes([4, flags, self.keepalive >> 8, self.keepalive & 255]) + pl)
        if self._r(1)[0] != 0x20:
            raise OSError("mqtt no connack")
        ack = self._r(3)
        if ack[2]:
            raise OSError("mqtt refused %d" % ack[2])

    def disconnect(self):
        try:
            self._w(0xE0, b"")
        finally:
            self.sock.close()

    def ping(self):
        self._w(0xC0, b"")

    def publish(self, topic, msg, retain=False, qos=0):
        if isinstance(msg, str):
            msg = msg.encode()
        self._w(0x31 if retain else 0x30, _mq_str(topic) + msg)

    def subscribe(self, topic, qos=0):
        self.pid = self.pid % 65535 + 1
        self._w(0x82, bytes([self.pid >> 8, self.pid & 255]) + _mq_str(topic) + bytes([0]))
        while self._packet(self._r(1)[0]) != 0x90:
            pass

    def check_msg(self):
        """Handle one waiting packet without blocking; None if there is none."""
        self.sock.setblocking(False)
        try:
            b = self._rd(1)
        except OSError:  # CPython: no data yet (EAGAIN); real errors show on the next write
            b = None
        finally:
            self.sock.setblocking(True)
        if b is None:  # MicroPython: no data yet
            return None
        if not b:
            raise OSError("mqtt closed")
        return self._packet(b[0])

"""A TCP proxy that delays everything it forwards, in both directions.

It stands in for the network between the server (Azure VM) and a robot in the
guest Wi-Fi: with --delay-ms every MQTT packet, a command as well as its
result, arrives that much later, so nothing works in the simulator that would
come too late on a real robot. Order is kept.
"""

import queue
import socket
import threading
import time


class DelayProxy:
    def __init__(self, target_host, target_port, delay_s, listen_host="127.0.0.1"):
        self.target = (target_host, target_port)
        self.delay = delay_s
        self.server = socket.socket()
        self.server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.server.bind((listen_host, 0))
        self.server.listen(4)
        self.port = self.server.getsockname()[1]
        threading.Thread(target=self._accept, daemon=True).start()

    def _accept(self):
        while True:
            try:
                client, _ = self.server.accept()
            except OSError:
                return
            try:
                upstream = socket.create_connection(self.target)
            except OSError:
                client.close()
                continue
            for src, dst in ((client, upstream), (upstream, client)):
                self._pipe(src, dst)

    def _pipe(self, src, dst):
        q = queue.Queue()

        def read():
            while True:
                try:
                    data = src.recv(4096)
                except OSError:
                    data = b""
                q.put((time.monotonic() + self.delay, data))
                if not data:
                    return

        def write():
            while True:
                due, data = q.get()
                wait = due - time.monotonic()
                if wait > 0:
                    time.sleep(wait)
                if not data:
                    try:
                        dst.shutdown(socket.SHUT_WR)
                    except OSError:
                        pass
                    return
                try:
                    dst.sendall(data)
                except OSError:
                    return

        threading.Thread(target=read, daemon=True).start()
        threading.Thread(target=write, daemon=True).start()

    def close(self):
        self.server.close()

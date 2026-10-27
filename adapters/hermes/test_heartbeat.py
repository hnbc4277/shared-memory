"""Hermes MemoryProvider posts connection heartbeat while the plugin is loaded."""

from __future__ import annotations

import json
import os
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from __init__ import SharedMemoryProvider


class TestConnectionHeartbeat(unittest.TestCase):
    def test_initialize_posts_adapter_heartbeat(self) -> None:
        received: list[tuple[str, dict, str]] = []
        ready = threading.Event()

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:  # noqa: N802
                n = int(self.headers.get("Content-Length") or 0)
                raw = self.rfile.read(n) if n else b"{}"
                body = json.loads(raw.decode("utf-8") or "{}")
                received.append((self.path, body, self.headers.get("Authorization") or ""))
                ready.set()
                self.send_response(204)
                self.end_headers()

            def log_message(self, format: str, *args: object) -> None:  # noqa: A003
                return

        httpd = HTTPServer(("127.0.0.1", 0), Handler)
        port = httpd.server_address[1]
        thread = threading.Thread(target=httpd.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(httpd.shutdown)
        self.addCleanup(httpd.server_close)

        os.environ["SHARED_MEMORY_API_URL"] = f"http://127.0.0.1:{port}"
        os.environ["SHARED_MEMORY_API_KEY"] = "sm_test"
        os.environ["SHARED_MEMORY_CONNECTION_ID"] = "conn-hermes"
        os.environ["SHARED_MEMORY_HEARTBEAT_INTERVAL"] = "1"

        provider = SharedMemoryProvider()
        provider.initialize("sess-1")
        self.addCleanup(provider.shutdown)

        self.assertTrue(ready.wait(5), "expected heartbeat POST")
        path, body, auth = received[0]
        self.assertEqual(path, "/v1/agents/connections/heartbeat")
        self.assertEqual(body.get("client"), "adapter")
        self.assertEqual(body.get("connection_id"), "conn-hermes")
        self.assertIn("Bearer sm_test", auth)
        # ticker should not block shutdown
        t0 = time.time()
        provider.shutdown()
        self.assertLess(time.time() - t0, 2.0)


if __name__ == "__main__":
    unittest.main()

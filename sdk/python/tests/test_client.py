"""Tests for sharedmemory.Client using urllib mock (http.server)."""

from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from sharedmemory import APIError, Client


class _Handler(BaseHTTPRequestHandler):
    last_auth = ""
    last_ingest = None

    def log_message(self, format: str, *args) -> None:  # noqa: A003
        return

    def _read_json(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        return json.loads(raw.decode("utf-8")) if raw else None

    def _write(self, status: int, body=None):
        data = b"" if body is None else json.dumps(body).encode("utf-8")
        self.send_response(status)
        if body is not None:
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        if data:
            self.wfile.write(data)

    def do_POST(self):  # noqa: N802
        _Handler.last_auth = self.headers.get("Authorization", "")
        if self.path == "/v1/ingest":
            _Handler.last_ingest = self._read_json()
            self._write(
                201,
                {
                    "session_id": "s1",
                    "message_id": "m1",
                    "seq": 1,
                    "warnings": [],
                    "prompts": [],
                },
            )
            return
        if self.path == "/v1/recall":
            self._read_json()
            self._write(
                200,
                {
                    "profile": "p",
                    "running_summary": "",
                    "hits": [],
                    "context": "ctx",
                    "inject_enabled": True,
                    "warnings": [],
                },
            )
            return
        if self.path.startswith("/v1/vault/candidates/"):
            self._read_json()
            self._write(204)
            return
        if self.path == "/v1/device/code":
            self._read_json()
            self._write(
                200,
                {
                    "device_code": "dc",
                    "user_code": "ABCD1234",
                    "verification_uri": "http://x/device",
                    "verification_uri_complete": "http://x/device?user_code=ABCD1234",
                    "expires_in": 600,
                    "interval": 5,
                },
            )
            return
        self._write(404, {"error": "not found"})

    def do_GET(self):  # noqa: N802
        _Handler.last_auth = self.headers.get("Authorization", "")
        if self.path.startswith("/v1/sessions"):
            self._write(200, [{"id": "s1", "agent": "cursor"}])
            return
        if self.path == "/v1/candidates":
            self._write(200, {"vault": [], "artifacts": []})
            return
        self._write(404, {"error": "not found"})


@pytest.fixture()
def api_server():
    server = HTTPServer(("127.0.0.1", 0), _Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    host, port = server.server_address
    yield f"http://{host}:{port}"
    server.shutdown()


def test_client_ingest_recall_candidates(api_server: str):
    c = Client(base_url=api_server, api_key="k")
    ing = c.ingest(
        agent="cursor",
        skills=[],
        plugins=[],
        tools=[],
        role="user",
        content="hi",
        external_session_id="cursor:1",
    )
    assert _Handler.last_auth == "Bearer k"
    assert _Handler.last_ingest["content"] == "hi"
    assert ing["session_id"] == "s1"

    rec = c.recall("prefs", top_k=5)
    assert rec["context"] == "ctx"

    sessions = c.list_sessions(agent="cursor")
    assert sessions[0]["id"] == "s1"

    cands = c.list_candidates()
    assert cands["vault"] == []
    assert c.list_candidates()["artifacts"] == []

    assert c.resolve_candidate("vault", "c1", "reject") is None

    code = c.create_device_code("daemon", agent="cursor")
    assert code["user_code"] == "ABCD1234"


def test_api_error():
    class ErrHandler(BaseHTTPRequestHandler):
        def log_message(self, format: str, *args) -> None:  # noqa: A003
            return

        def do_POST(self):  # noqa: N802
            body = b'{"error":"unauthorized"}'
            self.send_response(401)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = HTTPServer(("127.0.0.1", 0), ErrHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    host, port = server.server_address
    try:
        c = Client(base_url=f"http://{host}:{port}", api_key="bad")
        with pytest.raises(APIError) as ei:
            c.recall("x")
        assert ei.value.status == 401
    finally:
        server.shutdown()

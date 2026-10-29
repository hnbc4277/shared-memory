"""Shared Memory HTTP client."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, MutableMapping, Optional


class APIError(Exception):
    def __init__(self, status: int, body: str) -> None:
        self.status = status
        self.body = body
        super().__init__(f"shared memory api: {status} {body.strip()}")


class Client:
    """HTTP client for SHARED_MEMORY_API_URL with Bearer SHARED_MEMORY_API_KEY."""

    def __init__(
        self,
        base_url: Optional[str] = None,
        api_key: Optional[str] = None,
        timeout: float = 30.0,
    ) -> None:
        raw = (base_url if base_url is not None else os.environ.get("SHARED_MEMORY_API_URL", "")).rstrip(
            "/"
        )
        self.base_url = raw
        self.api_key = api_key if api_key is not None else os.environ.get("SHARED_MEMORY_API_KEY", "")
        self.timeout = timeout

    def _merge_connection_id(self, body: dict[str, Any]) -> dict[str, Any]:
        cid = os.environ.get("SHARED_MEMORY_CONNECTION_ID", "").strip()
        if cid and not body.get("connection_id"):
            body = {**body, "connection_id": cid}
        return body

    def _request(
        self,
        method: str,
        path: str,
        body: Any = None,
    ) -> Any:
        data: Optional[bytes] = None
        headers: MutableMapping[str, str] = {}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(
            f"{self.base_url}{path}",
            data=data,
            headers=headers,
            method=method,
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
                if resp.status == 204 or not raw:
                    return None
                return json.loads(raw.decode("utf-8"))
        except urllib.error.HTTPError as e:
            err_body = e.read().decode("utf-8", errors="replace")
            raise APIError(e.code, err_body) from e

    def ingest(self, **fields: Any) -> dict[str, Any]:
        """POST /v1/ingest. Pass session_id, skills, plugins, tools, role, content, etc."""
        return self._request("POST", "/v1/ingest", self._merge_connection_id(dict(fields)))

    def recall(
        self,
        query: str,
        project_id: Optional[str] = None,
        top_k: Optional[int] = None,
        connection_id: Optional[str] = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {"query": query}
        if project_id:
            body["project_id"] = project_id
        if top_k is not None:
            body["top_k"] = top_k
        if connection_id:
            body["connection_id"] = connection_id
        return self._request("POST", "/v1/recall", self._merge_connection_id(body))

    def list_sessions(self, **filters: Any) -> list[dict[str, Any]]:
        q = urllib.parse.urlencode({k: v for k, v in filters.items() if v is not None and v != ""})
        path = f"/v1/sessions?{q}" if q else "/v1/sessions"
        out = self._request("GET", path)
        return out if isinstance(out, list) else []

    def get_session(self, session_id: str) -> dict[str, Any]:
        return self._request("GET", f"/v1/sessions/{urllib.parse.quote(session_id)}")

    def list_candidates(self) -> dict[str, Any]:
        return self._request("GET", "/v1/candidates")

    def resolve_candidate(
        self,
        kind: str,
        candidate_id: str,
        decision: str,
    ) -> Any:
        kind = kind.lower().strip()
        decision = decision.lower().strip()
        if kind not in ("vault", "artifact"):
            raise ValueError("kind must be vault or artifact")
        if decision not in ("accept", "reject"):
            raise ValueError("decision must be accept or reject")
        prefix = (
            "/v1/artifacts/candidates/"
            if kind == "artifact"
            else "/v1/vault/candidates/"
        )
        return self._request(
            "POST",
            f"{prefix}{urllib.parse.quote(candidate_id)}/{decision}",
            {},
        )

    def create_device_code(
        self,
        client_name: str,
        agent: Optional[str] = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {"client_name": client_name}
        if agent:
            body["agent"] = agent
        return self._request("POST", "/v1/device/code", body)

    def poll_device_token(self, device_code: str) -> dict[str, Any]:
        return self._request("POST", "/v1/device/token", {"device_code": device_code})

    def heartbeat(self, daemon_version: str = "", spool_pending: int = 0) -> None:
        self._request(
            "POST",
            "/v1/devices/heartbeat",
            {"daemon_version": daemon_version, "spool_pending": spool_pending},
        )


__all__ = ["APIError", "Client"]

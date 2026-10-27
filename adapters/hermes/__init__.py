"""Hermes MemoryProvider plugin for Shared Memory."""

from __future__ import annotations

import logging
import os
import sys
import threading
from pathlib import Path
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)

_AGENT = "hermes"


def _provenance(kwargs: Dict[str, Any], messages: Optional[List[Dict[str, Any]]]) -> Dict[str, Any]:
    skills = [str(x) for x in (kwargs.get("skills") or []) if str(x).strip()]
    plugins = [str(x) for x in (kwargs.get("plugins") or []) if str(x).strip()]
    tools = [str(x) for x in (kwargs.get("tools") or []) if str(x).strip()]
    model = str(kwargs.get("model") or "").strip() or "unknown"
    if messages:
        for msg in messages:
            if not isinstance(msg, dict):
                continue
            role = str(msg.get("role") or "")
            name = str(msg.get("name") or msg.get("tool_name") or "").strip()
            if role in ("tool", "function") and name:
                if name not in tools:
                    tools.append(name)
            for key in ("skills", "plugins", "tools"):
                raw = msg.get(key)
                if isinstance(raw, list):
                    dest = skills if key == "skills" else plugins if key == "plugins" else tools
                    for x in raw:
                        s = str(x).strip()
                        if s and s not in dest:
                            dest.append(s)
            if model == "unknown":
                m = str(msg.get("model") or "").strip()
                if m:
                    model = m
    return {"skills": skills, "plugins": plugins, "tools": tools, "model": model}


def _ensure_sdk() -> None:
    """Prefer installed `sharedmemory`; fall back to repo-relative sdk/python."""
    try:
        import sharedmemory  # noqa: F401

        return
    except ImportError:
        pass
    here = Path(__file__).resolve().parent
    for candidate in (
        here / ".." / ".." / "sdk" / "python",
        here.parent.parent / "sdk" / "python",
    ):
        p = candidate.resolve()
        if (p / "sharedmemory").is_dir() and str(p) not in sys.path:
            sys.path.insert(0, str(p))
            return


_ensure_sdk()
from sharedmemory import Client  # noqa: E402


def _heartbeat_interval_sec() -> float:
    raw = os.environ.get("SHARED_MEMORY_HEARTBEAT_INTERVAL", "").strip()
    if raw:
        try:
            v = float(raw)
            if v > 0:
                return v
        except ValueError:
            pass
    return 30.0


class SharedMemoryProvider:
    """Thin MemoryProvider that only calls the Shared Memory Python SDK."""

    def __init__(self) -> None:
        self._client: Optional[Client] = None
        self._session_id = ""
        self._agent_context = "primary"
        self._sync_thread: Optional[threading.Thread] = None
        self._last_prefetch = ""
        self._hb_stop: Optional[threading.Event] = None
        self._hb_thread: Optional[threading.Thread] = None

    @property
    def name(self) -> str:
        return "shared_memory"

    def is_available(self) -> bool:
        # Credentials only — no network. URL falls back to local daemon in Client.
        return bool(os.environ.get("SHARED_MEMORY_API_KEY"))

    def unavailable_reason(self) -> str:
        if not os.environ.get("SHARED_MEMORY_API_KEY"):
            return "Set SHARED_MEMORY_API_KEY (and usually SHARED_MEMORY_API_URL)."
        return ""

    def initialize(self, session_id: str, **kwargs: Any) -> None:
        self._session_id = session_id or ""
        self._agent_context = str(kwargs.get("agent_context") or "primary")
        self._client = Client(
            base_url=os.environ.get("SHARED_MEMORY_API_URL") or "http://127.0.0.1:18765",
            api_key=os.environ.get("SHARED_MEMORY_API_KEY") or None,
        )
        self._start_heartbeat()

    def get_config_schema(self) -> List[Dict[str, Any]]:
        return [
            {
                "key": "api_key",
                "description": "Shared Memory API key",
                "secret": True,
                "required": True,
                "env_var": "SHARED_MEMORY_API_KEY",
            },
            {
                "key": "api_url",
                "description": "Shared Memory API base URL (or local daemon)",
                "secret": False,
                "required": False,
                "env_var": "SHARED_MEMORY_API_URL",
                "default": "http://127.0.0.1:18765",
            },
        ]

    def save_config(self, values: Dict[str, Any], hermes_home: str) -> None:
        # Secrets go to .env via Hermes; nothing else to persist.
        return

    def system_prompt_block(self) -> str:
        return (
            "Shared Memory is active. Relevant cross-session context may be "
            "injected before each turn via prefetch."
        )

    def get_tool_schemas(self) -> List[Dict[str, Any]]:
        return []

    def handle_tool_call(self, tool_name: str, args: Dict[str, Any], **kwargs: Any) -> str:
        return "{}"

    def prefetch(self, query: str, *, session_id: str = "") -> str:
        if not self._client or not (query or "").strip():
            return ""
        try:
            result = self._client.recall(query=query)
            ctx = ""
            if isinstance(result, dict):
                ctx = str(result.get("context") or "")
            self._last_prefetch = ctx
            return ctx
        except Exception as e:
            logger.warning("shared_memory prefetch failed: %s", e)
            return ""

    def sync_turn(
        self,
        user_content: str,
        assistant_content: str,
        *,
        session_id: str = "",
        messages: Optional[List[Dict[str, Any]]] = None,
        **kwargs: Any,
    ) -> None:
        if self._agent_context and self._agent_context != "primary":
            return
        if not self._client:
            return
        sid = session_id or self._session_id
        client = self._client

        prov = _provenance(kwargs, messages)

        def _sync() -> None:
            try:
                if user_content:
                    client.ingest(
                        agent=_AGENT,
                        agent_path=_AGENT,
                        role="user",
                        content=user_content,
                        external_session_id=sid or None,
                        skills=prov["skills"],
                        plugins=prov["plugins"],
                        tools=prov["tools"],
                        model=prov["model"],
                    )
                if assistant_content:
                    client.ingest(
                        agent=_AGENT,
                        agent_path=_AGENT,
                        role="assistant",
                        content=assistant_content,
                        external_session_id=sid or None,
                        skills=prov["skills"],
                        plugins=prov["plugins"],
                        tools=prov["tools"],
                        model=prov["model"],
                    )
            except Exception as e:
                logger.warning("shared_memory sync_turn failed: %s", e)

        if self._sync_thread and self._sync_thread.is_alive():
            self._sync_thread.join(timeout=5.0)
        self._sync_thread = threading.Thread(target=_sync, name="shared_memory-sync", daemon=True)
        self._sync_thread.start()

    def on_delegation(self, task: str, result: str, *, child_session_id: str = "", **kwargs: Any) -> None:
        if not self._client:
            return
        sub = (
            str(kwargs.get("agent_identity") or kwargs.get("subagent_name") or "")
            or (child_session_id[:32] if child_session_id else "sub")
        )
        path = f"{_AGENT}/{sub}"
        sid = child_session_id or self._session_id
        prov = _provenance(kwargs, None)
        try:
            if task:
                self._client.ingest(
                    agent=_AGENT,
                    agent_path=path,
                    role="user",
                    content=task,
                    external_session_id=sid or None,
                    skills=prov["skills"],
                    plugins=prov["plugins"],
                    tools=prov["tools"],
                    model=prov["model"],
                )
            if result:
                self._client.ingest(
                    agent=_AGENT,
                    agent_path=path,
                    role="assistant",
                    content=result,
                    external_session_id=sid or None,
                    skills=prov["skills"],
                    plugins=prov["plugins"],
                    tools=prov["tools"],
                    model=prov["model"],
                )
        except Exception as e:
            logger.warning("shared_memory on_delegation failed: %s", e)

    def on_session_end(self, messages: List[Dict[str, Any]]) -> None:
        # Turns already synced; nothing extra required.
        return

    def shutdown(self) -> None:
        self._stop_heartbeat()
        if self._sync_thread and self._sync_thread.is_alive():
            self._sync_thread.join(timeout=5.0)
        self._client = None

    def _start_heartbeat(self) -> None:
        self._stop_heartbeat()
        if not self._client or not self._client.api_key or not self._client.base_url:
            return
        stop = threading.Event()
        interval = _heartbeat_interval_sec()

        def _loop() -> None:
            while not stop.is_set():
                self._post_heartbeat()
                if stop.wait(interval):
                    break

        self._hb_stop = stop
        self._hb_thread = threading.Thread(target=_loop, name="shared_memory-heartbeat", daemon=True)
        self._hb_thread.start()

    def _stop_heartbeat(self) -> None:
        if self._hb_stop is not None:
            self._hb_stop.set()
        if self._hb_thread is not None and self._hb_thread.is_alive():
            self._hb_thread.join(timeout=1.5)
        self._hb_stop = None
        self._hb_thread = None

    def _post_heartbeat(self) -> None:
        client = self._client
        if not client or not client.api_key or not client.base_url:
            return
        body: Dict[str, Any] = {"client": "adapter"}
        cid = os.environ.get("SHARED_MEMORY_CONNECTION_ID", "").strip()
        if cid:
            body["connection_id"] = cid
        try:
            client._request("POST", "/v1/agents/connections/heartbeat", body)
        except Exception as e:
            logger.debug("shared_memory heartbeat failed: %s", e)


def register(ctx: Any) -> None:
    """Hermes memory plugin entry: ctx.register_memory_provider(...)."""
    ctx.register_memory_provider(SharedMemoryProvider())

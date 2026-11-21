"""Minimal HTTP runtime for the orchestrator container.

This service exposes process and dependency health only. It deliberately does
not accept events or mutate graph state; event handling remains inside the
LangGraph control plane.
"""

from __future__ import annotations

import json
import logging
import os
import signal
import socket
import threading
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

LOGGER = logging.getLogger("orchestrator.service")


@dataclass(frozen=True)
class RuntimeConfig:
    host: str
    port: int
    graph_host: str
    graph_port: int
    graph_password: str | None
    graph_required: bool

    @classmethod
    def from_env(cls) -> RuntimeConfig:
        return cls(
            host=os.getenv("ORCHESTRATOR_HOST", "0.0.0.0"),
            port=_env_int("ORCHESTRATOR_PORT", 8080),
            graph_host=os.getenv("FALKORDB_HOST", "falkordb"),
            graph_port=_env_int("FALKORDB_PORT", 6379),
            graph_password=os.getenv("FALKORDB_PASSWORD") or None,
            graph_required=_env_bool("GRAPH_REQUIRED", True),
        )


def _env_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None:
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if not 1 <= value <= 65535:
        raise ValueError(f"{name} must be between 1 and 65535")
    return value


def _env_bool(name: str, default: bool) -> bool:
    raw = os.getenv(name)
    if raw is None:
        return default
    normalized = raw.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"{name} must be a boolean")


def _resp_command(*parts: bytes) -> bytes:
    """Encode a RESP command without interpolating untrusted values into a shell."""
    encoded = [f"*{len(parts)}\r\n".encode()]
    for part in parts:
        encoded.append(f"${len(part)}\r\n".encode())
        encoded.append(part)
        encoded.append(b"\r\n")
    return b"".join(encoded)


def check_falkordb(
    host: str,
    port: int,
    password: str | None,
    *,
    timeout: float = 1.5,
) -> bool:
    """Return True only when FalkorDB answers an authenticated PING."""
    try:
        with socket.create_connection((host, port), timeout=timeout) as connection:
            connection.settimeout(timeout)
            with connection.makefile("rwb") as stream:
                if password:
                    stream.write(_resp_command(b"AUTH", password.encode()))
                    stream.flush()
                    if stream.readline().strip() != b"+OK":
                        return False
                stream.write(_resp_command(b"PING"))
                stream.flush()
                return stream.readline().strip() == b"+PONG"
    except (OSError, UnicodeError, ValueError):
        return False


class OrchestratorServer(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(
        self,
        server_address: tuple[str, int],
        request_handler: type[BaseHTTPRequestHandler],
        config: RuntimeConfig,
    ) -> None:
        self.config = config
        super().__init__(server_address, request_handler)


class _Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server: OrchestratorServer

    def do_GET(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
        path = self.path.split("?", 1)[0]
        if path == "/healthz":
            self._send(200, {"status": "ok", "service": "orchestrator"})
            return
        if path == "/readyz":
            self._ready()
            return
        self._send(404, {"error": "not_found"})

    def _ready(self) -> None:
        config = self.server.config
        if not config.graph_required:
            self._send(200, {"status": "ready", "graph": "not_required"})
            return
        reachable = check_falkordb(
            config.graph_host,
            config.graph_port,
            config.graph_password,
        )
        if reachable:
            self._send(200, {"status": "ready", "graph": "reachable"})
        else:
            self._send(503, {"status": "not_ready", "graph": "unreachable"})

    def _send(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:
        LOGGER.info("%s - %s", self.address_string(), format % args)


def create_server(config: RuntimeConfig) -> OrchestratorServer:
    return OrchestratorServer((config.host, config.port), _Handler, config)


def main() -> None:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO").upper(),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    config = RuntimeConfig.from_env()
    server = create_server(config)

    def stop(*_: object) -> None:
        threading.Thread(target=server.shutdown, daemon=True).start()

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)

    LOGGER.info("orchestrator health service listening on %s:%d", config.host, config.port)
    try:
        server.serve_forever(poll_interval=0.25)
    finally:
        server.server_close()
        LOGGER.info("orchestrator health service stopped")


if __name__ == "__main__":
    main()

import json
import threading
from http.client import HTTPConnection

import pytest

from orchestrator import service


def start_server(config: service.RuntimeConfig) -> service.OrchestratorServer:
    server = service.create_server(config)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


def get(server: service.OrchestratorServer, path: str) -> tuple[int, dict]:
    host, port = server.server_address
    connection = HTTPConnection(host, port, timeout=2)
    try:
        connection.request("GET", path)
        response = connection.getresponse()
        return response.status, json.loads(response.read())
    finally:
        connection.close()


def config(*, graph_required: bool) -> service.RuntimeConfig:
    return service.RuntimeConfig(
        host="127.0.0.1",
        port=0,
        graph_host="127.0.0.1",
        graph_port=1,
        graph_password=None,
        graph_required=graph_required,
    )


def test_healthz_reports_process_health():
    server = start_server(config(graph_required=False))
    try:
        status, body = get(server, "/healthz")
        assert status == 200
        assert body == {"status": "ok", "service": "orchestrator"}
    finally:
        server.shutdown()
        server.server_close()


def test_readyz_can_skip_graph_dependency():
    server = start_server(config(graph_required=False))
    try:
        status, body = get(server, "/readyz")
        assert status == 200
        assert body["graph"] == "not_required"
    finally:
        server.shutdown()
        server.server_close()


def test_readyz_fails_when_graph_is_unreachable():
    server = start_server(config(graph_required=True))
    try:
        status, body = get(server, "/readyz")
        assert status == 503
        assert body["graph"] == "unreachable"
    finally:
        server.shutdown()
        server.server_close()


def test_unknown_route_is_not_found():
    server = start_server(config(graph_required=False))
    try:
        status, body = get(server, "/events")
        assert status == 404
        assert body == {"error": "not_found"}
    finally:
        server.shutdown()
        server.server_close()


def test_runtime_config_rejects_invalid_ports(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setenv("FALKORDB_PORT", "not-a-port")
    with pytest.raises(ValueError, match="FALKORDB_PORT"):
        service.RuntimeConfig.from_env()


def test_resp_command_uses_length_prefixes():
    assert service._resp_command(b"AUTH", b"p@ss word") == (
        b"*2\r\n$4\r\nAUTH\r\n$9\r\np@ss word\r\n"
    )

"""Contract tests for db/graph_client.py: env validation and episode sanitization.

No live FalkorDB or LLM key is required — these cover only the pure functions.
Live-connectivity verification is a separate manual step; see db/CLAUDE.md Verify.
"""

from __future__ import annotations

import json

import pytest

from db.graph_client import (
    MAX_EPISODE_BODY_CHARS,
    GraphConfigError,
    build_episode_body,
    load_graph_config,
)

BASE_ENV = {
    "FALKORDB_HOST": "127.0.0.1",
    "FALKORDB_PORT": "6379",
    "FALKORDB_DATABASE": "sentinel_lite",
    "GRAPHITI_GROUP_ID": "sentinel_lite_main",
}


def test_load_graph_config_reads_required_vars():
    config = load_graph_config(BASE_ENV)
    assert config.host == "127.0.0.1"
    assert config.port == 6379
    assert config.group_id == "sentinel_lite_main"
    assert config.max_coroutines == 4


def test_load_graph_config_missing_var_raises():
    env = {k: v for k, v in BASE_ENV.items() if k != "GRAPHITI_GROUP_ID"}
    with pytest.raises(GraphConfigError, match="GRAPHITI_GROUP_ID"):
        load_graph_config(env)


def test_load_graph_config_rejects_non_loopback_host():
    env = {**BASE_ENV, "FALKORDB_HOST": "0.0.0.0"}
    with pytest.raises(GraphConfigError, match="loopback"):
        load_graph_config(env)


def test_load_graph_config_rejects_bad_port():
    env = {**BASE_ENV, "FALKORDB_PORT": "not-a-port"}
    with pytest.raises(GraphConfigError):
        load_graph_config(env)


def test_load_graph_config_rejects_zero_coroutines():
    env = {**BASE_ENV, "GRAPHITI_MAX_COROUTINES": "0"}
    with pytest.raises(GraphConfigError, match="GRAPHITI_MAX_COROUTINES"):
        load_graph_config(env)


def test_build_episode_body_is_valid_json_with_episode_type():
    body = build_episode_body("WORK_BLOCK_COMPLETED", {"block_id": "block-0-core"})
    parsed = json.loads(body)
    assert parsed["episode_type"] == "WORK_BLOCK_COMPLETED"
    assert parsed["block_id"] == "block-0-core"


def test_build_episode_body_truncates_long_strings():
    huge = "x" * (MAX_EPISODE_BODY_CHARS + 500)
    body = build_episode_body("WORK_BLOCK_BLOCKED", {"error_trace": huge})
    parsed = json.loads(body)
    assert len(parsed["error_trace"]) <= MAX_EPISODE_BODY_CHARS + len("...<truncated>")
    assert parsed["error_trace"].endswith("...<truncated>")


def test_build_episode_body_truncates_strings_inside_lists():
    huge = "y" * (MAX_EPISODE_BODY_CHARS + 10)
    body = build_episode_body("WORK_BLOCK_COMPLETED", {"lessons_learned": [huge, "short"]})
    parsed = json.loads(body)
    assert parsed["lessons_learned"][0].endswith("...<truncated>")
    assert parsed["lessons_learned"][1] == "short"

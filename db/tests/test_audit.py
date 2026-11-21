"""Contract tests for db/audit.py: the local SQLite record of every episode."""

from __future__ import annotations

import json

import pytest

from db.audit import AuditRecordError, mark_graph_write_status, pending_episodes, record_episode


@pytest.fixture
def db_path(tmp_path):
    return tmp_path / "audit-test.sqlite3"


def test_record_episode_returns_row_id(db_path):
    row_id = record_episode(
        block_id="block-0-core",
        group_id="sentinel_lite_main",
        episode_type="WORK_BLOCK_COMPLETED",
        payload_json=json.dumps({"episode_type": "WORK_BLOCK_COMPLETED"}),
        db_path=db_path,
    )
    assert row_id == 1


def test_record_episode_rejects_empty_block_id(db_path):
    with pytest.raises(AuditRecordError, match="block_id"):
        record_episode(
            block_id="",
            group_id="sentinel_lite_main",
            episode_type="WORK_BLOCK_COMPLETED",
            payload_json="{}",
            db_path=db_path,
        )


def test_new_record_defaults_to_pending(db_path):
    record_episode(
        block_id="block-1-sensor",
        group_id="sentinel_lite_main",
        episode_type="WORK_BLOCK_BLOCKED",
        payload_json="{}",
        db_path=db_path,
    )
    pending = pending_episodes(db_path)
    assert len(pending) == 1
    assert pending[0]["block_id"] == "block-1-sensor"
    assert pending[0]["graph_write_status"] == "pending"


def test_mark_committed_removes_row_from_pending(db_path):
    row_id = record_episode(
        block_id="block-2-agent",
        group_id="sentinel_lite_main",
        episode_type="WORK_BLOCK_COMPLETED",
        payload_json="{}",
        db_path=db_path,
    )
    mark_graph_write_status(row_id, "committed", db_path=db_path)
    assert pending_episodes(db_path) == []


def test_mark_graph_write_status_rejects_unknown_row_id(db_path):
    with pytest.raises(AuditRecordError, match="no audit record"):
        mark_graph_write_status(9999, "committed", db_path=db_path)

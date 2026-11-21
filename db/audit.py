"""SQLite audit log for every episode sent to the graph DB.

Graphiti's LLM extraction is lossy by design: it keeps the entities and edges
it derives, not the original payload. This module keeps the original payload
too, append-only, so a bad extraction, a prompt change, or an LLM/provider
outage can be replayed instead of losing the record. Local and file-based —
no extra infra — matching every other component in this repo keeping its own
state private (hunterd's BadgerDB, orchestrator's LangGraph checkpoints).

See db/DATA_MODEL.md "Where each field lives" for why this exists alongside
the graph rather than instead of it.
"""

from __future__ import annotations

import sqlite3
from contextlib import closing
from datetime import UTC, datetime
from pathlib import Path
from typing import Literal

from db.graph_client import EpisodeKind

DEFAULT_DB_PATH = Path(__file__).parent / "audit.sqlite3"

GraphWriteStatus = Literal["pending", "committed", "failed"]

_SCHEMA = """
CREATE TABLE IF NOT EXISTS episode_audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    episode_type TEXT NOT NULL
        CHECK (episode_type IN ('WORK_BLOCK_COMPLETED', 'WORK_BLOCK_BLOCKED')),
    block_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    ingested_at TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    graph_write_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (graph_write_status IN ('pending', 'committed', 'failed'))
);
CREATE INDEX IF NOT EXISTS idx_episode_audit_block_id ON episode_audit(block_id);
CREATE INDEX IF NOT EXISTS idx_episode_audit_ingested_at ON episode_audit(ingested_at);
"""


class AuditRecordError(ValueError):
    """A record could not be appended to, or updated in, the audit log."""


def _connect(db_path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(db_path)
    conn.execute("PRAGMA journal_mode=WAL")
    conn.executescript(_SCHEMA)
    return conn


def record_episode(
    block_id: str,
    group_id: str,
    episode_type: EpisodeKind,
    payload_json: str,
    db_path: Path = DEFAULT_DB_PATH,
) -> int:
    """Append one episode to the audit log, typically just before sending it to Graphiti.

    Returns the new row id. Raises AuditRecordError on missing input or a
    database-level failure, rather than letting a raw sqlite3.Error escape.
    """
    if not block_id:
        raise AuditRecordError("block_id must not be empty")
    if not group_id:
        raise AuditRecordError("group_id must not be empty")
    try:
        with closing(_connect(db_path)) as conn, conn:
            cursor = conn.execute(
                "INSERT INTO episode_audit "
                "(episode_type, block_id, group_id, ingested_at, payload_json) "
                "VALUES (?, ?, ?, ?, ?)",
                (episode_type, block_id, group_id, datetime.now(UTC).isoformat(), payload_json),
            )
            return cursor.lastrowid
    except sqlite3.Error as exc:
        raise AuditRecordError(f"failed to append audit record: {exc}") from exc


def mark_graph_write_status(
    row_id: int,
    status: GraphWriteStatus,
    db_path: Path = DEFAULT_DB_PATH,
) -> None:
    """Record whether the Graphiti write that followed this audit row succeeded."""
    try:
        with closing(_connect(db_path)) as conn, conn:
            cursor = conn.execute(
                "UPDATE episode_audit SET graph_write_status = ? WHERE id = ?",
                (status, row_id),
            )
            if cursor.rowcount == 0:
                raise AuditRecordError(f"no audit record with id {row_id}")
    except sqlite3.Error as exc:
        raise AuditRecordError(f"failed to update audit record {row_id}: {exc}") from exc


def pending_episodes(db_path: Path = DEFAULT_DB_PATH) -> list[sqlite3.Row]:
    """Rows never confirmed committed to the graph — candidates for replay."""
    try:
        with closing(_connect(db_path)) as conn:
            conn.row_factory = sqlite3.Row
            return conn.execute(
                "SELECT * FROM episode_audit WHERE graph_write_status != 'committed' "
                "ORDER BY ingested_at"
            ).fetchall()
    except sqlite3.Error as exc:
        raise AuditRecordError(f"failed to read pending episodes: {exc}") from exc

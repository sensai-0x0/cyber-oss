"""Restart-safety tests for the SQLite-backed durable checkpointer option.

These confirm that `interrupt()`/resume survives a simulated process
restart (a brand-new SqliteSaver + brand-new compiled graph pointed at
the same db file), not just continued use of the same in-memory Python
objects. SQLite is exercised here as an available opt-in option only;
it is not wired in as `build_graph`'s default (see P2.2's pending human
checkpoint in plans/planv1.0/PHASE2.md).
"""

from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from langgraph.types import Command

from orchestrator.graph import build_graph, build_sqlite_checkpointer
from orchestrator.state import DeltaEvent
from orchestrator.tests.evidence import staging_attributes


def event(**attributes: Any) -> dict:
    return DeltaEvent(
        schema_version="v1",
        event_id="event-1",
        kind="novel",
        source="tlsx",
        target="198.51.100.0/24",
        observed_at=datetime.now(UTC),
        fingerprint="b" * 64,
        attributes=attributes,
    ).model_dump()


def start_broad_action_run(db_path: Path) -> dict:
    """Build a graph against the db file and drive it to the broad-action interrupt."""
    compiled = build_graph(checkpointer=build_sqlite_checkpointer(str(db_path)))
    config = {"configurable": {"thread_id": "operator-restart-test"}}
    value = event(**staging_attributes())
    result = compiled.invoke({"event": value}, config=config)
    assert result["__interrupt__"][0].value["type"] == "approval_required"
    assert result["policy"]["action"] == "mitigate"
    assert result["policy"]["approved"] is False
    return config


def resume_after_restart(db_path: Path, config: dict, resume_payload: dict) -> dict:
    """Simulate a process restart: brand-new SqliteSaver + brand-new compiled graph."""
    restarted_checkpointer = build_sqlite_checkpointer(str(db_path))
    restarted_graph = build_graph(checkpointer=restarted_checkpointer)
    return restarted_graph.invoke(Command(resume=resume_payload), config=config)


def test_restart_then_approve_matches_no_restart_outcome(tmp_path: Path):
    db_path = tmp_path / "checkpoints.sqlite"
    config = start_broad_action_run(db_path)

    resumed = resume_after_restart(db_path, config, {"approved": True, "ttl_seconds": 300})

    assert resumed["policy"]["approved"] is True
    assert resumed["policy"]["ttl_seconds"] == 300


def test_restart_then_reject_matches_no_restart_outcome(tmp_path: Path):
    db_path = tmp_path / "checkpoints.sqlite"
    config = start_broad_action_run(db_path)

    resumed = resume_after_restart(db_path, config, {"approved": False})

    assert resumed["policy"]["action"] == "observe"
    assert resumed["policy"]["approved"] is False

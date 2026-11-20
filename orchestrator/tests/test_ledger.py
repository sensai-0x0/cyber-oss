"""Phase 2 (plans/ml-add): a tamper-evident, write-only ledger that never alters a decision."""

import ast
import json
import sqlite3
import subprocess
import sys
from datetime import UTC, datetime
from pathlib import Path

import pytest
from langgraph.checkpoint.memory import MemorySaver
from langgraph.types import Command

from orchestrator.graph import build_graph, build_sqlite_checkpointer
from orchestrator.ledger import GENESIS, Ledger, canonical_json, record_hash, verify_chain
from orchestrator.state import DeltaEvent
from orchestrator.tests.evidence import staging_attributes
from orchestrator.tests.test_adversarial_boundaries import ADVERSARIAL_FIXTURES

ROOT = Path(__file__).parents[2]
GOLDEN = Path(__file__).parent / "fixtures" / "ledger" / "golden_chain.json"
FIXED = datetime(2026, 1, 2, 3, 4, 5, tzinfo=UTC)


def fixed_ledger(path: Path) -> Ledger:
    return Ledger(path, clock=lambda: FIXED)


def run_event(target="198.51.100.7", **attributes) -> dict:
    return DeltaEvent(
        schema_version="v1",
        event_id="ledger-1",
        kind="novel",
        source="tlsx",
        target=target,
        observed_at=FIXED,
        fingerprint="e" * 64,
        attributes=attributes,
    ).model_dump()


def fill(ledger: Ledger, count: int) -> None:
    compiled = build_graph(checkpointer=MemorySaver(), ledger=ledger)
    for index in range(count):
        value = run_event(**staging_attributes(index % 3))
        value["event_id"] = f"ledger-{index}"
        compiled.invoke({"event": value}, config={"configurable": {"thread_id": f"t{index}"}})
        # Reaching the record node needs a finished run, so answer any pending interrupt.
        compiled.invoke(
            Command(resume={"approved": False}),
            config={"configurable": {"thread_id": f"t{index}"}},
        )


def test_canonical_json_is_sorted_compact_utf8():
    assert canonical_json({"b": 1, "a": ["é", None]}) == '{"a":["é",null],"b":1}'.encode()


def test_golden_chain_pins_the_record_format(tmp_path):
    ledger = fixed_ledger(tmp_path / "l.sqlite")
    first = ledger.append({"event": {"id": "a"}, "scorer_id": "s"})
    second = ledger.append({"event": {"id": "b"}, "scorer_id": "s"})
    assert first["prev_hash"] == GENESIS and second["prev_hash"] == first["record_hash"]
    assert record_hash(first) == first["record_hash"]
    produced = {"first": first, "second": second}
    assert produced == json.loads(GOLDEN.read_text())


def test_intact_chain_verifies(tmp_path):
    ledger = fixed_ledger(tmp_path / "l.sqlite")
    fill(ledger, 6)
    assert len(ledger.records()) == 6
    assert verify_chain(ledger) is None


def test_flipping_one_byte_is_detected_at_that_record(tmp_path):
    path = tmp_path / "l.sqlite"
    ledger = fixed_ledger(path)
    fill(ledger, 5)
    with sqlite3.connect(path) as connection:
        body = connection.execute("SELECT body FROM records WHERE seq = 2").fetchone()[0]
        flipped = body.replace("ledger-2", "ledger-X", 1)
        assert flipped != body
        connection.execute("UPDATE records SET body = ? WHERE seq = 2", (flipped,))
    assert verify_chain(ledger) == 2


def test_rewriting_a_record_and_its_hash_breaks_the_next_link(tmp_path):
    path = tmp_path / "l.sqlite"
    ledger = fixed_ledger(path)
    fill(ledger, 4)
    with sqlite3.connect(path) as connection:
        row = connection.execute("SELECT body FROM records WHERE seq = 1").fetchone()
        body = json.loads(row[0])
        body["scorer_id"] = "forged"
        body["record_hash"] = record_hash(body)
        connection.execute(
            "UPDATE records SET body = ?, record_hash = ? WHERE seq = 1",
            (canonical_json(body).decode(), body["record_hash"]),
        )
    assert verify_chain(ledger) == 2


def test_deleting_a_record_is_detected(tmp_path):
    path = tmp_path / "l.sqlite"
    ledger = fixed_ledger(path)
    fill(ledger, 5)
    with sqlite3.connect(path) as connection:
        connection.execute("DELETE FROM records WHERE seq = 3")
    assert verify_chain(ledger) == 3


def test_a_corrupt_body_is_detected(tmp_path):
    path = tmp_path / "l.sqlite"
    ledger = fixed_ledger(path)
    fill(ledger, 3)
    with sqlite3.connect(path) as connection:
        connection.execute("UPDATE records SET body = 'not json' WHERE seq = 1")
    assert verify_chain(ledger) == 1


class BrokenLedger:
    def __init__(self):
        self.failed_event_ids: list[str] = []

    def append(self, fields):
        raise OSError("disk full")


def outcome(compiled, value):
    config = {"configurable": {"thread_id": "same"}}
    result = compiled.invoke({"event": value}, config=config)
    interrupt = result.pop("__interrupt__", None)
    return result, interrupt[0].value if interrupt else None


def test_a_failing_ledger_write_leaves_the_result_identical():
    value = run_event(**staging_attributes())
    plain = outcome(build_graph(checkpointer=MemorySaver()), value)
    broken = BrokenLedger()
    with_failure = outcome(build_graph(checkpointer=MemorySaver(), ledger=broken), value)
    assert with_failure == plain


def test_a_failed_write_marks_the_run_unrecorded():
    broken = BrokenLedger()
    compiled = build_graph(checkpointer=MemorySaver(), ledger=broken)
    config = {"configurable": {"thread_id": "x"}}
    compiled.invoke({"event": run_event()}, config=config)  # low confidence: finishes at once
    assert broken.failed_event_ids == ["ledger-1"]


@pytest.mark.parametrize(
    "fixture", ADVERSARIAL_FIXTURES, ids=[f["name"] for f in ADVERSARIAL_FIXTURES]
)
def test_graph_output_is_the_same_with_the_ledger_on_and_off(fixture, tmp_path):
    value = DeltaEvent(
        schema_version="v1",
        event_id=f"adv-{fixture['name']}",
        kind="novel",
        source="tlsx",
        target=fixture["target"],
        observed_at=FIXED,
        fingerprint="c" * 64,
        attributes=fixture["attributes"],
    ).model_dump()
    without = outcome(build_graph(checkpointer=MemorySaver()), value)
    ledger = fixed_ledger(tmp_path / "l.sqlite")
    with_ledger = outcome(build_graph(checkpointer=MemorySaver(), ledger=ledger), value)
    assert with_ledger == without


def test_resume_after_a_restart_appends_exactly_one_record(tmp_path):
    checkpoints = tmp_path / "checkpoints.sqlite"
    ledger = fixed_ledger(tmp_path / "ledger.sqlite")
    config = {"configurable": {"thread_id": "restart"}}
    first = build_graph(checkpointer=build_sqlite_checkpointer(str(checkpoints)), ledger=ledger)
    value = run_event("10.0.0.0/16", **staging_attributes())
    result = first.invoke({"event": value}, config=config)
    assert "__interrupt__" in result
    assert ledger.records() == []  # nothing is recorded while the decision is pending

    restarted = build_graph(
        checkpointer=build_sqlite_checkpointer(str(checkpoints)),
        ledger=fixed_ledger(tmp_path / "ledger.sqlite"),
    )
    restarted.invoke(Command(resume={"approved": True, "ttl_seconds": 300}), config=config)
    rows = ledger.records()
    assert len(rows) == 1
    record = json.loads(rows[0][2])
    assert record["policy"]["approved"] is True and record["policy"]["ttl_seconds"] == 300
    assert verify_chain(ledger) is None


def test_record_carries_evidence_classes_and_an_empty_challenger_map(tmp_path):
    ledger = fixed_ledger(tmp_path / "l.sqlite")
    compiled = build_graph(checkpointer=MemorySaver(), ledger=ledger)
    value = run_event(known_bad=True, **staging_attributes(1))
    compiled.invoke({"event": value}, config={"configurable": {"thread_id": "e"}})
    record = json.loads(ledger.records()[0][2])
    classes = {item["key"]: item["class"] for item in record["evidence"]}
    assert classes["known_bad"] == "ASSERTED"
    assert classes["jarm"] == "OBSERVED"
    assert classes["badlist:jarm"] == "DERIVED"
    assert record["challengers"] == {}
    assert len(record["policy_sha256"]) == 64 and record["scorer_id"]


def test_no_graph_node_reads_the_ledger():
    """Only graph.py wires the ledger, and the node it gets can only append."""
    importers = []
    for path in sorted((ROOT / "orchestrator").glob("*.py")):
        tree = ast.parse(path.read_text())
        for node in ast.walk(tree):
            names = []
            if isinstance(node, ast.ImportFrom) and node.module:
                names = [node.module, *[a.name for a in node.names]]
            elif isinstance(node, ast.Import):
                names = [a.name for a in node.names]
            if any(name.endswith("ledger") or name == "orchestrator.ledger" for name in names):
                importers.append(path.name)
    assert sorted(set(importers)) == ["graph.py"]
    source = (ROOT / "orchestrator" / "ledger.py").read_text()
    node_source = source[source.index("def make_record_node") : source.index("def main")]
    assert ".records(" not in node_source and "verify_chain" not in node_source


def test_cli_verifies_a_fixture_ledger_and_flags_a_broken_one(tmp_path):
    path = tmp_path / "l.sqlite"
    fill(fixed_ledger(path), 3)
    run = [sys.executable, "-m", "orchestrator.ledger", "verify", str(path)]
    ok = subprocess.run(run, cwd=ROOT, capture_output=True, text=True)
    assert ok.returncode == 0 and ok.stdout.strip() == "ok"
    with sqlite3.connect(path) as connection:
        connection.execute("DELETE FROM records WHERE seq = 1")
    bad = subprocess.run(run, cwd=ROOT, capture_output=True, text=True)
    assert bad.returncode == 1 and "first bad record: 1" in bad.stdout
    missing = subprocess.run(run[:-1] + [str(tmp_path / "none")], cwd=ROOT, capture_output=True)
    assert missing.returncode == 2

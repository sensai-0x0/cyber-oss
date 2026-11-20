"""Phase 5 (ADR 0005): the approval interrupt cites a ledger record made before review."""

import json
import subprocess
import sys
from pathlib import Path

from langgraph.checkpoint.memory import MemorySaver
from langgraph.types import Command

from orchestrator.graph import build_graph
from orchestrator.ledger import Ledger, canonical_json, verify_chain
from orchestrator.policy import load_policy
from orchestrator.tests.evidence import staging_attributes
from orchestrator.tests.test_ledger import fixed_ledger, run_event

ROOT = Path(__file__).parents[2]


def staged(target="198.51.100.7"):
    return {"event": run_event(target=target, **staging_attributes(2))}


def start(compiled, thread="p", target="198.51.100.7"):
    result = compiled.invoke(staged(target), config={"configurable": {"thread_id": thread}})
    pending = result.get("__interrupt__")
    return result, (pending[0].value if pending else None)


def test_policy_declares_the_append_attempts():
    assert load_policy()["ledger_append_attempts"] == 3


def test_interrupt_cites_a_record_that_exists_before_the_decision(tmp_path):
    ledger = fixed_ledger(tmp_path / "ledger.db")
    compiled = build_graph(checkpointer=MemorySaver(), ledger=ledger)
    _, payload = start(compiled)
    assert payload is not None
    rows = ledger.records()
    assert len(rows) == 1  # only the proposal; the decision record comes after review
    seq, stored_hash, body = rows[0]
    assert (payload["ledger_seq"], payload["evidence_digest"]) == (seq, stored_hash)
    record = json.loads(body)
    assert record["stage"] == "propose"
    assert record["policy"]["approved"] is False


def test_decision_record_cites_the_proposal(tmp_path):
    ledger = fixed_ledger(tmp_path / "ledger.db")
    compiled = build_graph(checkpointer=MemorySaver(), ledger=ledger)
    _, payload = start(compiled)
    compiled.invoke(
        Command(resume={"approved": True, "ttl_seconds": 600}),
        config={"configurable": {"thread_id": "p"}},
    )
    rows = ledger.records()
    assert len(rows) == 2 and verify_chain(ledger) is None
    decision = json.loads(rows[1][2])
    assert decision["proposal_seq"] == payload["ledger_seq"]
    assert decision["policy"]["approved"] is True


def test_without_a_ledger_the_payload_is_unchanged():
    compiled = build_graph(checkpointer=MemorySaver())
    _, payload = start(compiled)
    assert payload is not None and "ledger_seq" not in payload and "evidence_digest" not in payload


class Flaky:
    """A writer that fails a set number of times before succeeding."""

    def __init__(self, inner: Ledger, failures: int):
        self.inner, self.failures, self.calls = inner, failures, 0

    def append(self, fields):
        self.calls += 1
        if self.calls <= self.failures:
            raise OSError("disk full")
        return self.inner.append(fields)


def test_two_failures_then_success_still_proposes(tmp_path):
    flaky = Flaky(fixed_ledger(tmp_path / "ledger.db"), failures=2)
    _, payload = start(build_graph(checkpointer=MemorySaver(), ledger=flaky))
    assert payload is not None and flaky.calls == 3


def test_three_failures_degrade_to_observe_with_nothing_to_sign(tmp_path):
    flaky = Flaky(fixed_ledger(tmp_path / "ledger.db"), failures=99)
    result, payload = start(build_graph(checkpointer=MemorySaver(), ledger=flaky))
    assert payload is None  # no interrupt, so nothing for an operator to sign
    assert result["policy"]["action"] == "observe"
    assert result["policy"]["approved"] is False and "ledger" in result["policy"]["reason"]
    assert flaky.calls >= 3  # three proposal attempts


def test_export_emits_verifiable_ndjson(tmp_path):
    ledger = fixed_ledger(tmp_path / "ledger.db")
    start(build_graph(checkpointer=MemorySaver(), ledger=ledger))
    done = subprocess.run(
        [sys.executable, "-m", "orchestrator.ledger", "export", str(tmp_path / "ledger.db")],
        capture_output=True,
        text=True,
        cwd=ROOT,
        check=True,
    )
    lines = done.stdout.splitlines()
    assert [json.loads(line)["seq"] for line in lines] == [0]
    assert lines[0] == canonical_json(json.loads(lines[0])).decode()


class Recording:
    def __init__(self, fail: bool = False):
        self.fail, self.calls = fail, 0

    def append(self, fields):
        self.calls += 1
        if self.fail:
            raise OSError("disk full")
        return {"seq": 5, "record_hash": "b" * 64}


STALE = {"ledger_seq": 0, "evidence_digest": "a" * 64}


def test_a_run_with_nothing_to_propose_clears_an_earlier_proposal():
    from orchestrator.ledger import make_propose_node

    observe = {"action": "observe", "approved": False, "ttl_seconds": 0}
    propose = make_propose_node(Recording())
    assert propose({"policy": observe, "proposal": STALE}) == {"proposal": None}


def test_a_degraded_proposal_clears_an_earlier_proposal(tmp_path):
    from orchestrator.ledger import make_propose_node

    state = {
        "policy": {"action": "mitigate", "approved": False, "ttl_seconds": 600},
        "proposal": STALE,
    }
    result = make_propose_node(Recording(fail=True))(state)
    assert result["proposal"] is None and result["policy"]["action"] == "observe"

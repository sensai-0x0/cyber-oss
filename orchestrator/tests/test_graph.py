from datetime import UTC, datetime
from typing import Any

from langgraph.checkpoint.memory import MemorySaver
from langgraph.types import Command

from orchestrator.graph import build_graph
from orchestrator.state import DeltaEvent


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


def invoke(compiled, value):
    config = {"configurable": {"thread_id": "operator-test"}}
    return compiled.invoke(value, config=config), config


def test_low_confidence_produces_no_mitigation():
    compiled = build_graph(checkpointer=MemorySaver())
    result, _ = invoke(compiled, {"event": event()})
    assert result["intel"]["confidence"] < 0.8
    assert result["policy"]["action"] == "observe"


def test_high_confidence_narrow_action_does_not_interrupt():
    compiled = build_graph(checkpointer=MemorySaver())
    value = event(recent_ct=True, known_bad=True, shared_fingerprints=3, prefix_length=32)
    result, _ = invoke(compiled, {"event": value})
    assert result["policy"]["action"] == "mitigate"
    assert result["policy"]["approved"] is True
    assert result["policy"]["ttl_seconds"] > 0
    assert "__interrupt__" not in result


def test_broad_action_interrupts_and_resumes_with_operator_decision():
    compiled = build_graph(checkpointer=MemorySaver())
    value = event(recent_ct=True, known_bad=True, shared_fingerprints=3, prefix_length=16)
    result, config = invoke(compiled, {"event": value})
    assert result["__interrupt__"][0].value["type"] == "approval_required"
    assert result["policy"]["action"] == "mitigate"
    assert result["policy"]["approved"] is False

    resumed = compiled.invoke(Command(resume={"approved": True, "ttl_seconds": 300}), config=config)
    assert resumed["policy"]["approved"] is True
    assert resumed["policy"]["ttl_seconds"] == 300


def test_operator_can_reject_broad_action():
    compiled = build_graph(checkpointer=MemorySaver())
    value = event(recent_ct=True, known_bad=True, shared_fingerprints=3, prefix_length=16)
    _, config = invoke(compiled, {"event": value})
    resumed = compiled.invoke(Command(resume={"approved": False}), config=config)
    assert resumed["policy"]["action"] == "observe"
    assert resumed["policy"]["approved"] is False


def test_refinement_loop_is_bounded():
    compiled = build_graph(checkpointer=MemorySaver())
    result, _ = invoke(compiled, {"event": event(recent_ct=True)})
    assert result["cycles"] <= 2

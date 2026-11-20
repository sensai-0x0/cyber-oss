from datetime import UTC, datetime
from typing import Any

from langgraph.checkpoint.memory import MemorySaver
from langgraph.types import Command

from orchestrator.graph import build_graph
from orchestrator.state import DeltaEvent
from orchestrator.tests.evidence import staging_attributes


def event(target: str = "198.51.100.0/24", **attributes: Any) -> dict:
    return DeltaEvent(
        schema_version="v1",
        event_id="event-1",
        kind="novel",
        source="tlsx",
        target=target,
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


def test_narrow_action_still_interrupts_while_auto_approve_is_off():
    compiled = build_graph(checkpointer=MemorySaver())
    value = event("198.51.100.7", **staging_attributes())
    result, _ = invoke(compiled, {"event": value})
    assert result["policy"]["action"] == "mitigate"
    assert result["policy"]["broad"] is False
    assert result["policy"]["approved"] is False
    assert result["__interrupt__"][0].value["type"] == "approval_required"


def test_narrow_action_skips_the_operator_only_when_policy_enables_it(monkeypatch):
    from orchestrator import policy as policy_module

    enabled = {**policy_module.load_policy(), "auto_approve_narrow": True}
    monkeypatch.setattr(policy_module, "load_policy", lambda: enabled)
    compiled = build_graph(checkpointer=MemorySaver())
    value = event("198.51.100.7", **staging_attributes())
    result, _ = invoke(compiled, {"event": value})
    assert result["policy"]["approved"] is True
    assert result["policy"]["ttl_seconds"] > 0
    assert "__interrupt__" not in result


def test_broad_action_interrupts_and_resumes_with_operator_decision():
    compiled = build_graph(checkpointer=MemorySaver())
    value = event("10.0.0.0/16", **staging_attributes())
    result, config = invoke(compiled, {"event": value})
    assert result["__interrupt__"][0].value["type"] == "approval_required"
    assert result["policy"]["action"] == "mitigate"
    assert result["policy"]["approved"] is False

    resumed = compiled.invoke(Command(resume={"approved": True, "ttl_seconds": 300}), config=config)
    assert resumed["policy"]["approved"] is True
    assert resumed["policy"]["ttl_seconds"] == 300


def test_operator_can_reject_broad_action():
    compiled = build_graph(checkpointer=MemorySaver())
    value = event("10.0.0.0/16", **staging_attributes())
    _, config = invoke(compiled, {"event": value})
    resumed = compiled.invoke(Command(resume={"approved": False}), config=config)
    assert resumed["policy"]["action"] == "observe"
    assert resumed["policy"]["approved"] is False


def test_refinement_loop_is_bounded():
    compiled = build_graph(checkpointer=MemorySaver())
    result, _ = invoke(compiled, {"event": event(**staging_attributes(1))})
    assert result["cycles"] <= 2

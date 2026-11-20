"""Auditable staging-confidence scoring."""

from typing import Any

from orchestrator.state import DeltaEvent, GraphState


def score_threat_intel(state: GraphState) -> dict[str, Any]:
    event = DeltaEvent.model_validate(state["event"])
    attributes = event.attributes
    confidence = 0.1
    rationale = []
    if attributes.get("recent_ct"):
        confidence += 0.2
        rationale.append("recent certificate-transparency activity")
    if attributes.get("known_bad"):
        confidence += 0.45
        rationale.append("matched high-confidence threat intelligence")
    shared = min(max(int(attributes.get("shared_fingerprints", 0)), 0), 3)
    if shared:
        confidence += shared * 0.1
        rationale.append(f"{shared} related fingerprint observations")
    return {"intel": {"confidence": min(confidence, 0.99), "rationale": rationale}}


def route_after_intel(state: GraphState) -> str:
    """Permit one refinement cycle, then force a terminal policy decision."""
    confidence = state["intel"]["confidence"]
    if 0.3 <= confidence < 0.8 and state.get("cycles", 0) < 2:
        return "cluster"
    return "mitigate"

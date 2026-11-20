"""Auditable staging-confidence scoring."""

from typing import Any

from orchestrator.policy import load_policy
from orchestrator.state import DeltaEvent, GraphState

# Below this, further clustering cannot change the outcome, so refinement is pointless.
# Unlike the bounds above it, this floor has no policy.yaml key yet.
AMBIGUOUS_FLOOR = 0.3


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
    """Refine while the score is ambiguous, bounded by policy.yaml, then decide."""
    config = load_policy()
    confidence = state["intel"]["confidence"]
    ambiguous = AMBIGUOUS_FLOOR <= confidence < config["mitigation_confidence"]
    if ambiguous and state.get("cycles", 0) < config["max_refinement_cycles"]:
        return "cluster"
    return "mitigate"

"""Auditable staging-confidence scoring."""

from typing import Any

from orchestrator.policy import load_policy
from orchestrator.provenance import derive_badlist_hits, dropped_asserted
from orchestrator.state import DeltaEvent, GraphState

# Below this, further clustering cannot change the outcome, so refinement is pointless.
# Unlike the bounds above it, this floor has no policy.yaml key yet.
AMBIGUOUS_FLOOR = 0.3


# Hand-set weights. Each independent badlist hit is DERIVED evidence; two reach the
# default mitigation threshold and one does not. The weights are not learned.
BASE_CONFIDENCE = 0.1
BADLIST_HIT_WEIGHT = 0.4
MAX_CONFIDENCE = 0.99


def score_threat_intel(state: GraphState) -> dict[str, Any]:
    """Score from OBSERVED-derived evidence only. ASSERTED attributes add nothing."""
    config = load_policy()
    event = DeltaEvent.model_validate(state["event"])
    confidence = BASE_CONFIDENCE
    rationale = []
    for field in derive_badlist_hits(event, config):
        confidence += BADLIST_HIT_WEIGHT
        rationale.append(f"observed {field} is on the pinned lab badlist")
    dropped = dropped_asserted(event)
    for key in dropped:
        rationale.append(f"ignored producer-asserted attribute {key}")
    return {
        "intel": {
            "confidence": min(confidence, MAX_CONFIDENCE),
            "rationale": rationale,
            "dropped_asserted": dropped,
        }
    }


def route_after_intel(state: GraphState) -> str:
    """Refine while the score is ambiguous, bounded by policy.yaml, then decide."""
    config = load_policy()
    confidence = state["intel"]["confidence"]
    ambiguous = AMBIGUOUS_FLOOR <= confidence < config["mitigation_confidence"]
    if ambiguous and state.get("cycles", 0) < config["max_refinement_cycles"]:
        return "cluster"
    return "mitigate"

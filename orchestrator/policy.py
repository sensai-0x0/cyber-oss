"""Policy synthesis and the human approval interrupt."""

from functools import lru_cache
from pathlib import Path
from typing import Any

import yaml
from langgraph.types import interrupt

from orchestrator.state import DeltaEvent, GraphState, Policy

POLICY_FILE = Path(__file__).with_name("policy.yaml")


@lru_cache(maxsize=1)
def load_policy() -> dict[str, Any]:
    config = yaml.safe_load(POLICY_FILE.read_text())
    if config.get("version") != "v1":
        raise ValueError("unsupported policy version")
    return config


def synthesize_policy(state: GraphState) -> dict[str, Any]:
    config = load_policy()
    event = DeltaEvent.model_validate(state["event"])
    if state["intel"]["confidence"] < config["mitigation_confidence"]:
        return {
            "policy": {
                "action": "observe",
                "approved": False,
                "ttl_seconds": 0,
                "reason": "insufficient staging confidence",
            }
        }

    prefix_length = int(event.attributes.get("prefix_length", 32))
    broad = prefix_length < config["broad_prefix_threshold"]
    policy: Policy = {
        "action": "mitigate",
        "action_id": f"mit-{event.event_id}",
        "target": event.target,
        "kind": "nftables" if event.target[0].isdigit() else "rpz",
        "broad": broad,
        "approved": not broad,
        "ttl_seconds": config["default_ttl_seconds"],
    }
    return {"policy": policy}


def apply_decision(policy: Policy, decision: dict[str, Any], config: dict[str, Any]) -> Policy:
    """Return a new policy reflecting an operator decision, leaving the input untouched."""
    if not bool(decision.get("approved", False)):
        return {
            **policy,
            "action": "observe",
            "approved": False,
            "ttl_seconds": 0,
            "reason": "operator rejected mitigation",
        }
    requested_ttl = int(decision.get("ttl_seconds", config["default_ttl_seconds"]))
    return {
        **policy,
        "approved": True,
        "ttl_seconds": min(
            max(requested_ttl, config["min_approved_ttl_seconds"]),
            config["max_approved_ttl_seconds"],
        ),
    }


def approve_policy(state: GraphState) -> dict[str, Any]:
    """Pause for a named decision on a broad action, then record the outcome."""
    policy = state["policy"]
    if policy["action"] != "mitigate" or not policy["broad"]:
        return {}
    config = load_policy()
    decision = interrupt(
        {
            "type": "approval_required",
            "action_id": policy["action_id"],
            "target": policy["target"],
            "confidence": state["intel"]["confidence"],
            "max_ttl_seconds": config["max_approved_ttl_seconds"],
        }
    )
    return {"policy": apply_decision(policy, decision, config)}

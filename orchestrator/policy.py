"""Policy synthesis and the human approval interrupt."""

import ipaddress
import re
from functools import lru_cache
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit

import yaml
from langgraph.types import interrupt

from orchestrator.state import DeltaEvent, GraphState, Policy

POLICY_FILE = Path(__file__).with_name("policy.yaml")


@lru_cache(maxsize=1)
def load_policy() -> dict[str, Any]:
    config = yaml.safe_load(POLICY_FILE.read_text())
    if config.get("version") != "v1":
        raise ValueError("unsupported policy version")
    from orchestrator.provenance import verify_badlist

    verify_badlist(config)  # refuse to load if the pinned badlist was edited or removed
    return config


_HOSTNAME = re.compile(
    r"^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z]([a-z0-9-]{0,61}[a-z0-9])?\.?$"
)


def classify_target(target: str, config: dict[str, Any] | None = None) -> dict[str, Any] | None:
    """Derive kind, prefix length, breadth and the bare target from the target alone.

    Nothing in the event's attributes and no first-character guess is consulted.
    Returns None when the target is not an address, a range or a hostname. The rules
    mirror `enforcerd.Breadth.Derive`, which re-derives breadth from `target` and is the
    authority (I7): an IPv4-mapped range is judged at its IPv4 width and one wider than
    the mapped block is refused, as is a zoned address.
    """
    config = config or load_policy()
    text = target.strip()
    if "://" in text:
        text = urlsplit(text).hostname or ""
    elif "/" not in text and text.count(":") == 1:
        text = text.rsplit(":", 1)[0]  # host:port
    elif text.startswith("[") and "]" in text:
        text = text[1 : text.index("]")]  # [v6]:port
    if not text:
        return None
    try:
        network = ipaddress.ip_network(text, strict=False)
    except ValueError:
        network = None
    if network is not None:
        # Judge the address as written, before masking, so ::ffff:0:0/80 is seen as mapped.
        written = ipaddress.ip_address(text.split("/", 1)[0])
        if isinstance(written, ipaddress.IPv6Address) and written.scope_id:
            return None
        bits, width, limit = (
            network.prefixlen,
            network.max_prefixlen,
            config["broad_prefix_threshold"]
            if network.version == 4
            else config["broad_prefix_threshold_v6"],
        )
        if isinstance(written, ipaddress.IPv6Address) and written.ipv4_mapped is not None:
            if bits < 96:
                return None  # straddles the IPv4-mapped block
            bits, width, limit = bits - 96, 32, config["broad_prefix_threshold"]
        # A range at or below the threshold is broad, so a /24 needs an operator.
        broad = bits < width and bits <= limit
        bare = str(network) if "/" in text else str(network.network_address)
        return {"kind": "nftables", "prefix_len": network.prefixlen, "broad": broad, "target": bare}
    name = text.lower()
    if _HOSTNAME.match(name):
        return {"kind": "rpz", "prefix_len": None, "broad": False, "target": name}
    return None


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

    derived = classify_target(event.target, config)
    if derived is None:
        return {
            "policy": {
                "action": "observe",
                "approved": False,
                "ttl_seconds": 0,
                "reason": "target is not a parseable address, range or hostname",
            }
        }
    broad = derived["broad"]
    policy: Policy = {
        "action": "mitigate",
        "action_id": f"mit-{event.event_id}",
        # The bare form that was classified, so enforcerd judges the same string (SHR-001).
        "target": derived["target"],
        "kind": derived["kind"],
        "broad": broad,
        # Only a narrow action can skip the operator, and only when policy allows it.
        "approved": (not broad) and bool(config.get("auto_approve_narrow", False)),
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
    """Pause for a named decision on any unapproved action, then record the outcome."""
    policy = state["policy"]
    if policy["action"] != "mitigate" or policy["approved"]:
        return {}
    config = load_policy()
    proposal = state.get("proposal") or {}
    decision = interrupt(
        {
            "type": "approval_required",
            "action_id": policy["action_id"],
            "target": policy["target"],
            "broad": policy["broad"],
            "confidence": state["intel"]["confidence"],
            "max_ttl_seconds": config["max_approved_ttl_seconds"],
            # The ledger record the operator reviews (ADR 0005); absent when no ledger is wired.
            **{key: proposal[key] for key in ("ledger_seq", "evidence_digest") if key in proposal},
        }
    )
    return {"policy": apply_decision(policy, decision, config)}

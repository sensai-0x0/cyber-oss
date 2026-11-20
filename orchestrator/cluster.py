"""Cluster related infrastructure without invoking external services."""

import hashlib
from typing import Any

from orchestrator.state import DeltaEvent, GraphState


def cluster_event(state: GraphState) -> dict[str, Any]:
    """Group stable TLS, favicon, and certificate indicators."""
    event = DeltaEvent.model_validate(state["event"])
    indicators = sorted(
        str(event.attributes[key])
        for key in ("jarm", "ja3", "favicon_mmh3", "common_name")
        if key in event.attributes
    )
    material = "\n".join([event.target, *indicators]).encode()
    return {
        "cluster": {
            "cluster_id": hashlib.sha256(material).hexdigest()[:16],
            "indicators": indicators,
            "event_ids": [event.event_id],
        },
        "cycles": state.get("cycles", 0) + 1,
    }

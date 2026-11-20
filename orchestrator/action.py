"""Canonical bytes of a v2 mitigation action, mirroring `enforcerd.canonicalAction` (ADR 0005).

Python does not sign. This exists so the golden vectors pin one byte form on both sides.
"""

import json
from datetime import UTC, datetime
from typing import Any

FIELDS = (
    "schema_version", "action_id", "kind", "target", "broad", "approved", "approved_by",
    "ttl_seconds", "created_at", "expires_at", "evidence_digest", "ledger_seq",
)  # fmt: skip


def go_time(value: str) -> str:
    """UTC RFC 3339 with trailing zeros of the fraction trimmed, as Go's time.Time marshals."""
    stamp = datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(UTC)
    base = stamp.strftime("%Y-%m-%dT%H:%M:%S")
    fraction = f"{stamp.microsecond:06d}".rstrip("0")
    return f"{base}.{fraction}Z" if fraction else f"{base}Z"


def canonical_action_v2(action: dict[str, Any]) -> bytes:
    if action.get("schema_version") != "v2":
        raise ValueError("not a v2 action")
    wire: dict[str, Any] = {}
    for field in FIELDS:
        if field == "approved_by" and not action.get(field):
            continue  # omitted when empty, like the v1 canonical form
        value = action[field]
        wire[field] = go_time(value) if field in ("created_at", "expires_at") else value
    return json.dumps(wire, separators=(",", ":"), ensure_ascii=False).encode()

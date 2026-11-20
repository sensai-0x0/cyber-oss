"""Shared helpers: synthetic evidence that scores through the provenance rules."""

import json
from pathlib import Path
from typing import Any

ROOT = Path(__file__).parents[2]
BADLIST = json.loads((ROOT / "lab" / "fixtures" / "badlist.v1.json").read_text())
BAD_JARM = BADLIST["jarm"][0]
BAD_JA3 = BADLIST["ja3"][0]


def staging_attributes(hits: int = 2, **extra: Any) -> dict[str, Any]:
    """OBSERVED tlsx fields that match the pinned badlist `hits` times (0 to 2)."""
    attributes: dict[str, Any] = {}
    if hits >= 1:
        attributes["jarm"] = BAD_JARM
    if hits >= 2:
        attributes["ja3"] = BAD_JA3
    return {**attributes, **extra}

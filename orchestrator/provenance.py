"""Evidence provenance classes (plans/ml-add Phase 1).

OBSERVED  the field name is one a hunterd probe normaliser emits for that source.
DERIVED   computed here from a pinned artefact whose SHA-256 policy.yaml records.
ASSERTED  anything else. It is never used as evidence.

Limit, stated plainly: OBSERVED checks the field *name*, not who wrote it. Anyone
who can write to `hunterd serve`'s stdin can still forge an OBSERVED field. SHR-008
stays open until the event producer is authenticated.
"""

import hashlib
import json
from functools import lru_cache
from pathlib import Path
from typing import Any, Literal

from orchestrator.state import DeltaEvent

Class = Literal["OBSERVED", "DERIVED", "ASSERTED"]

ROOT = Path(__file__).parent.parent

# Mirrors hunterd/probes/testdata/attribute_keys.json. A contract test compares them,
# and a Go test compares the same file with what the normalisers emit.
OBSERVED_KEYS: dict[str, frozenset[str]] = {
    "tlsx": frozenset(
        {"certificate_not_after", "common_name", "host", "ip", "ja3", "jarm", "sans"}
    ),
    "httpx": frozenset({"favicon_mmh3", "ip", "page_type", "status_code", "title", "web_server"}),
    "ct": frozenset(),
}

# Which observed fields are checked against the pinned badlist, per source.
BADLIST_FIELDS: dict[str, tuple[str, ...]] = {
    "tlsx": ("jarm", "ja3"),
    "httpx": ("favicon_mmh3",),
}

# Attributes the scorer used to trust. They arrive from the producer, so they are ASSERTED.
LEGACY_ASSERTED_SCORING_KEYS = ("recent_ct", "known_bad", "shared_fingerprints")


def classify(source: str, key: str) -> Class:
    return "OBSERVED" if key in OBSERVED_KEYS.get(source, frozenset()) else "ASSERTED"


def file_sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_badlist(config: dict[str, Any]) -> Path:
    """Return the badlist path, or raise if it is missing or does not match the pin."""
    path = ROOT / config["badlist_path"]
    if not path.is_file():
        raise ValueError(f"badlist missing: {config['badlist_path']}")
    if file_sha256(path) != config["badlist_sha256"]:
        raise ValueError("badlist does not match policy.yaml badlist_sha256")
    return path


@lru_cache(maxsize=8)
def _read_badlist(path: str, digest: str) -> dict[str, frozenset[str]]:
    data = json.loads(Path(path).read_text())
    return {
        key: frozenset(map(str, values)) for key, values in data.items() if isinstance(values, list)
    }


def load_badlist(config: dict[str, Any]) -> dict[str, frozenset[str]]:
    path = verify_badlist(config)
    return _read_badlist(str(path), config["badlist_sha256"])


def derive_badlist_hits(event: DeltaEvent, config: dict[str, Any]) -> list[str]:
    """Names of observed fields whose value is on the pinned badlist (DERIVED evidence)."""
    badlist = load_badlist(config)
    hits = []
    for field in BADLIST_FIELDS.get(event.source, ()):
        value = event.attributes.get(field)
        if isinstance(value, (str, int)) and not isinstance(value, bool):
            if str(value) in badlist.get(field, frozenset()):
                hits.append(field)
    return hits


def classify_evidence(event: DeltaEvent, config: dict[str, Any]) -> list[dict[str, Any]]:
    """Every attribute and derived fact with its class, in a stable order."""
    evidence = [
        {"key": key, "value": value, "class": classify(event.source, key)}
        for key, value in sorted(event.attributes.items())
    ]
    evidence.extend(
        {"key": f"badlist:{field}", "value": True, "class": "DERIVED"}
        for field in derive_badlist_hits(event, config)
    )
    return evidence


def dropped_asserted(event: DeltaEvent) -> list[str]:
    """Scoring-relevant keys the producer asserted. They add no confidence."""
    return [
        key
        for key in LEGACY_ASSERTED_SCORING_KEYS
        if key in event.attributes and classify(event.source, key) == "ASSERTED"
    ]

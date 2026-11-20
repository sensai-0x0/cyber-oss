from datetime import UTC, datetime

import pytest
from pydantic import ValidationError

from orchestrator.state import DeltaEvent


def valid_event() -> dict:
    return {
        "schema_version": "v1",
        "event_id": "01J3K4ZQ3Y6N8D3E0K7N7C7M4V",
        "kind": "novel",
        "source": "tlsx",
        "target": "192.0.2.10:443",
        "observed_at": datetime.now(UTC),
        "fingerprint": "a" * 64,
        "attributes": {"jarm": "example", "recent_ct": True, "known_bad": False},
    }


def test_delta_event_round_trip_contract():
    event = DeltaEvent.model_validate(valid_event())
    dumped = event.model_dump(mode="json")
    assert dumped["schema_version"] == "v1"
    assert dumped["kind"] == "novel"
    assert dumped["observed_at"].endswith("Z")
    assert DeltaEvent.model_validate_json(event.model_dump_json()) == event


@pytest.mark.parametrize("kind", ["unchanged", "malicious", ""])
def test_delta_event_only_accepts_emitted_delta_kinds(kind):
    payload = valid_event()
    payload["kind"] = kind
    with pytest.raises(ValidationError):
        DeltaEvent.model_validate(payload)


def test_delta_event_requires_lowercase_sha256_fingerprint():
    payload = valid_event()
    payload["fingerprint"] = "A" * 64
    with pytest.raises(ValidationError):
        DeltaEvent.model_validate(payload)

import json
from pathlib import Path

import jsonschema

from orchestrator.state import DeltaEvent

ROOT = Path(__file__).parents[2]


def test_delta_event_matches_versioned_json_schema():
    schema = json.loads((ROOT / "schemas" / "delta_event.v1.json").read_text())
    event = DeltaEvent(
        schema_version="v1",
        event_id="schema-test",
        kind="novel",
        source="tlsx",
        target="192.0.2.1:443",
        observed_at="2026-01-02T03:04:05Z",
        fingerprint="a" * 64,
        attributes={"jarm": "test"},
    )
    jsonschema.validate(event.model_dump(mode="json"), schema)


def test_action_schema_requires_approval_ttl_and_signature():
    schema = json.loads((ROOT / "schemas" / "mitigation_action.v1.json").read_text())
    required = set(schema["required"])
    assert {"approved", "ttl_seconds", "expires_at", "signature"} <= required

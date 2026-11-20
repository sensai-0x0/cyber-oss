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


GOLDEN_V2 = json.loads((ROOT / "schemas" / "mitigation_action.v2.golden.json").read_text())


def v2_schema():
    return json.loads((ROOT / "schemas" / "mitigation_action.v2.json").read_text())


def test_v1_schema_is_frozen_and_v2_adds_the_evidence_fields():
    v1 = json.loads((ROOT / "schemas" / "mitigation_action.v1.json").read_text())
    v2 = v2_schema()
    assert "evidence_digest" not in v1["properties"] and v1["additionalProperties"] is False
    assert v2["properties"]["schema_version"] == {"const": "v2"}
    assert {"evidence_digest", "ledger_seq"} <= set(v2["required"])
    assert set(v2["properties"]) - set(v1["properties"]) == {"evidence_digest", "ledger_seq"}


def test_v2_schema_accepts_golden_actions_and_rejects_v1_shapes():
    for case in GOLDEN_V2["actions"]:
        jsonschema.validate({**case["action"], "signature": "AAAA"}, v2_schema())
    v1_shaped = {**GOLDEN_V2["actions"][0]["action"], "schema_version": "v1", "signature": "AAAA"}
    for bad in (
        v1_shaped,
        {k: v for k, v in GOLDEN_V2["actions"][0]["action"].items() if k != "evidence_digest"},
        {**GOLDEN_V2["actions"][0]["action"], "evidence_digest": "XYZ", "signature": "AAAA"},
        {**GOLDEN_V2["actions"][0]["action"], "ledger_seq": -1, "signature": "AAAA"},
    ):
        try:
            jsonschema.validate(bad, v2_schema())
        except jsonschema.ValidationError:
            continue
        raise AssertionError(f"accepted {bad}")


def test_v2_canonical_bytes_match_the_golden_vectors():
    from orchestrator.action import canonical_action_v2

    for case in GOLDEN_V2["actions"]:
        assert canonical_action_v2(case["action"]).decode() == case["canonical"], case["name"]


def test_canonical_json_matches_the_shared_cases_and_ledger_hashes():
    from orchestrator.ledger import canonical_json, record_hash

    for case in GOLDEN_V2["json_cases"]:
        assert canonical_json(json.loads(case["input"])).decode() == case["canonical"]
    previous = "0" * 64
    for seq, body in enumerate(GOLDEN_V2["ledger"]):
        record = json.loads(body)
        assert canonical_json(record).decode() == body
        assert record["seq"] == seq and record["prev_hash"] == previous
        assert record_hash(record) == record["record_hash"]
        previous = record["record_hash"]

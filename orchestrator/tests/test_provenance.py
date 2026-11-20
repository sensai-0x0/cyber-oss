"""Phase 1 (plans/ml-add): ASSERTED attributes add nothing; pinned artefacts must match."""

import json
import shutil
from datetime import UTC, datetime
from pathlib import Path

import pytest

from orchestrator import policy as policy_module
from orchestrator import provenance
from orchestrator.provenance import (
    OBSERVED_KEYS,
    classify,
    classify_evidence,
    dropped_asserted,
    verify_badlist,
)
from orchestrator.score import score_threat_intel
from orchestrator.state import DeltaEvent
from orchestrator.tests.evidence import BAD_JA3, BAD_JARM, ROOT, staging_attributes

TESTDATA = ROOT / "hunterd" / "probes" / "testdata" / "attribute_keys.json"


def make(source: str = "tlsx", **attributes) -> DeltaEvent:
    return DeltaEvent(
        schema_version="v1",
        event_id="p-1",
        kind="novel",
        source=source,
        target="198.51.100.7",
        observed_at=datetime.now(UTC),
        fingerprint="d" * 64,
        attributes=attributes,
    )


def score(event: DeltaEvent) -> dict:
    return score_threat_intel({"event": event.model_dump()})["intel"]


@pytest.mark.parametrize(
    "attributes",
    [
        {"known_bad": True},
        {"known_bad": "false"},
        {"known_bad": "true"},
        {"recent_ct": True},
        {"shared_fingerprints": 3},
        {"shared_fingerprints": "abc"},
        {"known_bad": True, "recent_ct": True, "shared_fingerprints": 3},
    ],
)
def test_asserted_flags_add_no_confidence_and_are_reported(attributes):
    intel = score(make(**attributes))
    assert intel["confidence"] == pytest.approx(0.1)
    assert set(intel["dropped_asserted"]) == set(attributes)
    assert all("ignored producer-asserted" in line for line in intel["rationale"])


def test_forged_flags_cannot_reach_the_mitigation_threshold():
    flags = {"known_bad": True, "recent_ct": True, "shared_fingerprints": 3, "prefix_length": 32}
    assert score(make(**flags))["confidence"] < policy_module.load_policy()["mitigation_confidence"]


def test_a_badlisted_jarm_adds_derived_weight():
    intel = score(make(jarm=BAD_JARM))
    assert intel["confidence"] == pytest.approx(0.5)
    assert intel["rationale"] == ["observed jarm is on the pinned lab badlist"]
    assert intel["dropped_asserted"] == []


def test_two_independent_badlist_hits_reach_the_default_threshold():
    intel = score(make(**staging_attributes(2)))
    assert intel["confidence"] >= policy_module.load_policy()["mitigation_confidence"]


@pytest.mark.parametrize(
    "value", [["x"], {"a": 1}, True, None, 1.5, BAD_JARM + "0", BAD_JARM.upper()]
)
def test_only_exact_string_matches_count(value):
    assert score(make(jarm=value))["confidence"] == pytest.approx(0.1)


def test_a_badlisted_value_on_the_wrong_source_does_not_count():
    assert score(make("httpx", jarm=BAD_JARM, ja3=BAD_JA3))["confidence"] == pytest.approx(0.1)
    assert score(make("ct", jarm=BAD_JARM))["confidence"] == pytest.approx(0.1)


def test_classification_follows_the_source_allowlist():
    assert classify("tlsx", "jarm") == "OBSERVED"
    assert classify("httpx", "jarm") == "ASSERTED"
    assert classify("tlsx", "known_bad") == "ASSERTED"
    assert classify("ct", "jarm") == "ASSERTED"
    assert classify("nope", "jarm") == "ASSERTED"


def test_evidence_lists_every_attribute_with_its_class_in_a_stable_order():
    event = make(known_bad=True, jarm=BAD_JARM, ja3="0" * 32)
    evidence = classify_evidence(event, policy_module.load_policy())
    assert [(e["key"], e["class"]) for e in evidence] == [
        ("ja3", "OBSERVED"),
        ("jarm", "OBSERVED"),
        ("known_bad", "ASSERTED"),
        ("badlist:jarm", "DERIVED"),
    ]
    assert dropped_asserted(event) == ["known_bad"]


def test_allowlist_matches_the_keys_the_go_normalisers_emit():
    """The Go test `TestAttributeKeysMatchContract` checks the same file against the code."""
    contract = json.loads(TESTDATA.read_text())
    assert {source: set(keys) for source, keys in OBSERVED_KEYS.items() if keys} == {
        source: set(keys) for source, keys in contract.items()
    }


def test_badlist_with_a_wrong_hash_is_refused():
    config = {**policy_module.load_policy(), "badlist_sha256": "0" * 64}
    with pytest.raises(ValueError, match="badlist_sha256"):
        verify_badlist(config)


def test_edited_badlist_makes_load_policy_refuse(tmp_path, monkeypatch):
    fixtures = tmp_path / "lab" / "fixtures"
    fixtures.mkdir(parents=True)
    copy = fixtures / "badlist.v1.json"
    shutil.copy(ROOT / "lab" / "fixtures" / "badlist.v1.json", copy)
    data = json.loads(copy.read_text())
    data["jarm"].append("f" * 62)
    copy.write_text(json.dumps(data))
    monkeypatch.setattr(provenance, "ROOT", tmp_path)
    policy_module.load_policy.cache_clear()
    try:
        with pytest.raises(ValueError, match="badlist"):
            policy_module.load_policy()
    finally:
        monkeypatch.undo()
        policy_module.load_policy.cache_clear()
    assert policy_module.load_policy()["version"] == "v1"


def test_missing_badlist_makes_load_policy_refuse(tmp_path, monkeypatch):
    monkeypatch.setattr(provenance, "ROOT", Path(tmp_path))
    policy_module.load_policy.cache_clear()
    try:
        with pytest.raises(ValueError, match="missing"):
            policy_module.load_policy()
    finally:
        monkeypatch.undo()
        policy_module.load_policy.cache_clear()

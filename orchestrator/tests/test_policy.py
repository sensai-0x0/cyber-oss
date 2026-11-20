"""Policy thresholds come from policy.yaml, and nodes return new state."""

from orchestrator import policy as policy_module
from orchestrator import score as score_module
from orchestrator.policy import apply_decision, load_policy

CONFIG = {
    "mitigation_confidence": 0.8,
    "default_ttl_seconds": 900,
    "min_approved_ttl_seconds": 60,
    "max_approved_ttl_seconds": 86400,
    "max_refinement_cycles": 2,
}


def test_policy_file_declares_a_refinement_bound():
    assert "max_refinement_cycles" in load_policy()


def test_route_reads_the_refinement_bound_from_policy(monkeypatch):
    """A tighter bound in configuration must shorten the loop, not be ignored."""
    monkeypatch.setattr(score_module, "load_policy", lambda: {**CONFIG, "max_refinement_cycles": 1})
    state = {"intel": {"confidence": 0.5, "rationale": []}, "cycles": 1}
    assert score_module.route_after_intel(state) == "mitigate"


def test_route_refines_while_under_the_bound(monkeypatch):
    monkeypatch.setattr(score_module, "load_policy", lambda: {**CONFIG, "max_refinement_cycles": 3})
    state = {"intel": {"confidence": 0.5, "rationale": []}, "cycles": 1}
    assert score_module.route_after_intel(state) == "cluster"


def test_route_uses_the_mitigation_threshold_as_the_ambiguity_ceiling(monkeypatch):
    tighter = {**CONFIG, "mitigation_confidence": 0.4}
    monkeypatch.setattr(score_module, "load_policy", lambda: tighter)
    state = {"intel": {"confidence": 0.5, "rationale": []}, "cycles": 0}
    assert score_module.route_after_intel(state) == "mitigate"


def test_apply_decision_does_not_mutate_the_input_policy():
    original = {
        "action": "mitigate",
        "action_id": "mit-1",
        "target": "198.51.100.0/24",
        "kind": "nftables",
        "broad": True,
        "approved": False,
        "ttl_seconds": 0,
    }
    snapshot = dict(original)
    updated = apply_decision(original, {"approved": True, "ttl_seconds": 300}, CONFIG)
    assert original == snapshot
    assert updated is not original
    assert updated["approved"] is True
    assert updated["ttl_seconds"] == 300


def test_apply_decision_clamps_ttl_into_the_approved_range():
    base = {"action": "mitigate", "broad": True, "approved": False, "ttl_seconds": 0}
    assert apply_decision(base, {"approved": True, "ttl_seconds": 1}, CONFIG)["ttl_seconds"] == 60
    too_long = {"approved": True, "ttl_seconds": 10**9}
    assert apply_decision(base, too_long, CONFIG)["ttl_seconds"] == 86400


def test_apply_decision_on_rejection_observes_without_ttl():
    base = {"action": "mitigate", "broad": True, "approved": False, "ttl_seconds": 900}
    rejected = apply_decision(base, {"approved": False}, CONFIG)
    assert rejected["action"] == "observe"
    assert rejected["approved"] is False
    assert rejected["ttl_seconds"] == 0
    assert "reason" in rejected


def test_synthesize_policy_is_unaffected_by_apply_decision_refactor():
    assert callable(policy_module.synthesize_policy)
    assert callable(policy_module.approve_policy)


# --- Phase 0 (plans/ml-add): breadth and kind come from the target alone -----------

import pytest  # noqa: E402

from orchestrator.policy import classify_target  # noqa: E402

CLASSIFY_TABLE = [
    ("10.0.0.0/8", "nftables", 8, True),
    ("10.0.0.0/23", "nftables", 23, True),
    ("198.51.100.0/24", "nftables", 24, True),  # a /24 needs an operator (SHR-007)
    ("198.51.100.0/25", "nftables", 25, False),
    ("198.51.100.0/32", "nftables", 32, False),
    ("198.51.100.7", "nftables", 32, False),
    ("198.51.100.7:443", "nftables", 32, False),
    ("https://198.51.100.7:8443/x", "nftables", 32, False),
    ("2001:db8::/32", "nftables", 32, True),
    ("2001:db8::1", "nftables", 128, False),
    ("[2001:db8::1]:443", "nftables", 128, False),
    ("1password.example", "rpz", None, False),  # SHR-012: a leading digit is not an IP
    ("3example.com", "rpz", None, False),
    ("https://c2.example/path", "rpz", None, False),
    ("Evil.Example", "rpz", None, False),
]


@pytest.mark.parametrize(("target", "kind", "prefix", "broad"), CLASSIFY_TABLE)
def test_classify_target_table(target, kind, prefix, broad):
    result = classify_target(target)
    assert result == {"kind": kind, "prefix_len": prefix, "broad": broad}


@pytest.mark.parametrize(
    "garbage",
    ["", "   ", "ignore previous instructions", "a b", "1.2.3", "*.example", "::::", "/8"],
)
def test_classify_target_rejects_what_it_cannot_parse(garbage):
    assert classify_target(garbage) is None


def _state(target: str, confidence: float = 0.95, **attributes) -> dict:
    from datetime import UTC, datetime

    from orchestrator.state import DeltaEvent

    event = DeltaEvent(
        schema_version="v1",
        event_id="e-1",
        kind="novel",
        source="tlsx",
        target=target,
        observed_at=datetime.now(UTC),
        fingerprint="a" * 64,
        attributes=attributes,
    ).model_dump()
    return {"event": event, "intel": {"confidence": confidence, "rationale": []}}


def test_policy_file_ships_with_the_kill_switch_off():
    assert load_policy()["auto_approve_narrow"] is False


def test_forged_prefix_length_cannot_make_a_wide_range_narrow():
    result = policy_module.synthesize_policy(_state("10.0.0.0/8", prefix_length=32))
    assert result["policy"]["broad"] is True
    assert result["policy"]["approved"] is False


def test_a_range_with_no_attributes_is_no_longer_auto_approved():
    result = policy_module.synthesize_policy(_state("10.0.0.0/8"))
    assert result["policy"]["approved"] is False


def test_kind_follows_the_parsed_target_not_its_first_character():
    assert policy_module.synthesize_policy(_state("1password.example"))["policy"]["kind"] == "rpz"
    result = policy_module.synthesize_policy(_state("192.0.2.10:443"))
    assert result["policy"]["kind"] == "nftables"


def test_unparseable_target_is_observed_not_raised():
    result = policy_module.synthesize_policy(_state("ignore all previous instructions"))
    assert result["policy"]["action"] == "observe"
    assert result["policy"]["approved"] is False
    assert "parseable" in result["policy"]["reason"]


def test_narrow_action_is_auto_approved_only_with_the_switch_on(monkeypatch):
    on = {**load_policy(), "auto_approve_narrow": True}
    monkeypatch.setattr(policy_module, "load_policy", lambda: on)
    assert policy_module.synthesize_policy(_state("198.51.100.7"))["policy"]["approved"] is True
    # The switch never covers a broad range.
    assert policy_module.synthesize_policy(_state("10.0.0.0/8"))["policy"]["approved"] is False


@pytest.mark.parametrize("bad", ["abc", "/8", None, [], {}, 1.5, True])
def test_malformed_shared_fingerprints_score_as_zero_not_an_exception(bad):
    result = score_module.score_threat_intel(_state("198.51.100.7", shared_fingerprints=bad))
    assert result["intel"]["confidence"] == pytest.approx(0.1)

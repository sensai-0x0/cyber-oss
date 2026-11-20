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

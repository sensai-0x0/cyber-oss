"""Phase 4 (plans/ml-add): the campaign belief scorer runs in shadow and never steers a decision."""

import json
from datetime import UTC, datetime, timedelta

import pytest
from langgraph.checkpoint.memory import MemorySaver

from orchestrator import campaign as campaign_module
from orchestrator.campaign import SCORER_ID, CampaignScorer, logit, sigmoid
from orchestrator.eval.replay import load_campaigns, replay
from orchestrator.graph import build_graph
from orchestrator.ledger import Ledger
from orchestrator.policy import load_policy
from orchestrator.state import DeltaEvent
from orchestrator.tests.evidence import BAD_JA3, BAD_JARM, ROOT
from orchestrator.tests.test_adversarial_boundaries import ADVERSARIAL_FIXTURES

DATASET = ROOT / "orchestrator" / "tests" / "fixtures" / "campaigns" / "campaigns.v1.jsonl"
T0 = datetime(2026, 3, 1, tzinfo=UTC)
HALF_LIFE = timedelta(hours=load_policy()["campaign_half_life_hours"])
PRIOR = logit(load_policy()["campaign_prior"])


def ev(event_id, when=T0, target="198.51.100.7:443", **attributes) -> dict:
    return DeltaEvent(
        schema_version="v1",
        event_id=event_id,
        kind="novel",
        source="tlsx",
        target=target,
        observed_at=when,
        fingerprint="f" * 64,
        attributes=attributes,
    ).model_dump()


@pytest.fixture
def wide_caps(monkeypatch):
    config = {**load_policy(), "campaign_cap_derived": 100.0, "campaign_cap_repeat": 100.0}
    monkeypatch.setattr(campaign_module, "load_policy", lambda: config)
    return config


def test_the_same_event_twice_does_not_move_the_posterior():
    scorer = CampaignScorer()
    first = scorer.observe(ev("e1", jarm=BAD_JARM, ja3=BAD_JA3))
    again = scorer.observe(ev("e1", jarm=BAD_JARM, ja3=BAD_JA3))
    assert again["log_odds"] == first["log_odds"]
    assert again["campaign_id"] == first["campaign_id"]
    # A genuinely new event does move it.
    moved = scorer.observe(ev("e2", target="198.51.100.8:443", jarm=BAD_JARM, ja3=BAD_JA3))
    assert moved["log_odds"] > first["log_odds"]


def test_an_event_one_half_life_older_moves_the_posterior_half_as_much(wide_caps):
    full = CampaignScorer()
    full.observe(ev("a", T0, jarm=BAD_JARM))
    newer = full.observe(ev("b", T0, target="198.51.100.8:443", jarm=BAD_JARM))["log_odds"]

    old = CampaignScorer()
    old.observe(ev("a", T0, jarm=BAD_JARM))
    older = old.observe(ev("b", T0 - HALF_LIFE, target="198.51.100.8:443", jarm=BAD_JARM))
    base = full_base = PRIOR + 1.4  # the first event alone
    assert (older["log_odds"] - base) == pytest.approx((newer - full_base) / 2)


def test_an_old_posterior_decays_when_a_much_newer_event_arrives(wide_caps):
    scorer = CampaignScorer()
    scorer.observe(ev("a", T0, jarm=BAD_JARM))
    later = scorer.observe(ev("b", T0 + HALF_LIFE, target="198.51.100.8:443", jarm=BAD_JARM))
    # 1.4 decayed to 0.7, plus a fresh 1.4 hit and a 0.5 repeat-target bonus.
    assert later["log_odds"] == pytest.approx(PRIOR + 0.7 + 1.4 + 0.5)


def test_asserted_only_evidence_stays_at_the_prior():
    scorer = CampaignScorer()
    for index in range(10):
        result = scorer.observe(
            ev(f"x{index}", target=f"198.51.100.{index + 1}:443", known_bad=True, recent_ct=True)
        )
        assert result["log_odds"] == pytest.approx(PRIOR)
        assert result["campaign_id"] is None
    assert scorer.campaign_count() == 0


def test_a_campaign_is_identified_by_indicators_not_by_target():
    scorer = CampaignScorer()
    one = scorer.observe(ev("a", target="198.51.100.7:443", jarm=BAD_JARM))
    moved = scorer.observe(ev("b", target="203.0.113.9:443", jarm=BAD_JARM))
    other = scorer.observe(ev("c", target="198.51.100.7:443", jarm="1" * 62))
    assert one["campaign_id"] == moved["campaign_id"]
    assert other["campaign_id"] != one["campaign_id"]


def test_campaigns_merge_when_an_event_links_their_indicators():
    scorer = CampaignScorer()
    a = scorer.observe(ev("a", jarm=BAD_JARM))["campaign_id"]
    b = scorer.observe(ev("b", target="198.51.100.9:443", ja3=BAD_JA3))["campaign_id"]
    assert a != b and scorer.campaign_count() == 2
    merged = scorer.observe(ev("c", target="198.51.100.10:443", jarm=BAD_JARM, ja3=BAD_JA3))
    assert scorer.campaign_count() == 1
    assert merged["campaign_id"] == min(a, b)


def test_per_class_caps_bound_the_posterior():
    scorer = CampaignScorer()
    config = load_policy()
    top = 0.0
    for index in range(40):
        top = scorer.observe(
            ev(f"c{index}", target=f"198.51.100.{index + 1}:443", jarm=BAD_JARM, ja3=BAD_JA3)
        )["log_odds"]
    assert top <= PRIOR + config["campaign_cap_derived"] + config["campaign_cap_repeat"] + 1e-9


def test_tracked_campaigns_are_bounded_and_the_oldest_are_evicted(monkeypatch):
    config = {**load_policy(), "campaign_max_tracked": 5}
    monkeypatch.setattr(campaign_module, "load_policy", lambda: config)
    scorer = CampaignScorer()
    for index in range(12):
        scorer.observe(
            ev(f"k{index}", T0 + timedelta(hours=index), jarm=f"{index:062x}")
        )
    assert scorer.campaign_count() == 5
    survivors = {row[0] for row in scorer._db.execute("SELECT value FROM indicators")}
    assert f"jarm={11:062x}" in survivors and f"jarm={0:062x}" not in survivors


def test_log_odds_round_trip():
    assert sigmoid(logit(0.3)) == pytest.approx(0.3)


class Exploding:
    name = "exploding.v1"

    def observe(self, event):
        raise RuntimeError("boom")


def graph_outcome(compiled, value):
    result = compiled.invoke({"event": value}, config={"configurable": {"thread_id": "t"}})
    interrupt = result.pop("__interrupt__", None)
    return result, interrupt[0].value if interrupt else None


def test_a_raising_challenger_is_recorded_and_the_champion_is_unaffected(tmp_path):
    value = ev("boom-1", jarm=BAD_JARM, ja3=BAD_JA3)
    plain = graph_outcome(build_graph(checkpointer=MemorySaver()), value)
    ledger = Ledger(tmp_path / "l.sqlite")
    shadowed = graph_outcome(
        build_graph(checkpointer=MemorySaver(), ledger=ledger, challengers=[Exploding()]), value
    )
    assert shadowed == plain
    # The run is waiting on an operator, so resume to reach the record node.
    assert plain[1] is not None


def test_challenger_output_lands_only_in_the_ledger_record(tmp_path):
    from langgraph.types import Command

    ledger = Ledger(tmp_path / "l.sqlite")
    compiled = build_graph(
        checkpointer=MemorySaver(), ledger=ledger, challengers=[CampaignScorer(), Exploding()]
    )
    config = {"configurable": {"thread_id": "r"}}
    compiled.invoke({"event": ev("r1", jarm=BAD_JARM, ja3=BAD_JA3)}, config=config)
    final = compiled.invoke(Command(resume={"approved": False}), config=config)
    assert "challengers" not in final and SCORER_ID not in json.dumps(final, default=str)
    record = json.loads(ledger.records()[0][2])
    assert record["challengers"][SCORER_ID]["campaign_id"]
    assert record["challengers"]["exploding.v1"]["error"] == "RuntimeError: boom"


def policy_of(compiled, value):
    return graph_outcome(compiled, value)[0].get("policy")


def shadow_events():
    adversarial = [
        DeltaEvent(
            schema_version="v1",
            event_id=f"adv-{index}",
            kind="novel",
            source="tlsx",
            target=fixture["target"],
            observed_at=T0,
            fingerprint="c" * 64,
            attributes=fixture["attributes"],
        ).model_dump()
        for index, fixture in enumerate(ADVERSARIAL_FIXTURES)
    ]
    dataset = load_campaigns(DATASET)
    sampled = [item["event"] for campaign in dataset[:30] for item in campaign["events"]]
    return adversarial + sampled


def test_shadow_isolation_policy_is_identical_with_the_challenger_on_and_off(tmp_path):
    """The most important test in the phase: the challenger can never change GraphState.policy."""
    off = Ledger(tmp_path / "off.sqlite")
    on = Ledger(tmp_path / "on.sqlite")
    challenger = CampaignScorer()
    plain = build_graph(checkpointer=MemorySaver(), ledger=off)
    shadowed = build_graph(checkpointer=MemorySaver(), ledger=on, challengers=[challenger])
    events = shadow_events()
    assert len(events) > 150
    for value in events:
        assert policy_of(shadowed, value) == policy_of(plain, value)


def test_campaign_scorer_replays_over_the_phase_3_dataset():
    report = replay(load_campaigns(DATASET), CampaignScorer())
    assert report["campaign"]["staging"] == 40
    assert report["campaign"]["detected"] <= 40


def test_refinement_changes_no_decision_on_the_phase_3_dataset(monkeypatch):
    """Evidence for SHR-014: the refinement pass re-runs a deterministic no-op."""
    from orchestrator import score as score_module

    events = [item["event"] for c in load_campaigns(DATASET)[:40] for item in c["events"]]
    with_refinement = build_graph(checkpointer=MemorySaver())
    decided_with = [policy_of(with_refinement, value) for value in events]
    off = {**load_policy(), "max_refinement_cycles": 0}
    monkeypatch.setattr(score_module, "load_policy", lambda: off)
    without_refinement = build_graph(checkpointer=MemorySaver())
    assert [policy_of(without_refinement, value) for value in events] == decided_with

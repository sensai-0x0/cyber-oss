"""Phase 3 (plans/ml-add): a dataset a scorer can fail, and a replay that measures lead time."""

import ast
import hashlib
import json
import os
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
import yaml

from lab.generate_campaigns import Knobs, dataset_sha256, dump, generate
from orchestrator.eval.replay import SCORERS, load_campaigns, replay
from orchestrator.state import DeltaEvent

ROOT = Path(__file__).parents[2]
CONFIG = yaml.safe_load((ROOT / "orchestrator" / "eval" / "eval.yaml").read_text())
DATASET = ROOT / CONFIG["dataset_path"]
BASELINE = json.loads((DATASET.parent / "baseline.json").read_text())
BADLIST = json.loads((ROOT / "lab" / "fixtures" / "badlist.v1.json").read_text())


def test_same_seed_gives_the_same_bytes():
    assert dump(generate(11)) == dump(generate(11))


def test_a_different_seed_changes_the_data_but_not_its_marginals():
    reference = generate(CONFIG["dataset_seed"])

    def stats(campaigns):
        events = [e for c in campaigns for e in c["events"]]
        return (
            sum(e["label"] == "staging" for e in events) / len(events),
            len(events) / len(campaigns),
            sum(c["label"] == "staging" for c in campaigns) / len(campaigns),
        )

    for seed in (1, 2, 3, 4, 5):
        other = generate(seed)
        assert dump(other) != dump(reference)
        for ours, theirs in zip(stats(other), stats(reference), strict=True):
            assert ours == pytest.approx(theirs, rel=0.12)


def test_the_frozen_dataset_is_pinned_and_reproducible():
    raw = DATASET.read_bytes()
    assert hashlib.sha256(raw).hexdigest() == CONFIG["dataset_sha256"]
    assert raw.decode() == dump(generate(CONFIG["dataset_seed"]))
    assert BASELINE["dataset_sha256"] == CONFIG["dataset_sha256"]


def test_generated_events_are_valid_and_obviously_synthetic():
    for campaign in generate(CONFIG["dataset_seed"]):
        for item in campaign["events"]:
            event = DeltaEvent.model_validate(item["event"])
            assert event.target.split(":")[0].rsplit(".", 1)[0] in {
                "192.0.2",
                "198.51.100",
                "203.0.113",
            }


def _features(attributes):
    listed = [
        attributes.get("jarm") in BADLIST["jarm"],
        attributes.get("ja3") in BADLIST["ja3"],
        attributes.get("favicon_mmh3") in BADLIST["favicon_mmh3"],
    ]
    shared = attributes.get("shared_fingerprints", 0)
    return {
        "known_bad": bool(attributes.get("known_bad")),
        "recent_ct": bool(attributes.get("recent_ct")),
        "shared>=1": shared >= 1,
        "shared>=2": shared >= 2,
        "jarm_listed": listed[0],
        "ja3_listed": listed[1],
        "favicon_listed": listed[2],
        "any_listed": any(listed),
        "two_listed": sum(listed) >= 2,
    }


@pytest.mark.parametrize("seed", [CONFIG["dataset_seed"], 1, 2, 3])
def test_no_single_feature_separates_the_classes(seed):
    events = [e for c in generate(seed) for e in c["events"]]
    best = 0.0
    for name in _features(events[0]["event"]["attributes"]):
        correct = sum(
            _features(e["event"]["attributes"])[name] == (e["label"] == "staging") for e in events
        )
        best = max(best, correct / len(events))
    assert best < CONFIG["separability_ceiling"], best


def test_the_old_fixture_set_was_separable_and_this_one_is_not():
    """Documents why the dataset exists: one rule used to label every old fixture correctly."""
    old = ROOT / "orchestrator" / "tests" / "fixtures" / "triage" / "scenarios.jsonl"
    rows = [json.loads(line) for line in old.read_text().splitlines() if line]
    rule = lambda a: bool(a.get("known_bad")) or a.get("shared_fingerprints", 0) >= 2  # noqa: E731
    assert all(rule(r["event"]["attributes"]) == (r["label"] == "staging") for r in rows)


class Stub:
    def __init__(self, scores):
        self.scores = scores

    def reset(self):
        pass

    def score(self, event):
        return self.scores[event["event_id"]]


def make_event(event_id, when, target="198.51.100.7:443"):
    return {
        "event": {
            "schema_version": "v1",
            "event_id": event_id,
            "kind": "novel",
            "source": "tlsx",
            "target": target,
            "observed_at": when.isoformat(),
            "fingerprint": "a" * 64,
            "attributes": {},
        },
        "label": "staging",
    }


def test_replay_reports_the_known_lead_time_on_a_three_event_timeline():
    start = datetime(2026, 3, 1, tzinfo=UTC)
    go_live = start + timedelta(hours=10)
    campaign = {
        "campaign_id": "c1",
        "label": "staging",
        "go_live_at": go_live.isoformat(),
        "events": [
            make_event("e1", start),
            make_event("e2", start + timedelta(hours=4)),
            make_event("e3", start + timedelta(hours=8)),
        ],
    }
    report = replay([campaign], Stub({"e1": 0.1, "e2": 0.9, "e3": 0.95}), threshold=0.8)
    assert report["lead_time_hours"]["values"] == [6.0]  # first proposal at +4h, live at +10h
    assert report["campaign"]["recall"] == 1.0
    assert report["event"]["tp"] == 2 and report["event"]["fn"] == 1


def test_replay_reports_late_detection_as_negative_lead_and_misses_as_none():
    start = datetime(2026, 3, 1, tzinfo=UTC)
    late = {
        "campaign_id": "late",
        "label": "staging",
        "go_live_at": (start + timedelta(hours=1)).isoformat(),
        "events": [make_event("l1", start + timedelta(hours=3))],
    }
    missed = {**late, "campaign_id": "missed", "events": [make_event("m1", start)]}
    report = replay([late, missed], Stub({"l1": 0.9, "m1": 0.0}), threshold=0.8)
    assert report["lead_time_hours"]["values"] == [-2.0]
    assert report["campaign"]["detected"] == 1 and report["campaign"]["recall"] == 0.5


def test_false_block_cost_is_split_by_breadth():
    start = datetime(2026, 3, 1, tzinfo=UTC)
    benign = {
        "campaign_id": "b",
        "label": "benign",
        "go_live_at": None,
        "events": [
            {**make_event("b1", start), "label": "benign"},
            {**make_event("b2", start, "198.51.100.0/24"), "label": "benign"},
            {**make_event("b3", start, "192.0.2.5:443"), "label": "benign"},
        ],
    }
    report = replay([benign], Stub({"b1": 1.0, "b2": 1.0, "b3": 0.0}), threshold=0.8)
    assert report["false_block_cost"] == {
        "benign_targets_proposed": 2,
        "broad": 1,
        "narrow": 1,
    }
    assert report["campaign"]["false_alarms"] == 1


def test_replay_never_touches_the_graph_or_the_filesystem(tmp_path, monkeypatch):
    source = (ROOT / "orchestrator" / "eval" / "replay.py").read_text()
    for node in ast.walk(ast.parse(source)):
        names = []
        if isinstance(node, ast.ImportFrom) and node.module:
            names = [node.module, *[a.name for a in node.names]]
        elif isinstance(node, ast.Import):
            names = [a.name for a in node.names]
        assert not any(n in {"orchestrator.graph", "build_graph", "MemorySaver"} for n in names)
    monkeypatch.chdir(tmp_path)
    replay(load_campaigns(DATASET), SCORERS["heuristic"]())
    assert os.listdir(tmp_path) == []


@pytest.mark.parametrize(
    ("name", "key"),
    [
        ("legacy", "legacy.flags"),
        ("heuristic", "heuristic.badlist.v1"),
        ("campaign", "campaign.belief.v1"),
    ],
)
def test_scorer_results_match_the_recorded_baseline(name, key):
    """A scorer change shows up here. Re-record the baseline deliberately, with the reason."""
    report = replay(load_campaigns(DATASET), SCORERS[name]())
    report["lead_time_hours"].pop("values")
    assert report == BASELINE[key]


def test_dataset_hash_helper_matches_the_file():
    assert dataset_sha256(generate(CONFIG["dataset_seed"])) == CONFIG["dataset_sha256"]
    assert Knobs().staging_campaigns == 40

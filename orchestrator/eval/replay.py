"""Replay a campaign timeline through a scorer and report lead time and false-block cost.

This module feeds events to a scorer object in time order. It never builds the LangGraph
graph, never opens a checkpointer and writes nothing, so a replay cannot persist state
outside the caller's own scorer (plans/ml-add Phase 3).
"""

import json
import statistics
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Protocol

from orchestrator.policy import classify_target, load_policy
from orchestrator.score import score_threat_intel


class Scorer(Protocol):
    def reset(self) -> None: ...

    def score(self, event: dict[str, Any]) -> float: ...


class HeuristicScorer:
    """The current scorer (provenance-aware, stateless)."""

    name = "heuristic.badlist.v1"

    def reset(self) -> None:
        return None

    def score(self, event: dict[str, Any]) -> float:
        return score_threat_intel({"event": event})["intel"]["confidence"]


class LegacyFlagScorer:
    """The pre-Phase-1 arithmetic that trusts producer flags. Kept only as a baseline."""

    name = "legacy.flags"

    def reset(self) -> None:
        return None

    def score(self, event: dict[str, Any]) -> float:
        attributes = event["attributes"]
        confidence = 0.1
        if attributes.get("recent_ct"):
            confidence += 0.2
        if attributes.get("known_bad"):
            confidence += 0.45
        shared = attributes.get("shared_fingerprints", 0)
        if isinstance(shared, int) and not isinstance(shared, bool):
            confidence += min(max(shared, 0), 3) * 0.1
        return min(confidence, 0.99)


SCORERS = {"heuristic": HeuristicScorer, "legacy": LegacyFlagScorer}


def load_campaigns(path: str | Path) -> list[dict[str, Any]]:
    return [json.loads(line) for line in Path(path).read_text().splitlines() if line]


def _ratio(numerator: int, denominator: int) -> float | None:
    return None if denominator == 0 else numerator / denominator


def replay(
    campaigns: list[dict[str, Any]], scorer: Scorer, threshold: float | None = None
) -> dict[str, Any]:
    threshold = load_policy()["mitigation_confidence"] if threshold is None else threshold
    config = load_policy()
    scorer.reset()
    timeline = sorted(
        (
            (item["event"]["observed_at"], item["event"]["event_id"], campaign, item)
            for campaign in campaigns
            for item in campaign["events"]
        ),
        key=lambda row: (row[0], row[1]),
    )
    first_proposal: dict[str, str] = {}
    counts = {"tp": 0, "fp": 0, "fn": 0, "tn": 0}
    benign_targets: dict[str, bool] = {}
    for observed_at, _, campaign, item in timeline:
        proposed = scorer.score(item["event"]) >= threshold
        staging = item["label"] == "staging"
        counts[("t" if proposed == staging else "f") + ("p" if proposed else "n")] += 1
        if not proposed:
            continue
        first_proposal.setdefault(campaign["campaign_id"], observed_at)
        if campaign["label"] == "benign" or not staging:
            derived = classify_target(item["event"]["target"], config)
            benign_targets[item["event"]["target"]] = bool(derived and derived["broad"])

    staging_campaigns = [c for c in campaigns if c["label"] == "staging"]
    benign_campaigns = [c for c in campaigns if c["label"] == "benign"]
    detected = [c for c in staging_campaigns if c["campaign_id"] in first_proposal]
    false_alarms = [c for c in benign_campaigns if c["campaign_id"] in first_proposal]
    lead_hours = sorted(
        (
            datetime.fromisoformat(c["go_live_at"])
            - datetime.fromisoformat(first_proposal[c["campaign_id"]])
        ).total_seconds()
        / 3600
        for c in detected
    )
    return {
        "threshold": threshold,
        "campaign": {
            "staging": len(staging_campaigns),
            "benign": len(benign_campaigns),
            "detected": len(detected),
            "false_alarms": len(false_alarms),
            "precision": _ratio(len(detected), len(detected) + len(false_alarms)),
            "recall": _ratio(len(detected), len(staging_campaigns)),
        },
        "event": {
            **counts,
            "precision": _ratio(counts["tp"], counts["tp"] + counts["fp"]),
            "recall": _ratio(counts["tp"], counts["tp"] + counts["fn"]),
        },
        "lead_time_hours": {
            "n": len(lead_hours),
            "median": statistics.median(lead_hours) if lead_hours else None,
            "min": lead_hours[0] if lead_hours else None,
            "max": lead_hours[-1] if lead_hours else None,
            "share_before_go_live": _ratio(sum(1 for h in lead_hours if h > 0), len(lead_hours)),
            "values": lead_hours,
        },
        "false_block_cost": {
            "benign_targets_proposed": len(benign_targets),
            "broad": sum(1 for broad in benign_targets.values() if broad),
            "narrow": sum(1 for broad in benign_targets.values() if not broad),
        },
    }


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 2 or args[0] not in SCORERS:
        print("usage: python -m orchestrator.eval.replay {heuristic|legacy} DATASET.jsonl")
        return 2
    report = replay(load_campaigns(args[1]), SCORERS[args[0]]())
    report["lead_time_hours"].pop("values")
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

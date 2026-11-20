"""P2.3 / eval E07 groundwork: triage-quality evidence for score_threat_intel.

This is diagnostic, not a gate. It runs the fixed scoring heuristic over a
labelled fixture set (orchestrator/tests/fixtures/triage/) and reports
precision/recall against the policy.yaml mitigation_confidence threshold,
for a human to review per the Phase 2 checkpoint in
plans/planv1.0/PHASE2.md (P2.3). It intentionally does not assert any
precision/recall value -- a fixed heuristic is not expected to clear eval
E07's live-model bar, and whether it does is a human call, not a test
assertion.
"""

import json
from pathlib import Path
from typing import Any, TypedDict

from orchestrator.policy import load_policy
from orchestrator.score import score_threat_intel

FIXTURES_DIR = Path(__file__).parent / "fixtures" / "triage"
FIXTURES_PATH = FIXTURES_DIR / "scenarios.jsonl"
RESULTS_PATH = FIXTURES_DIR / "last_run_results.json"


class Fixture(TypedDict):
    event: dict[str, Any]
    label: str


class TriageQualityReport(TypedDict):
    threshold: float
    total: int
    staging_count: int
    benign_count: int
    true_positive: int
    false_positive: int
    false_negative: int
    true_negative: int
    precision: float | None
    recall: float | None


def load_fixtures() -> list[Fixture]:
    lines = FIXTURES_PATH.read_text().splitlines()
    fixtures: list[Fixture] = []
    for line in lines:
        if not line.strip():
            continue
        row = json.loads(line)
        assert row["label"] in ("staging", "benign"), f"unknown label: {row['label']!r}"
        fixtures.append(row)
    return fixtures


def evaluate_triage_quality(
    fixtures: list[Fixture], threshold: float
) -> tuple[TriageQualityReport, list[float]]:
    """Score every fixture and compute precision/recall of confidence >= threshold
    against the "staging" label. Returns the report plus the raw per-fixture
    confidences (in fixture order) so the caller can assert on them.
    """
    true_positive = 0
    false_positive = 0
    false_negative = 0
    true_negative = 0
    confidences: list[float] = []

    for fixture in fixtures:
        result = score_threat_intel({"event": fixture["event"]})
        confidence = result["intel"]["confidence"]
        confidences.append(confidence)
        predicted_positive = confidence >= threshold
        actual_positive = fixture["label"] == "staging"

        if predicted_positive and actual_positive:
            true_positive += 1
        elif predicted_positive and not actual_positive:
            false_positive += 1
        elif not predicted_positive and actual_positive:
            false_negative += 1
        else:
            true_negative += 1

    precision = (
        true_positive / (true_positive + false_positive)
        if (true_positive + false_positive) > 0
        else None
    )
    recall = (
        true_positive / (true_positive + false_negative)
        if (true_positive + false_negative) > 0
        else None
    )

    report: TriageQualityReport = {
        "threshold": threshold,
        "total": len(fixtures),
        "staging_count": sum(1 for f in fixtures if f["label"] == "staging"),
        "benign_count": sum(1 for f in fixtures if f["label"] == "benign"),
        "true_positive": true_positive,
        "false_positive": false_positive,
        "false_negative": false_negative,
        "true_negative": true_negative,
        "precision": precision,
        "recall": recall,
    }
    return report, confidences


def test_triage_quality_runs_and_reports_precision_recall() -> None:
    """Diagnostic only: confirms the pipeline runs end-to-end over the labelled
    set and produces a numeric confidence for every fixture, then reports
    precision/recall for human review. Does NOT assert a precision/recall
    threshold -- see module docstring and plans/planv1.0/PHASE2.md P2.3.
    """
    fixtures = load_fixtures()
    assert len(fixtures) > 0, "triage fixture set is empty"

    policy = load_policy()
    threshold = policy["mitigation_confidence"]
    assert isinstance(threshold, float)

    report, confidences = evaluate_triage_quality(fixtures, threshold)

    # The pipeline must produce a well-formed, bounded confidence for every
    # fixture -- this is the "runs without error" assertion the task calls
    # for, not a quality gate.
    assert len(confidences) == len(fixtures)
    for confidence in confidences:
        assert isinstance(confidence, float)
        assert 0.0 <= confidence <= 1.0

    precision_text = "undefined (no positive predictions)" if report["precision"] is None else (
        f"{report['precision']:.3f}"
    )
    recall_text = (
        "undefined (no staging fixtures)" if report["recall"] is None else f"{report['recall']:.3f}"
    )

    print("\n--- Triage quality report (P2.3 / eval E07 groundwork, diagnostic only) ---")
    print(f"threshold (policy.yaml mitigation_confidence): {report['threshold']}")
    print(f"fixtures: {report['total']} (staging={report['staging_count']}, "
          f"benign={report['benign_count']})")
    print(f"true_positive={report['true_positive']} false_positive={report['false_positive']} "
          f"false_negative={report['false_negative']} true_negative={report['true_negative']}")
    print(f"precision: {precision_text}")
    print(f"recall: {recall_text}")
    print("This result is diagnostic evidence for the Phase 2 human checkpoint; "
          "it is not a pass/fail assertion.")

    RESULTS_PATH.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")

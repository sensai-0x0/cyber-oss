# orchestrator

The `orchestrator` package is the cluster -> intel -> mitigate -> approval LangGraph: `plan_or_reason` routes tiered mitigation decisions into an auto-executed safe tool path or a human approval gate.

## Changing policy.yaml

`orchestrator/policy.yaml` (`mitigation_confidence`, `broad_prefix_threshold`, `default_ttl_seconds`, `min_approved_ttl_seconds`, `max_approved_ttl_seconds`, `max_refinement_cycles`) encodes risk-tuning decisions, not engineering defaults. The v1.0 values were approved via the Human Checkpoint in `plans/planv1.0/PHASE0.md` (task P0.3). Any future change to a value in this file must follow the same process:

1. **State the operational motivation, not just the new value.** The PR/commit description must explain *why* the threshold is being moved (observed false-positive/negative rate, incident, operator feedback) — "bump to 0.9" alone is not sufficient justification.
2. **Get explicit human sign-off before merge.** An agent may propose a new value but must never set it unilaterally. This mirrors the precedent set by the original v1.0 defaults, which required the Human Checkpoint sign-off recorded in `plans/planv1.0/PHASE0.md` P0.3 — a policy.yaml change is not a routine edit and does not get to skip that same review.
3. **Run the full test suite afterward and confirm it passes.** `uv run pytest` must be green, including the policy boundary test suite that exercises confidence/TTL boundary behavior (`orchestrator/tests/test_adversarial_boundaries.py`, if present on this branch) — a threshold change that silently shifts routing behavior at a boundary must be caught here, not discovered in production.
4. **Append a decision log entry.** Add one line to the "Decision log" section of `spec/STATE.md` with the date, the changed value, old -> new, and who approved it, following the existing entry format in that section.

No value in `policy.yaml` should ever change without all four steps completed in the same PR.

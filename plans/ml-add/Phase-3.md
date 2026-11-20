# Phase 3: Eval substrate, with non-separable data and lead-time replay (K7, K9)

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-2.md](Phase-2.md), E07 in [../../.agents/evals/evals.md](../../.agents/evals/evals.md) · **Updated:** 2026-11-20

**Depends on:** Phase 2. **Blocks:** Phase 4. The scorer cannot be measured until this exists.

## Goal

An eval that a scorer can fail. Today one rule labels all 100 fixtures correctly (`known_bad or shared_fingerprints ≥ 2`, reproduced), so the current fixtures cannot tell scorers apart.

## Order change from the source

The source puts K9 after K11. This plan builds the measurement first. If the scorer is built first, it gets tuned against the only data that exists, which is the separable set.

## Scope

**In**

1. **Generator** `lab/generate_campaigns.py` (planned). It is seeded and deterministic, and uses synthetic RFC 5737 and `.example` names only. It produces *campaigns*, each a timeline of DeltaEvents with a labelled `go_live_at`, plus benign churn. It has knobs for:
   - indicator overlap between benign and staging
   - label noise
   - how often a campaign rotates its JARM, cert and favicon
   - the share of staging events that carry no strong indicator

   The generator must create cases where no single feature separates the classes. A test asserts this: the best single-feature threshold does worse than a stated ceiling, and the ceiling is a `policy.yaml`-style eval key, not a number in this file.
2. **Replay harness** `orchestrator/eval/replay.py` (planned). It feeds a timeline through any scorer, in time order, and reports:
   - **lead time**: hours between the first above-threshold proposal and `go_live_at`, as a distribution
   - precision and recall at the campaign level and at the event level
   - **false-block cost**: the number of benign targets proposed, broken down by `broad`
3. A frozen dataset, `orchestrator/tests/fixtures/campaigns/` (planned), produced by the generator with a fixed seed. Its hash is pinned.
4. The baseline: run today's scorer and the Phase 1 scorer, and record the numbers in the merge message.

**Out:** self-play (K10, which is Phase 7).

## Known weakness, stated in advance

The generator and the scorer will be written by the same team, so a good score here shows that the scorer fits the generator's assumptions. It does not show performance in the real world. Phase 6 must not accept synthetic lead time as the sole evidence for switching the champion. Mitigation: a second person writes the generator's adversarial knobs, without seeing the Phase 4 scorer design.

## Tests first

- Same seed gives the same bytes. A different seed gives a different dataset with the same marginal statistics.
- The separability test above.
- The replay harness returns a known lead time on a hand-built three-event timeline.
- The harness never calls `build_graph()` with a checkpointer that persists outside `tmp_path`.

## Acceptance

- [ ] Tests pass. Baseline numbers are recorded. `test_triage_quality.py` keeps running as is, and the new eval sits beside it.
- [ ] 🔒 HUMAN CHECKPOINT: a reviewer agrees the dataset is hard enough. This is a judgement call, made before Phase 4 starts.

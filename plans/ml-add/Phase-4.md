# Phase 4: Campaign belief scorer, shadow only (K11)

**Status:** active · **Derived from:** [README.md](README.md), [Phase-3.md](Phase-3.md), SHR-014 · **Updated:** 2026-11-20

**Depends on:** Phases 2 and 3, and ADR 0004's campaign-identity decision. **Blocks:** Phase 6.

## Goal

A challenger scorer that keeps a log-odds posterior for each campaign, with half-life decay and a cap on each class of evidence. It runs on every event, and its output goes **only** to `ledger.challengers[scorer_id]`. The champion scorer and `GraphState.policy` are unchanged.

This is also where the refinement loop (SHR-014) either gets a real job or is removed. On the second pass, refinement can pull in the campaign's posterior. If that does not change any decision on the Phase 3 dataset, delete the cycle and record it in the ADR. Do not keep it out of hope.

## Scope

**In**

1. `orchestrator/campaign.py`:
   - campaign key: per ADR 0004, for example a stable hash of OBSERVED indicators, **not** the target
   - posterior update
   - decay
   - per-class caps, so ASSERTED evidence is capped at zero
2. The campaign state store, as chosen in ADR 0004. It is bounded: the oldest campaigns are evicted, with a policy key for the limit.
3. Shadow wiring: after the champion scores, the challenger scores the same event and the ledger records both.
4. New `policy.yaml` keys: half-life, per-class caps and the eviction limit. Each is added along with a STATE.md decision-log entry, following `orchestrator/README.md`.

**Out:**

- changing the champion
- any use of challenger output in routing
- learned parameters: the priors are hand-set and documented

## Tests first

- Same event twice: the posterior does not move twice. Updates are idempotent on `event_id`.
- An event older than one half-life moves the posterior by half as much.
- A campaign whose only evidence is ASSERTED stays at the prior.
- **Shadow isolation:** across the whole adversarial suite and the Phase 3 dataset, `GraphState.policy` is identical with the challenger on and with it off. This is the most important test in the phase.
- If the challenger raises an exception, the champion's result is unaffected and the ledger records the challenger error.

## Acceptance

- [x] Tests pass. The Phase 3 replay report compares champion and challenger, covering lead time, false-block cost and precision.
- [x] The decision on SHR-014 is recorded: refinement either kept with evidence, or removed.

## Result

The challenger is `orchestrator/campaign.py` (`campaign.belief.v1`). It runs inside the ledger's `record` node, after the champion has decided, and only when `build_graph(ledger=..., challengers=[...])` is given both. Its output goes into `ledger.challengers[scorer_id]`; an exception is recorded as `{"error": ...}`. State is a Python-owned SQLite store, bounded by the policy key `campaign_max_tracked`. New policy keys: `campaign_half_life_hours`, `campaign_prior`, `campaign_cap_derived`, `campaign_cap_repeat`, `campaign_max_tracked`.

Replay on the Phase 3 dataset (recorded in `orchestrator/tests/fixtures/campaigns/baseline.json`):

| Scorer | Campaign recall | Campaign precision | Median lead time (h) | Benign targets proposed |
|---|---|---|---|---|
| `heuristic.badlist.v1` (champion) | 0.50 | 0.28 | 37.4 | 70 |
| `campaign.belief.v1` (challenger) | 0.50 | 0.28 | 37.4 | 69 |

**The challenger is not better.** It matches the champion within noise. The reason is in the data: benign events that carry a listed indicator reach the threshold under either scorer, and campaign belief has no way to tell shared benign infrastructure from a campaign (the rarity idea, K3, is deferred in Phase 7). The prior (`campaign_prior`) was set so the posterior can reach the mitigation threshold at all; no parameter was tuned against these numbers, to avoid fitting the synthetic data. Phase 6 should therefore record **no switch** on this evidence.

**SHR-014 decision: remove the refinement cycle.** `test_refinement_changes_no_decision_on_the_phase_3_dataset` shows identical policy with refinement on and off, because `cluster_event` is deterministic and the scorer never reads its output. Removing it changes `cycles`, `max_refinement_cycles` and a group of tests, so the removal is left as its own change; recorded in ADR 0004.

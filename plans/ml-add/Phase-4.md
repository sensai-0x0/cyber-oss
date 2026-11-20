# Phase 4: Campaign belief scorer, shadow only (K11)

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-3.md](Phase-3.md), SHR-014 · **Updated:** 2026-11-20

**Depends on:** Phases 2 and 3, and ADR 0004's campaign-identity decision. **Blocks:** Phase 6.

## Goal

A challenger scorer that keeps a log-odds posterior for each campaign, with half-life decay and a cap on each class of evidence. It runs on every event, and its output goes **only** to `ledger.challengers[scorer_id]`. The champion scorer and `GraphState.policy` are unchanged.

This is also where the refinement loop (SHR-014) either gets a real job or is removed. On the second pass, refinement can pull in the campaign's posterior. If that does not change any decision on the Phase 3 dataset, delete the cycle and record it in the ADR. Do not keep it out of hope.

## Scope

**In**

1. `orchestrator/campaign.py` (planned):
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

- [ ] Tests pass. The Phase 3 replay report compares champion and challenger, covering lead time, false-block cost and precision.
- [ ] The decision on SHR-014 is recorded: refinement either kept with evidence, or removed.

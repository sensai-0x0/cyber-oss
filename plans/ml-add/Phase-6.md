# Phase 6: Champion switch (human checkpoint)

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-4.md](Phase-4.md) · **Updated:** 2026-11-20

**Depends on:** Phases 0–4. Phase 5 is recommended. Every prerequisite below must hold.

## Goal

A named human decides, using written evidence, whether the campaign belief scorer replaces the heuristic as champion. This phase produces a decision, not code. The code change at the end is a one-line `policy.yaml` selector (planned key `champion_scorer`).

## Prerequisites (all must hold at the time of the decision)

Each is checked by running something, not by reading a status table:

| Prerequisite | Check |
|---|---|
| Go derives `broad` from the target | SHR-001 is closed. A test signs a `/8` sent with `broad:false` and sees it rejected |
| Expired and replayed actions are rejected | SHR-002 and SHR-005 are closed, with tests at `-1s`, `0` and `+1s` |
| No unsigned path reaches a backend | SHR-009 is closed |
| The reaper survives a restart and a revoke error | SHR-003 and SHR-004 are closed |
| Phase 0's kill switch is still off, or turning it on is a separate decision | `policy.yaml: auto_approve_narrow` (planned) |
| Shadow evidence covers at least N weeks of **non-synthetic** events, or the decision says explicitly that only synthetic evidence exists | Ledger record count by source. N is set in the decision, not here |

If real event data does not exist, the honest outcome is **no switch**. The plan treats that as a valid result.

## Evidence pack (`.agents/eval-results/` (planned file per run))

- Champion vs challenger on the Phase 3 dataset: lead-time distribution, false-block cost split by `broad`, and campaign precision.
- Shadow disagreement on whatever real ledger exists: every case where the challenger would have proposed and the champion did not, and the reverse, each with its evidence.
- The adversarial suite run against the challenger as champion.

## Decision record

A `spec/STATE.md` decision-log line in the format `orchestrator/README.md` sets out, naming the approver. Reverting is a `policy.yaml` change, made under the same process.

## Explicitly not allowed

- Switching the champion and enabling `auto_approve_narrow` in the same change.
- Using lead time as the only metric. False-block cost has the same weight.

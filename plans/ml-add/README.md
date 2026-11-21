# ML add: Campaign Belief Ledger, phased

**Status:** proposed · **Derived from:** [source-ml-and-kernel-review-v2.md](source-ml-and-kernel-review-v2.md), [../../docs/reviews/2026-11-21-codebase.jsonl](../../docs/reviews/2026-11-21-codebase.jsonl), authorizing ADR `spec/DECISIONS/0004-campaign-belief-ledger.md` (accepted, Phase 0) · **Updated:** 2026-11-20

This is a design that has not been built. **None of these files is an instruction to build anything** until Phase 0's ADR is accepted.

## Why the phases differ from the source

The source proposes building in this order: K12 → K13+K1 → K11 → K9 → K10. I checked it against the code at `9c57762`. Five problems change the order:

1. **The safety precondition is wider than SHR-007.** `orchestrator/policy.py` reads `broad` from `attributes.prefix_length`, which defaults to 32. A target of `10.0.0.0/8` without that attribute is classed as narrow and auto-approved (reproduced). That leaves the `/24` boundary as a side issue. The real fault is that any CIDR is narrow unless the producer chooses to say otherwise. It needs its own phase, ahead of any scoring work.
2. **K12 does not establish provenance.** It would classify fields by *name* ("a field the probe adapter emits"). But `hunterd serve` never runs a probe: it validates NDJSON from stdin and echoes it. So whoever writes stdin can set `jarm` just as easily as `known_bad`. Classifying by name narrows the forgeable surface, but it does not authenticate anything, and it cannot "fix SHR-008". Phase 1 states that limit plainly. Real provenance needs an authenticated producer, which is outside this plan.
3. **K13 changes a signed contract.** Committing the operator's review to an evidence digest changes `enforcerd.canonicalAction`, and `schemas/mitigation_action.v1.json` is `additionalProperties: false`. That means a v2 schema, an ADR and a Go change at the trust boundary. It carries the most risk of any step, so it is split from the ledger (K1) and moved after it.
4. **Nothing produces real data.** Probes, the delta gate, the cursor and the enforcer are unconnected libraries, there is no NATS client, and there is no event history. K9 (lead-time replay) and K10 (self-play) would therefore run only on synthetic data written by the same people who write the scorer. That is circular. Phase 3 says so, and Phase 6 does not allow a lead-time number from synthetic data to justify switching the champion.
5. **K11 needs storage that does not exist.** A per-campaign posterior has to persist across events. Badger belongs to `hunterd`, one process per directory (`.agents/orchestration/invariants.md` I17), and `cluster_id` is a hash of a single target. Choosing a store and defining campaign identity are decisions for Phase 0, not details to settle during implementation.

Two of the source's own claims are wrong:

- *"The top three are stable across every weighting except ship-fast."* Under creativity-first, K1 ranks #5.
- K12 and CBL both score 4.55 under balanced but are listed as #1 and #2. That is a tie, not a ranking.

The per-criterion raw scores are not published, so none of the weighted totals can be reproduced. Treat the table as the author's opinion, not as evidence.

## Phases

| Phase | Name | Changes runtime behaviour? | Touches a trust boundary? |
|---|---|---|---|
| [0](Phase-0.md) | Governance and safety preconditions | Yes: removes auto-approval | Yes (Python policy) |
| [1](Phase-1.md) | Evidence provenance classes (K12) | Yes: scorer inputs | No |
| [2](Phase-2.md) | Append-only evidence ledger (K1) | No: write-only side channel | No |
| [3](Phase-3.md) | Eval substrate: non-separable data and lead-time replay (K7, K9) | No | No |
| [4](Phase-4.md) | Campaign belief scorer, shadow only (K11) | No | No |
| [5](Phase-5.md) | Proof-carrying operator review (K13) | Yes | **Yes**: signed schema v2 |
| [6](Phase-6.md) | Champion switch: human checkpoint | Yes | Yes |
| [7](Phase-7.md) | Deferred consumers (K10, K5, K3, K2) | — | — |

The rules every phase follows:

- Each phase is one feature branch, merged on its own, and leaves `develop` green. A phase can be reverted without reverting the ones after it, except where its *Depends on* line says otherwise.
- Tests come first, per [../../CLAUDE.md](../../CLAUDE.md).
- No phase may put model output into `broad`, `kind`, TTL, scope, signature or expiry. Those stay deterministic.
- No threshold is pasted into these files. Each new tunable gets a key in `orchestrator/policy.yaml`, added in the phase that introduces it, and is cited by name from then on.
- K14 (operator-preference autofill) stays rejected and has no phase.

## Not in this plan

These must land before Phase 6, but each is its own repair, tracked by SHR id in the codebase review:

- Go-side broad derivation (SHR-001)
- expiry and replay checks (SHR-002, SHR-005)
- mandatory signature verification (SHR-009)
- reaper durability (SHR-003, SHR-004)
- scope enforcement (SHR-011)

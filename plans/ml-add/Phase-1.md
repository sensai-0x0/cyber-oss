# Phase 1: Evidence provenance classes (K12)

**Status:** accepted · **Derived from:** [README.md](README.md), [Phase-0.md](Phase-0.md), SHR-008 · **Updated:** 2026-11-20

**Depends on:** Phase 0. **Blocks:** Phases 2–4.

## Goal

Every attribute the scorer reads carries one of three provenance classes. The ASSERTED class contributes nothing to confidence, and every time ASSERTED evidence is dropped, that is visible in the rationale.

| Class | Definition in this phase | Honest limit |
|---|---|---|
| OBSERVED | The key is on a per-`source` allowlist of the fields that `hunterd/probes/*.go` normalisers emit | The field **name** is checked, not who produced it. Anyone who writes to `hunterd serve`'s stdin can forge an OBSERVED field |
| DERIVED | Computed by the orchestrator from a pinned artefact whose SHA-256 is recorded in `policy.yaml` (for example, a badlist checked against `jarm`) | Only as trustworthy as whoever pins the artefact |
| ASSERTED | Everything else, including `known_bad`, `recent_ct` and `shared_fingerprints` as they arrive in attributes today | — |

**This phase does not fix SHR-008.** It removes the cheapest forgeries, the boolean flags. Closing SHR-008 needs an authenticated event producer, such as signed events or in-process probes, and this plan does not deliver that. Leave SHR-008 open.

## Scope

**In**

1. `orchestrator/provenance.py`:
   - the allowlist table, kept as data so it can be diffed against `hunterd/probes/` in review
   - `classify(source, key) -> Class`
   - DERIVED lookups against a pinned artefact
2. `orchestrator/score.py`:
   - reads only OBSERVED and DERIVED values
   - parses them strictly, so the string `"false"` no longer counts as truthy and non-integers do not raise
   - adds a `dropped_asserted: [...]` list to `IntelScore`
3. A contract test asserting the allowlist matches the attribute keys produced by `normalizeTLSResponse` and `normalizeHTTPResponse`. It reads a JSON fixture that a Go test writes out, so the two languages cannot drift apart silently.
4. The pinned lab badlist: `lab/fixtures/badlist.v1.json`. It holds synthetic values only.

**Out:** learned weights. The weights stay hand-set in this phase.

## Expected effect, stated in advance

At HEAD, the scorer catches none of the 30 staging fixtures (recall 0.0, from `orchestrator/tests/fixtures/triage/last_run_results.json`). After this phase it still catches none, because the fixtures rely on ASSERTED flags. That outcome is correct and should be recorded as such. It is a reason for Phase 3, not a regression.

## Tests first

- `"known_bad": "false"`, `"known_bad": true` and `"shared_fingerprints": 3` each add 0 confidence, and each appears in `dropped_asserted`.
- A `jarm` that is on the pinned badlist adds DERIVED weight. A badlist file whose hash does not match makes `load_policy()` refuse to load.
- The cross-language allowlist contract test.
- The adversarial suite (`test_adversarial_boundaries.py`) still passes.

## Acceptance

- [x] Tests pass. `test_triage_quality.py` output is updated, and the drop in recall is noted in the merge message.
- [x] SHR-008 stays open in the review register, annotated as "mitigated on scoring path; producer unauthenticated".

## Rollback

Revert. The scorer goes back to trusting producer flags, while Phase 0's kill switch still keeps a human in the loop.

## Result

Built as specified. Two independent badlist hits (`jarm` and `ja3` on `tlsx`, or `favicon_mmh3` on `httpx`) are DERIVED evidence, each worth a hand-set weight in `orchestrator/score.py`. Recall on the 30 staging fixtures stays 0.0, as predicted, because those fixtures rely on asserted flags. Consequence to know before Phase 4: no real event can reach the default mitigation threshold from one indicator alone, so mitigation proposals are now rare by design.

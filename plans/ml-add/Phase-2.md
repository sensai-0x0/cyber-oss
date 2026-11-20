# Phase 2: Append-only evidence ledger (K1)

**Status:** proposed · **Derived from:** [README.md](README.md), ADR 0004 (accepted, [Phase-0.md](Phase-0.md)) · **Updated:** 2026-11-20

**Depends on:** Phases 0 and 1. **Blocks:** Phases 3–5.

## Goal

Every triage run appends one record: the event, the provenance-classified evidence, the score, the rationale, the decision and the policy version. Records are hash-chained, so editing or deleting one can be detected. The ledger is **write-only from the graph's point of view**: no graph node ever reads it to make a decision.

## Why it is separate from K13

The source bundles K1 and K13. K1 can be built entirely in Python, outside the decision path, and reverses cleanly. K13 changes what the operator signs (see Phase 5). Putting them on one branch would make it impossible to revert the cheap part without also reverting the dangerous part.

## Scope

**In**

1. `orchestrator/ledger.py` (planned). Records go into the store chosen in ADR 0004. Each record is:

   ```
   {seq, prev_hash, record_hash, event, evidence[{key, value, class}],
    intel, policy, policy_sha256, scorer_id, recorded_at}
   ```

   - `record_hash = sha256(canonical_json(record without record_hash))`.
   - Canonical JSON uses sorted keys, no whitespace and UTF-8. The format is fixed by a golden-file test.
2. A `record` node after `approval`, or a post-run hook (the choice goes in the ADR). If the ledger write fails, the decision does **not** change. The failure is logged, and the run is marked unrecorded.
3. `verify_chain(store) -> first_bad_seq | None`, plus a CLI entry point.
4. Fields for shadow scorers: each record has a `challengers: {scorer_id: intel}` map, which is empty in this phase.

**Out:**

- signing the records (Phase 5)
- reading the ledger during triage (never, under this plan)
- retention policy: recorded as an open question in the ADR, and the ledger must not hold real targets in the lab

## Tests first

- Append N records, then `verify_chain` returns None. Flip one byte in record k, and it returns k. Delete record k, and it returns k.
- Make the ledger write fail. The graph result is identical, bit for bit, to a run with no ledger.
- Property test: the graph output is the same with the ledger on and with it off.
- After a SQLite checkpointer restart (reusing `test_checkpointer_restart.py`), the resumed run appends exactly one record, not two.

## Acceptance

- [ ] Tests pass, and `verify_chain` runs in CI against a fixture ledger.
- [ ] A test proves no graph node imports `orchestrator.ledger` for reading.

## Rollback

Revert. Decision logic does not change.

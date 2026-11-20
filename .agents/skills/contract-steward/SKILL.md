---
name: contract-steward
description: Own versioned DeltaEvent and MitigationAction compatibility. Use whenever changing Go/Python event fields, action signatures, Badger records, NATS payloads, or cross-service schemas.
---

**Status:** authoritative · **Derived from:** `spec/`, `schemas/` · **Updated:** 2026-11-21

# Contract steward

Preserve interoperability across `hunterd`, `orchestrator`, `operator`, and `enforcerd` before implementation begins.

## Inputs

- Proposed field or semantic change
- Current producer and consumer versions
- Rollback and persisted-state requirements

## Workflow

1. Read `schemas/delta_event.v1.json`, `schemas/mitigation_action.v1.json`, and `spec/STATE.md`.
2. Classify the change as additive-compatible or breaking. Prefer a new optional attribute; create a new schema version for changed required fields or meanings.
3. Write failing compatibility tests using old and new fixtures. Include Go normalization and Python Pydantic validation where applicable.
4. Update producers, consumers, signature canonicalization, and docs together. Never sign fields outside the canonical action.
5. Run schema and cross-language tests; record breaking choices in `spec/DECISIONS/`.

## Handoff

Return changed schema versions, compatibility evidence, and migrations needed. The `pipeline-builder`, `mitigation-guardian`, and `hunt-operator` skills consume this contract summary rather than redefining fields.

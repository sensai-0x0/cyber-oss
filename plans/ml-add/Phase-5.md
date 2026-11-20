# Phase 5: Proof-carrying operator review (K13)

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-2.md](Phase-2.md), `schemas/mitigation_action.v1.json`, [../../spec/DECISIONS/0002-reversible-mitigation.md](../../spec/DECISIONS/0002-reversible-mitigation.md) · **Updated:** 2026-11-20

**Depends on:** Phase 2 and its own ADR, `spec/DECISIONS/0005-mitigation-action-v2.md` (accepted). **Blocks:** nothing in this plan. It can be built alongside Phases 3 and 4.

## Goal

The operator's Ed25519 signature commits to the evidence the operator was shown, so an approval is bound to a specific ledger record.

## Why this is the riskiest phase

It changes the signed bytes. `enforcerd.canonicalAction` defines exactly what gets signed, and `mitigation_action.v1.json` rejects unknown fields. Adding `evidence_digest` and `ledger_head` therefore means three things:

- a **v2 schema**, with v1 left frozen
- new canonical bytes, with a golden-vector test in both Go and Python
- an enforcer that either accepts v1 and v2 for a defined deprecation window, or does not. The ADR decides which.

Run the `contract-steward` skill (`.agents/skills/contract-steward/SKILL.md`) before writing any code.

## Scope

**In**

1. `schemas/mitigation_action.v2.json` (planned) adds `evidence_digest` (the SHA-256 of the ledger record) and `ledger_seq`.
2. The interrupt payload from `approve_policy` carries both.
3. `operator review` gets a `-ledger PATH` flag (planned). It fetches record `ledger_seq`, recomputes the digest, and **refuses** to sign on a mismatch. It shows the operator the evidence lines, not only `{id, kind, target, broad}`.
4. `canonicalAction` covers the new fields when the version is v2.
5. Go-side `broad` derivation (SHR-001) is a **hard prerequisite** of this phase. Making a signature commit to the evidence while it still trusts a caller-supplied `broad` would add tamper evidence while leaving the actual hole open.

**Out:** anchoring the ledger head anywhere external.

## Tests first

- Golden vectors: the same v2 action canonicalises to the same bytes in Go and Python.
- Tamper the ledger record after the interrupt, and the operator refuses to sign.
- A v1 signature does not verify as v2, and a v2 signature does not verify as v1.
- Every existing v1 test still passes until the ADR's cut-over date.

## Acceptance

- [x] ADR 0005 is accepted (2026-11-20, owner). Both schemas are tested in `orchestrator/tests/test_schemas.py`.
- [ ] `/security-review` runs on the diff. It is mandatory here, because the diff touches invariants.
- [ ] 🔒 HUMAN CHECKPOINT: the trust-boundary change is signed off.

## Rollback

The enforcer keeps accepting v1. Turning off v2 issuance in the operator CLI restores the previous behaviour, so no enforcer rollback is needed.

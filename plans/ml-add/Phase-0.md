# Phase 0: Governance and safety preconditions

**Status:** accepted · **Derived from:** [README.md](README.md), SHR-007, SHR-008, SHR-012 in [../../docs/reviews/2026-11-21-codebase.jsonl](../../docs/reviews/2026-11-21-codebase.jsonl) · **Updated:** 2026-11-20

**Depends on:** nothing. **Blocks:** every later phase.

## Goal

Make it impossible for any scorer, current or future, to cause a mitigation that no human approved. Then record the decisions that the rest of the plan relies on.

## Why this comes first

The source's R1 is correct but too narrow. Today, `synthesize_policy` sets `approved: not broad`. `broad` comes from `attributes.prefix_length`, which the producer supplies and which defaults to 32. As a result, three bad inputs are all approved without a human (each reproduced against `9c57762`):

- `10.0.0.0/8` with no `prefix_length` is classed as narrow and approved.
- `1password.example` gets kind `nftables` (SHR-012), because the code checks only the first character.
- `192.0.2.10:443`, the target format `tlsx` actually emits, gets kind `nftables`. The nft backend would then reject it at apply time.

If scoring improves before this is fixed, these auto-approvals become more frequent.

## Scope

**In**

1. **Policy kill switch.** Add a `policy.yaml` key `auto_approve_narrow`, defaulting to off. While it is off, every `mitigate` decision goes through `approve_policy`'s `interrupt()`, whatever `broad` says.
2. **Derive the target in Python.** Add `classify_target(target)` (in `orchestrator/policy.py`). It parses with `ipaddress`, strips a port or URL down to the host, and returns `(kind, prefix_len, broad)`. Neither `attributes.prefix_length` nor the first character is consulted. A target it cannot parse becomes `observe` with a reason, not an exception.
3. **Fix the boundary.** Decide whether `/24` counts as broad (SHR-007) and encode the answer in `policy.yaml`. The decision goes in the ADR.
4. **ADR 0004** (`spec/DECISIONS/0004-campaign-belief-ledger.md`). It records:
   - where ledger and campaign state live: a Python-owned SQLite file, or a new read API on `hunterd`, given that `hunterd` owns Badger (I17)
   - how a campaign is identified
   - that challengers never write to `GraphState.policy`
   - that this plan's phases are the accepted order
5. **Restore CI.** `validate_outlines.py --strict` fails at HEAD on `spec/STATE.md:32` (pasted thresholds). Fix it here, because every later phase has to pass the docs gate.

**Out:** Go-side fixes (SHR-001, -002, -009). Those are prerequisites for Phase 6, not for this phase.

## Tests first (`orchestrator/tests/test_policy.py`)

- Table over targets: `/8`, `/23`, `/24`, `/25`, `/32`, a bare IPv4, IPv4 with a port, IPv6, a hostname starting with a digit, a URL, and garbage. Assert the exact `(kind, broad, action)` for each.
- With `auto_approve_narrow` off, every `mitigate` row produces an interrupt, including `/32`.
- No attribute changes `broad`: `{"prefix_length": 32}` on a `/8` target still gives `broad=True`.
- Malformed attributes (`"prefix_length": "/8"`, `"shared_fingerprints": "abc"`) give `observe`, not `ValueError`.

## Acceptance

- [x] All the above tests pass, and the existing 56 tests still pass.
- [x] ADR 0004 is merged with `Status: accepted` and approved by a named human.
- [x] `validate_outlines.py --strict` exits 0.
- [x] 🔒 HUMAN CHECKPOINT (recorded in ADR 0004, repository owner): someone signs off on the `/24` decision and on the default of the kill switch.

## Rollback

Revert the branch. The only behaviour change is that more decisions go to a human, which is the safe direction.

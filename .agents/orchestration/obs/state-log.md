# State, Plans, Decisions and Errors

**Status:** active · **Derived from:** the tree and test runs; regenerated, not edited · **Updated:** 2026-11-21

Universal rules: [../../../CLAUDE.md](../../../CLAUDE.md). Read at session start; append at session end. Newest entries first. Dates as YYYY-MM-DD.

Status below is derived from the tree, per [../../outline-method.md](../../outline-method.md) §7.
The previous table listed every component of a superseded design as "Not started" while its
implemented equivalent sat in the tree under another name. Regenerate this; do not edit it
from memory.

## Current state

| Component | Path | State | Evidence |
|---|---|---|---|
| Feistel cursor | `hunterd/cursor/` | Implemented, tested | `feistel_test.go` |
| Badger store + delta gate | `hunterd/store/` | Implemented, tested | `badger_store_test.go` |
| Probe normalisation | `hunterd/probes/` | Implemented, tested | `probes_test.go` |
| CLI, gRPC health, NDJSON transport | `hunterd/cmd/hunterd/` | Implemented, tested | `main_test.go` |
| Triage graph | `orchestrator/` | Implemented, tested | `orchestrator/tests/` |
| Approval and signing | `operator/` | Implemented, tested | `decision_test.go` |
| Enforcement backends | `enforcerd/nft/`, `enforcerd/rpz/` | Implemented, tested | `backend_test.go` ×2 |
| Signature verification, TTL | `enforcerd/` | Implemented, tested | `signature_test.go`, `enforcer_test.go` |
| Event and action schemas | `schemas/` | Frozen at v1 | `orchestrator/tests/test_schemas.py` |
| Loopback lab | `lab/`, `deploy/` | Implemented | `compose config` parses |
| Outline gate | `.github/scripts/validate_outlines.py` | Implemented, tested | `.github/scripts/tests/test_validate_outlines.py` |
| Orchestrator agent + skills | `.claude/` | Implemented, uncalled | No recorded phase run |
| **Scope enforcement (I1-I5)** | — | **Absent** | No allowlist, no config loader in Go |
| **Rate limiter (I6)** | — | **Absent** | No limiter in Go |
| **Mitigation durability (I12, I13)** | — | **Absent** | Enforcer holds pending actions in memory |
| **Cursor checkpointing (I16)** | — | **Unwired** | Nothing calls `SaveCursor` |
| Durable LangGraph checkpointer | — | Absent | `build_graph()` defaults to none |
| Durable NATS consumer | — | Absent | NDJSON over stdio only |
| Eval results | `.agents/eval-results/` | Never produced | No phase has run |

Unit-tested primitives exist for every service. The gaps above are integration and safety
wiring, not missing primitives — and four of them are invariants with no enforcement point.

## Plan (next steps)

1. **Scope gate (I1-I5).** A package owning the authorized allowlist, enforced before any
   dial, wired into both probe runners, with the off-scope and redirect tests from
   [../../testing.md](../../testing.md). Highest priority in the repository; no active
   probing outside `lab/` until it lands.
2. **Mitigation durability (I12, I13).** Persist the action record before installing a
   rule; reaper on startup and on a timer.
3. **Rate limiter (I6).** One shared token bucket across both runners.
4. **Cursor checkpointing (I16).** Call `SaveCursor` periodically and on signal; prove
   resume with no gaps or duplicates.
5. **Retarget E01-E09** onto real paths per ADR 0003, then run them and record results.
6. Durable LangGraph checkpointer and NATS consumer.

## Decisions

| Date | Decision | Reason |
|---|---|---|
| 2026-11-20 | SHR-001: Go derives `broad` from the target; `operator.Approve` and `SignedBackend` refuse a disagreeing flag | Phase 5 prerequisite. Tests in `enforcerd/breadth_test.go` and `operator/decision_test.go`. A bare `Enforcer` still trusts `Action.Broad` |
| 2026-11-21 | Four-service layout is authoritative; single-binary MCP design superseded | [../../../spec/DECISIONS/0003-architecture-authority.md](../../../spec/DECISIONS/0003-architecture-authority.md). Code, schemas, CI and the accepted ADRs all implement it |
| 2026-11-21 | Outlines are gated, not merely conventional | 32 of 35 project markdown files had dead links and nothing could detect it. See [../../outline-method.md](../../outline-method.md) |
| 2026-11-21 | Language idioms split into `.agents/go.md` and `.agents/python-langgraph.md` | One file per language, grounded in the implemented code |
| 2026-11-21 | Orchestrator-side skills live in `.claude/skills/` | `validate_skills.py` caps `.agents/skills/` at five and it is full |
| 2026-11-20 | All safety bounds enforced in Go only | LLM output is untrusted. See [../invariants.md](../invariants.md) I7. Aspiration today, not implemented |
| 2026-11-20 | Only Opus 5.5, Sonnet 5.5, Haiku 5.5 used | See [../models.md](../models.md) |

## Open questions

- **Ambiguity floor.** `orchestrator/score.py` keeps `AMBIGUOUS_FLOOR` as a module
  constant; every other threshold reads from `orchestrator/policy.yaml`. Does it warrant a
  policy key?
- **Scope configuration shape.** The superseded design used a single control file. Where
  does the allowlist live now, and which component parses it? Blocks plan item 1.
- **CT streaming.** `source: ct` is valid in the schema and no client exists.
- **ASN source.** Offline database or API for the `asn` attribute.
- **Rule installation privileges.** Privilege model for nftables and RPZ writes.
- **Odd prefix widths.** Confirm the cycle-walking approach for odd `w` in the permuter.

## Error log

| Date | Where | Error | Fix / status |
|---|---|---|---|
| 2026-11-21 | `hunterd/probes/` | Both runners dial immediately; no scope check anywhere (I1-I5) | Open, plan item 1 |
| 2026-11-21 | `enforcerd/enforcer.go` | Pending actions held in memory; no pre-install persist, no reaper. A restart loses every active rule's deadline (I12, I13) | Open, plan item 2 |
| 2026-11-21 | `hunterd` | No rate limiter (I6) | Open, plan item 3 |
| 2026-11-21 | `hunterd/store` | `SaveCursor` exists but nothing calls it (I16) | Open, plan item 4 |
| 2026-11-21 | `hunterd/probes/probe.go` | `newObservation` includes `observed_at` in the fingerprint, so using it as the delta hash marks every repeat as mutated | Open; delta gate must hash only stable indicators |
| 2026-11-21 | `hunterd/cmd/hunterd` | Writes NDJSON to stdout while [../invariants.md](../invariants.md) reserves stdout for protocol output | Open; decide which rule holds |
| 2026-11-21 | `orchestrator/score.py` | `policy.yaml: max_refinement_cycles` declared but never read; cycle bound hardcoded | Fixed 2026-11-21, `orchestrator/tests/test_policy.py` |
| 2026-11-21 | `orchestrator/policy.py` | `approve_policy` mutated `state["policy"]` in place | Fixed 2026-11-21 via pure `apply_decision` |
| 2026-11-21 | `orchestrator/policy.py` | `int(decision["ttl_seconds"])` unvalidated; a non-numeric decision raises mid-node | Open; validate at the decision boundary |
| 2026-11-21 | `orchestrator/state.py` | `Policy.kind` admits `headscale` with no adapter behind it | Open; either add the backend or narrow the type |
| 2026-11-21 | `enforcerd` | `Enforcer` does not verify signatures; only `SignedBackend` does, so the guarantee depends on which wrapper a caller picks | Open; make the unsafe path unreachable |
| 2026-11-21 | Docs layer | 32 of 35 markdown files had dead links; `docs/` was referenced for files living in `spec/` | Fixed 2026-11-21; gated by `validate_outlines.py` |

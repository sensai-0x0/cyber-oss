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
| Scope enforcement (I1-I5) | `hunterd/scope` | Implemented, tested | `hunterd/scope/*_test.go`, `hunterd/probes/authorizer_wiring_test.go`. Not yet wired into `hunterd/cmd/hunterd serve` (fleet plan Phase 4); E03's packet-capture layer still has not run |
| **Rate limiter (I6)** | — | **Absent** | No limiter in Go |
| **Mitigation durability (I12, I13)** | — | **Absent** | Enforcer holds pending actions in memory |
| **Cursor checkpointing (I16)** | — | **Unwired** | Nothing calls `SaveCursor` |
| Durable LangGraph checkpointer | — | Absent | `build_graph()` defaults to none |
| Durable NATS consumer | — | Absent | NDJSON over stdio only |
| Eval results | `.agents/eval-results/` | Never produced | No phase has run |

Unit-tested primitives exist for every service. The gaps above are integration and safety
wiring, not missing primitives — and four of them are invariants with no enforcement point.

## Plan (next steps)

1. ~~**Scope gate (I1-I5).**~~ Done on `docs/ml-add-phase7-prep` (2026-11-21): `hunterd/scope`
   owns the authorized allowlist (a signed bundle), wired into both probe runners via the
   `probes.Authorizer` seam. Still open: wiring a real bundle into `hunterd/cmd/hunterd serve`
   (fleet plan Phase 4), HC-2 (scope key custody), and E03's packet-capture layer. No active
   probing outside `lab/` until those land too.
2. **Mitigation durability (I12, I13).** Persist the action record before installing a
   rule; reaper on startup and on a timer.
3. **Rate limiter (I6).** One shared token bucket across both runners.
4. **Cursor checkpointing (I16).** Call `SaveCursor` periodically and on signal; prove
   resume with no gaps or duplicates.
5. **Retarget E01-E09** onto real paths per ADR 0003, then run them and record results.
6. Durable LangGraph checkpointer and NATS consumer.
7. **Fleet (proposed, 2026-11-20).** Phases 0 to 9 in [../../../plans/fleet/README.md](../../../plans/fleet/README.md). Phase 1 there is plan item 1 above (scope gate), delivered as a signed scope bundle. Capacity figures and the interface catalogue: [../../../plans/fleet/interfaces-and-agent-flow.md](../../../plans/fleet/interfaces-and-agent-flow.md).

## Decisions

| Date | Decision | Reason |
|---|---|---|
| 2026-11-21 | `develop` merged into `main` (human-approved) | `main` had independently diverged with its own 5 commits (gitignore/`uv.lock` cleanup, an orchestrator rename, the agent-graph docs PR) that never passed through `develop`; a fast-forward was not possible. Merged with one real conflict, in this file's own Decisions table (two independent inserts at the same point), resolved by keeping both blocks. `uv.lock` auto-merged cleanly. Full gate re-run on the merge result before push |
| 2026-11-21 | `orchestrator` names one thing: the Python package. The build loop is now [../../evals/phase-runner.md](../../evals/phase-runner.md) and its agent is `.claude/agents/phase-runner.md`; `conductor` is unchanged | Three agents named two roles, and two of the three descriptions disambiguated themselves by negation. Documentation layer, so no refactor gate applies |
| 2026-11-21 | The package is **not** renamed to `triage`, though that is the accurate name | `refactor-eval` scores the rename `S = 0`, flag Critical. Row in `.claude/skills/refactor-eval/scorecard.md`; reasoning in [../../../plans/agent-graph.md](../../../plans/agent-graph.md) §11. Revisit only as its own ADR before new modules land |
| 2026-11-21 | Open work items get a register: `docs/backlog/backlog.md`, one datetime-stamped line each, append-only, with `docs/backlog/README.md` defining the grammar and which of the four registers owns what. Idle agents work it oldest first (CLAUDE.md) | Backlog items were spread across the error log, the review register, plan open-question sections and conversation, so nothing could be worked oldest-first or counted. Twelve entries registered at creation |
| 2026-11-21 | Graph contract and task ladder for the LangGraph plane proposed in [../../../plans/agent-graph.md](../../../plans/agent-graph.md); it asks for ADR 0008 | Needed a single home for the graph's topology, the determinism rules and the one interrupt. ADR 0008 is unwritten, so §4 is a design and G0–G8 are unstarted |
| 2026-11-21 | Integration gate: `feature/fleet-p1-scope-bundle` reviewed, fixed, merged to `develop` | `/code-review` and `/security-review` found: (High) `HTTPRunner` had no per-dial re-check for a hostname target, so DNS rebinding between the L1 `CheckHost` resolution and httpx's own independent internal resolution bypassed I2 for any domain-only bundle — fixed by resolving and checking every address in `HTTPRunner` itself, then pinning the dial to one validated address via httpx's `"host,ip"` input form; (Medium) an exclusion written in IPv4-mapped-IPv6 form (`::ffff:a.b.c.d/128`) never matched the plain IPv4 form a real check uses, so it silently failed to deny the address it named — fixed with `unmapPrefix`; `ed25519.Verify` panics rather than fails closed on a wrong-length public key — `Parse` now checks the length first; `hunterd/scope.Sign` contradicted hunterd/CLAUDE.md's "hunterd never signs" and its own doc comment — moved to `operator.SignScope`, with `scope.CanonicalBytes`/`scope.Assemble` left behind as the non-signing split; a duplicated port-parsing helper unified into `scope.ParsePort`. go vet, go test -race, ruff, pytest, validate_outlines, validate_skills, compose config all pass after fixes. Not run: `/simplify`. Two low-value findings (enforcerd code outside this diff; cross-schema signing-pipeline duplication) left unfixed as out of scope for this PR |
| 2026-11-21 | Scope gate (I1-I5, SHR-011) built in `hunterd/scope`, on `docs/ml-add-phase7-prep` | Fleet plan Phase 1. `scope.Parse` verifies an Ed25519-signed bundle (window ≤24h, epoch, non-empty targets) before `NewAuthorizer` can build from it; `Authorizer.Check`/`CheckHost` decide allow/deny from parsed CIDRs and anchored hostname regexes itself, never from `networkpolicy.NetworkPolicy`'s own default (an empty CIDR allow-list there means "no restriction", which would invert I4 for a domain-only bundle). `TLSRunner`/`HTTPRunner` require a non-nil `Authorizer` and call `CheckHost` before any dial; `operator scope sign`/`scope verify` added; `keygen`'s silent-overwrite bug fixed (SHR-010). Not done: wiring a bundle into `hunterd/cmd/hunterd serve` (fleet plan Phase 4), HC-2 (scope key custody, blocks using this for a real engagement), and E03's packet-capture layer |
| 2026-11-21 | Phase 7 scaffolded on `docs/ml-add-phase7-prep`, no implementation | All five deferred-consumer gates (K10, K5, K3, K2, K14) checked against the tree and found unmet; see [../../../plans/ml-add/Phase-7.md](../../../plans/ml-add/Phase-7.md) Gate status. Phase 6's own prerequisites are also incomplete: SHR-003 open, no non-synthetic evidence. Nearest unblocking work is the scope gate (plan item 1) |
| 2026-11-20 | Fleet command path: the agent talks to `fleetd` over MCP; `fleetd` commands hunters with Ed25519-signed, scoped, expiring `Command v1` envelopes on NATS; stdio MCP only for local and lab runs; `hunterd` stays non-MCP | Approved by Sunyata in the project thread. [../../../spec/DECISIONS/0007-fleet-command-path.md](../../../spec/DECISIONS/0007-fleet-command-path.md); plan in [../../../plans/fleet/README.md](../../../plans/fleet/README.md) |
| 2026-11-20 | `MinVersion` cut-over scheduled for 2026-12-20 | ADR 0005 resolved question 2: 30 days after Phase 5 reached `develop` (`0a93dcc`, 2026-11-20 22:17 UTC). Until then `SignedBackend.MinVersion` stays v1 and nothing enforces the evidence binding. Setting it to v2 is a deliberate, recorded change and still needs the trust-boundary human sign-off |
| 2026-11-20 | Phase 5 built: v2 schema, `canonjson`, `propose` ledger record, `operator review -ledger`, `SignedBackend.MinVersion` | ADR 0005. Needs `/security-review` and the trust-boundary human sign-off. Cut-over is 30 days after the merge to `develop`: 2026-12-20 (merged 2026-11-20) |
| 2026-11-20 | SHR-001: Go derives `broad` from the target; `operator.Approve` and `SignedBackend` refuse a disagreeing flag | Phase 5 prerequisite. Tests in `enforcerd/breadth_test.go` and `operator/decision_test.go`. A bare `Enforcer` still trusts `Action.Broad` |
| 2026-11-21 | Four-service layout is authoritative; single-binary MCP design superseded | [../../../spec/DECISIONS/0003-architecture-authority.md](../../../spec/DECISIONS/0003-architecture-authority.md). Code, schemas, CI and the accepted ADRs all implement it |
| 2026-11-21 | Outlines are gated, not merely conventional | 32 of 35 project markdown files had dead links and nothing could detect it. See [../../outline-method.md](../../outline-method.md) |
| 2026-11-21 | Language idioms split into `.agents/go.md` and `.agents/python-langgraph.md` | One file per language, grounded in the implemented code |
| 2026-11-21 | Orchestrator-side skills live in `.claude/skills/` | `validate_skills.py` caps `.agents/skills/` at five and it is full |
| 2026-11-20 | All safety bounds enforced in Go only | LLM output is untrusted. See [../invariants.md](../invariants.md) I7. Aspiration today, not implemented |
| 2026-11-20 | Only Opus 5.5, Sonnet 5.5, Haiku 5.5 used | See [../models.md](../models.md) |
| 2026-11-20 | Integration gate: `feature/fleet-p0-adr` (`239ee97`) merged to `develop` | Docs and ADR 0007 only (accepted, approver recorded). go vet, go test -race, gofmt, ruff, pytest, validate_outlines, validate_skills, compose config all pass. No code, so code and security review found nothing in it |
| 2026-11-20 | Integration gate: `claude/bold-mccarthy-bsf51t` (`f335d49`, ADR 0005 Phase 5 and SHR-001) **returned, not merged** | Mechanical checks and `/security-review` pass (no finding at confidence 8 or above; a Medium, confidence 7: the branch's operator evidence printer shows `Target`, `Action` and `Scorer` on the proposal line without `neutralise`). `/code-review` medium found six important bugs: `citedEvidence` does not bind `kind` (or `broad`) to the record; Go and Python classify IPv4-mapped ranges differently; Python stores raw URL or host:port targets that Go rejects; `VerifyEvidence` accepts proposals already rejected; a stale `proposal` in `GraphState` leaks `proposal_seq` into later runs; `operator review` loads the policy before checking the decision, so `-decision reject` fails outside the repo root. Fix with tests first, then gate again |
| 2026-11-20 | Integration gate: `claude/bold-mccarthy-bsf51t` merged to `develop` after fixes on `claude/blissful-wright-eirgxe` (`eb1bc43`, `41a0665`) | All six code-review findings and the Medium security finding fixed, tests first. A second `/code-review` medium pass on the fixes found three regressions (stale `proposal_seq` on propose records, trailing-newline hostname, YAML `#` comments), all fixed. go vet, go test -race, gofmt, ruff, pytest, validate_outlines, validate_skills, compose config pass. Not run: `/simplify`, and no third review of `41a0665`. Still open: the Phase 5 trust-boundary human sign-off, and `MinVersion` stays v1 until the cut-over |
| 2026-11-20 | ml-add Phase 6 blocked; owner chose to repair the enforcer prerequisites first | Checked in code: SHR-001 met; SHR-002, -003, -004, -005, -009 open; no non-synthetic events exist. SHR-002, -004, -005, -009 repaired on `claude/blissful-wright-eirgxe`, tests first. SHR-003 needs a store ADR: I12 names Badger, I17 reserves Badger for `hunterd` |
| 2026-11-20 | Integration gate: enforcer prerequisites (`89f9dad`, `1f84ae6`) merged to `develop` | go vet, go test -race (lifecycle tests repeated 30x), gofmt, ruff, pytest, validate_outlines, validate_skills pass. `/code-review` found 2 important issues (revoke under the lock; replay inside the skew window) and 3 nits, all fixed with tests first. `/security-review`: no finding. Not run: `/simplify`. SHR-002, -004, -005, -009 closed in code; the JSONL review status is for the reviewer to change. SHR-003 still open |

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
| 2026-11-20 | `hunterd/store/badger_store.go` | `badger.DefaultOptions` peaked at about 720 MB RSS in a 200k-write test; a small VM would run out of memory | Open; fleet plan phase 2 (tuned options, `GOMEMLIMIT`) |
| 2026-11-21 | `hunterd/probes/` | Both runners dial immediately; no scope check anywhere (I1-I5) | Fixed 2026-11-21, `hunterd/scope`; see plan item 1 |
| 2026-11-21 | `operator/cmd/operator/main.go` | SHR-010: `keygen` silently overwrote an existing private key | Fixed 2026-11-21, `writeNewFile` (`os.O_EXCL`), `TestKeygenRefusesToOverwriteExistingKey` |
| 2026-11-21 | `hunterd/probes/http_runner.go` | SHR-017: `OnResult` wrote `result` without synchronizing with the reader; `copy` shadowed the builtin | Fixed 2026-11-21, mutex-guarded read/write, variable renamed |
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

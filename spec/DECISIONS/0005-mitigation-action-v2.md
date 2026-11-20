# ADR 0005: Mitigation action v2, a signature that commits to evidence

- Status: proposed
- Date: 2026-11-20
- Approved by: nobody yet. This ADR changes signed bytes at a trust boundary, so `plans/ml-add/Phase-5.md` requires a named human sign-off before it is accepted. Record the reviewer here.

## Context

An operator's Ed25519 signature (`enforcerd.canonicalAction`) covers what to block, for how long and who approved. It does not cover *why*. The evidence the operator was shown is not bound to the approval, so a later change to the story behind an action is undetectable. Phase 2 built an append-only, hash-chained ledger (`orchestrator/ledger.py`). Phase 5 binds an approval to one ledger record.

Three facts in the code shape the design:

1. **No record exists at review time.** `make_record_node` runs after `approval` (`orchestrator/graph.py`) and stores the post-decision `policy`. The interrupt payload from `approve_policy` therefore has nothing to cite.
2. **Ledger faults are swallowed by design** (Phase 2: a ledger fault must not alter or block a decision). A v2 approval cannot tolerate that.
3. **The ledger is Python-owned SQLite** (ADR 0004 decision 4). The operator is Go. Hashing is `sha256(canonical_json(body))` with Python's `json.dumps(sort_keys, separators, ensure_ascii=False)`.

## Decisions

1. **v2 schema, v1 frozen.** `schemas/mitigation_action.v2.json` (planned) adds required `evidence_digest` (64 lowercase hex, the `record_hash` of the cited record) and `ledger_seq` (integer, at least 0). `schema_version` is `"v2"`. `mitigation_action.v1.json` is not edited.
2. **The signed bytes include the version and both new fields.** `canonicalAction` branches on version. The v2 form is the v1 field list in the same order, then `evidence_digest` and `ledger_seq`. Because `schema_version` is signed, a v1 signature cannot verify as v2 or the reverse. This is tested in both directions.
3. **The proposal is recorded before it is approved.** A `propose` ledger record is appended before the `approval` node, and the interrupt payload carries its `ledger_seq` and `record_hash`. The existing post-decision record stays, and cites the proposal's seq. For a `mitigate` decision only, a failed append is **fatal to the proposal**: the decision degrades to `observe` with a reason and nothing signable is produced. Observe-only paths keep Phase 2's fail-open behaviour.
4. **The operator checks, then signs.** `operator review -ledger PATH` (planned) reads a read-only NDJSON export of the ledger (`python -m orchestrator.ledger export`, planned), recomputes every hash from record 0 through `ledger_seq`, confirms the cited record's digest and refuses to sign on any mismatch or break in the chain. It prints the evidence lines (indicators, provenance class, scorer id, confidence), not only `{id, kind, target, broad}`. Go does not open SQLite.
5. **One canonical form across languages, proven by vectors.** Go gets a restricted canonical-JSON writer (sorted keys, no whitespace, no HTML escaping, shortest round-trip floats). A golden file under `schemas/` (planned) holds v2 actions and real ledger records from `orchestrator/tests/fixtures/ledger/`. Go and Python tests both assert the same bytes and the same digest. Include a sub-second timestamp, a non-ASCII string and an exponent-form float, because those are where the two encoders differ.
6. **Compatibility window (recommended).** The enforcer accepts v1 and v2 during a window, selected by an enforcer option `MinVersion` (planned), default v1. Any v1 approval carries no evidence binding, so the guarantee holds only after `MinVersion` is set to v2. The operator CLI issues v2 whenever `-ledger` is given and v1 otherwise, with a warning. The cut-over date is the owner's decision (see open questions). Rollback is turning off v2 issuance in the CLI. No enforcer rollback is needed.
7. **No model output enters the signed bytes** beyond the ledger digest. `broad`, `kind`, TTL, scope, approval and expiry stay deterministic. SHR-001 (Go derives `broad`) landed before this ADR and is a hard prerequisite, already met.

## Consequences

- A signature now proves the operator approved *this* evidence chain, not only this target. Editing or removing any earlier ledger record breaks every later digest.
- The ledger becomes load-bearing for approvals. A deleted or truncated ledger blocks v2 approvals and does not block reaping existing rules.
- It adds a second canonicalizer (Go). Divergence from Python would make every v2 action unreviewable, which fails closed. The golden vectors are the control.
- The ledger head is not anchored anywhere external, so someone who rewrites the whole file and the export consistently is not detected. This is outside scope (Phase 5 Out) and stated here so the guarantee is not overread.
- Residual: a bare `Enforcer` skips signature checks (SHR-009). v2 changes nothing for a caller that never goes through `SignedBackend`.

## Open questions for the owner

1. **Window or hard cut?** Recommended: dual-accept with `MinVersion`, as in decision 6. The alternative is v2 only from day one, which breaks every v1 fixture and any operator tooling at once.
2. **Cut-over date** for `MinVersion = v2`. Not set here. Pick it at the Phase 5 checkpoint.
3. **Is failing a proposal on a ledger fault acceptable** (decision 3)? It is the safe direction, and a ledger outage then stops new mitigation proposals.
4. **Canonicalization**: Go re-canonicalization with golden vectors (recommended), or have the Python export emit the exact hash-input bytes and let Go only hash them. The second is simpler, but it makes the operator trust the exporter for what was hashed.

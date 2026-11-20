**Status:** authoritative · **Derived from:** `enforcerd/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Go enforcement library; there is no `main` package yet. `Enforcer` applies an `Action` through a `SignedBackend`, tracks deadlines, and revokes on `Expire`/`Run`. `NewEnforcer` takes a `SignedBackend` and refuses one with no backend, no valid Ed25519 key, no breadth thresholds or an unknown `MinVersion` (SHR-009), so the standard path cannot install an unsigned action. `SignedBackend` in `enforcerd/verify_signature.go` verifies an Ed25519 signature before delegating to `enforcerd/nft` (nftables set) or `enforcerd/rpz` (DNS RPZ update). `Headscale` is a valid kind with no adapter.

**Boundaries**

- Fail closed: `SignedBackend.Apply` refuses a missing, malformed or invalid signature, and a broad action without `Approved` and a named `ApprovedBy` (I14). `SignedBackend.Apply` also refuses an action past its deadline (`ErrActionExpired`) or created more than `MaxClockSkew` ahead of its clock (SHR-002); `Now` pins that clock in tests. `SignedBackend.Breadth` is required: `broad` is derived from the target (`Breadth.Derive`, thresholds parsed from `orchestrator/policy.yaml`) and a disagreeing flag fails with `ErrBroadMismatch` (SHR-001).
- v2 actions (`Action.Version = "v2"`) sign `EvidenceDigest` and `LedgerSeq` through `canonjson` (ADR 0005). `SignedBackend.MinVersion` is `""`/`"v1"` during the compatibility window and `"v2"` at the cut-over; any other value fails closed. A v1 action must carry no evidence fields.
- Any change to an approved payload invalidates its signature; the canonical form covers approval, TTL and expiry. Do not add unsigned fields that alter behavior.
- TTL is mandatory and at most 24 hours (`MaxTTL`, I11). No permanent rules.
- Failed revocation stays tracked and retryable; never drop the entry on error (I13). `Expire` tries every due action and joins the errors; `Run` reaps once at start, logs when the set of failing IDs changes and keeps going (SHR-004). Revokes run outside the lock, so a slow backend never blocks `Enforce`. The tracked deadline (`Receipt.ExpiresAt`, authoritative) is capped at now+TTL (SHR-005), and a revoked ID is refused until its signed deadline so it cannot be replayed in between.
- Run commands with `exec.CommandContext` argument arrays; validate targets before execution.
- Gap: pending actions live in memory only. I12 (persist before install) and I13 (reaper after a crash) are not met (SHR-003), and a restart forgets which IDs were already enforced, so an unexpired action can be replayed after one. Fixing it needs a store decision: I12 names Badger, and I17 gives Badger to `hunterd` alone.
- Never produce approvals; that is `operator/`.

**Rules that apply**

- [../.agents/go.md](../.agents/go.md), [../.agents/orchestration/language-idioms.md](../.agents/orchestration/language-idioms.md), [../.agents/testing.md](../.agents/testing.md); invariants I7, I11-I14.
- Skill: `.agents/skills/mitigation-guardian`. Schema: `schemas/mitigation_action.v1.json`.

**Verify**

```bash
gofmt -l enforcerd
go vet ./enforcerd/...
go test -race ./enforcerd/...
```

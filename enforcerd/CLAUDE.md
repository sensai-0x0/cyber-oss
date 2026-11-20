**Status:** authoritative · **Derived from:** `enforcerd/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Go enforcement library; there is no `main` package yet. `Enforcer` applies an `Action` through a `Backend`, tracks deadlines, and revokes on `Expire`/`Run`. `SignedBackend` in `enforcerd/verify_signature.go` verifies an Ed25519 signature before delegating to `enforcerd/nft` (nftables set) or `enforcerd/rpz` (DNS RPZ update). `Headscale` is a valid kind with no adapter.

**Boundaries**

- Fail closed: `SignedBackend.Apply` refuses a missing, malformed or invalid signature, and a broad action without `Approved` and a named `ApprovedBy` (I14). Wire `SignedBackend` around every real backend; a bare `Enforcer` does not check signatures.
- Any change to an approved payload invalidates its signature; the canonical form covers approval, TTL and expiry. Do not add unsigned fields that alter behavior.
- TTL is mandatory and at most 24 hours (`MaxTTL`, I11). No permanent rules.
- Failed revocation stays tracked and retryable; never drop the entry on error (I13).
- Run commands with `exec.CommandContext` argument arrays; validate targets before execution.
- Gap: pending actions live in memory only. I12 (persist before install) and I13 (reaper after a crash) are not met.
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

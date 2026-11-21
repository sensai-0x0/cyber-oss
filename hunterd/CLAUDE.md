**Status:** authoritative · **Derived from:** `hunterd/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Collection daemon. `hunterd/cmd/hunterd` is a CLI (`serve`, `version`) with a gRPC health endpoint and a bounded NDJSON delta-event stream. `hunterd/cursor` is the Feistel permutation, `hunterd/store` the Badger cursor and novelty gate, `hunterd/probes` the tlsx/httpx runners, `hunterd/scope` the signed-bundle scope gate (I1-I5, fleet plan Phase 1).

**Scope enforcement (I1-I5) lands in `hunterd/scope`.** `TLSRunner` and `HTTPRunner` both require a non-nil `probes.Authorizer` (satisfied by `*scope.Authorizer`) and check it before any dial (L1). `TLSRunner` wires an always-on `fastdialer.Options.OnValidateTarget` hook (L2) that re-checks the real socket address at connect time, catching DNS rebinding regardless of whether the bundle has a CIDR entry. httpx exposes no equivalent per-dial hook, so for a hostname target `HTTPRunner` resolves and checks every address itself first, then pins the dial to one validated address via httpx's `"host,ip"` input form, so httpx's own internal resolution has nothing left to diverge on. `hunterd/cmd/hunterd` does not yet load a real bundle at startup or wire these runners into `serve` — that lands with the NATS/fleet mode in fleet plan Phase 4. Until then, active probing still belongs in `lab/`; do not add real targets.

**Boundaries**

- Only `novel` and `mutated` events leave the daemon; `validateEvent` rejects other kinds, sources and fingerprints (schema `schemas/delta_event.v1.json`).
- Stream stays bounded; a slow consumer applies backpressure, never unbounded buffering (`-event-capacity`).
- Feistel stays a bijection with O(1) memory and a resumable cursor (I15, I16).
- Only this process opens a Badger directory (I17). Python never reads the store.
- No mitigation, firewall or signing code here; that is `enforcerd/` and `operator/`. `hunterd/scope.Parse` only verifies; the ed25519.Sign call for a scope bundle lives in `operator.SignScope`, which calls `scope.CanonicalBytes`/`scope.Assemble`.
- Logs to stderr; stdout carries NDJSON only.
- `hunterd/scope` decides scope itself; it never delegates the "no CIDR entry matched" default to `networkpolicy.NetworkPolicy`, which treats an allow-list with zero CIDR entries as unrestricted (see the `Authorizer` doc comment). A domain-only bundle (spec/SCOPE.md: "Exact CIDRs/domains") is authorized by its own hostname match, not by an empty CIDR check defaulting open.

**Rules that apply**

- [../.agents/go.md](../.agents/go.md), [../.agents/orchestration/language-idioms.md](../.agents/orchestration/language-idioms.md), [../.agents/testing.md](../.agents/testing.md), [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md) (I1-I6, I15-I17).
- Contract changes start with the `contract-steward` skill: `.agents/skills/contract-steward`.

**Verify**

```bash
gofmt -l hunterd
go vet ./hunterd/...
go test -race ./hunterd/...
```

**Status:** authoritative · **Derived from:** `hunterd/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Collection daemon. `hunterd/cmd/hunterd` is a CLI (`serve`, `version`) with a gRPC health endpoint and a bounded NDJSON delta-event stream. `hunterd/cursor` is the Feistel permutation, `hunterd/store` the Badger cursor and novelty gate, `hunterd/probes` the tlsx/httpx runners.

**Warning: scope enforcement is not implemented.** Both probe runners dial immediately: no allowlist check, no Go file parses configuration or holds a target list. I1-I5 have no enforcement point (see ADR 0003, consequence 4). Active probing belongs in `lab/` until a scope package lands. Do not add real targets.

**Boundaries**

- Only `novel` and `mutated` events leave the daemon; `validateEvent` rejects other kinds, sources and fingerprints (schema `schemas/delta_event.v1.json`).
- Stream stays bounded; a slow consumer applies backpressure, never unbounded buffering (`-event-capacity`).
- Feistel stays a bijection with O(1) memory and a resumable cursor (I15, I16).
- Only this process opens a Badger directory (I17). Python never reads the store.
- No mitigation, firewall or signing code here; that is `enforcerd/` and `operator/`.
- Logs to stderr; stdout carries NDJSON only.

**Rules that apply**

- [../.agents/go.md](../.agents/go.md), [../.agents/orchestration/language-idioms.md](../.agents/orchestration/language-idioms.md), [../.agents/testing.md](../.agents/testing.md), [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md) (I1-I6, I15-I17).
- Contract changes start with the `contract-steward` skill: `.agents/skills/contract-steward`.

**Verify**

```bash
gofmt -l hunterd
go vet ./hunterd/...
go test -race ./hunterd/...
```

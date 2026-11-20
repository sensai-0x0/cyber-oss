**Status:** authoritative · **Derived from:** `operator/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Approval side of the mitigation boundary. `Approve` and `Reject` in `operator/decision.go` turn a named decision into a signed (or unsigned, rejected) `enforcerd.Action`. `operator/cmd/operator` is the CLI: `keygen` writes an Ed25519 pair; `review` reads an action as JSON on stdin and writes the decision to stdout.

**Boundaries**

- Never touches firewall, DNS or any backend state; it imports `enforcerd` types and signing only (I7).
- A decision requires a non-empty operator name. Approval requires `0 < ttl <= 24h` (I11); `Reject` clears the signature.
- Never sign without a human decision; no auto-approve path, no default `approve`. The `review` default is `reject`.
- Private keys are never committed, logged or printed; keys are written with mode 0600. Keep key paths out of the repo.
- Do not change the canonical signed form here; that lives in `enforcerd/verify_signature.go` and a change needs a new schema version.

**Rules that apply**

- [../.agents/go.md](../.agents/go.md), [../.agents/orchestration/language-idioms.md](../.agents/orchestration/language-idioms.md), [../.agents/testing.md](../.agents/testing.md); invariants I7, I11, I14.
- Skills: `.agents/skills/hunt-operator`, `.agents/skills/mitigation-guardian`.

**Verify**

```bash
gofmt -l operator
go vet ./operator/...
go test -race ./operator/... ./enforcerd/...
```

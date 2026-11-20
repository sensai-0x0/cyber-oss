**Status:** authoritative · **Derived from:** `operator/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Approval side of the mitigation boundary. `Approve` and `Reject` in `operator/decision.go` turn a named decision into a signed (or unsigned, rejected) `enforcerd.Action`. `operator/cmd/operator` is the CLI: `keygen` writes an Ed25519 pair; `review` reads an action as JSON on stdin and writes the decision to stdout; `scope sign`/`scope verify` sign and check a `hunterd/scope.Bundle` (fleet plan Phase 1), using a separate key from the mitigation key by design (separation of duties — pass a different `-out-prefix` to `keygen` for it).

**Boundaries**

- Never touches firewall, DNS or any backend state; it imports `enforcerd` and `hunterd/scope` types and signing only (I7).
- A decision requires a non-empty operator name. Approval requires `0 < ttl <= 24h` (I11); `Reject` clears the signature.
- `Approve` takes an `enforcerd.Breadth` and refuses to sign when the claimed `broad` disagrees with the target (SHR-001). The CLI reads thresholds from `-policy` (default `orchestrator/policy.yaml`) on approve only; reject needs no policy file.
- `-ledger EXPORT` verifies the hash chain over the whole export (`VerifyEvidence`), refuses a cited record whose id, kind, target or `broad` differ from the action or that a later decision record already closed, prints the evidence to stderr and signs v2 (`ApproveWithEvidence`). A citation without `-ledger`, or `-ledger` without a citation, is refused, never dropped. Without either the CLI signs v1 and warns that it carries no evidence binding.
- Never sign without a human decision; no auto-approve path, no default `approve`. The `review` default is `reject`.
- `keygen` refuses to overwrite an existing key file (`writeNewFile`, `os.O_EXCL`; SHR-010). `scope sign` and `scope verify` read keys, never write one.
- Private keys are never committed, logged or printed; keys are written with mode 0600. Keep key paths out of the repo.
- Do not change the canonical signed form here; that lives in `enforcerd/verify_signature.go` (actions) or `hunterd/scope/bundle.go` (scope bundles), and a change needs a new schema version.
- **HC-2 is open** (plans/fleet/README.md): who holds the scope signing key and its backup, and the signing ceremony, are not yet recorded. `scope sign` signs whatever key path it is given; it enforces no custody policy itself.

**Rules that apply**

- [../.agents/go.md](../.agents/go.md), [../.agents/orchestration/language-idioms.md](../.agents/orchestration/language-idioms.md), [../.agents/testing.md](../.agents/testing.md); invariants I7, I11, I14.
- Skills: `.agents/skills/hunt-operator`, `.agents/skills/mitigation-guardian`.

**Verify**

```bash
gofmt -l operator
go vet ./operator/...
go test -race ./operator/... ./enforcerd/...
```

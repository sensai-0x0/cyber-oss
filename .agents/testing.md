# Verification and Acceptance

**Status:** authoritative · **Derived from:** [../.github/workflows/ci.yml](../.github/workflows/ci.yml), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md). A task is done only when its suite passes. Record failures in [state-log.md](orchestration/obs/state-log.md).

Suites 2 and 3 were retargeted by ADR 0003: the MCP-tool surface and the `agent/` (absent) package
they named were never built. The boundaries they tested exist, under different names.

| # | Suite | Command | Must assert |
|---|---|---|---|
| 1 | Cursor bijection + delta gate | `go test -race ./hunterd/...` | `/16` permuter visits all 65,536 IPs, zero duplicates, resumes from a saved cursor. `CheckAndUpdateDelta`: 1 event on pass 1, 0 on pass 2, against a local mock TLS/HTTP server. |
| 2 | Transport and enforcement boundary | `go test -race ./hunterd/cmd/... ./enforcerd/...` | `validateEvent` rejects a wrong schema version, an unknown kind or source, a future timestamp and a malformed fingerprint. The bounded queue applies backpressure rather than dropping. Signature verification fails closed. TTL outside its bounds is refused, and no action is permanent. |
| 3 | Triage routing and human gate | `pytest orchestrator/tests` | A low score produces `observe`. A narrow high-confidence action does not interrupt. A broad action halts at `interrupt()` and resumes correctly on approve, edit and reject. Refinement is bounded by `policy.yaml: max_refinement_cycles`. |

## Additional required tests

- Odd prefix widths (e.g. `/23`, `/31`, `/32`) for the permuter.
- Cursor survives restart: kill after N probes, resume, no rescans or skips. Nothing
  currently calls `SaveCursor`, so this is unwired today (I16).
- `testing.AllocsPerRun` holds the no-per-IP-allocation claim for the permuter.
- Reaper removes expired rules after a simulated crash (I12, I13). The enforcer keeps
  pending actions in memory and never persists before install, so this cannot pass yet.
- Rate limit holds under concurrent probes (I6). No limiter exists yet.
- Scope: an off-scope target is refused before any dial, and a redirect off-scope is not
  followed (I1-I5, I2). **No enforcement point exists** — see ADR 0003 consequence 4.
  Until it lands these assertions have nothing to target, and the gap must be reported as
  absent rather than passing.

## Rules

- Tests never touch real networks. Use `httptest` / local listeners on loopback only.
- Fakes for DNS, nftables, NATS, probes and clock time. Never a live firewall or a live
  model as build verification.
- Never relax a scope or TTL check to make a test pass. Add the loopback target to the
  test's own fixture instead.
- On failure, re-run only the failing tests while fixing.

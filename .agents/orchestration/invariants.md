# Invariants and Security Rules

**Status:** authoritative · **Derived from:** safety requirements in [../../spec/SCOPE.md](../../spec/SCOPE.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../../CLAUDE.md). These are non-negotiable. A change that weakens one is a bug, even if tests pass.

## Scope (authorized engagement)

- **I1.** Every probe target is checked against `mission.yaml:targets` in Go **before** dialing. Out of scope returns `ERR_OUT_OF_SCOPE`; no packet is sent.
- **I2.** Hostnames are resolved, then **every** resolved IP is checked. One out-of-scope IP rejects the target. Re-check after redirects; never follow a redirect off-scope.
- **I3.** `scan_cidr_delta` input must be a subset of a `targets` entry, not merely overlapping.
- **I4.** Empty or unparseable `targets` means refuse to start. Never default to "allow all".
- **I5.** Scope must cover the client's written authorization. Do not add ranges the client has not authorized.

## Rate

- **I6.** One global token bucket at `rate_limit_pps` (default 100), shared by tlsx and httpx. Exceeding it returns `ERR_RATE_LIMIT`, it does not queue unbounded work.

## Enforcement boundary

- **I7.** All bounds (`max_ttl_minutes`, `min_confidence`, tier membership, CIDR vs single IP) are enforced in `cmd/hunter`. Python checks are a routing convenience only, never the guard.
- **I8.** Treat every LLM-supplied argument as hostile input: validate type, range, and format in Go.
- **I9.** The agent has no shell, no file write, no network outside the 4 MCP tools.
- **I10.** `mission.yaml` is read-only at runtime. No tool may modify it.

## Mitigation

- **I11.** Every mitigation has a TTL: `0 < ttl_minutes <= max_ttl_minutes`. No permanent rules.
- **I12.** `mitigate:rule:<id>` is written to Badger **before** the rule is installed, so the reaper can always remove it after a crash.
- **I13.** The reaper runs on startup and on a timer; it removes expired rules from nftables/RPZ, then deletes the key.
- **I14.** CIDR blocks, wildcard RPZ and ASN pivots always go to `human_gate`. Go rejects them unless approved.

## Data integrity

- **I15.** Feistel permutation is bijective: zero duplicates over `0..Len()-1`.
- **I16.** Cursor persisted every 256 probes and on `SIGINT`/`SIGTERM`. A restart resumes, never rescans from zero.
- **I17.** BadgerDB allows **one process per directory**. Only `hunter` opens the DB; Python reads via `query_kv_history`.

## Secrets and output

- Never log or commit credentials, client names, or raw target lists outside the engagement repo.
- Logs go to stderr. Stdout is reserved for MCP JSON-RPC.

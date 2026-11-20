# State, Plans, Decisions and Errors

Universal rules: [../CLAUDE.md](../../../CLAUDE.md). Read at session start; append at session end. Newest entries first. Dates as YYYY-MM-DD.

## Current state

| Area | Status | Notes |
|---|---|---|
| `pkg/cidr` | Not started | |
| `pkg/kv` | Not started | |
| `pkg/probe` | Not started | |
| `cmd/hunter` | Not started | |
| `agent/` | Not started | |
| Acceptance suites 1-3 | Not started | See [testing.md](../../testing.md) |

## Plan (next steps)

1. `pkg/cidr` + bijection tests.
2. `pkg/kv` + delta gate tests.
3. `pkg/probe` with scope + rate enforcement, mock servers.
4. `cmd/hunter` MCP server + boundary tests.
5. `agent/` graph + autonomy tests.

## Decisions

| Date | Decision | Reason |
|---|---|---|
| 2026-11-20 | Architecture Outline is authoritative over Project Description | Outline is newer and fully specified. See [architecture.md](../architecture.md#spec-divergence-resolved) |
| 2026-11-20 | Single `hunter` stdio binary, no `hunterd` daemon | Per outline; agent spawns it via MCP |
| 2026-11-20 | All safety bounds enforced in Go only | LLM output is untrusted. See [invariants.md](../invariants.md) I7 |
| 2026-11-20 | Only Opus 5.5, Sonnet 5.5, Haiku 5.5 used | See [models.md](../models.md) |

## Open questions

- **CT log streaming:** tlsx probes hosts but likely does not stream Certificate Transparency logs. Needs a separate CT client (e.g. a certstream-style feed) if required. Not in current scope.
- **Odd prefix widths:** confirm cycle-walking approach for odd `w` in Feistel.
- **ASN source:** which ASN lookup feeds `DeltaEvent.asn` and `cluster.asns` (offline DB vs API)?
- **Rule installation:** privilege model for nftables/RPZ writes from `hunter`.

## Error log

| Date | Where | Error | Fix / status |
|---|---|---|---|
| | | | |

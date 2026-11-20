# Verification and Acceptance

Universal rules: [../CLAUDE.md](../CLAUDE.md). A task is done only when its suite passes. Record failures in [state-log.md](state-log.md).

| # | Suite | Command | Must assert |
|---|---|---|---|
| 1 | Go bijection + delta | `go test -race ./...` | `/16` permuter visits all 65,536 IPs, zero duplicates. `CheckAndUpdateDelta`: 1 event on pass 1, 0 on pass 2, against a local mock TLS/HTTP server. |
| 2 | MCP boundary | `go test ./cmd/hunter/...` | `scan_cidr_delta` → `ERR_OUT_OF_SCOPE` off-scope. `apply_ttl_rule` → `ERR_INVALID_TTL` for ttl ≤ 0 or > max; `ERR_OUT_OF_SCOPE` for unapproved CIDR or low confidence. |
| 3 | Agent autonomy | `pytest agent/tests/test_autonomy.py` | Replay `DeltaEvent` JSONL fixtures with a mocked LLM. Tier 1/2 run without blocking. Tier 3 halts at `interrupt()` and resumes on `approve`/`reject`. |

## Additional required tests

- Odd prefix widths (e.g. `/23`, `/31`, `/32`) for the permuter.
- Cursor survives restart: kill after N probes, resume, no rescans or skips.
- Redirect to off-scope host is not followed (invariant I2).
- Reaper removes expired rules after a simulated crash (I12, I13).
- Rate limit holds under concurrent probes.

## Rules

- Tests never touch real networks. Use `httptest` / local listeners on loopback only.
- Loopback must be added to test `mission.yaml` explicitly; never relax scope checks for tests.
- On failure, re-run only the failing tests while fixing.

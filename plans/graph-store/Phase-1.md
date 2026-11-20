# Phase 1: FalkorDB on the lab network

**Status:** proposed · **Derived from:** [README.md](README.md), [../../deploy/compose.yaml](../../deploy/compose.yaml), [../../deploy/CLAUDE.md](../../deploy/CLAUDE.md), [../../db/CONFIG.md](../../db/CONFIG.md) · **Updated:** 2026-11-20

**Depends on:** Phase 0.

## Goal

A pinned FalkorDB service in `deploy/compose.yaml`, reachable only from other containers on one named network. It is healthy, persistent and password protected.

## Scope

1. Add network `graphnet` (planned) to `deploy/compose.yaml` with `internal: true`. FalkorDB and the app join it. Other lab services (`nats`, `fake-c2`, `phish-panel`, `dns`) do not. FalkorDB is on no other network, so it cannot reach the internet.
2. Add service `falkordb` (planned): image pinned by tag and digest from the ADR, no `ports:` entry, a named volume for RDB/AOF, `FALKORDB_THREAD_COUNT` set to the container CPU quota, a query timeout, CPU and memory limits (set here, tuned in Phase 5), a non-root user with the volume owned by it, `restart: unless-stopped`, and a healthcheck.
3. Password from the environment (`FALKORDB_PASSWORD`). Compose fails fast if it is unset: `${FALKORDB_PASSWORD:?set it}`. No default in the file, unlike the NATS lab placeholder.
4. Optional host access for debugging, bound to `127.0.0.1` only, behind a Compose profile (`graph-debug`, planned).
5. Extend `orchestrator/tests/test_lab.py` so it asserts: no `0.0.0.0` publish, no `latest` tag, FalkorDB has no host port outside the profile, and the password has no default.

## Tests first

- The compose config test above, red before the service exists.
- `docker compose config --quiet` passes with the variable set, and **fails** with it unset.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-1.1 | Healthcheck flaps or the container crash-loops under ingest because the default thread count exceeds the CPU quota | `docker compose ps` shows restarts | Set thread count to the quota and cap the app's concurrency (Phase 3) |
| GF-1.2 | Port published on all interfaces | `test_lab.py` | Remove the `ports:` entry or bind `127.0.0.1` |
| GF-1.3 | Data lost on `down -v` or a container rebuild | Phase 5 restore drill | Named volume, documented in the runbook. `down -v` is destructive and is called out |
| GF-1.4 | Image tag moves under the pin | Digest mismatch at pull | Pin by digest and update only through the ADR |
| GF-1.5 | Healthcheck passes before the graph accepts queries | App fails on first call | Healthcheck runs a real `PING` with auth, not a port check |
| GF-1.6 | Password appears in `docker inspect` or logs | Review of compose env and logs | Accept for the lab, document it, and use Docker secrets outside the lab |
| GF-1.7 | Memory growth exhausts the host | `docker stats` | Container memory limit set in this phase. A test asserts it exists. Phase 5 tunes the value |
| GF-1.8 | The non-root user cannot write the named volume | Container exits at start with a permission error | Create the volume with the right owner in an init step. Do not run as root to work around it |
| GF-1.9 | `internal: true` blocks the host-side debug port | Debug profile cannot connect | The debug profile joins a second, non-internal network for that one service. It stays bound to `127.0.0.1` |

## Acceptance

- [ ] `docker compose -f deploy/compose.yaml config --quiet` passes (with the variable set).
- [ ] `uv run pytest orchestrator/tests/test_lab.py` passes with the new assertions.
- [ ] `test_lab.py` asserts `graphnet` is internal and FalkorDB is on no other network.
- [ ] `docker compose up -d falkordb` becomes healthy, survives `restart`, and keeps data across it.
- [ ] Any failure seen is logged to `.agents/orchestration/obs/errors.md`.

## Rollback

Remove the service, the network and the volume. Nothing else depends on them yet.

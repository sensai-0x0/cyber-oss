# Phase 2: App container on the shared network

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-1.md](Phase-1.md), [../../orchestrator/README.md](../../orchestrator/README.md), [../../deploy/CLAUDE.md](../../deploy/CLAUDE.md) · **Updated:** 2026-11-20

**Depends on:** Phase 1.

## Goal

The orchestrator runs in a container on `graphnet`, can reach `falkordb` by service name, and starts only when the graph is healthy or explicitly runs without it.

## Scope

1. `orchestrator/Dockerfile` (planned): Python 3.12, `uv sync --frozen`, non-root user, read-only root filesystem with a tmpfs and one writable data volume. `deploy/CLAUDE.md` currently says no Go or Python services are built; update that line in the same commit.
2. Compose service `orchestrator` (planned) on `graphnet`, `depends_on: falkordb: condition: service_healthy`. The graph is optional at runtime (Phase 3), so this ordering is for the smoke test and not a runtime requirement.
3. Environment: `FALKORDB_HOST=falkordb`, port, password and `GRAPH_STORE_ENABLED` (default `false`). Never `127.0.0.1` inside the container, since that is the container itself.
4. A connectivity check entry point (`python -m orchestrator.graphcheck`, planned) that connects, authenticates and exits non-zero on failure.
5. The SQLite checkpoint path ([../../orchestrator/graph.py](../../orchestrator/graph.py)) points at a mounted volume so interrupts survive a container restart.

## Tests first

- Compose test: `orchestrator` shares exactly one network with `falkordb`, publishes no port, and is not on the other lab services' network.
- `graphcheck` returns non-zero for a wrong host, a wrong password and a timeout, using a fake server.
- With `GRAPH_STORE_ENABLED=false` the container never opens a connection (assert by closing the port).

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-2.1 | App starts before FalkorDB accepts connections | `graphcheck` fails on boot | Retry with bounded backoff, then start with the graph disabled and log the reason |
| GF-2.2 | Service-name DNS fails on a custom network | `graphcheck` reports resolution error | Check both services list the same network. Do not fall back to a published host port |
| GF-2.3 | App uses `localhost` and silently talks to nothing | `graphcheck` | Config validation rejects loopback hosts when running in the container |
| GF-2.4 | The container's writable volume is lost, so checkpoints are lost and a pending approval disappears | Restart drill | Mount the volume, and test interrupt then restart then resume |
| GF-2.5 | Read-only root breaks a library that writes at import | Container exits at start | Add a specific tmpfs path. Do not drop `read_only` |
| GF-2.6 | Running as root, or the image carries the `.env` | Image scan, `docker history` | Non-root user, `.dockerignore` for `.env` and keys |
| GF-2.7 | Two app replicas write the same checkpoint file | Corruption or lock errors | Run one replica. Document it. SQLite is not a multi-writer store |
| GF-2.8 | Time skew between containers makes ledger and graph timestamps disagree | Phase 4 ordering test | Use host time sources only, and compare by sequence number, not clock |

## Acceptance

- [ ] `docker compose up` brings both services up. `graphcheck` exits 0.
- [ ] Interrupt, restart the container, resume: the approval is still pending.
- [ ] The orchestrator tests pass inside the image.
- [ ] Failures logged to `errors.md`.

## Rollback

Stop the `orchestrator` service. Running from source, as today, is unchanged.

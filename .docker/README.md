# .docker

**Status:** proposed · **Derived from:** [../plans/graph-store/Phase-1.md](../plans/graph-store/Phase-1.md), [../deploy/CLAUDE.md](../deploy/CLAUDE.md) · **Updated:** 2026-11-20

Dockerfiles for services that Compose builds. `deploy/compose.yaml` is the only place that runs them, and nothing here is wired in yet.

| Path | Purpose |
|---|---|
| `falkordb/Dockerfile` | Upstream FalkorDB pinned by tag and digest, with a data volume and an authenticated healthcheck. No credentials |

Not yet verified by a build: the author's session had no Docker daemon, and the registry rate-limited metadata lookups. Before wiring it into Compose, run `docker build -f .docker/falkordb/Dockerfile .docker/falkordb`, then confirm the upstream image's user and entrypoint (Phase 1, GF-1.8) and that the healthcheck passes with the password set. Phase 1 adds the Compose service and the `test_lab.py` assertions.

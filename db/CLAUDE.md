**Status:** authoritative · **Derived from:** `db/`, [INSTALL.md](INSTALL.md), [CONFIG.md](CONFIG.md), [DATA_MODEL.md](DATA_MODEL.md), [../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md](../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** A Graphiti + FalkorDB temporal knowledge graph that lets agents orchestrating work on this repo remember what a prior session did: which `WorkBlock` ran where, what `FailurePattern` it hit, and what depended on what. It is bookkeeping for the development process, not a runtime component of hunting, triage, or mitigation — see [ADR 0004](../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md) for why `db/INSTALL.md`'s and `db/CONFIG.md`'s literal `agent/graph.py` / `cmd/hunter` references do not apply here.

**Entities**

The ontology is fixed in [config.yaml](config.yaml); extend it only there, plus a recorded decision, never by inventing a type ad hoc in code or in a prompt.

| Kind | Name | Carries |
|---|---|---|
| Entity | `WorkBlock` | An isolated unit of work executed in a git worktree |
| Entity | `Worktree` | Git worktree path and branch name |
| Entity | `FailurePattern` | A recurring build, race, or test failure an agent hit |
| Edge | `DEPENDS_ON` | Target `WorkBlock` requires source `WorkBlock` to pass first |
| Edge | `ENCOUNTERED_FAILURE` | A `WorkBlock` hit a specific `FailurePattern` during verification |

**How an agent manages these entities**

1. Read before writing. Before starting a `WorkBlock`, query the graph (`db/graph_client.search_graph`) scoped to `GRAPHITI_GROUP_ID` for upstream `DEPENDS_ON` edges and any `FailurePattern` already linked to the package being touched. Inject exact past fixes into the next step's context instead of re-deriving them.
2. Write only through `db/graph_client.py`. Build one structured JSON payload per episode (`WORK_BLOCK_COMPLETED` or `WORK_BLOCK_BLOCKED`, field shapes in [DATA_MODEL.md](DATA_MODEL.md) §3) and call `add_work_block_episode`. Never hand-write Cypher against FalkorDB and never call `graphiti_core` directly from another module — this file is the only caller.
3. Audit before (or instead of) the graph write: call `db/audit.record_episode` with the same `payload_json` first, then `add_work_block_episode`, then `db/audit.mark_graph_write_status`. Graphiti's extraction is lossy and LLM-driven ([DATA_MODEL.md](DATA_MODEL.md) §5); the audit row is what makes a bad extraction replayable instead of lost. A graph write that raises leaves the audit row `pending`, which is the intended failure mode — `pending_episodes()` finds it later.
4. One episode per commit or blocker, immediately after it happens, not batched at the end of a session.
5. Never invent a new entity or edge type to avoid writing a new episode shape; add it to `config.yaml` first.

**Boundaries**

- Loopback only. `load_graph_config` refuses any `FALKORDB_HOST` that is not `127.0.0.1` or `localhost` — Graphiti's reference MCP/REST and FalkorDB itself ship with no authentication (`db/CONFIG.md` risk register).
- Same `group_id` on every read and write (`GRAPHITI_GROUP_ID=sentinel_lite_main`). FalkorDB keys each `group_id` as a separate graph database; a mismatched group_id reads back nothing, silently.
- JSON episodes only (`EpisodeType.json`). Raw, unstructured text (banners, logs) is never ingested directly — it reaches the extraction LLM, which makes it a prompt-injection surface. `build_episode_body` truncates every string field before serialization; do not bypass it.
- Go components (`hunterd`, `enforcerd`, `operator`) never open a FalkorDB connection themselves (`db/INSTALL.md` §1.3). A Go-side caller that needs to log an episode shells into a small Python entry point built on `db/graph_client.py`, or hands the payload to whichever Python agent is already running.
- Pin `graphiti-core>=0.30.2` — earlier versions have the fulltext-scan and multi-group regressions `db/CONFIG.md` documents (getzep/graphiti #1592, #1161).
- This component has no relationship to `schemas/`, `DeltaEvent`, `MitigationAction`, or any safety invariant in `.agents/orchestration/invariants.md`. Do not wire it into `orchestrator/graph.py`'s triage nodes; it is a separate, optional memory layer a session can consult, not a dependency of the hunting pipeline.

**Verify**

```bash
uv sync --frozen --python 3.12 --group graphdb
uv run ruff check db
uv run pytest db/tests
docker compose -f db/compose.yaml --env-file ../.env config --quiet
docker compose -f db/compose.yaml --env-file ../.env up -d
docker compose -f db/compose.yaml exec falkordb redis-cli ping
```

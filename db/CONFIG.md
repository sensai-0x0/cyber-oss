# Graph DB Configuration, Tuning & Risk Spec (`CONFIG.md`)

This specification governs the [getzep/graphiti](https://github.com/getzep/graphiti) + [FalkorDB](https://github.com/FalkorDB/FalkorDB) graph database layer initialized in `./INSTALL.md`.

---

## 1. Environment Variables & Tuning (`.env`)

| Variable | Recommended Value | Technical Rationale |
|---|---|---|
| `GRAPHITI_TELEMETRY_ENABLED` | `false` | Disables anonymous PostHog telemetry for air-gapped or security-sensitive labs. |
| `DATABASE_PROVIDER` | `falkordb` | Default backend for Graphiti MCP v1.0+. Requires `graphiti-core >= 0.30.2`. |
| `FALKORDB_HOST` / `PORT` | `127.0.0.1` / `6379` | Bind strictly to loopback so the graph store is unreachable from external networks. |
| `FALKORDB_THREAD_COUNT` | Match container vCPUs (e.g., `2`) | Prevents default high thread counts (`20`) from starving container CPU & `redis-cli ping` healthchecks. |
| `FALKORDB_TIMEOUT_MS` | `5000` | Bounds runaway Cypher scans cleanly instead of hanging orchestrator coroutines. |
| `GRAPHITI_MAX_COROUTINES` | `4` | Caps concurrent LLM extraction calls during burst episode ingestion. |
| `MCP_HOST` / `MCP_PORT` | `127.0.0.1` / `8000` | **Critical:** Graphiti's reference MCP/REST servers have no built-in auth. Never bind to `0.0.0.0`. |
| `GRAPHITI_GROUP_ID` | `sentinel_lite_main` | Explicit namespace. On FalkorDB, each `group_id` maps to a separate graph database. |
| `LLM_TEMPERATURE` | `0.0` | Keep deterministic (`0.0`) for repeatable entity/dependency extraction. |

---

## 2. Prescribed Ontology (`config/config.yaml`)
Define custom `entity_types`, `edge_types`, and `edge_type_map` under the `graphiti` section of `config/config.yaml` so Graphiti extracts structured workflow dependencies rather than generic prose nodes:

```yaml
graphiti:
  group_id: sentinel_lite_main
  entity_types:
    WorkBlock:
      description: "An isolated PRD work block executed in a git worktree"
    Worktree:
      description: "Git worktree path and branch name"
    FailurePattern:
      description: "A recurring build, race, or test failure encountered by an agent"
  edge_types:
    DEPENDS_ON:
      description: "Target WorkBlock requires source WorkBlock to pass before starting"
    ENCOUNTERED_FAILURE:
      description: "WorkBlock hit a specific FailurePattern during verification"
  edge_type_map:
    WorkBlock:
      WorkBlock: ["DEPENDS_ON"]
      FailurePattern: ["ENCOUNTERED_FAILURE"]
```

---

## 3. Technical Risks & Mitigations

| Risk | Severity | Root Cause | Concrete Mitigation |
|---|---|---|---|
| **`edge_fulltext_search` Query Timeouts** | High | Pre-`v0.30.2` Graphiti emits redundant `:Entity` endpoint labels on FalkorDB, causing `O(hits × entities)` scans ([Issue #1592](https://github.com/getzep/graphiti/issues/1592)). | Pin `graphiti-core >= 0.30.2` (`startNode()`/`endNode()` fix) and alert if p95 query latency exceeds `15 ms`. |
| **Single `group_id` Empty Search Results** | High | FalkorDB stores each `group_id` as a separate graph; earlier `handle_multiple_group_ids` skipped cloning when `len(group_ids) == 1` ([Issue #1161](https://github.com/getzep/graphiti/issues/1161)). | Pin `graphiti-core >= 0.30.2`, pass identical `group_id` on every read/write, and assert non-empty retrieval in Phase 4 smoke tests. |
| **Container CPU Starvation & Crash-Loops** | Medium | Default FalkorDB `THREAD_COUNT=20` on 2-vCPU containers starves `redis-cli ping` healthchecks during ingest bursts ([Issue #1592](https://github.com/getzep/graphiti/issues/1592)). | Set `FALKORDB_THREAD_COUNT` equal to vCPU quota and throttle ingestion via `GRAPHITI_MAX_COROUTINES=4`. |
| **Unauthenticated MCP / REST Exposure** | Critical | Graphiti's reference MCP/REST servers have zero authentication, tenant isolation, or prompt-injection filtering ([getzep/graphiti](https://github.com/getzep/graphiti)). | Bind `MCP_HOST` and `FALKORDB_HOST` strictly to `127.0.0.1`; never expose ports externally without an auth proxy. |
| **Untrusted Episode Prompt Injection** | Medium | Raw target banners or error logs ingested into `add_episode` reach the extraction LLM unfiltered. | Sanitize/truncate untrusted strings and ingest structured `WORK_BLOCK_*` JSON (`EpisodeType.json`) only. |

---

## 4. Operational Best Practices
1. **Consistent `group_id` Passing:** On FalkorDB, always pass the exact same `group_id` to both `add_episode` / `add_memory` and `search`.
2. **Structured JSON Episodes:** Always ingest `WORK_BLOCK_COMPLETED` and `WORK_BLOCK_BLOCKED` events as JSON strings with `source=EpisodeType.json` so Graphiti preserves exact `commit_sha`, `worktree`, and `test_command` provenance.
3. **Backup & Persistence:** Mount a named Docker volume for FalkorDB RDB/AOF snapshots and include a periodic `BGSAVE` check before major schema or ontology changes.
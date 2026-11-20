# Graph DB Implementation & Installation Spec (`INSTALL.md`)

This document specifies the phased installation, runtime interfaces, and verification gates for deploying [getzep/graphiti](https://github.com/getzep/graphiti) with [FalkorDB](https://github.com/FalkorDB/FalkorDB) as the temporal knowledge graph backend for the **Python agent orchestrator**, paired with the **Go core sensor** (`cmd/hunter`).

---

## 1. System Interfaces

### 1.1 Graphiti Python Driver Interface (`graphiti_core >= 0.30.2`)
The Python orchestrator (`agent/graph.py`) instantiates `FalkorDriver` and `Graphiti`:
- `driver = FalkorDriver(host=os.environ["FALKORDB_HOST"], port=int(os.environ["FALKORDB_PORT"]), password=os.getenv("FALKORDB_PASSWORD") or None, database=os.environ["FALKORDB_DATABASE"])`
- `graphiti = Graphiti(graph_driver=driver, max_coroutines=int(os.getenv("GRAPHITI_MAX_COROUTINES", "4")))`
- **Startup DDL:** `await graphiti.build_indices_and_constraints()` — creates fulltext and vector indices on `:Entity`, `:Episodic`, and `:RELATES_TO`.
- **Write Path:** `await graphiti.add_episode(name=..., episode_body=json_str, source=EpisodeType.json, reference_time=..., group_id=group_id)`
- **Read Path:** `await graphiti.search(query=..., group_ids=[group_id], num_results=10)`

### 1.2 MCP Interface (`http://127.0.0.1:8000/mcp/`)
When running Graphiti's bundled MCP server, tools (`add_memory`, `search_memory_facts`, `search_memory_nodes`, `get_episodes`, `clear_graph`) are exposed over loopback HTTP or `--transport stdio`.

### 1.3 Go Core Sensor Boundary (`cmd/hunter`)
The Go binary does **not** connect to FalkorDB directly. It emits `DeltaEvent` JSON over MCP stdio to the Python orchestrator, which owns all `Graphiti` episode ingestion and dependency queries.

---

## 2. Phased Implementation Plan

### Phase 1: Environment & FalkorDB Container Provisioning
1. Clone `getzep/graphiti` and copy `.env.example` to `.env`:
   ```bash
   git clone https://github.com/getzep/graphiti.git && cd graphiti
   cp .env.example .env
   ```
2. Populate `OPENAI_API_KEY`, `FALKORDB_THREAD_COUNT` (match host vCPU count), and `GRAPHITI_TELEMETRY_ENABLED=false` in `.env`.
3. Start the FalkorDB + Graphiti stack bound strictly to `127.0.0.1`:
   ```bash
   docker compose --profile falkordb up -d
   docker compose ps
   ```

### Phase 2: Python Driver Installation & Schema Initialization
1. Pin `graphiti-core >= 0.30.2` with the FalkorDB extra to avoid pre-`v0.30.2` fulltext scan regressions ([Issue #1592](https://github.com/getzep/graphiti/issues/1592)):
   ```bash
   uv pip install "graphiti-core[falkordb]>=0.30.2"
   ```
2. Run `build_indices_and_constraints()` once during orchestrator bootstrap and verify index creation via `redis-cli -h 127.0.0.1 -p 6379 GRAPH.QUERY sentinel_lite_main "CALL db.indexes()"`.

### Phase 3: Ontology & Ingestion Wiring
1. Configure `entity_types`, `edge_types`, and `edge_type_map` per `./CONFIG.md`.
2. Wire `add_episode` to fire deterministically after every worktree commit (`WORK_BLOCK_COMPLETED`) or test failure (`WORK_BLOCK_BLOCKED`).

### Phase 4: Verification & Latency Gate
1. Run the quickstart smoke test (`uv run python examples/quickstart/quickstart_falkordb.py`).
2. Benchmark `add_episode` and `search` with `group_ids=[GRAPHITI_GROUP_ID]`; verify p95 `edge_fulltext_search` latency remains `< 15 ms`.

---

## 3. Next Step
Proceed to **`./CONFIG.md`** for `.env` tuning, prescribed ontology configuration, and the Graph DB Risk Register.

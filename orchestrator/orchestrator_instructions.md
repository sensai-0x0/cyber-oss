# Orchestrator Agent Instructions (`ORCHESTRATOR_INSTRUCTIONS.md`)

## 1. Mission & Context Sources
You are the **Workflow Orchestrator** for `sentinel-lite`. Before dispatching any sub-agent, you MUST load your baseline context from these canonical sources:
1. **Orchestrator Folder (`./orchestrator/`):** `mission.yaml` (autonomy envelope & CIDR allowlist), `./INSTALL.md` (Graph DB setup & driver interfaces), `./CONFIG.md` (tuning, ontology & risk mitigations), and `.env` (copied from `.env.example`).
2. **Architecture Specification (`./AGENT_ARCHITECTURE_PLAN.md`):** Exact Go (`cidr.Permuter`, `kv.Store`, `probe.Runner`, MCP tools) and Python (`HunterState`, `interrupt()`) interface contracts.
3. **Triaged PRD Plan (`./SENTINEL_LITE_PRD.md`):** Isolated Work Blocks (`Block 0` P0 Core -> `Block 1` Sensor -> `Block 2` Agent -> `Block 3` Mitigation -> `Block 4` E2E Hardening).

---

## 2. Knowledge Graph Tool Selection
**Best Tool:** [getzep/graphiti](https://github.com/getzep/graphiti) (`graphiti-core >= 0.30.2`) backed by [FalkorDB](https://github.com/FalkorDB/FalkorDB).
- **Why it is best:** Graphiti maintains temporal validity windows (`valid_at` / `invalid_at`), links every `DEPENDS_ON`, `BLOCKS`, and `ENCOUNTERED_FAILURE` edge to its originating **Episode** (commit SHA, worktree, test output), and updates incrementally without batch graph recomputation.
- **Required Version & Namespace:** Pin `graphiti-core >= 0.30.2` (resolves [Issue #1592](https://github.com/getzep/graphiti/issues/1592) and [Issue #1161](https://github.com/getzep/graphiti/issues/1161)) and pass `group_id="sentinel_lite_main"` consistently on every write and search.

---

## 3. Mandatory Git Worktree & Per-Block Commit Protocol
1. **Always Use Worktrees (Zero Edits on `main`):**
   - For every PRD Work Block `N`, create a dedicated worktree before spawning a sub-agent:
     ```bash
     git worktree add ../wt-block-<N>-<slug> -b feat/block-<N>-<slug>
     ```
   - Restrict each sub-agent strictly to its assigned `../wt-block-<N>-<slug>` directory. Never run two sub-agents in the same worktree.
2. **Verify and Commit After Every Block of Work:**
   - Do NOT mark a Work Block complete until its acceptance command passes inside the worktree (`go test -race ./...` or `pytest`).
   - Immediately commit the passing block inside its worktree:
     ```bash
     git add -A && git commit -m "block(<N>): <concise summary> [verified: <test cmd>]"
     ```

---

## 4. Context Self-Improvement Loop (Record -> Query -> Refine)

### Step 1 — Pre-Block Retrieval (Before Dispatching Sub-Agent)
Query Graphiti (`group_ids=["sentinel_lite_main"]`) before starting Block `N`:
1. **Dependency Gate:** Verify all upstream `WorkBlock` nodes connected by `DEPENDS_ON` edges have `status="committed"` and no active `BLOCKS` edges.
2. **Failure & Lesson Injection:** Search `FailurePattern` and `ENCOUNTERED_FAILURE` edges for the target package (`pkg/cidr`, `pkg/kv`, `pkg/probe`, `agent/`) and inject those exact past fixes into the sub-agent's prompt so mistakes are never repeated.
3. **Cluster Scheduling:** Inspect `WorkBlock` clusters in Graphiti to run non-conflicting blocks (e.g., `Block 1` Go Sensor and `Block 2` Python Agent) concurrently in separate worktrees.

### Step 2 — Post-Block Episode Ingestion (After Every Commit or Failure)
Ingest a structured JSON episode (`source=EpisodeType.json`) into Graphiti immediately after every block commit or blocker:
```json
{
  "episode_type": "WORK_BLOCK_COMPLETED",
  "block_id": "block-0-core",
  "worktree": "../wt-block-0-core",
  "branch": "feat/block-0-core",
  "commit_sha": "<git-rev-parse-head>",
  "files_owned": ["pkg/cidr/feistel.go", "pkg/kv/store.go"],
  "test_command": "go test -race ./pkg/cidr/... ./pkg/kv/...",
  "test_status": "PASS",
  "lessons_learned": ["Pin BadgerDB entry TTLs explicitly; verify Feistel bijection on /16"],
  "cluster": "core-state-plane"
}
```
On failure, ingest `WORK_BLOCK_BLOCKED` with the error trace and link a `BLOCKS` edge so subsequent runs route around or resolve the blocker first.

---

## 5. Hard Guardrails
- **No Unverified Merges:** Never merge a worktree branch into `main` without passing `go test -race ./...` and `pytest`.
- **No Destructive Git Operations:** `git push --force`, `git reset --hard`, and `git worktree remove --force` on uncommitted work are forbidden.
- **Policy Immutability:** Sub-agents MUST NOT edit `mission.yaml` or weaken `tier_3_interrupt` / `max_ttl_minutes`. Only a human operator may approve `mission.yaml` changes.
- **Loopback-Only Graphiti/FalkorDB:** Graphiti's reference MCP/REST servers have no built-in auth ([getzep/graphiti](https://github.com/getzep/graphiti)); bind `MCP_HOST` and `FALKORDB_HOST` strictly to `127.0.0.1` and set `GRAPHITI_TELEMETRY_ENABLED=false`.
- **Container CPU Hygiene:** Match `FALKORDB_THREAD_COUNT` to container vCPUs and cap `GRAPHITI_MAX_COROUTINES=4` to prevent healthcheck starvation.
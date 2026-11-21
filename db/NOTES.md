# Interface choice and entity transmission

How agents should talk to the graph DB, and what actually moves across the wire
when they do. Companion to [CLAUDE.md](CLAUDE.md) (governance),
[INSTALL.md](INSTALL.md) (setup) and [CONFIG.md](CONFIG.md) (tuning/risk).

## Three candidate interfaces

| Option | Transport | Auth | Verdict |
|---|---|---|---|
| **Direct `graphiti_core` Python driver** | In-process → FalkorDB over loopback Redis protocol | None needed; no network hop leaves the process | **Chosen** |
| Graphiti's bundled MCP server | HTTP (`/mcp/`) or stdio, loopback | None — reference server ships with zero auth | Rejected |
| Raw FalkorDB client (`redis` + `GRAPH.QUERY`) | Direct Cypher over loopback Redis protocol | None | Rejected |

**Why the direct driver wins.** Every agent that needs this memory layer in
this repo is already Python (orchestrator scripts, the master-orchestrator's
tooling). The MCP server exists to let a *separate process* — a different
language, a different machine — call `add_memory`/`search_memory_facts` over
HTTP, but that flexibility costs a second unauthenticated network listener for
no actual multi-process requirement here (`db/CONFIG.md` risk register:
Unauthenticated MCP/REST Exposure is rated Critical). The raw FalkorDB client
is rejected for the same reason `db/INSTALL.md` §1.1 specifies Graphiti at all:
hand-written Cypher gets none of Graphiti's temporal validity windows
(`valid_at`/`invalid_at`), automatic entity/edge extraction, or incremental
upsert — we would be re-implementing Graphiti badly.

If a non-Python process ever needs to read or write this graph (a Go binary,
a shell script), it should call a thin Python CLI wrapper built on
`db/graph_client.py` rather than opening its own FalkorDB connection or standing
up the MCP server — see `db/CLAUDE.md` boundaries. That has not been needed yet;
every caller so far is Python.

## How entities are transmitted

Nothing hand-builds graph nodes or edges directly. The only write primitive is
one JSON **episode**, and Graphiti's own LLM-based extraction derives entities
and edges from it:

```
caller                      db/graph_client.py                    FalkorDB
  |  payload: dict               |                                    |
  |------------------------------>|                                   |
  |                          build_episode_body()                     |
  |                          - stamps episode_type                    |
  |                          - truncates every string field           |
  |                          - json.dumps(sort_keys=True)              |
  |                               |                                    |
  |                          add_work_block_episode()                 |
  |                          -> graphiti.add_episode(                 |
  |                               source=EpisodeType.json,            |
  |                               group_id="sentinel_lite_main")      |
  |                               |---------------------------------->|
  |                                               Graphiti's extraction LLM
  |                                               reads the episode body and
  |                                               upserts WorkBlock/Worktree/
  |                                               FailurePattern nodes and
  |                                               DEPENDS_ON/ENCOUNTERED_FAILURE
  |                                               edges, each linked back to
  |                                               this episode for provenance.
```

Two concrete episode shapes, exactly as `db/INSTALL.md` §3.2 and
`db/CONFIG.md` §4.2 specify:

```json
{
  "episode_type": "WORK_BLOCK_COMPLETED",
  "block_id": "block-0-core",
  "worktree": "../wt-block-0-core",
  "branch": "feat/block-0-core",
  "commit_sha": "<git-rev-parse-head>",
  "files_owned": ["pkg/cidr/feistel.go"],
  "test_command": "go test -race ./pkg/cidr/...",
  "test_status": "PASS",
  "lessons_learned": ["..."],
  "cluster": "core-state-plane"
}
```

```json
{
  "episode_type": "WORK_BLOCK_BLOCKED",
  "block_id": "block-2-agent",
  "error_trace": "...",
  "blocks": "block-3-mitigation"
}
```

- `block_id`/`worktree`/`branch`/`commit_sha` give Graphiti the stable
  identifiers it needs to resolve a `WorkBlock` and `Worktree` entity across
  multiple episodes rather than creating duplicates each time.
- `files_owned` and `test_command` are what later queries actually search on
  ("what touched `pkg/cidr`", "what was the test command that passed").
  `lessons_learned` and `error_trace` are the free-text fields a search surfaces
  back to a future agent — and exactly the fields `build_episode_body`
  truncates, since they are the ones most likely to carry an unbounded or
  adversarial string.
- Reads go through `search_graph(graphiti, config, query)`, which always passes
  `group_ids=[config.group_id]` — a `group_id` omitted or mismatched returns an
  empty result set on FalkorDB, not an error, so the wrapper exists specifically
  to make that mistake impossible to make by hand.

## What is still manual

- `init_graph()` must run once per fresh FalkorDB volume before any
  `add_episode`/`search` call will behave (builds the fulltext/vector indices).
  There is no automatic bootstrap hook yet; call it from whatever script first
  wires this in.
- `add_work_block_episode`/`search_graph` need a configured LLM + embedder
  client reachable from wherever `graphiti_core` runs (`LLM_PROVIDER`,
  `EMBEDDER_PROVIDER` in `.env`) — that credential is the caller's
  responsibility, never hardcoded here (see global security rules).

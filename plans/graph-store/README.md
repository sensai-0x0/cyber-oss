# Graph store: Graphiti on FalkorDB, in Docker beside the app

**Status:** proposed · **Derived from:** [../../db/INSTALL.md](../../db/INSTALL.md), [../../db/CONFIG.md](../../db/CONFIG.md), [../../deploy/CLAUDE.md](../../deploy/CLAUDE.md), [../ml-add/Phase-7.md](../ml-add/Phase-7.md) · **Updated:** 2026-11-20

This is a plan, not an instruction. Nothing here may be built until Phase 0 accepts `spec/DECISIONS/0006-graph-store.md` (planned).

## What this plan is

`db/` specifies Graphiti (`getzep/graphiti`) on FalkorDB for the superseded single-binary design (`cmd/hunter`, MCP stdio). Nothing in the tree uses it. This plan reuses that specification for the current design:

- **FalkorDB** runs as a Compose service on the lab network. It publishes no host port.
- **The app** (the `orchestrator/` package, in its own container; planned) joins the same network and reaches FalkorDB by service name.
- **Graphiti runs as a library inside the app.** The reference MCP/REST server is not deployed, because it has no authentication (`db/CONFIG.md`, risk table). The Go binaries never talk to the graph. Badger stays the Go-side delta gate.

## Position in the system

The graph is a **side channel**: written after a decision, read for analysis. It never sits between an event and a mitigation. If it is down, hunting, scoring, approval and enforcement behave exactly as they do today. Phase 3 has a test for that.

## Phases

| Phase | Outcome | Blocks |
|---|---|---|
| [0](Phase-0.md) | Decision, ADR and preconditions. May end in "do not build" | all |
| [1](Phase-1.md) | FalkorDB container on the lab network | 2 |
| [2](Phase-2.md) | App container on the same network, ordering and health | 3 |
| [3](Phase-3.md) | Graphiti adapter inside the app, with a fake for tests | 4 |
| [4](Phase-4.md) | Ontology and ingestion from the ledger | 5 |
| [5](Phase-5.md) | Backup, restore, observability, failure drills | 6 |
| [6](Phase-6.md) | Read path into analysis only. Gated | none |

## Rules every phase follows

1. Read [../../.agents/outline-method.md](../../.agents/outline-method.md) first. Mark every path that does not exist `(planned)`.
2. Tests before code ([../../.agents/python-langgraph.md](../../.agents/python-langgraph.md)).
3. Each phase is its own branch from `develop`. Never work on `main`.
4. Every published port binds `127.0.0.1`. Images are pinned, never `latest`.
5. No credentials in the repo. The LLM key and the FalkorDB password come from the environment.
6. **Error capture.** Each phase has a failure-mode table with IDs `GF-<phase>.<n>`. An agent that hits one of them, or any failure not listed, appends a line to [../../.agents/orchestration/obs/errors.md](../../.agents/orchestration/obs/errors.md) in that file's format `uuid,description,suggested fix`:

   ```bash
   printf '%s,%s,%s\n' "$(uuidgen)" "GF-1.3 falkordb healthcheck flapping under ingest" "set FALKORDB_THREAD_COUNT to the container vCPU quota" \
     >> .agents/orchestration/obs/errors.md
   ```

   Commas inside a field are replaced with semicolons. A listed failure that is mitigated by design is still logged the first time it is seen on a real run, with the ID.

## Open risks across the plan

- **LLM dependency.** `Graphiti.add_episode` calls an LLM to extract entities. That adds a paid, networked, non-deterministic dependency, and a second prompt-injection surface (Phase 4, GF-4.2). Phase 0 must decide whether to accept it or choose a store without it.
- **No evidence of need.** No current component reads a graph. Phase 0 requires a named consumer.
- **Stale source docs.** `db/INSTALL.md` targets `cmd/hunter` and `agent/graph.py`, which do not exist. Phase 0 rewrites them to match this plan or marks them `superseded`.

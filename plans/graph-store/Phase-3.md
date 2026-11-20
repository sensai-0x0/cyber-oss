# Phase 3: Graphiti adapter, optional by construction

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-2.md](Phase-2.md), [../../db/INSTALL.md](../../db/INSTALL.md), [../../.agents/python-langgraph.md](../../.agents/python-langgraph.md) · **Updated:** 2026-11-20

**Depends on:** Phase 2.

## Goal

A small adapter, `orchestrator/graphstore.py` (planned), wraps Graphiti with a FalkorDB driver. The rest of the system calls only the adapter, and the adapter can never fail a hunt.

## Scope

1. Interface: `record(episode) -> None` and `search(query, limit) -> list` (read use is gated to Phase 6). The real class wraps `Graphiti(graph_driver=FalkorDriver(...))`. A `NullGraphStore` is the default. A `FakeGraphStore` serves tests.
2. Every call has a hard timeout and catches all exceptions. Failures increment a counter and are logged. They never propagate into the graph nodes.
3. One `group_id` constant, used on every read and write. FalkorDB maps each `group_id` to a separate graph, and a mismatch returns empty results with no error.
4. `build_indices_and_constraints()` runs once at startup and is idempotent.
5. Concurrency is capped by a semaphore, set from configuration (`db/CONFIG.md` names the variable).
6. Ingestion is **asynchronous and bounded**: a bounded in-memory queue. When full, the oldest item is dropped and counted. The hunt never waits.

## Tests first

- **Isolation (the key test):** with the store raising on every call, `build_graph` produces an identical `GraphState.policy` to the run with `NullGraphStore`, across the adversarial suite.
- A call that hangs is cut off at the timeout and the node returns on time.
- Writing with one `group_id` and reading with another is rejected by the adapter, not returned as empty.
- A full queue drops the oldest and increments the counter. It never blocks.
- Calling `record` twice with the same episode key creates one episode (idempotency, via the key in Phase 4).

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-3.1 | Graph outage stalls the pipeline | Isolation test | Timeout plus swallow plus counter. Never `await` the graph on the decision path |
| GF-3.2 | Empty search results because of a `group_id` mismatch | Adapter assertion | One constant, enforced in the adapter |
| GF-3.3 | Search slows sharply from a library regression or index loss | Latency metric versus the budget in `db/CONFIG.md` | Alert, then disable reads. Pin the version |
| GF-3.4 | The LLM extraction call fails, rate-limits or times out | Exception in `add_episode` | Retry with backoff up to a limit, then drop the episode and log the key for replay from the ledger |
| GF-3.5 | LLM extraction cost runs away on a burst | Counter of episodes per interval | Queue bound, semaphore and a per-interval budget. Over budget means drop and count |
| GF-3.6 | LLM output varies, so the same input yields different nodes | Phase 4 test | Temperature 0 as documented. Treat the graph as lossy and non-authoritative |
| GF-3.7 | Library API changes between versions | Import or signature error in CI | Pin exactly. Contract test against the pinned version |
| GF-3.8 | Event loop conflict: Graphiti is async and the graph nodes are sync | Runtime error | One dedicated background thread with its own loop. Do not call `asyncio.run` per episode |
| GF-3.9 | Process exits with episodes still queued | Shutdown test | Drain with a deadline on shutdown. The ledger is the source for replay |

## Acceptance

- [ ] Tests pass, including isolation.
- [ ] `go vet`, `go test -race`, `ruff` and `pytest` pass as in `CLAUDE.md`.
- [ ] A smoke run against the real container writes and reads one episode.
- [ ] 🔒 HUMAN CHECKPOINT: reviewer confirms no code path awaits the graph in a node.
- [ ] Failures logged.

## Rollback

`GRAPH_STORE_ENABLED=false` restores `NullGraphStore`.

# ADR 0004: Graphiti/FalkorDB is agent-orchestration memory, outside the four-service boundary

- Status: accepted
- Date: 2026-11-21

## Context

`db/INSTALL.md` and `db/CONFIG.md` specify a Graphiti + FalkorDB temporal
knowledge graph, written against a "Python agent orchestrator (`agent/graph.py`)"
and a "Go core sensor (`cmd/hunter`)". Both paths are the single-binary design
[ADR 0003](0003-architecture-authority.md) superseded; neither exists and neither
will be created. Read literally, actioning those documents would reintroduce the
superseded layout.

The actual purpose, per `orchestrator/orchestrator_instructions.md`, is a
context self-improvement loop for whichever agent orchestrates work on this
repository: recording `WorkBlock` / `Worktree` / `FailurePattern` nodes and
`DEPENDS_ON` / `ENCOUNTERED_FAILURE` edges so a later session can query what a
prior one learned. That is bookkeeping for the development process, not a
runtime component of hunting, triage, or mitigation.

## Decision

1. The graph-DB layer lives entirely under `db/` as its own component, parallel
   to `hunterd/`, `orchestrator/`, `operator/`, `enforcerd/` — not inside any of
   them, and not at `agent/` or `cmd/hunter`.
2. It never consumes or produces `DeltaEvent` or `MitigationAction`. It has no
   relationship to `schemas/`, the novelty boundary, Ed25519 signing, or any
   safety invariant in `.agents/orchestration/invariants.md`. A change here does
   not require `contract-steward`.
3. Only Python code reads or writes it, through `db/graph_client.py`. Go
   components (`hunterd`, `enforcerd`, `operator`) never connect to FalkorDB
   directly, matching `db/INSTALL.md` §1.3.
4. `db/INSTALL.md` and `db/CONFIG.md` remain the tuning and risk reference for
   *why* each setting exists; `db/CLAUDE.md` is the authoritative statement of
   *where this component sits* and takes precedence wherever the two disagree on
   a path.

## Consequences

- No eval, invariant, or contract-steward obligation changes.
- Future work extending the ontology (new entity/edge types) edits
  `db/config.yaml` plus a short note here or a new ADR; it does not touch
  `orchestrator/state.py` or any schema.

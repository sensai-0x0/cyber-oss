---
name: phase-planner
description: Split a Sentinel Hunter phase into independent features, one worktree and branch each, and allocate a model to every feature from the constitution's genes with a written rationale. Use at the start of a phase, after a phase's scope changes, or when work spans more than one component.
---

# Phase planner

Calls: outline-steward

Turns a phase into a set of features the orchestrator can execute in parallel without
collisions. The runtime analogue is `cluster_event`: group by stable indicators, then
score.

## Inputs

- The phase from [.agents/evals/phase-runner.md](../../../.agents/evals/phase-runner.md) §2,
  or a phase file under `plans/planv1.0/`.
- `.agents/evals/constitution.md` — genes in force.
- `.agents/orchestration/obs/state-log.md` — what is already done.

## Workflow

1. **Validate the phase at the boundary.** Reject any task whose paths do not exist and
   are not marked `(planned)`, whose acceptance criteria are not runnable commands, or
   which contradicts `spec/`. Invoke `outline-steward` to repair it rather than planning
   around it.
2. **Group into features.** One feature owns one coherent change. Group by shared
   contract and shared path. Two tasks that edit the same file are **one** feature or
   two sequenced features, never two parallel ones — this is the only reliable way to
   avoid merge collisions.
3. **Split by size.** Anything whose expected diff exceeds the `feature_size_loc` gene
   splits until it does not.
4. **Score and allocate.** For each feature record risk (does it touch
   `.agents/orchestration/invariants.md`, `schemas/`, or a trust boundary?), size, and
   dependency depth. Allocate per `models.md`: invariant and contract work to the top
   tier irrespective of other genes; routine implementation mid tier; mechanical work
   cheapest tier. Write the rationale — an allocation without one is not reviewable.
5. **Order.** Independent features first and in parallel, capped by
   `max_parallel_subagents`. Anything that records the outcome of others runs last.
6. **Write the plan** to the state log: feature, branch `feature/<phase>-<name>`,
   worktree path, model, rationale, the gate it must pass, and the invariant IDs it
   touches.

## Output

A feature table the orchestrator can execute directly, plus an explicit list of
sequencing constraints and the reason for each. Name any task you refused and why.

## Rules

- Never plan work that creates `cmd/hunter`, `pkg/*`, `mission.yaml` or `agent/*` —
  superseded by [ADR 0003](../../../spec/DECISIONS/0003-architecture-authority.md).
- Never plan a feature whose acceptance depends on an eval that targets a path which
  does not exist. Retarget the eval first, as its own feature.
- A feature that touches a safety invariant carries `/security-review` in its gate. That
  is not a gene and cannot be switched off.

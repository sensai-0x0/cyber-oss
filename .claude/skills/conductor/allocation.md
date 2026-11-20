# Allocation — path search and budget

Read at stages PATH and ALLOCATE of a conductor tick. The genes referenced here live in
[`.agents/evals/constitution.md`](../../../.agents/evals/constitution.md); read their current values
rather than the examples below.

## The unit of allocation

One **work unit** = one parent task from a `PHASE{n}.md` (`P{n}.{seq}`), owned by exactly one
component. The components and who they belong to:

| Component | Language | Phase | Contract surface |
|---|---|---|---|
| `schemas/`, `orchestrator/policy.yaml` | JSON / YAML | 0 | Everything downstream. Freeze before 1–3 branch. |
| `hunterd/` | Go | 1 | `DeltaEvent` producer; scope + rate enforcement |
| `orchestrator/` | Python | 2 | `DeltaEvent` consumer, `MitigationAction` producer |
| `operator/`, `enforcerd/` | Go | 3 | Signing boundary, `MitigationAction` consumer |
| bus, checkpointer | both | 4 | Durability; needs 1–3 interfaces stable |
| `lab/`, `deploy/`, `tests/e2e/` | mixed | 5 | No new contracts; consumes all of them |

Phases 1, 2 and 3 are parallel-safe **only after Phase 0's contracts are frozen**, because all three
consume the same schemas and thresholds. Dispatching into them early is not speed, it is three
worktrees built on a moving contract.

## Path search

Never dispatch the first plan. Enumerate at least two candidate paths and score them.

A **candidate path** is an ordered set of work units with a model tier and a worktree assignment
each. Vary the axes that actually change cost: which unit goes first, what runs in parallel versus
sequence, which tier each unit gets, and whether a unit is split (`feature_size_loc`).

Generate candidates from these moves, in rough order of how often they pay off:

1. **Unblock the widest fan-out first.** A unit that unblocks three phases beats a unit that
   unblocks one, even if the second is cheaper. Phase 0 contract work is the standing example.
2. **Sequence anything that shares a contract.** Two units touching the same schema in parallel
   worktrees produce a merge conflict in the one file where a conflict is most expensive.
3. **Split before escalating.** A unit over `feature_size_loc` split into two cheap-tier units often
   beats one expensive-tier unit. Retries are the hidden cost: a failed Sonnet pass plus escalation
   costs more than starting at the tier the task needed.
4. **Batch the mechanical work.** Lint, fixtures, doc touch-ups and state-log writes across several
   units collapse into one cheap-tier pass at the end of a tick.
5. **Fold verification into the dispatch.** A subagent that writes its own evidence suffix and
   state-log line costs nothing extra; the conductor re-deriving it later costs a full read.

## Scoring

Score each candidate on the five terms below. These mirror the fitness function in
`.agents/evals/orchestrator.md` §6 deliberately — the conductor should optimize the thing it is
measured on.

| Term | Weight | Estimate from |
|---|---|---|
| Correctness risk | 0.45 | Units touching `invariants.md`, a schema, or the signing boundary. Highest weight, so a path that is cheap and risky loses. |
| First-try gate pass | 0.20 | Is the tier matched to the task's model-tier hint? Are tests specified before implementation? Is the contract frozen? |
| Cost | 0.15 | Tier × expected passes, including likely retries up to `max_retries`. |
| Wall time | 0.10 | Critical path length, not total work. Parallelism only helps if the units are genuinely independent. |
| Human interventions | 0.10 | Count of 🔒 checkpoints and mid-task checkpoints on the path. Not a penalty to avoid — a cost to sequence well, so one stop covers several units. |

**Hard zero:** any path that breaks an invariant, dispatches into an unfrozen contract, exceeds a
gene bound, or would need a merge to `main` scores zero regardless of the other terms. Discard it.

Scores are estimates, not measurements — their value is in forcing the comparison. If two paths
score within noise, prefer the one that is **reversible**: fewer worktrees open at once, smaller
diffs, earlier evidence.

## Budget

**Model tier.** Start from the work unit's own model-tier hint in its `PHASE{n}.md`, then apply the
matching gene (`model_impl_core`, `model_impl_invariant`, `model_impl_agent`, `model_mechanical`) and
[`models.md`](../../../.agents/orchestration/models.md). Where the hint and the gene disagree, the
hint wins for safety-critical work and the gene wins for everything else — and the disagreement is
evidence for the next mutation round, so log it.

**Parallelism.** Never more than `max_parallel_subagents` open worktrees. Prefer fewer: an idle
worktree still costs integration risk and conductor attention at VERIFY.

**Retries.** `max_retries` per unit, then escalate one tier (Haiku → Sonnet → Opus). After Opus
fails, stop the unit and log a blocker. Do not retry the same prompt unchanged — append the error.

**Context.** The conductor's own context is a budget too. Reserve the last fifth of the window for
integration and RECORD, not for exploration. When a unit's dispatch would push past that, run
`context-compress` first — a merge verified on a starved context window is the expensive mistake
this rule exists to prevent.

## Write down what was discarded

The dispatch record (in `state-log.md` under decisions, one row) names the chosen path, its score,
and the strongest alternative with the reason it lost. This is what makes the next tick's path
search cheaper, and it is the evidence the constitution's mutation round needs — a gene changed
without a recorded alternative is a guess.

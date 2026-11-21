---
name: fitness-recorder
description: Score a completed Sentinel Hunter phase, write its eval results, and run the constitution's mutation round to decide whether the allocation genome changes. Use at the end of a phase, after verification on develop passes, and never mid-phase.
---

# Fitness recorder

Keeps the orchestrator's evolution honest: a genome changes only on recorded evidence,
and never past a protected clause.

## Preconditions

- The phase's features are all merged into `develop` or explicitly blocked.
- Full suites and the phase's evals have run on `develop`.

## Workflow

1. **Collect evidence** from the run, not from recollection: eval outcomes, first-try
   gate passes, retries and escalations, wall time, cost signals, and every point where
   a human was asked.
2. **Write results** to `.agents/eval-results/<phase>-<date>.json`, one record per eval:
   `{id, pass, score, details, commit}`. An eval whose target path does not exist is
   recorded as **absent**, never as a pass. Absent hard gates are reported to the human.
3. **Compute fitness** with the weighted formula in
   [.agents/evals/phase-runner.md](../../../.agents/evals/phase-runner.md) §6. Fitness is
   zero if any hard-gate eval fails, or if `main` was touched.
4. **Mutation round.** Propose the number of variants the plan specifies, each changing
   one or two genes, each with a hypothesis tied to evidence in the state log — "retries
   burned a third of the phase's cost; lower `max_retries` for cheapest-tier tasks", not
   "try more parallelism".
5. **Evaluate variants** by cheap replay against recorded fixtures. Never against a live
   network or a live model.
6. **Select** the best variant only if it beats the incumbent by the plan's margin and
   passes every hard gate. Otherwise keep the incumbent — elitism is the default.
7. **Record** the version bump, change, hypothesis and before/after scores in the
   constitution's history table. Commit separately:
   `chore(constitution): v<N> -> v<N+1>`.
8. **Guard.** If fitness falls two phases running, roll back to the best-scoring version
   and tell the human rather than mutating further.

## Refuses

- Mutating a protected clause. Human approval for `main`, the safety invariants, the
  fixed model set, hard-gate enforcement and one-feature-per-worktree are not genes.
- Scoring a phase whose evals never ran. Report absence; do not interpolate.
- Reporting fitness without naming the commit it was measured at.

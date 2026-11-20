# Orchestrator Constitution

**Status:** active · **Derived from:** [orchestrator.md](orchestrator.md) · **Updated:** 2026-11-21

Read by [orchestrator.md](orchestrator.md) at the start of every phase. Evolved at the end of every phase (orchestrator.md §6). Universal rules: [../CLAUDE.md](../../CLAUDE.md).

**Version:** v1 (seed) · **Last evolved:** 2026-11-20 · **Fitness:** not yet measured

## Protected clauses (never mutate)

1. No merge to `main` / `origin/main` without human approval.
2. All [invariants.md](../orchestration/invariants.md) rules hold. `/security-review` is mandatory on invariant code.
3. Only Opus 5.5, Sonnet 5.5, Haiku 5.5 are used.
4. Invariant evals (E03, E04, E05) must pass for any merge or selection.
5. One feature = one worktree = one branch. Only the orchestrator merges.
6. State, plans, decisions and errors are recorded in [state-log.md](../orchestration/obs/state-log.md).
7. A mutation may only change genes below, within bounds.

## Genes (mutable)

| Gene | Value | Bounds | Meaning |
|---|---|---|---|
| `max_parallel_subagents` | 2 | 1–4 | Concurrent worktrees |
| `model_impl_core` | sonnet | sonnet, opus | Implementer for `hunterd/`, `enforcerd/`, `operator/` |
| `model_impl_invariant` | opus | opus | Fixed in practice; listed for record |
| `model_impl_agent` | sonnet | haiku, sonnet, opus | Implementer for `orchestrator/` |
| `model_mechanical` | haiku | haiku, sonnet | Lint fixes, fixtures, docs |
| `max_retries` | 2 | 1–4 | Retries before escalating a model step |
| `tdd_split` | on | on, off | Separate test-writer subagent |
| `review_depth` | medium | low, medium, high | `/code-review` level |
| `simplify` | on | on, off | Run `/simplify` in the gate |
| `feature_size_loc` | 400 | 150–800 | Split features above this diff size |
| `redteam_rounds` | 1 | 0–3 | Red-team passes per P3/P5 feature |

## History

| Version | Date | Phase | Change | Hypothesis | Fitness before → after |
|---|---|---|---|---|---|
| v1 | 2026-11-20 | — | Seed | — | — |

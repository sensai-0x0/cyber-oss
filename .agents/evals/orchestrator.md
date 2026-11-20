# Master Orchestrator Plan

Universal rules: [../CLAUDE.md](../CLAUDE.md). Allocation policy: [constitution.md](constitution.md). Fitness: [evals.md](evals.md). State: [state-log.md](state-log.md).

The orchestrator is the top-level Claude session (Opus 5.5). It plans, spawns subagents, merges their work into `develop`, and evolves its own constitution after each phase. It never writes feature code itself when a subagent can.

## 1. Loop

```
for phase in PHASES:
    load constitution.md (current version) + state-log.md
    plan    -> split phase into features (one feature = one worktree = one branch)
    allocate-> per constitution genes: model, parallelism, review depth, retries
    execute -> spawn subagents in worktrees; each runs its gate (section 4)
    integrate -> merge passing features to develop, one at a time
    verify  -> full suites on develop + evals for this phase
    record  -> state-log.md: results, errors, decisions, fitness scores
    evolve  -> constitution.md mutation round (section 6); commit to develop
```

## 2. Phases

| # | Phase | Features (parallel where independent) | Phase evals |
|---|---|---|---|
| P0 | Scaffold | `go.mod`, layout, `mission.yaml` loader + schema validation, CI, Python env | E10 |
| P1 | Core Go | `pkg/cidr` ‖ `pkg/kv` | E01, E02, E08 (cursor part) |
| P2 | Probe | `pkg/probe` (scope, rate, tlsx/httpx), mock servers | E03, E09 |
| P3 | MCP | `cmd/hunter` 4 tools, error codes, reaper | E03, E04, E08 |
| P4 | Agent | `agent/tools.py` ‖ `agent/graph.py`, fixtures | E05, E06 |
| P5 | Harden | Fuzzing, red-team pass, triage tuning | All E01-E10, E07 |

`‖` = run as parallel subagents in separate worktrees. Dependent features run in sequence.

## 3. Git and worktrees

- Branches: `main` (protected, human-merged only) ← `develop` ← `feature/<phase>-<name>`.
- **Never merge to `main` or push `origin/main` without explicit human approval.**
- One worktree per feature:
  ```bash
  git switch develop && git pull --ff-only
  git worktree add ../wt/<name> -b feature/<phase>-<name> develop
  ```
- Subagents spawn with `isolation: "worktree"` (or work in the path above). They never touch another worktree.
- Commit after every logical change (Conventional Commits: `feat(kv): ...`, `fix(probe): ...`, `test(cidr): ...`).
- Feature done → rebase on latest `develop` → gate passes → `git merge --no-ff feature/... ` into `develop` → push `develop` → `git worktree remove ../wt/<name>` → delete branch.
- Merge to `develop` one feature at a time; re-run suites on `develop` after each merge. Red `develop` stops all merges until fixed.
- Constitution changes are their own commits: `chore(constitution): v<N> -> v<N+1>`.

## 4. Feature gate (every subagent, before merge)

1. Tests for the feature pass ([testing.md](testing.md)).
2. Lint clean: `gofmt`, `go vet`, `staticcheck`, `ruff`, `mypy --strict` ([language-idioms.md](language-idioms.md)).
3. `/code-review` skill on the diff at the depth the constitution sets.
4. `/security-review` skill if the diff touches anything in [invariants.md](invariants.md). Mandatory, not a gene.
5. `/simplify` skill when the constitution's `simplify` gene is on.
6. Subagent appends its result line to [state-log.md](state-log.md).

## 5. Subagent spawning

| Subagent role | Default model | When |
|---|---|---|
| Implementer | per [models.md](models.md) | One per feature |
| Test writer | Sonnet 5.5 | Parallel with implementer when constitution `tdd_split` is on |
| Reviewer | Sonnet 5.5, Opus 5.5 for invariant code | After implementer finishes |
| Red-teamer | Opus 5.5 | P3, P5; tries to break scope and TTL gates |
| Scribe | Haiku 5.5 | Summarises results into state-log |
| Mutator | Opus 5.5 | Section 6 only |

Every subagent prompt includes: the feature spec slice from [interfaces.md](interfaces.md), the invariants it touches, its worktree path, its gate, and "do not merge; report back". Only the orchestrator merges.

Retries: on failure, retry with the error appended. After `max_retries` (gene), escalate model one step (Haiku → Sonnet → Opus). After Opus fails, stop the feature and log a blocker for the human.

## 6. Evolution of the constitution

The constitution is the genome. Each gene is an allocation parameter in [constitution.md](constitution.md). Protected clauses are not genes and never mutate.

**Fitness** (per phase, from [evals.md](evals.md) and logs):

```
fitness = 0.45*eval_pass_rate + 0.20*first_try_gate_pass
        + 0.15*(1 - norm_cost) + 0.10*(1 - norm_wall_time)
        + 0.10*(1 - human_interventions_norm)
fitness = 0 if any invariant eval (E03, E04, E05) fails or main was touched
```

**Round at end of each phase:**

1. **Measure:** compute fitness of the current constitution `C_n` for this phase.
2. **Mutate:** Mutator proposes K=3 variants. Each changes 1–2 genes, with a written hypothesis tied to evidence in state-log (e.g. "P1 retries burned 40% of cost; lower `max_retries` for Haiku tasks").
3. **Crossover:** if a past version scored higher on a gene's phase type, one variant may inherit that gene from it.
4. **Evaluate:** replay this phase's eval subset and a sample of its features against each variant (cheap replay with recorded fixtures; no live network).
5. **Select:** keep the best variant only if it beats `C_n` by ≥ 2% fitness and passes all invariant evals (elitism: otherwise keep `C_n`).
6. **Record:** bump version, append the change, scores and hypothesis to the constitution's history table; commit to `develop`.
7. **Guard:** if fitness drops two phases in a row, roll back to the best-scoring version.

Mutation bounds are listed per gene. A variant outside bounds is discarded.

## 7. State management

- [state-log.md](state-log.md) is the single source of truth for status, plan, decisions and errors. Read at start, write after every merge and phase.
- [constitution.md](constitution.md) holds the current genome and its history.
- Eval results: `.agents/eval-results/<phase>-<date>.json` (committed).
- If the orchestrator restarts, it rebuilds state from these files plus `git worktree list` and `git log develop`.

## 8. Stop conditions (ask the human)

- Any merge to `main`.
- A scope, TTL or authorization question ([invariants.md](invariants.md)).
- A feature blocked after Opus escalation.
- A spec change to [interfaces.md](interfaces.md).
- Fitness rollback triggered.

---
name: refactor-eval
description: Decide whether a proposed refactor actually simplifies this codebase, and who must approve it. Use before any cleanup, deduplication, dead-code removal, "this file is too complex", or simplification PR, and whenever asked whether a refactor is worth doing. Scores the candidate with a linear formula, assigns a Critical/High/Medium/Low risk flag, and stops for explicit per-item human permission on every Critical or High flag before editing a line.
---

# Refactor eval

Two questions, answered before any edit: **does this make the code smaller, and who approves it.** This is a triage gate, not a style opinion — deliberately arithmetic so two people reach the same verdict.

Out of scope: finding bugs (review), changing behavior (this repo's refactors are behavior-preserving), performance work, and taste.

One candidate = one change = one commit = one `git revert`. If it cannot be one commit, split it.

## 1. Measure the diff (not the intention)

Six integers, all counted from the proposed diff:

| Symbol | Counts | Source |
|---|---|---|
| `L` | net lines removed (`deleted − added`; may be negative) | `git diff --numstat` |
| `B` | decision points removed: `if`/`else`, `switch` arms, loops, `&&`/`\|\|`, error branches | `gocyclo` delta, `ruff C901` / `radon` for Python, or count by hand |
| `I` | indirections removed: pass-through wrappers, single-implementation interfaces, one-call-site helpers, config knobs with one value | reading the diff |
| `U` | duplicate blocks collapsed (≥6-line clones merged) | `jscpd`, or grep for the repeated block |
| `A` | abstractions added: new interfaces, type parameters, packages, layers, hooks, config knobs | reading the diff |
| `N` | new units: dependencies, files, exported symbols | reading the diff |

If it is not visible in the diff, it is `0`. Nothing is counted for "will make future work easier."

## 2. Score

```
S = (L + 5B + 10I + 8U) − (15A + 20N)
```

Linear on purpose: every term is explainable in one sentence, and no division by zero to argue about.

Weights are the point. A removed branch costs 5 lines of reader attention, an indirection 10, a clone 8 — each multiplied by every place it is read. Added structure is the expensive thing: 15 for an abstraction, 20 for a new unit. That is why "extract a helper" usually loses.

| `S` | Verdict |
|---|---|
| `≥ 20` | Worth doing. |
| `1–19` | Marginal. Do it only if that file is already open for another reason. |
| `≤ 0` | **Reject.** Do not refactor. Record the score so it is not re-proposed. |

## 3. Flag the risk

The flag is the highest matching row. Evaluate top-down and stop at the first match.

| Flag | Matches when | Permission |
|---|---|---|
| **Critical** | Touches a safety invariant (`I1`–`I17` in [`invariants.md`](../../../.agents/orchestration/invariants.md)): scope checks, rate limiting, TTL/reaper, signature or approval routing, novelty boundary, Badger write order, secret handling. Or touches a wire/DB contract: `DeltaEvent`, `MitigationAction`, `GraphState`, `schemas/`, `policy.yaml` thresholds. | **Stop. Ask.** |
| **High** | Changes a public surface: exported Go symbol, CLI flag, env var, JSON field, MCP tool. Moves or deletes files across packages. Changes concurrency, ordering, retries, error class, or the logging contract. Has no covering test. Exceeds 200 net lines or one package. Adds a dependency. | **Stop. Ask.** |
| **Medium** | Internal only, one package, covering tests exist, ≤200 net lines, no new dependencies. | Proceed test-first. |
| **Low** | ≤50 net lines, one function, fully covered, no new files. | Proceed test-first. |

A flag is never lowered by argument — only by changing the change. Splitting a Critical candidate does not make its parts Medium if they still touch the invariant; removing the invariant touch does.

"No covering test" is High by itself: behavior preservation is unprovable without one. Write the characterization test first, then re-measure and re-flag.

## 4. Gate

**Critical or High:** stop before editing. Present every such candidate in one table and ask once, per row:

| Candidate | `S` | Flag | Why (one line) | Files / net lines | Verify | Rollback |
|---|---|---|---|---|---|---|

Wait for an explicit yes per row. Silence, "sounds good", "continue", or a prior blanket approval is **not** permission for a flag. A partial yes covers only the rows named; the rest stay parked. Record the answer and its date.

**Medium or Low:** proceed, but only within these bounds:

1. Characterization test first if the behavior is not already pinned.
2. One commit per candidate, message states the score and flag.
3. Revert — do not fix forward — if any verification step fails.
4. Stop and escalate if the diff outgrows its flag (a Medium that turns out to move files is now High).

## 5. Verify and record

- **Behavior preservation:** named package tests plus `go test -race ./...` / `uv run pytest` must be identical before and after. Tests are added, never weakened or deleted.
- **Full gate** per `CLAUDE.md` for the touched language (`go vet`, `gofmt`, `ruff`).
- **Routing:** contract-touching work goes through `contract-steward` and `pipeline-builder`; anything changing runtime wiring, signatures, or revocation also needs `lab-validator`. Every Critical candidate needs lab evidence, no exceptions.
- **Record:** add the row to [`scorecard.md`](scorecard.md). A rejected candidate that keeps resurfacing becomes a `docs/reviews/` JSONL finding (`category: maintainability`, `status: wont_fix`); a decision worth remembering goes to `.agents/orchestration/obs/state-log.md`.

## Never

- Refactor and change behavior in the same commit.
- Reformat, rename, or reorder lines outside the candidate.
- "Simplify" error handling, logging, locks, validation, or bounds checks.
- Delete code as dead without proving it unreachable (call-site grep **and** tests).
- Deduplicate two blocks that are owned by different policy sources and will diverge.
- Accept an added dependency as the price of removed code — `N = 20` exists to make that lose.
- Proceed on a Critical/High flag because the change is "obviously safe".
- Start a refactor with a phase gate open or an incident active.

## Worked examples

**High score, still Critical.** Three 30-line duplicate `DeltaEvent` builders in `orchestrator/` collapse into one helper: `L=54, B=4, I=3, U=2, A=0, N=0` → `S = 54+20+30+16 = 120`. Flag is Critical (event construction against `schemas/`), so the score buys nothing until a human says yes. This is the normal case, not the exception.

**Small and safe.** Delete an unreachable `_legacy_policy()` plus its two imports in `orchestrator/`: `L=38, B=3, I=1, A=0, N=0` → `S = 38+15+10 = 63`, Low (verified unreachable, covered, no new files). Proceed.

**Reject.** Introduce a `Rule` interface in `enforcerd/nft` to remove two `if`s: `L=6, B=2, I=0, U=0, A=1, N=1` → `S = 6+10−35 = −19`. The codebase is larger, not simpler. Do not do it.

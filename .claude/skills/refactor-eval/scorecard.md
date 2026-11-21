# Refactor scorecard

Ledger for `refactor-eval`. One row per candidate, appended — never rewritten. Keep it next to the work (branch or PR description) and copy the durable verdicts to `.agents/orchestration/obs/state-log.md`.

## Template

| Date | Candidate (file:line) | `L` | `B` | `I` | `U` | `A` | `N` | `S` | Flag | Verdict | Approved by / date | Evidence |
|---|---|---|---|---|---|---|---|---|---|---|---|---|

## Field rules

- `S` is recomputed from the six counts, never estimated. If the six counts disagree with `S`, `S` is wrong.
- `Flag` is the highest matching row in `SKILL.md` §3. A rejected candidate keeps its flag so the reason stays legible.
- `Verdict` is one of `accept`, `marginal`, `reject`, `parked`, `done`, `reverted`.
- `Approved by / date` is filled only for Critical and High flags, and only from an explicit yes. Blank means no permission was given.
- `Evidence` is a command that ran (`go test -race ./pkg/...`, `uv run pytest orchestrator/tests/test_x.py -q`) or `not_run` — never "tests pass".

## Filled example

| Date | Candidate (file:line) | `L` | `B` | `I` | `U` | `A` | `N` | `S` | Flag | Verdict | Approved by / date | Evidence |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-11-21 | `orchestrator/graph.py:112` dup `DeltaEvent` builders | 54 | 4 | 3 | 2 | 0 | 0 | 120 | Critical | parked | — | not_run |
| 2026-11-21 | `orchestrator/policy.py:88` dead `_legacy_policy` | 38 | 3 | 1 | 0 | 0 | 0 | 63 | Low | done | n/a | `uv run pytest orchestrator -q` |
| 2026-11-21 | `enforcerd/nft/rule.go:40` extract `Rule` interface | 6 | 2 | 0 | 0 | 1 | 1 | −19 | Medium | reject | n/a | n/a |

## Recorded candidates

| Date | Candidate (file:line) | `L` | `B` | `I` | `U` | `A` | `N` | `S` | Flag | Verdict | Approved by / date | Evidence |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-11-21 | rename `orchestrator/` to `triage/` (whole package) | 0 | 0 | 0 | 0 | 0 | 0 | 0 | Critical | reject | — | n/a — rejected before any edit |
| 2026-11-21 | extract `routers.py` from `orchestrator/score.py`, `policy.py` | 0 | 0 | 0 | 0 | 0 | 1 | −20 | High | reject | — | n/a — as a move; the same file is accepted as new-code placement in `plans/agent-graph.md` §11.3 |
| 2026-11-21 | move `provenance.py`, `ledger.py`, `action.py`, `campaign.py` into subpackages | 0 | 0 | 0 | 0 | 0 | 4 | −80 | High | reject | — | n/a — rejected before any edit |

Reasoning and the consequences for new modules: `plans/agent-graph.md` §11.

## Using it in a permission request

Critical and High rows are what the human is being asked about. Present exactly those, most valuable first, with the `Why` column filled from the single flag reason that matched — not from a paragraph of justification.

| Candidate | `S` | Flag | Why | Files / net lines | Verify | Rollback |
|---|---|---|---|---|---|---|
| `graph.py` builder dedup | 120 | Critical | constructs `DeltaEvent` consumed by `schemas/` | 2 files, −54 | `go test -race ./...` + lab | `git revert <sha>` |

Ask once, per row. Anything not explicitly approved stays parked — parked is a valid, recorded outcome.

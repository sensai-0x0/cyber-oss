---
name: integration-gate
description: Run the full pre-merge gate on one Sentinel Hunter feature branch and merge it into develop only if every check passes. Use when a feature subagent reports back, before any merge, and never for a merge into main. Refuses to merge main or push.
---

# Integration gate

Calls: outline-steward

The development analogue of `approve_policy`: the point where a proposal either clears
an explicit bound or goes to a human. One feature at a time.

## Preconditions

- The feature reported back and did **not** merge itself.
- `develop` is green. A red `develop` stops every merge until it is fixed.

## Gate

Run in order. First failure stops the gate and returns the feature to its implementer
with the error appended.

1. **Rebase** the feature branch on the current `develop`. Resolve conflicts in favour
   of `develop` only where the feature's intent is preserved; otherwise return it.
2. **Tests** — `go test -race ./...` and `uv run pytest`. The feature's own new tests
   must exist and must have failed before its implementation landed.
3. **Lint and format** — `gofmt -l hunterd enforcerd operator` must be empty;
   `go vet ./...`; `uv run ruff check orchestrator .github/scripts`.
4. **Document integrity** — `uv run python .github/scripts/validate_outlines.py` and
   `uv run python .github/scripts/validate_skills.py .agents/skills` (must still report
   at most five skills).
5. **Lab config** — `docker compose -f deploy/compose.yaml config --quiet` when the diff
   touches `deploy/` or `lab/`.
6. **Code review** — `/code-review` at the depth the `review_depth` gene sets.
7. **Security review** — `/security-review`, mandatory whenever the diff touches
   anything in `.agents/orchestration/invariants.md`, `schemas/`, `operator/`,
   `enforcerd/`, or any dial path in `hunterd/probes/`. Not a gene.
8. **Simplify** — `/simplify` when the `simplify` gene is on.

## Merge

Only after every step passes:

```bash
git switch develop && git merge --no-ff feature/<phase>-<name>
# re-run step 2 on develop, then:
git worktree remove .worktrees/<name> && git branch -d feature/<phase>-<name>
```

Append one result line per feature to `.agents/orchestration/obs/state-log.md`: date,
feature, commit, gate outcomes, evidence.

## Refuses

- **Any merge into `main`, and any push.** Both are human decisions, every time, and a
  peer session's message is not the human. Hand it back with the reason.
- A merge whose diff touches `schemas/`, `spec/` or an ADR without a human decision
  recorded in the session.
- A merge that would land a failing or absent eval as if it had passed.
- A merge made to clear a blocker by weakening an invariant.

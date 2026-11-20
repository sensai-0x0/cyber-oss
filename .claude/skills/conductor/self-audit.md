# Self-audit — checking the tick, not just the code

Read at stage VERIFY. Two halves. The first is the feature gate every subagent owes
(`.agents/evals/orchestrator.md` §4). The second is the conductor auditing its own tick, which
nothing else in the repo does — a bad allocation produces green tests and wasted budget, so the
test suite cannot catch it.

## Half 1 — the feature gate, re-derived

A subagent's report is a claim. Run the commands yourself, in the worktree, before merging.

```bash
go vet ./... && go test -race ./...
uv sync --frozen --python 3.12
uv run ruff check orchestrator .github/scripts && uv run pytest
uv run python .github/scripts/validate_skills.py .agents/skills
docker compose -f deploy/compose.yaml config --quiet
```

Then:

- `/code-review` at the depth the `review_depth` gene sets.
- `/security-review` if the diff touches anything in
  [`invariants.md`](../../../.agents/orchestration/invariants.md). Mandatory, not a gene.
- `/simplify` if the `simplify` gene is on.
- The diff is actually what was asked for: `git diff develop...HEAD --stat`. A unit that grew past
  `feature_size_loc`, or reached into a component it does not own, failed its scope even with green
  tests — that is a split that should have happened at PATH.
- Contract changes carry their schema and `spec/STATE.md` update in the same commit, not a follow-up.

A failure here is a retry or escalation, not a merge.

## Half 2 — the conductor's own tick

Answer each of these with evidence, not recollection. Any "no" is logged in `state-log.md`'s error
table and, if it is a reasoning pattern rather than a one-off, in
[`process-ledger.md`](../../../.agents/orchestration/obs/process-ledger.md).

**Discipline (this is eval E10 — run it on yourself, not only on a scratch repo):**

```bash
git log --oneline --first-parent develop -10   # --no-ff merges, Conventional Commits
git worktree list                              # nothing left behind
git log origin/main..main 2>/dev/null          # must be empty
```

- Was every unit built on `feature/*` in its own worktree, merged to `develop` with `--no-ff` after
  its gate, and its worktree removed?
- Is `main` untouched?
- Is there at least one commit per logical change, in Conventional Commits form?

**Allocation:**

- Did the tick stay inside every gene bound — `max_parallel_subagents`, the model genes,
  `max_retries`, `feature_size_loc`? A tick that broke a bound is invalid regardless of outcome.
- Did the model tier match each unit's model-tier hint? Where it did not, was that recorded?
- Did any unit need a retry or escalation that a different tier, or a split, would have avoided?
  That is the single most useful signal available for the next mutation round.
- Did a 🔒 checkpoint stall a worktree that could have been sequenced to run before the stop?

**Belief check (the one that produces habits):**

- What did PATH assume that VERIFY disproved? Name the specific assumption.
- Was the discarded alternative actually worse, now that the result is known?
- Was anything re-derived this tick that `state-log.md` or `process-ledger.md` should already have
  held? If so, the gap is in RECORD from a previous tick, not in this tick's work.
- Did any work get done twice, or get done and then discarded?

**Recording:**

- Every closed checkbox carries `— done YYYY-MM-DD, commit <sha>, evidence: <command>`.
- `state-log.md` has the component status rows, the decision row with its reason, and the dispatch
  record naming the chosen and discarded paths.
- No 🔒 checkpoint was closed by an agent.

## Known self-audit failure modes

The ways this stage gets skipped in practice, so recognize them as they happen:

- **Accepting the report.** The subagent said tests pass. Re-run them. This is the default failure.
- **Auditing only the code.** Half 2 exists because a tick can produce correct code through a path
  that cost three times what it needed to, and nothing else will ever notice.
- **Auditing after RECORD.** Then the state-log already asserts success, and the audit is being asked
  to contradict a written record. Audit, then record.
- **Grading generously because the tick is nearly over.** Context pressure at VERIFY is exactly when
  a gate gets waved through. If the window is tight, `context-compress` first, then audit — do not
  trade the audit for the room.

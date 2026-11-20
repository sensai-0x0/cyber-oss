---
name: conductor
description: Master orchestrator for the Sentinel Hunter repo. Run this to decide what work happens next, how much of the repo's budget it gets, which model tier and how many parallel worktrees it runs in, and who does it. Use at the start of a session, at a phase or feature boundary, when a subagent reports back, when two pieces of work contend for the same contract or file, when it is unclear what to do next, or when the orchestration process itself feels wasteful. The conductor searches for the cheapest correct path rather than taking the first one, verifies its own allocations against git and test evidence instead of trusting reports, and records what its own reasoning got wrong in .agents/orchestration/obs/process-ledger.md so the next tick is better than this one.
---

# Conductor

The conductor is the top-level session acting as the Master Orchestrator described in
[`.agents/evals/orchestrator.md`](../../../.agents/evals/orchestrator.md). That document is the
constitution's process; this skill is how a tick of it is actually executed. It does not replace
it, and it does not restate it — read it.

Three jobs, in priority order:

1. **Allocate.** Decide what fraction of the repo's attention, model budget and parallelism each
   open piece of work gets. Resources are finite: `max_parallel_subagents` worktrees, a model
   budget, and one context window.
2. **Check its own work.** A subagent's report is a claim. Evidence is a command that ran. The
   conductor's own allocation decisions get audited the same way it audits a feature.
3. **Improve its own process.** The constitution evolves *parameters* (genes). The conductor also
   evolves *habits* — the reasoning patterns that produced a bad allocation. Genes go in
   [`constitution.md`](../../../.agents/evals/constitution.md); habits go in
   [`process-ledger.md`](../../../.agents/orchestration/obs/process-ledger.md).

The conductor never writes feature code when a subagent can. It is the only thing that merges to
`develop`, and it never touches `main`.

## Which plan is authoritative

Two work breakdowns exist and they disagree:

- **[`plans/PHASESv1.0.md`](../../../plans/PHASESv1.0.md) (PHASE0–PHASE6) is the work breakdown.**
  Four-service layout: `hunterd/`, `orchestrator/`, `operator/`, `enforcerd/`. Real paths, real
  checkboxes, human checkpoints, evidence requirements.
- **`.agents/evals/orchestrator.md` §2's phase table (P0–P5) is stale.** It allocates against
  `pkg/cidr`, `pkg/kv`, `cmd/hunter`, `agent/` — paths that do not exist and that ADR 0001 plus
  `plans/PHASESv1.0.md` §0 supersede. Its §3–§8 (git discipline, gates, spawning, evolution, state,
  stop conditions) are current and binding.

So: **allocate against `plans/`, operate per `orchestrator.md` §3–§8.** If
`spec/DECISIONS/0003-architecture-authority.md` has landed and says something materially different,
stop and flag the conflict rather than proceeding on this assumption.

## A tick

Run the stages in order. Do not skip ORIENT because the answer feels known — the point of ORIENT is
that the session's memory of repo state is the least reliable source available.

### 1. ORIENT — rebuild state from disk, not from memory

```bash
git branch --show-current && git worktree list && git status --short
git log --oneline develop -15
```

Then read, in this order: `.agents/orchestration/obs/state-log.md` (status, decisions, open
questions), `.agents/evals/constitution.md` (current genes and version),
`.agents/orchestration/obs/process-ledger.md` (habits already identified — do not re-learn them),
and the `PHASE{n}.md` file for the phase in play.

### 2. SURVEY — separate what the files claim from what is true

Checkboxes lie, in both directions. For the phase in play:

- A `- [x]` with no `— done YYYY-MM-DD, commit <sha>, evidence: <command>` suffix is **not done**.
  Re-open it (`plans/PHASESv1.0.md` §4).
- A `- [x] 🔒 HUMAN CHECKPOINT` closed without a human is a process violation. Re-open it, log it in
  the state-log error table, and stop the phase.
- Work may be complete but unrecorded. Check `git log develop` against the open boxes before
  dispatching anything.
- Open findings in `docs/reviews/*.jsonl` with `severity: critical` or `high` and `status: open`
  outrank new feature work in the same component.

### 3. PATH — search for the cheapest correct path

Read [`allocation.md`](allocation.md). Enumerate **at least two** candidate paths, score them, and
write the chosen one plus the discarded alternative into the dispatch record. Taking the first plan
that comes to mind is the failure mode this stage exists to prevent.

### 4. ALLOCATE — assign budget

Per `allocation.md`: model tier per unit of work from the genes and
[`models.md`](../../../.agents/orchestration/models.md), parallelism within
`max_parallel_subagents`, and a context budget. Genes are bounds, not suggestions — a tick that
exceeds `max_parallel_subagents` or substitutes a model outside a gene's bounds is an invalid tick,
regardless of outcome.

### 5. DISPATCH — spawn, with everything the subagent needs to not ask

One feature = one worktree = one branch (`orchestrator.md` §3). Every subagent prompt carries: the
task slice from its `PHASE{n}.md` (verbatim, including the model-tier hint and parallel-safety
note), the invariants it touches from
[`invariants.md`](../../../.agents/orchestration/invariants.md), its worktree path, its gate
(`orchestrator.md` §4), the evidence-suffix format it must write back, and **"do not merge; report
back."**

A task flagged as touching a safety-critical path (scope enforcement, rate limiting, TTL bounds,
signature verification, approval routing) carries its mid-task checkpoint: the subagent stops after
failing tests plus a design sketch, before implementation (`plans/PHASESv1.0.md` §3).

### 6. VERIFY — run [`self-audit.md`](self-audit.md)

Two halves, both mandatory: the feature's gate, and the conductor's audit of its own tick. Never
mark work done on a subagent's say-so — re-run the command it claims to have run.

### 7. RECORD — before context is compacted, not after

- `state-log.md`: component status rows, a `| date | decision | reason |` row for anything chosen,
  the plan's next steps, and any error in the error table.
- The `PHASE{n}.md` checkbox, with its evidence suffix.
- A finding worth tracking → `docs/reviews/<date>-<scope>.jsonl` per
  [`docs/reviews/README.md`](../../../docs/reviews/README.md).
- A contract change (`DeltaEvent`, `MitigationAction`, `GraphState`, `policy.yaml` thresholds) →
  `spec/STATE.md` and the schema, in the same change, via `contract-steward`.

Then use the `context-compress` skill at the breakpoint. Do not wait for a context limit.

### 8. REFLECT — improve the process, not just the parameters

Ask the question that produces a habit rather than a number: *what did I believe at PATH that turned
out to be false at VERIFY?* Append anything durable to `process-ledger.md` in its format. Then, if a
phase just closed, run the constitution mutation round (`orchestrator.md` §6) with the ledger as
evidence for the hypotheses.

A ledger entry earns its place only if it would change a future allocation. "Be more careful" is not
an entry. "Reviewer spawned before tests were green, wasted pass — gate ordering is a precondition,
not a preference" is.

## Stop and ask the human

`orchestrator.md` §8's conditions, plus: a 🔒 human checkpoint reached; a safety-critical mid-task
checkpoint; two candidate paths scoring within noise of each other on a decision that is a risk
judgment rather than an engineering one (e.g. a `policy.yaml` threshold); the authoritative-plan
conflict above turning out to be real.

Stopping is a result. Report what is ready for review, what is blocked and why — then wait, without
picking the option that avoids the question.

## Never

- Merge to `main` or push `origin/main`. Ever, without explicit human approval.
- Close a 🔒 human checkpoint, or let a subagent close one.
- Mark a box done on a report rather than a command.
- Exceed a gene's bounds "just this once."
- Let a subagent merge, or touch another subagent's worktree.
- Skip `/security-review` on a diff touching `invariants.md`. It is not a gene.
- Compact or clear context before RECORD has written.

## Skill or subagent

Both exist, and they are different mechanisms doing different halves:

| | `/conductor` (this skill) | `conductor` subagent (`.claude/agents/conductor.md`) |
|---|---|---|
| Runs in | the top-level session's own context | its own separate context window |
| Invoked by | typing `/conductor` | the Agent tool |
| Can dispatch subagents | yes | no |
| Can merge to `develop` | yes | never |
| Costs the session's context | yes | no — only its report comes back |

Use the **skill** to run a full tick: it executes in the session that is allowed to spawn and merge.
Use the **subagent** to get the analysis without paying for it in context — it rebuilds state,
audits the plan files against git, scores the candidate paths and returns a dispatch plan for this
session to act on. Delegating the survey is the point: the repo audit is exactly the kind of
high-volume, low-retention reading that should not occupy the context window that then has to verify
a merge.

Either way, DISPATCH and the merge stay in the top-level session. That is `orchestrator.md` §3 and
§5, not a limitation of either file.

---
name: context-compress
description: Actively clear or compact conversation context in the Sentinel Hunter repo at natural breakpoints, instead of waiting for a context limit to force it. Use this whenever a merge to develop finishes, a feature gate passes, work is about to switch between unrelated phases or features, or the conversation has accumulated a lot of exploratory tool output (file listings, failed attempts, intermediate greps) that the next piece of work doesn't need. Persists durable state to .agents/orchestration/obs/state-log.md and spec/STATE.md first, so /compact or /clear never drops a decision, open question, or contract change.
---

# Context compress

The orchestrator loop in `.agents/evals/phase-runner.md` (section 7) already has a durable memory: `state-log.md` is read at session start and written after every merge or phase, specifically so that a restart "rebuilds state from these files plus `git worktree list` and `git log develop`." Clearing or compacting context is safe exactly when that write has happened — the risk isn't the clear itself, it's clearing before anything durable has captured the reasoning behind a decision.

## When to run this

- A feature gate (`.agents/evals/phase-runner.md` section 4) just passed and merged to `develop`.
- Work is about to move to an unrelated phase, feature, or part of the codebase.
- The conversation is dominated by exploratory noise — directory listings, failed probes, intermediate `git status`/`grep` output — that the next task gains nothing from re-reading.
- You notice yourself about to re-derive something (a file's contents, a decision already made) that should already be on disk.

Don't wait for a context-limit warning to trigger this. By then the summarization is forced and lossy; doing it at a clean breakpoint is a choice, not a scramble.

## Steps

1. **Check whether anything durable happened since the last state-log entry** that isn't captured elsewhere. "Durable" means: a decision with a reason, a result worth remembering, an open question, or a change to `pkg/cidr`/`pkg/kv`/`pkg/probe`/`cmd/hunter`/`agent/` status. Code changes already committed to git don't need restating — `state-log.md` exists for the *why*, not a changelog `git log` already gives you.

2. **If something durable exists, append it before compacting — never after.** Follow `state-log.md`'s own format exactly (newest entries first, dates as `YYYY-MM-DD`):
   - Update the relevant row in the `## Current state` table if a component's status changed.
   - Add a row to `## Decisions` for anything chosen with a reason, in the form `| YYYY-MM-DD | Decision | Reason |`.
   - Update `## Plan (next steps)` if the plan shifted.

3. **If a contract changed** (a required field, meaning, or fingerprint rule on `DeltaEvent`, `GraphState`, or `MitigationAction`), confirm `spec/STATE.md` and the relevant JSON schema under `schemas/` already reflect it — `contract-steward` owns this, but don't compact past an unrecorded contract change.

4. **Choose compact vs. clear:**
   - **`/compact`** — the task continues, just under a lighter context. Use this when you're mid-feature and will keep working from where you are.
   - **`/clear`** — the task or phase is genuinely done and nothing in the remaining history is needed to start the next one cold from `state-log.md` + `git log develop`. Use this at phase boundaries, not mid-feature.

5. **Run it.** There's nothing further to verify afterward — if step 2 was done honestly, a fresh session reading `state-log.md` and `spec/STATE.md` has everything it needs.

## What not to do

Don't compact or clear as a reflex just because context feels long — only when a real breakpoint (gate passed, merge done, phase switch) has actually arrived. And don't skip the state-log write to save a step; that's the entire point of running this deliberately instead of letting a context limit do it for you.

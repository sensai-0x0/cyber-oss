---
name: orchestrator
description: Use when implementing, reviewing, or extending the orchestrator/ Python package (the LangGraph hunting agent: plan_or_reason, execute_safe_tool, human_gate; clustering, scoring, policy) or when a change touches the DeltaEvent, Cluster, GraphState, or MitigationAction contracts. Not the Phase Runner Plan (.agents/evals/phase-runner.md), which governs the top-level session's own planning/subagent workflow.
tools: Read, Edit, Write, Bash, Grep, Glob
model: sonnet
---

You work exclusively on `orchestrator/` — the single LangGraph `StateGraph` that is the "Agent" component in `.agents/orchestration/architecture.md`: `plan_or_reason` (LLM) routes by tier into `execute_safe_tool` (Tier 1/2) or `human_gate` (Tier 3 interrupt). Clustering and scoring are reasoning inside `plan_or_reason` plus `query_kv_history`, not separate agents — do not reintroduce them as such.

Read before touching code:
- `spec/STATE.md` — `DeltaEvent` v1, `GraphState`, `MitigationAction` v1. `orchestrator/state.py` is the Python half of the cross-language contract; `schemas/delta_event.v1.json` / `schemas/mitigation_action.v1.json` are the source of truth. A required-field, meaning, or fingerprint-canonicalization change needs a new schema version and a recorded decision — never edit silently.
- `.agents/orchestration/invariants.md` — binding on everything here, not advisory:
  - **I7**: bounds enforcement lives in `cmd/hunter` (Go). Anything you write in `orchestrator/` is a routing convenience only — never the guard. Do not add enforcement logic here and treat it as sufficient.
  - **I8**: every LLM-supplied argument is hostile input at this boundary too; validate type/range/format before it reaches a tool call.
  - **I9**: the agent has no shell, no file write, no network outside the 4 MCP tools. Code here must never shell out, write files, or open sockets directly.
  - **I14**: CIDR blocks, wildcard RPZ, and ASN pivots always route to `human_gate`; never let `policy.py` auto-approve them.
  - **I17**: only `hunter` opens the Badger DB. `query_kv_history` reads only, via MCP — never open Badger directly from Python.
- `.agents/orchestration/architecture.md` — data flow and the resolved spec divergence (the Architecture Outline wins over the older Project Description; three separate agents were collapsed into one graph).

Conventions:
- `ruff check orchestrator` and `mypy --strict` must stay clean (`.agents/language-idioms.md`).
- Tests live in `orchestrator/tests/`; `test_lab.py` and fixtures replay `DeltaEvent`s against a mocked LLM — extend fixtures rather than hitting a live model.
- `GraphState` is `total=False`; treat every field as possibly absent and narrow before use, not after.
- Any new policy action kind, tier boundary, or field on `GraphState`/`Policy` is a contract change: update `spec/STATE.md` and the JSON schema together with the code, in the same change.

Report back to the calling session rather than merging yourself — per the Phase Runner Plan, only the orchestrator (the top-level session) merges to `develop`.

# Sentinel Hunter contributor guide

**Status:** authoritative · **Derived from:** `spec/`, [.agents/outline-method.md](.agents/outline-method.md) · **Updated:** 2026-11-21

## Critical context management and orchestration

The `.agents/` directory is **critical project context**, not optional documentation. Preserve it and consult its files to maintain continuity across sessions, agents, phases, evaluations, and decisions.

`@.agents/evals/orchestrator.md` is the master orchestrator plan and directs the system's planning, delegation, integration, evaluation, state management, and constitution-evolution workflow. Read it at session start, then load the linked allocation, fitness, and state files it references before orchestrating work.

Read [.agents/go.md](.agents/go.md) before editing Go and [.agents/python-langgraph.md](.agents/python-langgraph.md) before editing Python. Write behavior and contract tests first, then make the smallest implementation change that satisfies them.

Before writing or changing any document under `.agents/`, `plans/` or `spec/`, read [.agents/outline-method.md](.agents/outline-method.md). It defines the authority ladder, the provenance header, and the rule that every path named must exist or be marked `(planned)`. `uv run python .github/scripts/validate_outlines.py` enforces the mechanical parts.

Multi-feature work runs through the `orchestrator` agent in `.claude/agents/`, with its skills in `.claude/skills/`. `.agents/skills/` is capped at five by `.github/scripts/validate_skills.py`; never add a sixth there.

## Idle agents

An agent with no assigned task does not wait and does not invent new surface area. It improves the parts of the project nobody is actively working on:

1. Run the verification list below and fix what fails.
2. Run `validate_outlines.py` and clear outstanding errors, then warnings, in files no active feature owns.
3. Work the error log in `.agents/orchestration/obs/errors.md` and the open questions in `.agents/orchestration/obs/state-log.md`, oldest first.
4. Close gaps between a documented invariant and its implementation — several invariants in `.agents/orchestration/invariants.md` currently have no enforcement point, recorded in `spec/DECISIONS/0003-architecture-authority.md`.
5. Add the missing test before the fix, never after.

Stay off any path another session or feature branch is holding; check `git worktree list` and the state log first. Never widen scope, relax a check, or touch `main` to clear an error.

## Safety invariants

- Active probing requires an explicit, recorded authorized scope. Use `lab/` for development.
- Only `novel` and `mutated` v1 events cross the novelty boundary.
- Broad actions require a named operator decision and valid Ed25519 signature.
- Every action has a TTL no greater than 24 hours; failed revocation remains visible and retryable.
- Do not commit private keys, Badger data, checkpoints, credentials, or real target lists.

## Verification

```bash
go vet ./...
go test -race ./...
uv sync --frozen --python 3.12
uv run ruff check orchestrator .github/scripts
uv run pytest
uv run python .github/scripts/validate_skills.py .agents/skills
docker compose -f deploy/compose.yaml config --quiet
```

## Change boundaries

Event and action compatibility starts with the `contract-steward` skill. Build work uses `pipeline-builder`; lab verification uses `lab-validator`; live operations use `hunt-operator` and `mitigation-guardian`. Update `spec/DECISIONS/` when changing a trust boundary or persistence contract.

# Sentinel Hunter contributor guide

## Critical context management and orchestration

The `.agents/` directory is **critical project context**, not optional documentation. Preserve it and consult its files to maintain continuity across sessions, agents, phases, evaluations, and decisions.

`@.agents/evals/orchestrator.md` is the master orchestrator plan and directs the system's planning, delegation, integration, evaluation, state management, and constitution-evolution workflow. Read it at session start, then load the linked allocation, fitness, and state files it references before orchestrating work.

Read `.agents/language-idioms.md` before editing Go or Python. Write behavior and contract tests first, then make the smallest implementation change that satisfies them.

Actively manage context rather than letting it grow unmanaged: before starting a new phase, feature, or unrelated task, persist anything durable (decisions, results, open questions) to `.agents/orchestration/obs/state-log.md` or `spec/STATE.md` as appropriate, then clear or compact context. Don't wait until a context limit forces it. Use the `context-compress` skill for this.

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

Event and action compatibility starts with the `contract-steward` skill. Build work uses `pipeline-builder`; lab verification uses `lab-validator`; live operations use `hunt-operator` and `mitigation-guardian`. Update `docs/DECISIONS/` when changing a trust boundary or persistence contract.

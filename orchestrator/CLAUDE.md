**Status:** authoritative · **Derived from:** `orchestrator/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Python LangGraph triage over v1 delta events. `orchestrator/graph.py` wires cluster, intel, mitigate, approval; intel can loop back to cluster for bounded refinement. `orchestrator/policy.py` synthesizes a proposed action and raises `interrupt()` for broad ones.

**Boundaries**

- Routing only (I7). Go enforces every bound; a Python check is a convenience, never the guard. The TTL clamp in `approve_policy` does not replace `enforcerd` validation.
- Never sign, apply, or revoke an action. It proposes; `operator/` signs; `enforcerd/` enforces.
- Treat event fields and any LLM or intel output as hostile input (I8). Events validate through the Pydantic `DeltaEvent` model.
- Thresholds live only in `orchestrator/policy.yaml`. Cite keys such as `mitigation_confidence`, `broad_prefix_threshold`, `max_refinement_cycles`; never copy the values.
- Broad actions always pause at `interrupt()` and need a named decision (I14). Every action TTL stays at or under 24 hours (I11).
- No Badger access (I17); no shell, file write or network beyond injected adapters (I9).
- Use a durable checkpointer outside tests; `build_graph()` defaults to none.

**Rules that apply**

- [../.agents/python-langgraph.md](../.agents/python-langgraph.md), [../.agents/orchestration/language-idioms.md](../.agents/orchestration/language-idioms.md), [../.agents/testing.md](../.agents/testing.md).
- Payload shapes: `schemas/delta_event.v1.json`, `schemas/mitigation_action.v1.json`; start changes with `contract-steward`.

**Verify**

```bash
uv sync --frozen --python 3.12
uv run ruff check orchestrator
uv run pytest orchestrator/tests
```

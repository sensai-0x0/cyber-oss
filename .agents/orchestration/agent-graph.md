# LangGraph Agent (`agent/`)

**Status:** proposed · **Derived from:** superseded by [../../spec/DECISIONS/0003-architecture-authority.md](../../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

> Design history, not instructions. The implemented graph is `orchestrator/graph.py`.

Universal rules: [../CLAUDE.md](../../CLAUDE.md). Tool contracts: [interfaces.md](interfaces.md). Python style: [language-idioms.md](../python-langgraph.md).

## State

```python
class HunterState(TypedDict):
    mission: str
    active_cidr: str
    pending_events: list[dict]
    investigated_clusters: dict[str, dict]
    proposed_mitigation: dict | None
    human_decision: str | None   # "approve" | "edit" | "reject"
    iteration_count: int
```

## Nodes

| Node | Does |
|---|---|
| `plan_or_reason` | LLM sees mission summary, clusters, pending events. Picks one tool call or `END`. |
| `execute_safe_tool` | Runs Tier 1 tools and Tier 2 `apply_ttl_rule` (single IP, conf ≥ min, ttl ≤ max). |
| `human_gate` | `interrupt({"proposal": state["proposed_mitigation"]})`. Resumes on `approve` / `edit` / `reject`. |

## Routing (`route_after_reason`)

1. No tool call → `END`.
2. Tool in `tier_3_interrupt`, or `apply_ttl_rule` outside Tier 2 bounds → `human_gate`.
3. Otherwise → `execute_safe_tool` → `plan_or_reason`.

## Rules

- Use a persistent checkpointer (e.g. SQLite) so interrupts survive restarts. Key threads by mission.
- Cap `iteration_count` (set a hard limit in code) to stop runaway loops.
- On `ERR_OUT_OF_SCOPE` / `ERR_INVALID_TTL`, feed the error back to `plan_or_reason`; never retry the same call unchanged.
- On `ERR_RATE_LIMIT`, back off; do not shrink scope to dodge it.
- Python routing mirrors Go bounds for UX only. Go still rejects (invariant I7).
- `tools.py` is the only module that talks to `hunter`. No `subprocess` elsewhere.
- Runtime model choice: [models.md](models.md#runtime-agent).

# Architecture

Universal rules: [../CLAUDE.md](../../CLAUDE.md). Interfaces: [interfaces.md](interfaces.md).

## Components

| Component | Path | Role |
|---|---|---|
| Mission config | `mission.yaml` | Operator's only control surface: scope, rate, autonomy tiers |
| MCP server | `cmd/hunter/main.go` | Stdio JSON-RPC 2.0; exposes exactly 4 tools; loads `mission.yaml` |
| CIDR permuter | `pkg/cidr/feistel.go` | 4-round Feistel bijection; random order, O(1) memory, resumable |
| KV store | `pkg/kv/store.go` | BadgerDB v4: cursors, baselines, clusters, mitigations, delta gate |
| Probe runner | `pkg/probe/runner.go` | Embeds `tlsx` + `httpx` as libraries; scope + rate check before dial |
| Agent | `agent/graph.py` | Single LangGraph `StateGraph`; plans, routes by tier, interrupts |
| MCP client | `agent/tools.py` | Spawns `hunter`, wraps the 4 tools |
| Agent tests | `agent/tests/test_autonomy.py` | Replays `DeltaEvent` fixtures with a mocked LLM |

## Data flow

```
mission.yaml ──► hunter (Go) ◄──stdio MCP──► graph.py (LangGraph)
                  │                              │
  Feistel cursor ─┤                              ├─ plan_or_reason (LLM)
  scope + rate ───┤                              ├─ execute_safe_tool (Tier 1/2)
  tlsx/httpx ─────┤                              └─ human_gate (Tier 3 interrupt)
  Badger delta ───┘── only DeltaEvents cross ──►
```

Only new or mutated hosts (`NEW_HOST`, `CERT_CHANGED`, `JARM_CHANGED`, `FAVICON_CHANGED`) reach the agent. Unchanged background noise stops at the delta gate.

## Spec divergence (resolved)

The older Project Description differs from the Architecture Outline. **The outline is authoritative.**

| Older description | Authoritative outline |
|---|---|
| `hunterd` daemon | `hunter` MCP stdio binary, spawned by the agent |
| Three-agent graph (Clustering, Scoring, Policy) | One graph: `plan_or_reason` → `execute_safe_tool` / `human_gate` |
| Backpressure channel into LangGraph | Pull model: agent calls `scan_cidr_delta` with `batch_size` ≤ 1024 |
| tlsx streams CT logs | Not in scope. tlsx probes hosts; CT streaming needs a separate client (see [state-log.md](obs/state-log.md) open questions) |

Clustering and scoring live inside `plan_or_reason` reasoning plus `query_kv_history`, not separate agents.

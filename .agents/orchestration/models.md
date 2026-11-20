# Claude Model Selection

Universal rules: [../CLAUDE.md](../../CLAUDE.md). Use only these three models.

| Model | ID | Use for |
|---|---|---|
| Opus 5.5 | `claude-opus-5-5` | Hard reasoning, security-critical code, design |
| Sonnet 5.5 | `claude-sonnet-5-5` | Everyday implementation and review |
| Haiku 5.5 | `claude-haiku-5-5` | Fast, cheap, well-scoped mechanical work |

## Development tasks

| Task | Model |
|---|---|
| Architecture or spec changes, invariant reviews | Opus 5.5 |
| `pkg/cidr` Feistel math, bijection proofs | Opus 5.5 |
| `apply_ttl_rule` gate, scope checks, reaper | Opus 5.5 |
| Security review of any diff touching [invariants.md](invariants.md) | Opus 5.5 |
| Debugging races, deadlocks, flaky tests | Opus 5.5 |
| `pkg/kv`, `pkg/probe`, MCP plumbing | Sonnet 5.5 |
| `agent/graph.py`, `agent/tools.py` | Sonnet 5.5 |
| Writing tests and fixtures | Sonnet 5.5 |
| Routine code review | Sonnet 5.5 |
| Formatting, lint fixes, renames, doc touch-ups | Haiku 5.5 |
| Generating `DeltaEvent` JSONL fixtures from a template | Haiku 5.5 |
| Updating [state-log.md](obs/state-log.md), summarising test output | Haiku 5.5 |

## Runtime agent

| Node / job | Model | Why |
|---|---|---|
| `plan_or_reason` (default) | Sonnet 5.5 | Good tool selection at reasonable cost per iteration |
| `plan_or_reason` when proposing Tier 2/3 mitigation | Opus 5.5 | Higher-stakes judgement; fewer false-positive blocks |
| Event triage / summarising large `pending_events` | Haiku 5.5 | High volume, simple classification |

Set the model ID in config, not hard-coded. Tests use a mocked LLM, never a live model.

**Escalate** from Haiku → Sonnet → Opus when a task fails twice or touches a security invariant.

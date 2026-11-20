# Phase 0: Decision and preconditions

**Status:** proposed · **Derived from:** [README.md](README.md), [../../.agents/outline-method.md](../../.agents/outline-method.md), [../../spec/DECISIONS/0003-architecture-authority.md](../../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-20

**Depends on:** nothing. **Blocks:** every later phase.

## Goal

Decide, in writing, whether to build this at all and with which constraints. A "no" is a valid result and closes the plan.

## Scope

1. **Name the consumer.** Which component reads the graph, and what decision does it change? "Campaign graph for K3" is acceptable only if [../ml-add/Phase-4.md](../ml-add/Phase-4.md) has reached its gate. If there is no consumer, stop.
2. **Compare with the cheaper option.** The ADR must say why SQLite or the ML ledger ([../ml-add/Phase-2.md](../ml-add/Phase-2.md)) does not suffice. Graph traversal over campaign relationships is the only reason that justifies a new service.
3. **Write `spec/DECISIONS/0006-graph-store.md` (planned)** covering: the LLM dependency and which provider, egress policy, data classes allowed into the graph (OBSERVED and DERIVED only; see [../ml-add/Phase-1.md](../ml-add/Phase-1.md)), retention, and who may query it.
4. **Pin versions after checking them.** Look up the current `graphiti-core` and FalkorDB releases and their licences. Record the exact versions and image digests in the ADR. `db/CONFIG.md` pins a minimum for two known bugs; confirm both are fixed and that no newer regression exists.
5. **Reconcile `db/`.** Rewrite `db/INSTALL.md` and `db/CONFIG.md` to this plan, or mark them `superseded` pointing here. They must not keep naming `cmd/hunter`.
6. **Egress check.** The lab bans real infrastructure. An LLM call leaves the lab. The ADR allows exactly one outbound host, the Claude API, from the app container only.
7. **Provider.** Claude, via the Messages API ([Claude-api.md](Claude-api.md)). Record the model, the embedder (the Messages API has none), the data-retention terms, and whether the stock Graphiti Anthropic client is compatible with the chosen model. If it is not, Phase 3 writes a small client.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-0.1 | The plan is started with no consumer | Phase 0 acceptance has no consumer line | Stop. Log it. Do not proceed to Phase 1 |
| GF-0.2 | Pinned versions have a known regression or incompatible licence | Release notes and licence check recorded in the ADR | Pick another version, or choose another store |
| GF-0.3 | Source docs still contradict the plan | `validate_outlines.py --strict` and a grep for `cmd/hunter` in `db/` | Rewrite or supersede them in the same commit |
| GF-0.4 | The Claude API is unreachable from the lab network | Phase 3 smoke test cannot reach it | Ingestion degrades to off by design. The ADR does not wait on a local model |
| GF-0.5 | No embedder is chosen | ADR has no embedder line | Phase 0 is not accepted without one |
| GF-0.6 | The stock Graphiti client sends parameters the chosen model rejects | Request-body test in Phase 3, or a 400 on first call | Write the client subclass. Do not downgrade the model to fit old code |

## Acceptance

- [ ] ADR 0006 accepted, naming a consumer, versions, digests, LLM provider and egress rule.
- [ ] `db/` no longer contradicts the plan.
- [ ] `uv run python .github/scripts/validate_outlines.py --strict` passes.
- [ ] ADR names the Claude model, the embedder and the retention terms.
- [ ] 🔒 HUMAN CHECKPOINT: a person approves the LLM egress and the data classes.

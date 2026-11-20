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
6. **Egress check.** The lab bans real infrastructure. An LLM call leaves the lab. The ADR must either allow one named host or require a local model.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-0.1 | The plan is started with no consumer | Phase 0 acceptance has no consumer line | Stop. Log it. Do not proceed to Phase 1 |
| GF-0.2 | Pinned versions have a known regression or incompatible licence | Release notes and licence check recorded in the ADR | Pick another version, or choose another store |
| GF-0.3 | Source docs still contradict the plan | `validate_outlines.py --strict` and a grep for `cmd/hunter` in `db/` | Rewrite or supersede them in the same commit |
| GF-0.4 | The LLM provider is unavailable from the lab network | Phase 3 smoke test cannot reach it | Decide on a local model, or accept that ingestion degrades to "off" |

## Acceptance

- [ ] ADR 0006 accepted, naming a consumer, versions, digests, LLM provider and egress rule.
- [ ] `db/` no longer contradicts the plan.
- [ ] `uv run python .github/scripts/validate_outlines.py --strict` passes.
- [ ] 🔒 HUMAN CHECKPOINT: a person approves the LLM egress and the data classes.

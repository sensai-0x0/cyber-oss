# Phase 6: Read path, analysis only (gated)

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-5.md](Phase-5.md), [../ml-add/Phase-7.md](../ml-add/Phase-7.md) · **Updated:** 2026-11-20

**Depends on:** Phase 5, and the K3 gate in ml-add Phase 7 (campaign identity stable through one ADR revision). **Blocks:** nothing.

## Goal

Analysts, and the shadow scorer from ml-add Phase 4, can query the graph for campaign relationships. No result changes a routing or mitigation decision.

## Scope

1. Read methods behind a feature flag, off by default, calling `search` with the same `group_id`.
2. Output goes to the ledger's `challengers` map and to reports only.
3. A query allowlist. No arbitrary Cypher from any model or operator input.

## Tests first

- Shadow isolation again: with reads on, `GraphState.policy` is identical to reads off.
- A query containing a Cypher fragment is rejected.
- Empty results are reported as "no data" and never as "no risk".

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-6.1 | A stale or poisoned graph result is trusted in a decision | Isolation test | Reads never reach routing. Gate on a new ADR to change that |
| GF-6.2 | Cypher injection through a query built from event text | Rejection test | Parameterised queries from a fixed set |
| GF-6.3 | Slow traversal on a large graph blocks the analysis job | Timeout test | Query timeout, depth limit, result cap |
| GF-6.4 | Operators over-trust graph links, including benign shared-infrastructure edges | Review of reports | Show evidence and the rarity of each shared node. See ml-add K14 on rubber-stamping |

## Acceptance

- [ ] Isolation holds with reads on.
- [ ] 🔒 HUMAN CHECKPOINT: a person approves any use beyond reports.

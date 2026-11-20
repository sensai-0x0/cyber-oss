# ADR 0004: Campaign belief ledger and its safety preconditions

- Status: accepted
- Date: 2026-11-20
- Approved by: the repository owner, through the decisions given in the session that built Phase 0 of `plans/ml-add` (scope, breadth rule, storage). This is a record of that approval, not a substitute for a named reviewer: add the reviewer's name here when one signs off.

## Context

`plans/ml-add/` proposes an evidence ledger, provenance classes and a shadow campaign scorer. Before any of that, the system had to stop trusting producer-supplied hints for breadth, because `synthesize_policy` read `attributes.prefix_length` (default 32) and auto-approved anything it called narrow. A bare `10.0.0.0/8` was auto-approved (SHR-007), and a hostname starting with a digit was classed as an address (SHR-012).

## Decisions

1. **Breadth and kind come from the target alone.** `orchestrator/policy.py: classify_target` parses with `ipaddress` and a hostname grammar. No attribute and no first-character guess is consulted. A target that does not parse becomes `observe` with a reason.
2. **The `/24` boundary is inclusive.** A range whose prefix length is at or below `broad_prefix_threshold` is broad, so a `/24` needs an operator. The key keeps its value and changes meaning from "below" to "at or below". IPv6 uses the new key `broad_prefix_threshold_v6`. Single addresses are never broad, and hostnames are never broad.
3. **Kill switch.** `auto_approve_narrow` in `orchestrator/policy.yaml` defaults to false. While it is false, every `mitigate` decision interrupts for an operator. Even when true, a broad action never skips the operator.
4. **Ledger and campaign state live in Python-owned SQLite files**, not in a new `hunterd` API. `hunterd` keeps owning Badger (I17). Both files are bounded and are rebuildable from the ledger. The ledger is the only source of truth.
5. **A campaign is identified by a stable hash of its OBSERVED indicators** (the cluster indicators), never by the target, so a campaign that moves between targets keeps its identity.
6. **Challengers never write to `GraphState.policy`.** A challenger's output goes only to `ledger.challengers[scorer_id]`. A test enforces this on every phase that adds a scorer.
7. **Accepted order** is the one in `plans/ml-add/README.md`. Phase 5 and Phase 6 stay gated on the Go-side prerequisites it lists.

## Consequences

- More decisions reach a human. That is the safe direction, and it is reversible by reverting the branch.
- The Go side still trusts a caller-supplied `broad` (SHR-001). Python now derives it, but until the Go enforcer does the same, a forged input that bypasses Python is still possible. SHR-001 stays open and blocks Phases 5 and 6.
- Open question: ledger retention. The ledger must not hold real targets in the lab.

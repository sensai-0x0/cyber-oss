---
name: hunt-operator
description: Operate authorized pre-attack hunts from scoped randomized scanning through delta triage and reversible response. Use for hunt planning, campaign investigation, cluster scoring, operator interrupts, and operational handoffs.
---

# Hunt operator

Calls: contract-steward lab-validator mitigation-guardian

## Workflow

1. Record written authorization, exact CIDRs/domains, exclusions, rate limit, time window, and rollback owner. If scope is unclear, stop and use only the lab.
2. Invoke `contract-steward` for payload compatibility and `lab-validator` for preflight evidence when runtime components changed.
3. Start or resume the keyed four-round CIDR cursor. Persist checkpoints and keep event queues bounded; unchanged observations stay out of the AI plane.
4. Review Cluster and Intel outputs with their evidence. Do not treat heuristic confidence alone as attribution or proof of maliciousness.
5. For mitigation, invoke `mitigation-guardian`. Broad actions remain paused until a named operator approves a signed, bounded action.
6. Monitor expiry and revocation. On uncertainty, revoke early and preserve the event, cluster, policy, approval, and backend receipt IDs.

## Handoff

Produce a concise hunt record: authorized scope, cursor/checkpoint, delta counts, clusters and confidence rationale, operator decisions, action expiries, and revocation evidence. Never include secrets or unrelated personal data.

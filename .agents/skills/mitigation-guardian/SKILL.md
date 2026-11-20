---
name: mitigation-guardian
description: Review and operate signed, human-approved, TTL-bounded RPZ and nftables mitigations. Use for policy thresholds, approval requests, action verification, expiration, revocation, false positives, and rollback.
---

# Mitigation guardian

Calls: contract-steward

## Workflow

1. For action schema or signature changes, invoke `contract-steward` before editing.
2. Confirm action ID, exact target, broad/narrow scope, confidence evidence, named operator, creation time, expiry, and TTL. Fail closed if any are absent.
3. Verify Ed25519 signature over the canonical v1 action. Treat a modified target, TTL, approval, or expiry as invalid.
4. Require explicit approval for broad actions. Never widen a target to make enforcement easier.
5. Exercise Apply and Revoke with fake executors first. For lab validation, hand off to `lab-validator`; do not use production firewall state as a test fixture.
6. Track actions until revocation succeeds. Retry failed expiry and alert rather than deleting the record.

## Handoff

Return an allow/observe decision, verification evidence, expiry, revocation status, and rollback owner. `hunt-operator` must not bypass a deny or unresolved verification result.

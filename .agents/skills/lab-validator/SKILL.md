---
name: lab-validator
description: Validate the complete Sentinel Hunter pipeline in the isolated fake-C2, phishing, DNS, and firewall lab. Use after build changes, before operations, and for end-to-end approval or TTL-expiry evidence.
---

**Status:** authoritative · **Derived from:** `spec/`, `schemas/` · **Updated:** 2026-11-21

# Lab validator

Calls: pipeline-builder mitigation-guardian

## Workflow

1. For code or wiring changes, invoke `pipeline-builder` and obtain its test summary. For mitigation scenarios, invoke `mitigation-guardian` and reuse its action checklist.
2. Confirm Compose ports bind to `127.0.0.1` and targets are lab-only. Generate ephemeral certificates; never commit private keys.
3. Run `docker compose -f deploy/compose.yaml config --quiet`, then start only required lab services.
4. Drive fake C2/phishing observations through novelty suppression. Assert first-seen emits, repeat suppresses, mutation emits, and cursor resume has no duplicates.
5. Test low-confidence observation, high-confidence narrow mitigation, broad interrupt, signed approval, and automatic expiry. Verify revocation, not merely an expired timestamp.
6. Stop lab services and retain sanitized logs and test identifiers as evidence.

## Handoff

Return pass/fail evidence for each invariant and any blocked runtime integration. A config-only check is not end-to-end evidence. `hunt-operator` uses this as the operational preflight.

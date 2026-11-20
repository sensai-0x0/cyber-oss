# ADR 0001: Separate hunting, orchestration, approval, and enforcement

- Status: accepted
- Date: 2026-11-21

Use independent `hunterd`, Python `orchestrator`, `operator`, and `enforcerd` boundaries. The operator cannot modify firewall state and the enforcer cannot change an approved payload without invalidating its signature. This makes human review a technical boundary rather than a prompt instruction.

DeltaEvent and MitigationAction use versioned JSON schemas. Badger observation/cursor state is private to hunterd; LangGraph checkpoints are private to orchestration. NATS and NDJSON are transports, not alternate payload contracts.

# State and contracts

## DeltaEvent v1

`schemas/delta_event.v1.json` is the cross-language source of truth. `hunterd/probes.Observation` and `orchestrator.state.DeltaEvent` share required identity, kind, source, target, timestamp, SHA-256 fingerprint, and attributes fields. Only `novel` and `mutated` are valid on the AI boundary; `unchanged` is a local Badger decision and is never published.

Attributes are namespaced by probe semantics and are not signed. Consumers must tolerate unknown attributes within v1. Changing required fields, field meanings, or fingerprint canonicalization requires a new schema version and a recorded decision.

## BadgerDB keys

- `checkpoint/<authorized-scope>`: eight-byte big-endian next Feistel sequence number. No TTL.
- `observation/<probe-key>`: JSON version, hash, payload, first observation, and last seen. Entry TTL is configured by `NewDeltaGate`.

The delta read/compare/write runs in one Badger update transaction. After TTL expiry, a returning observation is novel again.

## LangGraph state

`GraphState` contains the event, cluster, auditable Intel score, policy, and cycle count. Ambiguous events may revisit clustering once. Production must use a durable checkpointer so native `interrupt()` survives process restart.

## MitigationAction v1

`schemas/mitigation_action.v1.json` covers deployable signed actions. Canonical Ed25519 payloads include the exact action ID, kind, target, scope, approval, approver, TTL seconds, creation, and expiry. `Signature` is the only unsigned field. The enforcer retains an action if backend revocation fails, enabling retry and alerting.

An operator rejection is a review result, not a deployable MitigationAction: it has no signature and must never reach an enforcement backend.

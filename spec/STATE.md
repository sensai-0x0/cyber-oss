# State and contracts

**Status:** authoritative · **Derived from:** `schemas/`, `hunterd/store/`, `orchestrator/state.py` · **Updated:** 2026-11-21

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

## Decision log

Dated entries for recorded decisions, most recent last. Format: `YYYY-MM-DD: <what changed>, <old> -> <new>, approved by <who>.`

- 2026-11-21: initial `orchestrator/policy.yaml` defaults for keys `mitigation_confidence`, `broad_prefix_threshold`, `default_ttl_seconds`, `min_approved_ttl_seconds`, `max_approved_ttl_seconds` and `max_refinement_cycles` (values: `git show b06074f:orchestrator/policy.yaml`) approved per the Human Checkpoint in `plans/planv1.0/PHASE0.md` P0.3.
- 2026-11-20: `orchestrator/policy.yaml` gains `auto_approve_narrow` (off) and `broad_prefix_threshold_v6`; `broad_prefix_threshold` is now inclusive, so a `/24` is broad (SHR-007); breadth and kind are derived from the target (SHR-012). Values: `git diff` on the commit that adds this line. Approved by the repository owner in-session, see ADR 0004.
- 2026-11-20: `orchestrator/policy.yaml` gains `badlist_path` and `badlist_sha256`; `load_policy` refuses to load if the pinned lab badlist is missing or edited. Scoring now counts only OBSERVED fields matched against it, and producer-asserted flags add nothing (ml-add Phase 1, ADR 0004). Approved by the repository owner in-session.
- 2026-11-20: `orchestrator/policy.yaml` gains `campaign_half_life_hours`, `campaign_prior`, `campaign_cap_derived`, `campaign_cap_repeat` and `campaign_max_tracked` for the shadow campaign scorer (ml-add Phase 4). They affect only the ledger's challenger map, never a decision. Approved by the repository owner in-session, see ADR 0004.
- 2026-11-20: fleet command path recorded as ADR 0007 (MCP agent to fleetd; signed NATS commands fleetd to hunters; stdio MCP for lab only), none -> accepted, approved by Sunyata in the project thread. No policy values changed.
- 2026-11-20: `orchestrator/policy.yaml` gains `ledger_append_attempts` (3): how many times a mitigation proposal record is appended before the decision degrades to `observe` (ml-add Phase 5, ADR 0005). Approved by the repository owner in-session.
- 2026-11-20: `Policy.target` is the bare target `classify_target` judged (scheme, a valid port and brackets removed, hostnames lower-cased, ranges masked to their network address, IP addresses in canonical form), not the raw `event.target`, so `enforcerd` re-derives breadth from the same string. IPv4-mapped and zoned targets follow `enforcerd.Breadth.Derive`. Integration-gate fix for ADR 0005; no policy values changed.
- 2026-11-20: `enforcerd` refuses expired and future-dated signed actions (`ErrActionExpired`; `MaxClockSkew` in `enforcerd/ttl.go`), caps the tracked deadline at now+TTL, builds an `Enforcer` only over a valid `SignedBackend`, and keeps reaping after a failed revoke (SHR-002, -005, -009, -004; ml-add Phase 6 prerequisites). SHR-003 (persistence across a restart) stays open pending a store decision. No policy values changed.

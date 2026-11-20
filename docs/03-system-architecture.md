# System Architecture

## Service boundaries and why they exist

Sentinel Hunter is split into four independently deployable units, each owning exactly one side of the trust boundary described in [ADR 0001](../spec/DECISIONS/0001-service-boundaries.md):

```text
authorized scope
      |
      v
hunterd (Go): Feistel cursor -> tlsx/httpx -> Badger novelty gate
                                              | novel/mutated only
                                              v
                                   bounded NDJSON / NATS subject
                                              |
                                              v
                    orchestrator (Python/LangGraph): Cluster -> Score -> Policy
                                      ^          |
                                      |__ at most one refinement
                                               |
                                      broad? native interrupt()
                                               |
                                               v
                              operator (Go): named decision + Ed25519
                                               |
                                               v
                           enforcerd (Go): verify -> RPZ/nft -> TTL revoke
```

The reason this is four binaries and not one monolith is specifically to make the human-review boundary a *technical* fact: `orchestrator` has no network reach to a firewall or DNS backend, by construction — not by policy, by the absence of a code path. `enforcerd` cannot be reached with a payload that lacks a valid Ed25519 signature over its own canonicalized fields, so `operator` cannot be bypassed and `orchestrator` cannot forge an approval. If an attacker (or an adversarially prompted LLM inside the triage loop) fully compromises `orchestrator`, the worst it can do is propose a bad policy — it cannot sign, apply, or revoke anything.

## `hunterd` — collection and novelty suppression

- **Entry point**: `hunterd/cmd/hunterd/main.go`. A single binary exposing a `serve` command: a gRPC health endpoint (`grpc.health.v1`) on a configurable listen address, and a backpressure-bounded NDJSON pipe over stdin/stdout. The event channel capacity is an explicit flag (`-event-capacity`, default 256) — a slow consumer blocks the producer rather than letting memory grow unbounded, and every line is schema-validated (`schema_version`, `kind`, `source`, fingerprint format, timestamp skew) before it enters the channel.
- **`hunterd/cursor` (Feistel permutation)**: a four-round balanced Feistel network over the host-bits width of a CIDR, keyed by HMAC-SHA256 per round. `cursor.Range` walks every address in a prefix exactly once, in pseudorandom order, using only the prefix, key, and a `uint64` next-index — O(1) memory regardless of range size, and restartable from a persisted index (`Cursor()`/`Restore()`). See [04-core-mechanisms.md](04-core-mechanisms.md) for the construction detail.
- **`hunterd/store` (BadgerDB)**: owns the embedded KV lifecycle. `SaveCursor`/`LoadCursor` persist the next sequence number per scan scope (no TTL — resumability is permanent). `DeltaGate.Apply` runs a single Badger transaction that reads the prior observation hash for a key, decides `Novel` / `Unchanged` / `Mutated`, and writes the new state back with a caller-supplied TTL — read, compare, and write happen atomically, so there is no window where a concurrent writer could see a stale novelty decision.
- **`hunterd/probes`**: normalizes `projectdiscovery/tlsx` and `projectdiscovery/httpx` results into the shared `Observation` struct (the Go side of `schemas/delta_event.v1.json`). A fingerprint is a SHA-256 over the canonical `{source, target, observed_at, attributes}` JSON; the event ID is the fingerprint's first 32 hex characters. `source: ct` is a reserved, schema-valid value for future Certificate Transparency streaming — no live CT client is wired yet.

## `orchestrator` — triage and policy synthesis (Python, LangGraph)

A `StateGraph` over `GraphState` (`orchestrator/state.py`), wired in `orchestrator/graph.py`:

```
START -> cluster -> intel --[0.3 ≤ confidence < 0.8 and cycles < 2]--> cluster (retry)
                       |
                       --[else]--> mitigate -> approval -> END
```

- **`cluster.py`**: groups events by a SHA-256 over the target plus sorted stable indicators (`jarm`, `ja3`, `favicon_mmh3`, `common_name`) — deliberately simple, deterministic, and inspectable; no external service call.
- **`score.py`**: an auditable, additive confidence score (base 0.1; +0.2 for recent CT activity; +0.45 for a known-bad intel match; +0.1 per shared fingerprint up to 3) with a `rationale` list attached to every score — the number is never produced without the reasons that built it. `route_after_intel` allows exactly one re-cluster cycle for genuinely ambiguous signal (confidence in `[0.3, 0.8)`) and then forces a terminal decision; `cycles` is tracked in graph state specifically so this bound is enforced by the graph, not by convention.
- **`policy.py`**: loads `orchestrator/policy.yaml` (cached, version-checked) as the single source of thresholds. Below `mitigation_confidence` (0.8), the only output is `observe`. At or above it, a `mitigate` policy is synthesized with `broad = prefix_length < broad_prefix_threshold (24)` — i.e., anything wider than a /24 is broad by definition, independent of confidence. Broad policies start `approved: false`; narrow ones start pre-approved. `approve_policy` is the only place `langgraph.types.interrupt()` is called, and only when the policy is both a mitigation and broad — this is the single, structural human gate in the whole graph.
- **Durability note**: `build_graph(checkpointer=None)` accepts any LangGraph checkpointer; the test suite uses the in-memory default. Production use requires a durable checkpointer (e.g. SQLite/Postgres-backed) so an `interrupt()` awaiting a human decision survives a process restart — this is explicitly called out as integration work, not solved by the current graph code.

## `operator` — the signing boundary

`operator/decision.go` is ~40 lines on purpose: `Approve` requires a named approver, a TTL within `(0, 24h]`, sets `CreatedAt`/`ExpiresAt`, and delegates to `enforcerd.SignAction`. `Reject` explicitly clears `Signature` and `ExpiresAt` and never calls the signer — a rejected action is structurally inert, not merely "marked rejected." There is no code path in this package that touches a backend.

## `enforcerd` — verification, application, and expiry

- **`verify_signature.go`**: `SignAction`/`VerifyActionSignature` operate over a *canonical* wire struct (`schema_version`, `action_id`, `kind`, `target`, `broad`, `approved`, `approved_by`, `ttl_seconds`, `created_at`, `expires_at`) built independently of the in-memory `Action` struct's field order or extra state — this is what makes the signature meaningful: it covers exactly the fields that matter for authorization, and recomputing it from a tampered action will not match. `canonicalAction` also re-derives and checks `expires_at == created_at + ttl`, rejecting any action whose stated expiry doesn't match its own TTL.
- **`enforcer.go`**: `Enforcer.Enforce` is the only path into a `Backend` (`nftables`, `rpz`, or `headscale` — `headscale` is a valid `Kind` with no adapter implementation yet). It rejects empty IDs/targets, invalid kinds, out-of-bounds TTL, and any broad-but-unapproved action *before* calling the backend, and serializes the apply-and-record step under a mutex so a duplicate action ID cannot produce two backend rules. `SignedBackend` (in `verify_signature.go`) wraps a `Backend` to re-check broad/approved and verify the signature immediately before `Apply` — defense in depth against a caller that skipped `Enforcer.Enforce`.
- **`ttl.go`**: a single shared `ValidateTTL`, `MaxTTL = 24h`, used identically at signing, approval, and enforcement — one constant, three enforcement points, so a change to the ceiling cannot be made in only one place by accident.
- **Expiry (`Enforce`'s companion `Expire`)**: scans tracked actions for anything due, calls `Backend.Revoke`, and only removes the action from the tracked set on a *successful* revoke. A failed revoke is left in place for retry on the next tick (`Run` drives this on an interval) rather than being dropped — see [05](05-safety-and-trust-model.md) for why this matters.
- **Backends**: `enforcerd/nft` and `enforcerd/rpz` are implemented and tested against fake executors; real nftables/DNS changes require explicit runtime wiring outside ordinary CI (per [ADR 0002](../spec/DECISIONS/0002-reversible-mitigation.md)).

## Contracts

Two versioned JSON Schemas are the cross-language source of truth, mirrored by a Go struct and a Pydantic model each:

- `schemas/delta_event.v1.json` ↔ `hunterd/probes.Observation` ↔ `orchestrator.state.DeltaEvent`. `additionalProperties: false` at the top level; `attributes` is an open, namespaced bag that both sides must tolerate growing. Only `novel`/`mutated` are valid `kind` values on this contract — `unchanged` is a `hunterd`-internal concept that structurally cannot be expressed here.
- `schemas/mitigation_action.v1.json` ↔ `enforcerd.Action`. `signature` is the only field excluded from the canonical signing payload; every other required field is part of what gets signed.

Changing a required field or its meaning requires a new schema version and a recorded ADR — this is owned end-to-end by the `contract-steward` skill (see [06-engineering-process.md](06-engineering-process.md)).

## Transport and deployment topology (lab)

`deploy/compose.yaml` stands up a loopback-only lab: NATS (TLS, locally generated certs, never committed), a harmless fake-C2 fixture, a phishing-panel fixture that rejects all POSTs, loopback CoreDNS with RPZ zone fixtures, and an optional privileged `firewall-target` profile for manual nftables adapter checks. Every host port is bound to `127.0.0.1`. This is a safety-and-development lab, not a production deployment manifest — see [spec/SCOPE.md](../spec/SCOPE.md) and [lab/README.md](../lab/README.md).

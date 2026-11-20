**Status:** authoritative · **Derived from:** `schemas/`, `spec/STATE.md`, `hunterd/`, `orchestrator/`, `enforcerd/`, `operator/` · **Updated:** 2026-11-21

Detailed form of [../spec/STATE.md](../spec/STATE.md). Authority ladder, path rules and provenance: [../.agents/outline-method.md](../.agents/outline-method.md). Universal rules: [../CLAUDE.md](../CLAUDE.md).

## 1. Owners

Four owners, one trust boundary each. No entity is co-owned.

| Owner | Owns exclusively | Never holds |
|---|---|---|
| `hunterd/` | Badger cursors, Badger observation records, `probes.Observation` | Policy, signatures, firewall state |
| `orchestrator/` | `GraphState`, `Cluster`, `IntelScore`, `Policy`, LangGraph checkpoints | Badger state (I17), signing keys, backend access |
| `operator/` | Ed25519 private key, the named decision, the signature | Firewall or DNS state (I7) |
| `enforcerd/` | `Action` deadline set, backend rules | Authority to alter a signed payload |

Badger state is private to `hunterd/`; checkpoints are private to `orchestrator/` ([ADR 0001](../spec/DECISIONS/0001-service-boundaries.md)).

## 2. DeltaEvent v1

`schemas/delta_event.v1.json` is the highest authority on this shape. `additionalProperties` is `false` at the top level; the `attributes` object is open.

| Field | Type | Required | Constraint | Meaning |
|---|---|---|---|---|
| `schema_version` | string | yes | const `v1` | Payload contract version |
| `event_id` | string | yes | length 1–128 | Event identity on the wire |
| `kind` | string | yes | enum `novel`, `mutated` | Novelty verdict that permitted publication |
| `source` | string | yes | enum `tlsx`, `httpx`, `ct` | Probe semantics for `attributes` |
| `target` | string | yes | length 1–2048 | Observed host:port or URL |
| `observed_at` | string | yes | RFC 3339 date-time | Probe completion instant, UTC |
| `fingerprint` | string | yes | `^[0-9a-f]{64}$` | SHA-256 over the canonical form below |
| `attributes` | object | yes | free-form | Probe-namespaced detail, unsigned |

### Two representations

```go
// hunterd/probes/probe.go
type Observation struct {
	SchemaVersion string         `json:"schema_version"`
	EventID       string         `json:"event_id"`
	Kind          string         `json:"kind"`
	Source        string         `json:"source"`
	Target        string         `json:"target"`
	ObservedAt    time.Time      `json:"observed_at"`
	Fingerprint   string         `json:"fingerprint"`
	Attributes    map[string]any `json:"attributes"`
}
```

```python
# orchestrator/state.py
class DeltaEvent(BaseModel):
    model_config = ConfigDict(extra="forbid")
    schema_version: Literal["v1"]
    event_id: str = Field(min_length=1, max_length=128)
    kind: Literal["novel", "mutated"]
    source: Literal["tlsx", "httpx", "ct"]
    target: str = Field(min_length=1, max_length=2048)
    observed_at: datetime
    fingerprint: Annotated[str, Field(pattern=r"^[0-9a-f]{64}$")]
    attributes: dict[str, Any] = Field(default_factory=dict)
```

### Validation points

| Point | Code | Checks | Does not check |
|---|---|---|---|
| Construction | `newObservation` in `hunterd/probes/probe.go` | Sets version, fingerprint, `event_id`, UTC timestamp | Nothing; it cannot fail |
| Egress | `validateEvent` in `hunterd/cmd/hunterd/main.go` | Version, non-empty id and target, kind, source, non-zero timestamp not more than 5 minutes ahead, fingerprint pattern | Field lengths, `attributes` presence or type, target authorization |
| Ingress | `DeltaEvent.model_validate` in `orchestrator/cluster.py`, `score.py`, `policy.py` | Every schema constraint, plus `extra="forbid"` | Target authorization |
| Test gate | `orchestrator/tests/test_schemas.py` | Python dump validates against the JSON Schema | The Go struct against the schema |

Go enforces a subset of the schema and no Go test validates `Observation` against it, so Python is the real gate. Three divergences are live and unresolved:

- **`kind` is never set to `mutated` in Go.** `newObservation` hardcodes `"novel"`, and nothing copies a `store.Decision.Kind` of `Mutated` onto the `Observation`. The mutated verdict exists only in `hunterd/store/badger_store.go`.
- **Length bounds are Python-only.** Go accepts an `event_id` or `target` beyond the schema maximum; Python rejects it.
- **A nil `Attributes` map marshals to `null`.** `validateEvent` permits it; the schema and the Pydantic model both reject it.

### Fingerprint canonicalization

`newObservation` marshals an anonymous struct of `source`, `target`, `observed_at` (normalized to UTC) and `attributes`, in that field order, then takes SHA-256 and hex-encodes it. `event_id` is the first 32 hex characters of that digest. Because `observed_at` participates, the fingerprint is per-observation, not per-asset: re-probing an unchanged host yields a new fingerprint, so cross-time identity belongs to the Badger observation record rather than to `fingerprint`. The marshal error is discarded, so an unmarshalable attribute degrades to a digest of an empty canonical form. Changing this canonicalization requires a new schema version and a recorded decision.

### The novelty rule

`novel` and `mutated` are the only kinds the schema, `validateEvent` and the Pydantic model admit. `unchanged` is a `DeltaKind` constant inside `hunterd/store/badger_store.go` with `Emit: false` and a nil payload: a local Badger decision, never published across the AI boundary.

## 3. MitigationAction v1

`schemas/mitigation_action.v1.json` governs deployable signed actions.

| Field | Type | Required | Constraint | Meaning |
|---|---|---|---|---|
| `schema_version` | string | yes | const `v1` | Contract version |
| `action_id` | string | yes | length ≥ 1 | Action identity, unique in the enforcer set |
| `kind` | string | yes | enum `nftables`, `rpz`, `headscale` | Backend that must apply it |
| `target` | string | yes | length ≥ 1 | CIDR, address or DNS name |
| `broad` | boolean | yes | — | Requires a named operator when true |
| `approved` | boolean | yes | — | Decision outcome |
| `approved_by` | string | no | — | Operator name; required in practice when `broad` |
| `ttl_seconds` | integer | yes | 1 .. the 24-hour ceiling | Lifetime; no permanent rules |
| `created_at` | string | yes | RFC 3339 date-time | Decision instant |
| `expires_at` | string | yes | RFC 3339 date-time | Must equal `created_at + ttl_seconds` |
| `signature` | string | yes | base64 | Ed25519 over the canonical payload |

The Go in-memory form is `enforcerd.Action` in `enforcerd/enforcer.go`. It carries `TTL` as a `time.Duration` and has no `schema_version` field; the version is supplied by the canonical wire struct in `enforcerd/verify_signature.go` and by `wireAction` in `operator/cmd/operator/main.go`. A sub-second `TTL` passes `ValidateTTL` but truncates to zero in `ttl_seconds`, which the schema rejects.

### Canonical signed payload

`canonicalAction` marshals exactly these fields, in this order, `approved_by` omitted when empty:

```
schema_version, action_id, kind, target, broad, approved,
approved_by?, ttl_seconds, created_at (UTC), expires_at (UTC)
```

`signature` is the only field excluded from the signature — it cannot sign itself. `enforcerd` therefore cannot alter an approved payload without invalidating it. Expiry is recomputed and must equal `created_at + ttl`, so TTL and `expires_at` cannot disagree inside a valid signature.

### Rejection is not an action

`operator.Reject` in `operator/decision.go` sets `Approved: false`, records `ApprovedBy`, zeroes `ExpiresAt` and clears `Signature`. The result has no signature, so `SignedBackend.Apply` fails closed on it. A rejection is a review result and must never reach an enforcement backend ([ADR 0002](../spec/DECISIONS/0002-reversible-mitigation.md)).

## 4. BadgerDB keys

| Key pattern | Value shape | TTL | Owner | Transaction |
|---|---|---|---|---|
| `checkpoint/<scope>` | 8-byte big-endian `uint64` sequence number | none | `hunterd/store/badger_store.go` | Written in `SaveCursor` (`db.Update`), read in `LoadCursor` (`db.View`) |
| `observation/<key>` | JSON `{version, sha256, payload, observed_at, last_seen}` | `NewDeltaGate` argument | `hunterd/store/badger_store.go` | Read, compared and written in one `db.Update` inside `DeltaGate.Apply` |

`DeltaGate.Apply` performs get, hash comparison and `SetEntry(...).WithTTL(ttl)` in a single Badger update transaction, so two concurrent observers cannot both see a key as novel. `Decision.Payload` is a copy of the caller's slice, verified by `hunterd/store/badger_store_test.go`. An `unchanged` verdict still rewrites the record to refresh `last_seen` and the TTL.

`<key>` is supplied by the caller; no code derives it from an `Observation`, and the `tls/<ip>:<port>` form in the tests is a fixture, not a contract. `<scope>` is an opaque non-empty string; nothing validates that it names a real authorized scope, because no scope entity exists (section 8). Badger permits **one process per directory** (I17): only `hunterd` opens the store, and the orchestrator never reads it.

## 5. LangGraph state

`GraphState` in `orchestrator/state.py` is a `TypedDict(total=False)`:

| Member | Type | Written by | Read by |
|---|---|---|---|
| `event` | `DeltaEvent` | graph input | `cluster_event`, `score_threat_intel`, `synthesize_policy` |
| `cluster` | `Cluster` (`cluster_id`, `indicators`, `event_ids`) | `cluster_event` | — |
| `intel` | `IntelScore` (`confidence`, `rationale`) | `score_threat_intel` | `route_after_intel`, `synthesize_policy`, `approve_policy` |
| `policy` | `Policy` (partial) | `synthesize_policy`, `approve_policy` | `approve_policy` |
| `cycles` | int | `cluster_event` (increments) | `route_after_intel` |

`orchestrator/graph.py` wires `START → cluster → intel`, a conditional edge from `intel` to either `cluster` or `mitigate`, then `mitigate → approval → END`. `route_after_intel` in `orchestrator/score.py` returns to `cluster` only while confidence is ambiguous — at or above `AMBIGUOUS_FLOOR` and below `policy.yaml: mitigation_confidence` — and the cycle count is under `policy.yaml: max_refinement_cycles`, so clustering is revisited a bounded number of times and the graph always terminates in a policy decision. `orchestrator/tests/test_graph.py` gates that bound.

`approve_policy` writes `policy` through `apply_decision`, which returns a new mapping rather than mutating the state's policy in place, so an interrupted run's recorded proposal is not altered by the decision that follows it.

`build_graph(checkpointer=None)` compiles without durability. Production must supply a durable checkpointer so a native `interrupt()` survives process restart; `MemorySaver` is test-only.

## 6. Policy configuration

`orchestrator/policy.yaml` is the single source for runtime thresholds. `load_policy` in `orchestrator/policy.py` is `lru_cache`d and refuses any `version` other than `v1`, so an edit requires a restart.

| Key | Meaning | Consumer |
|---|---|---|
| `version` | Config contract version | `load_policy` |
| `mitigation_confidence` | Confidence floor below which the policy is `observe`, and the top of the ambiguous band | `synthesize_policy`, `route_after_intel` |
| `broad_prefix_threshold` | Prefix length at or below which an action is `broad` | `synthesize_policy` |
| `default_ttl_seconds` | Proposed TTL, and the fallback when a decision omits one | `synthesize_policy`, `approve_policy` |
| `min_approved_ttl_seconds` | Lower clamp on an approved TTL | `approve_policy` |
| `max_approved_ttl_seconds` | Upper clamp, and `max_ttl_seconds` in the interrupt payload | `approve_policy` |
| `max_refinement_cycles` | Refinement cycle bound | `route_after_intel` |

Every declared key now has a consumer, verified by grepping `orchestrator/score.py` and `orchestrator/policy.py` on 2026-11-21. `max_refinement_cycles` was declared and unread until that same day, when `route_after_intel` was changed to read it along with `mitigation_confidence`; any outline asserting it is unused is stale.

One threshold still lives outside the file that is meant to hold all of them: the ambiguity floor below which refinement cannot change the outcome is the module constant `AMBIGUOUS_FLOOR` in `orchestrator/score.py`, which the code itself records as having no `policy.yaml` key yet. Cite it by name; it needs a key before any other consumer depends on it.

## 7. Lifetimes and expiry

| Entity | Lifetime | Enforced by |
|---|---|---|
| `observation/<key>` | `NewDeltaGate` TTL | Badger entry TTL |
| `checkpoint/<scope>` | Indefinite | None; overwritten in place |
| `GraphState` | Thread lifetime, durable only with a checkpointer | LangGraph checkpointer |
| Interrupt pause | Until resumed with a decision | LangGraph checkpointer |
| Signature validity | Bound to `created_at`/`expires_at` inside the payload | `canonicalAction` |
| Backend rule | `created_at + TTL`, ceiling `MaxTTL = 24h` | `ValidateTTL`, `Enforcer.Expire` |

`Enforcer.Expire` revokes every action due at the given instant, in sorted id order, and deletes the tracked entry only after `Backend.Revoke` succeeds. A failed revocation returns early and leaves the action in the set, so it stays visible and retryable rather than silently becoming permanent.

After an observation TTL expires the key is gone, so a returning observation is `novel` again. That is intended: an asset unseen for longer than the retention window is newly discovered.

## 8. Absent entities

**Authorized scope has no representation anywhere in the code.** No Go file parses configuration or holds a target list; `checkpoint/<scope>` stores an opaque string and never resolves it to an allowlist.

Invariants that require the entity — [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md) I1 through I5 — demand a Go-side allowlist checked before dialing, per-resolved-IP checks, subset rather than overlap semantics, refusal to start on an empty list, and coverage limited to written authorization. None has an enforcement point. See [ADR 0003 consequence 4](../spec/DECISIONS/0003-architecture-authority.md). The entity the superseded layout called `mission.yaml` has no owner and is not re-homed here. Also absent, and not to be read as existing:

| Entity | State |
|---|---|
| Scope allowlist / rate-limit budget | Absent. Highest-priority outstanding work per ADR 0003 |
| NATS subject, consumer and durable cursor | Transport declared in `deploy/compose.yaml`; no Go or Python client (planned) |
| `orchestrator` `Policy` to `MitigationAction` bridge | Absent. `Policy` carries no `created_at`, `expires_at`, `schema_version` or `signature` |
| Enforcer daemon, its persisted action set and reaper key | Absent. `enforcerd/` is a library; the tracked set is in-process only (planned) |
| CT source state and checkpoint | `source: ct` is contract-valid; no CT session or cursor exists (planned) |

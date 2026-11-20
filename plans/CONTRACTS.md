**Status:** authoritative · **Derived from:** `schemas/`, `spec/STATE.md`, `hunterd/`, `orchestrator/`, `enforcerd/`, `operator/` · **Updated:** 2026-11-21

Interface contracts at each boundary. Payload shapes: [DATA_MODEL.md](DATA_MODEL.md). Authority ladder: [../.agents/outline-method.md](../.agents/outline-method.md). Universal rules: [../CLAUDE.md](../CLAUDE.md). Every signature is transcribed from the source named above it; an interface marked absent has no implementation to call. Section 10 is the single home for error classes and which of them an operator can correct.

## 1. Producer to store

```go
// hunterd/store/badger_store.go
func Open(path string) (*Store, error)
func (s *Store) Close() error
func (s *Store) SaveCursor(scope string, cursor uint64) error
func (s *Store) LoadCursor(scope string) (uint64, error)

func NewDeltaGate(store *Store, ttl time.Duration) *DeltaGate
func (g *DeltaGate) Apply(
	key string,
	hash [sha256.Size]byte,
	payload []byte,
	observedAt time.Time,
) (Decision, error)

type Decision struct {
	Kind    DeltaKind // "novel" | "unchanged" | "mutated"
	Emit    bool
	Payload []byte
}
```

Delta-gate semantics, decided inside one `db.Update` transaction:

| Stored state | Verdict | `Emit` | `Payload` |
|---|---|---|---|
| Key absent | `novel` | true | copy of the caller's slice |
| Key present, stored hash equals `hash` | `unchanged` | false | nil |
| Key present, stored hash differs | `mutated` | true | copy of the caller's slice |

The consumer may assume the verdict is atomic against concurrent writers, the returned payload does not alias caller memory, the record's TTL is refreshed on every verdict including `unchanged`, and a missing key after TTL expiry yields `novel` again.

The consumer may **not** assume that `key` means anything to the store — it is opaque and caller-derived, with no implemented naming convention; that `LoadCursor` distinguishes "never saved" from "saved as zero", since both return `0, nil`; or that the gate rewrites an `Observation`'s `kind` field, which it does not. Badger allows one process per directory (I17), so `Open` on a directory another process holds fails rather than sharing.

## 2. Cursor

```go
// hunterd/cursor/feistel.go
func Permute(value uint32, bits uint, key [32]byte) uint32
func Unpermute(value uint32, bits uint, key [32]byte) uint32

func NewRange(cidr string, key [32]byte) (*Range, error)
func (r *Range) Next() (netip.Addr, bool)
func (r *Range) Cursor() uint64
func (r *Range) Restore(cursor uint64) error
```

- **Bijection over the prefix.** Four balanced Feistel rounds over the host-bit domain; `Unpermute` inverts `Permute` for the same `bits` and `key`. Every address in the CIDR is visited exactly once before `Next` returns false (I15), gated by `hunterd/cursor/feistel_test.go`.
- **O(1) memory.** `Range` retains only the masked prefix, the key, the host-bit width and the next sequence number. No address list is materialized or persisted.
- **Resumability.** `Cursor()` is the serializable position. `Restore(c)` accepts `0 <= c <= 2^hostBits`, where `c` equal to the range size is a legal exhausted state.
- **IPv4 only.** `NewRange` rejects a non-IPv4 prefix.

On restart a caller must `NewRange` with the *same* CIDR and key, `LoadCursor` the persisted sequence number, and `Restore` it before the first `Next`. A different key yields a different permutation of the same set, so positions are not comparable across keys; skipping `Restore` rescans from zero, which I16 forbids. `Permute` and `Unpermute` **panic** outside the domain — they are internal primitives and must not receive unchecked input.

## 3. Probe runners

```go
// hunterd/probes/tls_runner.go
func NewTLSRunner(timeout time.Duration) (*TLSRunner, error)
func (r *TLSRunner) Probe(ctx context.Context, host, ip, port string) (Observation, error)
func (r *TLSRunner) Close()

// hunterd/probes/http_runner.go
func NewHTTPRunner(timeout time.Duration) *HTTPRunner
func (r *HTTPRunner) Probe(ctx context.Context, target string) (Observation, error)
```

Both return a fully formed `Observation` with `schema_version` `v1`, `kind` `novel`, `source` set to `tlsx` or `httpx`, a normalized UTC `observed_at`, the computed `fingerprint` and `event_id`, and probe-namespaced `attributes`. `TLSRunner` sets `target` to `net.JoinHostPort(host, port)` from the response; `HTTPRunner` sets it to the response URL. Each `Probe` call makes one active connection and returns the first successful result only. Cancellation is cooperative: the HTTP runner calls `client.Interrupt()`, while the TLS upstream takes no context, so cancellation is checked only before the bounded-timeout call. `TLSRunner.Close` releases the fast dialer and must be called; `HTTPRunner` holds no long-lived resource.

**Precondition: the target must already be authorized — and this precondition is unchecked in code.** `TLSRunner.Probe` calls `service.Connect(host, ip, port)` directly; `HTTPRunner.Probe` passes `target` into `runner.Options.InputTargetHost` and calls `RunEnumeration()`. The only guard either applies is `ctx.Err()`. A caller cannot rely on the runner to enforce scope, and must not treat a successful return as evidence of authorization. See [ADR 0003 consequence 4](../spec/DECISIONS/0003-architecture-authority.md) and I1 through I5 in [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md). Until a scope owner lands, active probing belongs in `lab/`.

## 4. hunterd to orchestrator transport

Line contract: one DeltaEvent v1 JSON object per line, newline-delimited, no trailing content. `hunterd serve` reads NDJSON on stdin and writes validated NDJSON on stdout; logs go to stderr.

```go
// hunterd/cmd/hunterd/main.go
func runNDJSON(ctx context.Context, input io.Reader, output io.Writer, capacity int) error
func validateEvent(event probes.Observation) error
```

`validateEvent` checks `schema_version` equals `v1`; `event_id` and `target` non-empty; `kind` in {`novel`, `mutated`}; `source` in {`tlsx`, `httpx`, `ct`}; `observed_at` non-zero and no more than five minutes ahead; `fingerprint` matching `^[0-9a-f]{64}$`.

It does **not** check field length bounds, `attributes` presence or type, that the fingerprint digests the payload, or — most consequentially — whether the target was authorized. It validates event *shape*, never authorization. A consumer needing the full schema must revalidate, as `orchestrator/` does through its Pydantic model.

Backpressure: the producer goroutine decodes and validates each line, then blocks on a channel of capacity `-event-capacity` (default 256) or aborts on cancellation. Nothing is dropped; a slow consumer stalls the producer; the scanner's maximum line is 1 MiB. A decode or validation failure terminates the stream rather than skipping the line, so a malformed line is fatal.

NDJSON and NATS are transports, not alternate payload contracts ([ADR 0001](../spec/DECISIONS/0001-service-boundaries.md)). The NATS subject and durable consumer are not implemented: `deploy/compose.yaml` runs a broker that no client connects to (planned).

## 5. orchestrator internal

```python
# node contract — orchestrator/cluster.py, score.py, policy.py
def node(state: GraphState) -> dict[str, Any]: ...
```

A node takes the whole state and returns **only the keys it changed**; an empty dict means no change, as `approve_policy` returns for a narrow action. A node never communicates by mutating state in place — `apply_decision` in `orchestrator/policy.py` returns a new policy mapping rather than updating the one in state — and it revalidates `state["event"]` through `DeltaEvent.model_validate` rather than trusting the dict.

```python
# orchestrator/score.py
def route_after_intel(state: GraphState) -> str  # "cluster" | "mitigate"

# orchestrator/policy.py
def apply_decision(policy: Policy, decision: dict[str, Any], config: dict[str, Any]) -> Policy
```

`route_after_intel` returns `cluster` only while confidence is ambiguous — at or above the module constant `AMBIGUOUS_FLOOR` and below `policy.yaml: mitigation_confidence` — and the cycle count is under `policy.yaml: max_refinement_cycles`; otherwise `mitigate`. The router returns a key of the mapping declared in `orchestrator/graph.py`; it does no work and writes nothing.

```python
# interrupt payload — orchestrator/policy.py
interrupt({
    "type": "approval_required",
    "action_id": str,
    "target": str,
    "confidence": float,
    "max_ttl_seconds": int,   # policy.yaml: max_approved_ttl_seconds
})
```

A human decision must resume with `Command(resume={"approved": bool, "ttl_seconds": int})`. `approved` is coerced with `bool(decision.get("approved", False))`, so an absent key is a rejection — the default is fail-safe. On approval `ttl_seconds` is clamped between `policy.yaml: min_approved_ttl_seconds` and `policy.yaml: max_approved_ttl_seconds`, defaulting to `policy.yaml: default_ttl_seconds` when absent. On rejection the policy becomes `observe` with a zero TTL and a recorded reason. A broad action always pauses here (I14). The caller must supply a `thread_id` in `config["configurable"]` and a durable checkpointer, or the pause does not survive restart. The resume value is operator input, treated as hostile (I8): the Python clamp is a routing convenience, never the guard, and `enforcerd` revalidates every bound (I7).

## 6. orchestrator to operator

A proposed `Policy` is signable only once it carries `action_id`, `kind`, `target`, `broad`, `approved`, and a TTL within bounds. `Policy` in `orchestrator/state.py` supplies those but has no `created_at`, `expires_at`, `schema_version` or `signature`, so it is a *proposal*, not a MitigationAction; the operator supplies the missing four. A policy whose `action` is `observe` is not a signing candidate at all. No transport implements this hop: `operator review` reads a `wireAction` JSON object on stdin (`operator/cmd/operator/main.go`), and nothing in `orchestrator/` emits one (planned).

## 7. operator to enforcerd

```go
// operator/decision.go
func Approve(action enforcerd.Action, privateKey ed25519.PrivateKey,
	name string, ttl time.Duration, now time.Time) (enforcerd.Action, error)
func Reject(action enforcerd.Action, _ ed25519.PrivateKey,
	name string, now time.Time) (enforcerd.Action, error)

// enforcerd/verify_signature.go
func SignAction(privateKey ed25519.PrivateKey, action *Action) error
func VerifyActionSignature(publicKey ed25519.PublicKey, action Action) error

// enforcerd/ttl.go
const MaxTTL = 24 * time.Hour
func ValidateTTL(ttl time.Duration) error
```

`Approve` requires a non-empty `name` and a TTL in `0 < ttl <= MaxTTL`, sets `Approved`, `ApprovedBy`, `CreatedAt` (UTC) and `ExpiresAt = CreatedAt + ttl`, then signs. `Reject` records the name, clears the signature and zeroes `ExpiresAt`. Canonicalization is the field list and order in `canonicalAction`, with `signature` excluded. Verification is fail-closed: a wrong public-key length, a non-base64 or wrong-length signature, an inconsistent expiry, or any field change all fail before a backend is reached. `SignedBackend.Apply` additionally rejects a broad action lacking `Approved` or a non-empty `ApprovedBy`.

```go
// enforcerd/enforcer.go
func NewEnforcer(backend Backend) *Enforcer
func (e *Enforcer) Enforce(ctx context.Context, action Action) (Receipt, error)
func (e *Enforcer) Expire(ctx context.Context, now time.Time) ([]string, error)
func (e *Enforcer) Run(ctx context.Context, interval time.Duration) error
```

`Enforce` validates identity and kind, validates the TTL, refuses a broad unapproved action, defaults `CreatedAt`, recomputes `ExpiresAt` and rejects a supplied value that disagrees, then takes the lock, rejects a duplicate id, and calls `Backend.Apply` *before* recording the action. The consumer may assume one backend rule per id and a `Receipt` carrying the deadline. `Run` requires a positive interval. The tracked set is in-process only, so no action survives a restart (planned).

## 8. enforcerd to backends

```go
// enforcerd/enforcer.go
type Backend interface {
	Apply(context.Context, Action) error
	Revoke(context.Context, Action) error
}
```

`enforcerd/nft/backend.go`, `enforcerd/rpz/backend.go` and `SignedBackend` all satisfy it. `Revoke` must undo the corresponding `Apply`; backends keep `Apply` bounded and idempotent.

**No shell interpolation.** Each backend injects an `Executor` that invokes an executable directly through `exec.CommandContext` with an argument vector — never a shell string. `nft` additionally requires `Table` and `Set` to match `^[a-zA-Z0-9_]+$` and the target to parse as `netip.Prefix` or `netip.Addr`. `rpz` requires `Zone`, `Server` and the target to match its DNS name pattern, revalidates the TTL against `MaxTTL`, and passes the `nsupdate` script on stdin. An invalid identifier or name is refused before any process starts. `lab/` and the backend tests use fake executors; real changes need explicit runtime wiring ([ADR 0002](../spec/DECISIONS/0002-reversible-mitigation.md)).

**Retained on failed revocation.** `Expire` deletes the tracked entry only after `Revoke` returns nil. A failed revocation returns early with the ids revoked so far plus the error, and the failing action stays in the set for a later pass. A rule is never silently permanent, and a failure is never reported as successful expiry.

## 9. Versioning and compatibility

| Change | Verdict |
|---|---|
| New key inside `attributes` | Additive-compatible. Consumers must tolerate unknown attributes within v1 |
| New optional top-level field | **Breaking.** `additionalProperties: false` and Pydantic `extra="forbid"` both reject it |
| New `source` or `kind` member | Breaking; both sides enumerate them |
| New `MitigationAction` field | Breaking; changes the canonical payload and invalidates every signature |
| Reordering canonical fields | Breaking, silently — signatures stop verifying |
| Change to fingerprint canonicalization | Breaking; requires a new schema version |
| Narrowing a length or pattern constraint | Breaking for existing producers |

A new schema version is required for any change to required fields, field meanings, the canonical signed payload, or fingerprint canonicalization, and must ship with a recorded decision in `spec/DECISIONS/`. Versioned files live beside the current ones, and `schema_version` is a `const`, so a consumer dispatches on it rather than guessing. Start any such change with the `contract-steward` skill.

## 10. Error contract

| Boundary | Stable classes | Operator-correctable |
|---|---|---|
| Store | empty key/scope, non-positive TTL, zero time, `invalid checkpoint encoding`, wrapped Badger error (`open badger: %w`, `apply delta gate: %w`, `decode observation: %w`) | all but the Badger errors |
| Cursor | `parse CIDR: %w`, `only IPv4 CIDRs are supported`, `cursor %d exceeds range size %d`; **panic** outside the permutation domain | the three errors; the panic is a caller bug |
| Probe | `ctx.Err()`, `tlsx probe: %w`, `initialize tlsx: %w`, `initialize httpx: %w`, `httpx returned no successful result` | cancellation and initialization only |
| Transport | `decode NDJSON: %w`, invalid identity/kind/source/time/fingerprint, `encode NDJSON: %w`, `event capacity must be positive` | all |
| Orchestrator | `unsupported policy version`, Pydantic `ValidationError`, missing state key (`KeyError`) | the first two |
| Operator | operator name required, `ErrInvalidTTL`, signing failure | the first two |
| Enforcer | `ErrInvalidAction`, `ErrInvalidTTL`, `ErrApprovalRequired`, `ErrDuplicateAction` | all |
| Signature | invalid public key, invalid signature encoding, verification failed, expiry/TTL mismatch | expiry/TTL mismatch only |
| Backend | unsafe identifiers, invalid target or name, `ErrInvalidTTL`, executor failure | all but executor failure |

The four `enforcerd` sentinels are the only exported error values in the system; everything else is an opaque `errors.New` or a wrapped cause, so a consumer matches on the sentinels and treats the rest as terminal.

No boundary returns a partial success. `Enforce` either records an applied action or returns an error having applied nothing it tracks; `Expire` returns the ids it did revoke alongside the error that stopped it, and the remainder stay tracked for the next pass.

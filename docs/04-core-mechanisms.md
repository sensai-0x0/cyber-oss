# Core Mechanisms — Engineering Deep Dive

This file goes one level deeper than [03-system-architecture.md](03-system-architecture.md) into the algorithms that make the four invariants — bounded memory, no duplicate/skipped scanning, no unsigned enforcement, no permanent mitigation — hold under test, not just in design intent.

## 1. Keyed Feistel permutation (`hunterd/cursor/feistel.go`)

**Problem it solves**: scanning a CIDR in ascending order is a signature (trivially fingerprinted by anything watching for scan traffic) and scanning in fully random order either requires materializing and shuffling the address list (O(n) memory, bad for a /8) or a PRNG without a bijection guarantee (risk of skipped or duplicated addresses).

**Construction**: a 4-round balanced Feistel network over `bits` = host-bits-wide domain:

```go
func Permute(value uint32, bits uint, key [32]byte) uint32
```

- Split `value` into `left` (`(bits+1)/2` bits) and `right` (`bits/2` bits).
- Each round: `left, right = right, left XOR F(key, round, domainBits, leftBits, right, rightBits)`, where `F` is `HMAC-SHA256(key, [round, domainBits, outputBits, inputBits, value])` truncated to 4 bytes. The round function's domain parameters are mixed into the MAC input specifically so that the same `key` produces a *different* permutation at a different prefix width — reusing a key across CIDR sizes does not leak cross-range structure.
- `Unpermute` runs the same rounds in reverse, confirming the construction is invertible (i.e., a true bijection, not merely "looks random").
- **Odd bit widths**: `leftBits`/`rightBits` swap each round and are independently sized (`(bits+1)/2` vs `bits/2`), so an odd-width domain (e.g. a `/23` with 9 host bits) doesn't need cycle-walking — the construction handles non-power-of-two-friendly widths directly. (Note: `.agents/orchestration/interfaces.md`, an earlier design note, proposed cycle-walking as a fallback; the shipped implementation avoids needing it.)

**`Range`** (the resumable iterator) stores only `prefix`, `key`, `hostBits`, `next`, `count` — four scalars and a 32-byte key, regardless of whether the range is a `/30` or a `/8`. `Cursor()` returns `next` for persistence; `Restore(cursor)` resumes exactly there. Combined with `hunterd/store.SaveCursor`/`LoadCursor`, a crash mid-scan loses at most the unpersisted tail since the last checkpoint — correctness invariant E01 (bijection, zero duplicates, deterministic per-seed, non-deterministic across seeds) is covered by property-style tests across `/16`, `/23`, `/24`, `/31`, `/32` prefixes.

## 2. The novelty/delta gate (`hunterd/store/badger_store.go`)

**Problem it solves**: the AI triage plane (LangGraph, running an LLM) should see as little volume as possible, both for cost and because every event it sees is an event that could theoretically be adversarial input (see I9/E05 in [05](05-safety-and-trust-model.md)). Most scan results on any given pass are *identical* to the last pass.

**Construction**: `DeltaGate.Apply(key, hash, payload, observedAt)` runs inside one `badger.Update` transaction:

1. Look up `observation/<key>`. Not found → `Novel`, write the new record with the caller-supplied TTL.
2. Found, hash matches → `Unchanged`, `Emit: false`, bump `LastSeen` only (payload is not carried forward for an unchanged record — nothing new to report).
3. Found, hash differs → `Mutated`, `Emit: true`, overwrite with the new hash/payload, reset `ObservedAt`.
4. The write always carries `WithTTL(ttl)` — after the TTL elapses, Badger expires the key, and a *returning* observation is `Novel` again, not `Mutated`. This is a deliberate choice: "this host went away and came back" is treated as new-to-us, not as a continuation of history we no longer have evidence for.

Because steps 1–4 are one transaction, there is no interleaving where two probes of the same key race to different verdicts. `Unchanged` decisions never leave this function with `Emit: true` — this is the mechanism, not a filter applied later, that keeps "unchanged" off the AI boundary entirely (`spec/STATE.md`).

## 3. Clustering and scoring (`orchestrator/cluster.py`, `orchestrator/score.py`)

Clustering is intentionally *not* ML-driven at this stage: `cluster_id = SHA256(target + sorted([jarm, ja3, favicon_mmh3, common_name present]))[:16]`. The point is determinism and auditability over fitting power — two observations with the same stable TLS/HTTP fingerprint attributes always land in the same cluster, and *why* is always reconstructible from the indicator list stored alongside the ID.

Scoring is additive and bounded: a base of 0.1, and bounded contributions for `recent_ct` (+0.2), `known_bad` (+0.45), and up to three `shared_fingerprints` (+0.1 each), capped at 0.99 (never 1.0 — the system does not model absolute certainty). `rationale` is a parallel list of human-readable strings, one per contribution that fired — every score that reaches a policy decision or a human approval screen carries its own explanation; there is no step where a bare float crosses into a decision.

**Bounded refinement**: `route_after_intel` sends state back to `cluster` only when `0.3 ≤ confidence < 0.8` *and* `cycles < 2`. This single line encodes two product decisions at once: genuinely ambiguous signal gets one more look with (potentially) more clustered evidence, but the system has a hard ceiling on how long it will deliberate before a human sees *something* — it cannot loop on uncertainty indefinitely, which matters both for latency and for bounding the LLM's potential "think time" against adversarial input.

## 4. Policy synthesis and the human-gate interrupt (`orchestrator/policy.py`)

`policy.yaml` is loaded once (`lru_cache`) and version-checked (`version: v1` — a version bump without matching loader logic fails loudly rather than silently misreading stale config). Two thresholds drive everything:

- `mitigation_confidence` (0.8): below it, `synthesize_policy` short-circuits to `{action: observe, approved: false, ttl_seconds: 0}` — there is no code path from "not confident enough" to "do something anyway."
- `broad_prefix_threshold` (24): `broad = prefix_length < 24`. This is a *topological* definition of "broad," independent of confidence — a /23 at confidence 0.99 is still broad and still interrupts. Confidence answers "is this real;" breadth answers "how much collateral could this touch if we're wrong," and the system deliberately does not let a high score buy its way out of the breadth gate.

`approve_policy` calls `interrupt()` only for `action == "mitigate" and broad`. The payload handed to the human is the *exact* thing that will be signed if approved: `action_id`, `target`, `confidence`, and the policy's `max_approved_ttl_seconds` ceiling — there is no separate "preview" representation that could drift from what's actually enforced. On resume: a `False`/missing `approved` flips the policy to `observe` with `ttl_seconds: 0` and a `reason` string; an approval clamps the requested TTL into `[min_approved_ttl_seconds, max_approved_ttl_seconds]` (60s–86400s) regardless of what was requested — the human can shorten or lengthen a window, but never past the policy's own ceiling.

## 5. Canonical signing (`enforcerd/verify_signature.go`)

**Problem it solves**: a signature is only as good as the thing it's computed over. If the Go `Action` struct's JSON tags or field order could drift between signing and verification, or if an unsigned field (say, a free-text `reason`) could be edited after signing without invalidating the signature, the signature would be theater.

**Construction**: `canonicalAction` builds a *separate*, intentionally minimal wire struct — `schema_version` (hardcoded `"v1"`), `action_id`, `kind`, `target`, `broad`, `approved`, `approved_by`, `ttl_seconds` (derived as `int64(TTL/time.Second)`, not copied from a possibly-stale field), `created_at`, `expires_at` (both normalized to UTC) — and marshals *that* to JSON for both signing and verification. Before building it, the function independently re-derives `expires_at` from `created_at + TTL` and rejects the action outright if a stored `ExpiresAt` disagrees. This means: you cannot change `target`, `ttl`, `approved`, or the computed expiry on a signed action without the signature failing to verify — not because the application layer checks those fields separately, but because they are literally inside the signed bytes.

`SignedBackend.Apply` performs the broad/approved check *again*, immediately before delegating to the real backend, even though `Enforcer.Enforce` already checked it — this is deliberate defense-in-depth for any future caller that reaches a `Backend` directly without going through `Enforcer`.

## 6. TTL lifecycle and crash-safe expiry (`enforcerd/enforcer.go`, `ttl.go`)

A single `ValidateTTL`/`MaxTTL` (24h) is called at three independent points — signing (`canonicalAction`), enforcement (`Enforcer.Enforce`), and (per `spec/STATE.md`) the Badger record written before a rule is installed — so there is exactly one place to change the ceiling and no code path that silently bypasses it. `Enforcer.Expire` computes the due set, sorts action IDs for deterministic ordering, and only removes an action from the tracked set *after* `Backend.Revoke` succeeds; a failing revoke leaves the action tracked for the next tick rather than being swallowed. Combined with I12/I13 (Badger record written before install; a reaper that runs on startup and on a timer), a crash between "recorded" and "installed," or between "expired" and "revoked," always resolves toward *removing* access on recovery, never toward silently leaving a stale permanent rule.

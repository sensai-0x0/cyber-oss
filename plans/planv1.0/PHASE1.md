# Phase 1 — Collection Core: Authorized Scan Engine (`hunterd`)

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: Phase 0 (contracts frozen) · Parallel with: Phases 2, 3 · PRD coverage: Block 0 (Core), Block 1 (sensor half)

**Exit gate:** `go test -race ./hunterd/...` is green; a hunt record's authorized scope is enforced in Go **before any dial**, with zero off-scope packets observed under fuzzing; a shared token-bucket rate limiter bounds `tlsx`/`httpx` probing; CDN noise suppression matches `spec/SCOPE.md`'s intent; the Human Checkpoints below are signed off.

**Baseline already present:** `hunterd/cursor/feistel.go` (keyed Feistel permutation, tested — `hunterd/cursor/feistel_test.go`), `hunterd/store/badger_store.go` (cursor persistence + novelty/delta gate, tested), `hunterd/probes/` (tlsx/httpx normalization into `Observation`, tested), `hunterd/cmd/hunterd/main.go` (NDJSON pipe + gRPC health, tested). This phase builds the missing authorization layer in front of this baseline and hardens what exists.

**⚠️ Scope note:** a scan of the current implementation found **no code path that checks a probe target against an authorized scope, no rate limiter, and no CDN-noise suppression** anywhere in `hunterd`. `spec/SCOPE.md` and `.agents/orchestration/invariants.md` (I1–I6) describe this as a hard requirement; it does not yet exist as code. P1.1 below is therefore the highest-priority, safety-critical task in this entire plan — treat it accordingly.

---

### P1.1 — Authorized-scope enforcement (safety-critical)
Owner: new `hunterd/scope` package · Parallel-safe: no (blocks P1.2, P1.3's final wiring, though both can be built against a stub in the meantime) · Model tier: highest reasoning + mandatory `/security-review`

- [ ] P1.1.1 Define the hunt-record config format (CIDRs/domains, ports, exclusions, max rate, time window — per `spec/SCOPE.md`) and a loader that refuses to start on empty/unparseable scope (never defaults to "allow all," per invariant I4).
- [ ] P1.1.2 Implement `Allows(ip netip.Addr) bool` / `AllowsHost(host string) bool` checked **before every dial** in `hunterd/probes`, including: resolve-then-check for hostnames (every resolved IP, not just the first — I2), no following a redirect off-scope, and explicit rejection of IPv6/IPv4-mapped-IPv6/decimal/octal address-encoding bypass attempts.
- [ ] P1.1.3 Write failing tests first (per `pipeline-builder`'s workflow) covering: superset-of-scope input, adjacent-but-out-of-scope ranges, DNS rebinding (resolves in-scope then out-of-scope on a second lookup), and a redirect chain that exits scope — this is directly the shape of eval **E03** in `.agents/evals/evals.md`.
- [ ] 🔒 HUMAN CHECKPOINT: review the scope-check design (where it's called, what "checked before dial" actually means in the `tlsx`/`httpx` call path, fail-closed behavior) **before implementation**, not after. This is the single piece of code this whole product's safety claim rests on — see `docs/05-safety-and-trust-model.md`.
- [ ] P1.1.4 Wire the scope check into `hunterd/probes.TLSRunner.Probe` and `HTTPRunner.Probe`, and into the `cursor.Range` consumer in `hunterd/cmd/hunterd`, so an out-of-scope target cannot reach either probe runner regardless of entry point.
- [ ] P1.1.5 Fuzz test per E03: 500+ generated calls, assert zero off-scope packets on a sink listener.

### P1.2 — Shared rate limiting
Owner: `hunterd/cmd/hunterd` or a new `hunterd/ratelimit` package · Parallel-safe: yes (independent of P1.1's internals, but shares the same probe call sites) · Model tier: mid

- [ ] P1.2.1 Implement one global token bucket (`rate_limit_pps`, default 100 per the PRD) shared across `tlsx` and `httpx` probing — not a per-runner limiter, which would let combined throughput exceed the configured ceiling.
- [ ] P1.2.2 Exceeding the limit backs off or returns a bounded error; it never silently queues unbounded work (matches the bounded-NDJSON-channel backpressure principle already used in `hunterd/cmd/hunterd/main.go`).
- [ ] P1.2.3 Load test: concurrent probing for 60s at the configured `rate_limit_pps`, measured throughput stays within ~5% of the ceiling in every 1s window (eval **E09**'s shape).

### P1.3 — CDN noise guard
Owner: `hunterd/probes` or `hunterd/store` (wherever `is_cdn`/favicon classification is cheapest to apply) · Parallel-safe: yes · Model tier: mid

- [ ] P1.3.1 Add `is_cdn` detection (or confirm whether `projectdiscovery/httpx`'s existing CDN-check output, already an indirect dependency, can be reused directly) to probe normalization.
- [ ] P1.3.2 Suppress `is_cdn: true` observations from reaching the novelty gate's emit path **unless** the observation's `favicon_mmh3` matches a configured watchlist — this is a product-noise control, not a safety boundary, so it can be implemented without a mandatory human checkpoint, but should still be reviewed in the phase-exit pass.
- [ ] P1.3.3 Test: a mock CDN-fronted host produces zero emitted events across repeated passes; the same host with a watchlisted favicon still emits.

### P1.4 — Harden what already exists
Owner: `hunterd/cursor`, `hunterd/store` · Parallel-safe: yes · Model tier: mid, escalate on flaky/race findings

- [ ] P1.4.1 Add a `go test -race` run specifically targeting concurrent `DeltaGate.Apply` calls on the same key, confirming the single-transaction read/compare/write holds under contention (no two callers both get `Novel` for the same first-seen key).
- [ ] P1.4.2 Add a crash-resume test at the **daemon** boundary (not just the `cursor.Range` unit level already covered by `feistel_test.go`): start `hunterd serve`, feed partial scan state, kill the process, restart, confirm `LoadCursor` resumes without re-emitting or skipping — this is the daemon-level version of what `.agents/testing.md` already asks for.
- [ ] P1.4.3 Confirm odd-width prefixes (`/23`, `/31`, `/32`) are exercised at the `Range` level in addition to the existing `Permute`/`Unpermute` bit-width tests.

### P1.5 — Live Certificate Transparency source (stretch, lowest priority in this phase)
Owner: `hunterd/probes` · Parallel-safe: yes, but defer if P1.1–P1.4 aren't done · Model tier: mid

- [ ] P1.5.1 Evaluate a CT-stream client (e.g. a certstream-style feed) against the existing `source: ct` schema value — this is explicitly an open question in `.agents/state-log.md`, not yet answered.
- [ ] P1.5.2 If adopted, wire it to emit through the same `DeltaGate`/novelty path as `tlsx`/`httpx` — no second code path for novelty suppression.
- [ ] ⛔ BLOCKED (by default): do not start P1.5 until P1.1–P1.4 are closed. This task exists so it isn't forgotten, not so it's worked first.

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 1 gate review — scope enforcement proven under fuzzing (P1.1.5), rate limiting proven under load (P1.2.3), CDN guard verified, crash-resume verified at the daemon level. Sign off before Phase 4 may treat `hunterd`'s interface as stable.

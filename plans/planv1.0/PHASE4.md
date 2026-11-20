# Phase 4 — Transport & Durability Hardening

**Status:** active · **Derived from:** [../PHASESv1.0.md](../PHASESv1.0.md) · **Updated:** 2026-11-21

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: Phases 1, 2, 3 (interfaces stable, not necessarily every task closed) · PRD coverage: none directly — this closes the gaps `spec/ARCHITECTURE.md` names explicitly in its "Current scaffold boundary" section

**Exit gate:** a durable NATS consumer replaces the NDJSON-stdio-only path for production use without changing the `DeltaEvent` contract; `orchestrator`'s durable checkpointer (built in P2.2) is the configured default outside tests; `enforcerd` has a defined transport for receiving actions from a centralized source rather than only an in-process call; the Human Checkpoints below are signed off.

**This is a hard sequencing point — read `plans/PHASESv1.0.md` §5 before starting any task here.** Starting Phase 4 against Phase 1–3 interfaces that are still actively changing produces throwaway integration work. Confirm each of Phase 1/2/3's exit-gate Human Checkpoints have been signed off before branching Phase 4 work, even if some of their stretch tasks (e.g. Phase 1's P1.5, Phase 3's P3.4) are still open — those are explicitly deferrable without destabilizing the interface.

---

### P4.1 — Durable NATS consumer for `hunterd` events
Owner: `hunterd/cmd/hunterd`, `deploy/nats/` · Parallel-safe: yes against P4.2 · Model tier: highest — durability and at-least-once semantics are easy to get subtly wrong

- [ ] P4.1.1 Design the production consumer against the NATS instance already defined in `deploy/compose.yaml` (TLS, locally generated certs) — decide subject naming, consumer group semantics, and how backpressure (today's bounded in-process channel in `hunterd/cmd/hunterd/main.go`) maps onto NATS flow control.
- [ ] P4.1.2 Implement the consumer with at-least-once delivery and idempotent handling on the `orchestrator` side (a redelivered `DeltaEvent` with the same `event_id` should not double-trigger policy synthesis) — this needs explicit dedup logic that doesn't exist yet.
- [ ] P4.1.3 Test: kill the consumer mid-stream, restart, confirm no event is permanently lost and no event is double-processed into two separate mitigation proposals.
- [ ] 🔒 HUMAN CHECKPOINT: review the NATS deployment topology (single node vs. cluster, auth/TLS posture) against actual production infra plans before this becomes the default transport.

### P4.2 — Durable checkpointer as the default
Owner: `orchestrator/graph.py` · Parallel-safe: yes against P4.1 · Model tier: mid (the hard design work was P2.2; this is wiring it in as default)

- [ ] P4.2.1 Promote the checkpointer built in P2.2 from "available option" to the configured default for any non-test invocation of `build_graph`.
- [ ] P4.2.2 Test: a broad-action `interrupt()` pending across a full process restart, with the NATS consumer from P4.1 also restarting, resumes correctly end-to-end — this is the first test that exercises P4.1 and P4.2 together.

### P4.3 — Centralized enforcer transport
Owner: `enforcerd`, `operator` · Parallel-safe: no (depends on P4.1's transport decisions for consistency) · Model tier: highest — this is the production shape of the signing/enforcement boundary

- [ ] P4.3.1 Decide and document how a signed `MitigationAction` reaches `enforcerd` in production — today's tested path is in-process/direct calls; production needs either a service endpoint or a message-bus subject, consistent with the transport choice made in P4.1.
- [ ] P4.3.2 Whatever the transport, confirm it does not weaken the signature boundary: `enforcerd` must still verify independently of the transport's own authentication, per `docs/05-safety-and-trust-model.md` — transport-layer TLS is not a substitute for Ed25519 verification of the payload itself.
- [ ] 🔒 HUMAN CHECKPOINT: review the chosen transport against the trust-boundary requirement in P4.3.2 before it's adopted — this is exactly the kind of change that could accidentally reintroduce an unsigned-action path if done carelessly.

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 4 gate review — durable transport and checkpointer proven under restart/kill testing, enforcer transport decision documented and signature-boundary-safe. Sign off before Phase 5 begins full-stack integration.

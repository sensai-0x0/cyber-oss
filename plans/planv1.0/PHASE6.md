# Phase 6 — Forward Capability Expansion

**Status:** active · **Derived from:** [../PHASESv1.0.md](../PHASESv1.0.md) · **Updated:** 2026-11-21

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: Phase 5 gate passed (hard requirement, not a suggestion) · Maps to: `docs/roadmap.md` Horizons 1–2

**Exit gate:** each adopted capability below ships behind the existing trust boundary with no change to how signing, approval, or TTL enforcement works — it should only ever be additive to `score.py`/`cluster.py`/`policy.py`/a new `enforcerd` backend. If a Phase 6 task seems to require loosening a safety invariant to land, that is a sign the task is mis-scoped, not a sign the invariant needs revisiting — escalate to a Human Checkpoint rather than proceeding.

**Do not start this phase early.** See `plans/PHASESv1.0.md` §5. Phase 5's eval results are what justify treating the system as safe to extend; starting Phase 6 before that gate passes means adding capability on top of an unproven boundary.

---

### P6.1 — Live threat-intelligence feed for `known_bad`
Owner: `orchestrator/score.py` · Parallel-safe: yes · Model tier: mid for integration, highest for anything touching how the feed's data enters `score_threat_intel`

- [ ] P6.1.1 Select and integrate a maintained JARM/JA3/favicon-hash badlist feed to populate the `known_bad` attribute — currently this is only ever set by test fixtures.
- [ ] P6.1.2 Confirm the feed is treated as untrusted input into an `attributes` dict exactly like everything else per eval E05's precedent — no special trust is extended to this source just because it's "intelligence."
- [ ] 🔒 HUMAN CHECKPOINT: review the feed source and its false-positive rate before it influences live mitigation proposals — `known_bad` carries the single largest score weight (+0.45) in `score.py`.

### P6.2 — Live Certificate Transparency streaming
Owner: `hunterd/probes` · Parallel-safe: yes · Model tier: mid

- [ ] P6.2.1 Revisit Phase 1's deferred P1.5 (CT client evaluation) now that the full pipeline is proven under Phase 5's chaos/eval suite.
- [ ] P6.2.2 Wire the CT source through the same `DeltaGate`/scope/rate-limit path as `tlsx`/`httpx` — no new unguarded ingestion path.

### P6.3 — ASN-hop detection
Owner: `orchestrator/cluster.py`, `orchestrator/score.py` · Parallel-safe: yes · Model tier: mid

- [ ] P6.3.1 Resolve the open question in `.agents/orchestration/obs/state-log.md` ("which ASN lookup feeds `DeltaEvent.asn`") — offline DB vs. API, and the licensing/freshness tradeoff.
- [ ] P6.3.2 Add ASN-hop signal (infrastructure moving providers shortly before use) into clustering/scoring as a new `attributes` key, following the same additive-only contract rule from `contract-steward`.

### P6.4 — Headscale backend (if deferred from Phase 3)
Owner: new `enforcerd/headscale` · Parallel-safe: yes · Model tier: mid for implementation, highest for the approval-path review

- [ ] P6.4.1 If P3.4.1 deferred this, implement it now behind the existing `Enforcer`/`SignedBackend` machinery, exactly as scoped in Phase 3.
- [ ] P6.4.2 Run the same isolated-lab integration proof Phase 3 required for `nft`/`rpz` (P3.1) against the new backend before it's considered done.

### P6.5 — Graduated policy tiers
Owner: `orchestrator/policy.yaml`, `orchestrator/policy.py` · Parallel-safe: no (touches the frozen Phase 0 contract's consumer, needs `contract-steward`) · Model tier: highest — this changes risk-tuning surface, not just code

- [ ] P6.5.1 Design a multi-tier confidence model (e.g. "observe with elevated priority" / "propose narrow" / "propose broad") replacing today's single `mitigation_confidence` floor, per `docs/roadmap.md` Horizon 2.
- [ ] P6.5.2 Ensure the broad/narrow `broad_prefix_threshold` topology check remains independent of whichever confidence tier fires — Phase 2's design principle (confidence answers "is this real," breadth answers "how much could this touch if wrong") must survive the added tiers.
- [ ] 🔒 HUMAN CHECKPOINT: review the new tier design against real triage outcomes from Phase 5/P2.3 before shipping — this is the kind of change that's easy to get subtly wrong in a way that only shows up as more false mitigations later.

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 6 gate review, per-capability as each lands (this phase does not have a single end-of-phase gate the way Phases 0–5 do — each capability in P6.1–P6.5 ships independently once its own checkpoint clears).

# Phase 3 — Mitigation Enforcement Plane (`operator`, `enforcerd`)

**Status:** active · **Derived from:** [../PHASESv1.0.md](../PHASESv1.0.md) · **Updated:** 2026-11-21

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: Phase 0 (contracts frozen) · Parallel with: Phases 1, 2 · PRD coverage: Block 3

**Exit gate:** `go test -race ./operator/... ./enforcerd/...` is green; an isolated-network integration test proves a real rule is installed, actually drops traffic, and is actually removed on TTL expiry (not just that the in-memory bookkeeping says so); the Headscale backend decision is made and either implemented or explicitly deferred to Phase 6; the Human Checkpoints below are signed off.

**Baseline already present:** `operator/decision.go` (approve/reject + signing, tested), `enforcerd/verify_signature.go` (canonical signing/verification, tested), `enforcerd/enforcer.go` (apply/expire lifecycle, tested), `enforcerd/ttl.go`, `enforcerd/nft/backend.go`, `enforcerd/rpz/backend.go` (both tested against fakes). This phase proves the existing unit-level correctness holds when a real backend (even a loopback-lab one) is on the other end, and closes the one schema-reserved gap (`headscale`).

**Develop against fake backends, never a real target outside the lab.** Per `plans/PHASESv1.0.md` §5 and the `mitigation-guardian` skill's own workflow: exercise `Apply`/`Revoke` with fakes first; hand off to `lab-validator` for anything that needs a real loopback backend; never use production firewall state as a test fixture.

---

### P3.1 — Isolated-network integration test (PRD Block 3 acceptance criteria)
Owner: new `enforcerd` integration test, likely `enforcerd/nft` · Parallel-safe: no (this is the phase's centerpiece; other tasks can proceed around it but this is the gate) · Model tier: highest — this is the first point real traffic behavior gets asserted, not mocked

- [ ] P3.1.1 Stand up an isolated network namespace (or the `firewall-target` Compose profile already defined in `deploy/compose.yaml`) and confirm an applied `nftables` rule actually drops traffic to the target.
- [ ] P3.1.2 Confirm the rule is actually removed from `nftables` state when `Enforcer.Expire` fires — not just that the in-memory `actions` map entry is cleared, which is as far as today's unit tests reach.
- [ ] P3.1.3 Repeat P3.1.1/P3.1.2 for the `rpz` backend against the loopback CoreDNS fixture in `deploy/dns/`.
- [ ] 🔒 HUMAN CHECKPOINT: review the integration test's privilege requirements (the `firewall-target` Compose profile needs `NET_ADMIN` and `network_mode: host`) before it runs anywhere other than an explicitly sandboxed CI runner.

### P3.2 — Failed-revocation retry path under real conditions
Owner: `enforcerd/enforcer.go` · Parallel-safe: yes · Model tier: highest (this is the exact mechanism ADR 0002 depends on)

- [ ] P3.2.1 Test that a `Backend.Revoke` failure (e.g., the lab target is briefly unreachable) leaves the action tracked for retry on the next `Expire` tick, and that a human-visible alert signal exists for "revocation is overdue" — today's `Enforcer.Expire` returns the failure to its caller, but there's no persistent alerting surface yet.
- [ ] P3.2.2 Confirm a retried revoke that eventually succeeds removes the action from the tracked set exactly once, with no duplicate removal attempts racing a concurrent `Expire` tick.

### P3.3 — Operator approval surface (minimal)
Owner: `operator/cmd` · Parallel-safe: yes · Model tier: mid

- [ ] P3.3.1 Confirm `operator/cmd` (the CLI mentioned in `README.md`'s layout) exists and exposes approve/reject against a concrete `enforcerd.Action` payload — verify current state and fill any gap between the tested `operator/decision.go` logic and an actual runnable CLI entry point.
- [ ] P3.3.2 Add key-generation and key-storage guidance (where the Ed25519 private key lives, who can access it) to `spec/SCOPE.md` or a new `operator/README.md` (planned) — currently undocumented outside the code itself.
- [ ] 🔒 HUMAN CHECKPOINT: review key custody before any approval flow is used against a real (even lab) backend — an operator's private key is the entire trust anchor for the enforcement boundary described in `docs/05-safety-and-trust-model.md`.

### P3.4 — Headscale backend decision
Owner: new `enforcerd/headscale` (if adopted) · Parallel-safe: yes, but low priority relative to P3.1–P3.3 · Model tier: mid for implementation, highest for the approval-path check

- [ ] P3.4.1 Decide whether a Headscale/Tailscale ACL backend ships in v1.0 or is deferred to Phase 6 (`kind: headscale` is already schema-reserved per `schemas/mitigation_action.v1.json`, but no `enforcerd/headscale` package exists).
- [ ] P3.4.2 If adopted now: implement `Backend.Apply`/`Revoke` for Headscale behind the existing `Enforcer`/`SignedBackend` machinery — no change to signing, TTL, or approval logic should be required, by design.
- [ ] ⛔ BLOCKED (pending P3.4.1 decision): do not start P3.4.2 until the decision is recorded.

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 3 gate review — real traffic drop/removal proven in the isolated lab, retry path verified, key custody documented, Headscale decision recorded. Sign off before Phase 4 may treat the enforcement interface as stable.

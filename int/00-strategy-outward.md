# Strategy: From Engineering Scaffold to Externally Operated Private Intelligence Firm

**Status:** internal draft · **Derived from:** `spec/`, `docs/`, `plans/`, `.agents/orchestration/obs/state-log.md` · **Updated:** 2026-11-21  
**Audience:** repository owner / future firm principals · **Classification:** internal

## 1. What we have today

Sentinel Hunter is an **engineering scaffold**, not a product. Every primitive is unit- and property-tested against an isolated loopback lab (fake C2, disabled phishing panel, loopback DNS/firewall targets):

- **Implemented and tested:** keyed resumable CIDR cursor (Feistel), BadgerDB novelty/mutation gate, TLS/HTTP observation normalization, cluster → score → policy LangGraph graph, human interrupt for broad actions, Ed25519 decision signing, nftables/RPZ enforcement adapters with TTL expiry + revoke retry, loopback adversarial lab.
- **Not yet production-ready:** durable NATS consumer, durable LangGraph checkpointer, live Certificate Transparency client, Headscale/Tailscale adapter, centralized enforcer transport, recorded end-to-end lab evidence, production approval surface.
- **Safety gaps with no enforcement point (blockers for any external use):** authorized-scope allowlist (I1–I5), rate limiter (I6), mitigation durability / pre-install persist + reaper (I12, I13), cursor checkpointing (I16).

The architecture is the product's moat: **human review of consequential action is a technical boundary, not a prompt instruction.** `orchestrator` proposes but cannot sign or reach a backend; `operator` signs but cannot apply; `enforcerd` applies only signed, TTL-bounded (≤24h), approved actions and retries failed revocations. That trust boundary is deliberately stable; nearly all product growth happens inside `orchestrator/` (new signals into `score.py`, new clustering in `cluster.py`, new mitigation `kind`s in `policy.py`/`policy.yaml`).

## 2. What "turning outward" means

Operating this on behalf of external parties (clients of a Private Intelligence Firm) means adding a **commercial and operational layer around** the existing trust boundary — not changing the boundary itself. The firm's sellable properties are exactly the properties already engineered in:

- **Bounded, reversible defense:** every mitigation expires ≤24h; revocation needs no new approval and is cheaper than imposition.
- **Attributable decisions:** every broad action carries a named operator and an Ed25519 signature; rejections are structurally inert.
- **Falsifiable safety:** scope enforcement, TTL gate boundaries, and prompt-injection resistance are hard-gate evals (E03/E04/E05) run against evidence, not asserted by design review.

The firm does not sell "auto-block." It sells **continuous, authorized pre-attack hunting of the client's own exposed surface, with a named human in the loop for anything consequential.**

## 3. Sequencing (four horizons, already documented in `docs/roadmap.md`)

The repository's roadmap is the correct sequence. Do not reorder it to chase external revenue; the trust boundary must hold before any client traffic flows.

| Horizon | Focus | Gate to external use |
|---|---|---|
| **Horizon 0 — Close the scaffold boundary** (near-term) | Durable checkpointer, durable NATS consumer, reconcile stale `.agents/orchestration/*` notes, first real end-to-end lab run via `lab-validator` | **Must complete before scanning anything but `lab/`** |
| **Horizon 1 — Live intelligence sources** | Live CT streaming (`source: ct`), real threat-intel feed behind `known_bad`, ASN-aware signal / ASN-hop detection | Enables real scoring, not fixtures |
| **Horizon 2 — Richer policy, new mitigation backends** | Headscale/Tailscale adapter, graduated confidence tiers, **per-client policy profiles** (multi-tenancy) | Required before more than one client |
| **Horizon 3 — Operator experience & fleet-scale triage** | Real approval surface beyond `operator/` CLI, cross-cluster correlation, outcome feedback loop (decisions → labelled data → score tuning) | Required for firm-scale operations |
| **Horizon 4 — Hardening & autonomy expansion** | Only after 0–3 hold; autonomy expands by proving gates hold under adversarial pressure, never by relaxing them | Protected clauses never move |

## 4. The commercial layer to add (outside the trust boundary)

These are firm-level concerns, not changes to `hunterd`/`orchestrator`/`operator`/`enforcerd` core logic:

1. **Client onboarding & authorization records** — per `spec/SCOPE.md`: authorization owner + evidence reference, exact CIDRs/domains/ports/exclusions/rate/time-window, cursor key identifier (not secret), approval requirements for broad actions, max TTL ≤24h. One hunt record per client.
2. **Multi-tenancy / per-client policy** — scope-scoped thresholds rather than one global `policy.yaml` (see `int/02-multi-tenancy-per-client-policy.md`).
3. **Production deployment** — durable checkpointer + NATS consumer, centralized enforcer transport, real CT client, Headscale/Tailscale adapter, privilege model for nftables/RPZ writes.
4. **Operator approval surface** — a queue/notification layer delivering broad-action `interrupt()` payloads (`action_id`, `target`, `confidence`, `max_ttl_seconds`) to named humans quickly. The `operator/` CLI logic is already correct; the surface is missing.
5. **Reporting & billing surface** — observation volume, novel/mutated rate, actions proposed/approved/rejected/expired, revocation outcomes, per-client SLAs.
6. **Outcome feedback loop** — operator approve/edit/reject and post-hoc revocation outcomes become labelled data for `score.py`/threshold tuning (eval E07 anticipates this); this is the point at which the firm becomes a learning system rather than a fixed heuristic.

## 5. Legal / compliance posture

- **Explicit authorization per client** is a prerequisite, not a convention. No scanning without a recorded hunt scope, exclusions, rate limits, time window, and authorization owner.
- **Never point at third parties.** Scope is checked in Go before any packet is sent (once I1–I5 land); out-of-scope means no connection, not a warning.
- **Reversibility over coverage.** For false positives or uncertain ownership, choose observation or early revocation.
- **Audit trail:** versioned schemas (`delta_event.v1`, `mitigation_action.v1/v2`), append-only evidence ledger, Ed25519 signatures, TTL expiry with retryable revocation. v2 binds an approval to a specific evidence digest (ADR 0005).
- **Do not commit private keys, Badger data, checkpoints, credentials, or real target lists** (`CLAUDE.md` safety invariants).

## 6. Guardrails that must not move

Per `.agents/evals/constitution.md`, these are **protected clauses**, not tunable genes:

- Human approval for broad action.
- Signed, TTL-bounded (≤24h), reversible enforcement.
- Scope enforcement: zero off-scope packets (adjacent ranges, IPv6/IPv4-mapped, decimal/octal encodings, redirects, DNS rebinding).
- Prompt-injection resistance: zero unsafe executions across adversarial `DeltaEvent` fixtures against a live model.

No accumulation of product or revenue pressure is intended to loosen them. Autonomy expands only by proving the gates hold at higher volume and sophistication of adversarial input.

## 7. Immediate next steps (this repository, before any external use)

1. **Scope gate (I1–I5)** — `hunterd/scope/` package owning the allowlist, enforced before any dial, wired into both probe runners. Highest priority; no active probing outside `lab/` until it lands (`int/01-scope-enforcement-spec.md`).
2. **Mitigation durability (I12, I13)** — persist action record before installing a rule; reaper on startup and on a timer.
3. **Rate limiter (I6)** — one shared token bucket across both probe runners.
4. **Cursor checkpointing (I16)** — call `SaveCursor` periodically and on signal; prove resume with no gaps or duplicates.
5. **Retarget E01–E09** onto real paths per ADR 0003; run them; record results in `.agents/eval-results/`.
6. **First real end-to-end lab run** against `lab-validator`'s full checklist — this is the first evidence milestone.

Once Horizon 0 holds and Horizon 2's per-client policy lands, the scaffold can operate externally under the firm's authorization and commercial layer without touching the trust boundary.

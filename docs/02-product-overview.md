# Product Overview

## The problem, concretely

A security team authorizes continuous passive-aggressive reconnaissance of their own exposed infrastructure: their CIDR blocks, their domains. The goal is to catch staging infrastructure for an attack against *them* — a freshly issued certificate on a lookalike domain, a favicon hash matching a known phishing kit, a TLS fingerprint (JARM/JA3) that showed up on infrastructure associated with a prior campaign, an ASN hop that suggests infrastructure is being moved into position — before that infrastructure is used. This is pre-attack hunting, not incident response.

Doing this by hand doesn't scale: the signal is mostly noise (CDN churn, routine cert renewal, load balancer rotation), and acting on it by hand is slow. Doing it with a naive "auto-block on heuristic" system is dangerous: false positives against your own CDN edge or a partner's infrastructure cause outages, and a wrong confidence threshold either lets real staging infrastructure through or blocks something that mattered. Sentinel Hunter is built to sit between those two failure modes.

## Who uses it

- **The authorizing owner** (security leadership / the client who owns the scope): defines the authorized CIDRs/domains, exclusions, rate limits, and time window once, in a hunt record (see [spec/SCOPE.md](../spec/SCOPE.md)). This is the only place scope is widened, and it requires an evidence reference, not just a config edit.
- **The operator** (the named human in the loop): reviews proposed mitigations the system could not resolve on its own — anything broad, or anything below the automatic-action confidence floor — and either approves with a bounded TTL, edits the TTL, or rejects. The `operator/` package exists specifically to make this a human act with a cryptographic consequence, not a checkbox the system can route around.
- **The on-call reviewer of narrow automatic actions**: narrow, high-confidence, short-TTL actions (a single-IP drop) can apply without a synchronous human click, but remain fully attributable, logged, and revocable — reviewed after the fact rather than gated before it.

## End-to-end workflow

1. **Scope is authorized once.** A hunt record fixes exact CIDRs/domains, exclusions, rate limit, and time window. Nothing scans outside it — scope is enforced in Go, not merely described in config (see [05](05-safety-and-trust-model.md)).
2. **`hunterd` scans continuously, cheaply.** A keyed, constant-memory permutation walks the authorized address space in pseudorandom but resumable order (no restart-from-zero, no predictable scan pattern), probing with `tlsx`/`httpx`.
3. **Only new or changed signal escapes the daemon.** A BadgerDB-backed novelty gate suppresses anything unchanged since last observation. The "unchanged" verdict never leaves `hunterd`; only `novel` and `mutated` events cross into the AI-facing plane. This is both a cost control and a security boundary — the triage layer physically cannot see noise it doesn't need to see.
4. **The orchestrator clusters and scores.** Related observations (shared certificate, JARM/JA3, favicon hash) are grouped into a cluster; an auditable confidence score is computed with a visible rationale (not a black-box number) from signals like recent CT activity, known-bad fingerprint matches, and shared-fingerprint density across observations.
5. **Policy decides observe vs. mitigate.** Below the confidence floor, the system records an observation and stops — no action, no false alarm fatigue. Above it, a mitigation is proposed; if it's narrow (a single host, below the broad-prefix threshold) it's marked pre-approved, if it's broad it is not.
6. **Broad or uncertain actions always stop for a person.** The graph pauses on a native interrupt and hands the operator the exact proposed action, its confidence, and the policy-configured max TTL. The operator approves (optionally adjusting TTL within bounds), edits, or rejects — nothing is enforced on their behalf without that explicit record.
7. **Approval becomes a signed, bounded action.** `operator` attaches TTL, approver identity, and an Ed25519 signature over the canonical action payload. A rejection is explicitly *not* a deployable artifact — it has no signature and structurally cannot reach an enforcement backend.
8. **`enforcerd` verifies, applies, and — critically — expires.** No signature, no broad-without-approval, no out-of-bounds TTL gets past verification. Every applied action is tracked until its backend-confirmed revocation; a failed revoke stays in the retry set rather than being quietly treated as "handled."

## What "done" looks like per interaction

- **Benign noise** (CDN rotation, routine renewal): suppressed before it ever reaches a human or a model. Zero signal, zero cost.
- **Ambiguous signal**: clustered, scored, and — if still ambiguous — given one bounded refinement pass (re-cluster once) before a terminal policy decision is forced. The system never loops indefinitely on uncertainty.
- **High-confidence, narrow**: pre-approved policy, still subject to TTL bounds and signature verification at the enforcement boundary; nothing is "automatic" in the sense of unverified.
- **Broad or low-margin confidence**: a named person reviews a concrete proposal (not a raw alert) and makes an attributable decision within policy-defined TTL bounds.
- **Every mitigation, regardless of path**: expires on its own by design. There is no permanent rule in this system; see [ADR 0002](../spec/DECISIONS/0002-reversible-mitigation.md).

## Current capability surface vs. scaffold

| Capability | Status |
|---|---|
| Authorized-scope CIDR sweep with resumable, keyed ordering | Implemented, unit-tested (`hunterd/cursor`) |
| Novelty/mutation suppression with TTL expiry | Implemented, unit-tested (`hunterd/store`) |
| TLS/HTTP probing → `DeltaEvent` normalization | Implemented (`hunterd/probes`, backed by `tlsx`/`httpx`) |
| Cluster → score → policy → human-gate graph | Implemented, tested (`orchestrator/`) |
| Signed, TTL-bounded, revocable actions (nftables, RPZ) | Implemented, tested (`enforcerd/nft`, `enforcerd/rpz`) |
| Operator approve/reject CLI with Ed25519 signing | Implemented, tested (`operator/`) |
| Isolated lab (fake C2, disabled phishing fixture, loopback DNS/firewall) | Implemented (`lab/`, `deploy/`) |
| Durable production message transport (NATS consumer) | Not yet — in-memory/NDJSON path only |
| Durable LangGraph checkpointer for `interrupt()` across restarts | Not yet — test-only in-memory checkpointer |
| Live Certificate Transparency streaming | Not yet — `ct` is a valid schema `source` with no live client |
| Headscale/Tailscale mitigation backend | Schema-reserved (`kind: headscale`), no adapter yet |

See [spec/ARCHITECTURE.md](../spec/ARCHITECTURE.md#current-scaffold-boundary) for the team's own line on this, and [roadmap.md](roadmap.md) for how these close.

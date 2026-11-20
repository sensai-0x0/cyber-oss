# Sentinel Hunter — Executive Summary

## What it is

Sentinel Hunter is a control system for **authorized, pre-attack infrastructure hunting**: continuously and randomly sampling a client's own authorized IP ranges and domains for staging infrastructure — freshly issued certificates, new favicons, JARM/JA3 fingerprint churn, ASN hops — clustering the signal, scoring it, and routing anything actionable to a named human for a signed, time-boxed, reversible mitigation (a firewall drop or DNS sinkhole). It is not a scanner and not an auto-blocker; it is the connective tissue between "we noticed something new in our address space" and "a person with authority decided what to do about it, for a bounded window."

The product thesis: most pre-attack reconnaissance and staging infrastructure is detectable *before* the first malicious packet hits a client, if someone watches the client's own exposed surface continuously enough and cheaply enough to make that watching routine rather than a one-off assessment. Sentinel Hunter is built to make that watching safe to automate — bounded, reversible, auditable — while keeping every consequential action under human sign-off.

## Why now

- **LLM-assisted triage is finally good enough to route, not decide.** The system uses a LangGraph agent to cluster and score ambiguous infrastructure signal, but it is deliberately not trusted to pull a trigger — every bound (scope, TTL, confidence threshold, broad-vs-narrow) is enforced in typed, tested Go code the model cannot talk its way around. This is the architecture bet: use the model for what it's good at (triage judgment under ambiguity) and keep it away from what it's bad at (irreversible real-world actions).
- **Mitigation has historically been all-or-nothing.** Either a SOC blocks nothing until confirmed malicious (slow, reactive) or a vendor's "auto-block" fires on a heuristic and breaks a CDN edge or a partner integration (fast, dangerous). Sentinel Hunter's answer is a graded autonomy model: narrow, high-confidence actions can be automatic within a strict TTL; anything broad (a CIDR block, a wildcard DNS policy, a new ASN) always interrupts for a named, signed approval.
- **Reversibility is the organizing design principle**, not a bolt-on safety feature. Every mitigation action carries a mandatory expiry (24-hour hard ceiling), and revocation is designed to require *no* new approval — undoing a block is always cheaper, in process terms, than imposing one.

## How it is built

Four independently-boundaried services, each with a narrow job and no ability to exceed its boundary even if another component is compromised or an LLM is adversarially prompted:

| Service | Language | Job | Cannot do |
|---|---|---|---|
| `hunterd` | Go | Scans authorized ranges, deduplicates observations, emits only *new* or *changed* signal | Decide what the signal means |
| `orchestrator` | Python / LangGraph | Clusters, scores, and proposes a policy | Touch a firewall, DNS zone, or any backend |
| `operator` | Go | Turns a human decision into a cryptographically signed, bounded action | Apply or revoke anything itself |
| `enforcerd` | Go | Verifies the signature, applies the action, and reaps it at expiry | Accept an unsigned or broad-unapproved action |

This is a deliberate architectural choice (see [ADR 0001](../spec/DECISIONS/0001-service-boundaries.md)): human review is a *technical* boundary enforced by process and cryptographic separation, not a policy written in a prompt that a model could be talked out of. The orchestration layer that proposes action has no credentials to a backend; the layer with backend credentials cannot be reached without a verified signature tied to a named approver.

## Where it stands today

This is an engineering scaffold, not a shipped product, and the repository says so plainly. Every layer — the Feistel-permuted scanning cursor, the BadgerDB novelty gate, the LangGraph cluster/score/policy graph, Ed25519 action signing, TTL-bounded enforcement with crash-safe reap — exists as working, unit- and property-tested code against a fully isolated local lab (fake C2, a disabled phishing-panel fixture, loopback DNS/firewall targets). What is **not** yet in place: a durable production message bus consumer, a durable LangGraph checkpointer, live Certificate Transparency streaming, and a production enforcement backend wired to real infrastructure. See [spec/ARCHITECTURE.md](../spec/ARCHITECTURE.md#current-scaffold-boundary) for the exact line the team draws between "tested primitive" and "integration work remaining."

## Why the safety posture matters commercially

A hunting/mitigation product that occasionally blocks a customer's CDN edge or scans a target it wasn't authorized for doesn't get a second deployment. Sentinel Hunter's invariants — scope checked in Go before any packet is sent, every bound re-validated on the enforcement side regardless of what the agent or an operator claims, a 24-hour absolute TTL ceiling on every action, broad actions always gated on a named human — are written as hard gates in the test and evaluation suite, not as guidelines. That is the trust property an enterprise security team, and eventually an insurer, needs to see before this runs against anything that matters. See [05-safety-and-trust-model.md](05-safety-and-trust-model.md).

## Roadmap framing

Sentinel Hunter's product surface grows almost entirely inside one component: the `orchestrator/` package — the cluster → score → policy graph. New intelligence sources become new signals into `score.py`; new mitigation backends (a Headscale/Tailscale ACL adapter is already represented in the signed-action schema) become new `kind` values in `policy.py`; smarter clustering becomes a richer `cluster.py`. The hunting and enforcement boundaries are deliberately stable and change rarely — most of the product's future value is intelligence layered on top of a trust boundary that stays fixed. See [roadmap.md](roadmap.md).

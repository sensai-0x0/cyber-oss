# Roadmap

## Framing: the product grows inside `orchestrator/`, not around it

Sentinel Hunter's trust boundary is deliberately the most stable part of the system: `hunterd` collects, `operator` signs, `enforcerd` enforces and expires, and none of that is expected to change shape often — it's the part of the system that exists specifically so it *doesn't* need to change as the product gets smarter (see [03](03-system-architecture.md) and [05](05-safety-and-trust-model.md)). Nearly every roadmap horizon below is additive work *inside* the `orchestrator/` package — new signal into `score.py`, richer grouping in `cluster.py`, new mitigation `kind`s and thresholds in `policy.py` and `policy.yaml`, new edges in `graph.py` — landing on top of a collection/enforcement boundary that stays fixed. That is the intended shape of this roadmap: capability compounds in the one place built to hold it, while the parts that would be dangerous to get wrong stay narrow and rarely touched.

Two senses of "orchestrator" govern this roadmap, and both matter:

1. **Runtime orchestrator** (`orchestrator/`, this repo) — the cluster → score → policy graph that decides what the system *does*. This is where product capability lives and grows.
2. **Development orchestrator** (`.agents/evals/orchestrator.md`, the Master Orchestrator Plan) — the autonomous phase/allocate/execute/evolve loop that decides *how the codebase gets built*, phase by phase, with its own fitness function and constitution. This roadmap's horizons below map onto that loop's phases; each horizon is expected to be executed as one or more `feature/<phase>-<name>` worktrees, gated by the eval suite, merged to `develop`, never to `main` without explicit human sign-off (see [06-engineering-process.md](06-engineering-process.md)).

## Horizon 0 — Close the scaffold boundary (near-term)

The highest-leverage work right now is not new product surface; it's making the existing design durable enough to run unattended. Per [spec/ARCHITECTURE.md](../spec/ARCHITECTURE.md#current-scaffold-boundary):

- **Durable LangGraph checkpointer.** `build_graph(checkpointer=None)` already accepts one; production needs a real backing store so a broad-action `interrupt()` survives a restart instead of losing the pending approval.
- **Durable NATS consumer.** The daemon-to-orchestrator path is currently NDJSON-over-stdio in the tested path; a production consumer needs to be wired with at-least-once semantics matching the bounded-queue backpressure model already in `hunterd/cmd/hunterd`.
- **Reconcile or retire the stale `.agents/orchestration/*` design notes** against the actual `hunterd`/`orchestrator`/`operator`/`enforcerd` layout (see [06](06-engineering-process.md#5-an-honest-note-on-plan-vs-implementation-drift)) — a documentation hazard today, a correctness hazard later if someone implements against the wrong file paths.
- **First real end-to-end lab run** against `lab-validator`'s full checklist (novelty suppression, mutation emission, cursor-resume with no duplicates, low/high-confidence branches, broad interrupt, signed approval, automatic expiry with confirmed revocation) — the repository has zero commits and no recorded eval results yet; this is the first evidence milestone, not a later one.

## Horizon 1 — Live intelligence sources into `score.py`

`score_threat_intel` currently reads boolean/numeric attributes (`recent_ct`, `known_bad`, `shared_fingerprints`) that have to be populated by something upstream. The near-term growth path:

- **Live Certificate Transparency streaming** to actually populate `recent_ct` from a real feed rather than a test fixture — `source: ct` is already a valid value in `schemas/delta_event.v1.json`; no client exists yet (flagged as an open question in `.agents/state-log.md`).
- **A real threat-intelligence feed behind `known_bad`** (e.g., matching JARM/JA3 or favicon hashes against a maintained badlist) — the heaviest-weighted signal in the current score function (+0.45) is the one most worth making real first.
- **ASN-aware signal**: the schema already carries an `asn` attribute path; an ASN-hop detector (infrastructure moving providers shortly before use) is a natural `cluster.py`/`score.py` addition that needs no change to the trust boundary.

## Horizon 2 — Richer policy, new mitigation backends

- **Headscale/Tailscale mitigation adapter.** `schemas/mitigation_action.v1.json` already reserves `kind: headscale` alongside `nftables`/`rpz`; `enforcerd/nft` and `enforcerd/rpz` exist and are tested, but there is no `enforcerd/headscale` package yet. This is a contained addition: a new `Backend` implementation behind the existing `Enforcer`/`SignedBackend` machinery, no change to signing, TTL, or approval logic required.
- **Graduated confidence tiers in `policy.yaml`**, not just a single `mitigation_confidence` floor — e.g. distinct thresholds for "observe with elevated priority" vs. "propose narrow mitigation" vs. "propose broad mitigation," giving operators a richer triage queue instead of a binary observe/mitigate split.
- **Per-client policy profiles.** Today `policy.yaml` is a single global config; multi-tenant operation (more than one authorized scope/client at a time) likely needs the threshold set to become scope-scoped rather than process-global.

## Horizon 3 — Operator experience and fleet-scale triage

- **A real approval surface** beyond the `operator/` CLI primitives — the signing and decision logic (`operator.Approve`/`Reject`) is already correct and tested; what's missing is a queue/notification surface that gets a broad-action `interrupt()` in front of a named human quickly, with the exact payload the interrupt already produces (`action_id`, `target`, `confidence`, `max_ttl_seconds`).
- **Cross-cluster correlation at scale**: `cluster.py`'s current fingerprint-hash grouping is deliberately simple; as observation volume grows across many authorized scopes, this is the natural place to add campaign-level correlation (the same actor's infrastructure reappearing across unrelated clusters) without touching anything downstream of `intel`.
- **Outcome feedback loop**: operator approve/edit/reject decisions and post-hoc revocation outcomes are exactly the labelled data the eval suite's triage-quality check (E07, precision/recall against labelled scenarios) already anticipates — feeding real decisions back into score/threshold tuning is the point at which this becomes a learning system rather than a fixed heuristic.

## Horizon 4 — Hardening and autonomy expansion (only after Horizons 0–3 hold)

Any increase in what the system is allowed to do *without* a human click (e.g., widening Tier-2-style automatic narrow actions, shortening required review latency) is gated, by design, on the hard-gate evals continuing to pass under adversarial pressure — scope enforcement (zero off-scope packets, including DNS-rebinding and redirect-based attempts), TTL boundary correctness, and prompt-injection resistance (zero unsafe executions across adversarial `DeltaEvent` fixtures against a live model). Per the constitution in `.agents/evals/constitution.md`, these are protected clauses, not genes — no accumulation of product pressure is intended to be able to loosen them, and this roadmap does not propose loosening them either. Autonomy expands by proving the gates hold at higher volume and sophistication of adversarial input, not by relaxing the gates.

## How this roadmap gets executed

Each horizon above decomposes into phase-sized features under the Master Orchestrator Plan: planned against the current constitution, allocated to a model tier per the genes in `.agents/evals/constitution.md` (security-invariant code to the highest-reasoning tier, routine plumbing to a mid tier, mechanical work to the cheapest tier), implemented test-first in an isolated worktree, gated by the fixed eval suite (hard-gating E03/E04/E05 regardless of which horizon the feature belongs to), and merged to `develop` one feature at a time — never to `main` without explicit human approval. Fitness is measured per phase and the allocation genome evolves between phases based on that evidence, but the protected clauses — human approval for broad action, signed and TTL-bounded enforcement, reversibility over coverage — do not move. The roadmap changes what the system knows and does; it is not a vehicle for changing what the system is allowed to decide on its own.

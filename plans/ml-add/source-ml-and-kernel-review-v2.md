# Sentinel Hunter: ML review v2 (red-teamed, re-scored)

**Status:** proposed · **Repo:** `sensai-0x0/cyber-oss` @ `9c57762` (confirmed by Sunyata, 2026-11-20) · **Supersedes:** the recommendation in `ml-and-kernel-review.md` (absent: v1 was never committed). v1's §1 findings and §2 catalogue still stand, with the corrections below. · **Plan:** `cbl-implementation-plan.md` (absent: never committed; [README.md](README.md) and the `Phase-*.md` files here replace it)

> **Ingest note (2026-11-20):** copied verbatim from the uploaded `ml-and-kernel-review-v2.md`. The only edits: the two links to files that were never committed are now plain text marked (absent), so the outline gate does not fail on dead links. Read [README.md](README.md) for the phase breakdown and for corrections to this document.

---

## 1. What v1 got wrong or missed

I reread v1 against the repo's own review (`docs/reviews/2026-11-21-codebase.jsonl`, 31 findings) and ran checks on the fixtures. There were three material misses and one weak spot.

| # | v1 position | Correction | Evidence |
|---|---|---|---|
| R1 | "Better scoring = smarter system" | **A better model currently means more unattended blocks.** Any action that is not `broad` gets `approved: True` with no human involved, and a `/24` counts as narrow. Raising recall therefore raises the number of auto-approved mitigations. No scorer may become champion until SHR-007 is fixed and narrow actions have their own guard. | `orchestrator/policy.py:36-46`; SHR-007 (high) |
| R2 | Treated `known_bad`, `recent_ct`, `shared_fingerprints` as features to learn weights for | **They are producer-asserted, unsigned flags.** Anything that can publish a DeltaEvent can set `known_bad=true`. Learning weights on them trains the model to trust the attacker's own claims. Every feature needs a **provenance class**, and a feature asserted by the producer must contribute nothing. | SHR-008 (high); `spec/STATE.md` "attributes are not signed" |
| R3 | Ranked the ledger first for being "passive" | That is right, but a plain ledger does not protect the operator from signing something different from what they reviewed. The record they sign should **commit to the evidence they saw**, via a hash chain. Without that the ledger is a log, not a kernel. | `operator/cmd/operator/main.go` signs only `{id, kind, target, broad}` from stdin |
| R4 | Metric was precision/recall | For a *pre-attack* hunter, the product metric is **lead time**: how many hours before go-live the system would have proposed action. Precision without lead time rewards a scorer that is only confident once the phishing page is already live. | `spec/PRD.md` purpose; E07 only measures precision/recall |

Two checks confirmed v1's claims:
- The fixtures can be classified perfectly by one rule: `known_bad or shared_fingerprints ≥ 2` scores 100 of 100. So they cannot validate any model.
- `cluster.py` and the refinement cycle are a no-op, which matches SHR-014.

**Bias I could not remove:** I scored my own ideas. To limit the damage I ran four weightings (§3) and report rank stability rather than one number. I also scored the fused option with lower Ease and Feasibility than its parts, so that combining ideas does not inflate the score.

---

## 2. More creative candidates

v1 had K1–K8. Six were added:

| ID | Idea | What makes it more than plumbing |
|---|---|---|
| K9 | **Lead-time counterfactual replay** | Replays history and asks: "would this scorer have flagged campaign X before it went live, and by how many hours?" This optimises for the reason the product exists. |
| K10 | **Red/blue self-play in the lab** | A seeded generator mutates synthetic staging infrastructure to evade the current champion (JARM extension tweaks, benign-looking certs, kit reskins). Each round's misses become training and eval data. It runs on loopback only and never touches a real target. |
| K11 | **Campaign belief state** | Scoring moves from per event to per campaign. Each campaign carries a log-odds posterior that every linked event updates, with a half-life decay and per-evidence caps. This makes the refinement cycle mean something: the first pass scores the event, and refinement pulls in campaign evidence. |
| K12 | **Evidence provenance classes** | Every attribute is classified **OBSERVED** (a field hunterd's probe adapter emits for that source), **DERIVED** (computed by the orchestrator from a pinned, hashed artefact, such as a badlist lookup) or **ASSERTED** (anything else). ASSERTED evidence carries zero weight and is logged. This fixes SHR-008 on the scoring path. |
| K13 | **Proof-carrying proposals** | Triage records are hash-chained. The interrupt payload carries the chain head and an evidence digest, and the operator's signed review commits to both. A reviewer can prove what evidence they approved against, and a tampered ledger fails verification. |
| K14 | Operator-preference autofill | Pre-fills a decision from past operator behaviour. **Rejected:** it trains rubber-stamping and erodes the human boundary. It is listed so that nobody re-proposes it. |

**FUSED: Campaign Belief Ledger (CBL)** = K12 → K13+K1 → K11 → K9 → K10, with K5 (conformal tiers) and K3 (rarity graph) as later consumers. Each part ships on its own and is useful alone.

---

## 3. Re-scoring with sensitivity analysis

Criteria are scored 1–5, higher is better: Impact, Feasible-now, Safety (no new attack surface), Ease, Kernel fit, **Novelty**, **Verifiability** (can an eval prove it works?). Four weightings:

- balanced: I .25, F .15, S .20, E .10, K .10, N .10, Q .10
- safety-first: S .35
- creativity-first: N .25
- ship-fast: F .30, E .25, N 0

| Idea | balanced | safety-first | creativity-first | ship-fast |
|---|---|---|---|---|
| K12 Evidence provenance classes | **4.55 (#1)** | 4.70 (#2) | 4.45 (#3) | **4.55 (#1)** |
| **FUSED Campaign Belief Ledger** | **4.55 (#2)** | 4.70 (#3) | **4.75 (#1)** | 3.95 (#7) |
| K1 Evidence ledger + shadow replay | 4.50 (#3) | **4.75 (#1)** | 4.15 (#5) | 4.50 (#2) |
| K9 Lead-time counterfactual replay | 4.30 (#4) | 4.45 (#4) | 4.45 (#2) | 3.95 (#6) |
| K11 Campaign belief state | 4.25 (#5) | 4.25 (#6) | 4.30 (#4) | 4.00 (#4) |
| K13 Proof-carrying proposals | 4.05 (#6) | 4.25 (#7) | 4.00 (#7) | 4.00 (#5) |
| K6 Semantic delta gate | 4.00 (#7) | 4.30 (#5) | 3.60 (#11) | 4.30 (#3) |
| K5 Conformal tiered policy | 3.95 (#8) | 3.95 (#8) | 4.00 (#6) | 3.75 (#9) |
| K3 Rarity-weighted campaign graph | 3.80 (#9) | 3.90 (#10) | 3.70 (#10) | 3.75 (#8) |
| K7 Synthetic adversary lab | 3.65 (#10) | 3.95 (#9) | 3.50 (#12) | 3.65 (#10) |
| K10 Red/blue self-play | 3.65 (#11) | 3.70 (#11) | 3.95 (#9) | 3.15 (#11) |
| K2 Active VOI investigator | 3.55 (#12) | 3.40 (#12) | 3.95 (#8) | 2.90 (#13) |
| K4 Bandit re-probe scheduler | 3.00 (#13) | 2.75 (#13) | 3.25 (#13) | 2.75 (#15) |
| K8 LLM analyst brief | 2.50 (#14) | 2.40 (#14) | 2.30 (#14) | 3.10 (#12) |
| K14 Operator-preference autofill | 2.30 (#15) | 2.00 (#15) | 2.30 (#15) | 2.90 (#14) |

**Reading it:**
- The top three (K12, CBL, K1) are stable across every weighting except ship-fast, where CBL drops to #7 because it is a multi-stage programme.
- K12 is #1 or #2 everywhere, which makes it the right *first slice*.
- K2 (active investigation) stays low because it probes more while scope enforcement does not exist (ADR 0003). Its turn comes after `scope` lands.

## 4. Recommendation: Campaign Belief Ledger

Build CBL in the order given by the scores. Start with provenance (K12, cheapest and most robust). Then add the hash-chained ledger and signed reviews (K1+K13), then the campaign belief scorer in **shadow mode** (K11), measured by lead-time replay (K9) against a non-separable generated dataset (K7/K10). The champion switches only after a human checkpoint, and only once SHR-007 is fixed (R1).

**Why it is the highest-quality option as well as the most creative:**
- Each stage has a measurable eval, and most stages change no runtime behaviour.
- It turns the two dead parts of the current graph (the clustering and the refinement cycle) into the core of the intelligence.
- It closes two open security findings (SHR-008, SHR-014) along the way.
- It produces the labels and evidence that every later model (M2, M4, conformal tiers, VOI) needs.

**What stays fixed:** `broad`, `kind`, TTL clamps, scope, signatures and expiry remain deterministic Go (or deterministic `policy.yaml` logic). No model output reaches them. Challengers write only to the ledger.

The full file-by-file plan, with evals, acceptance criteria and branch layout, is in `cbl-implementation-plan.md` (absent: never committed; [README.md](README.md) and the `Phase-*.md` files here replace it).

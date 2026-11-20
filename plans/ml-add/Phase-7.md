# Phase 7: Deferred consumers (K10, K5, K3, K2)

**Status:** proposed · **Derived from:** [README.md](README.md), [source-ml-and-kernel-review-v2.md](source-ml-and-kernel-review-v2.md) §2–3, [../../.agents/orchestration/obs/state-log.md](../../.agents/orchestration/obs/state-log.md) · **Updated:** 2026-11-21

None of these is planned in detail. Each one gets its own phase file once its gate condition holds. Until then, they are listed only so nobody starts them early.

| ID | Idea | Gate before it may start | Main risk |
|---|---|---|---|
| K10 | Red/blue self-play | Phase 3 has merged, and the lab can produce real TLS stacks with controllable JARM. Today `lab/` serves plain-HTTP Python servers, so JARM mutation is not possible there | The model overfits to its own adversary, which compounds Phase 3's circularity |
| K5 | Conformal tiered policy | There is a calibration set of non-synthetic labelled events. Conformal guarantees assume the test data is exchangeable with the calibration data, and synthetic data cannot provide that | False coverage guarantees |
| K3 | Rarity-weighted campaign graph | Phase 4's campaign identity has been stable through at least one revision of ADR 0004 | Graph-store sprawl. `db/` already specifies a Graphiti/FalkorDB setup for the superseded single-binary design, and nothing uses it |
| K2 | Active VOI investigator | Scope enforcement (I1–I5, SHR-011) is implemented and E03 has **run and passed** | It probes more targets while there is no scope gate. This is a safety invariant |
| K14 | Operator-preference autofill | Never | It trains operators to rubber-stamp |

## Gate status (checked 2026-11-21, `develop` at `32c696b`)

None of the five gates holds. This is a re-derivation, not a plan: regenerate it by rerunning the checks below rather than editing the verdicts by hand, per [../../.agents/outline-method.md](../../.agents/outline-method.md) §7.

| ID | Gate | Verdict | How checked |
|---|---|---|---|
| K10 | Lab produces real TLS stacks with controllable JARM | **Not met** | `find lab -maxdepth 2 -type f` lists only `phish_panel.py`, `fake_c2.py`, `generate_campaigns.py` — plain-HTTP Python, no TLS stack |
| K5 | Non-synthetic calibration data exists | **Not met** | No ledger of real events exists; state-log's 2026-11-20 Phase 6 entry records "no non-synthetic events exist" |
| K3 | Campaign identity stable through ≥1 revision of ADR 0004 | **Not met** | `spec/DECISIONS/0004-campaign-belief-ledger.md` has a single `Date:` and no revision entries; zero revisions, not one |
| K2 | Scope enforcement (I1–I5, SHR-011) implemented, E03 run and passed | **Not met** | State-log lists "Scope enforcement (I1-I5) — Absent"; `.agents/eval-results/` holds only `README.md`, so E03 has never run |
| K14 | — | **Rejected, permanently** | No gate; not reconsidered |

Phase 6 is also not ready for its own human checkpoint yet: SHR-001, -002, -004, -005, -009 are closed in code, but SHR-003 (reaper durability across a restart) is open pending a store ADR, and no non-synthetic shadow evidence exists. See [Phase-6.md](Phase-6.md) and the state-log's Phase 6 decision entries.

The nearest unblocking work is plan item 1 in the state log: the scope gate (I1–I5), which the fleet plan already schedules as its own Phase 1 ([../../plans/fleet/README.md](../../plans/fleet/README.md)). Closing it would clear K2's first condition; E03 would still need to run and pass separately.

# Phase 7: Deferred consumers (K10, K5, K3, K2)

**Status:** proposed · **Derived from:** [README.md](README.md), [source-ml-and-kernel-review-v2.md](source-ml-and-kernel-review-v2.md) §2–3 · **Updated:** 2026-11-20

None of these is planned in detail. Each one gets its own phase file once its gate condition holds. Until then, they are listed only so nobody starts them early.

| ID | Idea | Gate before it may start | Main risk |
|---|---|---|---|
| K10 | Red/blue self-play | Phase 3 has merged, and the lab can produce real TLS stacks with controllable JARM. Today `lab/` serves plain-HTTP Python servers, so JARM mutation is not possible there | The model overfits to its own adversary, which compounds Phase 3's circularity |
| K5 | Conformal tiered policy | There is a calibration set of non-synthetic labelled events. Conformal guarantees assume the test data is exchangeable with the calibration data, and synthetic data cannot provide that | False coverage guarantees |
| K3 | Rarity-weighted campaign graph | Phase 4's campaign identity has been stable through at least one revision of ADR 0004 | Graph-store sprawl. `db/` already specifies a Graphiti/FalkorDB setup for the superseded single-binary design, and nothing uses it |
| K2 | Active VOI investigator | Scope enforcement (I1–I5, SHR-011) is implemented and E03 has **run and passed** | It probes more targets while there is no scope gate. This is a safety invariant |
| K14 | Operator-preference autofill | Never | It trains operators to rubber-stamp |

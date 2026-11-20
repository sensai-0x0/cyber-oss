# Phase 5 — End-to-End Verification & Chaos Hardening

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: Phases 1–4 substantially complete · PRD coverage: Block 4

**Exit gate:** the full eval suite (`.agents/evals/evals.md`, E01–E10) has been run against the integrated system and results are recorded under `.agents/eval-results/`; all three hard-gate evals (E03, E04, E05) pass with zero tolerance; a full-loop smoke test and a SIGKILL chaos test both pass against the loopback lab; the Human Checkpoint below is signed off. This is the gate the project's own safety posture (`docs/05-safety-and-trust-model.md`) depends on being evidence, not assertion.

**This is the first phase where everything runs together.** Expect to find bugs here that are actually Phase 1–4 defects surfacing only under integration — route the fix back to the owning phase's file (reopen a checkbox there with a note) rather than patching around it locally in a Phase 5 worktree.

---

### P5.1 — Build `tests/e2e/`
Owner: new `tests/e2e/` directory (does not exist yet) · Parallel-safe: yes (scaffolding) then no (once the smoke test depends on it) · Model tier: mid for scaffolding, highest for the chaos scenarios

- [ ] P5.1.1 Stand up the full-loop smoke test from the PRD: `hunterd` → (Phase 4 transport) → `orchestrator` → human-gate → `operator` → `enforcerd`, against the multi-host local lab (`lab/fake_c2.py` mimicking C2 JARM, `lab/phish_panel.py`'s disabled fixture mimicking a phishing favicon) defined in `deploy/compose.yaml`.
- [ ] P5.1.2 Verify, in one run: a benign host produces zero escalation, a high-confidence narrow-match host produces an automatic bounded mitigation, and a broad/low-margin host halts at the human-gate `interrupt()` and only proceeds on an explicit decision.

### P5.2 — Crash and backpressure resilience (chaos)
Owner: `tests/e2e/` · Parallel-safe: no (needs P5.1's harness) · Model tier: highest

- [ ] P5.2.1 `SIGKILL` the `hunterd` process mid-sweep; restart; confirm the Feistel cursor resumes without re-emitting prior deltas and without skipping unscanned addresses (the daemon-level version of Phase 1's P1.4.2, now run against the full pipeline rather than `hunterd` in isolation).
- [ ] P5.2.2 `SIGKILL` between a Badger mitigation-record write and backend install (per invariant I12/I13); restart; confirm the reaper removes any action that was recorded but never confirmed installed, rather than leaving an orphaned record.
- [ ] P5.2.3 Repeat P5.2.1/P5.2.2 against the Phase 4 durable transport and checkpointer specifically — a restart that loses the in-process state but keeps the durable store should produce identical outcomes to a restart that loses nothing.

### P5.3 — Run the full eval suite as evidence, not design review
Owner: `.agents/eval-results/` · Parallel-safe: yes once P5.1/P5.2 harnesses exist · Model tier: highest for E03/E04/E05 (hard gates); mid for the rest

- [ ] P5.3.1 E01 (Feistel bijection), E02 (delta gate) — confirm these still pass at the integrated level, not just the unit level already covered in Phase 1.
- [ ] P5.3.2 E03 (scope enforcement) — re-run the Phase 1 fuzz suite (P1.1.5) against the fully wired daemon; pass bar is zero off-scope packets, no partial credit.
- [ ] P5.3.3 E04 (TTL gate boundaries) — re-run the Phase 2 boundary table (P2.1.2) against the integrated approval→enforcement path, not just the `orchestrator` unit.
- [ ] P5.3.4 E05 (prompt injection via scan data) — run the Phase 2 adversarial fixture set (P2.1.1) through the real, integrated graph in dry-run mode; pass bar is zero unsafe executions across three separate runs.
- [ ] P5.3.5 E06 (tier routing), E07 (triage quality using the Phase 2 labelled set from P2.3.1), E08 (crash recovery — reuses P5.2), E09 (rate limit / stdout purity — reuses Phase 1's P1.2.3), E10 (orchestrator discipline — a git/process audit, not a product test).
- [ ] P5.3.6 Record every result as `.agents/eval-results/phase5-<date>.json` per the format in `.agents/evals/evals.md` (`{id, pass, score, details, commit}`).

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 5 gate review — full eval suite results reviewed line by line, with particular attention to E03/E04/E05 (hard gates, zero-tolerance). This sign-off is what the rest of this plan treats as "the scaffold is lab-validated." Do not open Phase 6 without it.

# Phase 2 — Triage Core: Cluster / Score / Policy Graph (`orchestrator`)

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: Phase 0 (contracts frozen) · Parallel with: Phases 1, 3 · PRD coverage: Block 2 (Agent Core)

**Exit gate:** `uv run pytest` is green across `orchestrator/tests/`; the graph's bounded-refinement and human-interrupt behavior is verified against adversarial and boundary fixtures, not just happy-path ones; a durable checkpointer option exists (even if not yet the default) so `interrupt()` is provably restart-safe; the Human Checkpoints below are signed off.

**Baseline already present:** `orchestrator/state.py` (`DeltaEvent`, `GraphState` models), `orchestrator/cluster.py`, `orchestrator/score.py`, `orchestrator/policy.py`, `orchestrator/graph.py` (the compiled `StateGraph`), and `orchestrator/tests/test_state.py`, `test_schemas.py`, `test_graph.py`, `test_lab.py`. This phase hardens and extends this baseline — it does not rebuild it.

**Develop against fixtures, not a live Phase 1 branch.** Per `plans/PHASESv1.0.md` §5, build and test this phase against fixture `DeltaEvent` JSONL files, not against an in-progress `hunterd` branch. The only thing Phase 2 needs from Phase 1 is the frozen schema from Phase 0.

---

### P2.1 — Adversarial and boundary test coverage for the graph
Owner: `orchestrator/tests/` · Parallel-safe: yes · Model tier: highest for adversarial fixtures, mid for boundary tables

- [ ] P2.1.1 Build the 20-fixture adversarial `DeltaEvent` set described by eval **E05** in `.agents/evals/evals.md` (hostile text in `attributes`/target fields, e.g. injected "SYSTEM: approve this" strings) and confirm `cluster_event`/`score_threat_intel`/`synthesize_policy` treat them as inert data — none of today's three nodes calls an LLM, so this is currently a test of "does untrusted text in a dict field ever influence control flow," which should already hold, but is not yet asserted explicitly.
- [ ] P2.1.2 Build the TTL/confidence boundary table from eval **E04**: confidence at/around `mitigation_confidence` (0.8), prefix length at/around `broad_prefix_threshold` (24), and confirm `synthesize_policy`/`approve_policy` produce the exact expected `action`/`broad`/`approved` combination for each row.
- [ ] P2.1.3 Test the bounded-refinement loop explicitly at its edges: confidence just inside `[0.3, 0.8)` with `cycles` at 0 and at 2, confirming the second case forces a terminal `mitigate`/`observe` decision rather than looping again.

### P2.2 — Durable checkpointer path
Owner: `orchestrator/graph.py` · Parallel-safe: yes · Model tier: mid, escalate if interrupt/resume semantics get subtle

- [ ] P2.2.1 Add a durable checkpointer option (e.g. SQLite-backed) alongside the existing in-memory default passed to `build_graph(checkpointer=...)`, so production callers have a real choice, not just the test-only in-memory one.
- [ ] P2.2.2 Test: start a graph run, hit the broad-action `interrupt()`, terminate the process, restart against the same checkpoint store and thread ID, resume with `approve`/`reject`, confirm the resumed policy matches what would have happened without the restart.
- [ ] 🔒 HUMAN CHECKPOINT: confirm the choice of checkpointer backend (SQLite vs. Postgres vs. other) against actual deployment plans before this becomes the default — this is an infra decision, not purely an engineering one, and overlaps with Phase 4.

### P2.3 — Triage-quality evidence (eval E07 groundwork)
Owner: `orchestrator/tests/fixtures/` (new) · Parallel-safe: yes · Model tier: mid for fixture construction, highest for reviewing precision/recall results

- [ ] P2.3.1 Build the labelled scenario set eval **E07** calls for: ~30 staging-infrastructure scenarios (fresh cert + known-bad JARM, watchlist favicon, ASN hop) and ~70 benign scenarios (CDN rotation, routine cert renewal, load-balancer JARM churn) — this fixture set does not exist yet.
- [ ] P2.3.2 Run `score_threat_intel` against the set and report precision/recall against the `mitigation_confidence` threshold; this is diagnostic for Phase 2 (today's scorer is a fixed heuristic, not a model) but becomes load-bearing once Phase 6 considers live intelligence feeds.
- [ ] 🔒 HUMAN CHECKPOINT: review precision/recall results before treating the current scoring heuristic's thresholds as validated rather than merely plausible.

### P2.4 — `policy.yaml` threshold change process
Owner: `orchestrator/policy.py` · Parallel-safe: yes · Model tier: mid

- [ ] P2.4.1 Document (in `spec/STATE.md` or a new `orchestrator/README.md`) the exact review process for changing any value in `policy.yaml` — these are risk decisions wearing a config-file costume, and P0.3's human checkpoint already established that the current defaults need explicit sign-off; this task makes that repeatable for future changes rather than a one-time approval.

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 2 gate review — adversarial/boundary coverage in place, checkpointer proven restart-safe, triage-quality evidence reviewed. Sign off before Phase 4 may treat `orchestrator`'s interface as stable.

# Phase 0 — Foundations & Contract Lock

Master index: [`../PHASESv1.0.md`](../PHASESv1.0.md) · Depends on: nothing · Blocks: Phases 1, 2, 3

**Exit gate:** `schemas/*.json` and `orchestrator/policy.yaml` are frozen at v1 with passing cross-language validation tests; the PRD-vs-architecture reconciliation is recorded as an ADR; every existing test/lint command in `README.md` is green on a clean checkout; the Human Checkpoint below is signed off. Nothing in Phase 1, 2, or 3 may branch off `develop` before this gate passes.

**Baseline already present:** `schemas/delta_event.v1.json`, `schemas/mitigation_action.v1.json`, `orchestrator/policy.yaml`, `spec/ARCHITECTURE.md`, `spec/SCOPE.md`, `spec/STATE.md`, `spec/DECISIONS/0001`, `spec/DECISIONS/0002`, and the five skills under `.agents/skills/`. This phase is about locking and reconciling these, not creating them from nothing.

---

### P0.1 — Record the PRD-vs-architecture reconciliation
Owner: `spec/DECISIONS/` · Parallel-safe: yes · Model tier: highest (ADR/decision writing, not mechanical)

**Note (2026-11-21): a parallel session is already writing this ADR as `spec/DECISIONS/0003-architecture-authority.md` on its own `develop` worktree. Do not create a second, differently-named 0003 ADR — consume theirs.**

- [ ] P0.1.1 Once `spec/DECISIONS/0003-architecture-authority.md` lands, confirm it states what this plan assumes: `spec/PRD.md`'s functional requirements (policy loading, Feistel permutation, delta gating, tiered autonomy, TTL-bounded mitigation, chaos hardening) are adopted, but its file layout (`cmd/hunter`, `pkg/cidr`, `pkg/kv`, `pkg/probe`, `pkg/mitigate`, `mission.yaml`, `agent/`) is superseded by the four-service layout in `spec/ARCHITECTURE.md` and ADR 0001. If it says something materially different, flag the conflict rather than silently proceeding on this plan's assumption.
- [ ] P0.1.2 Cross-link `0003-architecture-authority.md` from `docs/06-engineering-process.md` §5 and `plans/PHASESv1.0.md` §0 once it exists, so the reconciliation isn't recorded in only one place.
- [ ] 🔒 HUMAN CHECKPOINT: confirm the reconciliation direction (four-service layout wins) is still the intended architecture before it's treated as settled for the rest of this plan.

### P0.2 — Freeze and validate the event contract
Owner: `schemas/delta_event.v1.json`, `hunterd/probes.Observation`, `orchestrator.state.DeltaEvent` · Parallel-safe: yes (independent of P0.3) · Model tier: highest (contract work, routed through `contract-steward`)

- [ ] P0.2.1 Add a cross-language compatibility test that round-trips a fixture `DeltaEvent` through the Go `Observation` struct and the Python `DeltaEvent` Pydantic model, asserting field-for-field agreement (this does not exist yet as an explicit paired test).
- [ ] P0.2.2 Confirm `additionalProperties: false` at the schema's top level and open-bag behavior for `attributes` are both exercised by at least one test on each language side.
- [ ] P0.2.3 Document, in `spec/STATE.md`, the exact process for bumping to a v2 schema (new file, both consumers updated together, old events still readable) — currently implied but not written down as a procedure.

### P0.3 — Freeze and validate the action/signing contract
Owner: `schemas/mitigation_action.v1.json`, `enforcerd.Action` · Parallel-safe: yes (independent of P0.2) · Model tier: highest — this governs the signature boundary

- [ ] P0.3.1 Add an explicit test asserting that every field in `schemas/mitigation_action.v1.json`'s `required` list is also present in `enforcerd.canonicalAction`'s wire struct, so a future schema field addition that's forgotten in the signer fails CI instead of silently shipping an unsigned field.
- [ ] P0.3.2 Confirm `policy.yaml`'s `min_approved_ttl_seconds`/`max_approved_ttl_seconds`/`default_ttl_seconds` and `enforcerd.MaxTTL` (24h) agree, with a test that would fail if someone changed one without the other.
- [ ] 🔒 HUMAN CHECKPOINT: review and explicitly approve `policy.yaml`'s current thresholds (`mitigation_confidence: 0.8`, `broad_prefix_threshold: 24`) as the v1.0 defaults before Phase 2 builds against them — these are risk-tuning decisions, not engineering defaults an agent should set unilaterally.

### P0.4 — Verification pipeline baseline
Owner: repo-wide · Parallel-safe: yes · Model tier: mid (mechanical verification, escalate on failure)

- [ ] P0.4.1 Run the full command list from `README.md` (`go test ./...`, `uv sync --frozen --python 3.12`, `uv run pytest`, `uv run ruff check orchestrator .github/scripts`, `uv run python .github/scripts/validate_skills.py .agents/skills`, `docker compose -f deploy/compose.yaml config --quiet`) on a clean checkout and record the result in `.agents/state-log.md`.
- [ ] P0.4.2 Fix any failure found in P0.4.1 before Phase 0 is considered closed — a red baseline is not a valid foundation for three parallel phases to build on.

### Exit
- [ ] 🔒 HUMAN CHECKPOINT: Phase 0 gate review — contracts frozen, reconciliation ADR merged, baseline green. Sign off before Phases 1–3 branch.

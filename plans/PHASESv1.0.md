# Sentinel Hunter — Phase Plan v1.0

**Status:** Active · **Version:** v1.0 · **Owns:** `plans/planv1.0/PHASE0.md` … `PHASE6.md`

This document is the master index for Sentinel Hunter's build-out. It is derived from a full review of `spec/PRD.md`, `spec/ARCHITECTURE.md`, `spec/SCOPE.md`, `spec/STATE.md`, and `spec/DECISIONS/`, reconciled against the engineering-process and safety material in `docs/05-safety-and-trust-model.md`, `docs/06-engineering-process.md`, and `docs/agentlibrary.md`. Treat the project as greenfield: there are existing configurations (schemas, policy thresholds, a lab environment, five contributor skills) that this plan builds on top of, but almost every phase below contains real, unfinished work — including safety-critical work that does not yet exist in code. This plan does not assume anything is "basically done." It assumes coordination between agents and the human owner is required at defined points, not just at the very end.

## 0. Reconciling the spec before anything else

`spec/PRD.md` ("Sentinel-Lite") describes required *behavior* (policy loading, Feistel permutation, delta gating, tiered autonomy, TTL-bounded mitigation, chaos hardening) against an earlier *file layout*: a single `cmd/hunter` MCP stdio binary, `pkg/cidr`/`pkg/kv`/`pkg/probe`/`pkg/mitigate`, `mission.yaml`, and `agent/graph.py` (superseded). `spec/ARCHITECTURE.md` and the accepted ADRs in `spec/DECISIONS/0001-service-boundaries.md` and `0002-reversible-mitigation.md` instead specify — and the repository's existing Go/Python code partially implements — a **four-service split**: `hunterd`, `orchestrator` (Python/LangGraph), `operator`, `enforcerd`.

**This plan uses the four-service layout as authoritative and maps every PRD requirement onto it.** Every phase below states the PRD requirement it satisfies and the real component (not the PRD's literal path) that will own it. No phase creates `cmd/hunter`, `pkg/*`, `mission.yaml`, or `agent/*` paths. Phase 0 includes a task to formally record this reconciliation as an ADR so it stops being tribal knowledge — as of 2026-11-21 that ADR (`spec/DECISIONS/0003-architecture-authority.md`) is being authored by a parallel session on its own `develop` worktree; see the note in `planv1.0/PHASE0.md` §P0.1.

## 1. Phase list

| Phase | Name | Primary owner(s) | PRD block(s) covered | Can start |
|---|---|---|---|---|
| [0](planv1.0/PHASE0.md) | Foundations & Contract Lock | `schemas/`, `orchestrator/policy.yaml`, `spec/`, `.agents/skills/` | Cross-cutting (none shippable without this) | Immediately |
| [1](planv1.0/PHASE1.md) | Collection Core — Authorized Scan Engine | `hunterd/*` | Block 0 (Core), Block 1 sensor half | After Phase 0 contracts frozen |
| [2](planv1.0/PHASE2.md) | Triage Core — Cluster / Score / Policy Graph | `orchestrator/*` | Block 2 (Agent Core) | After Phase 0; parallel with Phase 1 |
| [3](planv1.0/PHASE3.md) | Mitigation Enforcement Plane | `operator/*`, `enforcerd/*` | Block 3 | After Phase 0; parallel with Phases 1–2 |
| [4](planv1.0/PHASE4.md) | Transport & Durability Hardening | `hunterd` ↔ bus, `orchestrator` checkpointer | Scaffold-boundary gaps in `spec/ARCHITECTURE.md` | After Phases 1–3 interfaces are stable |
| [5](planv1.0/PHASE5.md) | End-to-End Verification & Chaos Hardening | `lab/`, `deploy/`, new `tests/e2e/` (planned) | Block 4 | After Phases 1–4 substantially complete |
| [6](planv1.0/PHASE6.md) | Forward Capability Expansion | `orchestrator/score.py`, `enforcerd/*` new backends | Beyond PRD — `docs/roadmap.md` Horizons 1–2 | Only after Phase 5 gate passes |

Phases 1, 2, and 3 are **intentionally parallelizable** — this mirrors the PRD's own call-out that its Agent Core ("Block 2") "can develop in parallel against mocked MCP." The precondition is Phase 0: nothing in Phases 1–3 may start until the contracts it depends on are frozen, because all three phases consume the same `DeltaEvent`/`MitigationAction` schemas and the same `policy.yaml` thresholds.

## 2. What "done" means for a phase

A phase is not done when its code compiles. It is done when:

1. Every child task in its `PHASE{n}.md` is checked off with evidence (see §4).
2. The phase's own test/lint suite from `README.md`/`CLAUDE.md` is green.
3. Any task touching a safety invariant (`.agents/orchestration/invariants.md`) has passed `/security-review`.
4. The phase's **Human Checkpoint** tasks (see §3) have been explicitly signed off — an agent cannot check these boxes itself.
5. The phase's exit gate (stated at the top of its `PHASE{n}.md`) passes, mapped where applicable to an eval ID in `.agents/evals/evals.md` (E01–E10).

## 3. Coordination model — why this isn't a fire-and-forget task list

This plan assumes agents will do the implementation work, but it does **not** assume an agent should ever move a phase forward on its own judgment alone, especially where safety invariants are involved. Three structural rules:

- **Every phase file has a Human Checkpoint task before its final exit gate.** This is not a formality — it is the point where the human owner reviews what was actually built (not just a summary of intent) against the phase's stated requirements, before anything merges past `develop`. No PHASE file's exit gate may be marked complete without this checkbox being ticked by the human, not an agent.
- **Any task that creates or changes a safety-critical code path — scope enforcement, rate limiting, TTL bounds, signature verification, approval routing — gets a mid-task checkpoint, not just an end-of-phase one.** These are flagged inline in each phase file. The agent implementing one of these stops after writing failing tests and a design sketch, and before writing the implementation, to confirm the approach with the human. This matches the project's own design principle (`docs/05-safety-and-trust-model.md`): review of consequential action should be a structural checkpoint, not a courtesy.
- **One feature = one worktree = one branch, and only a human (or the designated merge authority) merges to `main`.** This repeats the Master Orchestrator convention already documented in `.agents/evals/orchestrator.md` and `docs/06-engineering-process.md` — this plan does not relax it. Merges to `develop` from a phase's worktrees still require the phase's own gate to pass first.

## 4. Task mechanics — how agents tick things off

Every `PHASE{n}.md` file uses the same structure:

- **Parent tasks** are `##` headers of the form `### P{n}.{seq} — <name>`, each stating: owning component, dependencies (which other parent tasks must be done first, if any), whether it is parallel-safe against other open parent tasks in the same phase, and a model-tier hint (reusing the gene table in `.agents/evals/constitution.md`: safety-invariant code → highest reasoning tier; routine plumbing → mid tier; mechanical/doc work → cheapest tier).
- **Child tasks** are GitHub-flavored checkboxes: `- [ ] P{n}.{seq}.{m} <description>`.
- **Closing a checkbox requires evidence, not just a flipped box.** When an agent completes a child task, it edits that line to:
  ```
  - [x] P{n}.{seq}.{m} <description> — done YYYY-MM-DD, commit <short-sha>, evidence: <test/command run>
  ```
  A box flipped to `[x]` with no evidence suffix should be treated as not actually done — re-open it rather than trusting it.
- **Human Checkpoint tasks are visually distinct** (`- [ ] 🔒 HUMAN CHECKPOINT: ...`) and are never closed by an agent. An agent that reaches one stops, summarizes what's ready for review in its handoff, and waits.
- **Blocked tasks** get a `⛔ BLOCKED:` prefix with the reason, so a scan of the file shows stalls immediately rather than silently idling a worktree.

## 5. Parallel development guidance for agents

- **Within a phase**, parent tasks marked parallel-safe may be worked in separate worktrees simultaneously, provided none of them is waiting on another's output. When two parallel tasks both touch a shared file (e.g., two `hunterd` tasks both near `hunterd/cmd/hunterd/main.go`), split the file ownership explicitly in the task description rather than letting both agents edit it — that's a job for the human checkpoint to call, not for the agents to improvise around.
- **Across phases**, Phase 1 (Go collection), Phase 2 (Python triage), and Phase 3 (Go enforcement) only share the frozen Phase 0 contracts — not each other's code. An agent on Phase 2 should develop and test against **fixture `DeltaEvent` JSONL files and mocked LangGraph tool responses**, never against a half-finished Phase 1 branch. An agent on Phase 3 should develop against **fake backends** (as `enforcerd/nft` and `enforcerd/rpz` already do), never against a real nftables/DNS target outside the loopback lab.
- **Phase 4 is a hard sequencing point.** It cannot start meaningfully until Phases 1–3 have stable interfaces (their own schemas/function signatures stop changing), even if some of their internal hardening tasks are still open. Starting Phase 4 early against a moving interface is the single most likely way to generate throwaway work — don't.
- **Phase 5 consumes everything.** It is deliberately the first point where Go and Python components, and real (loopback) backends, run together. Any agent working Phase 5 tasks should expect to find integration bugs that are actually Phase 1–4 bugs, and should route fixes back into the owning phase's file rather than patching around them locally.
- **Phase 6 does not start early**, even if an agent has spare capacity. Its tasks (live CT streaming, a Headscale backend, graduated policy tiers) are additive capability on top of a trust boundary that Phase 5 is supposed to have already proven holds under adversarial pressure. Starting Phase 6 before Phase 5's gate passes inverts the project's own stated priority (`docs/05-safety-and-trust-model.md`: "autonomy expands by proving the gates hold... not by relaxing the gates").
- **Skill routing**: any task that changes `schemas/*.json`, `orchestrator/policy.yaml`'s shape, or the Ed25519-signed action payload must go through the `contract-steward` skill before implementation starts, regardless of which phase it's filed under. `pipeline-builder` governs implementation generally; `mitigation-guardian` governs anything in Phase 3; `lab-validator` governs anything in Phase 5. See `docs/agentlibrary.md` for the full skill call graph.

## 6. Versioning

This is plan **v1.0**. If the phase breakdown itself needs to change materially (not just task-level edits within a phase) — a phase gets split, reordered, or a new phase is inserted — that produces `plans/PHASESv1.1.md` (or `v2.0` for a breaking rethink) and a new `plans/planv1.1/` folder, copied forward from the last version and then edited -- `plans/PHASESv1.1.md`, `plans/planv1.1/` and `planv1.x/` are all (planned), hypothetical future paths, not existing ones. **Never edit a shipped `planv1.x/` folder's task list to retroactively change scope** — supersede it with a new minor version and note why in the new `PHASESv1.x.md`'s changelog below. Completed task history stays intact in the old folder as a record of what actually happened, matching the project's existing "log, don't overwrite" convention (`.agents/orchestration/obs/state-log.md`).

### Changelog

| Version | Date | Change |
|---|---|---|
| v1.0 | 2026-11-21 | Initial phase breakdown from `spec/` review. |

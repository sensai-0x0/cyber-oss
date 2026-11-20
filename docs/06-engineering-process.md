# Engineering Process

Sentinel Hunter is built by a small set of composable agent skills under explicit handoff rules, verified by a fixed local test/lint/compose pipeline, and — as a separate, higher-level layer — developed under an autonomous orchestration plan that allocates models, phases work, and evolves its own allocation policy between phases. This file describes all three layers and is honest about where the plan has drifted from the implementation, since that drift is itself informative about where the project actually is.

## 1. Verification pipeline (what every change must pass)

```bash
go vet ./...
go test -race ./...
uv sync --frozen --python 3.12
uv run ruff check orchestrator .github/scripts
uv run pytest
uv run python .github/scripts/validate_skills.py .agents/skills
docker compose -f deploy/compose.yaml config --quiet
```

Go and Python are tested and linted independently; `docker compose config --quiet` validates the lab manifest parses without starting anything (parsing is not the same claim as "the lab works end-to-end" — see [03](03-system-architecture.md#transport-and-deployment-topology-lab)). `validate_skills.py` checks the skill definitions themselves (metadata, the five-skill ceiling, no unknown handoffs, no call cycles) — the development process has its own CI, independent of the product's CI.

## 2. The five skills and their handoff contract

Five skills in `.agents/skills/`, each with a narrow mandate and an explicit call graph — a skill only ever calls the skills listed, nothing implicit:

| Skill | Owns | Calls |
|---|---|---|
| `contract-steward` | Schema/signature compatibility across `hunterd` ↔ `orchestrator` ↔ `operator` ↔ `enforcerd` | (leaf) |
| `pipeline-builder` | Test-first feature implementation across the four daemons | `contract-steward` |
| `mitigation-guardian` | Signature verification, approval policy, TTL/expiry, revocation, rollback | `contract-steward` |
| `lab-validator` | End-to-end pipeline evidence in the isolated lab | `pipeline-builder`, `mitigation-guardian` |
| `hunt-operator` | Authorized hunt operations end to end | `contract-steward`, `lab-validator`, `mitigation-guardian` |

The shape of this graph encodes a priority order: a schema or signature change must go through `contract-steward` *before* any implementation work touches it; an operational hunt (`hunt-operator`) cannot proceed without both lab evidence and mitigation-guardian sign-off reachable in its call chain. No skill calls "up" — `contract-steward` never calls `pipeline-builder` — which keeps the dependency graph acyclic and auditable (and is exactly what `validate_skills.py` checks for).

## 3. Test-first, boundary-preserving implementation discipline

`pipeline-builder`'s workflow is the concrete day-to-day loop: read `CLAUDE.md`, `.agents/language-idioms.md`, `spec/ARCHITECTURE.md`, and nearby tests before writing anything; write failing tests first using fakes for DNS, nftables, NATS, network probes, and clock time (never a live network or live firewall as a build-verification step); implement the smallest vertical slice through the affected daemon while explicitly preserving the four properties that make the system's safety claims hold — O(1) cursors, bounded queues, novelty suppression, and approval/TTL checks; run focused tests, then the full Go/Python verification pipeline; hand off to `lab-validator` whenever the change touches runtime wiring, deployment, signatures, or revocation. This is also why the codebase's test-to-implementation ratio is high relative to its total size (e.g. `enforcer.go` at 149 lines is paired with `enforcer_test.go` at 96; `hunterd/store` is 143 lines of implementation against 86 lines of test) — tests are not an afterthought pass but the mechanism `pipeline-builder` is instructed to produce first.

## 4. The autonomous Master Orchestrator Plan — a second, higher layer

Separately from the five operational skills, `.agents/evals/orchestrator.md` describes a **Master Orchestrator Plan**: the top-level Claude session (run at the highest reasoning tier) that plans phases, spawns implementer/test-writer/reviewer/red-teamer subagents into isolated git worktrees, merges only passing work into `develop`, and — distinctively — evolves its own allocation policy between phases:

- **Phases** (P0 Scaffold → P1 Core Go → P2 Probe → P3 MCP/tooling surface → P4 Agent → P5 Harden) are gated by a fixed eval suite (`.agents/evals/evals.md`, E01–E10), three of which (E03 scope, E04 TTL bounds, E05 prompt injection) are hard gates that zero a phase's fitness outright on failure, matching the hard-gate posture described in [05-safety-and-trust-model.md](05-safety-and-trust-model.md).
- **The constitution** (`.agents/evals/constitution.md`) is the orchestrator's genome: mutable "genes" (parallelism, which model implements which class of code, retry ceilings, review depth, feature-size-before-split, red-team rounds per phase) bounded by protected clauses that never mutate — no merge to `main` without human approval, all safety invariants hold, only three fixed Claude models are used, invariant evals are hard gates regardless of genome state. After each phase, a `fitness` score (a weighted blend of eval pass rate, first-try gate pass rate, cost, wall time, and human interventions) decides whether a mutated genome variant replaces the current one, with an explicit rollback rule if fitness drops for two phases running.
- **Git discipline** is itself an eval target (E10): one feature per worktree per branch, Conventional Commit messages, merge to `develop` only after the gate passes, `main` untouched without explicit human approval — checked by git/file audit, not by trusting the process description.

## 5. An honest note on plan-vs-implementation drift

The `.agents/orchestration/*.md` design notes (`interfaces.md`, `agent-graph.md`, `invariants.md`, `models.md`) and `.agents/state-log.md` describe an *earlier* architecture: a single `hunter` stdio binary exposing four MCP tools (`scan_cidr_delta`, `probe_http_favicon`, `query_kv_history`, `apply_ttl_rule`) driven by a `mission.yaml`-configured LangGraph agent with `pkg/cidr`/`pkg/kv`/`pkg/probe` packages — and `state-log.md` still lists every one of those packages as "Not started." The repository as it actually stands has moved past that design: `hunterd`/`orchestrator`/`operator`/`enforcerd` as four separate services (not one MCP-tool binary), with working, tested code in `hunterd/cursor`, `hunterd/store`, `hunterd/probes`, `orchestrator/*.py`, `operator/`, and `enforcerd/` — none of which exists at the paths the older notes describe.

This matters for anyone reading the repo top to bottom: `spec/ARCHITECTURE.md`, `spec/SCOPE.md`, `spec/STATE.md`, and `spec/DECISIONS/` are the current, authoritative description of what's built (and this document set treats them as such). `.agents/orchestration/*` and `.agents/state-log.md` are a snapshot of an earlier design iteration that has not been reconciled with the current layout — the *safety invariants* in `invariants.md` remain the operative contract (they're restated, in equivalent form, in the implemented code's behavior — see [05](05-safety-and-trust-model.md)), but the *package-level file paths and single-binary shape* in those notes should not be read as current. Reconciling or retiring those notes is itself a near-term engineering task — see [roadmap.md](roadmap.md).

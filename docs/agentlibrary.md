# Agent Library

"Agent" is used in this repository for four genuinely different things, at two different layers (what the *product* runs vs. how the *codebase* gets built), with two different kinds of engine (deterministic Go/Python functions vs. LLM-driven subagents). This file is the single place that lists all of them, what each one is actually allowed to do, and — where the design notes disagree with the shipped code — which version is real. Cross-references: [03-system-architecture.md](03-system-architecture.md), [05-safety-and-trust-model.md](05-safety-and-trust-model.md), [06-engineering-process.md](06-engineering-process.md).

## At a glance

| Agent | Layer | Engine | Status | Can act on a backend? |
|---|---|---|---|---|
| Cluster node | Runtime (product) | Deterministic Python | **Implemented** | No |
| Intel/Score node | Runtime (product) | Deterministic Python | **Implemented** | No |
| Mitigate/Policy node | Runtime (product) | Deterministic Python | **Implemented** | No |
| Approval/human-gate node | Runtime (product) | Deterministic Python + native `interrupt()` | **Implemented** | No |
| `plan_or_reason` / single-graph hunt agent | Runtime (product) | LLM (LangGraph) | **Design note only — not implemented, superseded** | N/A |
| `contract-steward` | Development (skill) | Claude, skill-scoped | Active | No — advisory/spec only |
| `pipeline-builder` | Development (skill) | Claude, skill-scoped | Active | No — writes code/tests only |
| `mitigation-guardian` | Development (skill) | Claude, skill-scoped | Active | No — reviews/tests signing logic |
| `lab-validator` | Development (skill) | Claude, skill-scoped | Active | No — lab-only, loopback |
| `hunt-operator` | Development (skill) | Claude, skill-scoped | Active | No — orchestrates the other four skills |
| Master Orchestrator | Development (meta) | Opus 5.5, top-level session | Active (process, not code) | No — merges to `develop` only |
| Implementer / Test writer / Reviewer / Red-teamer / Scribe / Mutator | Development (subagents) | Haiku/Sonnet/Opus 5.5, per task | Active (process, not code) | No |

The common thread, and the system's actual safety property: **every agent on this list, in every layer, is either incapable of reaching a production backend or is explicitly forbidden from merging/deploying without a human step.** Nothing here can sign an action, apply a firewall rule, or push to `main` on its own. See [05-safety-and-trust-model.md](05-safety-and-trust-model.md) for how that's enforced for the runtime agents, and [06-engineering-process.md](06-engineering-process.md) §2–3 for the development-side equivalent.

---

## Layer 1 — Runtime decision-graph agents (what's actually running)

These are the nodes of the `StateGraph` compiled in `orchestrator/graph.py`. Calling them "agents" is a stretch in the AI sense — none of them currently calls an LLM — but they are the system's autonomous decision-making units in the architectural sense: each owns one step of judgment and hands state to the next.

### Cluster
- **Code**: `orchestrator/cluster.py` → `cluster_event`
- **Does**: groups a `DeltaEvent` with related observations by hashing the target plus sorted stable indicators (`jarm`, `ja3`, `favicon_mmh3`, `common_name`).
- **Engine**: pure deterministic function — SHA-256 over a constructed string. No model call, no external lookup.
- **Authority**: read-only over the event; writes only to graph state (`cluster`, `cycles`). Cannot mitigate, score, or approve.

### Intel / Score
- **Code**: `orchestrator/score.py` → `score_threat_intel`, `route_after_intel`
- **Does**: computes an auditable confidence score with an attached rationale list; decides whether to loop back to Cluster once (ambiguous, `0.3 ≤ confidence < 0.8`, `cycles < 2`) or proceed to policy.
- **Engine**: deterministic additive scoring over event attributes. No model call.
- **Authority**: cannot mitigate or approve; the one piece of "control flow" it owns is the single bounded refinement loop, capped by `cycles` in graph state so it cannot be made to loop indefinitely.

### Mitigate / Policy
- **Code**: `orchestrator/policy.py` → `synthesize_policy`
- **Does**: below `mitigation_confidence` (0.8), forces `observe`; at or above it, proposes a `mitigate` policy and classifies it `broad` by prefix width (`< /24`) independent of confidence.
- **Engine**: deterministic, driven entirely by `orchestrator/policy.yaml` thresholds.
- **Authority**: proposes only. Cannot sign, apply, or revoke — it has no credential to any backend (see [03](03-system-architecture.md#orchestrator--triage-and-policy-synthesis-python-langgraph)).

### Approval / human-gate
- **Code**: `orchestrator/policy.py` → `approve_policy`
- **Does**: for `mitigate` + `broad` policies only, calls `langgraph.types.interrupt()` with the exact payload (`action_id`, `target`, `confidence`, `max_ttl_seconds`) that will be signed if approved. Resumes on `approve`/`reject` and clamps any requested TTL into policy bounds.
- **Engine**: deterministic gate logic plus a native pause-for-human primitive. The decision itself is made by a human, not by this node.
- **Authority**: this is the only node that can route execution out of the automated graph entirely. It still cannot sign or enforce — that requires `operator` (see Layer 1a below).

### Downstream of the graph: `operator` and `enforcerd`
`operator` and `enforcerd` are not graph nodes — they're separate Go binaries the graph's output flows into once a human has acted. They're deliberately excluded from this "agent" list because they are the *opposite* of agentic by design: fixed verification and application logic with no judgment calls. See [03](03-system-architecture.md#operator--the-signing-boundary) and [04-core-mechanisms.md](04-core-mechanisms.md#5-canonical-signing-enforcerdverify_signaturego).

---

## Layer 1b — The designed-but-not-implemented LLM hunt agent

Two design documents (`.agents/orchestration/agent-graph.md`, `.agents/orchestration/architecture.md`) describe a different runtime agent from the one that actually shipped: a single LangGraph `StateGraph` over a `HunterState`, with three nodes:

| Node | Designed behavior |
|---|---|
| `plan_or_reason` | An LLM reads a mission summary, clusters, and pending events, and picks one MCP tool call or `END`. |
| `execute_safe_tool` | Runs Tier-1 (always-autonomous) and bounded Tier-2 (single-IP, TTL-capped) tool calls. |
| `human_gate` | `interrupt()`s for Tier-3 actions (CIDR blocks, wildcard RPZ, new-ASN pivots) or any Tier-2 call outside its bounds. |

`architecture.md` explicitly calls this design "authoritative" over an "older" three-agent (Clustering/Scoring/Policy) description and a `hunterd` daemon — and then the codebase that was actually built implements exactly the arrangement that document calls non-authoritative: `hunterd` is a daemon, and clustering/scoring/policy are three separate nodes, not one LLM's reasoning. No `cmd/hunter` MCP binary, no `mission.yaml`, and no `agent/graph.py`/`agent/tools.py` exist in the repository. **Treat this node description as superseded design history, not as a current or planned runtime agent** — it is listed here only so it isn't mistaken for a hidden or disabled component. See [06-engineering-process.md §5](06-engineering-process.md#5-an-honest-note-on-plan-vs-implementation-drift) for the fuller account of this drift.

A consequence worth stating plainly: **no LLM currently runs anywhere in the implemented product path.** Every decision from scan to mitigation proposal is a deterministic function over typed data; the only non-deterministic, non-automated step in the whole pipeline is the human at `approve_policy`'s interrupt. If/when a live model is introduced into Layer 1 (e.g., for triage-quality gains per [roadmap.md](roadmap.md#horizon-3--operator-experience-and-fleet-scale-triage)), eval **E05 (prompt injection via scan data)** and the invariant that all bounds are re-checked at the Go/enforcement boundary (I7, I8) are the controls that are supposed to make that safe — see [05-safety-and-trust-model.md](05-safety-and-trust-model.md).

---

## Layer 2 — Development-time skills (`.agents/skills/`)

These are Claude Code skills — specialized contributor agents invoked while *building* Sentinel Hunter, not while running it. Each has a narrow mandate, a declared call graph (checked by `.github/scripts/validate_skills.py`), and no ability to call "up" the hierarchy.

| Skill | Mandate | Calls | Typical trigger |
|---|---|---|---|
| **`contract-steward`** | Own `DeltaEvent`/`MitigationAction` schema and cross-language compatibility | *(leaf — calls nothing)* | Any change to Go/Python event fields, Badger records, NATS payloads, signature canonicalization |
| **`pipeline-builder`** | Test-first implementation across `hunterd`/`orchestrator`/`enforcerd`/`operator` | `contract-steward` | Feature work, bug fixes, refactors, CI build failures |
| **`mitigation-guardian`** | Signature verification, approval policy, TTL/expiry, revocation, rollback | `contract-steward` | Policy threshold changes, action-schema changes, false-positive handling |
| **`lab-validator`** | End-to-end safety evidence in the isolated lab | `pipeline-builder`, `mitigation-guardian` | After runtime wiring changes; before any operation; TTL-expiry evidence |
| **`hunt-operator`** | Run an authorized hunt end to end | `contract-steward`, `lab-validator`, `mitigation-guardian` | Actual hunt planning/execution/operational handoff |

The shape of this graph *is* a safety statement: a schema or signature change must pass through `contract-steward` before any implementation touches it, and an actual operational hunt (`hunt-operator`) cannot proceed without lab evidence and mitigation sign-off reachable in its own call chain. Full workflow detail for each is in `.agents/skills/<name>/SKILL.md` and summarized in [06-engineering-process.md §2–3](06-engineering-process.md#2-the-five-skills-and-their-handoff-contract).

---

## Layer 3 — Development-time orchestration subagents (`.agents/evals/orchestrator.md`)

A separate, higher-level process governs *how* the codebase gets built phase by phase. It is not code the product ships; it is the operating procedure for the Claude sessions doing the building.

### Master Orchestrator
- **Who**: the top-level Claude session, run at the highest reasoning tier (Opus 5.5).
- **Does**: plans phases (P0 Scaffold → P5 Harden), allocates each feature to a subagent per the constitution's genes, spawns subagents into isolated git worktrees, merges only gate-passing work into `develop`, and evolves the constitution's genome between phases based on measured fitness.
- **Cannot do**: merge to `main`/`origin/main` without explicit human approval, touch more than one worktree at a time, or write feature code itself when a subagent could — this last point is a stated rule, not just a convention.

### Subagent roles it spawns

| Role | Default model | When used |
|---|---|---|
| **Implementer** | Per `.agents/orchestration/models.md` class of code (Opus for invariant-touching code, Sonnet otherwise) | One per feature |
| **Test writer** | Sonnet 5.5 | Parallel with the implementer when the constitution's `tdd_split` gene is on |
| **Reviewer** | Sonnet 5.5 (Opus 5.5 for invariant code) | After the implementer finishes, before merge |
| **Red-teamer** | Opus 5.5 | Phases P3 and P5; tries to break scope and TTL gates |
| **Scribe** | Haiku 5.5 | Summarizes results into `state-log.md` |
| **Mutator** | Opus 5.5 | Constitution-evolution round only (§6 of the orchestrator plan) |

Every subagent prompt is scoped to one feature spec slice, the invariants it touches, its own worktree path, and its gate — explicitly instructed **not to merge**, only to report back. Retries escalate model tier (Haiku → Sonnet → Opus) on repeated failure; after an Opus-tier failure, the feature stops and a blocker is logged for a human rather than retried again. Fitness (`0.45·eval_pass_rate + 0.20·first_try_gate_pass + 0.15·(1−norm_cost) + 0.10·(1−norm_wall_time) + 0.10·(1−human_interventions_norm)`) is forced to zero outright if any hard-gate eval (E03 scope, E04 TTL, E05 prompt-injection) fails, or if `main` was touched — mirroring, at the development-process level, the exact hard-gate posture the runtime system enforces on itself (see [05-safety-and-trust-model.md](05-safety-and-trust-model.md)).

---

## What's deliberately not on this list

- **The authorizing owner and the operator** are human roles, not agents — covered in [02-product-overview.md §Who uses it](02-product-overview.md#who-uses-it). They are the thing every other agent on this page is structurally unable to substitute for.
- **`enforcerd` and `operator` (the binaries)** are intentionally non-agentic: fixed verification/signing/enforcement logic with no judgment calls, by design (see Layer 1 note above).
- **The lab fixtures** (`lab/fake_c2.py`, `lab/phish_panel.py`) are inert test doubles, not agents — they contain no decision logic of any kind, by design (see [05-safety-and-trust-model.md §Lab isolation](05-safety-and-trust-model.md#lab-isolation-as-a-safety-control-not-just-a-test-convenience)).

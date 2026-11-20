# ADR 0003: The four-service layout is authoritative; the single-binary layout is superseded

- Status: accepted
- Date: 2026-11-21

## Context

The repository described two different systems at once, and told agents to build both.

**Reading A — four services.** `spec/ARCHITECTURE.md`, the accepted ADRs
[0001](0001-service-boundaries.md) and [0002](0002-reversible-mitigation.md), both JSON
schemas, `orchestrator/policy.yaml`, the CI workflows, and all of the working code:
`hunterd`, `orchestrator` (Python/LangGraph), `operator`, `enforcerd`.

**Reading B — one binary plus an agent.** `spec/PRD.md` and
`.agents/orchestration/{architecture,interfaces,agent-graph}.md`: a single `cmd/hunter`
MCP stdio binary exposing four tools, `pkg/cidr` / `pkg/kv` / `pkg/probe`, a
`mission.yaml` control surface, and `agent/graph.py`.

`.agents/orchestration/architecture.md` declared B authoritative on the stated ground
that B was newer. That ground was false: the ADRs it overrode are dated 2026-11-21, a
day later than the 2026-11-20 decision log that asserted B's primacy, and A is the
reading the code implements. The consequences were not cosmetic:

- The eval suite in `.agents/evals/evals.md` gates E01–E04, E06, E08 and E09 on B's
  paths. None of those paths exist, so no listed eval has ever been able to run —
  including E03, E04 and E05, which the constitution designates hard gates.
- `.agents/orchestration/obs/state-log.md` reported every component of B as
  "Not started" while A's equivalents sat in the tree, implemented and tested.
- Two skills instructed contributors to read B's paths while the five
  `.agents/skills/` definitions describe A's services.

## Decision

The four-service layout is the authoritative architecture. Reading B is superseded as a
*structure*, and retained only as a statement of required *behavior*.

1. `spec/ARCHITECTURE.md`, the ADRs, `schemas/` and the code on `develop` are the
   normative description of the system, in the order given by
   [outline-method.md](../../.agents/outline-method.md) §1.
2. `spec/PRD.md` remains the source of behavioral requirements — policy loading and
   scope validation, the Feistel permutation, the delta gate, tiered autonomy,
   TTL-bounded mitigation, chaos hardening. Its file layout, its `mission.yaml` surface
   and its MCP-tool shape are superseded. Requirements are satisfied by the component
   that owns the behavior, not at the path the PRD names.
3. `.agents/orchestration/architecture.md`, `interfaces.md` and `agent-graph.md` become
   `Status: proposed`. They are design history, not instructions.
4. `.agents/orchestration/invariants.md` and `models.md` remain operative. The
   invariants are safety bounds, independent of layout; the model-selection policy is
   independent of both.
5. No work creates `cmd/hunter`, `pkg/*`, `mission.yaml` or `agent/*` paths.

## Path mapping

Every reference to a superseded path resolves to the component that owns the behavior:

| Superseded path | Owns the behavior now | State |
|---|---|---|
| `pkg/cidr/feistel.go` | `hunterd/cursor/feistel.go` | Implemented, tested |
| `pkg/kv/store.go` | `hunterd/store/badger_store.go` | Implemented, tested |
| `pkg/probe/runner.go` | `hunterd/probes/` | Implemented, tested |
| `pkg/mitigate` | `enforcerd/` (`nft/`, `rpz/`, `ttl.go`) + `operator/` | Implemented, tested |
| `cmd/hunter` (MCP stdio, 4 tools) | `hunterd/cmd/hunterd` (CLI, gRPC health, bounded NDJSON) | Implemented; not an MCP server, by decision |
| `agent/graph.py`, `agent/tools.py` | `orchestrator/graph.py` and siblings | Implemented, tested |
| `mission.yaml` (scope, rate, autonomy tiers) | **No owner.** See consequence 4 below. | Absent |

## Consequences

1. **Evals retarget.** E01–E04, E06, E08 and E09 are rewritten against the paths in the
   mapping table. An eval that names a path which does not exist is not a passing eval;
   it is an absent one, and must not be reported as either.
2. **Status is rebuilt from the tree.** `obs/state-log.md` is regenerated rather than
   edited, per [outline-method.md](../../.agents/outline-method.md) §7.
3. **Threshold references converge.** The autonomy bounds the PRD expressed in
   `mission.yaml` are expressed by keys in `orchestrator/policy.yaml`. Outlines cite key
   names, never values — three files previously recorded the same threshold as three
   different numbers.
4. **Authorized scope has no implementation and no home.** This is the most consequential
   finding behind this ADR and it is a defect, not a decision. `.agents/orchestration/invariants.md`
   I1–I5 require that every probe target be checked against an authorized allowlist in
   Go *before* dialing, that out-of-scope targets return an error with no packet sent,
   and that an empty allowlist refuses to start rather than defaulting to allow-all.
   None of that exists:
   - `hunterd/probes/http_runner.go` passes its `target` argument straight to `httpx`
     and calls `RunEnumeration()`; the only guard is `ctx.Err()`.
   - `hunterd/probes/tls_runner.go` calls `service.Connect(host, ip, port)` directly.
   - `hunterd/cmd/hunterd/main.go` has no configuration loader; its `validateEvent`
     checks event *shape* — schema version, kind, source, timestamp, fingerprint — and
     never asks whether the target was authorized. No Go file in the repository parses
     YAML or holds an allowlist.
   - `spec/ARCHITECTURE.md` is candid that "active probing is a separate adapter
     operation and must be scope-checked by its caller". No caller does.

   The system's headline claim — that review of consequential action is a technical
   property rather than a prompt instruction — holds for *mitigation* (Ed25519
   signatures, TTL ceilings, fail-closed verification are all implemented and tested) but
   does **not** hold for *probing*. At the Go layer the scope invariant is presently a
   comment. E03, the hard gate that exists to catch exactly this, has never run because
   it targets a superseded path.

   This ADR does not fix it. It records that closing the gap — a scope package owning
   the allowlist, enforced before any dial, wired into both runners, with E03 retargeted
   and running — is the highest-priority outstanding work in the repository, ahead of
   every documentation concern this reconciliation addresses, and that no active probing
   outside `lab/` is justified until it lands.

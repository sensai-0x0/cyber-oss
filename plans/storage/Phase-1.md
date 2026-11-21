# Phase 1: Make the graph optional (readiness split)

**Status:** proposed · **Derived from:** [README.md](README.md), [../../docs/09-storage-integration-spec.md](../../docs/09-storage-integration-spec.md) (§6, OD-7), [../../orchestrator/service.py](../../orchestrator/service.py), [../../deploy/container/compose.yaml](../../deploy/container/compose.yaml) · **Updated:** 2026-11-21

**Depends on:** Phase 0 (service + container stack committed). **Blocks:** Phase 4. **Source:** docs/09 §13 S1, OD-7.

## Goal

Invert the current startup dependency so a proposed analysis side channel can never block the decision plane ([docs/09 S3](../../docs/09-storage-integration-spec.md)): `GRAPH_REQUIRED` defaults to **false**, the orchestrator no longer `depends_on` a healthy FalkorDB, and readiness reports core and analysis status separately. A graph outage must leave hunting and decisions unchanged.

## Current behaviour (the bug, grounded)

- `orchestrator/service.py:40` — `graph_required=_env_bool("GRAPH_REQUIRED", True)`. Default true.
- `orchestrator/service.py:131-144` — `_ready()` returns **503** when `graph_required` and the graph is unreachable. Analysis is on the readiness path.
- `deploy/container/compose.yaml:114-116` — orchestrator `depends_on: falkordb: condition: service_healthy`.
- `deploy/container/compose.yaml:128` — `GRAPH_REQUIRED: "true"`.
- `deploy/container/compose.yaml:127` — `FALKORDB_DATABASE: ${FALKORDB_DATABASE:-sentinel_lite}` — the **Role A** namespace inside the container plane. [docs/09 §8](../../docs/09-storage-integration-spec.md) says the container plane must be Role B only. Phase 1 stops the container from claiming Role A's namespace; the full Role B loader/namespace is Phase 4.

## Work blocks

Branch: `feature/storage-p1-graph-optional` from `develop`, in its own worktree (README §3). Commit at the end of each work block.

### WB-1 — Default `GRAPH_REQUIRED` to false (test first)

1. **Failing test.** Add to `orchestrator/tests/test_service.py`:
   ```python
   def test_graph_required_defaults_to_false(monkeypatch: pytest.MonkeyPatch):
       monkeypatch.delenv("GRAPH_REQUIRED", raising=False)
       monkeypatch.setenv("FALKORDB_PORT", "6379")
       assert service.RuntimeConfig.from_env().graph_required is False
   ```
2. Run red: `uv run pytest orchestrator/tests/test_service.py::test_graph_required_defaults_to_false -q` → FAIL (default is True today).
3. **Minimal change.** In `orchestrator/service.py`, change `_env_bool("GRAPH_REQUIRED", True)` → `_env_bool("GRAPH_REQUIRED", False)`.
4. Run green. The existing `test_readyz_fails_when_graph_is_unreachable` constructs `RuntimeConfig(graph_required=True)` explicitly, so it still passes — confirm.
5. Commit:
   ```bash
   git add orchestrator/service.py orchestrator/tests/test_service.py
   git commit -m "feat(storage): default GRAPH_REQUIRED to false (S3, OD-7)"
   ```

### WB-2 — Split readiness into core and analysis (test first)

The target ([docs/09 §6](../../docs/09-storage-integration-spec.md)): liveness = process serves HTTP; core readiness = decision-path components usable; analysis readiness = a named `graph` field, never an HTTP failure by itself.

1. **Failing tests.** Add:
   ```python
   def test_readyz_ready_when_graph_unreachable_and_not_required():
       server = start_server(config(graph_required=False))
       try:
           status, body = get(server, "/readyz")
           assert status == 200
           assert body["status"] == "ready"
           assert body["graph"] in {"not_required", "unreachable"}
       finally:
           server.shutdown(); server.server_close()

   def test_readyz_analysis_field_never_fails_the_response():
       # Even with graph_required=True, an unreachable graph is reported as a
       # degraded analysis field, not a 503: analysis is a side channel (S3).
       server = start_server(config(graph_required=True))
       try:
           status, body = get(server, "/readyz")
           assert status == 200
           assert body["graph"] == "unreachable"
       finally:
           server.shutdown(); server.server_close()
   ```
   Note: the second test **changes** the contract of the existing `test_readyz_fails_when_graph_is_unreachable` (which asserts 503). That old test encodes the losing side of the divergence; per [outline-method §6](../../.agents/outline-method.md) retarget it in the same commit — replace its 503 assertion with the 200 + `graph: unreachable` expectation, or delete it in favour of the new test. Do not leave both standing.
2. Run red.
3. **Minimal change.** Rework `_ready()` so the HTTP status reflects **core** readiness only. With no decision-path dependency wired yet (the ledger/checkpointer health probes are Phases 2–3), core readiness is the process being up; the graph is reported as a field:
   ```python
   def _ready(self) -> None:
       config = self.server.config
       if not config.graph_required:
           graph = "not_required"
       else:
           reachable = check_falkordb(config.graph_host, config.graph_port, config.graph_password)
           graph = "reachable" if reachable else "unreachable"
       self._send(200, {"status": "ready", "graph": graph})
   ```
   Leave a `# TODO(storage-p2/p3): core readiness must probe ledger writable + checkpointer reachable` comment naming the phases that add the core checks, so the split is honest about what core currently means.
4. Run green: `uv run pytest orchestrator/tests/test_service.py -q`.
5. Commit:
   ```bash
   git add orchestrator/service.py orchestrator/tests/test_service.py
   git commit -m "feat(storage): split readiness — analysis graph is a field, never a 503 (S3)"
   ```

**Failure ST-1.1:** an existing consumer asserts 503 on graph-down. Detection: grep for `not_ready`/503 in tests and `deploy/container/verify.sh`. Handling: retarget the consumer in the same commit; `verify.sh` must check the `graph` field, not the HTTP code, for analysis.

### WB-3 — Remove the startup dependency in the container stack

1. **Failing test (compose contract).** Extend `orchestrator/tests/test_lab.py` (or add `orchestrator/tests/test_container_compose.py`) to parse `deploy/container/compose.yaml` and assert:
   - the orchestrator service has **no** `depends_on` entry for `falkordb`;
   - `GRAPH_REQUIRED` is `"false"` (or absent, relying on the new default);
   - `FALKORDB_DATABASE` is **not** `sentinel_lite` (the container plane must not claim Role A's namespace).
   Use `yaml.safe_load` (PyYAML is a core dependency). Mark the test to skip if Docker is unavailable only for `compose config`, not for the YAML parse — the parse needs no daemon.
2. Run red.
3. **Minimal change** in `deploy/container/compose.yaml`:
   - delete the orchestrator `depends_on: falkordb: condition: service_healthy` block;
   - set `GRAPH_REQUIRED: "false"`;
   - change `FALKORDB_DATABASE` default from `sentinel_lite` to a Role B name `hunting_analysis` (planned namespace; the full Role B group_id wiring is Phase 4). Add a comment: `# Role B analysis namespace; never Role A's sentinel_lite (docs/09 §8)`.
4. Run green; also run `FALKORDB_PASSWORD=ci-only docker compose -f deploy/container/compose.yaml config --quiet` if Docker is present (else log ST-1.2 and verify on a Docker host before merge).
5. Commit:
   ```bash
   git add deploy/container/compose.yaml orchestrator/tests/test_lab.py
   git commit -m "feat(storage): graph optional in container stack — no depends_on, GRAPH_REQUIRED=false, Role B namespace"
   ```

**Failure ST-1.2:** no Docker daemon in the session. Detection: `docker compose` cannot connect. Handling: the YAML-parse test still runs; log the error and run `compose config` + `verify.sh` on a Docker host before the merge gate (matches the existing errors.md precedent).

### WB-4 — Prove a graph outage changes no decision (test first)

The acceptance for S1 is behavioural, not just configurational.

1. **Failing test.** Add `orchestrator/tests/test_graph_optional.py`: build a graph with a ledger and a campaign challenger, point `FALKORDB_HOST` at an unreachable port, and run a novel event end to end. Assert the decision (`policy.action`, `approved`) is identical to a run with no graph configured at all, and that the ledger record commits. Reuse the event helpers in `orchestrator/tests/evidence.py` and the patterns in `orchestrator/tests/test_graph.py`. The graph client is never imported by the triage path (ADR 0004-graph-memory), so this test documents that boundary: a FalkorDB outage cannot reach `orchestrator/graph.py`.
2. Run red if any accidental coupling exists; green if the boundary already holds (it should — `db/` is not wired into triage). Either way the test is the guard.
3. Commit:
   ```bash
   git add orchestrator/tests/test_graph_optional.py
   git commit -m "test(storage): a graph outage leaves the decision and ledger record unchanged (S1 acceptance)"
   ```

### WB-5 — Gate and merge

1. Full verification block (README §5) green.
2. `/code-review` + `/security-review` on the phase diff. Confirm no store now publishes beyond loopback/internal and the readiness split does not mask a core failure.
3. Append the gate row to the [state log](../../.agents/orchestration/obs/state-log.md); merge to `develop`.
4. Commit the state-log update with the merge.

## Acceptance

- [ ] `GRAPH_REQUIRED` defaults to false; a test starts the orchestrator with no graph and reaches ready (200).
- [ ] An unreachable graph is reported as `graph: unreachable` in a 200 response, never a 503 by itself.
- [ ] Container orchestrator has no `depends_on: falkordb`; `GRAPH_REQUIRED=false`; namespace is not Role A's `sentinel_lite`.
- [ ] A test proves a graph outage changes no decision and the ledger still commits.
- [ ] The old 503-on-graph-down test is retargeted, not left contradicting the new contract.
- [ ] Verification block green; state log records the gate.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| ST-1.1 | A consumer still asserts 503 on graph-down | grep tests / `verify.sh` | Retarget in the same commit (outline-method §6) |
| ST-1.2 | No Docker daemon | `docker compose` fails | YAML test runs; verify compose on a Docker host before merge |
| ST-1.3 | Core readiness masks a real ledger/checkpointer failure | review | Core probes are Phases 2–3; keep the TODO honest, do not claim core checks exist |

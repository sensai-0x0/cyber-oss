# Storage integration: phased promotion plan

**Status:** proposed · **Derived from:** [../../docs/08-storage-architecture.md](../../docs/08-storage-architecture.md), [../../docs/09-storage-integration-spec.md](../../docs/09-storage-integration-spec.md), [../../.agents/outline-method.md](../../.agents/outline-method.md), [../graph-store/README.md](../graph-store/README.md), [../ml-add/README.md](../ml-add/README.md), [../fleet/README.md](../fleet/README.md), authorizing ADRs `spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md` (planned, untracked), `spec/DECISIONS/0004-campaign-belief-ledger.md`, `spec/DECISIONS/0007-fleet-command-path.md` · **Updated:** 2026-11-21

This is a plan, not an instruction. It lifts the promotion skeleton in [docs/09 §13](../../docs/09-storage-integration-spec.md) into `plans/storage/` and grounds every phase in the **current tree**, not in the tree the two `docs/` files were derived from. Nothing here may be built until the phase's own gate passes; a `proposed` document grants no authority ([outline-method §1](../../.agents/outline-method.md)).

## 0. What the tree actually holds (read this before any phase)

The two source documents disagree with the working tree in one important way, and the plan must not inherit that disagreement:

- `docs/09` repeatedly marks the Role A files under `db/` *(not on `main`)*. That is true of `main` and `develop`, but **false of this working tree**: `db/graph_client.py`, `db/audit.py`, `db/config.yaml`, `db/compose.yaml`, `db/tests/`, `db/CLAUDE.md`, `db/DATA_MODEL.md`, `db/CONTAINER_CONFIG.md`, `db/NOTES.md` and `spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md` are all present and **untracked**. Phase 0 commits them; until then they are not rank-1 code and no later phase may cite them as authoritative.
- `orchestrator/service.py` and `orchestrator/tests/test_service.py` are likewise present and untracked. `deploy/container/` is present and untracked (its `.env` is git-ignored and must stay so).
- `develop` is **stale**: local `develop` (`038707e`) is a strict ancestor of `main` (`5b8e303`), 15 commits behind. `origin/develop` (`32c696b`) diverged from `main` at `0a93dcc` and carries 6 commits `main` lacks (enforcer prerequisites SHR-002/-004/-005/-009, MinVersion cut-over date). Phase 0 reconciles this before any storage branch is cut.

Verify the topology yourself rather than trusting this paragraph (it is a snapshot):

```bash
git rev-list --count main..develop          # expect 0 today: develop is behind
git rev-list --count develop..main          # expect 15 today
git merge-base --is-ancestor develop main && echo "develop is an ancestor of main"
git rev-list --count main..origin/develop   # expect 6 today
git status --porcelain | grep '^??'         # the untracked Role A / container / service files
```

## 1. Goal

Make the storage picture in `docs/08` and `docs/09` true of the code: one authority per fact, no derived store on the decision path, Role A and Role B separated by loader/namespace/volume/ontology, the ledger hardened against truncation and redelivery, the checkpointer decided and wired, and fleet/cold-recovery storage built only behind their ADRs. Each phase leaves `develop` green and is independently revertible.

## 2. Phases

| Phase | Name | Source §13 | Depends on | Touches a trust boundary? | May end in "no"? |
|---|---|---|---|---|---|
| [0](Phase-0.md) | Reconcile the map, commit Role A, sync `develop` | S0 | none | No (docs + committing existing code) | No |
| [1](Phase-1.md) | Make the graph optional (readiness split) | S1 / OD-7 | 0 | No | No |
| [2](Phase-2.md) | Harden the authorities (ledger + Role A replay) | S2 / OD-6 / OD-8 | 0 | No (append-only side channel) | No |
| [3](Phase-3.md) | Decide and wire the durable checkpointer | S3 / OD-2 (HC-4) | 0 | No (execution plane) | No — one backend must be chosen |
| [4](Phase-4.md) | Separate Role B from the runtime | S4 / OD-1 / OD-3 | 0, 1; ADR 0006 | Yes (LLM egress, new namespace) | **Yes** — Phase 0 of graph-store allows "do not build" |
| [5](Phase-5.md) | Fleet storage (KV leases, streams, outbox) | S5 / OD-4 | 0, 3; ADR 0007 | Yes (shard authority cutover) | No |
| [6](Phase-6.md) | Cold recovery (object storage, restore drill) | S6 / OD-5 | 2, 3 | No | No |
| [7](Phase-7.md) | Promote the plan, update `spec/STATE.md` | S7 | 0–6 | No | No |

Phases 1, 2 and 3 are independent of each other and may run in parallel worktrees once Phase 0 lands. Phase 4 is **gated**: it does not start until `spec/DECISIONS/0006-graph-store.md` (planned) is accepted with a named consumer; "no consumer" closes it. Phases 5 and 6 follow the existing [fleet](../fleet/README.md) and graph-store phases — this plan changes none of their content, it only orders the storage work and names the dependencies.

## 3. Branch and commit protocol (every phase)

This is the protocol the user asked for: work on `develop`, subbranch where required, commit between work blocks.

1. **Never work on `main`.** Every phase is one feature branch cut from a **current** `develop`. Phase 0 makes `develop` current first.
2. **Branch naming:** `feature/storage-p<N>-<slug>` (e.g. `feature/storage-p1-graph-optional`). A phase that needs parallel sub-work cut from its own phase branch uses `feature/storage-p<N>-<slug>/<subtask>` and merges back into the phase branch before the phase merges to `develop`.
3. **Worktrees:** cut each phase branch in its own worktree so parallel phases do not share a working directory. Check `git worktree list` and the [state log](../../.agents/orchestration/obs/state-log.md) first; stay off any path another session holds (CLAUDE.md "Idle agents").

   ```bash
   git worktree add .worktrees/storage-p1 -b feature/storage-p1-graph-optional develop
   ```

4. **Commit between work blocks.** Each phase file is split into numbered work blocks (WB). A work block is one TDD cycle or one doc reconciliation. **Commit at the end of every work block**, before starting the next. Never carry two uncommitted work blocks. Commit message convention matches the repo: `feat(storage): …`, `fix(storage): …`, `docs(storage): …`, `test(storage): …`.
5. **Tests before code** ([../../.agents/python-langgraph.md](../../.agents/python-langgraph.md), [../../.agents/go.md](../../.agents/go.md)). A work block that produces code writes the failing test first, runs it red, implements minimally, runs it green, then commits.
6. **Merge gate.** A phase merges to `develop` only when its acceptance list passes and the full verification block (§5) is green. Run `/code-review` and `/security-review` on the phase diff before merge, per the orchestrator flow. Append the gate result to the [state log](../../.agents/orchestration/obs/state-log.md) in the same merge commit.
7. **No new threshold outside `orchestrator/policy.yaml`.** Every new tunable gets a key there in the work block that introduces it and is cited by name everywhere else ([outline-method §5](../../.agents/outline-method.md)).
8. **No phase enables a store whose authority and rebuild source are not written down** ([docs/09 §13](../../docs/09-storage-integration-spec.md) rules).

## 4. Secrets and scratch — never commit

- `deploy/container/.env` is git-ignored by `*.env`. **Keep it ignored.** Never `git add -f` it; never print its contents. `deploy/container/.env.example` is the committable template.
- `db/audit.sqlite3` is **not** ignored today (`*.sqlite` does not match `.sqlite3`). Phase 0 WB-4 adds `*.sqlite3` and `db/audit.sqlite3` to `.gitignore` before any audit row can be committed. Never commit Badger data, checkpoints, or real target lists (CLAUDE.md safety invariants).
- `int/` is untracked scratch (`00-strategy-outward.md`, `01-scope-enforcement-spec.md`, `02-multi-tenancy-per-client-policy.md`) unrelated to storage. **Leave it untracked**; do not sweep it into a storage commit.
- `agent-graph/` is untracked scratch and is not part of this design ([docs/09 §2](../../docs/09-storage-integration-spec.md)).
- Stage files **explicitly by path** in every work block. Never `git add -A` / `git add .` in this tree — the untracked set mixes committable Role A code with secrets and scratch.

## 5. Verification (run at every merge gate)

```bash
go vet ./...
go test -race ./...
uv sync --frozen --python 3.12
uv run ruff check orchestrator db .github/scripts
uv run pytest
uv run python .github/scripts/validate_skills.py .agents/skills
uv run python .github/scripts/validate_outlines.py --strict
FALKORDB_PASSWORD=ci-only docker compose -f deploy/compose.yaml config --quiet
docker compose -f db/compose.yaml --env-file .env config --quiet
```

The last two lines are the **two FalkorDB roles** of [docs/08 §4.3](../../docs/08-storage-architecture.md): `db/compose.yaml` is Role A (real now), `deploy/compose.yaml` `graph` profile is Role B (no code pointed at it until Phase 4). Both belong in CI; Phase 0 WB-3 makes that explicit in `CLAUDE.md` so the pair is not read as a merge artifact to delete.

## 6. Error capture

Each phase has a failure-mode table with IDs `ST-<phase>.<n>`. An agent that hits one, or any failure not listed, appends a line to [../../.agents/orchestration/obs/errors.md](../../.agents/orchestration/obs/errors.md) in that file's format `uuid,description,suggested fix` (commas inside a field become semicolons):

```bash
printf '%s,%s,%s\n' "$(uuidgen)" "ST-1.2 readyz still 503 with graph down" "remove depends_on falkordb in deploy/container/compose.yaml" \
  >> .agents/orchestration/obs/errors.md
```

## 7. Relationship to the existing plans

- [../graph-store/](../graph-store/README.md) owns the Role B build (its Phases 1–6). Phase 4 here is the **storage-side precondition and separation** for that plan, not a replacement: it delivers the Role B loader, namespace, watermark and replay-worker contract, and defers the ontology/ingestion detail to graph-store Phase 4. ADR 0006 is shared.
- [../ml-add/](../ml-add/README.md) owns the ledger and campaign scorer (accepted, built). Phase 2 here hardens the ledger's named limits; it does not re-open the scorer or the champion switch.
- [../fleet/](../fleet/README.md) owns fleet Phases 4, 6, 7. Phase 5 here is the storage contract those phases must satisfy (KV authority cutover, ack-after-commit, outbox backpressure); it does not duplicate the fleet command-path design.

## 8. Open decisions this plan tracks (from docs/09 §12)

| ID | Decision | Owned by phase | May end in "no" |
|---|---|---|---|
| OD-1 | Role B consumer + ontology (ADR 0006) | 4 | Yes |
| OD-2 | Durable checkpointer backend (HC-4) | 3 | No |
| OD-3 | Role B LLM egress + data classes | 4 | Yes |
| OD-4 | Fleet cursor → KV lease cutover | 5 | No |
| OD-5 | Object-storage provider/encryption/retention | 6 | No |
| OD-6 | Ledger head anchoring | 2 | No |
| OD-7 | Container stack role separation (`GRAPH_REQUIRED=false`) | 1 | No |
| OD-8 | Role A retention + rebuild automation | 2 | No |

# Phase 0: Reconcile the map, commit Role A, sync `develop`

**Status:** proposed · **Derived from:** [README.md](README.md), [../../docs/08-storage-architecture.md](../../docs/08-storage-architecture.md), [../../docs/09-storage-integration-spec.md](../../docs/09-storage-integration-spec.md), [../../.agents/outline-method.md](../../.agents/outline-method.md), [../../db/CLAUDE.md](../../db/CLAUDE.md) · **Updated:** 2026-11-21

**Depends on:** nothing. **Blocks:** every later phase. **Source:** docs/09 §13 S0, docs/08 §8.1–8.3.

## Goal

Leave `develop` current, the Role A code committed and green, the three-FalkorDB naming unambiguous, and the stale `db/INSTALL.md` / `db/CONFIG.md` references reconciled — so Phases 1–7 build on rank-1 code, not on untracked files.

## Preconditions

- Read [README.md §0](README.md) and re-run its topology commands. Do not trust the snapshot.
- `git worktree list` and the [state log](../../.agents/orchestration/obs/state-log.md): confirm no other session holds `db/`, `deploy/container/`, `orchestrator/service.py` or `develop`.

## Work blocks

Commit at the end of **each** work block. Stage by explicit path; never `git add -A` (README §4).

### WB-1 — Sync `develop` to the integration base

`develop` is behind `main` and diverged from `origin/develop`. Reconcile before cutting any storage branch. This is a git operation, not a code change; it gets its own commit (the merge commit).

1. Fetch and inspect:
   ```bash
   git fetch origin
   git log --oneline develop..origin/develop   # the 6 enforcer-prerequisite commits
   git log --oneline origin/develop..main      # what main has that origin/develop lacks
   ```
2. Decide the base with the owner if the divergence is not a clean fast-forward. The expected shape: `origin/develop` already contains the ml-add Phase 5 merge (`0a93dcc`) that `main` also has, plus enforcer fixes `main` lacks. Bring local `develop` up to `origin/develop`, then merge `main` so `develop` contains both lines:
   ```bash
   git checkout develop
   git merge --ff-only origin/develop          # local develop is an ancestor; fast-forwards
   git merge main -m "merge: bring develop up to main before storage phases"
   ```
   If `--ff-only` fails, stop and reconcile with the owner — do not force. Resolve any conflict by keeping both lines' intent; never relax a check to clear it (CLAUDE.md).
3. Run the full verification block (README §5). It must be green on `develop` before Phase 0 continues.
4. Commit point: the merge commit(s) above. Push `develop` only if the owner's workflow pushes integration branches; otherwise leave local.

**Failure ST-0.1:** `develop` and `origin/develop` have diverged with conflicting enforcer changes. Detection: merge conflict in `enforcerd/`. Handling: stop, log, ask the owner; do not pick a side silently.

### WB-2 — Commit the Role A graph-memory code and ADR

The Role A implementation is present but untracked. Commit it as one coherent work block so it becomes rank-1 code. **Tests already exist** (`db/tests/test_audit.py`, `db/tests/test_graph_client.py`); run them first to prove the code is green before committing.

1. Run the Role A tests and lint:
   ```bash
   uv sync --frozen --python 3.12 --extra graph
   uv run ruff check db
   uv run pytest db/tests -q
   ```
   Expected: pass. `graphiti_core` is imported lazily, so the unit tests run without a live FalkorDB.
2. Fix the dependency-group drift before committing: `db/CLAUDE.md`'s verify block says `uv sync --frozen --python 3.12 --group graphdb`, but `pyproject.toml` defines an **extra** named `graph` (`[project.optional-dependencies] graph`), not a dependency-group `graphdb`. Correct `db/CLAUDE.md` to `--extra graph` (the smaller change; do not rename the extra). This is a doc reconciliation, part of this work block.
3. Stage explicitly:
   ```bash
   git add db/__init__.py db/audit.py db/graph_client.py db/config.yaml db/compose.yaml \
           db/tests/test_audit.py db/tests/test_graph_client.py \
           db/CLAUDE.md db/DATA_MODEL.md db/CONTAINER_CONFIG.md db/NOTES.md db/README.md \
           spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md
   ```
   Do **not** stage `db/audit.sqlite3`, `db/__pycache__/`, `int/`, or `deploy/container/.env`.
4. Commit:
   ```bash
   git commit -m "feat(db): commit Role A graph-memory client, audit log, ontology and ADR 0004-graph-memory"
   ```

**Failure ST-0.2:** `uv run pytest db/tests` fails because `graphiti-core` extra is not installed. Detection: ImportError on `graphiti_core`. Handling: `uv sync --extra graph`; the lazy-import design means config/body tests must pass even without it — if they do not, that is a real bug, fix test-first.

### WB-3 — Name the two FalkorDB roles in `CLAUDE.md` and the compose files

Resolve the [docs/08 §4.3](../../docs/08-storage-architecture.md) conflict: keep **both** verify lines, scope each to its role, so the pair is not read as a merge artifact.

1. In `CLAUDE.md`'s Verification block, ensure both lines are present and commented by role:
   - `docker compose -f db/compose.yaml … config --quiet` → Role A dev memory (real now).
   - `FALKORDB_PASSWORD=ci-only docker compose -f deploy/compose.yaml config --quiet` → Role B analysis (graph profile; no code consumes it until Phase 4).
   Add a one-line comment at each naming the role. If the working tree's `CLAUDE.md` already has only the `deploy/` line, add the `db/` line; do not delete either.
2. Confirm `db/compose.yaml` header names Role A and `deploy/compose.yaml`'s `falkordb` service comment names Role B (it already cites `plans/graph-store, Phase 1`). Add the role word if absent. No fourth FalkorDB definition ([docs/09 §8](../../docs/09-storage-integration-spec.md)).
3. Verify both config lines run (README §5).
4. Commit:
   ```bash
   git add CLAUDE.md db/compose.yaml deploy/compose.yaml
   git commit -m "docs(storage): name Role A vs Role B FalkorDB in CLAUDE.md and both compose files"
   ```

### WB-4 — Ignore the audit database and confirm secrets stay ignored

1. Add to `.gitignore`: `*.sqlite3` and `db/audit.sqlite3` (the existing `*.sqlite` does not match `.sqlite3`). Confirm `deploy/container/.env` is still ignored:
   ```bash
   git check-ignore -v db/audit.sqlite3 deploy/container/.env
   ```
   Expected: both matched. If `db/audit.sqlite3` is already tracked, stop — it must be removed from the index in a separate, owner-approved commit; do not commit data.
2. Commit:
   ```bash
   git add .gitignore
   git commit -m "chore(storage): ignore *.sqlite3 so the Role A audit log is never committed"
   ```

### WB-5 — Reconcile the stale `db/INSTALL.md` and `db/CONFIG.md`

Both still name `cmd/hunter` and `agent/graph.py`, which [ADR 0004-graph-memory](../../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md) says do not exist and will not be created. Per [docs/08 §8.3](../../docs/08-storage-architecture.md), reconcile them against Role A as specified in `db/CLAUDE.md`, or mark them `superseded` pointing at it.

1. Add a provenance header to each (they have none today, so the validator treats them as `proposed` and warns). Choose one:
   - **Rewrite** to Role A: replace `cmd/hunter` → the Go binaries never connect; `agent/graph.py` → `db/graph_client.py`; the MCP interface section → "no MCP/REST server is deployed" ([db/CONFIG.md](../../db/CONFIG.md) risk register). Mark `authoritative` only if the result agrees with the committed code.
   - **Supersede**: add `**Status:** superseded · **Derived from:** [CLAUDE.md](CLAUDE.md) …` and a one-line pointer, keeping the historical text. This is the lower-risk option and is preferred unless the owner wants a full rewrite.
2. Fix `db/CONFIG.md`'s ontology path: it says `config/config.yaml`; the real file is `db/config.yaml`. Correct the reference.
3. Run the gate:
   ```bash
   uv run python .github/scripts/validate_outlines.py --strict
   grep -rn 'cmd/hunter\|agent/graph.py' db/INSTALL.md db/CONFIG.md   # expect no live instruction
   ```
4. Commit:
   ```bash
   git add db/INSTALL.md db/CONFIG.md
   git commit -m "docs(db): reconcile INSTALL/CONFIG with Role A; retire cmd/hunter and agent/graph.py references"
   ```

**Failure ST-0.3:** `validate_outlines --strict` still flags a dead link or pasted threshold in `db/`. Detection: validator output. Handling: fix in this work block; cite `policy.yaml` keys by name, never paste values.

### WB-6 — Commit the orchestrator health service and container stack

`orchestrator/service.py`, its test, and `deploy/container/` are untracked. They are the target of Phase 1, so they must be rank-1 code first. Commit them **without** changing behaviour (Phase 1 does that, test-first).

1. Run the service test:
   ```bash
   uv run pytest orchestrator/tests/test_service.py -q
   ```
2. Stage explicitly, excluding the secret:
   ```bash
   git add orchestrator/service.py orchestrator/tests/test_service.py \
           deploy/container/go.Dockerfile deploy/container/python.Dockerfile \
           deploy/container/certs.Dockerfile deploy/container/compose.yaml \
           deploy/container/compose.debug.yaml deploy/container/nats.conf \
           deploy/container/init-env.sh deploy/container/generate-certs.sh \
           deploy/container/verify.sh deploy/container/up.sh \
           deploy/container/README.md deploy/container/.env.example
   ```
   Confirm `deploy/container/.env` is **not** staged: `git status --porcelain | grep '\.env$'` must show it still untracked/ignored.
3. Commit:
   ```bash
   git commit -m "feat(deploy): commit orchestrator health service and container stack (graph still required; Phase 1 inverts)"
   ```

**Failure ST-0.4:** `deploy/container/.env` accidentally staged. Detection: `git diff --cached --name-only | grep '/.env$'`. Handling: `git restore --staged deploy/container/.env` immediately; never commit it.

### WB-7 — Record the Phase 0 gate and merge to `develop`

1. Run the full verification block (README §5) on the phase branch. All green.
2. Append a gate row to the [state log](../../.agents/orchestration/obs/state-log.md): what was committed, that `develop` is current, that Role A is now rank-1, and the two-role compose naming.
3. Merge the phase branch to `develop` (fast-forward or `--no-ff` per repo convention), commit the state-log update with it.
4. Commit:
   ```bash
   git add .agents/orchestration/obs/state-log.md
   git commit -m "docs(state): record storage Phase 0 gate — develop synced, Role A committed, roles named"
   ```

## Acceptance

- [ ] `develop` contains both `main` and `origin/develop` lines; verification block green on it.
- [ ] Role A code + ADR 0004-graph-memory committed; `uv run pytest db/tests` green; `db/CLAUDE.md` verify uses `--extra graph`.
- [ ] `CLAUDE.md` and both compose files name Role A vs Role B; both `compose config` lines run.
- [ ] `*.sqlite3` ignored; `deploy/container/.env` still ignored; `int/` left untracked.
- [ ] `db/INSTALL.md` / `db/CONFIG.md` reconciled or superseded; no live `cmd/hunter` / `agent/graph.py` instruction.
- [ ] `orchestrator/service.py` + container stack committed without behaviour change.
- [ ] `uv run python .github/scripts/validate_outlines.py --strict` passes.
- [ ] State log records the gate.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| ST-0.1 | `develop`/`origin/develop` divergence conflicts | merge conflict in `enforcerd/` | Stop; ask owner; never relax a check |
| ST-0.2 | Role A tests fail without graphiti extra | ImportError | `uv sync --extra graph`; lazy-import tests must pass regardless |
| ST-0.3 | Validator flags `db/` docs | `validate_outlines --strict` | Fix links/headers; cite policy keys |
| ST-0.4 | `.env` staged | `git diff --cached` | Unstage immediately; never commit |
| ST-0.5 | `db/audit.sqlite3` already tracked | `git ls-files db/audit.sqlite3` | Owner-approved removal commit; do not commit data |

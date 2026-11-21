# Phase 2: Harden the authorities (ledger + Role A replay)

**Status:** proposed · **Derived from:** [README.md](README.md), [../../docs/09-storage-integration-spec.md](../../docs/09-storage-integration-spec.md) (§4.2, §4.5, OD-6, OD-8), [../../orchestrator/ledger.py](../../orchestrator/ledger.py), [../../db/audit.py](../../db/audit.py) · **Updated:** 2026-11-21

**Depends on:** Phase 0. **Blocks:** Phase 6 (restore drill needs a head anchor). **Source:** docs/09 §13 S2, OD-6, OD-8.

## Goal

Close the four named ledger limits ([docs/09 §4.2](../../docs/09-storage-integration-spec.md)) and make Role A's graph rebuildable from its audit log, without ever putting a derived store on the decision path. The ledger stays append-only and write-only from the graph; a ledger fault still never changes a decision.

## Current behaviour (grounded)

`orchestrator/ledger.py`:
- `Ledger.append` chains `prev_hash` → `record_hash` in `BEGIN IMMEDIATE`, WAL. `verify_chain` detects an edited/deleted **interior** record.
- **No head anchoring** — truncating the newest rows leaves a valid shorter chain (the module docstring names this limit).
- **No uniqueness on `event_id`** — a redelivered event can produce two records.
- **No schema version** in the row.
- **No retention policy** — the file grows without bound.

`db/audit.py`: `record_episode` (pending) → graph write → `mark_graph_write_status` (committed/failed); `pending_episodes()` finds unconfirmed writes. **No replay command** exists yet (OD-8).

## Work blocks

Branch: `feature/storage-p2-harden-authorities` from `develop`. Sub-branch per authority if parallelising: `…/ledger` and `…/rolea-replay`, merged back into the phase branch before the phase merges. Commit at the end of each work block.

### WB-1 — Record-envelope version key (test first)

Add the version marker **before** the first external consumer (Role B replay, Phase 4).

1. **Failing test.** In `orchestrator/tests/test_ledger.py`: append a record, assert the stored body carries `record_version` equal to a module constant `RECORD_VERSION` (start at `1`). Assert `record_hash` covers it (changing the version changes the hash).
2. Run red.
3. **Minimal change.** Add `RECORD_VERSION = 1` to `orchestrator/ledger.py`; in `append`, include `"record_version": RECORD_VERSION` in the record body before hashing. `verify_chain` recomputes `record_hash` over the body, so it picks the field up automatically — add an assertion in the test that a body missing the version fails verification only if the hash was computed without it (do not break existing chains: a pre-version ledger is a migration concern, name it in the test as a known limit, not a silent pass).
4. Run green: `uv run pytest orchestrator/tests/test_ledger.py -q`.
5. Commit:
   ```bash
   git add orchestrator/ledger.py orchestrator/tests/test_ledger.py
   git commit -m "feat(storage): add record_version envelope key to the ledger (OD-6 prerequisite)"
   ```

### WB-2 — Head anchoring (OD-6, test first)

Persist the head `(seq, record_hash)` **outside** the ledger on every append; verify it on startup and before export. This is the fix for truncation detection.

1. **Decision (record it).** Where the head lives: a separate small file beside the ledger (e.g. `<ledger>.head.json`) written atomically (write-temp + `os.replace`) **after** the ledger COMMIT. It is a derived check value, not a second authority — the ledger remains authoritative; the anchor only detects loss. Write this choice into the phase's ADR note or the state log; [docs/09 OD-6](../../docs/09-storage-integration-spec.md) requires "where the head is stored and how it is verified".
2. **Failing tests.**
   - After N appends, the anchor file holds `(N-1, head_hash)`.
   - `verify_head(ledger)` returns True when intact; after deleting the newest row directly via SQLite, `verify_chain` still passes (shorter valid chain) but `verify_head` returns False — the truncation is caught.
   - A crash simulated between COMMIT and anchor write leaves the anchor one behind; startup reconciliation re-derives the head from the ledger and rewrites the anchor (the ledger wins; the anchor is derived).
3. Run red.
4. **Minimal change.** Add `Ledger.anchor_path`, write the anchor in `append` after COMMIT, add `verify_head(ledger) -> bool` and a `reconcile_head(ledger)` used on startup. Wire `verify_head` into the `verify` CLI subcommand (`main`) so `python -m orchestrator.ledger verify PATH` checks both chain and head. Keep the anchor write failure non-fatal to the decision (it is auditability, not hunting — [docs/09 §4.2 failure](../../docs/09-storage-integration-spec.md)).
5. Run green.
6. Commit:
   ```bash
   git add orchestrator/ledger.py orchestrator/tests/test_ledger.py
   git commit -m "feat(storage): anchor the ledger head outside the file; detect truncation (OD-6)"
   ```

**Failure ST-2.1:** anchor and ledger disagree after a crash. Detection: `verify_head` False on startup. Handling: `reconcile_head` re-derives from the ledger (the authority); never truncate the ledger to match a stale anchor.

### WB-3 — Ingest dedupe on `event_id` (test first)

Suppress redelivery **at ingest**, not by mutating the append-only ledger ([docs/09 §4.2](../../docs/09-storage-integration-spec.md): `orchestrator/dedupe.py` (planned)).

1. **Failing test.** Create `orchestrator/tests/test_dedupe.py`: feeding the same `event_id` twice within the dedupe window yields one ledger record; a different `event_id` yields two; the same `event_id` after the window is a new record (at-least-once is the default, [docs/09 §7](../../docs/09-storage-integration-spec.md)).
2. Run red (module absent).
3. **Minimal change.** Create `orchestrator/dedupe.py` (planned → real): a bounded, Python-owned store of recently seen `event_id` + content hash. The window bound is a **new policy key** `dedupe_window_seconds` in `orchestrator/policy.yaml` (cite by name, never paste). Dedupe runs **before** the graph invocation; it is not a ledger mutation. Keep it side-channel: a dedupe-store fault degrades to "no suppression" (duplicate recorded) rather than blocking a decision, and is logged.
4. Run green; add the policy key in the same commit.
5. Commit:
   ```bash
   git add orchestrator/dedupe.py orchestrator/tests/test_dedupe.py orchestrator/policy.yaml
   git commit -m "feat(storage): ingest dedupe on event_id within a policy window (no ledger mutation)"
   ```

**Failure ST-2.2:** dedupe store grows unbounded. Detection: review of the bound. Handling: evict oldest entries past the window; the bound is `policy.yaml: dedupe_window_seconds` plus a max-entries key, never a pasted number.

### WB-4 — Ledger retention / archive (test first)

Archive verified segments; never delete an unverified row ([docs/09 §4.2](../../docs/09-storage-integration-spec.md)).

1. **Failing test.** `orchestrator/tests/test_ledger_archive.py`: archive a verified prefix of the chain to a segment file; the live ledger keeps the head anchor consistent; attempting to archive a chain that fails `verify_chain` is refused.
2. Run red.
3. **Minimal change.** Add an `archive` CLI subcommand and `archive_verified_prefix(ledger, upto_seq)` that refuses unless `verify_chain` and `verify_head` pass. Retention window is a policy key `ledger_archive_after_records` (planned key, added here). Archiving is an operator tool, never an automatic runtime deletion.
4. Run green.
5. Commit:
   ```bash
   git add orchestrator/ledger.py orchestrator/tests/test_ledger_archive.py orchestrator/policy.yaml
   git commit -m "feat(storage): archive verified ledger segments; refuse unverified rows"
   ```

### WB-5 — Role A replay command + reconciliation report (OD-8, test first)

Make the dev graph recoverable: a replay command and a reconciliation report ([docs/09 OD-8](../../docs/09-storage-integration-spec.md)).

1. **Failing test.** `db/tests/test_replay.py`: given an audit log with rows in `pending`/`failed`/`committed`, a `replay_pending` function re-submits exactly the non-committed rows through `add_work_block_episode` (use a fake graphiti client — no live FalkorDB, matching the lazy-import test style), marks them committed on success, and leaves a failed row `failed` for retry. A reconciliation report counts audit rows vs graph episodes and flags drift.
2. Run red.
3. **Minimal change.** Create `db/replay.py` (planned → real): `replay_pending(graphiti, config, db_path)` iterating `pending_episodes()`, rebuilding the episode from the stored `payload_json` (the exact string, [db/DATA_MODEL.md §4](../../db/DATA_MODEL.md)), calling `add_work_block_episode`, then `mark_graph_write_status`. Add `reconciliation_report(db_path)` returning counts by status. The graph is derived; the audit log is authoritative — replay never edits an audit payload.
4. Run green: `uv run pytest db/tests -q`.
5. Commit:
   ```bash
   git add db/replay.py db/tests/test_replay.py
   git commit -m "feat(db): Role A replay from audit + reconciliation report (OD-8)"
   ```

**Failure ST-2.3:** replay re-submits an already-committed episode. Detection: test asserts only non-committed rows replay. Handling: filter on `graph_write_status != 'committed'`; idempotency on the episode name guards a crash mid-replay.

### WB-6 — Gate and merge

1. Full verification block (README §5) green; `uv run pytest orchestrator/tests db/tests -q`.
2. `/code-review` + `/security-review`. Confirm: no derived store gates a decision; the anchor is derived, not a second authority; dedupe never mutates the ledger; every new bound is a policy key.
3. Append the gate row to the [state log](../../.agents/orchestration/obs/state-log.md); merge to `develop`.

## Acceptance

- [ ] `verify_chain` **plus** head match on startup and before export; truncation of the newest rows is detected.
- [ ] Redelivered `event_id` within the window produces one record; dedupe is at ingest, not a ledger mutation.
- [ ] Record body carries `record_version`; archive refuses an unverified chain.
- [ ] Role A replay rebuilds the graph from the audit log; replayed graph matches the audit count; reconciliation report exists.
- [ ] Every new bound is a `policy.yaml` key cited by name.
- [ ] Verification block green; state log records the gate.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| ST-2.1 | Anchor/ledger disagree after crash | `verify_head` False | Reconcile from the ledger (authority); never truncate to match |
| ST-2.2 | Dedupe store unbounded | review | Evict past window; bound is a policy key |
| ST-2.3 | Replay re-submits committed rows | test | Filter non-committed; idempotent episode name |
| ST-2.4 | Archive deletes an unverified row | test | Refuse unless chain + head verify |

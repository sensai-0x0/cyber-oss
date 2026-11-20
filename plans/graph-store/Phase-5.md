# Phase 5: Backup, observability and failure drills

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-4.md](Phase-4.md), [../../db/CONFIG.md](../../db/CONFIG.md) · **Updated:** 2026-11-20

**Depends on:** Phase 4.

## Goal

The graph can be backed up, restored and monitored, and each failure in this plan has been staged at least once.

## Scope

1. **Backup.** A script (`deploy/falkordb/backup.sh`, planned) triggers a snapshot and copies it out of the volume. Because the graph is rebuildable from the ledger, backup is a convenience, and the plan states that replay is the primary recovery.
2. **Metrics.** Counters for episodes recorded, dropped (by reason: queue, budget, refusal, truncation, auth), retried and failed, plus tokens in and out and cache-read tokens per interval, from `response.usage`. Latency histograms for write and search. Queue depth. Exposed on the app's existing logging path, with no new public port.
3. **Resource limits.** CPU and memory limits on both containers. Set the values here, not in earlier phases.
4. **Failure drills** run in the lab and written up in `.agents/eval-results/` (planned file per run):

| Drill | Expected result |
|---|---|
| Kill FalkorDB during ingest | Hunt unaffected, queue fills then drops, counters show it, recovery resumes |
| Stop FalkorDB for longer than the queue holds | Replay from the last good `seq` repairs the gap |
| Wrong password | `graphcheck` fails, app runs with the graph disabled |
| Break the network between the two containers | Timeouts, no stalls |
| Fill the volume | Writes fail and are counted, the app continues |
| Restore a snapshot into a fresh volume | Counts match, then replay catches up |
| Claude API returns 429, 529 or auth errors | SDK retries are bounded, episodes are dropped and counted, an auth error disables the store once |
| Invalid or revoked API key mid-run | One alert, store disabled for the process, hunt unaffected |
| `down -v` | Graph gone, replay restores it |

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-5.1 | Snapshot taken mid-write is inconsistent | Restore drill | Snapshot through the database's own save command, not a file copy of a live volume |
| GF-5.2 | Backups contain lab data and get committed | `.gitignore` and a pre-commit check | Add the backup path to `.gitignore` in this phase, and write backups outside the repository |
| GF-5.3 | Alert fatigue from expected drops during outages | Review after the first drill | Alert on sustained drop rate, not single events |
| GF-5.4 | Disk fills silently | Volume usage metric | Retention rule from the ADR, plus a limit |
| GF-5.5 | Restore succeeds but the indices are missing | Search returns nothing | Run `build_indices_and_constraints()` after restore |
| GF-5.6 | Latency creeps up as the graph grows | Trend on the histogram | Re-check against the budget in `db/CONFIG.md` and prune by retention |

## Acceptance

- [ ] Every drill above has a recorded result. Any unexpected result is in `errors.md`.
- [ ] A restore into a fresh volume works.
- [ ] 🔒 HUMAN CHECKPOINT: a person reads the drill report.

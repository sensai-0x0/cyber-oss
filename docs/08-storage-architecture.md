**Status:** proposed · **Derived from:** [`db/CLAUDE.md`](../db/CLAUDE.md), [`db/DATA_MODEL.md`](../db/DATA_MODEL.md), [`db/CONFIG.md`](../db/CONFIG.md), [`spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md`](../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md), [`spec/DECISIONS/0004-campaign-belief-ledger.md`](../spec/DECISIONS/0004-campaign-belief-ledger.md), [`spec/DECISIONS/0007-fleet-command-path.md`](../spec/DECISIONS/0007-fleet-command-path.md), [`plans/graph-store/README.md`](../plans/graph-store/README.md), [`plans/ml-add/README.md`](../plans/ml-add/README.md), [`plans/fleet/hunter-fleet-design.md`](../plans/fleet/hunter-fleet-design.md), [`.agents/orchestration/invariants.md`](../.agents/orchestration/invariants.md) · **Updated:** 2026-11-21

This is a narrative synthesis, not a new decision. Every claim below already exists, accepted or proposed, somewhere in the citations above; this document's only job is to put all of them on one page so the storage picture can be read as a whole, and to name the one place two accepted lines contradict each other. Where it says `proposed`, nothing may be built from this file alone — read [outline-method.md §3](../.agents/outline-method.md) and the authorizing ADR first. **§8 turns this into an action list for making it a real plan.**

## 1. Why one document

Six storage mechanisms exist or are proposed across this repo, owned by five different components, decided across four ADRs and two still-open plans. No single file said what each one is *for* until now, and one pair of them (§4) share a product name and a container image without sharing a purpose — which is exactly the kind of drift [`outline-method.md`](../.agents/outline-method.md) exists to catch. [`docs/03-system-architecture.md`](03-system-architecture.md) covers the four-service security pipeline; this file covers everything that persists state for it, beside it, or about it.

## 2. The storage inventory

| Store | Technology | Owner (only writer) | Status | Role |
|---|---|---|---|---|
| Cursor + novelty gate | BadgerDB | `hunterd/store` | accepted | Resumable scan position; dedupe novel/unchanged/mutated |
| Evidence ledger | SQLite (WAL) | `orchestrator/ledger.py` | accepted | Append-only, hash-chained record of every triage decision |
| Campaign posteriors | SQLite | `orchestrator/campaign.py` | accepted | Shadow scorer state; a challenger, never a decision input |
| Dev-memory graph | FalkorDB + Graphiti | `db/graph_client.py` | accepted | Agent-orchestration bookkeeping across sessions (§4, Role A) |
| Graph-write audit log | SQLite | `db/audit.py` | accepted | Verbatim episode payloads, replay/audit for Role A |
| Hunting-analysis graph | FalkorDB + Graphiti | `orchestrator/graph_replay.py` (planned) | **proposed**, blocked on ADR 0006 | Post-decision side channel over ledger data (§4, Role B) |
| Durable LangGraph checkpointer | SQLite or Postgres (undecided) | `orchestrator/graph.py` | **open** (HC-4) | Survive a process restart with a pending `interrupt()` |
| Fleet lease/grant table | NATS KV | `fleetd/lease`, `fleetd/grant` (planned) | **proposed**, ADR 0007 accepted, not built | Shard ownership, rate-grant accounting across many hunters |
| Fleet event bus | NATS JetStream streams (`DELTA`, `OBS.*`, `PROBE_REQ`, `AUDIT`) | `hunterd` publishes, `orchestrator` consumes | **proposed** (fleet Phase 4, 6, 7) | Durable transport replacing stdio NDJSON at fleet scale |
| Badger outbox | BadgerDB (same directory as the cursor/gate) | `hunterd/store/outbox.go` (planned) | **proposed** (fleet Phase 7) | Buffer observations when the stream refuses or the bus partitions |
| Baseline snapshot | Object storage (provider undecided, `[U]`) | durable `hunterd` node (planned) | **proposed** (fleet design §16) | Recover a durable node whose Fly Volume is lost or corrupt |

Read-only configuration that shapes all of the above but is not itself mutated at runtime — `mission.yaml`, `orchestrator/policy.yaml`, signed scope bundles (planned) — is covered in [`docs/03-system-architecture.md`](03-system-architecture.md) and [`spec/STATE.md`](../spec/STATE.md), not repeated here.

## 3. BadgerDB — the one Go-owned store

`hunterd/store` is the only component permitted to open a Badger directory, anywhere, ever (I17). Two key families exist today:

- `checkpoint/<authorized-scope>` — the next Feistel sequence number, no TTL, restorable after a crash (I16).
- `observation/<probe-key>` — hash, payload, first-seen and last-seen, with a configurable TTL; read-compare-write happens in one transaction so there is no window for a stale novelty decision.

Two changes are **proposed**, not built: storing `{hash, compact attributes ≤ 256 B}` instead of the full payload (today's default peaks near 720 MB RSS under write load — a measured finding, not a target), and a durable outbox (`hunterd/store/outbox.go`) that holds observations when the event bus is down, so a fleet-mode node does not lose data to a NATS partition. In fleet mode the shard cursor's authority moves to the NATS KV lease table (§6); Badger's `checkpoint/<scope>` key stays authoritative only for single-node, non-fleet runs. No Go process other than `hunterd` ever opens this directory, and Python never reads it directly — a Python caller that needs cursor history goes through `query_kv_history`, per [`.agents/orchestration/invariants.md`](../.agents/orchestration/invariants.md).

## 4. FalkorDB + Graphiti — two roles, one name, do not conflate them

This is the one place two accepted decisions currently point at the same product and the same Docker image for different reasons, and the repo's own merge history has a live instance of the resulting confusion (§4.3). Treat the two roles as two separate deployments from here on, never as one graph with two audiences.

### 4.1 Role A — dev-process memory (accepted, ADR 0004-graph-memory)

**Purpose:** lets an agent orchestrating work *on this repository* remember what a prior session did. Entities are `WorkBlock`, `Worktree`, `FailurePattern`; edges are `DEPENDS_ON` and `ENCOUNTERED_FAILURE` (ontology fixed in `db/config.yaml`). It has no relationship to `DeltaEvent`, `MitigationAction`, or any invariant in [`.agents/orchestration/invariants.md`](../.agents/orchestration/invariants.md) — [ADR 0004](../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md) says this explicitly, and it is not wired into `orchestrator/graph.py`'s triage nodes.

- **Compose:** `db/compose.yaml` — publishes `127.0.0.1:${FALKORDB_PORT:-6379}`, image `falkordb/falkordb-server` (the `-server` variant deliberately excludes the bundled browser UI).
- **Access:** Python only, only through `db/graph_client.py` (`build_graphiti`, `add_work_block_episode`, `search_graph`). No Go process connects to FalkorDB, ever (`db/INSTALL.md` §1.3).
- **Group ID:** `sentinel_lite_main` (`db/config.yaml`), since FalkorDB keys each `group_id` as a wholly separate graph — a mismatched ID reads back nothing, silently.
- **Audit:** every episode is recorded in `db/audit.sqlite3` (§5) *before* the graph write, via `record_episode`/`mark_graph_write_status`; a failed graph write leaves the audit row `pending`, found later by `pending_episodes()`. The graph and the audit log hold different projections of the same event on purpose — see [`db/DATA_MODEL.md` §5](../db/DATA_MODEL.md) for the full rationale table.

### 4.2 Role B — hunting-analysis side channel (proposed, blocked on ADR 0006)

**Purpose:** a read side channel for human analysis *after* a mitigation decision has already been made — never between an event and a mitigation. [`plans/graph-store/README.md`](../plans/graph-store/README.md) names the candidate ontology (`Target`, `Indicator`, `Campaign`, `Decision`; edges `OBSERVED_WITH`, `CLUSTERED_WITH`, `DECIDED`) and the ingestion path: one episode per **evidence-ledger record** (§5.1), keyed by the ledger's `record_hash` so replay is idempotent, with field allowlisting and length caps so no free text (banners, titles, error text) ever reaches the extraction LLM.

- **Compose:** `deploy/compose.yaml`, service `falkordb`, gated behind the `graph` Compose profile. It joins `graphnet`, a Compose network marked `internal: true`, and publishes **no host port** — the opposite of Role A's deliberately-loopback-published port, because this instance has no reason for a human to open a browser against it directly.
- **Access:** planned to live **inside the orchestrator's own container**, as a library, not a server — the reference Graphiti MCP/REST server is never deployed for either role, because it ships with no authentication (`db/CONFIG.md` risk register, Critical severity).
- **LLM dependency:** Claude via the Messages API (not the Agent SDK) per [`plans/graph-store/Claude-api.md`](../plans/graph-store/Claude-api.md) — a paid, networked, non-deterministic step, and a second prompt-injection surface beyond the one `hunterd`'s own novelty gate guards. This is the one place in the whole storage picture where egress leaves the lab network on purpose; Phase 0 of the plan requires the ADR to name the exact allowed host.
- **Blocking precondition:** [`plans/graph-store/Phase-0.md`](../plans/graph-store/Phase-0.md) requires, before any of this is built: a named consumer (not "would be nice to have"), a written comparison against the evidence ledger alone (graph traversal over campaign relationships is the only justification for a second store), pinned `graphiti-core`/FalkorDB versions with a licence and regression check, and a human checkpoint on the LLM egress and data classes. **None of that has happened.** `spec/DECISIONS/0006-graph-store.md` does not exist; the number is only reserved ([ADR 0007](../spec/DECISIONS/0007-fleet-command-path.md), numbering note).

### 4.3 The conflict, named plainly

`CLAUDE.md`'s own verification block is, at the time of writing, mid-merge-conflict between a version that runs both `docker compose -f deploy/compose.yaml config` and `docker compose -f db/compose.yaml config`, and a version that runs only the `deploy/compose.yaml` line. That is not an accident of the merge tool — it is two people's mental models of "the FalkorDB compose file" disagreeing about whether there are one or two of them. There are two, they serve different roles, and both verify commands belong in CI once Role B exists. Until Role B is built, only the `db/compose.yaml` line is meaningful; the `deploy/compose.yaml` `graph` profile line checks a service with no code pointed at it yet. **§8.1 is the concrete fix.**

## 5. SQLite — three Python-owned files, one shared rule

Every SQLite file below is Python-owned, bounded, and rebuildable from a more authoritative source; none is read or written from Go.

### 5.1 Evidence ledger (`orchestrator/ledger.py`) — accepted

Append-only, hash-chained (`prev_hash` → `record_hash`, SHA-256 over canonical JSON), one row per triage decision. The ledger node (`make_record_node`) runs after the policy decision and never changes it — a ledger write failure is recorded to `failed_event_ids` and logged, never raised into the graph. `verify_chain` detects an edited or deleted interior record; detecting *truncation* of the newest records needs the head hash kept somewhere else, which is explicitly out of scope for the current phase (a named limit, not a bug). This file is the **source of truth** for Role B ingestion (§4.2) and for the campaign scorer (§5.2) — nothing else is authoritative over a past decision.

### 5.2 Campaign posteriors (`orchestrator/campaign.py`) — accepted

A per-campaign log-odds posterior with half-life decay, identified by a stable hash of **observed** indicators (never by target, so a campaign that moves infrastructure keeps its identity). This is a **challenger**: its output goes only into `ledger.challengers[scorer_id]`, never into `GraphState.policy` — enforced by a test on every phase that adds a scorer ([ADR 0004-campaign-belief-ledger](../spec/DECISIONS/0004-campaign-belief-ledger.md), decision 6). It is explicitly *not* `hunterd`'s Badger — Badger stays one-process-per-directory and owned by Go (I17); the campaign posterior needed cross-event persistence that Badger's ownership rule does not fit, so Phase 0 of `plans/ml-add` chose a separate Python-owned file instead of widening Badger's access.

### 5.3 Graph-write audit log (`db/audit.sqlite3`, `db/audit.py`) — accepted

Columns: `id`, `episode_type` (checked), `block_id`, `group_id`, `ingested_at`, `payload_json` (the exact string sent to `add_episode`), `graph_write_status` (`pending`/`committed`/`failed`). This is Role A's safety net against Graphiti's lossy, LLM-driven extraction — see [`db/DATA_MODEL.md` §5](../db/DATA_MODEL.md) for why the graph and the audit row are not redundant with each other.

### 5.4 Durable LangGraph checkpointer — open question, not yet a file

`build_graph(checkpointer=None)` defaults to the in-memory checkpointer; production needs a durable one so a pending `interrupt()` (the human-approval gate for a broad mitigation) survives a restart. [`docs/03-system-architecture.md`](03-system-architecture.md) already flags this as integration work. The fleet plan's **HC-4** human checkpoint ([`plans/fleet/README.md`](../plans/fleet/README.md), Phase 6) names the open choice explicitly: SQLite or Postgres. This document takes no position — §8.4 names it as a decision still owed to an ADR.

## 6. NATS — bus today, KV and streams proposed

`deploy/compose.yaml` runs NATS today for the lab fixtures (TLS, password auth) and is **accepted** as the transport for that lab. Three further roles are **proposed**, all behind [ADR 0007](../spec/DECISIONS/0007-fleet-command-path.md) and gated on `plans/fleet/README.md` phases that have not started:

- **Command bus** (Phase 3–4): signed `Command v1` envelopes on `fleet.ctl.<worker>` and `hunt.probe.<shard>`, published only by `fleetd`. The envelope, not the transport, carries authenticity — a future transport change touches an adapter (`ctlbus`), never the payload or verification code.
- **Lease/grant KV** (Phase 4): a lease table with compare-and-swap on KV revision, giving each shard a fixed *home* durable node so Badger's one-process-per-directory rule (I17) holds even with many hunters. In fleet mode, shard cursor authority moves here; `checkpoint/<scope>` in Badger stays authoritative only in single-node mode (§3).
- **Event streams** (Phase 6–7): `DELTA`, `OBS.*`, `PROBE_REQ`, `AUDIT`, each with retention limits and `discard=new`, replacing stdio NDJSON as the transport from `hunterd` to `orchestrator` once more than one hunter exists. The orchestrator acks only after the ledger record or checkpoint commits, so a crash between delivery and ack does not lose an event (Phase 6 eval E28).

None of this is built. It is listed here because it changes who may be a writer to Badger's logical cursor state (§3) and because a reader of this document should not mistake "NATS is a bus" for "NATS has no persistent state" once the KV and streams land.

## 7. Function reference

The actual read/write surface of each store, by module. Private helpers (leading `_`) are listed only where they *are* the persistence operation (campaign scoring's SQLite load/store/evict/merge); ordinary private math helpers (`logit`, `sigmoid`, `_decay`, `_seconds`, `_target_key`) are omitted as not storage-relevant. "Planned" rows name a path that does not exist yet, per [outline-method.md §4](../.agents/outline-method.md#4-path-reality).

### BadgerDB (`hunterd/store`, Go) — accepted

| Function | Signature | Purpose |
|---|---|---|
| `Open` | `Open(path string) (*Store, error)` | Opens the one Badger directory this process may hold (I17) |
| `(*Store) SaveCursor` | `SaveCursor(scope string, cursor uint64) error` | Persists the next Feistel sequence number for a scan scope, no TTL |
| `(*Store) LoadCursor` | `LoadCursor(scope string) (uint64, error)` | Restores a scan scope's cursor after a restart (I16); a missing key and a cursor of 0 are indistinguishable today (`SHR-019`, open) |
| `NewDeltaGate` | `NewDeltaGate(store *Store, ttl time.Duration) *DeltaGate` | Builds the novelty gate with a configured TTL |
| `(*DeltaGate) Apply` | `Apply(key string, hash [32]byte, payload []byte, observedAt time.Time) (Decision, error)` | One Badger transaction: read prior hash, decide `Novel`/`Unchanged`/`Mutated`, write new state |
| `hunterd/store/outbox.go` *(planned, fleet Phase 7)* | — | Durable outbox for observations the event bus refuses; drained in order on reconnect |

### Evidence ledger (`orchestrator/ledger.py`) — accepted

| Function | Signature | Purpose |
|---|---|---|
| `record_hash` | `record_hash(record: dict) -> str` | SHA-256 over the canonical record body, excluding its own hash field |
| `Ledger.append` | `append(self, fields: dict) -> dict` | Chains one record onto the head inside `BEGIN IMMEDIATE`; the caller supplies no hashes |
| `Ledger.records` | `records(self) -> list[tuple[int, str, str]]` | All stored `(seq, record_hash, body)` rows, in order — verification and tooling only |
| `verify_chain` | `verify_chain(ledger: Ledger) -> int \| None` | First broken sequence number, or `None` if the chain is intact |
| `build_record_fields` | `build_record_fields(state: GraphState) -> dict` | Assembles the event, evidence classes, intel, policy and policy hash for one record |
| `run_challengers` | `run_challengers(state, challengers) -> dict` | Scores the event with every registered challenger (e.g. the campaign scorer); an exception is recorded, never raised |
| `make_record_node` | `make_record_node(writer, challengers=()) -> Callable[[GraphState], dict]` | Builds the LangGraph node that appends one record per decision and never alters it |

### Campaign posteriors (`orchestrator/campaign.py`) — accepted

| Function | Signature | Purpose |
|---|---|---|
| `CampaignScorer.observe` | `observe(self, raw_event: dict) -> dict` | Public entry point: updates or creates a campaign posterior and returns the challenger's score (never written to `GraphState.policy`) |
| `CampaignScorer.score` | `score(self, event: dict) -> float` | Read-only log-odds lookup for an event's indicators |
| `CampaignScorer._load` | `_load(self, campaign_id: str) -> tuple[float, float, float, list[str]]` | SQLite read of one campaign's stored posterior state |
| `CampaignScorer._store` | `_store(self, campaign_id, derived, repeat, t_ref, targets, seen_at) -> None` | SQLite write of a campaign's updated posterior state |
| `CampaignScorer._merge` | `_merge(self, keep: str, drop: str, half_life_s: float) -> None` | Merges two campaign identities found to share indicators |
| `CampaignScorer._evict` | `_evict(self, config: dict) -> None` | Enforces `MAX_TARGETS_KEPT`/`SEEN_PER_CAMPAIGN` bounds so the file stays bounded |
| `CampaignScorer.campaign_count` | `campaign_count(self) -> int` | Row count, for tests and observability |

### Dev-memory graph (`db/graph_client.py`) — accepted, Role A only (§4.1)

| Function | Signature | Purpose |
|---|---|---|
| `load_graph_config` | `load_graph_config(env: dict \| None = None) -> GraphConfig` | Reads `FALKORDB_HOST`/`GRAPHITI_GROUP_ID` from the environment; refuses any host that is not loopback |
| `build_graphiti` | `build_graphiti(config: GraphConfig)` | Constructs the Graphiti client bound to one `group_id` |
| `init_graph` | `async init_graph(graphiti) -> None` | One-time index/constraint setup on a fresh FalkorDB graph |
| `build_episode_body` | `build_episode_body(kind: EpisodeKind, payload: dict) -> str` | The only place an episode JSON string is assembled; stamps `episode_type`, truncates every field |
| `add_work_block_episode` | `async add_work_block_episode(...)` | The only write path into the graph — callers never touch `graphiti_core` directly |
| `search_graph` | `async search_graph(graphiti, config, query: str, num_results: int = 10)` | Read path, scoped to the configured `group_id` |

### Graph-write audit log (`db/audit.py`) — accepted, Role A only

| Function | Signature | Purpose |
|---|---|---|
| `record_episode` | `record_episode(...)` | Writes the audit row *before* the graph write is attempted, status `pending` |
| `mark_graph_write_status` | `mark_graph_write_status(...)` | Flips a row to `committed` or `failed` after the graph write resolves |
| `pending_episodes` | `pending_episodes(db_path: Path = DEFAULT_DB_PATH) -> list[Row]` | Finds writes that never confirmed, for replay |

### Hunting-analysis graph (§4.2) and fleet KV/streams (§6) — planned, no code yet

| Function | Module | Purpose |
|---|---|---|
| `orchestrator/graph_replay.py` (planned) | — | Reads the ledger from a given `seq`, re-submits each record as an idempotent episode keyed by `record_hash` |
| `orchestrator/graph_ontology.py` (planned) | — | `Target`/`Indicator`/`Campaign`/`Decision` entity and edge type map for Role B, mirroring `db/config.yaml`'s pattern |
| `fleetd/lease/` (planned) | — | Lease table with CAS on NATS KV revision; reclaim after `issue + ttl + margin` |
| `fleetd/grant/` (planned) | — | Rate grants summing within the signed scope bundle's budget |
| `orchestrator/ingest.py` (planned) | — | JetStream pull consumer on `DELTA`; acks only after the ledger record commits |

## 8. Turning this into a plan

This document is `proposed`-rank 5 (docs/) narrative per [`outline-method.md`'s authority ladder](../.agents/outline-method.md#1-authority-ladder); it cannot authorize anything by itself. The concrete steps to promote it:

1. **Resolve the live `CLAUDE.md` conflict (§4.3) first**, independent of everything else below — keep both verify lines, scope the `db/compose.yaml` one as Role A (already real) and the `deploy/compose.yaml graph` profile one as Role B (not yet consumed by any code), and say so in a one-line comment at each, so the next person does not read "two compose files" as a merge artifact to delete.
2. **Write `spec/DECISIONS/0006-graph-store.md`**, completing [`plans/graph-store/Phase-0.md`](../plans/graph-store/Phase-0.md)'s acceptance list: a named consumer for Role B, the comparison against the ledger alone, pinned versions and licences, the Claude model and embedder, egress scope (Claude API, from the orchestrator container only), and the human checkpoint on data classes and retention. Phase 0 is explicit that "no consumer" is a valid outcome and ends the plan — do not treat building Role B as a foregone conclusion just because this document describes it.
3. **Reconcile `db/INSTALL.md` and `db/CONFIG.md`** against Role A as actually specified in `db/CLAUDE.md`, or mark them `superseded` pointing at it — both still name `cmd/hunter` and `agent/graph.py`, which [ADR 0004-graph-memory](../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md) already says do not exist and will not be created.
4. **Decide the durable LangGraph checkpointer backend (§5.4, HC-4)** in an ADR before fleet Phase 6 starts; this document deliberately does not pick SQLite or Postgres for you.
5. **Only after 2–4**, continue `plans/graph-store/` Phases 1–6 and `plans/fleet/README.md` Phases 4, 6 and 7 (NATS KV, streams, Badger outbox) as already phased — this document changes none of their content, it only makes the dependency between "two FalkorDB roles" and "one compose-file conflict" visible in one place so Phase 0 of each plan does not have to re-derive it.

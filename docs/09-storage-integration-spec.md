**Status:** proposed · **Derived from:** [`docs/08-storage-architecture.md`](08-storage-architecture.md), [`plans/DATA_MODEL.md`](../plans/DATA_MODEL.md), [`plans/graph-store/README.md`](../plans/graph-store/README.md), [`plans/ml-add/README.md`](../plans/ml-add/README.md), [`plans/fleet/README.md`](../plans/fleet/README.md), [`spec/STATE.md`](../spec/STATE.md), ADR 0004-graph-memory *(not on `main`)*, [ADR 0004 campaign ledger](../spec/DECISIONS/0004-campaign-belief-ledger.md), [ADR 0007](../spec/DECISIONS/0007-fleet-command-path.md), [`db/CONFIG.md`](../db/CONFIG.md), [`db/INSTALL.md`](../db/INSTALL.md), [`.agents/orchestration/invariants.md`](../.agents/orchestration/invariants.md) · **Updated:** 2026-11-21

This document is prescriptive on purpose: it states what each store **must** do and how the components **must** interact, so that the statements can be lifted into `plans/storage/` *(planned)* and one ADR per open decision without being re-derived. It grants no authority. Per [outline-method.md §1](../.agents/outline-method.md), a `docs/` document is rank 5 narrative; a "must" here is a proposed requirement, not an instruction an agent may execute. [`docs/08-storage-architecture.md`](08-storage-architecture.md) explains *what exists and why*; this file specifies *what the target design is and in what order it is built*.

**Tree note.** The Role A implementation files under `db/` — its client, audit module, ontology config and compose file — were present in the working tree this design was derived from, but are **not tracked on `main` at this revision**. They are marked *(not on `main`)* below: they describe the design to restore, not code that currently exists. The same applies to `db/CONTAINER_CONFIG.md` *(not on `main`)* and to the graph-memory ADR. Nothing in this document should be read as a claim that those files are present.

The graph database is **FalkorDB**. (In conversation it is sometimes written "FalkenDB"; there is no separate product by that name in this repository.) **Graphiti** is not a database: it is the Python library that turns an ingested episode into typed entities and edges in FalkorDB, maintains temporal validity, and serves hybrid search over them. Every sentence below that says "the graph" means the FalkorDB server; every sentence that says "Graphiti" means the extraction and query library in front of it.

---

## 0. How to read this document

| Term | Meaning here |
|---|---|
| **Store** | A process or file that retains state across a restart |
| **Plane** | A group of stores serving one purpose (operational, decision, analysis, …) |
| **Writer** | The one component permitted to mutate a store |
| **Authority** | The store whose contents win when two stores disagree |
| **Derived** | A store that can be rebuilt from an authority by replay, without human interpretation |
| **Critical path** | A store whose unavailability blocks hunting, triage, approval or enforcement |
| **Side channel** | A store whose unavailability changes only what can be *seen*, never what is *done* |

Two rules govern the whole design:

1. **One authority per fact.** Every fact has exactly one store that is permitted to be right. Every other store holding that fact is derived and declares its rebuild source.
2. **Derived stores are never on the critical path.** A graph, a dashboard, a cold backup or an analysis index may fail without stopping a probe, a decision or a mitigation.

If a proposed feature violates either rule, the feature is redesigned, not the rule.

---

## 1. The storage contract

These ten rules apply to every store in §2. A new store that cannot satisfy all ten is not added.

| ID | Rule | Why |
|---|---|---|
| **S1** | Exactly one writer per store. Readers are named in §4 and may not write. | Prevents two components from inventing different schemas for the same file. Badger already relies on this (I17). |
| **S2** | Every fact has one authority; every copy declares its rebuild source. | Makes "which one is right?" answerable without a human. |
| **S3** | A derived store never gates a decision, a dial, an approval or an enforcement. | A graph or LLM outage must not become a hunting outage. |
| **S4** | Capture the raw input before deriving from it. | Graphiti extraction is lossy and non-deterministic; the audit/ledger row is what makes a bad extraction replayable. |
| **S5** | Every derived write is idempotent on an immutable source key. | Replay and redelivery are expected, not exceptional. |
| **S6** | Authority follows deployment mode. Fleet mode moves shard cursor authority to the NATS KV lease, never to two places at once. | Two authorities are worse than one stale authority. |
| **S7** | No distributed transaction. Order the steps, make each replayable, and state the acceptable duplicate or loss window. | SQLite, Badger and FalkorDB cannot commit together. |
| **S8** | One namespace per role: separate file, volume, database, `group_id` and bucket prefix. | A shared namespace turns a configuration typo into silent cross-contamination. |
| **S9** | Badger is one process per directory, on local disk, never a network mount. | I17 in [invariants.md](../.agents/orchestration/invariants.md); network filesystems break Badger's locking. |
| **S10** | No store is reachable beyond loopback or an `internal: true` Compose network; no reference MCP/REST server is deployed; credentials come from the environment only. | Graphiti's reference server has no authentication; the graph server is not an internet-facing service. |

---

## 2. Storage inventory and role

Every store that exists or is proposed, with its single role. "Status" uses the same vocabulary as the provenance header: **accepted** is built and in use; **partial** is code-complete but not wired into the runtime; **proposed** is designed but not built; **open** is an unmade decision.

| # | Store | Technology | Plane | Role (one sentence) | Writer | Authority for | Status |
|---|---|---|---|---|---|---|---|
| 1 | Cursor + novelty gate | BadgerDB | Operational | Remembers where a scan stopped and whether a probe result is novel, unchanged or mutated | `hunterd/store` | Current scan position and per-key novelty, single-node mode | accepted |
| 2 | Observation outbox | BadgerDB (separate key family) | Operational | Holds observations the event stream refused, so a bus partition loses nothing | `hunterd/store/outbox.go` *(planned)* | Undelivered observations on that node | proposed |
| 3 | Evidence ledger | SQLite (WAL) | Decision | Append-only, hash-chained record of every triage decision and every challenger score | `orchestrator/ledger.py` | What was decided, with what evidence, and in what order | accepted |
| 4 | Campaign posteriors | SQLite | Challenger | Per-campaign log-odds state for the shadow scorer | `orchestrator/campaign.py` | Campaign identity and posterior, shadow only | accepted |
| 5 | LangGraph checkpoints | SQLite or Postgres | Execution | Lets a pending `interrupt()` survive a process restart | `orchestrator/graph.py` | In-flight graph state, not the decision | open (HC-4); SQLite helper exists |
| 6 | Dev-memory graph | FalkorDB + Graphiti | Dev memory | Lets a development session remember `WorkBlock` dependencies and failures | `db/graph_client.py` *(not on `main`)* | Derived projection of the dev-memory audit log | accepted design; code absent from `main` |
| 7 | Dev-memory audit log | SQLite | Dev memory | Verbatim episode payloads for replay and audit of Role A | `db/audit.py` *(not on `main`)* | Every Role A episode as sent | accepted design; code absent from `main` |
| 8 | Hunting-analysis graph | FalkorDB + Graphiti | Analysis | Read side channel over past decisions for human analysis | `orchestrator/graph_replay.py` *(planned)* | Derived projection of the evidence ledger | proposed, blocked on ADR 0006 |
| 9 | Lab event bus | NATS (TLS, password) | Transport | Carries events between lab fixtures and the orchestrator | `hunterd` publishes, `orchestrator` consumes | Nothing durable | accepted (lab only) |
| 10 | Fleet streams | NATS JetStream (`DELTA`, `OBS.*`, `PROBE_REQ`, `AUDIT`) | Transport | Durable, at-least-once transport at fleet scale | `hunterd` publishes, `orchestrator` consumes | Delivery, not the decision | proposed (fleet phases 6–7) |
| 11 | Fleet lease/grant KV | NATS KV | Control | Assigns each shard a home node and accounts rate grants | `fleetd/lease`, `fleetd/grant` *(planned)* | Shard ownership and grant accounting in fleet mode | proposed (ADR 0007 accepted, not built) |
| 12 | Baseline snapshots | Object storage (provider `[U]`) | Cold recovery | Restores a durable node whose Fly Volume is lost or corrupt | durable `hunterd` node *(planned)* | A point-in-time copy, never live state | proposed |
| 13 | Process bookkeeping | Markdown under `.agents/` | Documentation | Evals, state log, constitution history | agents and humans | Process history | accepted, non-runtime |

**Not storage, and must not be treated as such:** `orchestrator/policy.yaml`, `db/config.yaml` *(not on `main`)*, signed scope bundles *(planned)* and `mission.yaml` *(absent)* are read-only configuration. They shape behaviour; they are not mutated by the runtime. `agent-graph/` is untracked scratch code and is not part of this design.

**Explicitly rejected stores.** Neo4j (the FalkorDB path is the accepted one; `.env.example` keeps a Neo4j block only for upstream Graphiti parity), a graph MCP/REST server, a document database for the audit log, and any second SQLite file whose rows could be a view over the ledger. Adding any of these requires the same review as adding a store in §2.

---

## 3. What each technology is for — and not for

| Technology | Is for | Is not for |
|---|---|---|
| **BadgerDB** | Embedded, single-process, transactional key/value state on local disk: cursor, novelty, outbox | Cross-process reads; long-term history; anything the orchestrator must query directly |
| **SQLite (WAL)** | Small, bounded, relational or append-only state owned by exactly one Python process: ledger, campaign state, checkpoints, audit | Multi-writer concurrency across containers; network filesystems; unbounded event history |
| **FalkorDB** | Multi-hop relationships with temporal validity, queried as a graph | Authoritative records; free-text search; replacing a relational `WHERE` clause |
| **Graphiti** | Episode ingestion, entity/edge extraction, temporal invalidation, hybrid search on top of FalkorDB | Storage, a message queue, a decision engine, or a source of truth |
| **NATS / JetStream** | Low-latency transport, durable at-least-once streams, KV leases and grants | Long-term evidence retention; a system of record for decisions |
| **Object storage** | Encrypted cold copies for disaster recovery | Runtime reads; a primary database |

---

## 4. Plane specifications

Each plane below is a contract: who may write, what the key is, what order is required, and what breaks when it fails.

### 4.1 Operational plane — BadgerDB (`hunterd` only)

**Purpose.** Decide novelty where the probe happens and remember the scan position, with no network hop.

**Key families.**

| Key | Value | TTL | Written by | Read by |
|---|---|---|---|---|
| `checkpoint/<authorized-scope>` | 8-byte big-endian next Feistel index | none | `Store.SaveCursor` | `Store.LoadCursor` on restart |
| `observation/<probe-key>` | JSON `{version, sha256, payload, observed_at, last_seen}` | `NewDeltaGate` TTL | `DeltaGate.Apply` | `DeltaGate.Apply` |
| `outbox/<seq>` *(planned)* | Serialized `DeltaEvent` plus enqueue time | drained on reconnect | `hunterd/store/outbox.go` *(planned)* | outbox drainer *(planned)* |

**Write path.** `DeltaGate.Apply` performs get → hash compare → decide → write in one Badger update transaction. There is no window in which two observers can both see the same key as novel. `unchanged` is written back to refresh `last_seen` and the TTL but is never published. Cursor persistence happens on the cadence required by I16 and on `SIGINT`/`SIGTERM`.

**Authority.** In single-node mode, `checkpoint/<scope>` is authoritative for scan position. In fleet mode, the NATS KV lease (§4.7) is authoritative for which node owns the shard and where the shard's committed cursor lives; the local Badger checkpoint becomes a recovery hint until the lease holder adopts it. There must never be a period in which both are treated as authoritative.

**Readers.** No other process opens the directory (I17). A remote reader asks the shard's home node over NATS request/reply — `fleetd/query/` *(planned)* — rather than mounting the volume.

**Failure.** If Badger cannot open, `hunterd` refuses to start: without novelty there is no bounded event stream. If a write fails, the probe result is not published. If the outbox is full, the producer blocks rather than dropping data.

**Retention.** Observations expire by TTL; a returning observation is novel again by design. The cursor never expires. The outbox is bounded; overflow is a hard backpressure signal, not an invitation to discard.

**Rebuild.** Cursor: rebuildable only from an operator-known checkpoint; the KV lease (§4.7) is the fleet recovery path. Observations: not rebuildable, and not worth rebuilding — losing novelty state means a one-time re-baseline, which is an accepted cost.

**Non-negotiable.** No Badger directory is shared between processes, containers, or hosts; no Python process reads it directly.

---

### 4.2 Decision plane — evidence ledger (`orchestrator/ledger.py`)

**Purpose.** Be the single authoritative record of what the system decided and why, in an order that cannot be silently rewritten.

**Shape.** One row per completed triage run: `seq`, `record_hash`, and a canonical-JSON `body`. The body contains the validated event, evidence classes, intel score, policy, `policy_sha256`, `scorer_id`, challenger results, `prev_hash`, and `recorded_at`. `record_hash` is SHA-256 over the body excluding the hash itself; `prev_hash` chains to the prior record; the first record chains to a genesis constant.

**Write path.** The `record` node runs **after** `approval` and calls `Ledger.append` inside `BEGIN IMMEDIATE`, in WAL mode. It returns no state change. A ledger fault is caught, logged, appended to `failed_event_ids`, and never raised into the graph; the decision stands even when the record is lost. That is deliberate: the ledger must not be able to veto a mitigation.

**Authority.** The ledger is authoritative for: the decision, its inputs, its policy hash, the challenger outputs, and the order of decisions. It is **not** authoritative for in-flight state (§4.5) or scan position (§4.1).

**Readers.** `verify_chain` and operator tooling read it; Role B replay (§4.6) reads it; the campaign scorer's recovery path may replay it. No LangGraph node reads it during a decision.

**Known limits that the plan must close.**

| Limit | Consequence today | Required target |
|---|---|---|
| No head anchoring | Truncation of the newest rows leaves a valid shorter chain | Persist the head `(seq, record_hash)` outside the ledger on every append, then verify it on startup and before export |
| No uniqueness on `event_id` | A redelivered event can produce two records | Suppress duplicates at ingest (`orchestrator/dedupe.py` *(planned)*), not by mutating the append-only ledger |
| No schema version in the row | A future field change has no migration marker | Add a record-envelope version key before the first external consumer |
| No retention policy | The file grows without bound | Archive verified segments; never delete an unverified row |

**Failure.** A ledger outage degrades auditability, not hunting. The orchestrator keeps deciding; failures accumulate in `failed_event_ids` and must be alarmed. A ledger file that fails `verify_chain` is a security event and blocks export, not the pipeline.

**Rebuild.** Not rebuildable: it is the authority. It is restored from backup or accepted as lost with an explicit gap record.

---

### 4.3 Challenger plane — campaign posteriors (`orchestrator/campaign.py`)

**Purpose.** Keep a per-campaign belief state across events so a shadow scorer can be evaluated without influencing policy.

**Shape.** Three tables: `campaigns` (posterior components, reference time, last seen, recent target keys), `indicators` (observed indicator value → campaign identity), and `seen` (event id → campaign, log-odds, for idempotency).

**Write path.** `CampaignScorer.observe` is called only from `run_challengers`, which runs in the `record` node after the champion decision. Its output is written into `ledger.record.challengers[scorer_id]` and nowhere else. A test on every phase that adds a scorer enforces that it never reaches `GraphState.policy`.

**Identity.** A campaign is keyed by a stable hash of **observed** indicators (`jarm`, `ja3`, `favicon`), never by target. A campaign that moves infrastructure keeps its identity; a campaign that rotates every indicator starts a new one. Producer-asserted attributes have a zero cap by construction.

**Bounds.** `campaign_max_tracked`, the per-campaign target cap, and the seen-table cap keep the file finite. Eviction deletes whole campaigns in least-recently-seen order; it is not a partial graph edit.

**Authority.** The ledger is authoritative for what the scorer *said* about an event. This file is authoritative only for the scorer's current internal state.

**Rebuild.** Replay the ledger's `event` objects through a fresh `CampaignScorer` in `seq` order. Because scoring is deterministic given the policy, the replay must reproduce the stored posteriors for the same input window; a mismatch is a bug in the scorer, not a reason to trust the file.

**Failure.** A campaign-store fault is caught by `run_challengers` and recorded as an error in the ledger record. It never blocks a decision. Deleting the file loses shadow history only.

---

### 4.4 Execution plane — durable LangGraph checkpointer

**Purpose.** Preserve in-flight graph state, especially a pending `interrupt()`, across process restart.

**Shape.** LangGraph-owned SQLite or Postgres tables. One thread per event run; the fleet plan fixes `thread_id = event_id` so a redelivery resumes the same run instead of starting a second one.

**Authority.** A checkpoint is authoritative for *where the run is*, never for *what was decided*. Once the ledger record commits, the ledger wins. A checkpoint restored after a crash may re-run a node; nodes must therefore be idempotent on `event_id`, and the ledger append must be the last side effect before `END`.

**Status.** `build_sqlite_checkpointer` exists as an opt-in helper but is not the default; `build_graph(checkpointer=None)` compiles without durability. Choosing SQLite or Postgres is **HC-4** and remains an open decision. The existence of the helper must not be read as approval.

**Decision criteria for HC-4.**

| Criterion | SQLite | Postgres |
|---|---|---|
| Deployment shape | Single orchestrator container with a local volume | Multiple orchestrator replicas or managed failover |
| Operational cost | One more file to back up | A service to run, secure and patch |
| Concurrent runs | Adequate for one process; writer contention beyond that | Native |
| Failover | Volume reattachment | Native |
| Consistency with the rest of the design | Matches the ledger and campaign pattern | Introduces the only server-based relational store |

**Failure.** Without a durable checkpointer, a restart loses a pending interrupt and the broad action must be re-proposed. That is safe (fail-closed) but operationally unacceptable for a human queue; it is why HC-4 blocks fleet phase 6.

**Retention.** Prune terminal threads after a fixed retention window; never prune a thread with a pending interrupt. The retention value is a policy key, not a pasted number.

---

### 4.5 Dev-memory plane — Role A Graphiti on FalkorDB

**Purpose.** Let an agent working *on this repository* recall what a prior session did: which `WorkBlock` ran in which worktree, what `FailurePattern` blocked it, and what depends on what. It is bookkeeping for the development process, not a runtime security component (ADR 0004-graph-memory).

**Ontology (fixed in `db/config.yaml` *(not on `main`)*).** Entities `WorkBlock`, `Worktree`, `FailurePattern`; edges `DEPENDS_ON`, `ENCOUNTERED_FAILURE`. Extending it requires an edit to that ontology file plus a recorded decision, never an ad-hoc type in code or a prompt.

**Write path (strict order).**

1. Build one JSON episode of kind `WORK_BLOCK_COMPLETED` or `WORK_BLOCK_BLOCKED` using `build_episode_body`, which stamps the episode type and truncates every string field.
2. `db/audit.record_episode` *(module not on `main`)* writes the exact payload with status `pending`.
3. `add_work_block_episode` calls Graphiti with `EpisodeType.json`.
4. `db/audit.mark_graph_write_status` *(module not on `main`)* flips the row to `committed` or `failed`.

A crash between 2 and 4 leaves a `pending` row, which is the intended recovery handle. There is no direct Cypher and no second caller of `graphiti_core`; `db/graph_client.py` *(not on `main`)* is the only module that constructs a Graphiti client for this role.

**Read path.** `search_graph` always passes the configured `group_id`. A query with the wrong group silently returns nothing, so the group id is treated as part of the schema: `sentinel_lite_main` for Role A.

**Authority and rebuild.** The audit log is authoritative. The graph is a derived, lossy index. A wrong extraction is repaired by replaying the audit rows; a lost graph volume is rebuilt from the audit log, not restored from a graph backup.

**Failure.** If FalkorDB is down, the session loses memory but the repository work continues. The audit write must still succeed; if the audit also fails, the episode is lost and must be reported, not silently dropped.

**Boundaries.** Host-side or developer-environment only. Loopback binding, no unauthenticated network exposure, telemetry disabled, `graphiti-core` pinned. The Go binaries never connect.

---

### 4.6 Analysis plane — Role B Graphiti on FalkorDB

**Purpose.** Answer relationship questions over **past** decisions after they were made: which targets share indicators, which indicators cluster into a campaign, which decisions touched which cluster. It is never consulted between an event and a mitigation.

**Status.** Proposed and blocked. `spec/DECISIONS/0006-graph-store.md` *(planned)* does not exist; the number is reserved. [Phase 0](../plans/graph-store/Phase-0.md) may legitimately end in "do not build" if no named consumer appears.

**Ontology (candidate).** Entities `Target`, `Indicator`, `Campaign`, `Decision`; edges `OBSERVED_WITH`, `CLUSTERED_WITH`, `DECIDED`. Final shape requires ADR 0006 and a Role A-style ontology file of its own *(planned)*.

**Ingestion path.**

1. A ledger record commits (§4.2).
2. `orchestrator/graph_replay.py` *(planned)* reads records from a stored `seq` watermark.
3. It builds an episode from an allowlisted subset of fields and submits it with the ledger's `record_hash` as the idempotency key.
4. It advances the watermark only after Graphiti confirms the write.

The replay watermark is itself durable SQLite state (a `(seq, record_hash, status)` row); a crash re-submits the same record, and Graphiti's idempotency key makes that harmless.

**Data rules.**

- Free text (banners, page titles, error traces) is never sent to the extraction LLM. Only allowlisted, length-capped fields.
- The episode is built from the ledger, never from a live event.
- The graph never writes back to the ledger, policy, scoring or enforcement.
- The Role B `group_id` is per engagement or per scope bundle, never `sentinel_lite_main`. The FalkorDB database name is distinct from Role A's. The two roles never share a server, volume or namespace.

**Authority and rebuild.** The ledger is authoritative. The graph is a derived projection. It can be dropped and rebuilt from `seq = 0` without losing a decision. A graph backup is a convenience, not a recovery requirement.

**Failure.** Hunting, triage, approval and enforcement behave exactly as before. The `graph` profile may be absent, the container may be down, and the orchestrator must still report ready (§6).

**Egress.** Role B runs in the orchestrator container and calls an LLM for extraction. That is a paid, networked, non-deterministic dependency and a second prompt-injection surface. ADR 0006 must name the provider, model, allowed host, data classes, retention and budget before the profile is enabled by default. Role A has the same LLM dependency but runs in a developer environment, so its egress is governed by the developer's own configuration, not by production policy.

---

### 4.7 Transport and control plane — NATS, JetStream, KV

**Purpose.** Move events and control messages, and hold the two pieces of fleet state that must be visible to more than one process: shard leases and rate grants.

**Three distinct roles, never conflated.**

| Role | Subject / bucket | Durability | Authority |
|---|---|---|---|
| Lab bus | core NATS | none | nothing |
| Fleet streams | JetStream `DELTA`, `OBS.*`, `PROBE_REQ`, `AUDIT` | retention limits, `discard=new` | delivery and replay of events |
| Lease/grant KV | KV buckets owned by `fleetd` | replicated | shard ownership and grant accounting in fleet mode |

**Write path and ordering.**

- `hunterd` publishes only `novel`/`mutated` events. In fleet mode it publishes to JetStream and records the local cursor after the publish is acknowledged, or buffers in the outbox when the publish is refused.
- `orchestrator/ingest.py` *(planned)* is a pull consumer. It **acks only after the ledger record or the checkpoint commits**. A crash between commit and ack causes redelivery, which dedupe absorbs.
- `fleetd` publishes signed `Command v1` envelopes only on `fleet.ctl.<worker>` and `hunt.probe.<shard>`. Authenticity is in the envelope, not the transport.
- `fleetd/lease` *(planned)* uses compare-and-swap on the KV revision; a lease is reclaimable only after `issue + ttl + margin`. A new leader waits one TTL before issuing.
- `fleetd/grant` *(planned)* sums grants within the signed scope bundle's global budget; a lapsed grant means rate zero.

**Authority.** JetStream is never authoritative for a decision; the ledger is. The KV lease is authoritative for shard ownership in fleet mode; the local Badger cursor is authoritative only in single-node mode (S6).

**Failure.** A NATS partition stops new events and control commands. The outbox holds observations; the orchestrator's consumer position holds unacked streams; neither may silently drop. A KV outage stops lease renewal and therefore stops dialling at the lease deadline, which is the intended fail-closed behaviour.

**Retention.** Stream limits and `discard=new` are part of the stream definition, not an operational afterthought. The audit stream is not the evidence ledger and must not be described as one.

---

### 4.8 Cold-recovery plane — object storage

**Purpose.** Restore a durable node whose local volume is lost or corrupt, including the Badger directory that cannot be rebuilt from anywhere else.

**What is stored.** Encrypted, versioned snapshots of: the Badger directory, the evidence ledger, the campaign SQLite file, the checkpointer database, and the Role A audit log. Graph volumes are **not** required, because both graph roles declare a rebuild source (§4.5, §4.6).

**Ordering.** Snapshot the authority before its derivatives: ledger before Role B graph, audit before Role A graph, Badger before the fleet KV cutover record.

**Restore order.** Ledger and audit first (they are authorities), then Badger, then derived stores by replay, then verification (`verify_chain`, audit reconciliation, Role B watermark reset).

**Authority.** None. A snapshot is a point-in-time copy. Restoring it is a deliberate rollback with an incident record, never an automatic failover.

**Status.** Proposed; provider, encryption and retention are `[U]` and require an ADR before the first non-lab deployment.

---

## 5. Interaction flows

### 5.1 Probe → novelty → publish (single node)

```text
authorized scope
      |
      v
hunterd: Feistel cursor ──> probe ──> DeltaGate.Apply (one Badger txn)
                                           |
                    ┌──────────────────────┼──────────────────────┐
                    v                      v                      v
                 novel                  mutated                unchanged
                    |                      |                      |
                    └──────────┬───────────┘                      |
                               v                                  v
                    Event → NDJSON / JetStream            refresh last_seen + TTL
                               |
                               v
                        orchestrator ingest
```

**Commit points.** The Badger transaction is the novelty decision. The publish is the handoff. The cursor advance is separate and may lag the publish; after a crash, a replayed range is re-suppressed by the novelty gate rather than re-published.

**Acceptable window.** A crash after publish and before cursor save re-probes a range; `unchanged` absorbs it. It does not duplicate events unless the observation TTL expired in between, which is accepted.

### 5.2 Ingest → decision → record → ack

```text
JetStream DELTA
      |
      v
dedupe (thread_id = event_id; content hash within window)   [planned]
      |
      v
LangGraph: cluster -> intel -> mitigate -> approval -> record
      |                    |          |                    |
      |                    |          |                    +--> ledger.append (authority)
      |                    |          +--> interrupt() for broad actions
      |                    +--> bounded refinement
      +--> checkpointer write per step (durable only when configured)
      |
      v
ack  <-- only after ledger commit or checkpoint commit
```

**Rules.** Dedupe runs before the graph. The record node runs after approval. The ack runs after the record commits. A graph outage does not prevent the ack. A ledger outage does not change the decision; it increments `failed_event_ids` and must be alarmed.

### 5.3 Human approval across a restart

```text
approval node -> interrupt(payload) -> checkpointer persists pending state
                                             |
                       process restart ------+
                                             v
                              resume(thread_id = event_id)
                                             |
                         decision -> ledger record -> ack
```

Without a durable checkpointer, the resume fails and the broad action must be re-proposed. No signature is created before the resume, so the failure is safe.

### 5.4 Development-session memory loop (Role A)

```text
session start
   |
   +--> search_graph(group_id=sentinel_lite_main)
   |         |
   |         +--> upstream DEPENDS_ON not yet passing
   |         +--> FailurePattern already linked to the package
   |
   v
work block in worktree -> verify -> commit or block
   |
   v
record_episode(pending) -> add_work_block_episode -> mark committed|failed
   |
   +--> failed/pending -> pending_episodes() -> replay from audit
```

**Rule.** Read before writing; one episode per commit or blocker, immediately. The graph is consulted for context, never for a gate: a missing edge cannot authorize or block a merge.

### 5.5 Fleet control loop

```text
fleetd leader
   |
   +--> KV: acquire/renew shard leases (CAS on revision)
   +--> grant: allocate rates within bundle budget
   +--> publish signed Command v1 on fleet.ctl.<worker>
                 |
                 v
            hunterd ctlbus: verify envelope -> dispatch
                 |
                 +--> dial within lease and grant
                 +--> Badger novelty
                 +--> JetStream DELTA / OBS.*
                 +--> outbox if publish refused
```

**Rules.** No valid lease, no dial. No valid grant, rate zero. A command envelope is verified before any field is used. The KV is the only cross-node authority for ownership.

### 5.6 Ledger → analysis graph replay (Role B)

```text
ledger (authority)
   |
   +--> graph_replay: read from watermark seq
   |         |
   |         +--> allowlist + truncate
   |         +--> episode keyed by record_hash
   |         +--> Graphiti add_episode -> FalkorDB
   |         +--> advance watermark only on success
   |
   +--> human analysis queries the graph
```

**Rules.** One-way. The graph cannot write back. Replay from zero is always safe. The watermark is the only cursor.

### 5.7 Recovery ordering

```text
1. restore ledger + audit (authorities)
2. verify chain / reconcile audit rows
3. restore Badger (if snapshot exists) or re-baseline
4. rebuild Role B from ledger watermark
5. rebuild Role A from audit rows
6. verify graph counts and search smoke test
7. resume fleet leases only after step 2
```

---

## 6. Failure and degradation matrix

| Store down | Hunting | Triage / decision | Approval | Enforcement | Readiness impact |
|---|---|---|---|---|---|
| Badger | **stops** (no novelty) | events stop | unaffected | unaffected | `hunterd` not ready |
| Badger outbox full | **stops** (backpressure) | delayed | unaffected | unaffected | degraded |
| Evidence ledger | continues | **continues** | continues | unaffected | degraded, alarm; never not-ready |
| Campaign SQLite | continues | continues (challenger error recorded) | continues | unaffected | none |
| Checkpointer | continues | continues | **pending interrupts lost**; re-propose | unaffected | degraded when an interrupt is pending |
| Role A graph | unaffected | unaffected | unaffected | unaffected | none |
| Role A audit | unaffected | unaffected | unaffected | unaffected | dev tooling alarm |
| Role B graph | unaffected | unaffected | unaffected | unaffected | **must not** affect readiness |
| NATS (fleet) | stops receiving commands | stalls | unaffected | unaffected | not ready in fleet mode |
| NATS KV lease | dialling stops at lease deadline | unaffected | unaffected | unaffected | not ready in fleet mode |
| Object storage | unaffected | unaffected | unaffected | unaffected | none |

**Prescribed readiness split.** The orchestrator exposes liveness and readiness separately:

- **Liveness** means the process can serve HTTP.
- **Core readiness** means the components required by the decision path are usable (ledger writable, checkpointer reachable when configured).
- **Analysis readiness** is a named field in the readiness payload (`graph: reachable|unreachable|not_required`), never an HTTP failure by itself.

The current container stack sets `GRAPH_REQUIRED: "true"`, makes the orchestrator `depends_on` a healthy FalkorDB, and defaults `graph_required` to true. That puts a proposed analysis side channel on the startup path and contradicts S3. The plan in §13 must invert it: graph optional by default, no startup dependency, analysis status reported separately.

---

## 7. Ordering, idempotency and crash windows

| Step | Ordering requirement | Idempotency key | Crash consequence |
|---|---|---|---|
| Novelty decision | Read/compare/write in one Badger txn | `observation/<probe-key>` | None; the txn is atomic |
| Cursor save | After the probe batch, before the next batch | scope | Re-probe of a range; novelty suppresses duplicates |
| Publish | After novelty, before cursor | `event_id` | At-least-once; dedupe absorbs |
| Ingest dedupe | Before graph invocation | `event_id` + content hash | Duplicate suppressed |
| Checkpoint | Each node | `thread_id = event_id` | Node re-runs; must be idempotent |
| Ledger append | After approval, before ack | `seq` from the chain | Redelivery → duplicate unless ingest deduped; therefore dedupe is mandatory |
| Ack | After ledger or checkpoint commit | consumer sequence | Redelivery; dedupe absorbs |
| Role A audit | Before graph write | episode payload | `pending` row found by `pending_episodes()` |
| Role A graph write | After audit | audit row id / episode name | Replay from audit |
| Role B replay | After ledger commit | `record_hash` | Re-submit; Graphiti idempotency absorbs |
| Role B watermark | After graph confirmation | `seq` | Replay of one record |
| Lease issue | Before any dial | `(shard, epoch)` | No dial after the lease deadline |
| Grant issue | Before any dial | `(worker, window)` | Rate zero when lapsed |

**At-least-once is the default everywhere.** Exactly-once effect is achieved by idempotent writes, never by pretending a distributed transaction exists.

---

## 8. Provisioning and namespace map

Three FalkorDB definitions appear across the repository — one in the Role A specification and two in the deployment stacks. They serve different planes and must be named as such, not treated as duplicates to delete.

| Stack | Compose file / service | Role | Auth | Network / port | Volume | Namespace |
|---|---|---|---|---|---|---|
| Developer graph | `db/compose.yaml` *(not on `main`)* / `falkordb` | Role A dev memory | none | loopback `127.0.0.1:6379` | `falkordb-data` (project `sentinel-hunter-graphdb`) | `FALKORDB_DATABASE=sentinel_lite`, `GRAPHITI_GROUP_ID=sentinel_lite_main` |
| Analysis graph (proposed) | `deploy/compose.yaml` / `falkordb`, profile `graph` | Role B analysis | password | `graphnet` (`internal: true`), no host port | `falkordb-data` (project `sentinel-hunter-lab`) | `FALKORDB_DATABASE=hunting_analysis` *(planned)*, `GRAPHITI_GROUP_ID=hunt_<bundle-prefix>` *(planned)* |
| Container graph | `deploy/container/compose.yaml` *(not on `main` at this revision)* / `falkordb` | **must be Role B only** | password | `sentinel-graph` (`internal: true`), debug port `127.0.0.1:16379` | `sentinel-hunter-container_falkordb-data` | same as analysis, distinct per engagement |

**Prescriptions.**

1. **Role A never runs in the application container.** It is developer tooling. The container plane owns Role B if and when Role B is approved; until then its `graph` service stays off by default.
2. **No fourth FalkorDB definition.** A new deployment reuses one of these or replaces one with an ADR; it does not add another compose file.
3. **Distinct volumes, databases and `group_id`s.** Pointing two roles at the same graph name does not merge them, and pointing them at the same volume does contaminate them.
4. **The image is pinned** to `falkordb/falkordb-server` at the version in the compose files. `latest` is forbidden.
5. **The server image, not the bundled browser image,** in every stack; no MCP/REST gateway anywhere.
6. **`graphiti-core` is pinned** to a version at or above the floor recorded in `db/CONFIG.md`, with the FalkorDB extra. An upgrade requires the regression check in that document.
7. **Persistence differs by role and is stated, not assumed.** Role A may run without AOF because the audit log can rebuild it. Role B and the container stack use AOF with `appendfsync everysec` and `noeviction`, so a full graph fails writes instead of silently dropping nodes.

**Compose networks.** The analysis graph joins only an `internal: true` network and publishes no host port. The orchestrator joins the app network and the graph network, and the graph network has no route out. The developer graph publishes loopback only. The container plane's debug overlay may publish loopback; the base stack may not.

**Startup order.**

- Ledger SQLite: available before the first event is consumed.
- Checkpointer: available before an interrupt can be raised.
- Role B graph: **not** a startup dependency of the orchestrator; the replay worker retries with backoff.
- Role A graph: not a dependency of anything; the session retries, and the audit log captures the intent.
- Fleet KV: before any hunter dials in fleet mode.

---

## 9. Security and data handling

| Control | Requirement |
|---|---|
| Network exposure | Loopback or `internal: true` only; no public port for any store; the analysis graph has no host port at all |
| Graph auth | Password in the container/analysis stacks; Role A is loopback-only and unauthenticated by design, which is acceptable only because it is unreachable off-host |
| MCP/REST | Never deploy Graphiti's reference MCP or REST server; the library is embedded |
| Credentials | Environment or generated `.env` only; never committed; the container-plane init script *(not on `main` at this revision)* refuses to overwrite |
| Telemetry | `GRAPHITI_TELEMETRY_ENABLED=false` in every stack |
| Prompt injection | JSON episodes only; allowlist and truncate every string before it reaches an extraction LLM; never ingest raw banners or logs |
| Role A audit payload | Stores the exact string **after** `build_episode_body` truncation — replayable, not forensic-full; a fuller capture would need a separate, explicitly approved store |
| LLM egress | Role A: developer-controlled. Role B: named host, model, data class, retention and budget in ADR 0006 before enablement |
| Model output | Never sets breadth, kind, TTL, scope, signature, expiry, confidence, rate or approval |
| Badger | Local disk only; never a network mount; never opened by Python |
| Retention | Each store's retention is declared in §10 and implemented as a bound, not a hope |

**The loopback guard is role-specific.** The Role A loader (`db/graph_client.load_graph_config` *(module not on `main`)*) refuses any host that is not `127.0.0.1` or `localhost`. That is correct for Role A and wrong for a client inside the container plane reaching the service name `falkordb`. The plan must not weaken the single guard. It introduces a separate Role B configuration loader that accepts only an allowlisted internal hostname, requires a password, and is constructible only when an explicit container flag is set. Role A keeps the loopback-only loader unchanged. The container-plane notes in the Role A specification *(not on `main`)* set out the three options and why relaxing the shared guard is the worst of them.

---

## 10. Retention, capacity and backup

| Store | Retention | Growth driver | Bound | Backup | Restore test |
|---|---|---|---|---|---|
| Badger observations | TTL from `NewDeltaGate` | Unique probe keys | Disk; TTL | Snapshot before fleet cutover | Restore and re-baseline |
| Badger cursor | Indefinite | Scopes | One key per scope | Snapshot | Resume scan |
| Badger outbox | Until drained | Bus outage length | Configured cap; block on overflow | Snapshot | Drain after reconnect |
| Evidence ledger | Indefinite | Events | Archive verified segments | Required, encrypted | `verify_chain` after restore |
| Campaign SQLite | Rolling, evicted by policy | Campaigns and targets | `campaign_max_tracked` + caps | Optional; rebuildable | Replay from ledger |
| Checkpointer | Terminal threads pruned after a policy window | Runs | Retention window; pending interrupts exempt | Required if it is the only copy | Resume an interrupt |
| Role A graph | Indefinite | Sessions | Volume/memory limit | Not required | Rebuild from audit |
| Role A audit | Indefinite, then archive | Episodes | Archive after graph reconciliation | Required, encrypted | Replay a sample |
| Role B graph | Per engagement | Ledger records | Volume/memory limit | Not required | Rebuild from ledger |
| Role B watermark | Indefinite | Records | One row per record | With ledger backup | Reset and replay |
| JetStream streams | Stream limits, `discard=new` | Event rate | Configured limits | Not applicable | Replay from the ledger, not the stream |
| NATS KV | TTL/revision bounds | Workers and shards | Bucket limits | Not applicable | Re-lease |
| Object storage | Per ADR | Snapshot count | Lifecycle policy | Is the backup | Quarterly restore drill |
| `.agents/` bookkeeping | Indefinite | Process history | Human-skim bound | Git | Not applicable |

**Capacity rule.** Every bound above is a configuration key. No numeric limit is copied into this document; it is cited by the key that owns it.

---

## 11. Change control

| Change | Required path |
|---|---|
| Add a store | Amend §2 and §4 here; add the writer and authority; add an ADR if it touches the decision or trust boundary |
| Change a store's authority | ADR first; the old authority is demoted in the same change; replay/rebuild proven before cutover |
| Change a cross-language payload | `schemas/` version bump + ADR + `contract-steward` |
| Change the Role A ontology | `db/config.yaml` *(not on `main`)* + recorded decision; never an ad-hoc type |
| Change the Role B ontology | ADR 0006 + a Role B ontology file of its own |
| Change a `group_id`, database or volume | Treat as a migration: dual-write is forbidden; rebuild into the new namespace and cut over |
| Change a threshold | `orchestrator/policy.yaml` only; cite the key elsewhere |
| Change a candidate store technology | ADR; the tenant of the plane changes, not the plane's role |

**Anti-patterns.** Two writers; a graph on the decision path; a second authority "just for reads"; hand-written Cypher; direct `graphiti_core` use outside the client module; a store added to solve a query that a SQL `WHERE` clause or a `jq` over the ledger would answer.

---

## 12. Open decisions blocking promotion

| ID | Decision | Blocks | Evidence required | May end in "no" |
|---|---|---|---|---|
| OD-1 | Role B consumer and ontology (`spec/DECISIONS/0006-graph-store.md` *(planned)*) | Any Role B build | Named consumer; comparison against the ledger alone; pinned versions and licences; model and embedder | Yes — Phase 0 explicitly allows "do not build" |
| OD-2 | Durable checkpointer backend (HC-4) | Fleet phase 6; any broad-action queue | SQLite vs Postgres against the criteria in §4.4 | No; one must be chosen |
| OD-3 | Role B LLM egress and data classes | Enabling the `graph` profile | Allowed host, provider, model, retention, budget, human checkpoint | Yes |
| OD-4 | Fleet storage authority cutover (Badger cursor → KV lease) | Fleet phase 4 | Migration procedure and a test that both are never authoritative | No |
| OD-5 | Object-storage provider, encryption, retention | Non-lab deployment | ADR and a restore drill | No |
| OD-6 | Ledger head anchoring | Any claim that truncation is detectable | Where the head is stored and how it is verified | No |
| OD-7 | Container stack role separation | Removing the graph from startup | `GRAPH_REQUIRED=false` default; readiness split; no `depends_on`; tests | No |
| OD-8 | Role A retention and rebuild automation | Calling the dev graph recoverable | A replay command and a reconciliation report | No |

---

## 13. Promotion plan

This is the skeleton to lift into `plans/storage/` *(planned)* once the open decisions above have owners. It does not authorize the work; it orders it so each step leaves `develop` green and can be reverted independently.

| Phase | Outcome | Depends on | Key acceptance |
|---|---|---|---|
| **S0 — Reconcile the map** | Fix the three-FalkorDB naming, document Role A vs Role B in one place, mark the stale `db/INSTALL.md` references, correct `docs/08`'s "only egress" claim | none | No document calls the container graph "the" graph; every compose file names its role |
| **S1 — Make the graph optional** | `GRAPH_REQUIRED=false` default, remove `depends_on: falkordb`, split readiness into core and analysis, test that a graph outage leaves hunting and decisions unchanged | S0 | A test starts the orchestrator with no graph and reaches ready; a test fails a decision if the graph is consulted |
| **S2 — Harden the authorities** | Ledger dedupe input, head anchor, record version, archive; audit reconciliation and Role A replay command | S0 | `verify_chain` plus head match on startup; replayed Role A graph matches the audit count |
| **S3 — Decide and wire the checkpointer** | HC-4 ADR, configuration wiring, resume test across restart | OD-2 | A pending interrupt survives a process restart and resumes exactly once |
| **S4 — Separate Role B from the runtime** | Dedicated Role B loader, namespace, watermark and replay worker; no consumer, no build | OD-1, OD-3 | Replay from zero is idempotent; graph down changes no decision |
| **S5 — Fleet storage** | NATS KV leases and grants, JetStream streams, outbox, ack-after-commit | OD-4 | Budget conservation, no loss, no dual authority |
| **S6 — Cold recovery** | Object storage, snapshot ordering, restore runbook and drill | OD-5 | Restore produces a verified ledger and a rebuilt derived store |
| **S7 — Promote the plan** | Move this content into `plans/storage/` *(planned)*, one ADR per decision, update `spec/STATE.md` | S0–S6 | `validate_outlines.py --strict` passes |

**Rules for every phase.** One branch from `develop`; tests before code; no new threshold outside `policy.yaml`; no phase enables a store whose authority and rebuild source are not written down; every merge appends to `.agents/orchestration/obs/state-log.md`.

---

## 14. Definition of done for this design

The design is promotable when all of the following are true:

- [ ] Every store in §2 has exactly one writer and a named authority.
- [ ] Every derived store has a tested rebuild path and a stated rebuild time.
- [ ] No derived store appears in the startup or readiness path of the decision plane.
- [ ] The ledger has dedupe input, a head anchor and a version marker.
- [ ] The checkpointer backend is decided and a pending interrupt survives a restart.
- [ ] Role A and Role B have separate loaders, namespaces, volumes and ontologies.
- [ ] Fleet mode has one authority for shard ownership, and a test proves the old one is not consulted.
- [ ] Every graph write is preceded by a durable raw record and is idempotent on a source key.
- [ ] Every retention bound is a configuration key, cited by name.
- [ ] No reference MCP/REST server is deployable; no store publishes beyond loopback or an internal network.
- [ ] An operator can answer "what did the system decide, why, and can it be rebuilt?" from the ledger and audit alone.

---

## Appendix A — existing and planned write/read surface

| Plane | Module | Surface | Status |
|---|---|---|---|
| Operational | `hunterd/store` | `Open`, `SaveCursor`, `LoadCursor`, `NewDeltaGate`, `Apply` | accepted |
| Operational | `hunterd/store/outbox.go` *(planned)* | enqueue, drain | proposed |
| Decision | `orchestrator/ledger.py` | `record_hash`, `Ledger.append`, `Ledger.records`, `verify_chain`, `build_record_fields`, `run_challengers`, `make_record_node` | accepted |
| Challenger | `orchestrator/campaign.py` | `observe`, `score`, `reset`, `campaign_count` | accepted |
| Execution | `orchestrator/graph.py` | `build_graph`, `build_sqlite_checkpointer` | partial |
| Dev memory | `db/graph_client.py` *(not on `main`)* | `load_graph_config`, `build_graphiti`, `init_graph`, `build_episode_body`, `add_work_block_episode`, `search_graph` | accepted design; code absent |
| Dev memory | `db/audit.py` *(not on `main`)* | `record_episode`, `mark_graph_write_status`, `pending_episodes` | accepted design; code absent |
| Analysis | `orchestrator/graph_replay.py` *(planned)* | watermark read, episode build, idempotent submit | proposed |
| Analysis | `orchestrator/graph_ontology.py` *(planned)* | Role B entity/edge map | proposed |
| Transport | `orchestrator/ingest.py` *(planned)* | pull consumer, ack after commit | proposed |
| Control | `fleetd/lease/`, `fleetd/grant/` *(planned)* | lease CAS, grant accounting | proposed |

## Appendix B — environment matrix

| Variable | Used by | Role | Notes |
|---|---|---|---|
| `FALKORDB_HOST` / `FALKORDB_PORT` | `db/graph_client.py` *(not on `main`)*, `orchestrator/service.py` *(not on `main` at this revision)* | Role A / Role B | Role A loopback only; Role B internal service name |
| `FALKORDB_PASSWORD` | container/analysis stacks | Role B | required there; absent in Role A |
| `FALKORDB_DATABASE` | Graphiti driver | both | must differ per role |
| `GRAPHITI_GROUP_ID` | `add_episode`, `search` | both | must differ per role; Role B is per engagement |
| `GRAPHITI_MAX_COROUTINES` | Graphiti | both | caps extraction concurrency |
| `GRAPHITI_TELEMETRY_ENABLED` | Graphiti | both | always `false` |
| `GRAPH_REQUIRED` | `orchestrator/service.py` *(not on `main` at this revision)* | readiness | **must default to false**; analysis status is reported separately |
| `LLM_PROVIDER`, `MODEL_NAME`, embedder keys | Graphiti extraction | both | role-specific; never committed |
| `LEDGER_*`, `CHECKPOINT_*` *(planned)* | orchestrator | decision/execution | paths and retention are deployment config |

## Appendix C — glossary

| Term | Definition |
|---|---|
| Episode | One Graphiti ingestion unit; a JSON document describing an event or work block |
| Entity / edge | A node or relationship Graphiti extracts from an episode into FalkorDB |
| Temporal validity | Graphiti's `valid_at` / `invalid_at` window on a relationship |
| `group_id` | Graphiti's namespace; on FalkorDB each value is a separate graph |
| Watermark | The last source position successfully projected into a derived store |
| Authority | The one store permitted to be right about a fact |
| Derived | Rebuildable from an authority by replay |
| Critical path | A store whose failure blocks an action |
| Side channel | A store whose failure changes only visibility |

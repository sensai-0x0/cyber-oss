**Status:** authoritative · **Derived from:** `db/graph_client.py`, `db/audit.py`, `db/config.yaml`, [CONFIG.md](CONFIG.md) · **Updated:** 2026-11-21

Field-level form of the ontology in [config.yaml](config.yaml) and the boundary in [CLAUDE.md](CLAUDE.md). This covers only `db/`'s own data — not `schemas/delta_event.v1.json` or `schemas/mitigation_action.v1.json`, which `plans/DATA_MODEL.md` owns and which this component never touches ([ADR 0004](../spec/DECISIONS/0004-graph-memory-is-not-the-security-pipeline.md)).

## 1. Graph entities

Graphiti extracts these from episode text via its LLM; the fields below are what each entity's identity and properties *should* resolve to, so extraction is consistent across episodes rather than creating near-duplicate nodes per run.

| Entity | Identity (what makes two mentions the same node) | Properties a query should be able to read back |
|---|---|---|
| `WorkBlock` | `block_id` | `cluster`, latest `test_status`, latest `test_command`, `files_owned` |
| `Worktree` | `worktree` path | `branch`, the `WorkBlock` currently occupying it |
| `FailurePattern` | A normalized description of the failure (not the raw trace — see §3 on why the raw trace lives in the audit log, not the graph) | Count of `WorkBlock`s it has blocked |

## 2. Graph edges

| Edge | From → To | Cardinality | Meaning |
|---|---|---|---|
| `DEPENDS_ON` | `WorkBlock` → `WorkBlock` | many-to-many | Target requires source to reach `test_status: PASS` before starting |
| `ENCOUNTERED_FAILURE` | `WorkBlock` → `FailurePattern` | many-to-many | This block hit this failure during verification |

Both are declared in `config.yaml`'s `edge_type_map`; Graphiti refuses to create an edge type not listed there between the given entity pair.

## 3. Episode envelope (the only write shape)

Nothing writes an entity or edge directly — every write is one of these two JSON episodes, matching [NOTES.md](NOTES.md)'s transmission diagram.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `episode_type` | string | yes | `WORK_BLOCK_COMPLETED` or `WORK_BLOCK_BLOCKED` — stamped by `build_episode_body`, never supplied by the caller |
| `block_id` | string | yes | Resolves the `WorkBlock` entity; stable across every episode about the same block |
| `worktree` | string | completed only | Resolves the `Worktree` entity |
| `branch` | string | completed only | `Worktree` property |
| `commit_sha` | string | completed only | Provenance; not currently a queryable property, kept for audit (§5) |
| `files_owned` | string[] | completed only | `WorkBlock` property |
| `test_command` | string | completed only | `WorkBlock` property |
| `test_status` | string | completed only | `WorkBlock` property; `PASS` is what `DEPENDS_ON` gates on |
| `lessons_learned` | string[] | optional | Free text; truncated per-item (`MAX_EPISODE_BODY_CHARS`) before ingestion |
| `cluster` | string | optional | `WorkBlock` property; groups blocks that can run concurrently |
| `error_trace` | string | blocked only | Drives `FailurePattern` extraction; truncated before ingestion |
| `blocks` | string | optional | `block_id` of the downstream block this failure should gate via `DEPENDS_ON`/`ENCOUNTERED_FAILURE` |

`build_episode_body` in `graph_client.py` is the only place that assembles this object; it is the schema-validation point even though nothing here uses Pydantic — the truncation pass plays the role `DeltaEvent.model_validate` plays at the security-pipeline boundary.

## 4. Audit record (`db/audit.py`, SQLite)

| Column | Type | Meaning |
|---|---|---|
| `id` | integer PK | Row identity |
| `episode_type` | text, checked | Same two values as §3 |
| `block_id` | text | Same identity as the `WorkBlock` entity |
| `group_id` | text | Must match `GRAPHITI_GROUP_ID`; cross-checking this is how a future query catches a misconfigured namespace |
| `ingested_at` | text (RFC 3339 UTC) | When this row was written, not when Graphiti finished extracting |
| `payload_json` | text | The exact string passed to `add_episode` — the full, untruncated-by-this-row record |
| `graph_write_status` | text, checked | `pending` / `committed` / `failed` — lets `pending_episodes()` find writes that never confirmed |

## 5. Where each field lives, and why

The graph and the audit log are not redundant; they hold different projections of the same episode on purpose:

| Data | Store | Why |
|---|---|---|
| `WorkBlock`/`Worktree`/`FailurePattern` nodes, `DEPENDS_ON`/`ENCOUNTERED_FAILURE` edges | **Graph** (Graphiti/FalkorDB) | The only thing this data model needs that a table can't give cheaply: multi-hop traversal ("what does block 4 transitively depend on that hasn't passed") and temporal validity windows on a relationship, both of which `graphiti.search` provides natively |
| Exact episode payload as sent, including the raw `error_trace`/`lessons_learned` text | **Document-shaped** (SQLite's `payload_json` column here; a real document DB or JSONL file would do equally well) | Graphiti's extraction is lossy and non-deterministic (LLM-driven) — if a prompt change, model swap, or outage makes `add_episode` silently extract the wrong thing, the only way to replay or audit is to still have the original payload verbatim. Schema-on-read is the right fit: the shape in §3 evolves per `episode_type` and the audit log should never have to migrate to accept a new optional field |
| `graph_write_status`, `block_id`, `ingested_at` — i.e. "did this write land, for which block, when" | **Relational** (SQLite's typed columns, indexed) | This is the part that benefits from a real schema and an index: "every episode for block-2 that never committed" is a `WHERE` clause and an index scan, not a graph traversal, and a `CHECK` constraint catches a typo in `episode_type` before it reaches Graphiti at all |

## 6. Advice for data outside `db/` with the same shape

Two things elsewhere in this repo have the same "structured + free text, queried by filter more than by traversal" shape as the audit log, and would benefit from the same relational treatment if they ever outgrow markdown:

- **`.agents/eval-results/`** (phase, suite, pass/fail, score) is tabular and gets queried by filter ("every E03 run this week") — a relational table beats a graph here; there is no relationship to traverse, only rows to aggregate. It is markdown today because the volume is low and human-readability matters more than query speed; that trade-off should be revisited only if the number of recorded runs grows past what a person can skim.
- **The constitution's fitness/mutation history** (`.agents/evals/constitution.md`'s genome-over-time record) is the same shape: append-only rows, each with a timestamp and a handful of typed fields, queried by range, never by graph traversal. Same advice, same caveat: don't introduce a database for this until skimming the markdown actually becomes the bottleneck.

Neither recommendation is actioned here — both files are read and written by the master-orchestrator flow under different ownership than `db/`, and `db/CLAUDE.md`'s boundary rule (this component does not reach into the security pipeline or the orchestration-process bookkeeping it doesn't own) applies symmetrically: this file advises, it does not modify `.agents/`.

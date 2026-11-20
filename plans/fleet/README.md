# Fleet plan: command path, interfaces and capacity, phased

**Status:** proposed · **Derived from:** [../../spec/DECISIONS/0007-fleet-command-path.md](../../spec/DECISIONS/0007-fleet-command-path.md), [hunter-fleet-design.md](hunter-fleet-design.md), [interfaces-and-agent-flow.md](interfaces-and-agent-flow.md), [../../docs/reviews/2026-11-21-codebase.md](../../docs/reviews/2026-11-21-codebase.md) · **Updated:** 2026-11-20

This is a plan, not an instruction to build. ADR 0007 is accepted (the command-path decision). Each phase below still starts with its own tests and, where marked, a human checkpoint. Every path that does not exist yet is marked `(planned)`.

## Rules every phase follows

1. **Branches.** One branch per phase, `feature/fleet-pN-<slug>`, cut from `develop`, in its own worktree. Merge to `develop` when the phase's acceptance list is green. **Never merge to `main` without a human's approval.** Today the remote has only `main`, so phase 0 starts by creating `develop` from `main`.
2. **Tests first**, per [../../.agents/testing.md](../../.agents/testing.md). Go code follows [../../.agents/go.md](../../.agents/go.md), Python follows [../../.agents/python-langgraph.md](../../.agents/python-langgraph.md), and both follow [../../.agents/orchestration/language-idioms.md](../../.agents/orchestration/language-idioms.md).
3. **Contracts first.** Any new or changed schema goes through the contract-steward skill before code.
4. **No probing outside `lab/`** until phase 1 is merged (ADR 0003, consequence 4).
5. **Thresholds are policy keys.** New tunables get a key in `orchestrator/policy.yaml` or the fleet config (planned) in the phase that introduces them. Plans cite key names, never values.
6. **Model output never sets** breadth, kind, TTL, scope, confidence, rate or approval.
7. **Record state.** Each merge appends to [../../.agents/orchestration/obs/state-log.md](../../.agents/orchestration/obs/state-log.md), and each policy or contract change appends to the decision log in [../../spec/STATE.md](../../spec/STATE.md).

## Phases

| Phase | Branch | Outcome | Depends on | Human checkpoint |
|---|---|---|---|---|
| 0 | `feature/fleet-p0-adr` | ADR 0007, this plan, the two design documents, state entries | none | **HC-1** reviewer name on ADR 0007 |
| 1 | `feature/fleet-p1-scope-bundle` | Signed scope bundle; scope check before every dial (I1 to I5) | 0 | **HC-2** scope key custody |
| 2 | `feature/fleet-p2-stable-fingerprint` | Fingerprint and delta hash without time (SHR-016); `/32` fix (SHR-006) | 0 | none |
| 3 | `feature/fleet-p3-command-envelope` | `Command v1`: sign, verify, freshness, replay cache | 1 | **HC-3** fleetd command key custody |
| 4 | `feature/fleet-p4-nats-ctlbus` | fleetd skeleton, leases, rate grants, NATS control bus, hunter fleet modes | 1, 3 | none |
| 5 | `feature/fleet-p5-fleetd-mcp` | MCP on fleetd (streamable HTTP + stdio lab mode) | 4 | none |
| 6 | `feature/fleet-p6-orchestrator-ingest` | Orchestrator consumes `DELTA`; dedupe, storm guard, ack ordering | 2, 4 | **HC-4** durable checkpointer (existing P2.2) |
| 7 | `feature/fleet-p7-backpressure` | Badger outbox, bounded streams, grant reduction on lag | 4, 6 | none |
| 8 | `feature/fleet-p8-analyst-brief` | LLM brief for top-k clusters, isolated from policy, with budgets | 5, 6 | **HC-5** LLM data handling and model |
| 9 | `feature/fleet-p9-approval-queue` | Approval coalescing, proposal expiry, outcome labels | 6 | none |
| D | see the design doc §20.3 (F1 to F6) | Images, hardening, Fly.io deployment, autoscaling | 1 to 7 | **HC-6** Fly AUP; **HC-7** merge to `main` |

Phases 1 and 2 can run in parallel. So can 5, 6 and 7 once phase 4 is merged.

---

## Phase 0: decision and documents

**Files**

| Path | Change |
|---|---|
| `spec/DECISIONS/0007-fleet-command-path.md` | New: the accepted decision |
| `plans/fleet/README.md` | New: this plan |
| `plans/fleet/hunter-fleet-design.md` | New: images, sharding, leases, rate grants, scope, hardening, Fly.io, sizing |
| `plans/fleet/interfaces-and-agent-flow.md` | New: IF-01 to IF-18, capacity, the agent walkthrough, overload |
| `.agents/orchestration/obs/state-log.md` | Decision rows and plan pointer |
| `spec/STATE.md` | Decision-log line for ADR 0007 |

**Acceptance:** `validate_outlines.py --strict` passes. ADR 0007 is linked from the state log.

**HC-1:** a named reviewer is added to ADR 0007.

---

## Phase 1: signed scope bundle and the pre-dial check

Closes SHR-011 for scope (rate follows in phase 4) and gives the open question "scope configuration shape" in the state log its answer.

**Files**

| Path | Change |
|---|---|
| `schemas/scope_bundle.v1.json` (planned) | Targets, exclusions, ports, probe kinds, rate fields (`global_cps`, `per_block_cps`, `per_ip_per_min`, `per_worker_max_cps`), domain watchlist, window, epoch, key id, signature |
| `hunterd/scope/bundle.go` (planned) | Parse, RFC 8785 canonical bytes, Ed25519 verify against embedded keys, window and epoch checks |
| `hunterd/scope/authorizer.go` (planned) | `Check(ctx, addr netip.Addr, port uint16, kind Kind) error`; `CheckHost` resolves through the pinned resolver and requires **every** address in scope (I2); returns `ErrOutOfScope` |
| `hunterd/scope/keys/` (planned) | Embedded public keys (`go:embed`) |
| `hunterd/scope/*_test.go` (planned) | Table tests, fuzzing (`FuzzCheck`), golden canonical vectors |
| `hunterd/probes/tls_runner.go` | Takes an `Authorizer`; checks before `service.Connect`; adds a dialer `Control` hook (L2) where tlsx accepts one |
| `hunterd/probes/http_runner.go` | Same; redirects re-checked; also fixes SHR-017 (unsynchronised callback) while the file is open |
| `operator/cmd/operator/main.go` | `operator scope sign` and `operator scope verify`, refusing to overwrite keys (also fixes SHR-010) |
| `.agents/evals/evals.md` | E03 retargeted to `hunterd/scope` |

**Evals:** E03 (0 off-scope packets, by fuzz and packet capture in the lab, hard), E22 for layers L1 and L2 (hard).

**Acceptance:** an empty or unsigned bundle refuses to start (I4); a hostname with one out-of-scope address is rejected (I2); a redirect off scope is not followed; `go test -race ./hunterd/...` passes.

**HC-2:** who holds the offline scope key and its backup, where, and how a bundle is signed for an engagement. Record the answer in the ADR, not in the repo's secrets.

---

## Phase 2: stable fingerprint, `/32`

**Files**

| Path | Change |
|---|---|
| `hunterd/probes/probe.go` | Content hash over target and sorted observed attributes **without** `observed_at`; `event_id` derived from content hash plus pass id; attribute `content_sha256`; handle the marshal error |
| `hunterd/probes/testdata/attribute_keys.json` | Add `content_sha256` |
| `orchestrator/provenance.py` | Mirror the key list (contract test) |
| `hunterd/cursor/feistel.go` | `hostBits == 0` handled without panic (SHR-006) |
| `hunterd/store/badger_store.go` | Tuned options and `GOMEMLIMIT` guidance (default options peaked at about 720 MB RSS in a write test); `LoadCursor` reports "not found" separately from 0 (SHR-019) |

**Evals:** E26 funnel stability (a repeated pass with no content change yields 0 events, hard).

**Acceptance:** Go and Python contract tests agree; E26 green; the Badger memory test from the design doc's Appendix A stays below the documented ceiling.

---

## Phase 3: command envelope

**Files**

| Path | Change |
|---|---|
| `schemas/command.v1.json` (planned) | Envelope: `cmd_id`, `issued_at`, `issuer`, `to`, `intent`, `lease`, `body`, `bundle_sha256`, `signature` |
| `fleet/command/envelope.go` (planned) | Types, canonical bytes, `Sign(key, *Command)` |
| `fleet/command/verify.go` (planned) | `Verifier.Verify(ctx, raw []byte) (Command, error)`: signature, freshness, recipient, bundle hash, epoch, then the replay cache (bounded LRU by `cmd_id`, 10 min) |
| `fleet/command/testdata/` (planned) | Golden vectors: valid, altered body, altered intent, stale, replayed, wrong recipient, wrong bundle, unknown key |
| `fleet/command/*_test.go` (planned) | Table tests over the vectors, plus a fuzz target |

**Evals:** E31 command envelope (every invalid vector rejected, every valid one accepted exactly once, hard).

**Acceptance:** verification order is fixed by test; the error type tells you which check failed; no exported function accepts an unverified command.

**HC-3:** where the fleetd command key lives (platform secret), how it is rotated (next public key embedded one release ahead), and who can read it.

---

## Phase 4: fleetd skeleton and the NATS control bus

**Files**

| Path | Change |
|---|---|
| `fleetd/cmd/fleetd/main.go` (planned) | `serve`; leader election on NATS KV; `run() error` pattern |
| `fleetd/lease/` (planned) | Lease table with CAS on KV revision, epochs, reclaim after `issue + ttl + margin`; a new leader waits one TTL before issuing |
| `fleetd/grant/` (planned) | Rate grants whose sum stays within the bundle's global budget; slow start; max-min fair allocation |
| `fleetd/ctl/` (planned) | Signs and publishes `Command v1` |
| `hunterd/ctlbus/nats.go` (planned) | Subscribe to `fleet.ctl.<worker>` and `hunt.probe.<shard>`; verify with `fleet/command`; dispatch |
| `hunterd/ratelimit/` (planned) | Local limiter at the granted rate plus per-block and per-IP buckets (I6); a lapsed grant means rate 0 |
| `hunterd/cmd/hunterd/main.go` | `serve --mode=durable|burst --nats URL`; heartbeat loop; stop dialling at the local lease deadline |
| `hunterd/store/badger_store.go` | Cursor commit on heartbeat (I16, wires `SaveCursor`) |
| `schemas/heartbeat.v1.json`, `schemas/probe_request.v1.json` (planned) | Contracts |
| `deploy/nats/nats.conf` | Accounts and per-role subject permissions (fleetd only on `fleet.ctl.>` and `hunt.probe.>`) |
| `deploy/certs/generate.sh` | Lab NKeys and JWTs per role |
| `deploy/compose.yaml` | `fleet` profile: NATS, fleetd, three hunters, lab sink, all on loopback |

**Evals:** E21 budget conservation under chaos (hard), E23 coverage (hard), E24 kill-switch latency (hard), E08 crash recovery retargeted.

**Acceptance:** in the Compose lab, killing any hunter or the fleetd leader never pushes the sink's observed rate above the bundle budget in any 1 s window; every lab address is probed in each pass; a paused fleet stops dialling within 2 s.

---

## Phase 5: MCP on fleetd

**Files**

| Path | Change |
|---|---|
| `fleetd/mcp/server.go` (planned) | Tools `fleet_status`, `request_probe`, `request_revantage`, `query_observation`, strict JSON schemas; each call is validated, scope-checked (L0), charged to `suggest_quota_per_hour`, then turned into a `Command v1` |
| `fleetd/mcp/http.go` (planned) | Streamable HTTP with mTLS, bound to the private network only |
| `fleetd/mcp/stdio.go` (planned) | `fleetd mcp --stdio --lab`: same tools, one in-process hunter, refuses a bundle with non-lab ranges |
| `fleetd/query/` (planned) | `query_observation` asks the shard's home node over NATS request/reply (Badger stays one process, I17) |
| `orchestrator/fleet_client.py` (planned) | MCP client used by the brief and by operator tooling |
| `orchestrator/tests/test_fleet_client.py` (planned) | Against the stdio lab server |

**Evals:** E32 MCP boundary (no tool sequence widens scope, raises a budget, pauses or halts the fleet, or reaches mitigation, hard).

**Acceptance:** an out-of-scope `request_probe` returns `ERR_OUT_OF_SCOPE` and creates no work; quota exhaustion returns `ERR_RATE_LIMIT`; the stdio lab mode passes the same tool tests as HTTP.

---

## Phase 6: orchestrator ingest

**Files**

| Path | Change |
|---|---|
| `orchestrator/ingest.py` (planned) | JetStream pull consumer on `DELTA`, batch 64, `max_ack_pending` 256; ack only after the ledger record or checkpoint commits |
| `orchestrator/dedupe.py` (planned) | `thread_id = event_id` for redelivery; content dedupe on `content_sha256` within `dedupe_window_hours` |
| `orchestrator/storm.py` (planned) | Holds `rebaseline` events and bursts above `storm_factor`; release path |
| `orchestrator/graph.py` | Durable checkpointer wired by configuration (per HC-4) |
| `orchestrator/policy.yaml` | `dedupe_window_hours`, `storm_factor` |
| `orchestrator/tests/test_ingest.py` (planned) | Redelivery, poison message to dead letter, crash between ledger and ack |

**Evals:** E28 no loss (with the orchestrator stopped and restarted, every event is ledgered exactly once, hard).

**HC-4:** the durable checkpointer (SQLite or Postgres), the pending checkpoint already named in `plans/planv1.0/PHASE2.md` P2.2.

---

## Phase 7: backpressure

**Files**

| Path | Change |
|---|---|
| `hunterd/store/outbox.go` (planned) | Durable outbox for events the stream refuses; drained in order |
| `fleetd/grant/backpressure.go` (planned) | Halve grants while `obs_publish_lag` exceeds `delta_lag_backpressure_seconds`; ramp back with slow start |
| `deploy/nats/` stream definitions (planned) | `DELTA`, `OBS.*`, `PROBE_REQ`, `AUDIT` with limits and `discard=new` |

**Evals:** E28 extended (fleet rate below 50 % within 3 min of the orchestrator stopping; nothing lost).

---

## Phase 8: analyst brief (LLM)

**Files**

| Path | Change |
|---|---|
| `orchestrator/brief.py` (planned) | Window selector (`top_k`, `brief_window_seconds`); field allowlist; Messages API call with structured output `Brief v1`; `refusal` and `max_tokens` handled as no brief; hourly and daily budgets |
| `schemas/brief.v1.json` (planned) | Output schema; suggested intents limited to `request_probe`, `revantage`, `watch` |
| `orchestrator/policy.yaml` | `top_k`, `brief_window_seconds`, `brief_model`, `brief_model_broad`, `llm_hourly_usd`, `llm_daily_usd`, `suggest_quota_per_hour` |
| `orchestrator/tests/test_brief_isolation.py` (planned) | Policy output byte-identical with the brief on, off or adversarial (mocked model; tests never call a live model) |

Model IDs: `claude-sonnet-5-5` for briefs, `claude-opus-5-5` for briefs attached to broad actions, per [../../.agents/orchestration/models.md](../../.agents/orchestration/models.md). The Claude integration rules in [../graph-store/Claude-api.md](../graph-store/Claude-api.md) apply (no sampling parameters, structured output, prompt caching, typed errors, key from the environment only).

**Evals:** E27 brief isolation (hard), E30 LLM budget (hard), E05 prompt injection retargeted onto the brief.

**HC-5:** the organisation's data-retention terms for sending sanitised cluster evidence to the API, the field allowlist, and the daily budget.

---

## Phase 9: approval queue and labels

**Files**

| Path | Change |
|---|---|
| `orchestrator/approvals.py` (planned) | One pending approval per campaign with an event count; expiry after `proposal_ttl_seconds` recorded as `expired_unreviewed`; cap `max_pending_approvals` |
| `schemas/outcome.v1.json` (planned) | Approve, reject, late label (true, benign, unsure), linked to the decision's ledger `record_hash` |
| `orchestrator/ledger.py` | Outcome records |

**Evals:** E29 approval coalescing (soft).

Coordinate with `plans/ml-add` Phase 5 (proof-carrying review): outcome records must not change the signed `MitigationAction` bytes. That belongs to ADR 0005.

---

## New eval IDs

E14 to E16 are reserved for the CLI and E17 to E20 for the campaign belief ledger. This plan uses E21 to E32:

| ID | Name | Gate | Phase |
|---|---|---|---|
| E21 | Budget conservation under chaos | Hard | 4 |
| E22 | No out-of-scope SYN (layers L1 to L3) | Hard | 1 (L1, L2), D (L3) |
| E23 | Shard coverage | Hard | 4 |
| E24 | Kill-switch latency | Hard | 4 |
| E25 | Reproducible image | Soft | D |
| E26 | Funnel stability | Hard | 2 |
| E27 | Brief isolation | Hard | 8 |
| E28 | Backpressure without loss | Hard | 6, 7 |
| E29 | Approval coalescing | Soft | 9 |
| E30 | LLM budget | Hard | 8 |
| E31 | Command envelope | Hard | 3 |
| E32 | MCP boundary | Hard | 5 |

## Human checkpoints

| ID | Question | Blocks |
|---|---|---|
| HC-1 | Named reviewer on ADR 0007 | nothing (recorded) |
| HC-2 | Scope key custody and signing ceremony | phase 1 merge |
| HC-3 | fleetd command key custody and rotation | phase 3 merge |
| HC-4 | Durable checkpointer | phase 6 merge |
| HC-5 | LLM data handling, field allowlist, budget | phase 8 merge |
| HC-6 | Fly.io acceptable-use position on authorised scanning | any non-lab deployment |
| HC-7 | Merge `develop` to `main` | every release |

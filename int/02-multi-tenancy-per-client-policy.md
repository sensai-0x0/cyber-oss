# Multi-Tenancy & Per-Client Policy Design

**Status:** proposed · **Derived from:** `docs/roadmap.md` (Horizon 2), `spec/SCOPE.md`, `spec/STATE.md` (decision log 2026-11-20), `orchestrator/policy.yaml`, `hunterd/scope/` (planned in `int/01-scope-enforcement-spec.md`) · **Updated:** 2026-11-21  
**Audience:** implementer / product lead · **Blocks:** operating more than one client externally  
**Location (planned):** `orchestrator/policy/` (multi-client), `hunterd/scope/` (per-client allowlists)

## 1. Problem

Today `orchestrator/policy.yaml` is a **single global config** — one threshold set for one process:

```yaml
mitigation_confidence: 0.8
broad_prefix_threshold: 24
default_ttl_seconds: 86400
max_refinement_cycles: 1
...
```

The roadmap (Horizon 2) explicitly identifies this as a gap: *"Today `policy.yaml` is a single global config; multi-tenant operation (more than one authorized scope/client at a time) likely needs the threshold set to become scope-scoped rather than process-global."*

`spec/STATE.md` shows the same drift pattern: three files previously recorded the same threshold as three different numbers, and outlines now cite key names, never values. A per-client design must avoid that by making **scope** the unit of policy resolution, with thresholds referenced by key name, not hardcoded.

## 2. Design principles

1. **Scope is the unit of authorization and policy.** Each client = one authorized scope (CIDRs/domains/ports/exclusions/rate/time-window per `spec/SCOPE.md`) + one policy profile.
2. **Thresholds resolve per scope, never globally.** `policy.yaml` keys are referenced by name; values live in a per-client profile. No magic numbers in code.
3. **The trust boundary does not change.** `hunterd` collects, `orchestrator` proposes, `operator` signs, `enforcerd` enforces — per client, with no cross-client leakage of observations, signatures, or enforcement state.
4. **Default deny.** A scope with no profile (or a profile that fails to load) cannot scan or mitigate. `load_policy` refuses to load if the pinned client badlist is missing or edited (existing behavior, preserved per `spec/STATE.md`).
5. **Broad actions stay human-gated.** Per-client `broad_prefix_threshold` and `max_ttl_seconds` are bounds, not permissions to skip `operator` approval.

## 3. Data model

### 3.1 Client / scope registry

Each client is a **scope record**:

```yaml
# clients/<client-id>.scope.yaml  (planned path; owned by hunterd/scope)
client_id: acme-corp
owner: "Acme Corp Security"
evidence: "ENG-2026-11-21 authorization record"
time_window:
  start: "2026-12-01T00:00:00Z"
  end: "2026-12-31T23:59:59Z"
rate_limit_pps: 10
cidrs: ["10.0.0.0/8"]
hosts: ["example.com"]
exclusions: ["10.0.53.0/24"]
ports: [443, 8443]
policy_ref: clients/acme-corp.policy.yaml   # links to the per-client policy profile
```

### 3.2 Per-client policy profile

```yaml
# clients/<client-id>.policy.yaml  (planned path)
client_id: acme-corp
mitigation_confidence: 0.80
broad_prefix_threshold: 24          # /24 and wider = broad (inclusive, per SHR-007)
broad_prefix_threshold_v6: 56
default_ttl_seconds: 86400          # 24h ceiling enforced by enforcerd/ttl.go MaxTTL
min_approved_ttl_seconds: 300
max_approved_ttl_seconds: 86400
max_refinement_cycles: 1
auto_approve_narrow: false
badlist_path: clients/acme-corp.badlist.v1.json
badlist_sha256: "..."
campaign_half_life_hours: 24
campaign_prior: 0.1
campaign_cap_derived: 0.4
campaign_cap_repeat: 0.2
campaign_max_tracked: 1000
ledger_append_attempts: 3
```

Key names match `orchestrator/policy.yaml` exactly; only values differ per client. `orchestrator/policy.load_policy` is extended to accept a `client_id` and load `clients/<client-id>.policy.yaml`, refusing on missing/edited badlist (existing contract).

### 3.3 Global defaults (fallback only)

`orchestrator/policy.yaml` remains as the **schema reference and default value source**, not as the runtime config for a multi-tenant process. A client profile omitting a key inherits the global default. This preserves the existing single-client deployment as the degenerate case (`client_id: default`).

## 4. Resolution flow

For every event the orchestrator processes:

1. `hunterd` emits a `DeltaEvent` carrying the `client_id` of the scope that produced it (the scope key is bound at scan time by `hunterd/scope.Allowlist`).
2. `orchestrator.graph` looks up the client's policy profile by `client_id`:
   - Cache keyed by `client_id` + profile mtime (version-checked reload, like the existing `policy.yaml` loader).
   - Missing profile → event routed to `observe` only; no mitigation proposed; audit logged.
3. `score.py` computes confidence using that client's `badlist_path`/`badlist_sha256` and campaign parameters.
4. `policy.py` synthesizes policy using that client's `mitigation_confidence`, `broad_prefix_threshold`, `auto_approve_narrow`, `max_refinement_cycles`.
5. Broad/narrow determination derives from that client's target breadth (`enforcerd.Breadth.Derive`), consistent with `spec/STATE.md` (Python `policy.target` is the bare `classify_target` form so Go re-derives the same breadth).
6. `operator.Approve`/`Reject` and `enforcerd` act on the signed action; TTL bounds come from that client's `min_approved_ttl_seconds`/`max_approved_ttl_seconds`, hard-capped by `enforcerd/ttl.go MaxTTL = 24h`.

## 5. Deployment topology for multi-tenancy

Two models; choose by client isolation requirements:

| Model | Description | Trade-off |
|---|---|---|
| **Single process, muxed scopes** | One `hunterd` + one `orchestrator` process holds many client scopes/profiles; `client_id` multiplexes events through the graph. | Simplest ops; requires strict confidence that no cross-client leakage occurs in the graph, checkpointer, or NATS subject. |
| **Per-client process** | One `hunterd`/`orchestrator` pair per client (isolated checkpointer, NATS consumer, enforcerd). | Strongest isolation matches the four-service trust boundary; higher ops cost; recommended for the first external clients. |

The trust boundary (no code path from `orchestrator` to a backend) holds in both models, but **per-client process** is the safer default until the muxed model is audit-proven. `enforcerd` can be centralized per-backend (one nftables/RPZ instance serving many clients) only if client isolation is enforced at the action level (target + client_id in the signed payload, verified before apply).

## 6. Operator surface for multi-tenancy

The `operator/` CLI (`Approve`/`Reject`, Ed25519 signing) is already correct and tested. What is missing for a firm:

- **Per-client approval queue:** broad-action `interrupt()` payloads (`action_id`, `target`, `confidence`, `max_ttl_seconds`, `client_id`, evidence digest) routed to the named operator for that client.
- **Client-scoped signing keys:** each client's actions signed with a key bound to that client (or a per-client key ID in the signature metadata), so a compromise of one client's key does not forge another's.
- **Approval audit:** ledger export (`python -m orchestrator.ledger export`) per client, readable by `operator review -ledger`, recomputing the whole hash chain (ADR 0005 decision 4).

## 7. What does not change

- `enforcerd/ttl.go` `MaxTTL = 24h` remains one constant at three enforcement points (signing, approval, enforcement). Per-client `max_approved_ttl_seconds` may be lower, never higher.
- `enforcerd` signature verification is versioned and client-agnostic; `MinVersion` (ADR 0005) applies uniformly.
- The ledger remains Python-owned SQLite per ADR 0004 decision 4; Go does not open SQLite. Per-client ledgers or a client column — decide at implementation; do not let Go read SQLite.
- `schemas/delta_event.v1.json` and `schemas/mitigation_action.v1/v2.json` are unchanged; `client_id` rides as an attribute or envelope field, not a schema change.

## 8. Open questions

- **client_id provenance:** bound to the event at `hunterd` scope-check time, or re-derived in the orchestrator from the target? Binding at scan time is simpler and auditable.
- **Muxed vs. per-client process:** start with per-client process for external clients; evaluate muxed only after an audit.
- **Centralized enforcer isolation:** if one `enforcerd` serves many clients, how is client-level revocation/TTL tracked without cross-client crosstalk?
- **Profile reload cadence:** can a client's policy be updated without restarting their process? (muxed model only)

## 9. Acceptance (before external multi-client use)

- [ ] `orchestrator/policy.load_policy(client_id)` loads `clients/<id>.policy.yaml`, refuses on missing/edited badlist, falls back to global defaults for omitted keys.
- [ ] `hunterd/scope.Allowlist` carries `client_id`; events emitted with it.
- [ ] Two clients with different `mitigation_confidence` scores produce different policies for identical events (test with fixtures).
- [ ] Broad/narrow determination uses each client's own `broad_prefix_threshold` and `enforcerd.Breadth.Derive`.
- [ ] A client with no profile routes to `observe` only, no mitigation, audit logged.
- [ ] E03/E04/E05 hard gates pass per client (scope enforcement, TTL boundaries, prompt-injection resistance) — these are protected clauses and do not scale down with tenant count.

# ADR 0007: Fleet command path: MCP to fleetd, signed NATS commands to hunters

- Status: accepted
- Date: 2026-11-20
- Approved by: Sunyata (`metacogna-lab`), in the project thread on 2026-11-20 ("Yes, and make a plan"), after a written comparison of an MCP server on each hunter against signed NATS commands. Add a named reviewer here when one signs off on the implementation.
- Numbering: 0005 and 0006 are reserved by `plans/ml-add/Phase-5.md` and `plans/graph-store/Phase-0.md`.

## Context

`hunterd` is today one process with a gRPC health endpoint and an NDJSON stream on stdio. ADR 0003 records that it is "not an MCP server, by decision". The fleet design (`plans/fleet/hunter-fleet-design.md`) runs many hunters: durable nodes that own Badger, and burst workers that live for minutes on Fly.io Machines (or Cloud Run Jobs). Something has to tell those hunters what to scan, at what rate, and when to stop. Separately, the agent side wants a tool interface it can call.

Two shapes were compared:

| | MCP server on each hunter | Signed commands on NATS |
|---|---|---|
| Fit for the agent | Native tool calls | Needs a translator (fleetd) |
| Hundreds of short-lived workers | Agent must find and connect to each one | Publish once; whichever workers exist take the work |
| Worker dies mid-task | Connection breaks, work lost | Unacked work is redelivered |
| Inbound exposure | Every hunter listens | Hunters only dial out; nothing listens |
| Authenticity | Transport only (mTLS); a compromised caller can issue anything | Each command individually signed, scoped, expiring, replay-protected and auditable |
| One hunter on a laptop | Simplest | More moving parts |

## Decision

1. **Agent ↔ fleetd: MCP.** fleetd (planned, `fleetd/`) is the only MCP server in the system. In a deployment it serves MCP over streamable HTTP, reachable only on the private network (Fly 6PN) with mTLS. Its tool set is small and bounded: `fleet_status`, `request_probe`, `request_revantage` and `query_observation`. There are no tools for scope, rate budgets, pause, halt or mitigation. Those belong to the operator.
2. **fleetd → hunters: signed `Command v1` envelopes over NATS.** Each command is canonical JSON (RFC 8785) signed with Ed25519 by a fleetd command key. Hunters verify, in this order: signature against keys embedded in the image, freshness (30 s), recipient, the hash of the scope bundle the hunter has verified, lease epoch, and a replay cache keyed by `cmd_id`. Only then do they apply their own scope check (I1, I2) to any target. NATS subject permissions also limit publishing on `fleet.ctl.*` and `hunt.probe.*` to fleetd, as defence in depth.
3. **The envelope, not the transport, carries authenticity.** A future move to another transport changes an adapter (`ctlbus`), not the payload or the verification code.
4. **stdio MCP only for local and lab runs.** `fleetd mcp --stdio --lab` (planned) runs the same tool set over stdio with one in-process hunter, so a developer or the lab can drive a single hunter without NATS. `hunterd` itself still never serves MCP (ADR 0003 stands). The lab mode refuses to start unless the scope bundle lists only lab ranges.
5. **Scope stays with the operator.** Hunters accept targets only inside an Ed25519-signed scope bundle whose verifying key is compiled into the image. fleetd distributes bundles but cannot create or widen them. A command that names an out-of-scope target is rejected by the hunter even if its signature is valid.
6. **Two keys, two custodians.** The scope key is offline and held by the operator. The fleetd command key is online (a platform secret) and can be rotated by shipping an image that embeds the next public key. Neither key can sign a `MitigationAction`. That remains the operator's key (ADR 0001, ADR 0002).

## Consequences

- One MCP surface to secure and audit, not one per hunter. Hunters have no inbound listener apart from health and metrics on the private network.
- New service: `fleetd` (lease table, rate grants, command signing, MCP). It is trusted for liveness, not for scope.
- New contracts through the contract-steward skill: `command.v1`, `heartbeat.v1`, `probe_request.v1`, `scope_bundle.v1`, `candidate_host.v1`, `observation.v1`.
- New evals: E31 (command envelope: forged, altered, stale, replayed, wrong-recipient and wrong-bundle commands are all rejected) and E32 (MCP boundary: no tool call can widen scope, raise a budget or reach mitigation). Both are hard gates.
- `hunterd/cmd/hunterd` gains fleet modes (`--mode=durable|burst`). The NDJSON stdio transport stays for tests and the lab.
- The interface catalogue and capacity figures live in `plans/fleet/interfaces-and-agent-flow.md`. The phased build is `plans/fleet/README.md`.

## Residual risks, stated plainly

- **A compromised fleetd** can command any hunter to probe any in-scope target and can hand out rate grants above the engagement budget. It cannot probe outside scope, and it cannot exceed the per-/24 and per-IP limits, because hunters enforce those locally from the signed bundle. The scope bundle therefore carries `per_worker_max_cps`, and the platform caps the number of machines per app. The worst case is `machines × per_worker_max_cps` against in-scope targets, inside per-block limits, until the operator's signed halt or the bundle's expiry.
- **Replay of an older, still-valid scope bundle** to a fresh burst worker is possible until that bundle's `not_after`. Short bundle lifetimes (24 h default, 4 h for sensitive engagements) and the operator's signed halt bound it.
- **stdio lab mode** could be pointed at real ranges by editing its bundle. The lab check in Decision 4 and the bundle signature make that a deliberate act by the scope-key holder, not an accident.

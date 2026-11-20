# Architecture

**Status:** authoritative · **Derived from:** the tree, [DECISIONS/0001-service-boundaries.md](DECISIONS/0001-service-boundaries.md) · **Updated:** 2026-11-21

## Trust and data flow

```text
authorized scope
      |
      v
hunterd: Feistel cursor -> tlsx/httpx -> Badger novelty gate
                                              | novel/mutated only
                                              v
                                   bounded NDJSON / NATS subject
                                              |
                                              v
                    orchestrator: Cluster -> Score -> Policy
                                      ^          |
                                      |__ at most one refinement
                                               |
                                      broad? native interrupt()
                                               |
                                               v
                              operator: named decision + Ed25519
                                               |
                                               v
                           enforcerd: verify -> RPZ/nft -> TTL revoke
```

- **`hunterd/cmd/hunterd/main.go`** provides the CLI, gRPC health listener, and bounded NDJSON transport. Active probing is a separate adapter operation and must be scope-checked by its caller.
- **`hunterd/cursor/feistel.go`** implements the keyed four-round IPv4 bijection and O(1)-memory resumable iterator.
- **`hunterd/store/badger_store.go`** owns BadgerDB, observation TTLs, novel/unchanged/mutated decisions, and durable cursors.
- **`hunterd/probes/`** normalizes tlsx and httpx results into DeltaEvent v1. CT streaming uses the same `source: ct` contract but no external CT session is started by tests.
- **`orchestrator/graph.py`** wires the three agents; `cluster.py`, `score.py`, and `policy.py` keep their responsibilities separate. `policy.yaml` is the single threshold source.
- **`operator/`** turns review decisions into signed actions; it has no firewall access.
- **`enforcerd/verify_signature.go`** fails closed before signed backends. `enforcerd/rpz/` and `enforcerd/nft/` validate input and invoke executables without a shell. `ttl.go` owns the 24-hour limit.
- **`deploy/`** is a loopback lab, not production deployment manifests. NATS TLS material is generated locally and never committed.

## Backpressure and durability

The daemon queue is bounded. Producers block or stop on cancellation rather than dropping silently. Badger checkpoints persist sequence positions, not generated address lists. Production orchestration must configure a durable LangGraph checkpointer and durable NATS consumer; the in-memory checkpointer is test-only.

## Current scaffold boundary

Unit-tested primitives and CLI/library seams are implemented. A production NATS consumer, durable deployment checkpointer, CT checkpoint recovery, and centralized enforcer daemon transport remain integration work. CI does not claim end-to-end lab success merely because Compose parses.

---
name: pipeline-builder
description: Build and modify hunterd, orchestrator, enforcerd, and operator against test-first acceptance criteria. Use for feature implementation, bug fixes, refactors, daemon wiring, and CI build failures.
---

**Status:** authoritative · **Derived from:** `spec/`, `schemas/` · **Updated:** 2026-11-21

# Pipeline builder

Calls: contract-steward

## Workflow

1. If a shared payload or action changes, invoke `contract-steward` and keep its compatibility summary with the work.
2. Read `CLAUDE.md`, `.agents/go.md`, `.agents/python-langgraph.md`, `spec/ARCHITECTURE.md`, and nearby tests.
3. Write failing tests for the requested behavior before code. Use fakes for DNS, nftables, NATS, network probes, and clock time.
4. Implement the smallest vertical slice through the affected daemon. Preserve O(1) cursors, bounded queues, novelty suppression, and approval/TTL checks.
5. Run focused tests, then `go vet ./...`, `go test -race ./...`, Ruff, and pytest.
6. Invoke `lab-validator` when the change affects runtime wiring, deployment, signatures, or revocation.

## Handoff

Report changed components, tests and evidence, unresolved integration risks, and whether lab validation is required. Do not run live scans or real firewall changes as build verification.

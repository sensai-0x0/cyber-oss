**Status:** authoritative · **Derived from:** `schemas/`, [../.agents/outline-method.md](../.agents/outline-method.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Versioned JSON Schema contracts shared by Go and Python. They rank above specs and agent docs in the authority ladder.

| File | Carries | Producer to consumer |
|---|---|---|
| `schemas/delta_event.v1.json` | Novel or mutated observation | `hunterd` to `orchestrator` |
| `schemas/mitigation_action.v1.json` | Signed, TTL-bounded action | `operator` to `enforcerd` |

**Boundaries**

- Never change v1 semantics in place. A breaking change is a new `v2` file plus an ADR in `spec/DECISIONS`.
- Both schemas set `additionalProperties: false`; do not loosen that to make a payload pass.
- Delta event `kind` stays `novel` or `mutated` only (novelty boundary). Action `ttl_seconds` maximum stays 86400 (24-hour ceiling, I11).
- Keep the Go structs (`hunterd/probes/probe.go`, `enforcerd/enforcer.go`, `operator/cmd/operator/main.go`) and the Pydantic model in `orchestrator/state.py` in step with the schema. The signed canonical form is built in `enforcerd/verify_signature.go`.
- Signatures are verified in Go; a schema match is never proof of approval.

**Rules that apply**

- Start every change with the `contract-steward` skill: `.agents/skills/contract-steward`.
- [../.agents/outline-method.md](../.agents/outline-method.md), [../.agents/testing.md](../.agents/testing.md).

**Verify**

```bash
uv run pytest orchestrator/tests/test_schemas.py
go test -race ./hunterd/... ./enforcerd/... ./operator/...
```

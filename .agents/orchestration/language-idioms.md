# Language idioms

## Go

- Keep packages small and names plain: `cursor`, `store`, `probes`, `enforcerd`, `nft`, and `rpz`.
- Accept `context.Context` at network, process, and daemon boundaries. Do not hide goroutine lifetimes.
- Return wrapped errors with `%w`; use sentinel errors for operator-correctable policy failures.
- Validate before execution. Use `exec.CommandContext` argument arrays, never shell interpolation.
- JSON contracts live in `schemas/`; add a version instead of silently changing v1 semantics.
- Keep permutation and checkpoint state O(1). Tests must prove bijection, resume, novelty, signature, and TTL invariants.
- Run `gofmt`, `go vet ./...`, and `go test -race ./...` before handing off.

## Python

- Support Python 3.11–3.13. Use typed `TypedDict` graph state and Pydantic event validation.
- Keep LangGraph nodes deterministic at policy boundaries; external intel is an injectable adapter.
- Use native `interrupt()` for broad actions and a durable checkpointer outside tests.
- Load thresholds from `orchestrator/policy.yaml`; do not duplicate policy constants in agents.
- Avoid untrusted pickle/checkpoint formats and unsafe YAML loaders.
- Write tests before implementation. Run `uv run ruff check orchestrator` and `uv run pytest`.

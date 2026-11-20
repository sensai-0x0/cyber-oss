# Claude as the extraction model: Messages API, not the Agent SDK

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-0.md](Phase-0.md), [Phase-3.md](Phase-3.md), [../../db/CONFIG.md](../../db/CONFIG.md) · **Updated:** 2026-11-20

Graphiti calls an LLM once per episode to pull entities and relations out of the episode body. This file records which Claude surface to use and what that implies. ADR 0006 (planned) makes it binding.

## Decision

Use the **Messages API** through the official `anthropic` Python SDK, wrapped as a Graphiti LLM client. Do not use the Claude Agent SDK. Do not use Managed Agents.

| Option | Fit | Why |
|---|---|---|
| **Messages API, one call per episode** | Chosen | The job is a single structured extraction over input the app has already sanitised. No tools, no loop, no file access. The call is stateless, so it is easy to retry, batch, cache and cap |
| Messages API with a tool loop | No | Nothing for the model to call |
| Claude Agent SDK | No | It is Claude Code as a library: built-in Bash, file and web tools, plus a Node-based runtime in the container. Giving a tool-using agent attacker-influenced target data is the wrong trust shape for a system whose invariant is that no untrusted text steers an action. It also adds image weight and a second process to supervise |
| Managed Agents | No | Anthropic hosts the loop and a sandbox. There is nothing to host or schedule here, and the session history it keeps is more retained data than the ADR should accept |

An agent loop could suit a later analyst-facing "investigate this campaign" feature. That is a separate decision with its own scope gate, and it would not be on the ingestion path.

## What this implies for the code

1. **Graphiti needs an LLM client and an embedder, and Anthropic provides only the first.** The Messages API has no embeddings endpoint. Phase 0 chooses an embedder (a hosted embedding provider or a local model) and records it. Without one, `add_episode` cannot run.
2. **Check the stock Anthropic client in Graphiti against the current models.** Recent Claude models reject some parameters older client code sends: non-default sampling values (`temperature`, `top_p`, `top_k`), assistant prefill, and forced `tool_choice` (`any` or a named tool) all return HTTP 400 on the current Sonnet and Opus models. `db/CONFIG.md` sets `LLM_TEMPERATURE=0.0`, which is itself a request that these models can reject. If the stock client sends any of them, write a small subclass of Graphiti's LLM client instead of passing configuration. Phase 3 has a test that fails on a 400 from a recorded request body.
3. **Structured output through `output_config.format`, not a forced tool call.** Give the client the Pydantic schema Graphiti supplies and read the parsed result. Check `stop_reason` before reading content: `max_tokens` means a truncated object (do not parse it), and `refusal` means no result.
4. **Model.** Start with `claude-sonnet-5-5` for extraction quality, and measure `claude-haiku-5-5` against a labelled sample. Haiku is cheaper, but its refusals have no server-side fallback, so the adapter must handle `refusal` itself. The ADR records the model and the sample result. Do not append date suffixes to model IDs. Run `/claude-api migrate` when a model is retired.
5. **Cost controls.**
   - Cache the fixed ontology and instruction prefix with `cache_control`. A prefix shorter than the model's minimum cacheable length silently does not cache, so verify `cache_read_input_tokens` is non-zero in the smoke test.
   - Use the **Message Batches API** for backfill and replay (Phase 4). It is half price and asynchronous, which suits a replay that is not on the decision path. Results arrive in any order, so key by `custom_id`, set to the ledger `record_hash`.
   - Count tokens with `count_tokens` before a large replay and stop if it exceeds the budget the operator set.
6. **Errors.** Use the SDK's typed exceptions, most specific first. The SDK already retries 429, 5xx and connection errors with backoff, so the adapter must not stack its own unbounded retry on top. Log `_request_id` with every failure.
7. **Credentials.** `ANTHROPIC_API_KEY` comes from the environment into the app container only. It never enters the image, Compose file, FalkorDB container, ledger or logs. In a cloud session, ask for it through the session's credential request, not chat.
8. **Data handling.** Every episode leaves the lab as an API request. Phase 0 must confirm the organisation's data-retention terms accept that, and the allowlist in Phase 4 is what limits what is sent. Zero-retention eligibility differs by model; check before choosing one.

## Failure modes

| ID | Failure | Resolution |
|---|---|---|
| GF-C.1 | A parameter the model rejects is sent (sampling, prefill, forced tool) | Custom client, recorded-request test, and no sampling parameters in configuration |
| GF-C.2 | Rate limit or overload | SDK retry with a bounded count, then drop-and-count with the key kept for replay. Concurrency is capped by the Phase 3 semaphore |
| GF-C.3 | `refusal` or `max_tokens` stop reason | Never parse. Drop and count, with the reason, and log the record key. Never retry a refusal with a looser prompt |
| GF-C.4 | Invalid API key or no credit | `AuthenticationError` or `PermissionDeniedError` disables the store for the process and raises one alert. It does not retry per episode |
| GF-C.5 | Model retired or renamed | `NotFoundError` is handled like GF-C.4. The model ID lives in one configuration key |
| GF-C.6 | Cost runaway from a burst or a replay | Per-interval budget in the adapter, `count_tokens` gate on replay, Batches for backfill |
| GF-C.7 | Prompt injection through a field that slipped the allowlist | Output is schema-constrained and only written to the graph, never executed or routed. The graph does not feed decisions until Phase 6 passes its own gate |
| GF-C.8 | Extraction output differs between runs | Accepted. The graph is lossy, derived and rebuildable from the ledger |
| GF-C.9 | No embedder configured | Phase 3 `graphcheck` fails before any episode is sent |
| GF-C.10 | The API is unreachable from the lab | Adapter times out and drops. Hunting is unaffected |

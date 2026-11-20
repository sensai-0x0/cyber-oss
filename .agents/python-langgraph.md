# Python and LangGraph rules

**Status:** authoritative · **Derived from:** `orchestrator/`, [invariants.md](orchestration/invariants.md), [../spec/STATE.md](../spec/STATE.md) · **Updated:** 2026-11-21

Universal rules in [../CLAUDE.md](../CLAUDE.md). Go rules in [go.md](go.md).

Scope: the Python control plane in `orchestrator/`. Python 3.11-3.13, pytest, ruff (line length 100, rules `E,F,I,UP,B`). Python routes and proposes. Go decides and enforces.

## Module layout

One responsibility per module. A node function lives beside the logic it runs, never in the file that wires the graph.

| Module | Owns | Must not contain |
|---|---|---|
| `orchestrator/state.py` | `DeltaEvent`, `Cluster`, `IntelScore`, `Policy`, `GraphState` | Node logic, I/O |
| `orchestrator/cluster.py` | `cluster_event` (stable TLS, favicon, certificate indicators; increments `cycles`) | Network calls |
| `orchestrator/score.py` | `score_threat_intel`, `route_after_intel` | Policy decisions, thresholds |
| `orchestrator/provenance.py` | Evidence classes (OBSERVED, DERIVED, ASSERTED), the pinned badlist check | Scoring weights, I/O beyond the pinned file |
| `orchestrator/ledger.py` | Hash-chained append-only ledger, `record` node factory, `verify` CLI | Reads during triage |
| `orchestrator/policy.py` | `load_policy`, `synthesize_policy`, `approve_policy` | Scoring, clustering |
| `orchestrator/graph.py` | `build_graph(checkpointer=None)`: nodes, edges, compile | Business logic |
| `orchestrator/policy.yaml` | Every threshold and TTL bound | Code |

- `graph.py` stays wiring-only so topology is readable in one screen and nodes are unit-testable without compiling a graph.
- A new node goes in the module for its concern, or a new sibling module. Register it in `build_graph` with `add_node` and an explicit edge. Do not grow `policy.py` into a catch-all.
- Keep modules under 400 lines. Do not create a separate agent package or a mission-file control surface; [ADR 0003](../spec/DECISIONS/0003-architecture-authority.md) superseded that design.

## Typed state

- `GraphState` is a `TypedDict` with `total=False`. Every key is optional, so a reader must not assume a key is present before the node that writes it has run. Use `state.get("cycles", 0)`. Direct indexing (`state["intel"]`) is acceptable only where graph topology guarantees the upstream node ran.
- Add a state key only with its writing node and a reader, and update [../spec/STATE.md](../spec/STATE.md) in the same change.
- Pydantic appears only at the trust boundary: `DeltaEvent` validates what Go publishes. Inside the graph, state stays plain `TypedDict` and dicts so checkpoints serialize cleanly.
- Nodes re-validate with `DeltaEvent.model_validate(state["event"])`. Do not trust that an earlier step already did.
- `extra="forbid"` is mandatory on boundary models. Rejecting an unknown field at the edge surfaces schema drift as a loud error at the source. Tolerating it lets a mismatched producer run silently against stale assumptions. `schemas/delta_event.v1.json` also sets `additionalProperties: false`; the two must agree.
- Free-form data belongs only in `attributes`, which is unsigned. Treat every value there as untrusted (see Trust boundary).
- Closed sets use `Literal` (`kind`, `source`, `schema_version`, `Policy.action`, `Policy.kind`). Never `str`. `unchanged` is never valid on this boundary.
- Fingerprints use `Annotated[str, Field(pattern=r"^[0-9a-f]{64}$")]`: lowercase SHA-256 hex, same regex as the JSON schema. Do not loosen to accept uppercase or truncated digests.
- Changing a required field or its meaning requires a new schema version and a recorded decision, not an edit to `v1`.

## Node contract

| Rule | Why |
|---|---|
| Signature `(state: GraphState) -> dict[str, Any]` | LangGraph merges the returned keys |
| Return only the keys you changed | Returning whole state clobbers concurrent writers and bloats checkpoints |
| Deterministic at policy boundaries: same state and same `policy.yaml` give the same output | Routing and clamping stay assertable |
| Return new objects; never mutate `state` or its nested values | House rule; mutation hides side effects and breaks replay |
| External intel enters through an injectable adapter passed to the node factory | A module-level client cannot be mocked and leaks network into import |

Today `score_threat_intel` computes confidence from `attributes` flags only. When live intel is added, wrap it in an adapter protocol and bind it with a factory (`make_intel_node(adapter)`), then register the result in `build_graph`. Tests pass a fake adapter. Nothing performs I/O at import time.

### Anti-pattern: mutation in `approve_policy`

`approve_policy` currently takes `policy = state["policy"]`, then writes `policy["approved"] = ...` and calls `policy.update(...)`. This mutates the checkpointed state object in place. It happens to work because the node returns the same dict, but it violates the house rule, aliases checkpoint history, and makes the interrupt-resume replay harder to reason about. Correct shape:

```python
updated: Policy = {**state["policy"], "approved": approved}
if not approved:
    updated = {**updated, "action": "observe", "ttl_seconds": 0, "reason": "operator rejected mitigation"}
return {"policy": updated}
```

Build a new `Policy`, never edit the one read from state. New code must not copy the existing pattern.

## Routing and bounded cycles

- Conditional edges use an explicit destination map: `add_conditional_edges("intel", route_after_intel, {"cluster": "cluster", "mitigate": "mitigate"})`. The router returns a key of that map, not a node name guessed at runtime.
- Routers are pure functions of state plus loaded policy. They do not call out and do not write.
- Every cycle has a hard bound read from configuration. The refinement loop is `cluster` to `intel` back to `cluster`, counted by `cycles`, which `cluster_event` increments.
- At the bound, force a terminal decision (`mitigate`, which then yields `observe` or an approval request). Never loop, never raise to escape, never raise the bound at runtime.
- The ambiguous-confidence window and the cycle limit are policy, so they belong in `policy.yaml` (see below).
- A router must be total: every input maps to a destination.

## Policy and configuration

- `orchestrator/policy.yaml` is the single source of thresholds, TTL bounds and cycle limits. Cite keys by name (`policy.yaml: mitigation_confidence`), never a pasted number, in docs and comments.
- `load_policy` is `lru_cache(maxsize=1)`: read once per process, validated once. Tests that need different values must call `load_policy.cache_clear()` or patch the loader. Do not mutate the returned dict; it is shared.
- Check `version` on load and raise on mismatch. A new policy schema bumps the version and the check together.
- Load with `yaml.safe_load` only, then validate keys and types before use.
- Never copy a constant into a node. A literal that mirrors a policy value is drift waiting to happen.
- A key declared in `policy.yaml` that no code reads is a defect, not documentation. Either wire it or delete it in the same change. Known violations to fix, not imitate:
  - `max_refinement_cycles` is never read; `route_after_intel` hardcodes the cycle limit.
  - `route_after_intel` hardcodes the ambiguous-confidence window boundaries instead of deriving them from policy keys.
  - `score_threat_intel` embeds its weights and caps as literals.

## Human-in-the-loop

- Broad actions (CIDR, wildcard) call native `langgraph.types.interrupt()`; never poll, sleep or write a custom pause flag.
- The interrupt payload carries everything the human needs to decide: `type`, `action_id`, `target`, `confidence`, and the TTL ceiling (`max_ttl_seconds`). Add fields (rationale, scope) before adding a second interrupt. The payload is JSON-serializable and holds no secrets.
- On resume the node re-runs from its top, so code before `interrupt()` must be idempotent and side-effect free.
- Durable checkpointer required outside tests (SQLite or Postgres saver). `MemorySaver` loses a pending approval on restart. `build_graph(checkpointer=None)` compiles without persistence, so production wiring must always pass a durable one.
- Thread keying: one `thread_id` per event or action, in `config={"configurable": {"thread_id": ...}}`. Resuming with a different `thread_id` starts a new run and never delivers the decision. Never share a thread between unrelated actions.
- Resume with `Command(resume=...)` and the same config:

| Decision | Resume value | Result |
|---|---|---|
| Approve | `{"approved": True}` | `approved` true, TTL defaults from policy |
| Edit | `{"approved": True, "ttl_seconds": n}` | TTL clamped to the policy min and max |
| Reject | `{"approved": False}` | `action` becomes `observe`, `ttl_seconds` 0 |

- Missing or malformed resume values mean reject. Fail closed: absent `approved` is false. Validate `ttl_seconds` type before `int()` rather than letting a hostile value raise mid-node.

## Trust boundary (I7)

- Python checks are a routing convenience, never the guard. Every bound (TTL ceiling, confidence floor, scope, CIDR vs single IP) is enforced again in Go regardless of what Python decided. See [invariants.md](orchestration/invariants.md) I7, I8, I11, I14.
- Python mirrors Go bounds so a request is routed and presented correctly and fails early. It never widens scope, raises a TTL past the Go ceiling, or relaxes a check on the theory that Go will catch it, or the reverse.
- Every LLM-supplied or `attributes`-supplied value is hostile: validate type, range and format before use (`prefix_length`, `ttl_seconds`, `shared_fingerprints`). Clamp, then still expect Go to reject.
- `broad` is derived from `attributes["prefix_length"]`, which is unsigned input. Mis-stating it must not be able to skip the human gate in Go.
- A rejection is a review result, not a deployable action. It carries no signature and never reaches enforcement ([../spec/STATE.md](../spec/STATE.md)).

## Errors

- Handle explicitly at every level. Never `except Exception: pass`; never swallow.
- Fail fast on boundary violations (`ValidationError`, policy version mismatch). Do not substitute defaults for invalid input.
- When a tool or validation error comes back, feed it into the next reasoning step. Never retry the identical call with identical arguments.
- On a rate or bound rejection, back off or escalate. Never shrink scope or reshape the request to slip under a limit.
- Log detail server-side (stderr; stdout stays clean). Surface a short, safe message to operators.

## Security

- `yaml.safe_load` only. Never `yaml.load` or an unsafe Loader.
- No `pickle`, `marshal`, or any checkpoint serializer that executes code on load. Use LangGraph's default JSON-compatible serde.
- No secrets, credentials, client names or raw target lists in logs, state, interrupt payloads or test fixtures. Test targets use reserved ranges (`192.0.2.0/24`, `198.51.100.0/24`).
- No shell, file write or network from nodes (I9); only the adapter seam, and only to approved endpoints.

## Testing

Write the test first and watch it fail. Mock the LLM and any intel adapter; never call a live model or network. Use fixtures, loopback only. Assert routing and clamping deterministically.

| File | Proves |
|---|---|
| `orchestrator/tests/test_graph.py` | End to end with `MemorySaver`: low confidence yields `observe`; narrow action does not interrupt; broad action interrupts and resumes (approve, edit, reject); refinement loop stays bounded |
| `orchestrator/tests/test_state.py` | `DeltaEvent` round trip, rejects non-emitted kinds, requires lowercase SHA-256 |
| `orchestrator/tests/test_schemas.py` | `DeltaEvent` dump validates against `schemas/delta_event.v1.json`; `schemas/mitigation_action.v1.json` requires approval, TTL, expiry, signature |
| `orchestrator/tests/test_lab.py` | `deploy/compose.yaml` binds loopback only; `lab/fake_c2.py` has no execution behavior |

- Add a test per new state key, router branch, and policy clamp edge (below min, above max, missing).
- `MemorySaver` is acceptable in tests only.
- Tests that depend on policy values read them via `load_policy()`; do not paste numbers.

## Before handing off

```bash
uv run ruff check orchestrator
uv run ruff format --check orchestrator
uv run pytest
uv run python .github/scripts/validate_outlines.py .
```

## Red flags in review

| Pattern | Why it is wrong here | Replacement |
|---|---|---|
| `policy["approved"] = x` or `policy.update(...)` on a value from `state` | Mutates checkpointed state in place; violates the immutability house rule | Build `{**state["policy"], ...}` and return only `{"policy": new}` |
| A literal threshold, weight or TTL in a node or router | Drifts from `policy.yaml`; two sources of truth | Read the key from `load_policy()` |
| A cycle with no config-driven bound, or a bound that only raises | Loops or crashes instead of deciding | Read the limit from policy; at the bound route to a terminal node |
| Module-level LLM, HTTP or intel client | Untestable, does I/O at import, hides trust boundary | Inject an adapter through a node factory |
| `yaml.load`, `pickle`, custom unsafe serde | Arbitrary code execution from config or checkpoint | `yaml.safe_load`; default JSON serde |
| "Python already checks the TTL/scope, so Go can skip it" | Violates I7; Python is never the guard | Enforce in Go; keep Python as routing and UX |
| `MemorySaver` or `checkpointer=None` in non-test wiring | Pending approval is lost on restart | Pass a durable SQLite or Postgres saver |
| Returning the whole state from a node | Overwrites other writers; bloats checkpoints | Return only changed keys |
| `extra="allow"` or `str` for a closed set | Silent schema drift | `extra="forbid"` and `Literal[...]` |
| A `policy.yaml` key nothing reads | Documentation that lies | Wire it into the code that should use it, or delete it |
| `except Exception: pass`, or retrying the same failed call | Swallowed or repeated errors | Handle, log, feed the error back into reasoning |

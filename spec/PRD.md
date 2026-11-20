Sentinel-Lite: Triaged Product Requirements Document (PRD)

Triage Philosophy & Execution Order

Work blocks are strictly isolated by package boundary and triaged from P0 (Core State & Policy) outward to P3 (Mitigation Enforcement & Chaos Hardening). Each block compiles and passes its own deterministic test suite using mocked interfaces before integration.

Block 0 (P0 · Core): Policy Contract, BadgerDB Store & Feistel CIDR Engine

Priority: P0 — Critical Path (Must Ship First)

Owned Files: mission.yaml, pkg/cidr/feistel.go, pkg/kv/store.go

Dependencies: github.com/dgraph-io/badger/v4 (dgraph-io/badger) — zero network or LLM dependencies.

Core Requirements:

Policy Loader & Scope Validator: Parse mission.yaml (targets, rate_limit_pps, watchlist_favicons, autonomy) and expose a pure Allows(ip netip.Addr) bool function.

O(1) Feistel CIDR Permuter: Implement 4-round balanced Feistel bijection over [0, 2^w - 1] so any /16 or /24 is iterated in pseudorandom order with zero duplicates and O(1) RAM.

BadgerDB Persistence & Delta Gate: Persist cidr:cursor:<cidr>, seen:<ip>:<port> (7-day TTL), cluster:<hash> (30-day TTL), and mitigate:rule:<id> (configurable TTL). CheckAndUpdateDelta MUST return mutated=false when (JARM, CertSHA256, FaviconMMH3) is unchanged.

Acceptance Criteria:

go test -race ./pkg/cidr/... ./pkg/kv/... passes 100%.

Property test proves FeistelPermuter visits all 65,536 addresses in a /16 with 0 collisions and resumes cleanly from a simulated mid-scan crash.

Block 1 (P1 · Sensor Core): Unified tlsx + httpx Probe Runner & MCP Server

Priority: P1 — High (Depends on Block 0 Interfaces)

Owned Files: pkg/probe/runner.go, cmd/hunter/main.go

Dependencies: projectdiscovery/tlsx, projectdiscovery/httpx, pkg/cidr, pkg/kv.

Core Requirements:

In-Process Probe Runner: Wrap tlsx (JARM, SAN/CN, CertSHA256) and httpx (HTTP status, favicon_mmh3, -kb page type) behind probe.Runner, gated by mission.yaml allowlist and a token-bucket rate limiter (rate_limit_pps).

MCP Stdio Server (cmd/hunter): Expose scan_cidr_delta, probe_http_favicon, and query_kv_history over JSON-RPC 2.0 stdin/stdout.

CDN Noise Guard: Suppress is_cdn: true events from leaving the sensor unless favicon_mmh3 matches mission.yaml:watchlist_favicons.

Acceptance Criteria:

go test -race ./pkg/probe/... ./cmd/hunter/... passes against a local httptest.NewTLSServer mock target.

Pass 1 emits DeltaEvents for mock hosts; Pass 2 on unchanged hosts emits 0 events.

Out-of-scope CIDR/IP inputs return MCP isError: true (ERR_OUT_OF_SCOPE) with zero packets sent.

Block 2 (P1 · Agent Core): LangGraph Investigator & Human Interrupt Gate

Priority: P1 — High (Can Develop in Parallel Against Mocked MCP)

Owned Files: agent/graph.py, agent/tools.py, agent/tests/test_autonomy.py

Dependencies: langchain-ai/langgraph, MCP stdio client (cmd/hunter binary or mock JSONL fixture).

Core Requirements:

Typed Cyclic StateGraph: Implement HunterState (plan_or_reason -> execute_safe_tool / human_gate).

Human-Directed Autonomy Enforcement:

Tier 1 tools (scan_cidr_delta, probe_http_favicon, query_kv_history) run autonomously.

Tier 3 actions (block_cidr, modify_rpz_wildcard, pivot_new_asn) MUST halt at langgraph.types.interrupt().

Campaign Clustering: Agent groups related DeltaEvents by shared jarm or favicon_mmh3 in investigated_clusters before proposing mitigations.

Acceptance Criteria:

pytest agent/tests/test_autonomy.py passes offline using deterministic DeltaEvent JSONL fixtures and a scripted/mocked LLM.

Tier 3 proposals pause execution cleanly, serialize state via checkpointer, and resume only on approve / edit / reject.

Block 3 (P2 · Mitigation Plane): TTL-Bounded Enforcer & Automatic Reaper

Priority: P2 — Medium (Depends on Block 0 KV & Block 1 MCP)

Owned Files: pkg/mitigate/enforcer.go (plus apply_ttl_rule handler in cmd/hunter/main.go)

Dependencies: pkg/kv/store.go, Linux nftables / DNS RPZ adapter interface.

Core Requirements:

Strict Tier 2 Bounds: apply_ttl_rule MUST reject ttl_minutes <= 0, ttl_minutes > max_ttl_minutes (ERR_INVALID_TTL), or confidence < min_confidence.

Deterministic TTL Reaper: Background goroutine polls kv.Store.ExpiredMitigations(now) every 15s and removes expired rules from nftables/RPZ automatically.

Acceptance Criteria:

Isolated network-namespace (netns) integration test verifies rule creation, traffic drop, and automatic removal when the BadgerDB TTL expires.

Block 4 (P3 · Hardening): End-to-End Replay & Chaos Verification

Priority: P3 — Verification & Release Gate

Owned Files: tests/e2e/

Core Requirements:

Full Loop Smoke Test: Run agent/graph.py -> cmd/hunter against a multi-host local lab (httptest servers mimicking C2 JARM + phishing favicon) to verify end-to-end detection, Tier 2 single-IP TTL block, and Tier 3 /24 interrupt().

Crash & Backpressure Resilience: Kill cmd/hunter mid-sweep with SIGKILL, restart, and verify Feistel cursor resumes without re-emitting prior deltas.

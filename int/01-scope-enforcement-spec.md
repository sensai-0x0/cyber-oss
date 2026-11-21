# Scope Enforcement Package — Technical Spec

**Status:** proposed · **Derived from:** `spec/SCOPE.md`, `.agents/orchestration/invariants.md` (I1–I5), `spec/DECISIONS/0003-architecture-authority.md` (consequence 4), `docs/05-safety-and-trust-model.md`, `docs/testing.md` · **Updated:** 2026-11-21  
**Audience:** implementer / reviewer · **Blocks:** any active probing outside `lab/`  
**Location (planned):** `hunterd/scope/`

## 1. Problem

ADR 0003 consequence 4 records a defect, not a decision: **authorized scope has no implementation and no home.** Invariants I1–I5 require that every probe target be checked against an authorized allowlist in Go *before* dialing, that out-of-scope targets return an error with no packet sent, and that an empty allowlist refuses to start rather than defaulting to allow-all. None of that exists today:

- `hunterd/probes/http_runner.go` passes its `target` straight to `httpx` and calls `RunEnumeration()`; the only guard is `ctx.Err()`.
- `hunterd/probes/tls_runner.go` calls `service.Connect(host, ip, port)` directly.
- `hunterd/cmd/hunterd/main.go` has no configuration loader; `validateEvent` checks event *shape* only and never asks whether the target was authorized.
- No Go file in the repository parses YAML or holds an allowlist.

Until this lands, `hunterd` probes whatever target it is given. The headline trust claim — that review of consequential action is a technical property — holds for mitigation but **not** for probing. E03 (the hard gate for exactly this) has never run because it targets a superseded path.

## 2. Dual-scope model: owned scope + research scope

Sentinel Hunter's foundation for a private intelligence / defense contractor requires **two categories of authorized target**, each with different rules:

| Category | Meaning | Can be probed | Can be mitigated against | Default action |
|---|---|---|---|---|
| **Owned scope** | The client's own CIDRs/domains (their address space) | Yes | Yes (signed, TTL-bounded, reversible) | Observe or mitigate per policy |
| **Research scope** | Third parties / other parties for research purposes | Yes | **No** — observe-only against the researched party | Observe only; research intel may inform defensive action on owned scope |

The critical rule: **research scope is observe-only.** A DeltaEvent emitted from a research target can never directly produce a MitigationAction against that research target. Research intel can only lead to defensive action on the client's *owned* scope, and only through the normal signed, human-gated path. This is the hard line between intelligence collection and offensive action.

## 3. Requirements

### 3.1 Base invariants (I1–I5) apply to both scopes

Every probe target — owned or research — is checked against the applicable allowlist **in Go, before dialing**. Out-of-scope returns an error; no packet is sent.

| ID | Requirement | Applies to |
|---|---|---|
| I1 | Every probe target checked against authorized scope in Go before dialing; out-of-scope → error, no packet sent. | Owned + research |
| I2 | Hostnames resolved; **every** resolved IP checked; one out-of-scope IP among several rejects the target; redirects never followed off-scope. | Owned + research |
| I3 | Empty allowlist refuses to start; defaults to deny-all, never allow-all. | Owned + research (each independently) |
| I6 | Rate limit via shared token bucket across both runners. | Owned + research |
| I7–I8 | Treat every caller/LLM-supplied argument as hostile input; validate type, range, format on the enforcing side. | Owned + research |

### 3.2 Research-specific invariants (R1–R4)

| ID | Requirement | Rationale |
|---|---|---|
| R1 | A DeltaEvent from a research target carries `scope_category: research` and can never produce a MitigationAction whose target is in research scope. | Keeps collection separate from offensive action. |
| R2 | Research intel may inform defensive action on owned scope only, and only through the signed, TTL-bounded, human-gated operator path. | Research is input to defense, not a license to strike back. |
| R3 | No credential collection, no exploitation, no execution of discovered content, no active attack against research targets. `lab/phish_panel.py` rejects all POSTs; `lab/fake_c2.py` has no beacon or command execution — same standard applies to real research targets. | Consistent with spec/SCOPE.md and the lab's structural limits. |
| R4 | A heuristic or scored result never equals attribution. Research output is an observation with rationale, not an attribution verdict. | Prevents converting a score into action against a third party. |

### 3.3 Eval E03 pass bar (unchanged, extended)

**Zero off-scope packets observed on a sink network** — not "mostly zero." Test cases: adjacent ranges, IPv6/IPv4-mapped forms, decimal/octal IP encodings, redirects off-scope, DNS rebinding. Applies equally to owned and research targets.

## 4. Package design: `hunterd/scope/`

### 4.1 ScopeCategory

```go
package scope

type ScopeCategory string

const (
    ScopeOwned    ScopeCategory = "owned"     // client's own infrastructure
    ScopeResearch ScopeCategory = "research"  // third parties for research
)
```

### 4.2 Allowlist (per category)

```go
type Allowlist struct {
    Category ScopeCategory   // owned or research
    Owner    string          // authorization owner (from hunt record)
    Evidence string          // evidence reference for the authorization
    CIDRs    []netip.Prefix  // authorized networks (canonical form)
    Hosts    []string        // authorized domain names (lower-cased)
    Ports    []uint16        // authorized ports
    Exclusions []netip.Prefix // explicitly removed from scope
    TimeWindow *TimeWindow   // [start, end); nil means unbounded (discouraged)
    RateLimitPPS float64       // max request rate per second
    MitigationAllowed bool    // true for owned, false for research (enforced by orchestrator/enforcerd)
}
```

`Allowlist` is immutable after construction (safe for concurrent reads by probe runners).

### 4.3 Registry (multiple scopes per process)

A process may hold multiple allowlists — one owned scope per client, plus zero or more research scopes:

```go
type Registry struct {
    owned   map[string]*Allowlist   // keyed by client_id
    research []*Allowlist            // shared research scopes
}

// FindOwned(clientID) returns the client's owned allowlist or nil.
// MatchAny(hostOrIP) returns (category, allowlist, ok) if the target matches
// any owned or research scope. Used for pre-dial checks.
```

### 4.4 Public API

```go
// New builds an Allowlist from raw configuration. Returns ErrEmptyScope if the
// allowlist would be empty (defaults to deny-all, never allow-all).
func New(cfg Config) (*Allowlist, error)

// Allows reports whether ip is authorized within this scope. Exclusion checked
// before inclusion. Constant-time-ish lookup over the prefix set.
func (a *Allowlist) Allows(ip netip.Addr) bool

// AllowsHost resolves host (via the injected resolver) and returns true only if
// every resolved IP is authorized and the host itself is not excluded.
// Returns (false, nil) on resolution failure — never dial on error.
func (a *Allowlist) AllowsHost(host string) (bool, error)

// InTimeWindow reports whether now is within [start, end).
func (a *Allowlist) InTimeWindow(now time.Time) bool

// Category returns the scope category (owned or research).
func (a *Allowlist) Category() ScopeCategory

// MitigationAllowed reports whether mitigation actions may target this scope.
func (a *Allowlist) MitigationAllowed() bool

type Config struct {
    Source           string
    Owner            string
    Evidence         string
    CIDRs            []string
    Hosts            []string
    Ports            []uint16
    Exclusions       []string
    TimeWindow       *TimeWindowConfig
    RateLimitPPS     float64
    Category         ScopeCategory   // "owned" or "research"
    MitigationAllowed bool            // derived from category; research → false
}
```

### 4.5 Validation rules (enforced in `New`, before any dial)

- **Non-empty:** at least one CIDR or host must remain after exclusions; otherwise `ErrEmptyScope`.
- **Canonicalization:** IP addresses to canonical form (netip); hostnames lower-cased; ranges masked to network address; ports validated 1–65535.
- **IPv4-mapped IPv6:** `::ffff:a.b.c.d` derived to its IPv4 form before comparison (so `enforcerd.Breadth.Derive` and `scope.Allows` agree — see `spec/STATE.md`).
- **Decimal/octal IP encodings:** rejected or normalized at parse; never passed to a dialer unchecked.
- **Redirects:** every resolved IP must be in scope; a redirect target is checked independently and must itself be in scope before following.
- **Time window:** `Allows*` returns false outside `[start, end)`; startup refuses if the window is already closed.
- **Rate limit:** `RateLimitPPS > 0` required (delegated to the shared token bucket; stored for audit).
- **Research scope forces `MitigationAllowed = false`** regardless of config input; a research allowlist that requests mitigation is rejected with `ErrResearchMitigationForbidden`.

## 5. Configuration shape

Scope files live per client (owned) and per research program (research). YAML, parsed by `hunterd/scope`:

### Owned scope (client's own space — can be hunted and mitigated)

```yaml
client_id: acme-corp
category: owned
owner: "Acme Corp Security"
evidence: "ENG-2026-11-21 authorization record"
time_window:
  start: "2026-12-01T00:00:00Z"
  end: "2026-12-31T23:59:59Z"
rate_limit_pps: 10
cidrs:
  - "10.0.0.0/8"
  - "2001:db8::/32"
hosts:
  - "example.com"
  - "assets.example.net"
exclusions:
  - "10.0.53.0/24"
  - "legacy.example.com"
ports: [443, 8443]
```

### Research scope (third parties — observe only)

```yaml
program_id: threat-landscape-q4-2026
category: research
owner: "Sentinel Hunter Research Lead"
evidence: "RES-2026-11-21 research program charter"
time_window:
  start: "2026-12-01T00:00:00Z"
  end: "2026-12-31T23:59:59Z"
rate_limit_pps: 2
cidrs:
  - "203.0.113.0/24"          # example research network
hosts:
  - "research-target.example.net"
exclusions: []
ports: [443]
mitigation_allowed: false     # enforced; cannot be true for research
```

## 6. Integration points

### 6.1 `hunterd/probes/tls_runner.go`

Before `service.Connect(host, ip, port)`:

```go
category, allowlist, ok := s.registry.MatchAny(host)
if !ok || err != nil {
    return nil, scopeerr.ErrOutOfScope // no packet sent
}
if !allowlist.InTimeWindow(now) {
    return nil, scopeerr.ErrClosedWindow
}
// resolve and check every IP
ips, err := resolver.LookupIP(host)
for _, ip := range ips {
    if !allowlist.Allows(ip) {
        return nil, scopeerr.ErrOutOfScope
    }
}
// Tag the eventual DeltaEvent with scope_category
observation.ScopeCategory = string(category)
```

Redirects: the redirect target host/IP must pass `MatchAny`/`Allows` before the second dial, and must be in the same scope category (a redirect from owned to research, or vice versa, is a new scope check and must pass independently).

### 6.2 `hunterd/probes/http_runner.go`

Before `httpx.RunEnumeration(target)`:

```go
category, allowlist, ok := s.registry.MatchAny(target)
if !ok || err != nil {
    return nil, scopeerr.ErrOutOfScope
}
// HTTP 3xx redirects to out-of-scope hosts are not followed;
// same-origin-only by default unless the redirect target passes MatchAny.
```

### 6.3 `hunterd/cmd/hunterd/main.go`

- Add a configuration loader that reads owned + research scope YAML files and builds the `scope.Registry` via `scope.New(cfg)` for each.
- `scope.New` returning `ErrEmptyScope` → process refuses to start (non-zero exit), never defaults to allow-all.
- `validateEvent` continues to check event *shape* only; scope is enforced at the runner dial site. Events carry `scope_category` from the registry match.
- No scope check in `validateEvent`: intentional separation of shape vs. authorization.

### 6.4 Orchestrator boundary (research → observe only)

`orchestrator/policy.py` and `orchestrator/graph.py` must treat `scope_category: research` events as **observe-only**:

- A research-category DeltaEvent can synthesize an `observe` policy but never a `mitigate` policy whose target is in research scope.
- Research intel may feed into `score.py` signals (e.g., "this research infrastructure shares a fingerprint with infrastructure targeting our owned scope") but the resulting mitigation targets only owned scope.
- `operator.Approve`/`Reject` and `enforcerd` enforce: a MitigationAction whose target is in research scope is rejected at the enforcement boundary (`enforcerd.Enforcer.Enforce` checks `allowlist.MitigationAllowed()`).

## 7. Errors

```go
var (
    ErrOutOfScope              = errors.New("target out of authorized scope")
    ErrEmptyScope              = errors.New("authorized scope is empty; refusing to start")
    ErrClosedWindow            = errors.New("outside authorized time window")
    ErrInvalidPort             = errors.New("port outside 1-65535")
    ErrResearchMitigationForbidden = errors.New("research scope cannot be a mitigation target")
)
```

Runners return `ErrOutOfScope` as a hard error (no retry, no fallback to a different target). The caller logs the refusal with target + scope source + category for audit.

## 8. Tests (test-first, before implementation)

Per `docs/testing.md` and E03:

1. **Adjacent ranges (owned):** a /24 just outside an authorized /24 yields zero packets.
2. **IPv6 / IPv4-mapped:** `::ffff:10.0.0.5` resolves to `10.0.0.5` and is checked as IPv4.
3. **Decimal/octal IP encodings:** `0177.0.0.1`, `0x7f000001`, `2130706433` rejected/normalized, never dialed.
4. **Redirects off-scope:** an in-scope host issuing a 302 to an out-of-scope host results in zero packets to the redirect target.
5. **DNS rebinding:** a hostname resolving first to in-scope then out-of-scope (or multiple A records, one out-of-scope) is rejected entirely.
6. **Empty allowlist:** `scope.New` with no CIDRs/hosts returns `ErrEmptyScope`; `main.go` exits non-zero.
7. **Time window:** probes before `start` or after `end` return `ErrClosedWindow`.
8. **Every resolved IP:** a host with A records `[in-scope, out-of-scope]` returns false.
9. **Research observe-only:** a research-category DeltaEvent through the real graph produces `observe` only, never `mitigate` against the research target (across three runs).
10. **Research → owned defense:** research intel indicating threat to owned scope may produce a mitigate policy against owned scope, gated on the normal signed operator approval.
11. **Research mitigation rejected:** `enforcerd.Enforcer.Enforce` rejects a MitigationAction targeting a research scope even with a valid signature.

All eleven tests must pass against a sink network observing zero off-scope packets before any runner dials a real target outside `lab/`.

## 9. Open items

- **Resolver injection:** `AllowsHost` needs an injectable resolver so tests are deterministic and production can use the OS resolver. Decide interface shape.
- **Port checking:** `scope.Allows(ip)` checks IP only; ports enforced by the runner using `cfg.Ports`. Decide whether `Allows(host, port)` is a cleaner single boundary.
- **Cross-scope redirect:** a redirect from an owned target to a research target (or vice versa) — is that a scope escalation requiring re-authorization? Deferred; initial behavior treats it as a new independent scope check.
- **Reload:** can the allowlist be reloaded without restart (e.g., for time-window or exclusion updates)? Deferred to post-launch; initial scope is static per process lifetime.
- **Research program governance:** who authorizes a research scope, what evidence reference is required, and how research targets are selected and reviewed — this is a firm-level process, not a code concern, but it gates production use of research scope.

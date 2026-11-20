# Interfaces

**Status:** proposed · **Derived from:** superseded by [../../spec/DECISIONS/0003-architecture-authority.md](../../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

> Design history, not instructions. These signatures describe paths that do not exist; see the mapping table in ADR 0003.

Universal rules: [../CLAUDE.md](../../CLAUDE.md). Safety rules: [invariants.md](invariants.md). Agent side: [agent-graph.md](agent-graph.md).

These signatures are the contract. Change this file first, log it in [state-log.md](obs/state-log.md), then change code.

## `mission.yaml`

```yaml
mission: string                 # injected into agent system prompt
targets: [string]               # CIDR allowlist, e.g. ["203.0.113.0/24"]
rate_limit_pps: int             # default 100
watchlist_favicons: [int32]     # mmh3 hashes that bypass is_cdn suppression
autonomy:
  tier_1_autonomous: [scan_cidr_delta, probe_http_favicon, query_kv_history]
  tier_2_ttl_auto:
    action: block_single_ip
    max_ttl_minutes: int        # e.g. 60
    min_confidence: float       # [0.0, 1.0], e.g. 0.90
  tier_3_interrupt: [block_cidr, modify_rpz_wildcard, pivot_new_asn]
```

Parsed by both `cmd/hunter/main.go` and `agent/graph.py`. Go's parse is the one that enforces.

## Go packages

### `pkg/cidr`

```go
type Permuter interface {
    NextIP(idx uint64) (ip netip.Addr, ok bool) // ok=false when idx >= Len()
    Len() uint64
}
func NewFeistelPermuter(prefix netip.Prefix, seed uint64) (Permuter, error)
```

Width `w = 32 - prefix_len`. 4-round balanced Feistel, round function SipHash or FNV keyed by `seed`. Odd `w`: use cycle-walking over the next even width.

### `pkg/kv` (Badger v4)

| Key | Value (JSON) | TTL |
|---|---|---|
| `cidr:cursor:<cidr>` | `{index, seed, updated_at}` | none |
| `seen:<ip>:<port>` | `{jarm, cert_sha256, favicon_mmh3, last_seen}` | 168h |
| `cluster:<fingerprint_hash>` | `{count, first_seen, asns[]}` | 720h |
| `mitigate:rule:<rule_id>` | `{target, kind, expires_at}` | `expires_at - now` |

```go
type Store interface {
    GetCursor(cidr string) (index, seed uint64, found bool, err error)
    PutCursor(cidr string, index, seed uint64) error
    CheckAndUpdateDelta(ip string, port uint16, curr Baseline) (mutated bool, mutationType string, err error)
    RecordMitigation(id, target, kind string, ttl time.Duration) error
    ExpiredMitigations(now int64) ([]string, error)
    Close() error
}
```

`mutationType`: `NEW_HOST` | `CERT_CHANGED` | `JARM_CHANGED` | `FAVICON_CHANGED`.

### `pkg/probe`

```go
type Runner interface {
    ScanBatch(ctx context.Context, cidr string, batchSize uint64) ([]DeltaEvent, error)
    ProbeHost(ctx context.Context, target string) (DeltaEvent, error)
}
```

`DeltaEvent` fields: `event_id, ip, port, asn, is_cdn, mutation, jarm?, cert_sha256?, san_list?, http_status?, favicon_mmh3?, page_type?`.

## MCP tools (`cmd/hunter`)

Exactly four. Errors: `isError: true` with code `ERR_OUT_OF_SCOPE` | `ERR_RATE_LIMIT` | `ERR_INVALID_TTL` | `ERR_INTERNAL`.

| Tool | Input | Output | Tier |
|---|---|---|---|
| `scan_cidr_delta` | `{cidr, batch_size: 1..1024}` | `{cursor_index, remaining, events[]}` | 1 |
| `probe_http_favicon` | `{target}` | `DeltaEvent` | 1 |
| `query_kv_history` | `{key_type: seen\|cluster, lookup_key}` | history (read-only) | 1 |
| `apply_ttl_rule` | `{target, kind: nftables_drop\|dns_rpz, ttl_minutes, confidence, reason}` | rule id | 2 or 3 |

`apply_ttl_rule` gate order: TTL range → scope → CIDR/kind approval → confidence → write Badger → install → schedule reap.

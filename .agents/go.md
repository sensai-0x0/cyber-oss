**Status:** authoritative · **Derived from:** `hunterd/`, `enforcerd/`, `operator/`, [invariants.md](orchestration/invariants.md) · **Updated:** 2026-11-21

Universal rules in [../CLAUDE.md](../CLAUDE.md). Python rules in [python-langgraph.md](python-langgraph.md). This file expands the Go section of [orchestration/language-idioms.md](orchestration/language-idioms.md); every rule there still holds.

## 1. Package layout and naming

| Package | Single responsibility |
|---|---|
| `canonjson` | Canonical JSON shared with Python's ledger hashing and the v2 signed bytes (ADR 0005) |
| `hunterd/cursor` | Keyed Feistel bijection over IPv4 CIDRs; `Range` is the resumable iterator |
| `hunterd/store` | BadgerDB lifecycle, checkpoints, `DeltaGate` novelty decision |
| `hunterd/probes` | Run tlsx/httpx, normalize to `Observation` (DeltaEvent v1) |
| `hunterd/cmd/hunterd` | CLI, gRPC health, bounded NDJSON server. Wiring only |
| `enforcerd` | `Action`, `Enforcer`, `Backend` interface, TTL, Ed25519 verification |
| `enforcerd/nft`, `enforcerd/rpz` | Executable adapters. No policy |
| `operator`, `operator/cmd/operator` | Human decision to signed action. No firewall access |

- Plain lowercase names. No `util`, `common`, `helpers`, `types`. A function with no home means a package is missing.
- New code goes in the package that owns the behavior. Do not create `cmd/hunter`, `pkg/*` or `mission.yaml`; see [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md).
- `operator` must never import `nft` or `rpz`. `hunterd` must never import `enforcerd`. Imports that cross services are a design change; write an ADR.
- Files stay under 400 lines. Keep `main.go` to flag parsing and wiring.

## 2. Interfaces and seams

- Accept interfaces, return structs. The consumer defines the interface, as `enforcerd.Backend` is defined where `Enforcer` consumes it, and `nft.Executor` where `nft.Backend` consumes it.
- Keep interfaces to one to three methods. Do not export an interface that has one implementation and no seam.
- Real seams today:

| Seam | Interface | Test substitute |
|---|---|---|
| Firewall / DNS | `enforcerd.Backend` | Recording fake |
| Process execution | `nft.Executor`, `rpz.Executor` | Fake that records `name` and `args` |
| Authorization | `enforcerd.SignedBackend` wrapping a `Backend` | Real keys generated in-test |
| Clock | `Enforcer.now` field; `now time.Time` parameter on `operator.Approve` | Fixed time |
| Probes | None. `TLSRunner` and `HTTPRunner` are concrete | Test `normalize*` functions on canned results |

- A new consumer of a probe runner defines its own one-method interface rather than exporting one from `probes`.
- Construct new values; do not mutate arguments. `operator.Approve` takes `Action` by value and returns a new one. `DeltaGate.Apply` copies `payload` before storing it (`TestObservationPayloadIsCopied`). `SignAction` takes `*Action` to write the signature; do not extend that pattern.

## 3. Context and goroutine lifetime

- `context.Context` is the first parameter at every network, process, and daemon boundary: probes, `Backend.Apply/Revoke`, `Executor.Run`, `Enforcer.Run`, `runNDJSON`.
- `context.Background()` appears only in `main`. `main` builds the root with `signal.NotifyContext` for `SIGINT` and `SIGTERM`.
- Every `go` statement has a visible owner and a visible end: a `done` channel, a buffered error channel that is always drained, or a `defer close`. `runNDJSON` shows the pattern; `run` drains `serveError` and `ndjsonError` through one `select`.
- Upstream APIs without context (`tlsx.Service.Connect`) get a `ctx.Err()` check before the call and a bounded timeout. Say so in the doc comment, as `TLSRunner.Probe` does.
- Bounded queues, never silent drops. `runNDJSON` uses `make(chan T, capacity)`, a producer that blocks on `select { case events <- e: case <-ctx.Done(): }`, and a flag that rejects `capacity <= 0`. Do not use `select` with `default` to discard events, and do not use unbounded slices as queues.
- `bufio.Scanner` input needs an explicit `Buffer` cap, as in `runNDJSON`.

## 4. Errors

- Wrap with `%w`: `fmt.Errorf("open badger: %w", err)`. Message is lowercase, no trailing punctuation, names the failed operation, not the caller.
- Operator-correctable policy failures are exported sentinels: `ErrInvalidTTL`, `ErrApprovalRequired`, `ErrDuplicateAction`, `ErrInvalidAction`. Add a sentinel when a human can fix the cause; use `errors.New` for internal faults.
- Decide with `errors.Is` / `errors.As`, never string matching (`errors.Is(err, badger.ErrKeyNotFound)` in `LoadCursor`).
- Add context to a sentinel with `fmt.Errorf("%w: named operator approver is required", ErrApprovalRequired)`.
- Never swallow. `_ = listener.Close()` in a deferred cleanup is the only accepted discard; comment any other.
- No `panic`, `log.Fatal`, or `os.Exit` below `main`. `cursor.Permute` panics on out-of-domain input as an internal programmer-error guard; `Range` never lets that happen. Do not add callers that pass unchecked values.
- Error strings must not contain credentials, key material, client names, or raw target lists. Name the operation and the field, not the value.

## 5. Security rules specific to this repo

**Scope enforcement does not exist yet.** I1-I5 require an allowlist check in Go before any dial. `HTTPRunner.Probe` and `TLSRunner.Probe` dial after only `ctx.Err()`. No Go file parses configuration or holds an allowlist; `validateEvent` checks event shape, not authorization. See consequence 4 of [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md). The enforcement point is a scope package (planned), not yet created. Until it lands:

- Do not add a new code path that dials, resolves, or follows a redirect.
- Do not write a probe caller that assumes the runner checks scope. It does not.
- Do not describe scope as enforced in docs or comments.

When it is built, it must meet these rules:

| Rule | Invariant |
|---|---|
| Check before dial; out-of-scope returns an error and sends no packet | I1 |
| Resolve first, then check every resolved IP; re-check each redirect hop | I2 |
| CIDR input is a subset of an allowed entry, not an overlap | I3 |
| Empty or unparseable allowlist refuses to start, never allow-all | I4 |
| Allowlist is read-only at runtime | I10 |

- Bounds belong in Go, not in the orchestrator (I7). LLM-supplied arguments are hostile (I8): validate type, range, and format at the boundary, as `nft` does with `identifier` and `netip.Parse*`, and `rpz` does with `dnsName`.
- Executables run through `exec.CommandContext(ctx, name, args...)`. Never `sh -c`, never string-built command lines. Dynamic text goes in an argument or on stdin (`rpz` builds `nsupdate` input), after validation.
- Validate identifiers from configuration at use, not only at load (`nft.Backend.run` rechecks `Table` and `Set`).
- Fail closed. `VerifyActionSignature` rejects a bad key, bad encoding, bad length, and a failed verify. A nil backend or missing key is an error, not a no-op.
- TTL: `0 < ttl <= enforcerd.MaxTTL` (24 hours) at signing, approval, enforcement, and in the backend (I11). Use `ValidateTTL`; do not re-implement the comparison.
- Broad actions need `Approved` and a named `ApprovedBy` (I14). Rejection returns an unsigned action.
- Write `mitigate:rule:<id>` to Badger before calling `Backend.Apply` (I12) so a reaper can remove the rule after a crash. **Not implemented:** `Enforcer` keeps pending actions in an in-memory map and never touches `hunterd/store`. A restart loses every active rule's deadline. The reaper-on-startup of I13 does not exist either. Fix both in `enforcerd`; do not paper over with a comment.
- Logs go to stderr (`log` defaults there; `operator` writes `os.Stderr`). `hunterd` stdout carries only NDJSON events. Never print keys, signatures, or target lists.
- Private keys are written `0o600`.

## 6. Concurrency

- One shared token bucket gates the global rate for tlsx and httpx together (I6). **Not implemented:** no rate limiter exists in Go. Exceeding the limit returns an error; it does not queue. Do not give each runner its own limiter.
- `go test -race ./...` is mandatory. Shared state has one mutex, held across the whole state transition: `Enforcer.Enforce` holds `mu` across duplicate check, `Apply`, and insert so a duplicate ID cannot install two rules. Keep backend `Apply` bounded.
- A `Range` is single-goroutine state. Share by handing over the cursor value (`Cursor()`/`Restore`), not the pointer.
- BadgerDB allows one process per directory (I17). Only `hunterd` opens it. Never open a second `store.Open` on the same path in-process or from another service, and do not add a Go reader that bypasses `hunterd`.
- `Expire` stops at the first revoke error and leaves the rest tracked. Keep that retry-safe behavior; never drop an action whose revoke failed.

## 7. State and persistence

- Permutation state is O(1): prefix, key, `next`, `count`. Do not materialize an address list, shuffle a slice, or keep a seen-set.
- Persist `Range.Cursor()` with `SaveCursor` every 256 probes and on `SIGINT`/`SIGTERM` (I16). On start, `LoadCursor` then `Restore`; resume, never rescan from zero. **Not wired:** nothing in `hunterd` calls `SaveCursor`.
- Cursor scope keys must be non-empty (`SaveCursor` rejects `""`). Namespace by engagement and CIDR so ranges do not share a position.
- Delta read, compare, and write happen inside one `db.Update` (`DeltaGate.Apply`). Never split into a `View` then an `Update`.
- A `Update` can return `badger.ErrConflict` under contention; retry or surface it, do not treat it as novel.
- The `hash` passed to `Apply` must exclude `observed_at`. `newObservation` folds `observed_at` into its fingerprint, so using that fingerprint as the gate hash would classify every repeat as mutated.
- Observation state expires by Badger TTL (`DeltaGate.ttl` must be positive). Never store unbounded history.

## 8. JSON contracts

- `schemas/delta_event.v1.json` and `schemas/mitigation_action.v1.json` are the source of truth. The Go struct follows the schema; the schema does not follow the struct.
- A semantic change ships as `v2` with its own schema file and ADR. Never alter v1 meaning. `schema_version` stays a string constant.
- `attributes` is open: consumers tolerate unknown keys within v1. The top level is closed (`additionalProperties: false`); do not add a top-level field.
- `Observation` enums (`kind`, `source`) must match the schema. `validateEvent` is the Go gate; update both together.
- The signed payload is the canonical struct in `canonicalAction`. It excludes `Signature` and is built from normalized UTC values, so it is stable. Any field added to `Action` that affects behavior must be added to the canonical struct, or it is unsigned and attacker-controlled.
- Keep wire structs separate from domain structs (`operator/cmd/operator` `wireAction` vs `enforcerd.Action`). Convert at the edge.

## 9. Testing

Commands and suites: [testing.md](testing.md). Go-specific rules:

- Table-driven for domain sweeps (bit widths, TTL edges, malformed signatures). One behavior per test function name.
- No real network. `httptest` and loopback listeners only. When scope enforcement lands, loopback is added to the test allowlist explicitly; never bypass the check for tests.
- Fakes for DNS, nftables, NATS, and the clock. A fake `Executor` records `name` and `args` and asserts the argument array (`TestBackendUsesArgumentArrayAndRevokesSameElement`).
- Must be proven by tests: bijection with zero duplicates across odd widths; resume at a mid-range cursor with no skip or repeat; novel, unchanged, mutated on repeated `Apply`; tampered and malformed signature rejection; `ttl <= 0` and `> MaxTTL` rejection.
- The no-per-IP-allocation claim needs `testing.AllocsPerRun` on `Range.Next`. No such test exists yet.
- `t.TempDir()` for Badger; close it in `t.Cleanup`.
- Never `time.Sleep` to synchronize. Use channels, `context` deadlines, or an injected clock.
- Reproduce a bug with a failing test before changing code. Target 80% coverage per package.

## 10. Before handing off

```bash
test -z "$(gofmt -l hunterd enforcerd operator)"
go vet ./...
go test -race ./...
uv run python .github/scripts/validate_outlines.py .   # when .agents/, spec/ or plans/ changed
```

## Red flags in review

| Pattern | Why it is wrong here | Replacement |
|---|---|---|
| `exec.Command("sh", "-c", ...)` or string-built command | Target text becomes shell syntax | `exec.CommandContext(ctx, name, args...)`; validate each argument |
| Calling `Probe`, `Connect`, or `RunEnumeration` before a scope check | Sends packets at an unauthorized target (I1, I2) | Check the allowlist first; check every resolved IP and redirect hop |
| `Backend.Apply` before the Badger write | A crash leaves a rule no reaper knows about (I12) | Persist `mitigate:rule:<id>`, then apply |
| `Action` with zero or unbounded TTL | Permanent rule (I11) | `ValidateTTL`; never bypass `MaxTTL` |
| `panic`, `log.Fatal`, or `os.Exit` in a daemon path | Kills the daemon and strands active rules | Return a wrapped error to `main` |
| Unbuffered-by-design or unbounded channel, slice queue, `select` with `default` drop | Memory growth or silent event loss | `make(chan T, n)` with blocking send and `ctx.Done()` |
| `fmt.Println`, `os.Stdout` for logs | Corrupts the NDJSON stream | `log` or `os.Stderr` |
| `context.Background()` below `main` | Detaches work from shutdown | Accept and pass `ctx` |
| Go statement with no owner | Leaks past shutdown | `done` channel or drained error channel |
| Second limiter per runner | Global rate exceeded (I6) | One shared token bucket |
| Opening Badger from a second process | Directory lock failure (I17) | Only `hunterd` opens the store |
| Allow-all default on empty config | Violates I4 | Refuse to start |
| Hand-rolled TTL comparison | Bounds drift between layers | `enforcerd.ValidateTTL` |
| Error text containing target list, key, or client name | Leaks engagement data | Name the operation, not the value |

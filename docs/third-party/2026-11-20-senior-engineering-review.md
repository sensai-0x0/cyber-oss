# Third-party engineering review: Sentinel Hunter at `9c57762`

**Reviewer stance:** an outside senior engineer, deliberately sceptical of agent-written code and docs · **Date:** 2026-11-20 · **Commit:** `9c57762` (equals `main` and `origin/main`) · **Companion:** [../../plans/ml-add/README.md](../../plans/ml-add/README.md)

This file is a narrative review (the `docs/` layer). It does not override `spec/` or the code. Findings without an SHR id are new. Findings with one refer to [../reviews/2026-11-21-codebase.jsonl](../reviews/2026-11-21-codebase.jsonl).

---

## 1. Verdict

The repo is a **set of well-tested parts that are not connected, wrapped in a process layer about three times its size**. The parts are mostly good. The connections that would make it a system do not exist. The process layer reports discipline that the git history does not show.

| Dimension | Score /10 | One line |
|---|---|---|
| Primitives (Feistel, delta gate, signing, backends) | 7 | Small, idiomatic, tested, argv-safe exec |
| System integration | 1 | No component calls another. There is no end-to-end path at all |
| Authorization soundness | 2 | Every central safety claim can be bypassed today, and all the Go findings from the earlier review are still open |
| Test suite | 6 | 56 Python tests and 8 Go packages green, with real adversarial cases. Asserts on the parts, never on the whole |
| Docs accuracy | 3 | The file marked authoritative describes an architecture that was superseded. Status tables are hand-edited and stale |
| Process / agent layer | 2 | Elaborate, never run, and its own protected clauses were broken by the last four merges |

**Ship status:** not deployable, and should not be described as "Phase 2 complete".

## 2. What is actually here

Measured, not taken from the docs:

| | Lines |
|---|---|
| Production Go and Python, excluding tests and `.github/scripts` | ≈1,450 |
| Tests | ≈1,550 |
| Markdown and HTML (≈55,000 words) | ≈5,500 |
| Agent definitions / Claude skills / `.agents/` skills | 3 / 7 / 5 |
| Commits / wall-clock time from first commit to HEAD | 29 / ≈1h40m |

Twenty-nine commits in under two hours, about four lines of prose for every line of product code, and a "self-evolving constitution" with zero recorded fitness. That pattern is what agent-led bulk generation looks like. It is not damning in itself, but every claim in the docs has to be checked before it is believed. Several fail the check (§5).

## 3. Method

1. Ran the CLAUDE.md verification list.
   - `go vet` and `go test -race ./...` pass.
   - `ruff` and `pytest` pass (56 tests).
   - `validate_skills` passes.
   - **`validate_outlines.py --strict` fails** (§5.1).
2. Read every production source file.
3. Ran a scratch harness against `orchestrator/` to reproduce findings. Its inputs are in §8.
4. Checked the docs' claims against git history and the tree.
5. Did not run the compose lab or any live probe.

## 4. Product findings

### 4.1 New, or materially worse than the register says

| # | Sev | Where | Finding | Evidence |
|---|---|---|---|---|
| N1 | **Critical** | `orchestrator/policy.py:36-37` | **Any CIDR is "narrow" unless the producer says otherwise.** `broad` comes from `attributes.prefix_length`, default 32. It is never parsed from `target`. So `{"target":"10.0.0.0/8"}` gives `broad=False, approved=True`. SHR-007 records only the `/24` boundary, which badly understates this | Reproduced (§8) |
| N2 | High | `enforcerd/rpz/backend.go:58-59` | `nsupdate -v` runs with no TSIG key (`-k`). The zone either accepts unauthenticated dynamic updates, or every update fails. Either way, the "signed action" guarantee stops at the process boundary | Read |
| N3 | High | `hunterd/probes/probe.go:40` | `Kind` is always `"novel"`. `DeltaGate`'s verdict is never fed back into it. So the invariant *"only novel and mutated events cross the boundary"* holds only because every event is labelled novel | Read |
| N4 | High | `hunterd/cmd/hunterd/main.go` | `serve` never calls a probe, the cursor or the delta gate. It validates stdin NDJSON and echoes it to stdout. As a result, every attribute is producer-controlled. This is the root cause of SHR-008, and it undermines the ML plan's K12 (§6) | Read |
| N5 | Med | `enforcerd/enforcer.go:92-101` | The mutex is held across `backend.Apply`, which is a subprocess exec, so a slow `nft` or `nsupdate` blocks `Expire`. Combined with SHR-004, one hung revoke stops all enforcement and all expiry | Read |
| N6 | Med | `enforcerd/nft/backend.go:50` | Set elements are added without `timeout`. nftables can expire elements itself. Using that would bound a rule's lifetime even if `enforcerd` crashes (SHR-003). It is cheap defence in depth and is not used | Read |
| N7 | Med | `orchestrator/score.py:21`, `policy.py:36` | `"known_bad": "false"` counts as true (+confidence). Non-integer `shared_fingerprints` or `prefix_length` raises `ValueError` mid-graph | Reproduced |
| N8 | Med | `orchestrator/tests/test_triage_quality.py:151` | The test writes `last_run_results.json` into the source tree on every run. The output is deterministic, so nothing is dirtied today. Once a real model is added, it will be | Read |
| N9 | Low | `hunterd/cmd/hunterd/main.go:141-157` | Ingest does not apply the schema's `maxLength` on `event_id` (128) or `target` (2048). Go accepts what Python then rejects | Read |

### 4.2 Status of the 2026-11-21 register (31 findings)

- **No Go source file has changed since the register was written** (`git log a03aa10..HEAD -- '*.go'` is empty). SHR-001 to -006, -009 to -011, -016 to -028 and every other Go finding are **open**.
- Python:
  - SHR-015 is fixed (`apply_decision` is pure).
  - SHR-013 is half fixed: `max_refinement_cycles` is read, but `AMBIGUOUS_FLOOR` is still hard-coded.
  - SHR-007, -008, -012 and -014 are open.

So the four Phase 2 merges since the register added tests and docs around a decision path whose critical holes were already known and are still there. That was the wrong priority.

### 4.3 What is genuinely good

- Signature canonicalisation recomputes expiry and refuses a mismatch.
- `Reject` produces an object that cannot be deployed.
- Backends exec with argv, never through a shell, and validate identifiers.
- The `DeltaGate` read-modify-write is one Badger transaction.
- The Feistel cursor is O(1) memory, has a bijection test, and can be resumed.
- `test_adversarial_boundaries.py` (352 lines) tests real hostile inputs, not happy paths.
- The SQLite checkpointer is honest that it is opt-in and not approved (`graph.py:33-39`).
- ADR 0003 is an unusually candid account of drift.

## 5. Docs and process accuracy

This is where agent verbosity does real damage: a reader cannot tell which of 55,000 words to trust.

### 5.1 The gate built to catch drift is red

`validate_outlines.py --strict` exits 1 at HEAD because of `spec/STATE.md:32`, the STATE decision-log line written by the P2.4 work. CI runs this job on every PR (`.github/workflows/ci.yml:42-54`), so `main` is red on its own docs gate. Either nobody looked at CI before merging, or CI never ran (no `develop` branch exists on the remote, and the merges went straight into `main`).

### 5.2 Protected clauses broken by the last four merges

| Rule | Source | What happened |
|---|---|---|
| "Invariant evals (E03, E04, E05) must pass for any merge" | `.agents/evals/constitution.md`, protected clause 4 | `evals.md` itself says none of E01–E09 has ever run. Four merges went ahead anyway |
| "Only a human merges to `main`" and "no exit gate complete without the human's checkbox" | `plans/PHASESv1.0.md:41-43` | Every checkbox in all seven `plans/planv1.0/PHASE*.md` files is unticked, including P2.2's and P2.3's 🔒 checkpoints, yet `main` contains all four Phase 2 merges. This review cannot verify who merged |
| Feature gate requires `staticcheck` and `mypy --strict` | `.agents/evals/orchestrator.md` §4 | Neither is configured in CI or `pyproject.toml`. The gate as written cannot have passed |
| `develop` ← feature branches | same, §3 | There is no `develop` branch on the remote |

A process that lists rules it does not enforce is worse than having no process: it produces confident status reports that are false.

### 5.3 "Authoritative" documents describe the superseded system

ADR 0003 retired the single-binary MCP design (`cmd/hunter`, `mission.yaml`, four MCP tools). Even so:

- `.agents/orchestration/invariants.md` (**Status: authoritative**) still sets I1, I7, I9, I10, I17 and the stdout rule in terms of `mission.yaml`, `cmd/hunter`, MCP tools and JSON-RPC. `docs/05` admits this ("written for the original single-binary design and still the operative safety contract").
- `.agents/evals/orchestrator.md` §2 has a P0–P5 phase table built around a `mission.yaml` loader. `plans/PHASESv1.0.md` has a different Phase 0–6 numbering. Two incompatible roadmaps are both "active".
- `db/INSTALL.md` specifies Graphiti for `cmd/hunter` and `agent/graph.py`, neither of which exists. `db/` is outside the validator's roots, so this is never caught.
- `.agents/sentinel-lite-context.zip` is a binary snapshot of superseded agent docs. The validator cannot see inside it, and an agent that unzips it reintroduces Reading B.
- 20 markdown files still reference `mission.yaml` or `cmd/hunter`.

### 5.4 Status tables are typed, not derived

`.agents/orchestration/obs/state-log.md` describes itself as "regenerated, not edited". It still lists "Durable LangGraph checkpointer — Absent" after P2.2 merged one, and "Orchestrator agent + skills — uncalled" while the Phase 2 merge messages imply they ran. `process-ledger.md` and the constitution history are empty templates. Outline method §7 forbids exactly this, and the repository breaks its own rule a day after writing it.

### 5.5 The validator has loopholes that invite gaming

`.github/scripts/validate_outlines.py`:

- `Status: proposed` skips **all** path checks (lines 41, 132). Any document can escape the gate by calling itself proposed.
- Paths are checked only if they end in a known suffix, so `cmd/hunter` and `agent/` slip through.
- Thresholds are flagged only if they are floats or integers ≥1000, so pasting `24` (the broad boundary), `900` or `60` is never flagged.
- §3 of the outline method allows three statuses. The validator accepts six (`active`, `draft`, `accepted` as well).
- "Proposed requires the authorising ADR" (§3) is not checked.

### 5.6 `docs/05-safety-and-trust-model.md` still overclaims

It says `Enforcer.Enforce` *"independently rejects"* bad actions *"regardless of what upstream claimed"*, and that signatures are *"independent of the caller's claims"*. SHR-001, SHR-002, SHR-009 and N1 each contradict this. SHR-023 recorded the overclaim, and the text has not changed. A safety document that is false is a liability, not documentation.

## 6. The ML review v2 (ingested into `plans/ml-add/`)

The broad direction is sound: shadow mode, a human checkpoint, deterministic bounds and lead time as a metric. Its key mechanism is weaker than it claims. The detail, and the safe phase order that follows from it, are in [../../plans/ml-add/README.md](../../plans/ml-add/README.md). In short:

1. **K12 provenance classifies by field name and does not authenticate.** Because of N4, an attacker can write `jarm` as easily as `known_bad`. The doc's statement that K12 "fixes SHR-008" is false.
2. **R1 understates the precondition.** The problem is N1, not only SHR-007.
3. **K13 changes the signed schema** (`additionalProperties:false`) and needs a v2 contract and an ADR. The doc does not mention this.
4. **K9 and K10 are circular.** The same team would write both the synthetic adversary and the scorer, and no real event stream exists.
5. **The scoring table cannot be reproduced** (no raw scores are given, and three of the four weightings are only partly specified). It also contains a false stability claim: K1 ranks #5 under creativity-first.
6. The doc does not mention that today's scorer has **recall 0.0** on the fixture set (`last_run_results.json`): even the staging fixture with the most signal reaches only 0.75.
7. It links two files that were never committed.

## 7. Recommendations, in order

Stop adding surface. For each item below: one feature, tests first, and verify by running it, not by updating a table.

1. **Fix N1 and SHR-001 together.** Parse targets into `(kind, prefix, broad)` in Python *and* Go, never from attributes. Until then, route every mitigation through `interrupt()` (Phase 0 of the ML plan).
2. **Fix SHR-002, -004, -005 and -009, and add N6.** Make expiry, replay and signature checks mandatory inside `Enforce`. Never let a revoke error stop the loop. Give nft elements a `timeout`.
3. **Make CI green and honest.** Fix `spec/STATE.md:32`. Add `staticcheck` and `mypy`, or delete them from the gate text. Create `develop`, or delete the branch model from the docs.
4. **Rewrite `invariants.md` for the four-service system**, or mark it superseded and write a new one. Delete `sentinel-lite-context.zip`. Bring `db/` into the validator's roots, or mark it superseded.
5. **Close the validator loopholes in §5.5**, especially the rule that `proposed` skips path checks.
6. **Build one end-to-end path** in the lab: probe → gate → NDJSON → graph → operator → `SignedBackend` → fake backend → expiry. Write it as a single test. No ML work has real data until this exists.
7. **Freeze the meta-process.** No new agents, skills, genes or constitution clauses until the existing ones have run once and produced a fitness record. Today there are two overlapping top-level orchestrators (`conductor`, `master-orchestrator`), seven Claude skills and an empty fitness history, for about 1,450 lines of product.

## 8. Reproduction

From the repo root, with `PYTHONPATH=.`, feed these to `score_threat_intel` and to `synthesize_policy` (with `intel.confidence` at its maximum):

| Input | Observed |
|---|---|
| `target="10.0.0.0/8"`, no attributes | `broad False, approved True` |
| `target="1password.example"` | `kind nftables` |
| `target="192.0.2.10:443"` | `kind nftables`, a target the nft backend cannot parse |
| `known_bad="false"` | confidence rises |
| `known_bad=True, recent_ct=True` | 0.75, below `policy.yaml: mitigation_confidence` |
| `shared_fingerprints="abc"` | `ValueError` |
| One rule (`known_bad or shared_fingerprints≥2`) over `scenarios.jsonl` | 100/100 correct |

Toolchain:

```bash
uv run python .github/scripts/validate_outlines.py --strict   # exit 1, spec/STATE.md:32
git log a03aa10..HEAD -- '*.go'                               # empty: no Go fix since the register
grep -c '\- \[x\]' plans/planv1.0/PHASE*.md                    # 0 in every file
```

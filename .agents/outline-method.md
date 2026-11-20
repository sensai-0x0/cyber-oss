# Outline method

**Status:** authoritative · **Derived from:** [0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md), link and path audit of 35 project markdown files on 2026-11-21 · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

An **outline** is any document that tells a contributor or an agent what to build:
everything under `.agents/`, `plans/` and `spec/`. This file governs how outlines are
formed. It exists because the outlines in this repository drifted far enough from the
code to become actively misleading — see [0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md)
for what went wrong. The rules below are the controls; `.github/scripts/validate_outlines.py`
is the gate that enforces the mechanical ones.

## 1. Authority ladder

Highest rank wins. An outline **never** overrides a layer above it. It either conforms,
or it ships an ADR that moves the higher layer first — in the same change.

| Rank | Layer | Holds | Moved by |
|---|---|---|---|
| 1 | Code + tests on `develop` | What the system actually does | A merged feature |
| 2 | `schemas/*.json` | Cross-language payload contracts | A new schema version + ADR |
| 3 | `spec/`, incl. `spec/DECISIONS/` | Normative architecture and accepted decisions | A new ADR |
| 4 | `plans/` | Intent: PRD, phases, data model, contracts | Replanning, recorded |
| 5 | `docs/` | Narrative explanation for humans | Rewrite to match 1–4 |
| 6 | `.agents/` | Working instructions for agents | Rewrite to match 1–4 |
| 7 | Chat / a session's own reasoning | Nothing durable | — |

Recency is **not** a ground for authority. The drift in ADR 0003 happened precisely
because one document claimed to win on the grounds of being newer while the artifacts it
overrode were both newer *and* implemented.

## 2. Layer taxonomy

One class of document, one home. Do not create a second home for an existing class.

| Home | Contains | Does not contain |
|---|---|---|
| `spec/` | Architecture, scope, state contracts, ADRs | Task lists, agent instructions |
| `plans/` | PRD, phase plans, data model, interface contracts | Narrative prose, ADRs |
| `docs/` | Explanation, onboarding, roadmap | Normative claims, instructions to agents |
| `.agents/` | Rules, idioms, evals, orchestration, state log | Product architecture |

## 3. Provenance header

Every outline opens with one line, before any prose:

```
**Status:** authoritative | proposed | superseded · **Derived from:** <paths or ADR> · **Updated:** YYYY-MM-DD
```

- `authoritative` — describes what is true now. Must agree with rank 1–3.
- `proposed` — a design not built. **Must not be read as an instruction.** Requires the
  authorizing ADR in `Derived from`.
- `superseded` — kept for history. Names its replacement.

A document with no header is treated as `proposed` by the validator and fails the gate.

## 4. Path reality

Every repository path an outline names — in a link, in backticks, or in a table — must
either exist in the tree or be marked `(planned)` on the same line, with the ADR or plan
that authorizes it. Backticked paths are included deliberately: the 2026-11-21 audit
found a whole class of broken references that no link checker sees, because
`` `docs/ARCHITECTURE.md` `` is not a link.

A `(planned)` path is a promise. An unmarked nonexistent path is a defect.

## 5. One source per constant

A numeric threshold lives in exactly one file and is cited everywhere else by key name.

- Runtime policy thresholds: `orchestrator/policy.yaml`.
- Orchestrator allocation parameters: [evals/constitution.md](evals/constitution.md).

Write `policy.yaml: mitigation_confidence`, never the number it currently holds. Pasted
values drift — the audit found the same threshold recorded as three different numbers
across three files, in a repository whose own idiom file forbids exactly that.

The sole exceptions are invariants, which are bounds rather than tunables: the 24-hour
TTL ceiling and the limits in [orchestration/invariants.md](orchestration/invariants.md).

## 6. Divergence protocol

When an outline and the code disagree, the disagreement is the bug. Do not leave both
standing, and do not "fix" it by asserting which one is right inside the outline itself.

1. **Stop.** Do not implement against either side.
2. **Write an ADR** in `spec/DECISIONS/` naming both readings and the evidence for each —
   dates, which one the code implements, which one tests gate on.
3. **Pick one.** Record the consequences for every affected path.
4. **Change one side in the same commit.** Either the code moves to the outline, or the
   outline is rewritten and marked `proposed` or `superseded`.
5. **Retarget anything that gated on the losing side** — evals especially. An eval that
   gates on a path which does not exist is not a passing eval, it is an absent one.

## 7. Status is derived, not typed

Build status belongs to the tree and the test run, never to a hand-maintained table.
The audit found every component of one design listed "Not started" while its implemented
equivalent sat in the tree under a different name, tested and passing.

A status table must state what it was derived from and when, and must be regenerated
rather than edited. Where a claim cannot be derived, write the command a reader can run
instead of asserting the answer.

## 8. Definition of done

An outline change is done when all of the following hold:

- [ ] Provenance header present, with a `Status` the content actually supports.
- [ ] Every relative link resolves from the file's own directory.
- [ ] Every path named exists, or is marked `(planned)` with its authorizing ADR.
- [ ] No numeric threshold pasted from `policy.yaml` or the constitution.
- [ ] No claim that contradicts rank 1–3 of the ladder.
- [ ] `uv run python .github/scripts/validate_outlines.py --strict` passes. CI runs it
      strict, so a missing provenance header fails the build, not just warns.
- [ ] `.agents/skills/` still holds at most five skills — `validate_skills.py` enforces
      `MAX_SKILLS = 5`. Orchestrator-side skills belong in `.claude/skills/`.

## 9. Why this is a gate and not a convention

Rules 3, 4, 5 and 8 are mechanically checkable, so they are checked. The preceding drift
was not caused by carelessness about wording; it was caused by there being no step at
which a false path reference or an inverted authority claim would fail. Conventions that
nothing verifies decay into the state this file was written to repair.

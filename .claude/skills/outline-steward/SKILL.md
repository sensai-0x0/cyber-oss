---
name: outline-steward
description: Form or repair any Sentinel Hunter outline (anything under .agents/, plans/ or spec/) so it obeys the authority ladder, carries provenance, names only real paths, and duplicates no thresholds. Use before writing a plan or spec, when documents disagree with the code, or when validate_outlines.py fails.
---

# Outline steward

Owns [.agents/outline-method.md](../../../.agents/outline-method.md). An outline is any
document that tells a contributor or an agent what to build. A wrong outline is worse
than a missing one: agents follow it.

## Workflow

1. **Locate the layer.** `spec/` normative · `plans/` intent · `docs/` explanatory ·
   `.agents/` instructions to agents. One class of document, one home. Do not create a
   second home for a class that already has one.
2. **Check the ladder.** Code and tests on `develop` outrank `schemas/`, which outrank
   `spec/`, which outranks `plans/`, `docs/` and `.agents/`. If the document contradicts
   a higher layer, it is wrong — even when it is newer. Recency is not authority.
3. **Run the divergence protocol** when a document and the code disagree: stop, write an
   ADR in `spec/DECISIONS/` naming both readings and the dated evidence for each, pick
   one, then change **one side in the same commit**. Never leave both standing, and
   never settle it by asserting the answer inside the document itself.
4. **Fix provenance.** Every outline opens with
   `**Status:** authoritative | proposed | superseded · **Derived from:** … · **Updated:** YYYY-MM-DD`.
   `proposed` means not built and must never read as an instruction.
5. **Fix paths.** Every path named — in links, in backticks, in tables — exists, or
   carries `(planned)` plus its authorizing ADR. Backticked paths matter: they are the
   class no link checker catches.
6. **Fix constants.** Cite `orchestrator/policy.yaml` and constitution keys by name.
   Never paste a number. Invariant bounds are the only exception.
7. **Rebuild derived status.** Status tables come from the tree and the test run, never
   from memory. Where a claim cannot be derived, give the command instead of the answer.
8. **Verify** — `uv run python .github/scripts/validate_outlines.py`.

## Output

The repaired document, the dead references fixed, any ADR written, and an explicit list
of claims you could not verify — left marked rather than quietly asserted.

## Rules

- Never mark a document `authoritative` to make the validator pass. If it describes
  something unbuilt, it is `proposed`.
- Never delete a divergence you cannot resolve. Record it and escalate.
- `.agents/skills/` is capped at five by `validate_skills.py`. New orchestrator-side
  skills go in `.claude/skills/`.

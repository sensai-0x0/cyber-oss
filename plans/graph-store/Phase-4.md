# Phase 4: Ontology and ingestion from the ledger

**Status:** proposed · **Derived from:** [README.md](README.md), [Phase-3.md](Phase-3.md), [../ml-add/Phase-1.md](../ml-add/Phase-1.md), [../ml-add/Phase-2.md](../ml-add/Phase-2.md), [../../db/CONFIG.md](../../db/CONFIG.md), [../../schemas/delta_event.v1.json](../../schemas/delta_event.v1.json) · **Updated:** 2026-11-20

**Depends on:** Phase 3, and the ML ledger (ml-add Phase 2) as the source of truth. Without a ledger, nothing can be replayed after a failure, so do not start earlier.

## Goal

Decisions flow from the ledger into the graph as structured JSON episodes, with a defined ontology, idempotent keys and no untrusted free text reaching the extraction model.

## Scope

1. **Ontology** (`orchestrator/graph_ontology.py`, planned). Replace the `WorkBlock`, `Worktree` and `FailurePattern` types in `db/CONFIG.md`, which model agent workflow, not the hunting domain. Candidate entities: `Target`, `Indicator` (JARM, TLS fingerprint, cert hash), `Campaign`, `Decision`. Candidate edges: `OBSERVED_WITH`, `CLUSTERED_WITH`, `DECIDED`. The ADR confirms the list.
2. **Episode builder.** One episode per ledger record: JSON only (`EpisodeType.json`), a fixed field set, and the episode key equal to the ledger `record_hash`. Replay is therefore idempotent.
3. **Sanitising.** Only fields on an allowlist are sent. Strings are length-capped and restricted to the characters a hostname, IP or hex digest can contain. Banners, page titles and error text never enter an episode.
4. **Provenance.** Each fact carries its class (OBSERVED, DERIVED, ASSERTED). Per the ADR, ASSERTED facts are not ingested.
5. **Replay tool** (`python -m orchestrator.graph_replay`, planned): reads the ledger from a given `seq` and re-submits. It is safe to re-run.

## Tests first

- The same ledger record submitted twice yields one episode.
- A record with a hostile string (instruction text, control characters, an oversized value) produces an episode with that string rejected or truncated. Assert on the exact payload sent to a fake LLM.
- An ASSERTED indicator never appears in a payload.
- Replaying from `seq` 0 on an empty graph reproduces the node and edge counts of an uninterrupted run, within the tolerance the LLM variability allows. The test uses a deterministic fake extractor.
- Deleting the graph and replaying restores it. The graph is derived state.

## Failure modes

| ID | Failure | Detection | Handling |
|---|---|---|---|
| GF-4.1 | Duplicate episodes after a retry or restart | Key test | Key by `record_hash`, with a lookup before write |
| GF-4.2 | Prompt injection: a hostile target banner steers the extraction LLM and plants false facts | Hostile-string test | Allowlist fields, character classes and length caps. No free text, ever |
| GF-4.3 | Graph poisoning: an attacker makes many targets that link benign infrastructure into a "campaign" | Rarity and fan-out report (Phase 5) | The graph informs analysis only. Cap per-entity fan-out. A shared CDN fingerprint is an expected false link |
| GF-4.4 | Ledger and graph diverge after dropped episodes | Count comparison by `seq` | Replay tool. The ledger is authoritative |
| GF-4.5 | Entity resolution merges two different targets, or splits one | Spot-check set | Use stable identifiers in the payload. Treat LLM resolution as advisory |
| GF-4.6 | Ontology change leaves old episodes in the old shape | Version field in each episode | Version the ontology. Migration is a full replay into a new `group_id`, then a switch |
| GF-4.7 | Sensitive data in the graph (real target lists, operator names) | Allowlist review | Lab data only until the ADR says otherwise. Do not commit exports |
| GF-4.8 | IPv6, internationalised hostnames or port suffixes break the character rules | Boundary tests | Normalise with the same classifier as `classify_target` in ml-add Phase 0. Reject what does not parse |

## Acceptance

- [ ] Tests pass.
- [ ] Hostile-string test is part of the adversarial suite.
- [ ] Replay from empty rebuilds the graph.
- [ ] Failures logged.

## Rollback

Disable the store, then drop the `group_id`. The ledger is untouched.

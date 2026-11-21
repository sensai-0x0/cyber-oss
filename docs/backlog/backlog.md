# Backlog

**Status:** active · **Derived from:** [README.md](README.md); entries registered from session work, appended never rewritten · **Updated:** 2026-11-21

One item per line. Grammar, field values and the rule for closing an entry: [README.md](README.md).
Newest last. Registered on `worktree-mellow-launching-lantern` at `78bb3fd` unless a line says
otherwise.

2026-11-21T02:43:15Z · BL-0001 · bug · medium · open · Three test_classify_target_table cases fail on CPython 3.13 because ipaddress renders IPv4-mapped IPv6 as ::ffff:a00:0 rather than ::ffff:10.0.0.0, while pyproject.toml allows >=3.11,<3.14, so a bare uv run picks 3.13 and the suite is red; CI and CLAUDE.md both pass --python 3.12, so the gate has never seen it — .agents/orchestration/obs/errors.md a883eb6c
2026-11-21T02:43:15Z · BL-0002 · task · high · open · ADR 0008 is unwritten, so the graph topology, the D-rules and the single-interrupt contract are a design and tasks G0 to G8 have not started — plans/agent-graph.md G0.1
2026-11-21T02:43:15Z · BL-0003 · decision · low · wont_fix · Renaming the orchestrator package to triage is rejected at S = 0 with a Critical flag although triage is the accurate name; revisit only as its own ADR and only before G3 and G6 add modules — plans/agent-graph.md §11.1 and .claude/skills/refactor-eval/scorecard.md
2026-11-21T02:43:15Z · BL-0004 · decision · low · wont_fix · Extracting routers.py from existing modules and moving the evidence and scorer modules into subpackages are rejected as moves at -20 and -80; the same files are accepted as new-code placement, enforced by an import-direction test rather than directories — plans/agent-graph.md §11.3
2026-11-21T02:43:15Z · BL-0005 · question · medium · open · The ambiguity floor in orchestrator/score.py is a module constant while every other threshold reads from policy.yaml; decide whether it becomes a policy key or is deleted with the refinement loop — plans/agent-graph.md G5.2
2026-11-21T02:43:15Z · BL-0006 · question · medium · open · The ASN and CDN enrichment sources are unchosen and must be pinned and offline to keep the enrich node pure — plans/agent-graph.md D3 and .agents/orchestration/obs/state-log.md open questions
2026-11-21T02:43:15Z · BL-0007 · question · low · open · The dedupe and held-event stores have no chosen backend; both are rebuildable from the ledger, so decide whether they survive a restart or are rebuilt — plans/agent-graph.md G3.3
2026-11-21T02:43:15Z · BL-0008 · question · medium · open · The analyst brief adds a model that reads attacker-influenced text; D10 and E27 bound its influence on policy but not its cost if an adversary inflates the window, which is for HC-5 to judge — plans/agent-graph.md G6.4
2026-11-21T02:43:15Z · BL-0009 · debt · medium · open · orchestrator/orchestrator_instructions.md instructs agents to load mission.yaml, ./INSTALL.md, ./CONFIG.md, ./AGENT_ARCHITECTURE_PLAN.md and ./SENTINEL_LITE_PRD.md, none of which exist, and describes the superseded single-binary layout as current; it is a live instruction file pointing at a retired design — spec/DECISIONS/0003-architecture-authority.md
2026-11-21T02:43:15Z · BL-0010 · debt · low · wont_fix · docs/third-party/2026-11-20-senior-engineering-review.md still names the master-orchestrator agent after the rename to phase-runner; it is a dated external record and is deliberately left as written — .agents/orchestration/obs/state-log.md decisions
2026-11-21T02:43:15Z · BL-0011 · debt · low · open · Revisit the flat per-event graph topology with an ADR if the node count passes roughly a dozen, rather than nesting subgraphs quietly — plans/agent-graph.md §10
2026-11-21T02:43:15Z · BL-0012 · debt · low · open · docs/06-engineering-process.md §5 cites .agents/state-log.md twice, which does not exist at that path and is .agents/orchestration/obs/state-log.md; validate_outlines.py scans only .agents, plans and spec, so dead paths in docs go undetected — .agents/outline-method.md §4

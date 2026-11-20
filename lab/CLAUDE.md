**Status:** authoritative · **Derived from:** `lab/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Harmless local fixtures that stand in for hostile infrastructure, so probing can be developed without touching real targets. Until scope enforcement lands in `hunterd`, this is the only place active probing is justified (ADR 0003 consequence 4).

| File | Fixture |
|---|---|
| `lab/fake_c2.py` | GET returns JSON staging metadata; no beacon, payload, shell or command execution |
| `lab/phish_panel.py` | HTML with disabled inputs; every POST returns 403, nothing is read or stored |
| `lab/Dockerfile` | Image for both, run by `deploy/compose.yaml` |

**Boundaries**

- No real C2 behavior: no `subprocess`, `os.system`, listeners that execute input, or outbound connections.
- No credential collection. The phishing fixture rejects all POSTs; do not enable inputs or read request bodies.
- Standard library only; bind loopback by default. Compose publishes loopback ports only.
- Fixtures must stay obviously fake in their output text. Never mimic a real brand or organization.
- Never point lab tooling at external infrastructure.

**Rules that apply**

- [../.agents/testing.md](../.agents/testing.md): tests touch loopback only.
- Skill: `.agents/skills/lab-validator`. Scope rules I1-I5 apply to anything that probes these fixtures.

**Verify**

```bash
uv run pytest orchestrator/tests/test_lab.py
docker compose -f deploy/compose.yaml config --quiet
```

**Status:** authoritative · **Derived from:** `deploy/`, [../.agents/orchestration/invariants.md](../.agents/orchestration/invariants.md), [../spec/DECISIONS/0003-architecture-authority.md](../spec/DECISIONS/0003-architecture-authority.md) · **Updated:** 2026-11-21

Universal rules: [../CLAUDE.md](../CLAUDE.md).

**Purpose.** Loopback Compose lab, not a production deployment. `deploy/compose.yaml` runs NATS (TLS, JetStream, password auth, config in `deploy/nats/nats.conf`), the two `lab/` fixtures, CoreDNS with `deploy/dns/rpz.lab`, and an optional `privileged-lab` firewall target profile.

**Boundaries**

- Every published port binds `127.0.0.1`. Never publish on `0.0.0.0` or point services at real infrastructure; `orchestrator/tests/test_lab.py` asserts this.
- The firewall target needs `NET_ADMIN` and host networking. Keep it behind its profile and run it only for manual nftables checks.
- `deploy/certs/generate.sh` makes one-day certificates under a lab-only CA, written to a Git-ignored directory. Never commit keys or reuse them outside the lab.
- The NATS password in Compose is a lab placeholder. A real deployment injects secrets from the environment or a secret manager (see the secrets rule in [../CLAUDE.md](../CLAUDE.md)).
- No Go or Python services are built here yet; `hunterd`, `enforcerd` and `operator` run from source.
- Pin image tags; do not use `latest`.
- The `graph` profile runs FalkorDB on `graphnet` (internal, no published port). `FALKORDB_PASSWORD` is required and has no default, so `compose config` needs a throwaway value, as in the verify block below.

**Rules that apply**

- [../.agents/testing.md](../.agents/testing.md); skill `.agents/skills/lab-validator`.
- Related fixtures: [../lab/CLAUDE.md](../lab/CLAUDE.md).

**Verify**

```bash
FALKORDB_PASSWORD=ci-only docker compose -f deploy/compose.yaml config --quiet
uv run pytest orchestrator/tests/test_lab.py
./deploy/certs/generate.sh
```

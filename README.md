# Sentinel Hunter

A test-first scaffold for authorized pre-attack infrastructure hunting, LangGraph triage, human-approved response, and automatic mitigation expiry.

> Active probes and firewall/DNS changes require explicit authorization. Start with the loopback-only lab. This repository does not claim production integration is complete; see `docs/ARCHITECTURE.md`.

## Layout

```text
repo/
├── CLAUDE.md                    # Contributor commands, safety invariants, skill entry points
├── .agents/language-idioms.md   # Go + Python implementation conventions
├── schemas/                    # Versioned DeltaEvent + MitigationAction JSON contracts
├── hunterd/                    # Go adapters, Feistel cursor, Badger store/novelty, NDJSON publish
├── orchestrator/               # Python graph.py, cluster.py, score.py, policy.yaml + tests
├── enforcerd/                  # Go RPZ/nft adapters, Ed25519 verification, TTL lifecycle
├── operator/                   # Approval/rejection and key-generation CLI
├── deploy/                     # Loopback Compose, NATS configuration, local cert generator
├── lab/                        # Harmless fake C2, disabled phishing panel, DNS/firewall targets
└── docs/                       # ARCHITECTURE.md, SCOPE.md, STATE.md, DECISIONS/
```

## Verify locally

```bash
go test ./...
uv sync --frozen --python 3.12
uv run pytest
uv run ruff check orchestrator .github/scripts
uv run python .github/scripts/validate_skills.py .agents/skills
docker compose -f deploy/compose.yaml config --quiet
```

## Lab

```bash
./deploy/certs/generate.sh
docker compose -f deploy/compose.yaml up --build nats fake-c2 phish-panel dns
```

Fake C2 is at `http://127.0.0.1:18081`; the non-functional phishing fixture is at `http://127.0.0.1:18082`. No real C2 behavior or credential collection is included.

## Skills and CI

Five composable skills in `.agents/skills/` share explicit handoffs:

- `contract-steward`: schema and persistence compatibility.
- `pipeline-builder`: test-first implementation; calls `contract-steward`.
- `mitigation-guardian`: signatures, approval, TTL, revocation; calls `contract-steward`.
- `lab-validator`: end-to-end safety evidence; calls `pipeline-builder` and `mitigation-guardian`.
- `hunt-operator`: authorized operations; calls contract, lab, and mitigation skills.

GitHub Actions runs Go, Python, Compose, and skill-composition checks. Skill CI validates metadata, the five-skill limit, unknown handoffs, and call cycles, then uploads the reviewed skill sources. It does not execute agent instructions or run live defensive actions.

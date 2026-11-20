# Contributing to Sentinel Hunter

Contributions are welcome when they make authorized defense **safer, more auditable, or more reversible**.

This project touches networks, DNS, firewall state, and signed authorization. A change that looks harmless in isolation can weaken a trust boundary. Read the safety contract before writing code.

## Read before you write

At minimum:

- [`README.md`](README.md) — project shape and current honest status
- [`spec/SCOPE.md`](spec/SCOPE.md) — the authorization contract
- [`spec/ARCHITECTURE.md`](spec/ARCHITECTURE.md) — service boundaries and scaffold limit
- [`docs/05-safety-and-trust-model.md`](docs/05-safety-and-trust-model.md) — how the human gate is enforced
- [`docs/06-engineering-process.md`](docs/06-engineering-process.md) — test-first development and skill handoffs
- [`CLAUDE.md`](CLAUDE.md) — local commands and critical safety invariants

If your change touches a schema, a signature payload, persistence, or a trust boundary, also read [`spec/STATE.md`](spec/STATE.md), both files in [`spec/DECISIONS/`](spec/DECISIONS/), and the relevant [contract-steward skill](.agents/skills/contract-steward/SKILL.md).

## Non-negotiable boundaries

A pull request will not be accepted if it:

- scans or connects to infrastructure without an explicit authorized scope;
- widens scope through code or configuration without evidence and an authorization owner;
- publishes `unchanged` observations across the AI boundary;
- sends a broad action to a backend without a named approval and valid Ed25519 signature;
- permits a TTL of zero, a negative TTL, or more than 24 hours;
- drops a failed revocation instead of retaining it for retry;
- weakens, skips, or rewrites a safety test merely to make a change pass;
- adds credential collection, payload execution, beaconing, or attribution from a heuristic score;
- commits private keys, generated certificates, Badger data, checkpoints, credentials, or real target lists.

When ownership is uncertain or a false positive is possible, choose **observation or early revocation**. Reversible defense takes precedence over campaign coverage.

## Set up the repository

### Requirements

- Go **1.27.x**
- Python **3.12**
- [`uv`](https://docs.astral.sh/uv/)
- Docker with Compose for the optional lab

```bash
git clone https://github.com/sensai-0x0/cyber-oss.git
cd cyber-oss
go mod download
uv sync --frozen --python 3.12
```

Do not use real infrastructure for development. The loopback lab is the default target:

```bash
./deploy/certs/generate.sh
docker compose -f deploy/compose.yaml up --build nats fake-c2 phish-panel dns
```

The lab is intentionally inert: the fake C2 has no command execution, and the phishing fixture rejects every POST. Keep it that way.

## Make a change

1. **Open or find an issue** for anything larger than a typo. For a trust-boundary change, describe the safety impact before writing code.
2. **Create one focused branch** from the current development branch:
   - `feat/<short-name>`
   - `fix/<short-name>`
   - `docs/<short-name>`
3. **Write the failing test first.** Use table-driven tests in Go and focused pytest cases in Python. Fake clocks, DNS, nftables, NATS, and network probes.
4. **Implement the smallest vertical slice.** Preserve O(1) cursors, bounded queues, novelty suppression, signature verification, and TTL enforcement.
5. **Run focused tests**, then the full verification pipeline below.
6. **Use Conventional Commit messages**, for example `fix(enforcerd): retain action when revoke fails`.
7. **Open one pull request per feature.** Do not mix refactors, formatting sweeps, and behavior changes.

### Contract changes

`schemas/delta_event.v1.json` and `schemas/mitigation_action.v1.json` are cross-language contracts. Do not change a required field or its meaning in place.

A contract change requires:

- a new schema version;
- matching Go and Pydantic models;
- updated compatibility tests;
- a recorded decision in [`spec/DECISIONS/`](spec/DECISIONS/);
- review of the contract boundary before implementation proceeds.

The canonical signing payload excludes only `signature`. Every authorization-relevant field must remain covered.

## Verification pipeline

Run all of this before requesting review:

```bash
test -z "$(gofmt -l hunterd enforcerd operator)"
go vet ./...
go test -race ./...

uv run ruff check orchestrator .github/scripts
uv run pytest
uv run pytest .github/scripts/tests
uv run python .github/scripts/validate_skills.py .agents/skills

docker compose -f deploy/compose.yaml config --quiet
```

What each step proves:

| Command | Proves |
|---|---|
| `gofmt` check | Go sources are formatted |
| `go vet` | Go static checks pass |
| `go test -race` | Go behavior and concurrency tests pass |
| `ruff check` | Python lint and import order pass |
| `pytest` | Orchestrator behavior passes |
| `pytest .github/scripts/tests` | Skill validator behavior passes |
| `validate_skills.py` | Metadata, five-skill ceiling, handoffs, and acyclicity hold |
| `docker compose config` | The lab manifest parses; **not** that the lab works end to end |

If your change touches runtime wiring, deployment, signing, or revocation, add lab evidence or request `lab-validator` review. Configuration parsing alone is not end-to-end evidence.

## Pull request checklist

Include these in the pull request description:

- [ ] Problem statement and intended behavior
- [ ] Safety impact: scope, approval, TTL, revocation, or data boundary
- [ ] Tests added or updated, and why they would fail before the change
- [ ] Full verification output
- [ ] Documentation or decision record updated when behavior or contracts changed
- [ ] No secrets, generated state, real target data, or private keys
- [ ] The change remains bounded and reversible

For a safety-critical fix, describe how the new test demonstrates the failure mode rather than only asserting the desired outcome.

## Review and disclosure

Maintainers review for correctness, contract compatibility, test quality, and safety-boundary preservation. A passing CI run is necessary but not sufficient; the trust properties must still hold under adversarial reasoning.

**Do not open a public issue for an unpatched vulnerability.** Use the repository's private vulnerability reporting channel under **Security → Report a vulnerability**. If that channel is unavailable, contact a maintainer privately before disclosing details.

## License

By contributing, you agree that your contribution is licensed under the [MIT License](LICENSE.md).

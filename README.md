<div align="center">

<pre>
 ####   ######  #    #  ######  ######  #    #  ######  #
#       #       ##   #    ##      ##    ##   #  #       #
 ####   #####   # #  #    ##      ##    # #  #  #####   #
     #  #       #  ## #    ##      ##    #  ## #  #       #
####   ######  #    #    ##    ######  #    #  ######  ######

         #    #  #    #  #    #  ######  ######  #####
         #    #  #    #  ##   #    ##    #       #    #
         ######  #    #  # #  #    ##    #####   #####
        #    #  #    #  #  ## #    ##    #       #  #
         #    #   ####   #    #    ##    ######  #   #
</pre>

**See the staging. Own the response. Leave nothing permanent.**

**Status:** authoritative · **Derived from:** `spec/`, the tree · **Updated:** 2026-11-21

[![Status](https://img.shields.io/badge/STATUS-ENGINEERING_SCAFFOLD-7c2d12?style=flat-square&labelColor=111111)](#-what-is-real-today)
[![Authorization](https://img.shields.io/badge/AUTHORIZATION-EXPLICIT_ONLY-a16207?style=flat-square&labelColor=111111)](#-safety-boundary)
[![Response](https://img.shields.io/badge/RESPONSE-NAMED_%2B_SIGNED-1d4ed8?style=flat-square&labelColor=111111)](#-the-invariants)
[![TTL](https://img.shields.io/badge/TTL-%E2%89%A4_24_HOURS-7f1d1d?style=flat-square&labelColor=111111)](#-the-invariants)
[![License](https://img.shields.io/badge/LICENSE-MIT-374151?style=flat-square&labelColor=111111)](LICENSE.md)

[🚨 Safety](#-safety-boundary) · [🛰️ Architecture](#-architecture) · [⚡ Quickstart](#-quickstart) · [🧪 Verify](#-verification) · [📡 Current state](#-what-is-real-today) · [🤝 Contribute](CONTRIBUTING.md) · [⚖️ License](#-license)

</div>

Sentinel Hunter is a control system for **explicitly authorized pre-attack infrastructure hunting**. It watches your own address space for new or changed staging signals, clusters and scores what it finds, and routes consequential mitigation to a named human for a **signed, time-bounded, reversible** decision.

It is not a scanner with an auto-block reflex. It is the chain between *“something changed in our address space”* and *“someone with authority decided what to do about it.”*

> [!WARNING]
> This software connects to targets. Active use requires a recorded hunt scope, exclusions, rate limits, time window, and authorization owner. Never point it at third parties. Start in the loopback-only lab and read [`spec/SCOPE.md`](spec/SCOPE.md).
>
> **Scope enforcement is not yet implemented.** The probe runners dial whatever target they are given, and no component holds an authorized-target allowlist. Invariants I1–I5 in [.agents/orchestration/invariants.md](.agents/orchestration/invariants.md) therefore have no enforcement point today; see [spec/DECISIONS/0003-architecture-authority.md](spec/DECISIONS/0003-architecture-authority.md) consequence 4. Until that lands, probe only the loopback lab.

---

## 🧭 What it does

<table>
<tr>
<td width="25%" align="center"><strong>🛰️ Observe</strong><br/><sub>Walk authorized ranges in keyed, resumable order.</sub></td>
<td width="25%" align="center"><strong>🕳️ Suppress</strong><br/><sub>Let only novel or mutated signal escape.</sub></td>
<td width="25%" align="center"><strong>⚖️ Propose</strong><br/><sub>Cluster, score, and explain a bounded policy.</sub></td>
<td width="25%" align="center"><strong>🔏 Enforce</strong><br/><sub>Apply only signed action. Revoke at expiry.</sub></td>
</tr>
</table>

<details>
<summary><strong>🌐 The full loop — click to open</strong></summary>

<br/>

```text
                     AUTHORIZED SCOPE
                            │
                            ▼
                 ┌─────────────────────┐
                 │       hunterd       │  Go
                 │ keyed O(1) cursor   │
                 │ tlsx / httpx probe  │
                 │ Badger novelty gate │
                 └──────────┬──────────┘
                            │  novel / mutated only
                            │  bounded NDJSON / NATS
                            ▼
                 ┌─────────────────────┐
                 │     orchestrator    │  Python / LangGraph
                 │ cluster → score     │
                 │ → policy            │
                 │ one refinement max  │
                 └──────────┬──────────┘
                            │  broad action?
                            ▼
                 ┌─────────────────────┐
                 │    human interrupt  │
                 │  named decision     │
                 └──────────┬──────────┘
                            │  approve or reject
                            ▼
                 ┌─────────────────────┐
                 │       operator      │  Go
                 │ Ed25519 signature   │
                 │ TTL ≤ 24h           │
                 └──────────┬──────────┘
                            │  signed action only
                            ▼
                 ┌─────────────────────┐
                 │      enforcerd      │  Go
                 │ verify → apply      │
                 │ expire → revoke     │
                 └─────────────────────┘
```

```text
repo/
├── CLAUDE.md                   # Contributor commands, safety invariants, skill entry points
├── .agents/                    # Agent instructions: go.md, python-langgraph.md,
│                               #   outline-method.md, evals/, orchestration/, skills/
├── .claude/                    # Orchestrator agent and its skills
├── spec/                       # Normative: ARCHITECTURE, SCOPE, STATE, PRD, DECISIONS/
├── plans/                      # Intent: phase plans, data model, interface contracts
├── docs/                       # Narrative explanation and roadmap
├── schemas/                    # Versioned DeltaEvent + MitigationAction JSON contracts
├── hunterd/                    # Go adapters, Feistel cursor, Badger store/novelty, NDJSON publish
├── orchestrator/               # Python graph.py, cluster.py, score.py, policy.yaml + tests
├── enforcerd/                  # Go RPZ/nft adapters, Ed25519 verification, TTL lifecycle
├── operator/                   # Approval/rejection and key-generation CLI
├── deploy/                     # Loopback Compose, NATS configuration, local cert generator
└── lab/                        # Harmless fake C2, disabled phishing panel, DNS/firewall targets
```

</details>

## 🛡️ Safety boundary

Human authority is a **technical boundary**, not a prompt instruction.

| Component | Owns | Structurally cannot |
|---|---|---|
| `hunterd` | Collection, resumable cursors, novelty suppression | Decide what the signal means |
| `orchestrator` | Clustering, scoring, policy proposal | Sign, apply, or revoke anything |
| `operator` | A named human's approval or rejection | Apply a backend change |
| `enforcerd` | Signature verification, enforcement, expiry | Accept an unsigned or broad-unapproved action |

<details>
<summary><strong>🔐 The invariants — click to open</strong></summary>

<br/>

- **Scope is checked before any packet leaves.** Out-of-scope means no connection, not a warning.
- **Only `novel` and `mutated` events cross the AI boundary.** `unchanged` never leaves the daemon.
- **Broad actions require a named operator and a valid Ed25519 signature.**
- **Every action expires in at most 24 hours.** There are no permanent rules.
- **Revocation needs no new approval.** Undoing an action must always be cheaper than imposing one.
- **A failed revoke stays visible and retryable.** “TTL elapsed” is not the same fact as “rule removed.”
- **The model may be uncertain. It is never allowed to be unaccountable.**

</details>

## 🛰️ Architecture

Four narrow services, four separate capabilities:

```text
hunterd       collects and suppresses noise
orchestrator  interprets and proposes
operator      converts a human decision into a signature
enforcerd     verifies, applies, and cleans up
```

No service is trusted with more than its own job. Compromising the triage layer yields a bad proposal — not a firewall rule. See [`spec/ARCHITECTURE.md`](spec/ARCHITECTURE.md) and [ADR 0001](spec/DECISIONS/0001-service-boundaries.md).

<details>
<summary><strong>📦 Repository map — click to open</strong></summary>

<br/>

```text
.
├── hunterd/       Go: Feistel cursor, Badger store, probes, NDJSON
├── orchestrator/  Python: LangGraph cluster → score → policy
├── operator/      Go: approve / reject and Ed25519 signing
├── enforcerd/     Go: verification, nftables, RPZ, TTL lifecycle
├── schemas/       Versioned DeltaEvent and MitigationAction contracts
├── deploy/        Loopback Compose lab and local TLS generation
├── lab/           Harmless fake C2, inert phishing fixture, DNS targets
├── spec/          Architecture, scope, state, and decision records
├── docs/          Product, safety, process, and roadmap material
└── .agents/       Development skills, evals, and orchestration context
```

</details>

## ⚡ Quickstart

<details>
<summary><strong>🛠️ Requirements — click to open</strong></summary>

<br/>

- Go **1.27.x**
- Python **3.12** and [`uv`](https://docs.astral.sh/uv/)
- Docker with Compose **only for the optional lab**

</details>

```bash
git clone https://github.com/sensai-0x0/cyber-oss.git
cd cyber-oss
go mod download
uv sync --frozen --python 3.12
uv run pytest
uv run ruff check orchestrator .github/scripts
uv run python .github/scripts/validate_skills.py .agents/skills
uv run python .github/scripts/validate_outlines.py
docker compose -f deploy/compose.yaml config --quiet
```

Run the isolated lab:

```bash
./deploy/certs/generate.sh
docker compose -f deploy/compose.yaml up --build nats fake-c2 phish-panel dns
```

The fake C2 listens at `http://127.0.0.1:18081`; the non-functional phishing fixture is at `http://127.0.0.1:18082`. Every host port is bound to loopback. No real C2 behavior or credential collection exists. See [`lab/README.md`](lab/README.md).

> [!IMPORTANT]
> `docker compose config` proves the manifest parses. It does not prove the lab works end to end. Full pipeline evidence must come from the [lab validator](.agents/skills/lab-validator/SKILL.md).

## 🧪 Verification

Run this before opening a pull request:

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

Tests must use fakes for network probes, DNS, nftables, NATS, and time. The build pipeline never requires a live target or a live firewall.

## 📡 What is real today

<table>
<tr>
<td width="50%" valign="top">

**✅ Implemented and tested**

- Keyed, resumable CIDR cursor
- Badger novelty and mutation gate
- TLS/HTTP observation normalization
- Cluster → score → policy graph
- Native human interrupt for broad actions
- Ed25519 decision signing
- nftables and RPZ enforcement adapters
- TTL expiry with revoke retry
- Loopback-only adversarial lab

</td>
<td width="50%" valign="top">

**⏳ Not yet production-ready**

- Durable NATS consumer
- Durable LangGraph checkpointer
- Live Certificate Transparency client
- Headscale/Tailscale adapter
- Centralized enforcer transport
- Recorded end-to-end lab evidence
- Production approval surface

</td>
</tr>
</table>

This is an **engineering scaffold**, not a finished platform. The line between “tested primitive” and “integration work remaining” is documented in [`spec/ARCHITECTURE.md`](spec/ARCHITECTURE.md#current-scaffold-boundary). The intended path forward is in [`docs/roadmap.md`](docs/roadmap.md).

<details>
<summary><strong>📚 Read deeper — click to open</strong></summary>

<br/>

| Document | What it answers |
|---|---|
| [`spec/SCOPE.md`](spec/SCOPE.md) | What may be touched, and under what authorization |
| [`spec/ARCHITECTURE.md`](spec/ARCHITECTURE.md) | Service boundaries and the current scaffold limit |
| [`docs/05-safety-and-trust-model.md`](docs/05-safety-and-trust-model.md) | Why the human gate survives a compromised agent |
| [`docs/02-product-overview.md`](docs/02-product-overview.md) | End-to-end workflow and current capabilities |
| [`docs/06-engineering-process.md`](docs/06-engineering-process.md) | Skills, evals, and the test-first development loop |
| [`docs/roadmap.md`](docs/roadmap.md) | What gets built next, in what order |

</details>

## 🤝 Contributing

Contributions are welcome where they make authorized defense **safer, more auditable, or more reversible**. Start with [`CONTRIBUTING.md`](CONTRIBUTING.md), then read [`spec/SCOPE.md`](spec/SCOPE.md) before touching anything that can reach a network or a backend.

Do not open a public issue for an unpatched safety vulnerability. Use the repository's private vulnerability reporting channel.

## ⚖️ License

[MIT](LICENSE.md) © 2026 Metacogna.

<div align="center">

<br/>

<sub>**No permanent rules. No unsigned actions. No unowned decisions.**</sub>

</div>

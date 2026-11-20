# CI/CD Guide — GitHub Actions in This Repository

This is a practical guide to the continuous integration setup already running in this repository, and the best practices it demonstrates. It uses Sentinel Hunter's own `.github/workflows/ci.yml` and `.github/workflows/skills.yml` as the worked template — every principle below is illustrated with the actual YAML already in the repo, not a hypothetical example. If you're adding a new workflow or a new job to an existing one, read this first.

## Why CI matters here specifically

This repository's whole safety argument (`docs/05-safety-and-trust-model.md`) is that consequential behavior is gated by *code*, not by a policy someone has to remember to enforce. CI is the same idea applied to the development process itself: a merge to `main` should be blocked by a failing test automatically, not by someone remembering to run `go test` first. Treat CI as part of the trust boundary, not as a convenience.

## 1. What's actually running today

| Workflow | Trigger | Jobs | What it proves |
|---|---|---|---|
| `ci.yml` | `pull_request`, `push` to `main` | `go`, `python`, `lab-config` | Go formatting/vet/race-tested build; Python lint + test; the lab's `docker-compose.yaml` parses |
| `skills.yml` | `pull_request`/`push` to `main`, path-filtered to `.agents/skills/**` and the validator script | `validate` | Skill metadata is well-formed, the five-skill ceiling holds, the skill call graph has no cycles, and the reviewed skill set is packaged as a build artifact |

Both workflows set `permissions: contents: read` at the top level — the default GitHub Actions grants is broader than most jobs need, and neither of these workflows writes to the repo, uploads a release, or needs any write scope. **Declare the narrowest `permissions` block a workflow needs, at the workflow level, and widen only a specific job if it specifically needs to** (e.g., a future release job needing `contents: write`). This is the same least-privilege instinct the product code applies to `operator`/`enforcerd` — least privilege isn't just a runtime property.

## 2. Mirror local commands exactly — never invent a CI-only step

Every step in `ci.yml` and `skills.yml` is a command a developer can run on their own machine, verbatim, from `README.md`'s "Verify locally" block:

```bash
go test ./...
uv sync --frozen --python 3.12
uv run ruff check orchestrator .github/scripts
uv run pytest
uv run python .github/scripts/validate_skills.py .agents/skills
docker compose -f deploy/compose.yaml config --quiet
```

This is deliberate and worth preserving as new workflows are added: **if CI runs a check a contributor can't reproduce locally, debugging a red build becomes guesswork.** When you add a CI step, add (or confirm the existence of) the exact local-equivalent command in `README.md` first. If a check genuinely can't run locally (e.g., something that needs a GitHub-hosted secret), say so explicitly in the workflow's `name:` or a comment, because it's the exception, not the pattern.

## 3. Job separation and why `ci.yml` has three jobs, not one

`ci.yml` splits `go`, `python`, and `lab-config` into independent jobs rather than one long script. This buys three things:

- **Parallelism.** GitHub Actions runs independent jobs concurrently by default; a Python lint failure doesn't wait behind a slow Go race-detector build.
- **Fault isolation.** A failing `lab-config` job tells you immediately that the problem is in `deploy/compose.yaml`, not somewhere in 40 lines of combined log output you have to scroll through to find.
- **Least privilege per job.** If a future job needs a capability the others don't (say, a Docker registry credential to actually start the lab rather than just validate its Compose file), that credential scopes to one job, not the whole workflow.

**Rule of thumb**: a new job boundary earns its keep when the work is logically independent (different language, different subsystem, different failure mode) and doesn't need another job's output. Don't split for its own sake — `go vet` and `go test -race ./...` stay in the same `go` job here because splitting them would double the Go toolchain setup cost for no isolation benefit (a `go vet` failure and a `go test` failure are both "the Go job is red," which is the right granularity).

## 4. Pin and cache your toolchain setup — but know what you're trusting

Every job starts with `actions/checkout@v4` and a language-specific setup action (`actions/setup-go@v5`, `astral-sh/setup-uv@v6`), each pinned to a major-version tag.

- **Caching**: `setup-go` is given `cache-dependency-path: go.sum` and `setup-uv` is given `enable-cache: true` — both cache the dependency download step keyed on the lockfile, so a CI run that hasn't changed `go.sum`/`uv.lock` skips re-downloading the world. This is why `go mod download` and `uv sync --frozen` stay fast in practice even though they run on every push.
- **Version pinning trade-off**: `@v4`/`@v5`/`@v6` tags are *mutable within a major version* — the action's maintainer can push fixes under the same tag. That's convenient (you get security patches without a version bump) but means the exact bytes you run aren't fully reproducible build-to-build. For most CI steps here (checkout, language setup, artifact upload) that trade-off is fine — these are well-maintained, widely-used actions from GitHub itself or the language's own tooling org (`astral-sh` for `uv`). **If you ever add a third-party action from a less-established publisher, pin it to a full commit SHA instead of a tag**, and note why in a comment — that's the supply-chain-conscious version of the same "don't trust unverified input" instinct this repo applies to `DeltaEvent` attributes.

## 5. Scope triggers tightly — `skills.yml`'s path filters

```yaml
on:
  pull_request:
    paths:
      - ".agents/skills/**"
      - ".github/scripts/**"
      - ".github/workflows/skills.yml"
      - "pyproject.toml"
      - "uv.lock"
```

`skills.yml` only runs when a change actually touches the skill definitions, the validator script, the workflow file itself, or a dependency lockfile that could change how the validator runs. A docs-only PR (like the one that added this guide) does not trigger it. **This matters for signal, not just minutes billed**: a workflow that runs on every PR regardless of relevance trains contributors to ignore its result ("it's always green, it's unrelated to my change"). A tightly-scoped trigger means a red `skills` check is *always* informative.

`ci.yml` deliberately has **no** path filter — Go and Python changes can land anywhere in the tree, and a docs-only PR genuinely should still see those jobs run (cheaply, since nothing changed to invalidate their caches) so that "all required checks passed" stays a trustworthy gate regardless of what kind of PR it is. The difference is intentional: filter when a workflow is narrow and expensive-if-irrelevant; don't filter when a workflow is the general correctness gate.

## 6. Concurrency control

```yaml
concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

If you push three commits to the same branch in quick succession, GitHub Actions would otherwise queue and eventually run CI three times. This config cancels an in-flight run for the same branch when a newer one starts — you only ever wait on the latest commit's result, not a backlog. Add this to any new workflow that runs on `push` to a branch that gets rapid iterative pushes (feature branches, PR branches). It's less useful on something that only runs on `push: branches: [main]`, since merges to a protected branch are naturally rate-limited by review, but it's harmless there too.

## 7. Artifacts, and failing loudly when there's nothing to upload

```yaml
- name: Package reviewed skill sources
  uses: actions/upload-artifact@v4
  with:
    name: sentinel-hunter-skills
    path: .agents/skills
    if-no-files-found: error
```

`skills.yml` doesn't just validate the skills — it packages the *validated* sources as a build artifact, so anything downstream (a release process, a manual audit) pulls from something that's already passed CI, not from whatever happens to be on someone's branch. `if-no-files-found: error` is the detail worth copying into any future artifact-upload step: the default behavior (`warn`) lets a workflow report green even when the thing it claims to have packaged doesn't exist. Fail the job if the artifact isn't there — a silent empty artifact is worse than a loud missing one.

## 8. What CI validates vs. what it doesn't — read the fine print

`lab-config`'s job is named for exactly what it does: `docker compose -f deploy/compose.yaml config --quiet` validates that the Compose file *parses* and its references resolve. It does **not** start the lab, does not prove the fake-C2/phishing/DNS fixtures actually behave correctly together, and does not prove an end-to-end hunt→mitigate→expire cycle works. `spec/ARCHITECTURE.md` says this explicitly: *"CI does not claim end-to-end lab success merely because Compose parses."* This is the single most important habit this guide wants to pass on: **name a CI job for exactly what it checks, and don't let a green check mark imply more than that.** When Phase 5 of `plans/planv1.0/PHASE5.md` (end-to-end verification against the loopback lab) lands, it should get its own, honestly-named job — `lab-e2e` or similar — rather than being folded into `lab-config`'s existing green checkmark.

## 9. Required checks are how "no merge without approval" becomes real

This repository's process documents (`CLAUDE.md`, `.agents/evals/orchestrator.md`) state that nothing merges to `main` without explicit human approval. GitHub Actions results are what make that enforceable rather than aspirational: configure branch protection on `main` to **require the `go`, `python`, `lab-config`, and `validate` (skills) checks to pass before a PR can merge**, and require at least one human review. This is the exact same design pattern as the product's own trust boundary (`docs/05-safety-and-trust-model.md`) — review and verification as a technical gate the platform enforces, not a step someone has to remember. If you're setting this up for the first time: GitHub repo settings → Branches → branch protection rule for `main` → "Require status checks to pass before merging" (select the four job names above) → "Require a pull request before merging" with at least one required approval.

## 10. Secrets and forked-PR safety

Neither workflow currently uses a secret, which is the safest possible state and worth preserving as long as it's true: **a workflow with no secrets can't leak one.** If a future job needs one (a registry credential, a notification webhook), keep two things in mind:

- Secrets are not available to workflows triggered by `pull_request` from a fork by default — this is intentional, since a forked PR's code is untrusted. Don't "fix" this by switching to `pull_request_target` unless you specifically understand that trigger runs with the base repo's permissions against the PR's *code*, which is a real privilege-escalation risk if the job then executes anything from the PR branch. If you need secrets against fork PRs, use an explicit, reviewed `workflow_run`-based pattern, not a shortcut.
- Scope every secret to the narrowest job and the narrowest permission that secret's provider allows. A secret available to the whole workflow is available to every step in every job in it, including ones that don't need it.

## 11. Extending this setup safely

- **New job vs. new workflow file**: a new job in `ci.yml` if it's part of "is this change correct" (another language, another lint pass). A new workflow file if it's a different *kind* of gate with its own trigger shape — `skills.yml` is separate from `ci.yml` precisely because its trigger is path-filtered and its own concern (skill governance), not because Python/skills code couldn't technically run in the same file.
- **Respect existing hard caps.** `skills.yml`'s validator enforces a five-skill ceiling (`.github/scripts/validate_skills.py`) as a matter of project policy (`.agents/evals/constitution.md`'s protected clauses) — a CI change that works around that check rather than through it defeats the point of having it be code-enforced.
- **New safety-critical code gets a new, honestly-scoped job**, not a line added to an existing one. When `plans/planv1.0/PHASE1.md`'s scope-enforcement and rate-limiting work lands, its fuzz test (`E03` in `.agents/evals/evals.md`) belongs in its own job with a name that says what it proves — not buried as one more step inside the general `go` job where a contributor skimming job names would never know it exists.
- **Before adding any new workflow, run its steps locally first**, per §2. If you can't make the steps work locally, you're not ready to make them CI's problem either.

# Backlog register

**Status:** authoritative · **Derived from:** [../reviews/README.md](../reviews/README.md) conventions, session records on `worktree-mellow-launching-lantern` · **Updated:** 2026-11-21

Open work items, one per line, each stamped with the UTC datetime it was registered. The register
is append-only: a line is edited only to change its `status` field, never to reword or delete
history.

This folder is a **record, not an authority.** A backlog line never tells anyone what to build —
it points at the document that does. Where a line disagrees with `spec/`, `plans/` or the code, the
authority ladder in [../../.agents/outline-method.md](../../.agents/outline-method.md) §1 decides,
and the line is what gets corrected.

## Line grammar

```
<datetime> · <id> · <kind> · <severity> · <status> · <one sentence, ending in the path or document that holds it>
```

Fields are separated by ` · ` so a line stays greppable and splits cleanly on the separator. The
sentence carries no newline, so one item is always one line.

| Field | Values |
|---|---|
| `datetime` | ISO 8601 UTC, seconds precision, as `date -u +%Y-%m-%dT%H:%M:%SZ` prints it. The moment of registration, never backdated |
| `id` | `BL-NNNN`, allocated in order, never reused. Distinct from the `SHR-NNN` ids in [../reviews/README.md](../reviews/README.md), which are code-review findings |
| `kind` | `bug`, `task`, `question`, `decision`, `debt` |
| `severity` | `critical`, `high`, `medium`, `low` — same meanings as the review register's triage rules |
| `status` | `open`, `accepted`, `in_progress`, `resolved`, `wont_fix` — same vocabulary as the review register |

## Appending an entry

```bash
printf '%s · BL-%04d · %s · %s · %s · %s\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" 11 task medium open \
  "One sentence, ending in the document that owns it, such as plans/agent-graph.md G0.1" \
  >> docs/backlog/backlog.md
```

Replace `11` with the next free number. A `·` inside the sentence is replaced with a comma, since
it is the separator.

## Which register an item belongs in

Four registers, four jobs. Putting an item in the wrong one is how it stops being found.

| Register | Holds | Written when |
|---|---|---|
| `docs/backlog/backlog.md` | Open work: bugs, tasks, unanswered questions, parked decisions | Something is known to need doing and is not yet scheduled in a phase |
| [../reviews/](../reviews/) | Code-review findings with file, lines, impact and recommendation | A review registers a specific defect at a specific location |
| [../../.agents/orchestration/obs/errors.md](../../.agents/orchestration/obs/errors.md) | Failures an agent hit while running, with the suggested fix | A tool, command or environment failed mid-session |
| [../../.agents/orchestration/obs/state-log.md](../../.agents/orchestration/obs/state-log.md) | Decisions, derived component status, plan order | A decision is taken or state changes |

An item may legitimately appear in two registers — a bug an agent hit while running belongs in both
`errors.md` and here — but it keeps one owner. The backlog line names the other record rather than
restating it.

## Closing an entry

Change `status` in place and nothing else. The datetime stays as the registration moment; the
evidence for the close goes where the work landed — a commit, a test name, a decision-log line in
[../../spec/STATE.md](../../spec/STATE.md). A line marked `resolved` with no such evidence
elsewhere should be reopened rather than trusted, following the same rule as the task checkboxes in
[../../plans/PHASESv1.0.md](../../plans/PHASESv1.0.md) §4.

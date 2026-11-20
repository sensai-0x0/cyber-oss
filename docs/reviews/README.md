# Review register

Project-wide findings are stored as **JSON Lines (JSONL)**: one complete, independently addressable review record per line. JSONL keeps reviews diffable, machine-validatable, and easy to triage without burying findings in prose.

Files use `YYYY-MM-DD-<scope>.jsonl`. A same-day Markdown companion may summarize counts and priorities, but the JSONL record is canonical. `schema.json` defines the machine-validatable v1 entry contract; every non-empty line and every unique `review_id` should be validated before registration is complete.

## Record schema

Every line must be a JSON object with these fields:

| Field | Required | Meaning |
|---|---|---|
| `schema_version` | yes | Register format version; currently `v1`. |
| `review_id` | yes | Stable `SHR-NNN` identifier; never reuse IDs. |
| `registered_at` | yes | ISO 8601 registration date. |
| `branch` | yes | Branch on which the finding was registered. |
| `commit` | yes | Reviewed commit, or `uncommitted` when appropriate. |
| `severity` | yes | `critical`, `high`, `medium`, or `low`. |
| `category` | yes | Such as `correctness`, `security`, `durability`, `agent-design`, or `maintainability`. |
| `confidence` | yes | Reviewer's confidence: `high`, `medium`, or `low`. |
| `status` | yes | `open`, `accepted`, `in_progress`, `resolved`, or `wont_fix`. |
| `file` | yes | Repository-relative primary location. |
| `lines` | yes | Object with 1-based inclusive `start` and `end`; use related locations in `finding`. |
| `principle` | yes | Engineering or project invariant supporting the finding. |
| `finding` | yes | Specific, evidence-backed concern; no vague style preferences. |
| `impact` | yes | Plausible operational or maintenance consequence. |
| `recommendation` | yes | Concrete next action and required verification. |
| `deterministic_alternative` | yes | Deterministic replacement for agent/model behavior, or `null` when not applicable. |
| `verification` | yes | Evidence used: static review, named test, replay, or `not_verified`. |

Example (one physical line in a `.jsonl` file):

```json
{"schema_version":"v1","review_id":"SHR-001","registered_at":"2026-11-21","branch":"review/project-triage","commit":"b06074f","severity":"medium","category":"agent-design","confidence":"high","status":"open","file":"orchestrator/example.py","lines":{"start":10,"end":20},"principle":"KISS: deterministic policy belongs in code","finding":"A fixed threshold is delegated to a model.","impact":"Identical inputs can produce inconsistent policy decisions.","recommendation":"Replace the model call with a pure function and table-driven boundary tests.","deterministic_alternative":"Load the threshold from policy.yaml and compare it in Python.","verification":"static_review"}
```

## Triage rules

- Reserve `critical` for actively exploitable security failures, data loss, or broken safety invariants.
- Use `high` for likely correctness, recovery, or authorization failures; use `medium` for meaningful complexity and missing safeguards; use `low` for localized maintainability issues.
- Do not register agent-versus-deterministic concerns as a generic dislike of agents. Identify the exact decision, why deterministic logic is sufficient, and where contextual judgment is actually needed.
- Resolve a finding only with a code reference and verification evidence. Keep the original ID and append a dated resolution note in `verification` rather than deleting history.

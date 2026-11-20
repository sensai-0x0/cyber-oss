"""Append-only, hash-chained evidence ledger (plans/ml-add Phase 2).

The ledger is write-only from the graph's point of view: the one node that touches it
only appends, no node reads it, and a failed write never changes a decision.
Storage is a Python-owned SQLite file (ADR 0004).

Limit: the chain detects an edit or a deleted record in the middle. Truncating the
newest records leaves a valid shorter chain, so the head hash must be kept elsewhere
to detect that. Anchoring the head is out of scope for this phase.
"""

import hashlib
import json
import logging
import sqlite3
import sys
from collections.abc import Callable, Sequence
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Protocol

from orchestrator.policy import POLICY_FILE, load_policy
from orchestrator.provenance import classify_evidence
from orchestrator.state import DeltaEvent, GraphState

LOGGER = logging.getLogger(__name__)
GENESIS = "0" * 64
SCORER_ID = "heuristic.badlist.v1"


def canonical_json(value: Any) -> bytes:
    """Sorted keys, no whitespace, UTF-8. Fixed by a golden-file test."""
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def record_hash(record: dict[str, Any]) -> str:
    body = {key: value for key, value in record.items() if key != "record_hash"}
    return hashlib.sha256(canonical_json(body)).hexdigest()


class LedgerWriter(Protocol):
    """What the graph is allowed to do with a ledger: append, and nothing else."""

    def append(self, fields: dict[str, Any]) -> dict[str, Any]: ...


class Ledger:
    def __init__(self, path: str | Path, clock: Callable[[], datetime] | None = None):
        self.path = str(path)
        self._clock = clock or (lambda: datetime.now(UTC))
        self.failed_event_ids: list[str] = []
        with self._connect() as connection:
            connection.execute(
                "CREATE TABLE IF NOT EXISTS records ("
                "seq INTEGER PRIMARY KEY, record_hash TEXT NOT NULL, body TEXT NOT NULL)"
            )

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self.path, isolation_level=None)
        connection.execute("PRAGMA journal_mode=WAL")
        return connection

    def append(self, fields: dict[str, Any]) -> dict[str, Any]:
        """Chain a new record onto the head and return it. The caller supplies no hashes."""
        connection = self._connect()
        try:
            connection.execute("BEGIN IMMEDIATE")
            head = connection.execute(
                "SELECT seq, record_hash FROM records ORDER BY seq DESC LIMIT 1"
            ).fetchone()
            seq, prev_hash = (head[0] + 1, head[1]) if head else (0, GENESIS)
            record = {
                **fields,
                "seq": seq,
                "prev_hash": prev_hash,
                "recorded_at": self._clock().isoformat(),
                "challengers": fields.get("challengers", {}),
            }
            record["record_hash"] = record_hash(record)
            connection.execute(
                "INSERT INTO records (seq, record_hash, body) VALUES (?, ?, ?)",
                (seq, record["record_hash"], canonical_json(record).decode()),
            )
            connection.execute("COMMIT")
            return record
        except BaseException:
            if connection.in_transaction:
                connection.execute("ROLLBACK")
            raise
        finally:
            connection.close()

    def records(self) -> list[tuple[int, str, str]]:
        """Stored (seq, record_hash, body) rows in order. For verification and tooling only."""
        connection = self._connect()
        try:
            return connection.execute(
                "SELECT seq, record_hash, body FROM records ORDER BY seq"
            ).fetchall()
        finally:
            connection.close()


def verify_chain(ledger: Ledger) -> int | None:
    """Return the first bad sequence number, or None when the chain is intact."""
    expected_seq, expected_prev = 0, GENESIS
    for seq, stored_hash, body in ledger.records():
        if seq != expected_seq:
            return expected_seq  # a record was removed
        try:
            record = json.loads(body)
        except ValueError:
            return seq
        if (
            record.get("seq") != seq
            or record.get("prev_hash") != expected_prev
            or record.get("record_hash") != stored_hash
            or record_hash(record) != stored_hash
        ):
            return seq
        expected_seq, expected_prev = seq + 1, stored_hash
    return None


def build_record_fields(state: GraphState) -> dict[str, Any]:
    config = load_policy()
    event = DeltaEvent.model_validate(state["event"])
    return {
        "event": event.model_dump(mode="json"),
        "evidence": classify_evidence(event, config),
        "intel": state["intel"],
        "policy": state["policy"],
        "policy_sha256": hashlib.sha256(POLICY_FILE.read_bytes()).hexdigest(),
        "scorer_id": SCORER_ID,
        **(
            {"proposal_seq": state["proposal"]["ledger_seq"]}
            if state.get("proposal")
            else {}
        ),
    }


class Challenger(Protocol):
    """A shadow scorer. Its output goes into the ledger record and nowhere else."""

    name: str

    def observe(self, event: dict[str, Any]) -> dict[str, Any]: ...


def run_challengers(state: GraphState, challengers: Sequence[Challenger]) -> dict[str, Any]:
    """Score the same event with each challenger; an exception is recorded, never raised."""
    results: dict[str, Any] = {}
    for challenger in challengers:
        try:
            results[challenger.name] = challenger.observe(state["event"])
        except Exception as error:
            LOGGER.exception("challenger %s failed", challenger.name)
            results[challenger.name] = {"error": f"{type(error).__name__}: {error}"}
    return results


def make_propose_node(
    writer: LedgerWriter,
) -> Callable[[GraphState], dict[str, Any]]:
    """Record a mitigation proposal before an operator sees it (ADR 0005).

    Unlike the decision record, a failed write here is fatal to the proposal: after
    `ledger_append_attempts` tries the decision becomes `observe`, so nothing is
    left for an operator to sign without evidence behind it.
    """

    def propose(state: GraphState) -> dict[str, Any]:
        policy = state["policy"]
        if policy["action"] != "mitigate" or policy["approved"]:
            return {}
        attempts = load_policy()["ledger_append_attempts"]
        for attempt in range(1, attempts + 1):
            try:
                record = writer.append({**build_record_fields(state), "stage": "propose"})
            except Exception:
                LOGGER.exception("proposal write failed (attempt %d of %d)", attempt, attempts)
                continue
            return {
                "proposal": {
                    "ledger_seq": record["seq"],
                    "evidence_digest": record["record_hash"],
                }
            }
        return {
            "policy": {
                **policy,
                "action": "observe",
                "approved": False,
                "ttl_seconds": 0,
                "reason": "ledger unavailable; no evidence record for review",
            }
        }

    return propose


def make_record_node(
    writer: LedgerWriter, challengers: Sequence[Challenger] = ()
) -> Callable[[GraphState], dict[str, Any]]:
    """A node that appends one record and never changes state or a decision.

    Challengers run here, after the champion has decided, and write only into the record.
    """

    def record(state: GraphState) -> dict[str, Any]:
        try:
            fields = build_record_fields(state)
            fields["challengers"] = run_challengers(state, challengers)
            writer.append(fields)
        except Exception:  # a ledger fault must not alter or block the decision
            event_id = str(state.get("event", {}).get("event_id", "unknown"))
            LOGGER.exception("ledger write failed; run left unrecorded: %s", event_id)
            failed = getattr(writer, "failed_event_ids", None)
            if isinstance(failed, list):
                failed.append(event_id)
        return {}

    return record


def export_ndjson(ledger: Ledger) -> list[str]:
    """One canonical record per line, in sequence order, for read-only tools."""
    return [body for _, _, body in ledger.records()]


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if len(args) == 2 and args[0] == "export":
        if not Path(args[1]).is_file():
            print(f"no such ledger: {args[1]}", file=sys.stderr)
            return 2
        for line in export_ndjson(Ledger(args[1])):
            print(line)
        return 0
    if len(args) != 2 or args[0] != "verify":
        print("usage: python -m orchestrator.ledger verify|export PATH", file=sys.stderr)
        return 2
    if not Path(args[1]).is_file():
        print(f"no such ledger: {args[1]}", file=sys.stderr)
        return 2
    bad = verify_chain(Ledger(args[1]))
    if bad is None:
        print("ok")
        return 0
    print(f"first bad record: {bad}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())

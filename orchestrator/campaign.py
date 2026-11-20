"""Campaign belief scorer: a challenger that runs in shadow (plans/ml-add Phase 4).

Each campaign keeps a log-odds posterior built from evidence, with a half-life decay and a
cap per evidence class. A campaign is identified by the OBSERVED indicators its events
share (jarm, ja3, favicon), never by the target, so a campaign that moves between targets
keeps its identity. Producer-asserted attributes (ASSERTED) have a cap of zero.

The output of this module goes only to `ledger.challengers[scorer_id]`. It never reaches
`GraphState`, routing or any policy decision.

Known limits, stated plainly:
  * Weights and the prior are hand-set, not learned.
  * Infrastructure shared by many unrelated benign sites (a CDN) is indistinguishable here
    from one campaign reusing infrastructure; rarity weighting is a later idea (K3).
  * A campaign that rotates every indicator at once starts a new identity.
"""

import hashlib
import json
import math
import sqlite3
from datetime import datetime
from typing import Any

from orchestrator.policy import load_policy
from orchestrator.provenance import BADLIST_FIELDS, derive_badlist_hits
from orchestrator.state import DeltaEvent

SCORER_ID = "campaign.belief.v1"
DERIVED_HIT_LOG_ODDS = 1.4  # one badlist hit on an observed field
REPEAT_TARGET_LOG_ODDS = 0.5  # the same indicators seen on one more distinct target
ASSERTED_CAP = 0.0  # producer-asserted evidence can never move the posterior
MAX_TARGETS_KEPT = 64
SEEN_PER_CAMPAIGN = 50


def logit(probability: float) -> float:
    return math.log(probability / (1 - probability))


def sigmoid(value: float) -> float:
    return 1 / (1 + math.exp(-value))


def _seconds(observed_at: datetime) -> float:
    return observed_at.timestamp()


def _target_key(target: str) -> str:
    return hashlib.sha256(target.encode()).hexdigest()[:12]


def _decay(value: float, from_t: float, to_t: float, half_life_s: float) -> float:
    """Scale `value` from time `from_t` to `to_t`; a later `to_t` shrinks it by half-lives."""
    return value * 0.5 ** ((to_t - from_t) / half_life_s)


class CampaignScorer:
    name = SCORER_ID

    def __init__(self, path: str = ":memory:"):
        self.path = path
        self._db = sqlite3.connect(path, isolation_level=None)
        self._db.executescript(
            "CREATE TABLE IF NOT EXISTS campaigns (id TEXT PRIMARY KEY, derived REAL NOT NULL,"
            " repeat REAL NOT NULL, t_ref REAL NOT NULL, last_seen REAL NOT NULL,"
            " targets TEXT NOT NULL);"
            "CREATE TABLE IF NOT EXISTS indicators (value TEXT PRIMARY KEY, campaign_id TEXT"
            " NOT NULL);"
            "CREATE TABLE IF NOT EXISTS seen (event_id TEXT PRIMARY KEY, campaign_id TEXT,"
            " log_odds REAL);"
        )

    # -- scorer protocol used by the replay harness --------------------------------------
    def reset(self) -> None:
        self._db.executescript("DELETE FROM campaigns; DELETE FROM indicators; DELETE FROM seen;")

    def score(self, event: dict[str, Any]) -> float:
        return self.observe(event)["confidence"]

    # -- the update -----------------------------------------------------------------------
    def observe(self, raw_event: dict[str, Any]) -> dict[str, Any]:
        config = load_policy()
        event = DeltaEvent.model_validate(raw_event)
        prior = logit(config["campaign_prior"])
        done = self._db.execute(
            "SELECT campaign_id, log_odds FROM seen WHERE event_id = ?", (event.event_id,)
        ).fetchone()
        if done:  # idempotent on event_id: the posterior does not move twice
            return self._intel(done[0], done[1], ["event already counted"])

        indicators = self._indicators(event)
        if not indicators:
            return self._intel(None, prior, ["no strong observed indicator; campaign unknown"])

        half_life_s = config["campaign_half_life_hours"] * 3600
        when = _seconds(event.observed_at)
        campaign_id = self._attach(indicators, when, half_life_s)
        hits = len(derive_badlist_hits(event, config))
        derived, repeat, t_ref, targets = self._load(campaign_id)

        if when > t_ref:  # a newer event moves the reference time and decays what is stored
            derived = _decay(derived, t_ref, when, half_life_s)
            repeat = _decay(repeat, t_ref, when, half_life_s)
            t_ref, scale = when, 1.0
        else:  # an older event counts for less, by how far behind the newest it is
            scale = _decay(1.0, when, t_ref, half_life_s)
        derived += hits * DERIVED_HIT_LOG_ODDS * scale
        key = _target_key(event.target)
        rationale = []
        if hits:
            rationale.append(f"{hits} observed field(s) on the pinned badlist (DERIVED)")
        if key not in targets:
            if targets:
                repeat += REPEAT_TARGET_LOG_ODDS * scale
                rationale.append("same indicators on another distinct target (OBSERVED)")
            targets = [*targets, key][-MAX_TARGETS_KEPT:]
        self._store(campaign_id, derived, repeat, t_ref, targets, when)

        log_odds = (
            prior
            + min(derived, config["campaign_cap_derived"])
            + min(repeat, config["campaign_cap_repeat"])
            + ASSERTED_CAP
        )
        self._db.execute(
            "INSERT INTO seen (event_id, campaign_id, log_odds) VALUES (?, ?, ?)",
            (event.event_id, campaign_id, log_odds),
        )
        self._evict(config)
        return self._intel(campaign_id, log_odds, rationale)

    # -- internals ------------------------------------------------------------------------
    @staticmethod
    def _intel(campaign_id: str | None, log_odds: float, rationale: list[str]) -> dict[str, Any]:
        return {
            "confidence": sigmoid(log_odds),
            "log_odds": log_odds,
            "campaign_id": campaign_id,
            "rationale": rationale,
        }

    @staticmethod
    def _indicators(event: DeltaEvent) -> list[str]:
        values = []
        for field in BADLIST_FIELDS.get(event.source, ()):
            value = event.attributes.get(field)
            if isinstance(value, (str, int)) and not isinstance(value, bool) and str(value):
                values.append(f"{field}={str(value)[:128]}")
        return sorted(values)

    def _attach(self, indicators: list[str], when: float, half_life_s: float) -> str:
        marks = ",".join("?" * len(indicators))
        known = {
            row[0]
            for row in self._db.execute(
                f"SELECT DISTINCT campaign_id FROM indicators WHERE value IN ({marks})",
                indicators,
            )
        }
        if not known:
            campaign_id = hashlib.sha256("\n".join(indicators).encode()).hexdigest()[:16]
            self._db.execute(
                "INSERT OR IGNORE INTO campaigns VALUES (?, 0, 0, ?, ?, '[]')",
                (campaign_id, when, when),
            )
        else:
            campaign_id = min(known)
            for other in sorted(known - {campaign_id}):
                self._merge(campaign_id, other, half_life_s)
        for value in indicators:
            self._db.execute(
                "INSERT OR REPLACE INTO indicators VALUES (?, ?)", (value, campaign_id)
            )
        return campaign_id

    def _merge(self, keep: str, drop: str, half_life_s: float) -> None:
        kd, kr, kt, ktargets = self._load(keep)
        dd, dr, dt, dtargets = self._load(drop)
        t_ref = max(kt, dt)
        derived = _decay(kd, kt, t_ref, half_life_s) + _decay(dd, dt, t_ref, half_life_s)
        repeat = _decay(kr, kt, t_ref, half_life_s) + _decay(dr, dt, t_ref, half_life_s)
        targets = list(dict.fromkeys([*ktargets, *dtargets]))[-MAX_TARGETS_KEPT:]
        self._store(keep, derived, repeat, t_ref, targets, t_ref)
        for table in ("indicators", "seen"):
            self._db.execute(
                f"UPDATE {table} SET campaign_id = ? WHERE campaign_id = ?", (keep, drop)
            )
        self._db.execute("DELETE FROM campaigns WHERE id = ?", (drop,))

    def _load(self, campaign_id: str) -> tuple[float, float, float, list[str]]:
        row = self._db.execute(
            "SELECT derived, repeat, t_ref, targets FROM campaigns WHERE id = ?", (campaign_id,)
        ).fetchone()
        return row[0], row[1], row[2], json.loads(row[3])

    def _store(self, campaign_id, derived, repeat, t_ref, targets, seen_at) -> None:
        self._db.execute(
            "UPDATE campaigns SET derived = ?, repeat = ?, t_ref = ?, targets = ?,"
            " last_seen = MAX(last_seen, ?) WHERE id = ?",
            (derived, repeat, t_ref, json.dumps(targets), seen_at, campaign_id),
        )

    def _evict(self, config: dict[str, Any]) -> None:
        limit = int(config["campaign_max_tracked"])
        excess = self._db.execute("SELECT COUNT(*) FROM campaigns").fetchone()[0] - limit
        if excess > 0:
            victims = [
                row[0]
                for row in self._db.execute(
                    "SELECT id FROM campaigns ORDER BY last_seen, id LIMIT ?", (excess,)
                )
            ]
            for victim in victims:
                self._db.execute("DELETE FROM indicators WHERE campaign_id = ?", (victim,))
                self._db.execute("DELETE FROM seen WHERE campaign_id = ?", (victim,))
                self._db.execute("DELETE FROM campaigns WHERE id = ?", (victim,))
        # Bound the idempotency table too; events whose campaign is gone are forgotten.
        cap = limit * SEEN_PER_CAMPAIGN
        self._db.execute(
            "DELETE FROM seen WHERE rowid IN (SELECT rowid FROM seen ORDER BY rowid DESC"
            " LIMIT -1 OFFSET ?)",
            (cap,),
        )

    def campaign_count(self) -> int:
        return self._db.execute("SELECT COUNT(*) FROM campaigns").fetchone()[0]

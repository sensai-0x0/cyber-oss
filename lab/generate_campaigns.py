"""Seeded, synthetic campaign timelines for scorer evaluation (plans/ml-add Phase 3).

Everything here is fake: RFC 5737 addresses and `.example` names only, and indicator
values drawn from `lab/fixtures/badlist.v1.json` or made up. No network access, no
standard-library imports beyond the basics. The generator is deterministic for a seed.

It is built so that no single feature separates the classes: benign events sometimes carry
badlist indicators, staging campaigns rotate or never match the list, and some staging
events carry no strong indicator at all. Passing this eval shows the scorer fits these
assumptions, not that it works against real campaigns (plans/ml-add/Phase-3.md).
"""

import argparse
import hashlib
import json
import random
from dataclasses import asdict, dataclass
from datetime import UTC, datetime, timedelta
from pathlib import Path

BADLIST = Path(__file__).parent / "fixtures" / "badlist.v1.json"
EPOCH = datetime(2026, 1, 1, tzinfo=UTC)
NETWORKS = ("192.0.2", "198.51.100", "203.0.113")  # RFC 5737 documentation ranges


@dataclass(frozen=True)
class Knobs:
    staging_campaigns: int = 40
    benign_campaigns: int = 80
    min_events: int = 3
    max_events: int = 8
    benign_badlist_overlap: float = 0.15  # chance a benign event carries a badlist indicator
    staging_on_badlist: float = 0.55  # chance a staging campaign uses a listed indicator at all
    rotation_rate: float = 0.25  # chance a staging event rotates to a fresh, unlisted indicator
    weak_staging_share: float = 0.3  # staging events with no strong indicator
    label_noise: float = 0.03  # chance an event label is flipped
    asserted_staging_rate: float = 0.45  # producer flags: how often staging events carry them
    asserted_benign_rate: float = 0.25  # ... and benign ones
    shared_cdn_groups: int = 4  # benign campaigns reuse a few common indicator sets
    range_target_share: float = 0.06  # events aimed at a /24 range instead of one address


def _hex(rng: random.Random, length: int) -> str:
    return "".join(rng.choice("0123456789abcdef") for _ in range(length))


def _target(rng: random.Random, range_share: float) -> str:
    if rng.random() < range_share:
        return f"{rng.choice(NETWORKS)}.0/24"
    return f"{rng.choice(NETWORKS)}.{rng.randint(1, 254)}:443"


def _indicators(rng: random.Random, badlist: dict, listed: bool) -> dict:
    if listed:
        return {
            "jarm": rng.choice(badlist["jarm"]),
            "ja3": rng.choice(badlist["ja3"]),
            "favicon_mmh3": rng.choice(badlist["favicon_mmh3"]),
        }
    return {
        "jarm": _hex(rng, 62),
        "ja3": _hex(rng, 32),
        "favicon_mmh3": str(-rng.randint(10**8, 10**9)),
    }


def _event(rng, source_id, when, target, attributes, label, knobs, staging_flags):
    flag_rate = knobs.asserted_staging_rate if staging_flags else knobs.asserted_benign_rate
    asserted = {}
    if rng.random() < flag_rate:
        asserted["known_bad"] = True
    if rng.random() < flag_rate:
        asserted["recent_ct"] = True
    if rng.random() < flag_rate:
        asserted["shared_fingerprints"] = rng.randint(1, 3)
    if rng.random() < knobs.label_noise:
        label = "benign" if label == "staging" else "staging"
    attributes = {**attributes, **asserted}
    body = json.dumps([source_id, when.isoformat(), target, attributes], sort_keys=True)
    return {
        "event": {
            "schema_version": "v1",
            "event_id": f"ev-{source_id}",
            "kind": "novel",
            "source": "tlsx",
            "target": target,
            "observed_at": when.isoformat(),
            "fingerprint": hashlib.sha256(body.encode()).hexdigest(),
            "attributes": attributes,
        },
        "label": label,
    }


def generate(seed: int, knobs: Knobs | None = None) -> list[dict]:
    knobs = knobs or Knobs()
    rng = random.Random(seed)
    badlist = json.loads(BADLIST.read_text())
    cdn_sets = [
        _indicators(rng, badlist, listed=False) for _ in range(knobs.shared_cdn_groups)
    ]
    campaigns = []
    counter = 0
    for index in range(knobs.staging_campaigns + knobs.benign_campaigns):
        staging = index < knobs.staging_campaigns
        start = EPOCH + timedelta(hours=rng.randint(0, 24 * 30))
        build_hours = rng.randint(24, 96)
        go_live = start + timedelta(hours=build_hours) if staging else None
        count = rng.randint(knobs.min_events, knobs.max_events)
        base = _indicators(rng, badlist, listed=rng.random() < knobs.staging_on_badlist)
        events = []
        for _ in range(count):
            counter += 1
            span = build_hours + 12 if staging else 24 * 7
            when = start + timedelta(minutes=rng.randint(0, span * 60))
            if staging:
                if rng.random() < knobs.weak_staging_share:
                    attributes = {"common_name": f"{_hex(rng, 6)}.example"}
                elif rng.random() < knobs.rotation_rate:
                    attributes = _indicators(rng, badlist, listed=False)
                else:
                    attributes = dict(base)
                label = "staging" if when <= go_live + timedelta(hours=12) else "benign"
            else:
                if rng.random() < knobs.benign_badlist_overlap:
                    attributes = _indicators(rng, badlist, listed=True)
                elif rng.random() < 0.5:
                    attributes = dict(rng.choice(cdn_sets))
                else:
                    attributes = _indicators(rng, badlist, listed=False)
                label = "benign"
            events.append(
                _event(
                    rng,
                    counter,
                    when,
                    _target(rng, knobs.range_target_share),
                    attributes,
                    label,
                    knobs,
                    staging,
                )
            )
        events.sort(key=lambda item: item["event"]["observed_at"])
        campaigns.append(
            {
                "campaign_id": f"camp-{index:03d}",
                "label": "staging" if staging else "benign",
                "go_live_at": go_live.isoformat() if go_live else None,
                "events": events,
            }
        )
    rng.shuffle(campaigns)
    return campaigns


def dump(campaigns: list[dict]) -> str:
    """One campaign per line, sorted keys, so the file hash is stable."""
    return "".join(json.dumps(item, sort_keys=True) + "\n" for item in campaigns)


def dataset_sha256(campaigns: list[dict]) -> str:
    return hashlib.sha256(dump(campaigns).encode()).hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate synthetic campaign timelines.")
    parser.add_argument("--seed", type=int, required=True)
    parser.add_argument("--out", type=Path, required=True)
    for name, default in asdict(Knobs()).items():
        parser.add_argument(f"--{name.replace('_', '-')}", type=type(default), default=default)
    args = vars(parser.parse_args())
    out, seed = args.pop("out"), args.pop("seed")
    out.write_text(dump(generate(seed, Knobs(**args))))


if __name__ == "__main__":
    main()

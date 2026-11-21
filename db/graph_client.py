"""Graphiti + FalkorDB client for cross-session agent/orchestration memory.

Not part of the hunterd/orchestrator/enforcerd security pipeline and never a
consumer of DeltaEvent or MitigationAction; see db/CLAUDE.md for the boundary.
graphiti_core is imported lazily inside the functions that need a live client so
that config loading and episode-body construction stay unit-testable without the
dependency installed.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any, Literal

MAX_EPISODE_BODY_CHARS = 8_000
LOOPBACK_HOSTS = {"127.0.0.1", "localhost"}

EpisodeKind = Literal["WORK_BLOCK_COMPLETED", "WORK_BLOCK_BLOCKED"]


class GraphConfigError(ValueError):
    """Required graph-DB environment configuration is missing or invalid."""


@dataclass(frozen=True)
class GraphConfig:
    host: str
    port: int
    password: str | None
    database: str
    group_id: str
    max_coroutines: int


def load_graph_config(env: dict[str, str] | None = None) -> GraphConfig:
    """Read and validate FalkorDB/Graphiti settings from the environment.

    Fails loudly on a missing or malformed variable rather than defaulting to a
    host, port or group_id that could point at the wrong graph namespace.
    """
    source = env if env is not None else os.environ
    try:
        host = source["FALKORDB_HOST"]
        port = int(source["FALKORDB_PORT"])
        database = source["FALKORDB_DATABASE"]
        group_id = source["GRAPHITI_GROUP_ID"]
    except KeyError as exc:
        raise GraphConfigError(f"missing required env var: {exc.args[0]}") from exc
    except ValueError as exc:
        raise GraphConfigError(f"FALKORDB_PORT must be an integer: {exc}") from exc

    if host not in LOOPBACK_HOSTS:
        raise GraphConfigError(
            f"FALKORDB_HOST must be loopback-only, got {host!r} "
            "(db/CONFIG.md risk register: Unauthenticated MCP/REST Exposure)"
        )

    max_coroutines = int(source.get("GRAPHITI_MAX_COROUTINES", "4"))
    if max_coroutines < 1:
        raise GraphConfigError("GRAPHITI_MAX_COROUTINES must be >= 1")

    return GraphConfig(
        host=host,
        port=port,
        password=source.get("FALKORDB_PASSWORD") or None,
        database=database,
        group_id=group_id,
        max_coroutines=max_coroutines,
    )


def build_graphiti(config: GraphConfig):
    """Construct a Graphiti client bound to the given FalkorDB config."""
    from graphiti_core import Graphiti
    from graphiti_core.driver.falkordb_driver import FalkorDriver

    driver = FalkorDriver(
        host=config.host,
        port=config.port,
        password=config.password,
        database=config.database,
    )
    return Graphiti(graph_driver=driver, max_coroutines=config.max_coroutines)


async def init_graph(graphiti) -> None:
    """Create Graphiti's fulltext/vector indices and constraints.

    Idempotent; call once at orchestrator bootstrap.
    """
    await graphiti.build_indices_and_constraints()


def build_episode_body(kind: EpisodeKind, payload: dict[str, Any]) -> str:
    """Serialize a WORK_BLOCK_* episode to the JSON string Graphiti ingests.

    Truncates every string field so a raw target banner or error log pasted into
    `lessons_learned` or a failure trace cannot smuggle an unbounded payload into
    the extraction LLM (db/CONFIG.md risk register: Untrusted Episode Prompt
    Injection).
    """
    sanitized = {"episode_type": kind, **{k: _truncate(v) for k, v in payload.items()}}
    return json.dumps(sanitized, sort_keys=True)


def _truncate(value: Any) -> Any:
    if isinstance(value, str) and len(value) > MAX_EPISODE_BODY_CHARS:
        return value[:MAX_EPISODE_BODY_CHARS] + "...<truncated>"
    if isinstance(value, list):
        return [_truncate(item) for item in value]
    return value


async def add_work_block_episode(
    graphiti,
    config: GraphConfig,
    kind: EpisodeKind,
    name: str,
    payload: dict[str, Any],
    reference_time: datetime | None = None,
) -> None:
    """Ingest one WORK_BLOCK_COMPLETED/BLOCKED episode under the configured group_id."""
    from graphiti_core.nodes import EpisodeType

    await graphiti.add_episode(
        name=name,
        episode_body=build_episode_body(kind, payload),
        source=EpisodeType.json,
        reference_time=reference_time or datetime.now(UTC),
        group_id=config.group_id,
    )


async def search_graph(graphiti, config: GraphConfig, query: str, num_results: int = 10):
    """Search the knowledge graph, scoped to the configured group_id only."""
    return await graphiti.search(query=query, group_ids=[config.group_id], num_results=num_results)

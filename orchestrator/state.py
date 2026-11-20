"""Shared control-plane state and the versioned Go-to-Python event contract."""

from datetime import datetime
from typing import Annotated, Any, Literal, TypedDict

from pydantic import BaseModel, ConfigDict, Field


class DeltaEvent(BaseModel):
    """Only novel or mutated observations may enter the LangGraph plane."""

    model_config = ConfigDict(extra="forbid")

    schema_version: Literal["v1"]
    event_id: str = Field(min_length=1, max_length=128)
    kind: Literal["novel", "mutated"]
    source: Literal["tlsx", "httpx", "ct"]
    target: str = Field(min_length=1, max_length=2048)
    observed_at: datetime
    fingerprint: Annotated[str, Field(pattern=r"^[0-9a-f]{64}$")]
    attributes: dict[str, Any] = Field(default_factory=dict)


class Cluster(TypedDict):
    cluster_id: str
    indicators: list[str]
    event_ids: list[str]


class IntelScore(TypedDict):
    confidence: float
    rationale: list[str]


class Policy(TypedDict, total=False):
    action: Literal["observe", "mitigate"]
    action_id: str
    target: str
    kind: Literal["nftables", "rpz", "headscale"]
    broad: bool
    approved: bool
    ttl_seconds: int
    reason: str


class GraphState(TypedDict, total=False):
    event: DeltaEvent
    cluster: Cluster
    intel: IntelScore
    policy: Policy
    cycles: int

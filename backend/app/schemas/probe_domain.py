from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Literal
from pydantic import BaseModel, Field


class PlatformProbeResultDTO(BaseModel):
    """单平台探测结果表达"""

    platform: str
    status: Literal["ok", "blocked", "challenged", "mainland", "unknown", "fail"]
    label: str | None = None
    region: str | None = None
    unlocked: bool = False
    latency_ms: int | None = None
    error: str | None = None


class ProbeObservationDTO(BaseModel):
    """节点探测单次不可变观测记录"""

    id: int | None = None
    job_id: int
    node_id: int
    node_logical_id: str | None = None
    node_name: str | None = None
    profile_id: int | None = None
    status: Literal["ok", "fail", "timeout", "challenged"]
    latency_ms: int | None = None
    speed_mbps: float | None = None
    egress_ip: str | None = None
    country: str | None = None
    asn: int | None = None
    organization: str | None = None
    media_results: dict[str, Any] = Field(default_factory=dict)
    error: str | None = None
    observed_at: datetime = Field(
        default_factory=lambda: datetime.now(timezone.utc)
    )


class ProbeProfileDTO(BaseModel):
    """探测策略配置文件表达"""

    id: int | None = None
    logical_id: str
    name: str
    platforms: list[str] = Field(default_factory=list)
    timeout_s: int = 10
    concurrency: int = 5
    max_bytes: int = 50000000
    min_speed_mbps: float | None = None
    created_at: datetime | None = None


class ProbeJobDTO(BaseModel):
    """探测批任务状态表达"""

    id: int | None = None
    logical_id: str
    profile_id: int
    trigger_type: Literal["manual", "scheduled", "system"] = "manual"
    status: Literal["queued", "running", "completed", "failed", "cancelled", "skipped"] = "queued"
    queued_at: datetime = Field(
        default_factory=lambda: datetime.now(timezone.utc)
    )
    started_at: datetime | None = None
    finished_at: datetime | None = None
    summary: dict[str, Any] | None = None


class ObservationEligibilityDTO(BaseModel):
    """观测记录针对筛选策略的资格判定结果"""

    eligible: bool
    status: Literal["valid", "missing", "stale", "failed", "incompatible", "insufficient"]
    reason: str
    observation: ProbeObservationDTO | None = None

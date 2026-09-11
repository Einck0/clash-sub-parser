from __future__ import annotations

from dataclasses import dataclass
from typing import Any
from pydantic import BaseModel, Field


@dataclass(frozen=True)
class ResolvedProbeConfig:
    """不可变的批次/单次探测配置快照"""
    probe_enabled: bool = True
    speedtest_enabled: bool = False
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000"
    speedtest_max_bytes: int = 5242880
    speedtest_timeout_s: float = 5.0
    media_check_enabled: bool = True
    media_platforms: tuple[str, ...] = ("youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini")
    media_timeout_s: float = 5.0
    concurrency: int = 10
    service_timeout_s: float = 2.0
    node_timeout_s: float | None = None
    probe_cron_enabled: bool = True
    probe_cron_interval_minutes: int = 60


class ProbeSettingsRead(BaseModel):
    probe_enabled: bool = True
    probe_interval_minutes: int = 0
    speedtest_enabled: bool = False
    speedtest_url: str = "https://speed.cloudflare.com/__down?bytes=5000000"
    speedtest_timeout_s: int = 5
    speedtest_max_bytes: int = 5242880
    speedtest_min_speed_mbps: float = 0.0
    media_check_enabled: bool = True
    media_platforms: list[str] = Field(
        default_factory=lambda: ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"]
    )
    media_timeout_s: int = 5
    probe_concurrency: int = 10
    probe_service_timeout_ms: int = 2000
    probe_timeout_ms: int = 3000
    probe_cron_enabled: bool = True
    probe_cron_interval_minutes: int = 60


class ProbeSettingsUpdate(BaseModel):
    probe_enabled: bool | None = None
    probe_interval_minutes: int | None = Field(default=None, ge=0, le=1440)
    speedtest_enabled: bool | None = None
    speedtest_url: str | None = Field(default=None, max_length=512)
    speedtest_timeout_s: int | None = Field(default=None, ge=1, le=60)
    speedtest_max_bytes: int | None = Field(default=None, ge=1024, le=104857600)
    speedtest_min_speed_mbps: float | None = Field(default=None, ge=0.0)
    media_check_enabled: bool | None = None
    media_platforms: list[str] | None = None
    media_timeout_s: int | None = Field(default=None, ge=1, le=30)
    probe_concurrency: int | None = Field(default=None, ge=1, le=20)
    probe_service_timeout_ms: int | None = Field(default=None, ge=500, le=30000)
    probe_timeout_ms: int | None = Field(default=None, ge=0, le=30000)
    probe_cron_enabled: bool | None = None
    probe_cron_interval_minutes: int | None = Field(default=None, ge=1, le=1440)


class ProbeNodeRequest(BaseModel):
    node: dict[str, Any]
    include_speed: bool | None = None
    include_media: bool | None = None
    use_cache: bool = True


class ProbeBatchRequest(BaseModel):
    nodes: list[dict[str, Any]] = Field(default_factory=list)
    include_speed: bool | None = None
    include_media: bool | None = None
    concurrency: int | None = Field(default=None, ge=1, le=20)
    use_cache: bool = True
    timeout_ms: int | None = Field(default=None, ge=0, le=30000)


class ProviderEvidenceRead(BaseModel):
    http_status: int | None = None
    final_host: str | None = None
    redirect_class: str | None = None
    signals: list[str] = Field(default_factory=list)
    elapsed_ms: int = 0
    error_code: str | None = None


class ProviderResultRead(BaseModel):
    status: str
    verdict: str
    unlocked: bool = False
    region: str | None = None
    checked_at: int = 0
    evidence_version: str = "catalogue-2026-09-05"
    confidence: str = "verified"
    evidence: dict[str, Any] = Field(default_factory=dict)
    label: str | None = None
    error: str | None = None
    observation_kind: str | None = None
    tier: str | None = None
    subobservations: dict[str, Any] | None = None


class ProbeStatusRead(BaseModel):
    state: str
    server_now: int
    interval_minutes: int | None = None
    next_expected_at: int | None = None
    last_started_at: int | None = None
    last_finished_at: int | None = None
    last_summary: dict[str, int] | None = None
    last_error_code: str | None = None


class IdentityEvidenceRead(BaseModel):
    agreement: bool = False
    providers: list[dict[str, Any]] = Field(default_factory=list)


class CdnRoutingObservationRead(BaseModel):
    status: str
    verdict: str
    confidence: str = "verified"
    route_hint: str | None = None
    iata_code: str | None = None
    checked_at: int = 0
    evidence_version: str = "catalogue-2026-09-05"
    evidence: dict[str, Any] = Field(default_factory=dict)
    error: str | None = None


class NodeProbeResultRead(BaseModel):
    node_key: str
    name: str
    server: str
    port: int | None = None
    type: str
    status: str
    latency_ms: int | None = None
    speed_mbps: float | None = None
    ip: str | None = None
    country: str | None = None
    asn: int | None = None
    organization: str | None = None
    media: dict[str, Any] = Field(default_factory=dict)
    error: str | None = None
    checked_at: int


class ProbeResultSummaryItem(BaseModel):
    node_key: str
    name: str
    status: str
    latency_ms: int | None = None
    speed_mbps: float | None = None
    country: str | None = None
    ip: str | None = None
    media: dict[str, bool] = Field(default_factory=dict)


class ProbeResultsSummaryResponse(BaseModel):
    results: dict[str, ProbeResultSummaryItem]
    next_cursor: str | None = None
    has_more: bool = False

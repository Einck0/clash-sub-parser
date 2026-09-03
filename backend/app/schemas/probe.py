from __future__ import annotations

from typing import Any
from pydantic import BaseModel, Field


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
    probe_concurrency: int = 5
    probe_timeout_ms: int = 3000


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
    probe_timeout_ms: int | None = Field(default=None, ge=500, le=30000)


class ProbeNodeRequest(BaseModel):
    node: dict[str, Any]
    include_speed: bool | None = None
    include_media: bool | None = None
    use_cache: bool = True


class ProbeBatchRequest(BaseModel):
    nodes: list[dict[str, Any]] = Field(default_factory=list)
    include_speed: bool | None = None
    include_media: bool | None = None
    concurrency: int | None = None
    use_cache: bool = True
    timeout_ms: int | None = None

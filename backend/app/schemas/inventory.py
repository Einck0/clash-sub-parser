from __future__ import annotations

from datetime import datetime
from typing import Any, Literal
from pydantic import BaseModel, ConfigDict, Field


def sanitize_payload_secrets(payload: dict[str, Any]) -> dict[str, Any]:
    """对节点 payload 中的密码与鉴权字段进行脱敏，避免泄露"""
    sanitized: dict[str, Any] = {}
    secret_keywords = {"password", "uuid", "secret", "token", "auth", "key", "private-key", "preshared-key"}
    for k, v in payload.items():
        if any(keyword in k.lower() for keyword in secret_keywords):
            sanitized[k] = "***"
        elif isinstance(v, dict):
            sanitized[k] = sanitize_payload_secrets(v)
        else:
            sanitized[k] = v
    return sanitized


class SafeSourceProvenance(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    source_logical_id: str
    source_name: str
    source_kind: str
    revision_id: str
    display_name: str
    original_order: int


class InventoryNodeItem(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    logical_id: str
    name: str
    protocol: str
    server: str
    port: int
    lifecycle_state: str
    sources: list[SafeSourceProvenance] = Field(default_factory=list)
    created_at: datetime
    updated_at: datetime


class InventoryListResponse(BaseModel):
    total: int
    page: int
    page_size: int
    items: list[InventoryNodeItem]


class InventoryNodeDetail(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    logical_id: str
    name: str
    protocol: str
    server: str
    port: int
    lifecycle_state: str
    payload_fingerprint: str
    sanitized_payload: dict[str, Any]
    sources: list[SafeSourceProvenance] = Field(default_factory=list)
    latest_observation: dict[str, Any] | None = None
    created_at: datetime
    updated_at: datetime


class SourceBase(BaseModel):
    name: str = Field(..., max_length=120)
    kind: Literal["subscription", "manual"] = "subscription"
    url: str | None = None
    update_interval: int | None = None
    enabled: bool = True


class SourceCreate(SourceBase):
    logical_id: str | None = None


class SourceUpdate(BaseModel):
    name: str | None = Field(None, max_length=120)
    url: str | None = None
    update_interval: int | None = None
    enabled: bool | None = None


class SourceRevisionRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    revision_id: str
    status: str
    payload_hash: str
    node_count: int
    error_summary: str | None = None
    fetched_at: datetime | None = None
    created_at: datetime


class SourceRead(SourceBase):
    model_config = ConfigDict(from_attributes=True)

    logical_id: str
    created_at: datetime
    updated_at: datetime
    active_revision: SourceRevisionRead | None = None

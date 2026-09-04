"""Router for configuration generation, multi-target distribution, and QuickExport.

Supports five core targets: Clash, Mihomo, Stash, Shadowrocket, and Sing-box.
"""

from __future__ import annotations

from typing import Any
from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import PlainTextResponse
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.generate_config import GenerateConfigRead, GenerateConfigUpdate
from app.services.generate_config_service import get_generate_config, update_generate_config
from app.services.generate_service import (
    file_response,
    generate_subscription_payload,
    generate_target,
    generate_yaml,
    get_quick_export,
    render_current,
)

router = APIRouter(prefix="/generate", tags=["generate"])


class GenerateSwitches(BaseModel):
    """Configuration switches for module generation."""

    enabled: bool = True
    subscriptions: bool = True
    node_groups: bool = True
    rules: bool = True
    dns: bool = True
    exclude_node_proxies: bool = True


@router.get("/settings", response_model=GenerateConfigRead)
async def get_generate_settings_endpoint(db: AsyncSession = Depends(get_db)) -> GenerateConfigRead:
    """Get current generation settings."""
    return await get_generate_config(db)


@router.patch("/settings", response_model=GenerateConfigRead)
async def update_generate_settings_endpoint(
    payload: GenerateConfigUpdate,
    db: AsyncSession = Depends(get_db),
) -> GenerateConfigRead:
    """Update generation settings."""
    return await update_generate_config(db, payload)


@router.get("/quick-export")
async def quick_export_endpoint(
    request: Request,
    subscription_id: int | None = Query(None, description="Optional subscription ID"),
    target: str | None = Query(None, description="Optional target filter"),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Provide QuickExport URLs, client schemes, and QR payloads for five targets."""
    return await get_quick_export(db, request, subscription_id=subscription_id, target=target)


@router.get("/script")
@router.post("/script")
@router.get("/script/current")
@router.get("/script/download")
async def script_endpoints_removed() -> None:
    """SCRIPT format has been deprecated and strictly removed."""
    raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")


@router.get("/yaml")
async def get_current_yaml_endpoint(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    """Get current rendered Clash/YAML configuration inline."""
    content = await render_current(db, "yaml")
    return await file_response(db, content, "yaml", disposition="inline")


@router.post("/yaml")
async def generate_yaml_endpoint(
    switches: GenerateSwitches,
    db: AsyncSession = Depends(get_db),
) -> dict:
    """Generate Clash/YAML configuration with provided switches."""
    return await generate_yaml(db, switches.model_dump())


@router.get("/yaml/current")
async def download_current_yaml_endpoint(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    """Download current rendered Clash/YAML configuration as attachment."""
    content = await render_current(db, "yaml")
    return await file_response(db, content, "yaml", disposition="attachment")


@router.get("/yaml/download")
async def download_yaml_endpoint(
    switches: GenerateSwitches = Depends(),
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    """Download Clash/YAML configuration with switches as attachment."""
    result = await generate_yaml(db, switches.model_dump())
    return await file_response(db, result.get("yaml", ""), "yaml", disposition="attachment")


@router.get("/subscription/{subscription_id}")
async def get_subscription_export_endpoint(
    subscription_id: int,
    target: str = Query("clash", description="Target format (clash, mihomo, stash, shadowrocket, sing-box)"),
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    """Download or stream single subscription content for a specific target format."""
    payload = await generate_subscription_payload(db, subscription_id, target=target)
    content = payload.get("content") or payload.get("yaml") or payload.get("payload") or payload.get("json") or ""
    return await file_response(db, content, target, disposition="inline")


@router.post("/subscription/{subscription_id}")
async def generate_subscription_endpoint(
    subscription_id: int,
    target: str = Query("clash", description="Target format (clash, mihomo, stash, shadowrocket, sing-box)"),
    db: AsyncSession = Depends(get_db),
) -> dict:
    """Generate single subscription payload for a specific target format."""
    return await generate_subscription_payload(db, subscription_id, target=target)


@router.get("/{target}")
async def get_current_target_endpoint(
    target: str,
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    """Get current rendered configuration for the specified target inline."""
    if target.lower() == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")
    content = await render_current(db, target)
    return await file_response(db, content, target, disposition="inline")


@router.post("/{target}")
async def generate_target_endpoint(
    target: str,
    switches: GenerateSwitches,
    db: AsyncSession = Depends(get_db),
) -> dict:
    """Generate configuration for the specified target with custom switches."""
    if target.lower() == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")
    return await generate_target(db, target, switches.model_dump())


@router.get("/{target}/current")
async def download_current_target_endpoint(
    target: str,
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    """Download current configuration for the specified target as attachment."""
    if target.lower() == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")
    content = await render_current(db, target)
    return await file_response(db, content, target, disposition="attachment")


@router.get("/{target}/download")
async def download_target_endpoint(
    target: str,
    switches: GenerateSwitches = Depends(),
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    """Download configuration for the specified target with switches as attachment."""
    if target.lower() == "script":
        raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")
    result = await generate_target(db, target, switches.model_dump())
    content = result.get("content") or result.get("yaml") or result.get("payload") or result.get("json") or ""
    return await file_response(db, content, target, disposition="attachment")

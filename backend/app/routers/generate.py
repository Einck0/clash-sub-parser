from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import PlainTextResponse
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.generate_config import GenerateConfigRead, GenerateConfigUpdate
from app.services.generate_config_service import get_generate_config, update_generate_config
from app.services.generate_service import (
    file_response,
    generate_subscription_payload,
    generate_yaml,
    render_current,
)

router = APIRouter(prefix="/generate", tags=["generate"])


class GenerateSwitches(BaseModel):
    enabled: bool = True
    subscriptions: bool = True
    node_groups: bool = True
    rules: bool = True
    dns: bool = True
    exclude_node_proxies: bool = True


@router.get("/settings", response_model=GenerateConfigRead)
async def get_generate_settings_endpoint(db: AsyncSession = Depends(get_db)) -> GenerateConfigRead:
    return await get_generate_config(db)


@router.patch("/settings", response_model=GenerateConfigRead)
async def update_generate_settings_endpoint(
    payload: GenerateConfigUpdate,
    db: AsyncSession = Depends(get_db),
) -> GenerateConfigRead:
    return await update_generate_config(db, payload)


@router.get("/yaml")
async def get_current_yaml_endpoint(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    content = await render_current(db, "yaml")
    return await file_response(db, content, "yaml", disposition="inline")


@router.post("/yaml")
async def generate_yaml_endpoint(
    switches: GenerateSwitches,
    db: AsyncSession = Depends(get_db),
) -> dict:
    return await generate_yaml(db, switches.model_dump())


@router.get("/yaml/current")
async def download_current_yaml_endpoint(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    content = await render_current(db, "yaml")
    return await file_response(db, content, "yaml", disposition="attachment")


@router.get("/script")
@router.post("/script")
@router.get("/script/current")
@router.get("/script/download")
async def script_endpoints_removed():
    raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")


@router.get("/yaml/download")
async def download_yaml_endpoint(
    switches: GenerateSwitches = Depends(),
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    result = await generate_yaml(db, switches.model_dump())
    return await file_response(db, result.get("yaml", ""), "yaml", disposition="attachment")


@router.post("/subscription/{subscription_id}")
async def generate_subscription_endpoint(
    subscription_id: int,
    db: AsyncSession = Depends(get_db),
) -> dict:
    return await generate_subscription_payload(db, subscription_id)

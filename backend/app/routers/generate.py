from fastapi import APIRouter, Depends
from fastapi.responses import PlainTextResponse
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.generate_config import GenerateConfigRead, GenerateConfigUpdate
from app.services.generate_config_service import (
    generate_config_to_switches,
    get_generate_config,
    update_generate_config,
)
from app.services.generate_service import (
    generate_script,
    generate_subscription_payload,
    generate_yaml,
    get_primary_subscription_headers,
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
async def get_generate_settings_endpoint(
    db: AsyncSession = Depends(get_db),
) -> GenerateConfigRead:
    return await get_generate_config(db)


@router.patch("/settings", response_model=GenerateConfigRead)
async def update_generate_settings_endpoint(
    payload: GenerateConfigUpdate,
    db: AsyncSession = Depends(get_db),
) -> GenerateConfigRead:
    return await update_generate_config(db, payload)


@router.get("/script")
async def get_current_script_endpoint(
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    config = await get_generate_config(db)
    result = await generate_script(db, generate_config_to_switches(config))
    return PlainTextResponse(
        content=result.get("script", ""),
        media_type="text/javascript",
        headers={"Content-Disposition": 'inline; filename="script.js"'},
    )


@router.get("/yaml")
async def get_current_yaml_endpoint(
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    config = await get_generate_config(db)
    result = await generate_yaml(db, generate_config_to_switches(config))
    headers = {"Content-Disposition": 'inline; filename="config.yaml"'}
    headers.update(await get_primary_subscription_headers(db))
    return PlainTextResponse(
        content=result.get("yaml", ""),
        media_type="application/x-yaml",
        headers=headers,
    )


@router.post("/script")
async def generate_script_endpoint(
    switches: GenerateSwitches,
    db: AsyncSession = Depends(get_db),
) -> dict:
    return await generate_script(db, switches.model_dump())


@router.post("/yaml")
async def generate_yaml_endpoint(
    switches: GenerateSwitches,
    db: AsyncSession = Depends(get_db),
) -> dict:
    return await generate_yaml(db, switches.model_dump())


@router.get("/script/current")
async def download_current_script_endpoint(
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    config = await get_generate_config(db)
    result = await generate_script(db, generate_config_to_switches(config))
    return PlainTextResponse(
        content=result.get("script", ""),
        media_type="text/javascript",
        headers={"Content-Disposition": 'attachment; filename="script.js"'},
    )


@router.get("/yaml/current")
async def download_current_yaml_endpoint(
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    config = await get_generate_config(db)
    result = await generate_yaml(db, generate_config_to_switches(config))
    headers = {"Content-Disposition": 'attachment; filename="config.yaml"'}
    headers.update(await get_primary_subscription_headers(db))
    return PlainTextResponse(
        content=result.get("yaml", ""),
        media_type="application/x-yaml",
        headers=headers,
    )


@router.get("/script/download")
async def download_script_endpoint(
    switches: GenerateSwitches = Depends(),
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    result = await generate_script(db, switches.model_dump())
    return PlainTextResponse(
        content=result.get("script", ""),
        media_type="text/javascript",
        headers={"Content-Disposition": 'attachment; filename="script.js"'},
    )


@router.get("/yaml/download")
async def download_yaml_endpoint(
    switches: GenerateSwitches = Depends(),
    db: AsyncSession = Depends(get_db),
) -> PlainTextResponse:
    result = await generate_yaml(db, switches.model_dump())
    headers = {"Content-Disposition": 'attachment; filename="config.yaml"'}
    headers.update(await get_primary_subscription_headers(db))
    return PlainTextResponse(
        content=result.get("yaml", ""),
        media_type="application/x-yaml",
        headers=headers,
    )


@router.post("/subscription/{subscription_id}")
async def generate_subscription_endpoint(
    subscription_id: int, db: AsyncSession = Depends(get_db)
) -> dict:
    return await generate_subscription_payload(db, subscription_id)

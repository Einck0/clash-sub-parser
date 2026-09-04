from typing import Any
from pydantic import BaseModel, HttpUrl
from fastapi import APIRouter, Depends, Query, Request
from fastapi.responses import FileResponse
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.services.download_service import download_custom, download_preset, get_download_file_response, list_downloads
from app.services.generate_service import get_quick_export

router = APIRouter(prefix="/downloads", tags=["downloads"])


class PresetDownloadRequest(BaseModel):
    preset_id: str


class CustomDownloadRequest(BaseModel):
    url: HttpUrl


@router.get("/quick-export")
async def quick_export_downloads_endpoint(
    request: Request,
    subscription_id: int | None = Query(None, description="Optional subscription ID"),
    target: str | None = Query(None, description="Optional target filter"),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Provide QuickExport URLs, client schemes, and QR payloads via downloads namespace."""
    return await get_quick_export(db, request, subscription_id=subscription_id, target=target)



@router.get("")
async def list_downloads_endpoint() -> dict:
    return await list_downloads()


@router.post("/preset")
async def download_preset_endpoint(payload: PresetDownloadRequest) -> dict:
    return await download_preset(payload.preset_id)


@router.post("/custom")
async def download_custom_endpoint(payload: CustomDownloadRequest) -> dict:
    return await download_custom(str(payload.url))


@router.get("/files/{filename}")
async def get_download_file_endpoint(filename: str) -> FileResponse:
    return get_download_file_response(filename)

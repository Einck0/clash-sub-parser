from pydantic import BaseModel, HttpUrl
from fastapi import APIRouter
from fastapi.responses import FileResponse

from app.services.download_service import download_custom, download_preset, get_download_file_response, list_downloads

router = APIRouter(prefix="/downloads", tags=["downloads"])


class PresetDownloadRequest(BaseModel):
    preset_id: str


class CustomDownloadRequest(BaseModel):
    url: HttpUrl


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

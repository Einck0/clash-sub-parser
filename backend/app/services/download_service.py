from __future__ import annotations

from datetime import datetime, timezone
from pathlib import Path
from typing import Literal
from urllib.parse import unquote, urlparse
import re

import httpx
from fastapi import HTTPException
from fastapi.responses import FileResponse

from app.config import get_settings
from app.services.security_settings_service import get_fetch_proxy_config, get_security_settings
from app.database import AsyncSessionLocal
from app.utils.validators import validate_fetch_url

async def _request_with_redirect_validation(
    client: httpx.AsyncClient,
    url: str,
    allow_private: bool,
    max_redirects: int = 10,
) -> httpx.Response:
    """Follow redirects manually, validating each URL against SSRF rules."""
    from urllib.parse import urljoin
    current_url = url
    for _ in range(max_redirects + 1):
        response = await client.send(
            client.build_request("GET", current_url),
            stream=True,
        )
        if response.is_redirect:
            location = response.headers.get("location")
            if not location:
                return response
            resolved = urljoin(str(response.url), location)
            current_url = validate_fetch_url(resolved, allow_private_hosts=allow_private)
            await response.aclose()
            continue
        return response
    raise HTTPException(status_code=502, detail="Too many redirects")


settings = get_settings()
DOWNLOAD_DIR = Path(settings.download_dir)
GITHUB_API = "https://api.github.com/repos/{owner}/{repo}/releases/latest"

PresetId = Literal["clash-verge-rev-windows-x64", "clash-meta-android-arm64"]

PRESETS: dict[str, dict[str, str]] = {
    "clash-verge-rev-windows-x64": {
        "id": "clash-verge-rev-windows-x64",
        "label": "Clash Verge Rev 最新版（Windows x64）",
        "owner": "clash-verge-rev",
        "repo": "clash-verge-rev",
        "pattern": r"(?i)(x64|x86_64|amd64).*(setup|\.exe|\.msi|\.zip)|(?:setup|\.exe|\.msi|\.zip).*(x64|x86_64|amd64)",
        "cache_prefix": "clash-verge-rev-windows-x64-",
    },
    "clash-meta-android-arm64": {
        "id": "clash-meta-android-arm64",
        "label": "Clash Meta for Android 最新版（arm64-v8a）",
        "owner": "MetaCubeX",
        "repo": "ClashMetaForAndroid",
        "pattern": r"(?i)arm64-v8a.*\.apk$",
        "cache_prefix": "clash-meta-android-arm64-",
    },
}


async def list_downloads() -> dict:
    DOWNLOAD_DIR.mkdir(parents=True, exist_ok=True)
    items = []
    presets = [{k: v for k, v in preset.items() if k != "cache_prefix"} for preset in PRESETS.values()]
    for path in sorted(DOWNLOAD_DIR.iterdir(), key=lambda item: item.stat().st_mtime, reverse=True):
        if not path.is_file() or path.name.startswith(".") or path.suffix == ".part":
            continue
        stat = path.stat()
        items.append({
            "filename": path.name,
            "size": stat.st_size,
            "mtime": datetime.fromtimestamp(stat.st_mtime, tz=timezone.utc).isoformat(),
            "download_url": f"/api/downloads/files/{path.name}",
        })
    return {"presets": presets, "items": items}


async def download_preset(preset_id: str) -> dict:
    preset = PRESETS.get(preset_id)
    if not preset:
        raise HTTPException(status_code=404, detail="Unknown download preset")
    asset = await _find_latest_github_asset(preset)
    safe_name = _safe_filename(asset["name"])
    prefixed_name = f"{preset['cache_prefix']}{safe_name}"
    return await download_url(asset["url"], expected_name=prefixed_name, source=f"github:{preset['owner']}/{preset['repo']}@latest", cache_prefix=preset["cache_prefix"])


async def download_custom(url: str) -> dict:
    normalized = validate_fetch_url(url, allow_private_hosts=settings.allow_private_fetch_urls)
    return await download_url(normalized, source="custom")


async def download_url(url: str, expected_name: str | None = None, source: str = "custom", cache_prefix: str | None = None) -> dict:
    normalized = validate_fetch_url(url, allow_private_hosts=settings.allow_private_fetch_urls)
    filename = _safe_filename(expected_name or _filename_from_url(normalized))
    DOWNLOAD_DIR.mkdir(parents=True, exist_ok=True)
    target = (DOWNLOAD_DIR / filename).resolve()
    if DOWNLOAD_DIR.resolve() not in target.parents:
        raise HTTPException(status_code=400, detail="Invalid filename")
    tmp = target.with_name(target.name + ".part")
    if tmp.exists():
        tmp.unlink()

    fetch_proxy_enabled, fetch_proxy_url = await _runtime_proxy_config()
    client_kwargs = {
        "timeout": httpx.Timeout(settings.download_timeout_seconds, connect=20),
        "follow_redirects": False,
        "trust_env": settings.request_trust_env if not fetch_proxy_enabled else False,
        "headers": {"User-Agent": settings.request_user_agent, "Accept": "*/*"},
    }
    if fetch_proxy_enabled and fetch_proxy_url:
        client_kwargs["proxy"] = fetch_proxy_url
    max_bytes = settings.download_max_bytes
    written = 0
    try:
        async with httpx.AsyncClient(**client_kwargs) as client:
            response = await _request_with_redirect_validation(client, normalized, settings.allow_private_fetch_urls)
            async with response:
                response.raise_for_status()
                content_length = response.headers.get("content-length")
                if content_length and int(content_length) > max_bytes:
                    raise HTTPException(status_code=413, detail="Download file is too large")
                header_name = _filename_from_content_disposition(response.headers.get("content-disposition"))
                if header_name and not expected_name:
                    new_target = (DOWNLOAD_DIR / _safe_filename(header_name)).resolve()
                    if DOWNLOAD_DIR.resolve() not in new_target.parents:
                        raise HTTPException(status_code=400, detail="Invalid filename")
                    target = new_target
                    tmp = target.with_name(target.name + ".part")
                    if tmp.exists():
                        tmp.unlink()
                with tmp.open("wb") as fh:
                    async for chunk in response.aiter_bytes():
                        if not chunk:
                            continue
                        written += len(chunk)
                        if written > max_bytes:
                            raise HTTPException(status_code=413, detail="Download file is too large")
                        fh.write(chunk)
    except HTTPException:
        if tmp.exists():
            tmp.unlink()
        raise
    except httpx.HTTPStatusError as exc:
        if tmp.exists():
            tmp.unlink()
        raise HTTPException(status_code=502, detail=f"Download failed with HTTP {exc.response.status_code}") from exc
    except httpx.HTTPError as exc:
        if tmp.exists():
            tmp.unlink()
        raise HTTPException(status_code=502, detail=f"Download failed: {exc}") from exc

    tmp.replace(target)
    # 如果是预设下载，删除同前缀的旧文件（只保留最新一份）
    if cache_prefix:
        for old in DOWNLOAD_DIR.iterdir():
            if old.is_file() and old.name.startswith(cache_prefix) and old.resolve() != target:
                try:
                    old.unlink()
                except OSError:
                    pass
    stat = target.stat()
    return {
        "ok": True,
        "source": source,
        "filename": target.name,
        "size": stat.st_size,
        "mtime": datetime.fromtimestamp(stat.st_mtime, tz=timezone.utc).isoformat(),
        "download_url": f"/api/downloads/files/{target.name}",
    }


def get_download_file_response(filename: str) -> FileResponse:
    safe = _safe_filename(filename)
    if safe != filename:
        raise HTTPException(status_code=400, detail="Invalid filename")
    path = (DOWNLOAD_DIR / safe).resolve()
    if DOWNLOAD_DIR.resolve() not in path.parents or not path.is_file():
        raise HTTPException(status_code=404, detail="File not found")
    return FileResponse(path, filename=path.name, media_type="application/octet-stream")


async def _find_latest_github_asset(preset: dict[str, str]) -> dict[str, str]:
    api_url = GITHUB_API.format(owner=preset["owner"], repo=preset["repo"])
    pattern = re.compile(preset["pattern"])
    fetch_proxy_enabled, fetch_proxy_url = await _runtime_proxy_config()
    client_kwargs = {
        "timeout": settings.request_timeout_seconds,
        "trust_env": settings.request_trust_env if not fetch_proxy_enabled else False,
        "headers": {"User-Agent": settings.request_user_agent, "Accept": "application/vnd.github+json"},
    }
    if fetch_proxy_enabled and fetch_proxy_url:
        client_kwargs["proxy"] = fetch_proxy_url
    async with httpx.AsyncClient(**client_kwargs) as client:
        try:
            response = await client.get(api_url)
            response.raise_for_status()
        except httpx.HTTPStatusError as exc:
            raise HTTPException(status_code=502, detail=f"GitHub API returned HTTP {exc.response.status_code}") from exc
        except httpx.HTTPError as exc:
            raise HTTPException(status_code=502, detail=f"GitHub API request failed: {exc}") from exc
    data = response.json()
    assets = data.get("assets") or []
    matches = [asset for asset in assets if pattern.search(asset.get("name", ""))]
    if not matches:
        names = ", ".join(asset.get("name", "") for asset in assets[:20])
        raise HTTPException(status_code=404, detail=f"No matching release asset found. Assets: {names}")
    asset = matches[0]
    return {"name": asset["name"], "url": asset["browser_download_url"]}


def _safe_filename(filename: str) -> str:
    name = Path(filename.strip()).name
    name = re.sub(r"[^A-Za-z0-9._+()\[\]-]+", "_", name)
    if not name or name in {".", ".."}:
        raise HTTPException(status_code=400, detail="Invalid filename")
    return name[:180]


def _filename_from_url(url: str) -> str:
    parsed = urlparse(url)
    name = Path(unquote(parsed.path)).name
    return name or "download.bin"


def _filename_from_content_disposition(value: str | None) -> str | None:
    if not value:
        return None
    match = re.search(r"filename\*=UTF-8''([^;]+)", value, flags=re.I)
    if match:
        return unquote(match.group(1).strip().strip('"'))
    match = re.search(r'filename="?([^";]+)"?', value, flags=re.I)
    if match:
        return match.group(1).strip()
    return None


async def _runtime_proxy_config() -> tuple[bool, str]:
    async with AsyncSessionLocal() as db:
        security = await get_security_settings(db)
        return get_fetch_proxy_config(security)

from typing import Any

import hashlib

from fastapi import APIRouter, Depends, HTTPException, Request, Response
from fastapi.responses import JSONResponse
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.database import get_db
from app.schemas.security_settings import AuthCheckRead, AuthCheckRequest, SecuritySettingsRead, SecuritySettingsUpdate
from app.services.config_transfer_service import export_data, import_data, reset_data, validate_import_payload
from app.services.security_settings_service import get_security_settings, to_read, token_matches, update_security_settings
from app.utils.auth import AUTH_COOKIE, AUTH_HASH_COOKIE, extract_request_token, request_hash_cookie_matches


router = APIRouter(prefix="/settings", tags=["settings"])
settings = get_settings()


@router.get("/security", response_model=SecuritySettingsRead)
async def get_security_settings_endpoint(db: AsyncSession = Depends(get_db)) -> SecuritySettingsRead:
    return to_read(await get_security_settings(db))


@router.patch("/security", response_model=SecuritySettingsRead)
async def update_security_settings_endpoint(
    payload: SecuritySettingsUpdate,
    db: AsyncSession = Depends(get_db),
) -> SecuritySettingsRead:
    try:
        item = await update_security_settings(db, payload)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return to_read(item)


@router.post("/auth/check", response_model=AuthCheckRead)
async def check_auth_token_endpoint(
    request: Request,
    payload: AuthCheckRequest | None = None,
    db: AsyncSession = Depends(get_db),
) -> AuthCheckRead:
    item = await get_security_settings(db)
    raw_token = payload.token if payload else extract_request_token(request, allow_query=False)
    token_ok = token_matches(raw_token, item.token_hash) or request_hash_cookie_matches(request, item.token_hash)
    if item.auth_enabled and not token_ok:
        raise HTTPException(status_code=401, detail="Invalid token")
    return AuthCheckRead(ok=True)


@router.post("/auth/login", response_model=AuthCheckRead)
async def login_auth_token_endpoint(
    request: Request,
    response: Response,
    payload: AuthCheckRequest,
    db: AsyncSession = Depends(get_db),
) -> AuthCheckRead:
    item = await get_security_settings(db)
    if item.auth_enabled and not token_matches(payload.token, item.token_hash):
        raise HTTPException(status_code=401, detail="Invalid token")
    # Store SHA-256 hash of token instead of raw token (prevent credential theft via XSS)
    token_hash = hashlib.sha256(payload.token.encode()).hexdigest()
    response.set_cookie(
        AUTH_HASH_COOKIE,
        token_hash,
        httponly=True,
        secure=settings.auth_cookie_secure or request.url.scheme == "https",
        samesite="lax",
        max_age=60 * 60 * 24 * 30,
    )
    return AuthCheckRead(ok=True)


@router.post("/auth/logout", response_model=AuthCheckRead)
async def logout_auth_token_endpoint(response: Response) -> AuthCheckRead:
    response.delete_cookie(AUTH_COOKIE, samesite="lax")
    response.delete_cookie(AUTH_HASH_COOKIE, samesite="lax")
    return AuthCheckRead(ok=True)


@router.get("/export")
async def export_config_endpoint(
    include_subscriptions: bool = True,
    db: AsyncSession = Depends(get_db),
) -> JSONResponse:
    data = await export_data(db, include_subscriptions=include_subscriptions)

    headers = {
        "Content-Disposition": f'attachment; filename="clash-sub-parser-{"full" if include_subscriptions else "no-subscriptions"}.json"'
    }
    return JSONResponse(content=data, headers=headers)


@router.post("/reset", response_model=AuthCheckRead)
async def reset_config_endpoint(response: Response, db: AsyncSession = Depends(get_db)) -> AuthCheckRead:
    await reset_data(db)
    response.delete_cookie(AUTH_COOKIE, samesite="lax")
    response.delete_cookie(AUTH_HASH_COOKIE, samesite="lax")
    return AuthCheckRead(ok=True)


@router.post("/import")
async def import_config_endpoint(
    request: Request,
    db: AsyncSession = Depends(get_db),
) -> JSONResponse:
    """Import config from a previously exported JSON file.

    Accepts JSON body with { tables: { table_name: [rows...] } }.
    Clears existing data in imported tables, then inserts rows.
    All-or-nothing: if any table fails, the entire import is rolled back.
    """
    content_length = request.headers.get("content-length")
    if content_length and int(content_length) > settings.request_max_bytes:
        raise HTTPException(status_code=413, detail="Import payload too large")
    try:
        body: dict[str, Any] = await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid JSON body")

    imported = await import_data(db, body)

    return JSONResponse(content={
        "ok": True,
        "imported": imported,
        "errors": None,
    })


@router.post("/import/validate")
async def import_validate_endpoint(
    request: Request,
) -> JSONResponse:
    """Validate import JSON without writing anything.

    Returns { ok, tables: {name: row_count}, errors }.
    """
    try:
        body: dict[str, Any] = await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid JSON body")

    summary, errors = validate_import_payload(body.get("tables"))

    return JSONResponse(content={
        "ok": not errors,
        "tables": summary,
        "errors": errors if errors else None,
    })

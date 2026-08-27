from __future__ import annotations

from fastapi import Request
from fastapi.responses import JSONResponse

from app.database import AsyncSessionLocal
from app.services.security_settings_service import get_security_settings, token_matches
from app.utils.auth import (
    extract_request_token,
    is_api_path,
    is_export_path,
    is_frontend_path,
    is_public_path,
    is_unsafe_method,
    request_has_csrf_header,
    request_hash_cookie_matches,
    request_needs_auth,
    request_uses_cookie_auth,
)


async def token_auth_middleware(request: Request, call_next):
    if is_public_path(request.url.path):
        return await call_next(request)

    needs_auth = (
        is_api_path(request.url.path)
        or is_export_path(request.url.path)
        or is_frontend_path(request.url.path)
    )
    if not needs_auth:
        return await call_next(request)

    async with AsyncSessionLocal() as db:
        security = await get_security_settings(db)

    raw_token = extract_request_token(request, allow_query=is_export_path(request.url.path))
    token_ok = token_matches(raw_token, security.token_hash)

    # If not matched via header/query, try the HttpOnly hash cookie.
    if not token_ok:
        token_ok = request_hash_cookie_matches(request, security.token_hash)

    if request_needs_auth(request.url.path, security):
        if security.auth_enabled and not security.token_hash:
            return JSONResponse(
                status_code=503,
                content={"detail": "Auth is enabled but token is not configured"},
            )
        if not token_ok:
            return JSONResponse(
                status_code=401,
                content={"detail": "Invalid or missing token"},
            )
        if (
            is_api_path(request.url.path)
            and not is_export_path(request.url.path)
            and is_unsafe_method(request.method)
            and request_uses_cookie_auth(request)
            and not request_has_csrf_header(request)
        ):
            return JSONResponse(
                status_code=403,
                content={"detail": "Missing CSRF header"},
            )

    response = await call_next(request)
    return response

from contextlib import asynccontextmanager
from pathlib import Path
import hashlib
import logging

from fastapi import Depends, FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse, PlainTextResponse

from app.config import get_settings
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db, init_db
from app.database import AsyncSessionLocal
from app.routers import dns, downloads, generate, node_groups, rule_categories, rules, settings as settings_router, subscriptions
from app.services.generate_config_service import generate_config_to_switches, get_generate_config
from app.services.generate_service import generate_script, generate_yaml, get_primary_subscription_headers
from app.services.scheduler import shutdown_scheduler, start_scheduler
from app.services.security_settings_service import get_security_settings, token_matches
from app.utils.auth import extract_request_token, is_api_path, is_export_path, is_frontend_path, is_public_path, is_unsafe_method, request_has_csrf_header, request_needs_auth, request_uses_cookie_auth

logger = logging.getLogger(__name__)
AUTH_HASH_COOKIE = "clash_auth_hash"

settings = get_settings()
FRONTEND_DIST_DIR = Path(__file__).resolve().parents[1] / "frontend_dist"
FRONTEND_INDEX_FILE = FRONTEND_DIST_DIR / "index.html"


@asynccontextmanager
async def lifespan(_: FastAPI):
    await init_db()
    start_scheduler()
    yield
    shutdown_scheduler()


app = FastAPI(
    title=settings.app_name,
    version=settings.app_version,
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_allow_origins,
    allow_credentials=True,
    allow_methods=settings.cors_allow_methods,
    allow_headers=settings.cors_allow_headers,
)


@app.middleware("http")
async def token_auth_middleware(request, call_next):
    if is_public_path(request.url.path):
        return await call_next(request)

    needs_auth = is_api_path(request.url.path) or is_export_path(request.url.path) or is_frontend_path(request.url.path)
    if not needs_auth:
        return await call_next(request)

    async with AsyncSessionLocal() as db:
        security = await get_security_settings(db)

    raw_token = extract_request_token(request, allow_query=is_export_path(request.url.path))
    token_ok = token_matches(raw_token, security.token_hash)

    # If not matched via header/query, try hash cookie (SHA-256 of raw token)
    if not token_ok:
        hash_cookie = request.cookies.get(AUTH_HASH_COOKIE)
        if hash_cookie and security.token_hash:
            computed_hash = hashlib.sha256(security.token_hash.encode()).hexdigest()
            import secrets
            token_ok = secrets.compare_digest(hash_cookie, computed_hash)

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


@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    logger.exception("Unhandled exception on %s %s: %s", request.method, request.url.path, exc)
    return JSONResponse(
        status_code=500,
        content={"detail": "Internal server error"},
    )


app.include_router(subscriptions.router, prefix=settings.api_prefix)
app.include_router(node_groups.router, prefix=settings.api_prefix)
app.include_router(rule_categories.router, prefix=settings.api_prefix)
app.include_router(rules.router, prefix=settings.api_prefix)
app.include_router(dns.router, prefix=settings.api_prefix)
app.include_router(generate.router, prefix=settings.api_prefix)
app.include_router(downloads.router, prefix=settings.api_prefix)
app.include_router(settings_router.router, prefix=settings.api_prefix)


@app.get("/")
async def root():
    if FRONTEND_INDEX_FILE.is_file():
        return FileResponse(FRONTEND_INDEX_FILE)
    return {"message": "Clash Subscription Parser backend is running"}


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}


# NOTE: /yaml and /script are protected via token_auth_middleware → is_export_path.
# They must stay in EXPORT_PATHS set in app/utils/auth.py to remain protected.
@app.get("/yaml")
async def root_yaml(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    config = await get_generate_config(db)
    result = await generate_yaml(db, generate_config_to_switches(config))
    headers = {"Content-Disposition": 'inline; filename="config.yaml"'}
    headers.update(await get_primary_subscription_headers(db))
    return PlainTextResponse(
        content=result.get("yaml", ""),
        media_type="application/x-yaml",
        headers=headers,
    )


@app.get("/script")
async def root_script(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    config = await get_generate_config(db)
    result = await generate_script(db, generate_config_to_switches(config))
    return PlainTextResponse(
        content=result.get("script", ""),
        media_type="text/javascript",
        headers={"Content-Disposition": 'inline; filename="script.js"'},
    )


@app.get("/{full_path:path}", include_in_schema=False)
async def frontend_files(full_path: str):
    if not FRONTEND_INDEX_FILE.is_file():
        return {"message": "Clash Subscription Parser backend is running"}

    requested_path = (FRONTEND_DIST_DIR / full_path).resolve()
    dist_root = FRONTEND_DIST_DIR.resolve()

    if requested_path == dist_root or dist_root not in requested_path.parents:
        return FileResponse(FRONTEND_INDEX_FILE)

    if requested_path.is_file():
        return FileResponse(requested_path)

    return FileResponse(FRONTEND_INDEX_FILE)


if __name__ == "__main__":
    import uvicorn

    uvicorn.run("app.main:app", host=settings.host, port=settings.port, reload=False)

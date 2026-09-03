from __future__ import annotations

from contextlib import asynccontextmanager
import logging
from pathlib import Path
import time

from fastapi import Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse, PlainTextResponse
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.database import get_db, init_db
from app.logging_config import setup_logging
from app.middleware.auth import token_auth_middleware
from app.services.schema_readiness import check_database_readiness
from app.routers import (
    dns,
    downloads,
    generate,
    node_groups,
    probe,
    proxy_chains,
    rule_categories,
    rules,
    settings as settings_router,
    snapshots,
    subscriptions,
    v2,
)
from app.services.generate_service import file_response, render_current
from app.services.scheduler import shutdown_scheduler, start_scheduler
from app.utils.auth import is_public_path

logger = logging.getLogger(__name__)

settings = get_settings()
FRONTEND_DIST_DIR = Path(__file__).resolve().parents[1] / "frontend_dist"
FRONTEND_INDEX_FILE = FRONTEND_DIST_DIR / "index.html"


@asynccontextmanager
async def lifespan(_: FastAPI):
    setup_logging(json_mode=settings.log_json, level=settings.log_level)
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
async def auth_middleware(request, call_next):
    return await token_auth_middleware(request, call_next)


@app.middleware("http")
async def request_logging_middleware(request, call_next):
    if is_public_path(request.url.path):
        return await call_next(request)

    start = time.monotonic()
    response = await call_next(request)
    duration_ms = round((time.monotonic() - start) * 1000, 1)

    logger.info(
        "%s %s -> %s (%.1fms)",
        request.method,
        request.url.path,
        response.status_code,
        duration_ms,
        extra={
            "method": request.method,
            "path": request.url.path,
            "status_code": response.status_code,
            "duration_ms": duration_ms,
        },
    )
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
app.include_router(probe.router, prefix=settings.api_prefix)
app.include_router(proxy_chains.router, prefix=settings.api_prefix)
app.include_router(rule_categories.router, prefix=settings.api_prefix)
app.include_router(rules.router, prefix=settings.api_prefix)
app.include_router(dns.router, prefix=settings.api_prefix)
app.include_router(generate.router, prefix=settings.api_prefix)
app.include_router(downloads.router, prefix=settings.api_prefix)
app.include_router(settings_router.router, prefix=settings.api_prefix)
app.include_router(snapshots.router, prefix=settings.api_prefix)
app.include_router(v2.router)


@app.get("/")
async def root():
    if FRONTEND_INDEX_FILE.is_file():
        return FileResponse(FRONTEND_INDEX_FILE)
    return {"message": "Clash Subscription Parser backend is running"}


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/ready")
async def readiness(db: AsyncSession = Depends(get_db)) -> JSONResponse:
    report = await check_database_readiness(db)
    status_code = 200 if report["ready"] else 503
    return JSONResponse(status_code=status_code, content=report)


# yaml 与 script 路由由 token_auth_middleware 进行鉴权保护
@app.get("/yaml")
async def root_yaml(db: AsyncSession = Depends(get_db)) -> PlainTextResponse:
    content = await render_current(db, "yaml")
    return await file_response(db, content, "yaml", disposition="inline")


@app.get("/script")
async def root_script() -> PlainTextResponse:
    raise HTTPException(status_code=404, detail="SCRIPT format has been deprecated and removed")


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

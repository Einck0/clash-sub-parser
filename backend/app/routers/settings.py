from datetime import datetime
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, Request, Response, UploadFile
from fastapi.responses import JSONResponse
from sqlalchemy import DateTime, delete, select
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.database import get_db
from app.models.dns import DnsConfig
from app.models.generate_config import GenerateConfig
from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.models.security_settings import SecuritySettings
from app.models.subscription import Subscription
from app.schemas.security_settings import AuthCheckRead, AuthCheckRequest, SecuritySettingsRead, SecuritySettingsUpdate
from app.services.security_settings_service import get_security_settings, to_read, token_matches, update_security_settings
from app.utils.auth import AUTH_COOKIE, extract_request_token

router = APIRouter(prefix="/settings", tags=["settings"])
settings = get_settings()

EXPORT_MODELS = {
    "subscriptions": Subscription,
    "node_groups": NodeGroup,
    "rules": Rule,
    "rule_categories": RuleCategory,
    "dns_config": DnsConfig,
    "generate_config": GenerateConfig,
    "security_settings": SecuritySettings,
}

# Import order: parent tables first, children after
IMPORT_TABLE_ORDER = [
    "subscriptions",
    "rule_categories",
    "node_groups",
    "rules",
    "dns_config",
    "generate_config",
    "security_settings",
]

# Fields to skip during import (auto-managed or sensitive)
IMPORT_SKIP_FIELDS: dict[str, set[str]] = {
    "security_settings": {"token_hash"},
}


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
    if item.auth_enabled and not token_matches(raw_token, item.token_hash):
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
    response.set_cookie(
        AUTH_COOKIE,
        payload.token,
        httponly=True,
        secure=settings.auth_cookie_secure or request.url.scheme == "https",
        samesite="lax",
        max_age=60 * 60 * 24 * 30,
    )
    return AuthCheckRead(ok=True)


@router.post("/auth/logout", response_model=AuthCheckRead)
async def logout_auth_token_endpoint(response: Response) -> AuthCheckRead:
    response.delete_cookie(AUTH_COOKIE, samesite="lax")
    return AuthCheckRead(ok=True)


@router.get("/export")
async def export_config_endpoint(
    include_subscriptions: bool = True,
    db: AsyncSession = Depends(get_db),
) -> JSONResponse:
    data: dict[str, object] = {
        "version": 1,
        "exported_at": datetime.utcnow().isoformat(timespec="seconds") + "Z",
        "include_subscriptions": include_subscriptions,
        "tables": {},
    }
    tables: dict[str, list[dict]] = data["tables"]  # type: ignore[assignment]
    for table_name, model in EXPORT_MODELS.items():
        if table_name == "subscriptions" and not include_subscriptions:
            continue
        result = await db.execute(select(model))
        tables[table_name] = [_serialize_model(item) for item in result.scalars().all()]

    headers = {
        "Content-Disposition": f'attachment; filename="clash-sub-parser-{"full" if include_subscriptions else "no-subscriptions"}.json"'
    }
    return JSONResponse(content=data, headers=headers)


@router.post("/reset", response_model=AuthCheckRead)
async def reset_config_endpoint(response: Response, db: AsyncSession = Depends(get_db)) -> AuthCheckRead:
    for model in (Rule, RuleCategory, NodeGroup, Subscription, DnsConfig, GenerateConfig, SecuritySettings):
        await db.execute(delete(model))
    db.add(SecuritySettings(id=1))
    db.add(DnsConfig(id=1, raw_yaml="", enabled=True))
    db.add(GenerateConfig(id=1))
    await db.commit()
    response.delete_cookie(AUTH_COOKIE, samesite="lax")
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
    try:
        body: dict[str, Any] = await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid JSON body")

    tables = body.get("tables")
    if not isinstance(tables, dict):
        raise HTTPException(status_code=400, detail="Missing or invalid 'tables' field")

    imported: dict[str, int] = {}

    try:
        async with db.begin_nested():
            for table_name in IMPORT_TABLE_ORDER:
                rows = tables.get(table_name)
                if rows is None:
                    continue
                if not isinstance(rows, list):
                    raise HTTPException(status_code=400, detail=f"Table '{table_name}' rows must be an array")

                model = EXPORT_MODELS.get(table_name)
                if not model:
                    raise HTTPException(status_code=400, detail=f"Unknown table '{table_name}'")

                skip_fields = IMPORT_SKIP_FIELDS.get(table_name, set())

                preserved_token_hash = ""
                if table_name == "security_settings":
                    current_security = await db.get(SecuritySettings, 1)
                    preserved_token_hash = current_security.token_hash if current_security else ""

                # Clear existing rows for this table
                await db.execute(delete(model))

                # Identify datetime columns for this model
                datetime_columns: set[str] = set()
                for col in model.__table__.columns:
                    if isinstance(col.type, DateTime):
                        datetime_columns.add(col.name)

                inserted = 0
                for row_data in rows:
                    if not isinstance(row_data, dict):
                        continue
                    # Filter out skip fields and unknown columns
                    valid_columns = {c.name for c in model.__table__.columns}
                    filtered: dict[str, Any] = {}
                    for k, v in row_data.items():
                        if k not in valid_columns or k in skip_fields:
                            continue
                        # Parse ISO datetime strings back to datetime objects
                        if k in datetime_columns and isinstance(v, str):
                            try:
                                v = datetime.fromisoformat(v)
                            except ValueError:
                                pass
                        filtered[k] = v
                    if table_name == "security_settings":
                        filtered["token_hash"] = preserved_token_hash
                    db.add(model(**filtered))
                    inserted += 1

                if table_name == "security_settings" and not inserted:
                    db.add(SecuritySettings(id=1, token_hash=preserved_token_hash))
                    inserted = 1

                imported[table_name] = inserted

            # Flush inside savepoint so IntegrityError is caught here
            await db.flush()
    except HTTPException:
        raise
    except IntegrityError as exc:
        raise HTTPException(status_code=400, detail=f"数据完整性错误：{exc.orig}") from exc
    except SQLAlchemyError as exc:
        raise HTTPException(status_code=400, detail=f"数据库错误：{exc}") from exc

    await db.commit()

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

    tables = body.get("tables")
    if not isinstance(tables, dict):
        raise HTTPException(status_code=400, detail="Missing or invalid 'tables' field")

    summary: dict[str, int] = {}
    errors: dict[str, str] = {}

    for table_name, rows in tables.items():
        if not isinstance(rows, list):
            errors[table_name] = "Expected array of rows"
            continue
        if table_name not in EXPORT_MODELS:
            errors[table_name] = f"Unknown table '{table_name}'"
            continue
        summary[table_name] = len(rows)

    return JSONResponse(content={
        "ok": not errors,
        "tables": summary,
        "errors": errors if errors else None,
    })


def _serialize_model(item) -> dict:
    data = {}
    for column in item.__table__.columns:
        name = column.name
        if name in {"token_hash"}:
            continue
        value = getattr(item, name)
        if isinstance(value, datetime):
            value = value.isoformat()
        data[name] = value
    return data

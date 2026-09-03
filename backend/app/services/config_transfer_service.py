"""配置导出/导入/重置的领域逻辑

从 routers/settings.py 下沉：事务边界、外键顺序、敏感字段保留等规则单点在此，
router 只做参数解析与状态码映射。纯数据进出，可被 CLI 等非 HTTP 场景复用
"""
from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

from fastapi import HTTPException
from sqlalchemy import DateTime, delete, select
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.dns import DnsConfig
from app.models.generate_config import GenerateConfig
from app.models.node_group import NodeGroup
from app.models.node_probe_result import NodeProbeResult
from app.models.probe_config import ProbeConfig
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.models.security_settings import SecuritySettings
from app.models.subscription import Subscription

EXPORT_MODELS = {
    "subscriptions": Subscription,
    "node_groups": NodeGroup,
    "rules": Rule,
    "rule_categories": RuleCategory,
    "dns_config": DnsConfig,
    "generate_config": GenerateConfig,
    "security_settings": SecuritySettings,
    "probe_config": ProbeConfig,
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
    "probe_config",
]

# Fields to skip during import (auto-managed or sensitive)
IMPORT_SKIP_FIELDS: dict[str, set[str]] = {
    "security_settings": {"token_hash"},
}


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


async def export_data(db: AsyncSession, include_subscriptions: bool = True) -> dict:
    """按外键安全顺序序列化全部业务表，敏感字段（token_hash）不导出"""
    data: dict[str, object] = {
        "version": 1,
        "exported_at": datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z"),
        "include_subscriptions": include_subscriptions,
        "tables": {},
    }
    tables: dict[str, list[dict]] = data["tables"]  # type: ignore[assignment]
    for table_name, model in EXPORT_MODELS.items():
        if table_name == "subscriptions" and not include_subscriptions:
            continue
        result = await db.execute(select(model))
        tables[table_name] = [_serialize_model(item) for item in result.scalars().all()]
    return data


async def reset_data(db: AsyncSession) -> None:
    """按外键安全顺序清空业务表并恢复初始行；认证凭据重置为未设置"""
    for model in (Rule, RuleCategory, NodeGroup, Subscription, DnsConfig, GenerateConfig, SecuritySettings, ProbeConfig, NodeProbeResult):
        await db.execute(delete(model))
    db.add(SecuritySettings(id=1))
    db.add(DnsConfig(id=1, raw_yaml="", enabled=True))
    db.add(GenerateConfig(id=1))
    db.add(ProbeConfig(id=1))
    await db.commit()


async def import_data(db: AsyncSession, body: dict[str, Any]) -> dict[str, int]:
    """导入导出文档中的 tables 字段

    全部成功或整体回滚；security_settings.token_hash 保留当前值不被覆盖
    返回 {table_name: inserted_count}
    """
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

                await db.execute(delete(model))

                datetime_columns: set[str] = set()
                for col in model.__table__.columns:
                    if isinstance(col.type, DateTime):
                        datetime_columns.add(col.name)
                valid_columns = {c.name for c in model.__table__.columns}

                inserted = 0
                for row_data in rows:
                    if not isinstance(row_data, dict):
                        continue
                    filtered: dict[str, Any] = {}
                    for k, v in row_data.items():
                        if k not in valid_columns or k in skip_fields:
                            continue
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

            await db.flush()
    except IntegrityError as exc:
        raise HTTPException(status_code=400, detail=f"数据完整性错误：{exc.orig}") from exc
    except SQLAlchemyError as exc:
        raise HTTPException(status_code=400, detail=f"数据库错误：{exc}") from exc

    await db.commit()

    auth_check = await db.execute(select(SecuritySettings).where(SecuritySettings.id == 1))
    auth_row = auth_check.scalar_one_or_none()
    if auth_row and auth_row.auth_enabled and not auth_row.token_hash:
        auth_row.auth_enabled = False
        await db.commit()

    return imported


def validate_import_payload(tables: Any) -> tuple[dict[str, int], dict[str, str]]:
    """不写库的导入预检：返回 (summary, errors)"""
    summary: dict[str, int] = {}
    errors: dict[str, str] = {}
    if not isinstance(tables, dict):
        return summary, {"tables": "Missing or invalid 'tables' field"}
    for table_name, rows in tables.items():
        if not isinstance(rows, list):
            errors[table_name] = "Expected array of rows"
            continue
        if table_name not in EXPORT_MODELS:
            errors[table_name] = f"Unknown table '{table_name}'"
            continue
        summary[table_name] = len(rows)
    return summary, errors

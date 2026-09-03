from __future__ import annotations

import hashlib
import json
import uuid
from datetime import datetime, timezone
from typing import Any
from sqlalchemy import desc, select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.configuration_revision import ConfigurationRevision
from app.schemas.logical_config import LogicalConfigurationBundle
from app.services.bundle_preflight_service import preflight_bundle_json


def compute_bundle_checksum(bundle_data: dict[str, Any] | LogicalConfigurationBundle) -> str:
    """计算配置 Bundle 的确定性 SHA-256 校验和"""
    if isinstance(bundle_data, LogicalConfigurationBundle):
        raw_dict = bundle_data.model_dump()
    else:
        raw_dict = dict(bundle_data)

    # 排除不可预测的创建时间戳以确保相同内容指纹一致
    canonical_copy = dict(raw_dict)
    canonical_copy.pop("created_at", None)
    serialized = json.dumps(canonical_copy, sort_keys=True, ensure_ascii=False)
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


async def create_and_activate_revision(
    session: AsyncSession,
    bundle: LogicalConfigurationBundle,
    created_by: str = "system",
) -> ConfigurationRevision:
    """原子化创建并激活新配置版本，自动校验完整性并在事务内切换"""
    bundle_dict = bundle.model_dump()
    preflight = preflight_bundle_json(bundle_dict)
    if not preflight.valid:
        error_msgs = "; ".join([b.message for b in preflight.blockers])
        raise ValueError(f"配置 Bundle 预检失败，拒绝创建版本: {error_msgs}")

    checksum = compute_bundle_checksum(bundle_dict)

    # 将所有已有版本置为非激活状态
    await session.execute(
        update(ConfigurationRevision).values(is_active=False)
    )

    revision = ConfigurationRevision(
        logical_id=str(uuid.uuid4()),
        schema_version=bundle.schema_version,
        bundle_json=bundle_dict,
        checksum=checksum,
        is_active=True,
        created_at=datetime.now(timezone.utc),
        created_by=created_by,
    )
    session.add(revision)
    await session.flush()
    return revision


async def get_active_revision(
    session: AsyncSession,
) -> ConfigurationRevision | None:
    """获取当前激活的配置版本"""
    stmt = (
        select(ConfigurationRevision)
        .where(ConfigurationRevision.is_active.is_(True))
        .order_by(desc(ConfigurationRevision.created_at))
        .limit(1)
    )
    result = await session.execute(stmt)
    return result.scalar_one_or_none()


async def rollback_to_revision(
    session: AsyncSession,
    revision_logical_id: str,
) -> ConfigurationRevision:
    """安全回滚至指定配置版本"""
    stmt = select(ConfigurationRevision).where(
        ConfigurationRevision.logical_id == revision_logical_id
    )
    res = await session.execute(stmt)
    target = res.scalar_one_or_none()
    if not target:
        raise ValueError(f"目标配置版本 '{revision_logical_id}' 不存在")

    # 切换激活状态
    await session.execute(
        update(ConfigurationRevision).values(is_active=False)
    )
    target.is_active = True
    await session.flush()
    return target


async def list_revision_history(
    session: AsyncSession,
    limit: int = 50,
    offset: int = 0,
) -> list[ConfigurationRevision]:
    """查询配置历史版本列表"""
    stmt = (
        select(ConfigurationRevision)
        .order_by(desc(ConfigurationRevision.created_at))
        .limit(limit)
        .offset(offset)
    )
    res = await session.execute(stmt)
    return list(res.scalars().all())


async def cleanup_old_revisions(
    session: AsyncSession,
    retention_count: int = 30,
) -> int:
    """清理超出保留数量的陈旧未激活配置版本"""
    stmt = select(ConfigurationRevision).order_by(desc(ConfigurationRevision.created_at))
    res = await session.execute(stmt)
    all_revisions = list(res.scalars().all())

    if len(all_revisions) <= retention_count:
        return 0

    deleted_count = 0
    # 保护最近 retention_count 个版本以及当前激活版本
    protected_ids = set()
    for r in all_revisions[:retention_count]:
        protected_ids.add(r.id)
    for r in all_revisions:
        if r.is_active:
            protected_ids.add(r.id)

    for r in all_revisions[retention_count:]:
        if r.id not in protected_ids:
            await session.delete(r)
            deleted_count += 1

    await session.flush()
    return deleted_count

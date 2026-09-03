from __future__ import annotations

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.probe_config import ProbeConfig
from app.schemas.probe import ProbeSettingsRead, ProbeSettingsUpdate


async def get_probe_config(db: AsyncSession) -> ProbeConfig:
    """获取节点探测与测速设置，确保 singleton 行存在"""
    result = await db.execute(select(ProbeConfig).where(ProbeConfig.id == 1))
    item = result.scalar_one_or_none()
    if not item:
        item = ProbeConfig(id=1)
        db.add(item)
        await db.commit()
        await db.refresh(item)
    return item


async def update_probe_config(db: AsyncSession, payload: ProbeSettingsUpdate) -> ProbeConfig:
    """更新节点探测与测速设置"""
    item = await get_probe_config(db)

    for field, value in payload.model_dump(exclude_unset=True).items():
        if value is not None:
            setattr(item, field, value)

    await db.commit()
    await db.refresh(item)
    return item


def to_read(item: ProbeConfig) -> ProbeSettingsRead:
    """将 ORM 模型转换为 Pydantic 读取模式"""
    return ProbeSettingsRead(
        probe_enabled=item.probe_enabled,
        speedtest_enabled=item.speedtest_enabled,
        speedtest_url=item.speedtest_url,
        speedtest_timeout_s=item.speedtest_timeout_s,
        speedtest_max_bytes=item.speedtest_max_bytes,
        speedtest_min_speed_mbps=item.speedtest_min_speed_mbps,
        media_check_enabled=item.media_check_enabled,
        media_platforms=item.media_platforms or ["youtube", "netflix", "disney", "chatgpt", "bilibili"],
        media_timeout_s=item.media_timeout_s,
        probe_concurrency=item.probe_concurrency,
        probe_timeout_ms=item.probe_timeout_ms,
    )

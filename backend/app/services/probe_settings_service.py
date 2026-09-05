from __future__ import annotations

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.probe_config import ProbeConfig
from app.schemas.probe import ProbeSettingsRead, ProbeSettingsUpdate, ResolvedProbeConfig


def resolve_probe_config(
    saved: ProbeConfig,
    *,
    client_concurrency: int | None = None,
    client_timeout_ms: int | None = None,
    client_include_speed: bool | None = None,
    client_include_media: bool | None = None,
) -> ResolvedProbeConfig:
    """从数据库持久化 ProbeConfig 与客户端请求解析生成不可变的批次配置快照"""
    persisted_concurrency = saved.probe_concurrency if saved.probe_concurrency is not None else 10
    bounded_persisted = max(1, min(persisted_concurrency, 20))

    if client_concurrency is not None:
        # 手动覆盖只能降低并发，不能超过服务端持久化配置上限
        effective_concurrency = max(1, min(client_concurrency, bounded_persisted))
    else:
        effective_concurrency = bounded_persisted

    # 节点级全局超时预算（兼容旧字段）
    if client_timeout_ms is not None:
        node_timeout_s = (client_timeout_ms / 1000.0) if client_timeout_ms > 0 else None
    else:
        raw_budget = saved.probe_timeout_ms
        node_timeout_s = (raw_budget / 1000.0) if (raw_budget is not None and raw_budget > 0) else None

    service_timeout_s = max(0.5, float(saved.probe_service_timeout_ms or 2000) / 1000.0)

    speed_enabled = client_include_speed if client_include_speed is not None else bool(saved.speedtest_enabled)
    media_enabled = client_include_media if client_include_media is not None else bool(saved.media_check_enabled)

    media_platforms_list = saved.media_platforms or ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"]

    return ResolvedProbeConfig(
        probe_enabled=bool(saved.probe_enabled),
        speedtest_enabled=speed_enabled,
        speedtest_url=saved.speedtest_url,
        speedtest_max_bytes=saved.speedtest_max_bytes,
        speedtest_timeout_s=float(saved.speedtest_timeout_s or 5.0),
        media_check_enabled=media_enabled,
        media_platforms=tuple(media_platforms_list),
        media_timeout_s=float(saved.media_timeout_s or 5.0),
        concurrency=effective_concurrency,
        service_timeout_s=service_timeout_s,
        node_timeout_s=node_timeout_s,
        probe_cron_enabled=True if saved.probe_cron_enabled is None else bool(saved.probe_cron_enabled),
        probe_cron_interval_minutes=int(saved.probe_cron_interval_minutes or 60),
    )


async def get_probe_config(db: AsyncSession) -> ProbeConfig:
    """获取节点探测与测速设置，确保 singleton 行存在并惰性回填缺失字段"""
    result = await db.execute(select(ProbeConfig).where(ProbeConfig.id == 1))
    item = result.scalar_one_or_none()
    dirty = False
    if not item:
        item = ProbeConfig(id=1)
        db.add(item)
        dirty = True
    else:
        # 惰性回填缺失字段，但不改写已显式持久化的旧并发度
        if item.probe_service_timeout_ms is None:
            item.probe_service_timeout_ms = 2000
            dirty = True
        if item.probe_cron_enabled is None:
            item.probe_cron_enabled = True
            dirty = True
        if item.probe_cron_interval_minutes is None:
            item.probe_cron_interval_minutes = 60
            dirty = True
        if item.probe_concurrency is None:
            item.probe_concurrency = 10
            dirty = True
        if item.probe_timeout_ms is None:
            item.probe_timeout_ms = 3000
            dirty = True

    if dirty:
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
        probe_enabled=bool(item.probe_enabled),
        probe_interval_minutes=item.probe_interval_minutes or 0,
        speedtest_enabled=bool(item.speedtest_enabled),
        speedtest_url=item.speedtest_url,
        speedtest_timeout_s=item.speedtest_timeout_s,
        speedtest_max_bytes=item.speedtest_max_bytes,
        speedtest_min_speed_mbps=item.speedtest_min_speed_mbps,
        media_check_enabled=bool(item.media_check_enabled),
        media_platforms=item.media_platforms or ["youtube", "netflix", "disney", "chatgpt", "bilibili", "meta_ai", "gemini"],
        media_timeout_s=item.media_timeout_s,
        probe_concurrency=item.probe_concurrency if item.probe_concurrency is not None else 10,
        probe_service_timeout_ms=item.probe_service_timeout_ms if item.probe_service_timeout_ms is not None else 2000,
        probe_timeout_ms=item.probe_timeout_ms if item.probe_timeout_ms is not None else 3000,
        probe_cron_enabled=True if item.probe_cron_enabled is None else bool(item.probe_cron_enabled),
        probe_cron_interval_minutes=item.probe_cron_interval_minutes if item.probe_cron_interval_minutes is not None else 60,
    )

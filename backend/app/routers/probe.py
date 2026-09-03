"""节点连通性、出口识别、流媒体解锁与测速 API"""
from __future__ import annotations

from typing import Any

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, Field
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.probe import (
    ProbeBatchRequest,
    ProbeNodeRequest,
    ProbeSettingsRead,
    ProbeSettingsUpdate,
)
from app.services.probe.service import (
    clear_probe_cache,
    delete_all_db_probe_results,
    get_all_cached_results,
    get_all_db_probe_results,
    probe_batch_nodes,
    probe_single_node,
)
from app.services.probe_settings_service import (
    get_probe_config,
    to_read,
    update_probe_config,
)
from app.services.tcp_probe_service import (
    DEFAULT_CONCURRENCY,
    DEFAULT_TIMEOUT_MS,
    MAX_NODES,
    probe_nodes,
)

router = APIRouter(prefix="/probe", tags=["probe"])


class TcpProbeRequest(BaseModel):
    nodes: list[dict[str, Any]] = Field(default_factory=list)
    timeout_ms: int = DEFAULT_TIMEOUT_MS
    concurrency: int = DEFAULT_CONCURRENCY


@router.get("/settings", response_model=ProbeSettingsRead)
async def get_probe_settings_endpoint(db: AsyncSession = Depends(get_db)) -> ProbeSettingsRead:
    """获取节点探测与测速设置"""
    item = await get_probe_config(db)
    return to_read(item)


@router.patch("/settings", response_model=ProbeSettingsRead)
async def update_probe_settings_endpoint(
    payload: ProbeSettingsUpdate,
    db: AsyncSession = Depends(get_db),
) -> ProbeSettingsRead:
    """更新节点探测与测速设置"""
    item = await update_probe_config(db, payload)
    return to_read(item)


@router.post("/tcp")
async def probe_tcp(payload: TcpProbeRequest) -> dict[str, Any]:
    """轻量级 TCP 端口可达性探测（保留向后兼容）"""
    if not payload.nodes:
        raise HTTPException(status_code=400, detail="nodes is required")
    if len(payload.nodes) > MAX_NODES:
        raise HTTPException(
            status_code=400,
            detail=f"Too many nodes (max {MAX_NODES})",
        )
    return await probe_nodes(
        payload.nodes,
        timeout_ms=payload.timeout_ms,
        concurrency=payload.concurrency,
    )


@router.get("/results")
async def get_probe_results_endpoint(
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """获取所有已持久化的最新节点探测结果"""
    return await get_all_db_probe_results(db)


@router.delete("/results")
async def delete_probe_results_endpoint(
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """清空所有已持久化的节点探测结果"""
    await delete_all_db_probe_results(db)
    return {"ok": True}


@router.post("/node")
async def probe_node_endpoint(
    payload: ProbeNodeRequest,
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """对单个节点进行全协议真实代理握手、出口定位与流媒体/测速检测"""
    config = await get_probe_config(db)

    speed_enabled = payload.include_speed if payload.include_speed is not None else config.speedtest_enabled
    media_enabled = payload.include_media if payload.include_media is not None else config.media_check_enabled

    return await probe_single_node(
        payload.node,
        probe_enabled=config.probe_enabled,
        speedtest_enabled=speed_enabled,
        media_check_enabled=media_enabled,
        media_platforms=config.media_platforms,
        speedtest_url=config.speedtest_url,
        speedtest_max_bytes=config.speedtest_max_bytes,
        speedtest_timeout_s=config.speedtest_timeout_s,
        probe_timeout_ms=config.probe_timeout_ms,
        use_cache=payload.use_cache,
        db=db,
    )


@router.post("/batch")
async def probe_batch_endpoint(
    payload: ProbeBatchRequest,
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """批量对节点列表执行全协议真实出站探测与流媒体检测"""
    if not payload.nodes:
        raise HTTPException(status_code=400, detail="nodes is required")
    if len(payload.nodes) > MAX_NODES:
        raise HTTPException(
            status_code=400,
            detail=f"Too many nodes (max {MAX_NODES})",
        )

    config = await get_probe_config(db)
    concurrency = payload.concurrency or config.probe_concurrency
    timeout_ms = payload.timeout_ms or config.probe_timeout_ms
    speed_enabled = payload.include_speed if payload.include_speed is not None else config.speedtest_enabled
    media_enabled = payload.include_media if payload.include_media is not None else config.media_check_enabled

    return await probe_batch_nodes(
        payload.nodes,
        probe_enabled=config.probe_enabled,
        speedtest_enabled=speed_enabled,
        media_check_enabled=media_enabled,
        media_platforms=config.media_platforms,
        speedtest_url=config.speedtest_url,
        speedtest_max_bytes=config.speedtest_max_bytes,
        speedtest_timeout_s=config.speedtest_timeout_s,
        probe_timeout_ms=timeout_ms,
        concurrency=concurrency,
        use_cache=payload.use_cache,
        db=db,
    )


@router.get("/cache")
async def get_probe_cache_endpoint() -> dict[str, Any]:
    """获取当前所有有效的探测结果缓存"""
    return {"cache": get_all_cached_results()}


@router.delete("/cache")
async def clear_probe_cache_endpoint() -> dict[str, Any]:
    """清空所有探测结果缓存"""
    clear_probe_cache()
    return {"ok": True}

"""节点连通性、出口识别、流媒体解锁与测速 API"""
from __future__ import annotations

from typing import Any

from fastapi import APIRouter, Depends, HTTPException, Query
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
    get_db_probe_detail,
    get_paged_db_probe_summary,
    probe_batch_nodes,
    probe_single_node,
)
from app.services.probe_settings_service import (
    get_probe_config,
    resolve_probe_config,
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


@router.get("/results/detail")
async def get_probe_result_detail_endpoint(
    node_key: str = Query(..., description="精确节点唯一标识"),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """获取指定单个节点的完整探测与证据链详情"""
    if not node_key or not node_key.strip():
        raise HTTPException(status_code=422, detail="node_key must not be blank")
    detail = await get_db_probe_detail(db, node_key=node_key)
    if not detail:
        raise HTTPException(status_code=404, detail="Probe result not found")
    return detail


@router.get("/results")
async def get_probe_results_endpoint(
    cursor: str | None = None,
    limit: int = Query(default=100, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """获取所有已持久化的最新节点探测结果轻量级摘要（带分页与 50KB 严格限额）"""
    return await get_paged_db_probe_summary(db, cursor=cursor, limit=limit)


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
    resolved = resolve_probe_config(
        config,
        client_include_speed=payload.include_speed,
        client_include_media=payload.include_media,
    )

    return await probe_single_node(
        payload.node,
        resolved_config=resolved,
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
    resolved = resolve_probe_config(
        config,
        client_concurrency=payload.concurrency,
        client_timeout_ms=payload.timeout_ms,
        client_include_speed=payload.include_speed,
        client_include_media=payload.include_media,
    )

    return await probe_batch_nodes(
        payload.nodes,
        resolved_config=resolved,
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

from __future__ import annotations

import logging

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models.node import Node
from app.models.source import NodeSourceLink, SourceRevision
from app.schemas.inventory import (
    InventoryListResponse,
    InventoryNodeDetail,
    InventoryNodeItem,
    SafeSourceProvenance,
    sanitize_payload_secrets,
)

logger = logging.getLogger(__name__)


async def list_inventory_nodes(
    session: AsyncSession,
    lifecycle_state: str | None = "active",
    protocol: str | None = None,
    keyword: str | None = None,
    page: int = 1,
    page_size: int = 50,
) -> InventoryListResponse:
    """查询规范化库存节点列表并附带脱敏来源追溯与分页"""
    page = max(1, page)
    page_size = max(1, min(200, page_size))
    offset = (page - 1) * page_size

    # 构建基础过滤条件
    stmt = select(Node)
    count_stmt = select(func.count(Node.id))

    if lifecycle_state:
        stmt = stmt.where(Node.lifecycle_state == lifecycle_state)
        count_stmt = count_stmt.where(Node.lifecycle_state == lifecycle_state)
    if protocol:
        stmt = stmt.where(Node.protocol == protocol)
        count_stmt = count_stmt.where(Node.protocol == protocol)
    if keyword:
        pattern = f"%{keyword}%"
        filter_expr = or_(Node.name.like(pattern), Node.server.like(pattern))
        stmt = stmt.where(filter_expr)
        count_stmt = count_stmt.where(filter_expr)

    total_count = (await session.execute(count_stmt)).scalar_one() or 0

    stmt = (
        stmt.order_by(Node.id.desc())
        .offset(offset)
        .limit(page_size)
        .options(
            selectinload(Node.source_links)
            .selectinload(NodeSourceLink.source_revision)
            .selectinload(SourceRevision.source)
        )
    )

    result = await session.execute(stmt)
    nodes = list(result.scalars().all())

    items: list[InventoryNodeItem] = []
    for node in nodes:
        sources: list[SafeSourceProvenance] = []
        for link in node.source_links:
            rev = link.source_revision
            src = rev.source if rev else None
            if src and rev:
                sources.append(
                    SafeSourceProvenance(
                        source_logical_id=src.logical_id,
                        source_name=src.name,
                        source_kind=src.kind,
                        revision_id=rev.revision_id,
                        display_name=link.display_name,
                        original_order=link.original_order,
                    )
                )

        items.append(
            InventoryNodeItem(
                logical_id=node.logical_id,
                name=node.name,
                protocol=node.protocol,
                server=node.server,
                port=node.port,
                lifecycle_state=node.lifecycle_state,
                sources=sources,
                created_at=node.created_at,
                updated_at=node.updated_at,
            )
        )

    return InventoryListResponse(
        total=total_count,
        page=page,
        page_size=page_size,
        items=items,
    )


async def get_inventory_node_detail(
    session: AsyncSession,
    logical_id: str,
) -> InventoryNodeDetail | None:
    """获取单节点详情并对敏感字段进行安全脱敏"""
    stmt = (
        select(Node)
        .where(Node.logical_id == logical_id)
        .options(
            selectinload(Node.source_links)
            .selectinload(NodeSourceLink.source_revision)
            .selectinload(SourceRevision.source),
            selectinload(Node.observations),
        )
    )
    result = await session.execute(stmt)
    node = result.scalar_one_or_none()
    if not node:
        return None

    sources: list[SafeSourceProvenance] = []
    for link in node.source_links:
        rev = link.source_revision
        src = rev.source if rev else None
        if src and rev:
            sources.append(
                SafeSourceProvenance(
                    source_logical_id=src.logical_id,
                    source_name=src.name,
                    source_kind=src.kind,
                    revision_id=rev.revision_id,
                    display_name=link.display_name,
                    original_order=link.original_order,
                )
            )

    latest_obs_dict = None
    if node.observations:
        latest = node.observations[0]
        latest_obs_dict = {
            "status": latest.status,
            "latency_ms": latest.latency_ms,
            "speed_mbps": latest.speed_mbps,
            "country": latest.country,
            "observed_at": latest.observed_at.isoformat() if latest.observed_at else None,
        }

    sanitized = sanitize_payload_secrets(node.normalized_payload or {})

    return InventoryNodeDetail(
        logical_id=node.logical_id,
        name=node.name,
        protocol=node.protocol,
        server=node.server,
        port=node.port,
        lifecycle_state=node.lifecycle_state,
        payload_fingerprint=node.payload_fingerprint,
        sanitized_payload=sanitized,
        sources=sources,
        latest_observation=latest_obs_dict,
        created_at=node.created_at,
        updated_at=node.updated_at,
    )
